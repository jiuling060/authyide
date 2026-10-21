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
	return u.builds.Enqueue(ctx, domain.AppBuildInput{
		AppID:       appID,
		Owner:       uid,
		Platform:    req.Platform,
		Version:     req.Version,
		VersionCode: req.VersionCode,
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

func (u *appUsecase) DownloadBuild(ctx context.Context, uid, id uuid.UUID) (domain.AppBuildArtifact, error) {
	return u.builds.Download(ctx, uid, id)
}

func (u *appUsecase) PublishBuild(ctx context.Context, uid, id uuid.UUID) (*domain.AppBuildRelease, error) {
	return u.builds.Publish(ctx, uid, id)
}

func (u *appUsecase) DeleteBuild(ctx context.Context, uid, id uuid.UUID) error {
	return u.builds.Delete(ctx, uid, id)
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

func (u *appUsecase) toApp(row *db.App, tier string) *domain.App {
	j := cvt.From(row, &domain.App{})
	j.Tier = tier
	if j.EnabledPlatforms == nil {
		j.EnabledPlatforms = []string{}
	}
	return j
}
