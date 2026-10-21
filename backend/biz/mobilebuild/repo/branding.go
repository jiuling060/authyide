// Package repo is the ent persistence layer of the mobile app builder.
package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/samber/do"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
	"github.com/chaitin/MonkeyCode/backend/db"
	"github.com/chaitin/MonkeyCode/backend/db/appbranding"
	"github.com/chaitin/MonkeyCode/backend/db/appbuildjob"
)

const brandingSingletonKey = "authyide"

type appBuilderRepo struct {
	db *db.Client
}

// NewAppBuilderRepo creates the app builder repository.
func NewAppBuilderRepo(i *do.Injector) (domain.AppBuilderRepo, error) {
	return &appBuilderRepo{db: do.MustInvoke[*db.Client](i)}, nil
}

func (r *appBuilderRepo) GetBranding(ctx context.Context) (*domain.Branding, error) {
	row, err := r.db.AppBranding.Query().First(ctx)
	if err != nil && db.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get app branding: %w", err)
	}
	return brandingFromRow(row), nil
}

func (r *appBuilderRepo) SaveBranding(ctx context.Context, req *domain.UpdateBrandingReq) (*domain.Branding, error) {
	create := r.db.AppBranding.Create().SetSingletonKey(brandingSingletonKey)
	applyBrandingCreate(req, create)
	err := create.
		OnConflictColumns(appbranding.FieldSingletonKey).
		Update(func(u *db.AppBrandingUpsert) {
			applyBrandingUpsert(req, u)
		}).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("upsert app branding: %w", err)
	}
	return r.GetBranding(ctx)
}

func (r *appBuilderRepo) SetAssetKey(ctx context.Context, slot, key string) (*domain.Branding, error) {
	if !domain.AssetSlotValid(slot) {
		return nil, fmt.Errorf("invalid asset slot %q", slot)
	}
	create := r.db.AppBranding.Create().SetSingletonKey(brandingSingletonKey)
	applyAssetKeyCreate(slot, key, create)
	err := create.
		OnConflictColumns(appbranding.FieldSingletonKey).
		Update(func(u *db.AppBrandingUpsert) {
			applyAssetKeyUpsert(slot, key, u)
		}).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("upsert app branding: %w", err)
	}
	return r.GetBranding(ctx)
}

func (r *appBuilderRepo) ClearAssetKey(ctx context.Context, slot string) (*domain.Branding, error) {
	return r.SetAssetKey(ctx, slot, "")
}

func applyBrandingCreate(req *domain.UpdateBrandingReq, c *db.AppBrandingCreate) {
	if req.Name != nil {
		c.SetName(*req.Name)
	}
	if req.Version != nil {
		c.SetVersion(*req.Version)
	}
	if req.VersionCode != nil {
		c.SetVersionCode(*req.VersionCode)
	}
	if req.ServerURL != nil {
		c.SetServerURL(*req.ServerURL)
	}
	if req.UpdatesServer != nil {
		c.SetUpdatesServer(*req.UpdatesServer)
	}
}

func applyBrandingUpsert(req *domain.UpdateBrandingReq, u *db.AppBrandingUpsert) {
	if req.Name != nil {
		u.SetName(*req.Name)
	}
	if req.Version != nil {
		u.SetVersion(*req.Version)
	}
	if req.VersionCode != nil {
		u.SetVersionCode(*req.VersionCode)
	}
	if req.ServerURL != nil {
		u.SetServerURL(*req.ServerURL)
	}
	if req.UpdatesServer != nil {
		u.SetUpdatesServer(*req.UpdatesServer)
	}
}

func applyAssetKeyCreate(slot, key string, c *db.AppBrandingCreate) {
	switch slot {
	case domain.SlotLogoLight:
		c.SetLogoLightKey(key)
	case domain.SlotLogoDark:
		c.SetLogoDarkKey(key)
	case domain.SlotIcon:
		c.SetIconKey(key)
	case domain.SlotIconDark:
		c.SetIconDarkKey(key)
	case domain.SlotAdaptiveIcon:
		c.SetAdaptiveIconKey(key)
	case domain.SlotSplash:
		c.SetSplashKey(key)
	case domain.SlotSplashDark:
		c.SetSplashDarkKey(key)
	}
}

func applyAssetKeyUpsert(slot, key string, u *db.AppBrandingUpsert) {
	switch slot {
	case domain.SlotLogoLight:
		u.SetLogoLightKey(key)
	case domain.SlotLogoDark:
		u.SetLogoDarkKey(key)
	case domain.SlotIcon:
		u.SetIconKey(key)
	case domain.SlotIconDark:
		u.SetIconDarkKey(key)
	case domain.SlotAdaptiveIcon:
		u.SetAdaptiveIconKey(key)
	case domain.SlotSplash:
		u.SetSplashKey(key)
	case domain.SlotSplashDark:
		u.SetSplashDarkKey(key)
	}
}

func brandingFromRow(row *db.AppBranding) *domain.Branding {
	if row == nil {
		return nil
	}
	return &domain.Branding{
		Name:          row.Name,
		Version:       row.Version,
		VersionCode:   row.VersionCode,
		ServerURL:     row.ServerURL,
		UpdatesServer: row.UpdatesServer,
		Assets: []domain.AssetSlotInfo{
			{Slot: domain.SlotLogoLight, Key: row.LogoLightKey, Set: row.LogoLightKey != ""},
			{Slot: domain.SlotLogoDark, Key: row.LogoDarkKey, Set: row.LogoDarkKey != ""},
			{Slot: domain.SlotIcon, Key: row.IconKey, Set: row.IconKey != ""},
			{Slot: domain.SlotIconDark, Key: row.IconDarkKey, Set: row.IconDarkKey != ""},
			{Slot: domain.SlotAdaptiveIcon, Key: row.AdaptiveIconKey, Set: row.AdaptiveIconKey != ""},
			{Slot: domain.SlotSplash, Key: row.SplashKey, Set: row.SplashKey != ""},
			{Slot: domain.SlotSplashDark, Key: row.SplashDarkKey, Set: row.SplashDarkKey != ""},
		},
	}
}

func (r *appBuilderRepo) CreateJob(ctx context.Context, userID uuid.UUID, req *domain.EnqueueBuildReq) (*domain.BuildJob, error) {
	create := r.db.AppBuildJob.Create().
		SetUserID(userID).
		SetPlatform(req.Platform).
		SetTargetVersion(req.Version).
		SetVersionCode(req.VersionCode).
		SetStatus(domain.BuildStatusQueued).
		SetEnqueuedAt(time.Now())
	if req.AppID != uuid.Nil {
		create = create.SetNillableAppID(&req.AppID)
	}
	row, err := create.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("create app build job: %w", err)
	}
	return jobFromRow(row), nil
}

func (r *appBuilderRepo) ListJobs(ctx context.Context, limit int) ([]*domain.BuildJob, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.AppBuildJob.Query().
		Order(db.Desc(appbuildjob.FieldCreatedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list app build jobs: %w", err)
	}
	jobs := make([]*domain.BuildJob, 0, len(rows))
	for _, row := range rows {
		jobs = append(jobs, jobFromRow(row))
	}
	return jobs, nil
}

func (r *appBuilderRepo) GetJob(ctx context.Context, id uuid.UUID) (*domain.BuildJob, error) {
	row, err := r.db.AppBuildJob.Query().Where(appbuildjob.ID(id)).Only(ctx)
	if err != nil && db.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get app build job: %w", err)
	}
	return jobFromRow(row), nil
}

func (r *appBuilderRepo) AnyRunning(ctx context.Context) (bool, error) {
	n, err := r.db.AppBuildJob.Query().
		Where(appbuildjob.StatusEQ(domain.BuildStatusRunning)).
		Count(ctx)
	if err != nil {
		return false, fmt.Errorf("count running app build jobs: %w", err)
	}
	return n > 0, nil
}

func (r *appBuilderRepo) PickOldestQueued(ctx context.Context) (*domain.BuildJob, error) {
	row, err := r.db.AppBuildJob.Query().
		Where(appbuildjob.StatusEQ(domain.BuildStatusQueued)).
		Order(db.Asc(appbuildjob.FieldEnqueuedAt)).
		First(ctx)
	if err != nil && db.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("pick queued app build job: %w", err)
	}
	return jobFromRow(row), nil
}

// ClaimJob switches a queued job to running. The status condition makes the
// switch atomic, so two workers can never claim the same job.
func (r *appBuilderRepo) ClaimJob(ctx context.Context, id uuid.UUID, workerID string) (bool, error) {
	err := r.db.AppBuildJob.UpdateOneID(id).
		Where(appbuildjob.StatusEQ(domain.BuildStatusQueued)).
		SetStatus(domain.BuildStatusRunning).
		SetWorkerID(workerID).
		SetNillableStartedAt(timePtr(time.Now())).
		Exec(ctx)
	if err != nil {
		if db.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("claim app build job: %w", err)
	}
	return true, nil
}

func (r *appBuilderRepo) SetJobLogPath(ctx context.Context, id uuid.UUID, path string) error {
	err := r.db.AppBuildJob.UpdateOneID(id).SetLogPath(path).Exec(ctx)
	if err != nil {
		return fmt.Errorf("set app build job log path: %w", err)
	}
	return nil
}

func (r *appBuilderRepo) SetJobStep(ctx context.Context, id uuid.UUID, step string) error {
	err := r.db.AppBuildJob.UpdateOneID(id).SetCurrentStep(step).Exec(ctx)
	if err != nil {
		return fmt.Errorf("set app build job step: %w", err)
	}
	return nil
}

func (r *appBuilderRepo) SetJobLogTail(ctx context.Context, id uuid.UUID, tail string) error {
	err := r.db.AppBuildJob.UpdateOneID(id).SetLogTail(tail).Exec(ctx)
	if err != nil {
		return fmt.Errorf("set app build job log tail: %w", err)
	}
	return nil
}

func (r *appBuilderRepo) FinishJob(ctx context.Context, id uuid.UUID, res domain.JobResult) error {
	update := r.db.AppBuildJob.UpdateOneID(id).
		SetStatus(res.Status).
		SetNillableFinishedAt(timePtr(time.Now())).
		SetDurationMs(res.DurationMS).
		SetArtifactSize(res.ArtifactSize)
	if res.Error != "" {
		update = update.SetError(res.Error)
	}
	if res.ExitCode != nil {
		update = update.SetNillableExitCode(res.ExitCode)
	}
	if res.ArtifactPath != "" {
		update = update.SetArtifactPath(res.ArtifactPath)
	}
	if res.ArtifactSHA256 != "" {
		update = update.SetArtifactSha256(res.ArtifactSHA256)
	}
	if err := update.Exec(ctx); err != nil {
		return fmt.Errorf("finish app build job: %w", err)
	}
	return nil
}

func (r *appBuilderRepo) FailAllRunning(ctx context.Context, reason string) error {
	err := r.db.AppBuildJob.Update().
		Where(appbuildjob.StatusEQ(domain.BuildStatusRunning)).
		SetStatus(domain.BuildStatusFailed).
		SetError(reason).
		SetNillableFinishedAt(timePtr(time.Now())).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("fail running app build jobs: %w", err)
	}
	return nil
}

func (r *appBuilderRepo) MaxVersionCode(ctx context.Context) (int, error) {
	row, err := r.db.AppBuildJob.Query().
		Order(db.Desc(appbuildjob.FieldVersionCode)).
		First(ctx)
	if err != nil && db.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("max app build version code: %w", err)
	}
	return row.VersionCode, nil
}

func (r *appBuilderRepo) MarkJobPublished(ctx context.Context, id uuid.UUID, artifactSHA256 string) error {
	err := r.db.AppBuildJob.UpdateOneID(id).
		SetNillablePublishedAt(timePtr(time.Now())).
		SetArtifactSha256(artifactSHA256).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("mark app build job published: %w", err)
	}
	return nil
}

func timePtr(t time.Time) *time.Time {
	return &t
}

// jobAppID unwraps the nullable app_id; admin-console jobs have none.
func jobAppID(v *uuid.UUID) uuid.UUID {
	if v == nil {
		return uuid.Nil
	}
	return *v
}

func jobFromRow(row *db.AppBuildJob) *domain.BuildJob {
	if row == nil {
		return nil
	}
	return &domain.BuildJob{
		ID:             row.ID,
		AppID:          jobAppID(row.AppID),
		Platform:       row.Platform,
		TargetVersion:  row.TargetVersion,
		VersionCode:    row.VersionCode,
		Status:         row.Status,
		CurrentStep:    row.CurrentStep,
		Error:          row.Error,
		ExitCode:       row.ExitCode,
		LogTail:        row.LogTail,
		ArtifactPath:   row.ArtifactPath,
		ArtifactSHA256: row.ArtifactSha256,
		ArtifactSize:   row.ArtifactSize,
		DurationMS:     row.DurationMs,
		EnqueuedAt:     row.EnqueuedAt,
		StartedAt:      row.StartedAt,
		FinishedAt:     row.FinishedAt,
		PublishedAt:    row.PublishedAt,
	}
}
