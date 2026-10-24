package usecase

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/samber/do"

	"github.com/chaitin/MonkeyCode/backend/config"
	"github.com/chaitin/MonkeyCode/backend/consts"
	"github.com/chaitin/MonkeyCode/backend/db"
	"github.com/chaitin/MonkeyCode/backend/domain"
	"github.com/chaitin/MonkeyCode/backend/errcode"
	"github.com/chaitin/MonkeyCode/backend/pkg/cvt"
)

var validPlatforms = map[string]bool{
	"web":       true,
	"android":   true,
	"ios":       true,
	"harmonyos": true,
	"desktop":   true,
}

type appUsecase struct {
	repo          domain.AppRepo
	builds        domain.AppBuildService
	cfg           *config.Config
	logger        *slog.Logger
	networks      []domain.AdNetworkOption
	validNetworks map[string]bool
	// snapshots 校验绑定的会话源码快照归属、就绪状态与过期状态。
	snapshots domain.SnapshotService
}

func NewAppUsecase(i *do.Injector) (domain.AppQuotaService, error) {
	logger := do.MustInvoke[*slog.Logger](i)
	networks := loadAdNetworks(logger)
	return &appUsecase{
		repo:          do.MustInvoke[domain.AppRepo](i),
		builds:        do.MustInvoke[domain.AppBuildService](i),
		cfg:           do.MustInvoke[*config.Config](i),
		logger:        logger,
		networks:      networks,
		validNetworks: adNetworkSet(networks),
		snapshots:     do.MustInvoke[domain.SnapshotService](i),
	}, nil
}

func (u *appUsecase) List(ctx context.Context, uid uuid.UUID, req *domain.ListAppsReq) (*domain.ListAppsResp, error) {
	rows, err := u.repo.List(ctx, uid)
	if err != nil {
		u.logger.ErrorContext(ctx, "failed to list apps", "error", err, "user_id", uid)
		return nil, errcode.ErrDatabaseQuery.Wrap(err)
	}

	ent, err := u.repo.EnsureEntitlement(ctx, uid)
	if err != nil {
		return nil, errcode.ErrDatabaseQuery.Wrap(err)
	}

	apps := cvt.Iter(rows, func(_ int, row *db.App) *domain.App {
		return u.toApp(row, ent.Tier)
	})
	return &domain.ListAppsResp{
		Apps:      apps,
		Tier:      ent.Tier,
		HostQuota: consts.HostQuotaForTier(consts.PlanTier(ent.Tier)),
	}, nil
}

func (u *appUsecase) Get(ctx context.Context, uid, id uuid.UUID) (*domain.App, error) {
	row, err := u.repo.Get(ctx, uid, id)
	if err != nil {
		return nil, errcode.ErrAppNotFound
	}
	ent, err := u.repo.EnsureEntitlement(ctx, uid)
	if err != nil {
		return nil, errcode.ErrDatabaseQuery.Wrap(err)
	}
	return u.toApp(row, ent.Tier), nil
}

func (u *appUsecase) Create(ctx context.Context, uid uuid.UUID, req *domain.CreateAppReq) (*domain.App, error) {
	adNetwork := req.AdNetwork
	if adNetwork == "" {
		adNetwork = "none"
	}
	if !u.validNetworks[adNetwork] {
		return nil, errcode.ErrAppAdNetworkInvalid
	}
	if err := u.validatePlatforms(req.EnabledPlatforms); err != nil {
		return nil, err
	}
	if len(req.EnabledPlatforms) == 0 {
		req.EnabledPlatforms = []string{"web"}
	}
	if err := u.validateSnapshotBinding(ctx, uid, req.SourceSnapshotID); err != nil {
		return nil, err
	}
	req.AdNetwork = adNetwork
	if adNetwork == "none" {
		req.AdAppID = ""
	}

	row, err := u.repo.Create(ctx, uid, req)
	if err != nil {
		// idx_apps_user_package_id (partial unique on user_id, package_id)
		// rejects a second app with the same package id; map it to the domain
		// code instead of leaking a raw constraint error.
		if db.IsConstraintError(err) {
			return nil, errcode.ErrAppPackageIDConflict
		}
		u.logger.ErrorContext(ctx, "failed to create app", "error", err, "user_id", uid)
		return nil, errcode.ErrDatabaseOperation.Wrap(err)
	}

	ent, err := u.repo.EnsureEntitlement(ctx, uid)
	if err != nil {
		return nil, errcode.ErrDatabaseQuery.Wrap(err)
	}
	return u.toApp(row, ent.Tier), nil
}

func (u *appUsecase) Update(ctx context.Context, uid, id uuid.UUID, req *domain.UpdateAppReq) (*domain.App, error) {
	current, err := u.repo.Get(ctx, uid, id)
	if err != nil {
		return nil, errcode.ErrAppNotFound
	}
	// Locked apps are frozen: no field of them can be edited in the user center.
	if current.Locked {
		return nil, errcode.ErrAppLocked
	}
	if err := u.validateSnapshotBinding(ctx, uid, updateSnapshotID(req.SourceSnapshotID)); err != nil {
		return nil, err
	}

	adNetwork := current.AdNetwork
	if req.AdNetwork != nil {
		adNetwork = *req.AdNetwork
	}
	if !u.validNetworks[adNetwork] {
		return nil, errcode.ErrAppAdNetworkInvalid
	}

	platforms := current.EnabledPlatforms
	if req.EnabledPlatforms != nil {
		platforms = req.EnabledPlatforms
	}
	if err := u.validatePlatforms(platforms); err != nil {
		return nil, err
	}
	if len(platforms) == 0 {
		platforms = []string{"web"}
	}
	req.EnabledPlatforms = platforms

	// "none" must never carry an ad app id, even when the caller omits the
	// field, so a direct API call cannot leave a contradictory row behind.
	if adNetwork == "none" && (req.AdAppID == nil || *req.AdAppID != "") {
		empty := ""
		req.AdAppID = &empty
	}

	row, err := u.repo.Update(ctx, uid, id, req)
	if err != nil {
		return nil, errcode.ErrDatabaseOperation.Wrap(err)
	}

	ent, err := u.repo.EnsureEntitlement(ctx, uid)
	if err != nil {
		return nil, errcode.ErrDatabaseQuery.Wrap(err)
	}
	return u.toApp(row, ent.Tier), nil
}

// SetSource 单独换绑会话源码快照。req 为 nil 或 SourceSnapshotID 为 nil 都表示
// 解绑回模板源码。
func (u *appUsecase) SetSource(ctx context.Context, uid, id uuid.UUID, req *domain.SetSourceAppReq) (*domain.App, error) {
	current, err := u.repo.Get(ctx, uid, id)
	if err != nil {
		return nil, errcode.ErrAppNotFound
	}
	if current.Locked {
		return nil, errcode.ErrAppLocked
	}

	var target *uuid.UUID
	if req != nil {
		target = req.SourceSnapshotID
	}
	if err := u.validateSnapshotBinding(ctx, uid, target); err != nil {
		return nil, err
	}

	row, err := u.repo.Update(ctx, uid, id, &domain.UpdateAppReq{ID: id, SourceSnapshotID: &target})
	if err != nil {
		return nil, errcode.ErrDatabaseOperation.Wrap(err)
	}
	ent, err := u.repo.EnsureEntitlement(ctx, uid)
	if err != nil {
		return nil, errcode.ErrDatabaseQuery.Wrap(err)
	}
	return u.toApp(row, ent.Tier), nil
}

func (u *appUsecase) Delete(ctx context.Context, uid, id uuid.UUID) error {
	if _, err := u.repo.Get(ctx, uid, id); err != nil {
		return errcode.ErrAppNotFound
	}
	if err := u.repo.Delete(ctx, uid, id); err != nil {
		return errcode.ErrDatabaseOperation.Wrap(err)
	}
	return nil
}

// AdNetworks returns the ad network options from packer/ad_networks.yaml.
// The frontend renders its picker from this list, so both sides share one source.
func (u *appUsecase) AdNetworks(ctx context.Context) ([]domain.AdNetworkOption, error) {
	return u.networks, nil
}

// EnqueueBuild starts a packaging job after the quota check passes.
func (u *appUsecase) EnqueueBuild(ctx context.Context, uid, appID uuid.UUID, req *domain.EnqueueAppBuildReq) (*domain.AppBuildJob, error) {
	app, err := u.repo.Get(ctx, uid, appID)
	if err != nil {
		return nil, errcode.ErrAppNotFound
	}
	if err := u.canBuild(ctx, uid, app); err != nil {
		return nil, err
	}
	snap, err := u.sourceSnapshotForBuild(ctx, uid, app)
	if err != nil {
		return nil, err
	}
	var snapID, taskID *uuid.UUID
	if snap != nil {
		snapID = &snap.ID
		taskID = &snap.TaskID
	}
	return u.builds.Enqueue(ctx, domain.AppBuildInput{
		AppID:            appID,
		Owner:            uid,
		Platform:         req.Platform,
		Version:          req.Version,
		VersionCode:      req.VersionCode,
		SourceSnapshotID: snapID,
		SourceTaskID:     taskID,
	})
}

func (u *appUsecase) ListBuilds(ctx context.Context, uid, appID uuid.UUID) ([]*domain.AppBuildJob, error) {
	if _, err := u.repo.Get(ctx, uid, appID); err != nil {
		return nil, errcode.ErrAppNotFound
	}
	return u.builds.List(ctx, uid, appID)
}

func (u *appUsecase) GetBuild(ctx context.Context, uid, id uuid.UUID) (*domain.AppBuildJob, error) {
	return u.builds.Get(ctx, uid, id)
}

func (u *appUsecase) DownloadBuild(ctx context.Context, uid, id uuid.UUID, channel string) (domain.AppBuildArtifact, error) {
	return u.builds.Download(ctx, uid, id, channel)
}

func (u *appUsecase) PublishBuild(ctx context.Context, uid, id uuid.UUID, channel string) (*domain.AppBuildRelease, error) {
	return u.builds.Publish(ctx, uid, id, channel)
}

// ListAppBuilds is the admin view of one app's history: it skips the owner
// check, because an admin reads every tenant's builds.
func (u *appUsecase) ListAppBuilds(ctx context.Context, appID uuid.UUID) ([]*domain.AppBuildRecord, error) {
	app, err := u.repo.GetByID(ctx, appID)
	if err != nil {
		return nil, errcode.ErrAppNotFound
	}
	jobs, err := u.builds.ListAll(ctx, appID)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.AppBuildRecord, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, &domain.AppBuildRecord{
			Job:       j,
			AppName:   app.Name,
			PackageID: app.PackageID,
			OwnerID:   app.UserID,
		})
	}
	return out, nil
}

func (u *appUsecase) DeleteBuild(ctx context.Context, uid, id uuid.UUID) error {
	return u.builds.Delete(ctx, uid, id)
}

func (u *appUsecase) UploadIcon(ctx context.Context, uid, appID uuid.UUID, data []byte) error {
	if _, err := u.repo.Get(ctx, uid, appID); err != nil {
		return errcode.ErrAppNotFound
	}
	return u.builds.UploadIcon(ctx, uid, appID, data)
}

func (u *appUsecase) ClearIcon(ctx context.Context, uid, appID uuid.UUID) error {
	if _, err := u.repo.Get(ctx, uid, appID); err != nil {
		return errcode.ErrAppNotFound
	}
	return u.builds.ClearIcon(ctx, uid, appID)
}

func (u *appUsecase) DownloadIcon(ctx context.Context, uid, appID uuid.UUID) (domain.AppBuildArtifact, error) {
	if _, err := u.repo.Get(ctx, uid, appID); err != nil {
		return domain.AppBuildArtifact{}, errcode.ErrAppNotFound
	}
	return u.builds.DownloadIcon(ctx, uid, appID)
}

func (u *appUsecase) Platforms(ctx context.Context, uid, appID uuid.UUID, desktopLicensed bool) ([]domain.AppBuildPlatform, error) {
	if _, err := u.repo.Get(ctx, uid, appID); err != nil {
		return nil, errcode.ErrAppNotFound
	}
	return u.builds.Platforms(ctx, uid, appID, desktopLicensed)
}

func (u *appUsecase) GetDesktopBranding(ctx context.Context, uid, appID uuid.UUID) (*domain.AppDesktopBranding, error) {
	if _, err := u.repo.Get(ctx, uid, appID); err != nil {
		return nil, errcode.ErrAppNotFound
	}
	return u.builds.GetDesktopBranding(ctx, uid, appID)
}

func (u *appUsecase) SaveDesktopBranding(ctx context.Context, uid, appID uuid.UUID, req *domain.AppUpdateDesktopBrandingReq) (*domain.AppDesktopBranding, error) {
	if _, err := u.repo.Get(ctx, uid, appID); err != nil {
		return nil, errcode.ErrAppNotFound
	}
	return u.builds.SaveDesktopBranding(ctx, uid, appID, req)
}

// canBuild is the packaging quota check. The free tier gets one successful
// build per app; the pro tier additionally respects the optional daily rate
// limit, which is unlimited when the configured limit is not greater than 0.
func (u *appUsecase) canBuild(ctx context.Context, uid uuid.UUID, app *db.App) error {
	ent, err := u.repo.EnsureEntitlement(ctx, uid)
	if err != nil {
		return errcode.ErrDatabaseQuery.Wrap(err)
	}
	if app.Locked {
		return errcode.ErrAppLocked
	}
	if consts.PlanTier(ent.Tier) == consts.PlanTierFree && app.BuildCount >= consts.FreeAppBuildsPerApp {
		return errcode.ErrAppBuildQuotaExceeded
	}
	if consts.PlanTier(ent.Tier) == consts.PlanTierPro {
		if limit := u.cfg.MobileBuild.DailyBuildLimit; limit > 0 {
			n, err := u.builds.CountToday(ctx, uid)
			if err != nil {
				return err
			}
			if n >= limit {
				return errcode.ErrBuildDailyQuotaExceeded
			}
		}
	}
	return nil
}

func (u *appUsecase) validatePlatforms(platforms []string) error {
	for _, p := range platforms {
		if !validPlatforms[p] {
			return errcode.ErrAppInvalidPlatform
		}
	}
	return nil
}

// validateSnapshotBinding 校验绑定的会话源码快照属于该用户、已就绪且未过期。
// id 为 nil 表示不绑定快照、沿用模板源码，直接放行。
func (u *appUsecase) validateSnapshotBinding(ctx context.Context, uid uuid.UUID, id *uuid.UUID) error {
	if id == nil {
		return nil
	}
	_, err := u.sourceSnapshotForBuild(ctx, uid, &db.App{SourceSnapshotID: id})
	return err
}

// sourceSnapshotForBuild 重查 App 的快照绑定：绑定值可能在上次编辑后过期或被
// 同会话更新取代。返回 nil 表示这次任务用模板源码。
func (u *appUsecase) sourceSnapshotForBuild(ctx context.Context, uid uuid.UUID, app *db.App) (*domain.Snapshot, error) {
	if app == nil || app.SourceSnapshotID == nil {
		return nil, nil
	}
	snap, err := u.snapshots.Get(ctx, uid, *app.SourceSnapshotID)
	if err != nil || snap == nil {
		return nil, errcode.ErrAppSourceSnapshotUnbound
	}
	if snap.Superseded {
		return nil, errcode.ErrAppSourceSnapshotUnbound
	}
	if snap.Stale {
		return nil, errcode.ErrSnapshotStale
	}
	return snap, nil
}

// updateSnapshotID 把三级指针摊平：外层 nil 表示请求未提及该字段，内层 nil 表示
// 显式解绑回模板源码。
func updateSnapshotID(v **uuid.UUID) *uuid.UUID {
	if v == nil {
		return nil
	}
	return *v
}

func (u *appUsecase) toApp(row *db.App, tier string) *domain.App {
	j := cvt.From(row, &domain.App{})
	j.Tier = tier
	if j.EnabledPlatforms == nil {
		j.EnabledPlatforms = []string{}
	}
	return j
}
