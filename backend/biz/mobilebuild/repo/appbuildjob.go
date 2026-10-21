package repo

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
	"github.com/chaitin/MonkeyCode/backend/consts"
	"github.com/chaitin/MonkeyCode/backend/db"
	"github.com/chaitin/MonkeyCode/backend/db/app"
	"github.com/chaitin/MonkeyCode/backend/db/appbuildjob"
	"github.com/chaitin/MonkeyCode/backend/db/entitlement"
	"github.com/chaitin/MonkeyCode/backend/pkg/entx"
)

// ListByApp returns one user's jobs for one app, newest first.
func (r *appBuilderRepo) ListByApp(ctx context.Context, userID, appID uuid.UUID, limit int) ([]*domain.BuildJob, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.AppBuildJob.Query().
		Where(appbuildjob.UserID(userID), appbuildjob.AppIDEQ(appID)).
		Order(db.Desc(appbuildjob.FieldEnqueuedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list app build jobs: %w", err)
	}
	out := make([]*domain.BuildJob, 0, len(rows))
	for _, row := range rows {
		out = append(out, jobFromRow(row))
	}
	return out, nil
}

// GetAppBranding returns the per-app build inputs of one user-center app,
// nil when the app is absent or deleted.
func (r *appBuilderRepo) GetAppBranding(ctx context.Context, appID uuid.UUID) (*domain.AppBranding, error) {
	row, err := r.db.App.Query().Where(app.ID(appID)).Only(ctx)
	if err != nil {
		if db.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get app branding: %w", err)
	}
	return &domain.AppBranding{
		ID:               row.ID,
		Name:             row.Name,
		PackageID:        row.PackageID,
		AdNetwork:        row.AdNetwork,
		AdAppID:          row.AdAppID,
		EnabledPlatforms: row.EnabledPlatforms,
	}, nil
}

// GetOwnedJob returns one of the user's own jobs; it is nil when the job is
// absent or belongs to someone else.
func (r *appBuilderRepo) GetOwnedJob(ctx context.Context, userID, id uuid.UUID) (*domain.BuildJob, error) {
	row, err := r.db.AppBuildJob.Query().
		Where(appbuildjob.ID(id), appbuildjob.UserID(userID)).
		Only(ctx)
	if err != nil {
		if db.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get app build job: %w", err)
	}
	return jobFromRow(row), nil
}

// HasActiveJobForUser reports whether the user has a queued or running job.
func (r *appBuilderRepo) HasActiveJobForUser(ctx context.Context, userID uuid.UUID) (bool, error) {
	n, err := r.db.AppBuildJob.Query().
		Where(appbuildjob.UserID(userID),
			appbuildjob.StatusIn(domain.BuildStatusQueued, domain.BuildStatusRunning)).
		Count(ctx)
	if err != nil {
		return false, fmt.Errorf("count active jobs for user: %w", err)
	}
	return n > 0, nil
}

// CountJobsToday counts one user's jobs across every app in the last 24 hours.
func (r *appBuilderRepo) CountJobsToday(ctx context.Context, userID uuid.UUID) (int, error) {
	n, err := r.db.AppBuildJob.Query().
		Where(appbuildjob.UserID(userID),
			appbuildjob.EnqueuedAtGTE(time.Now().Add(-24*time.Hour))).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count app build jobs today: %w", err)
	}
	return n, nil
}

// CountQueued counts every queued job, used to enforce the queue depth cap.
func (r *appBuilderRepo) CountQueued(ctx context.Context) (int, error) {
	n, err := r.db.AppBuildJob.Query().
		Where(appbuildjob.StatusEQ(domain.BuildStatusQueued)).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("count queued app build jobs: %w", err)
	}
	return n, nil
}

// MaxVersionCodeForApp returns the highest version code already used by one app.
func (r *appBuilderRepo) MaxVersionCodeForApp(ctx context.Context, appID uuid.UUID) (int, error) {
	row, err := r.db.AppBuildJob.Query().
		Where(appbuildjob.AppIDEQ(appID)).
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

// DeleteOwnedJob removes one of the user's own jobs.
func (r *appBuilderRepo) DeleteOwnedJob(ctx context.Context, userID, id uuid.UUID) error {
	_, err := r.db.AppBuildJob.Delete().
		Where(appbuildjob.ID(id), appbuildjob.UserID(userID)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete app build job: %w", err)
	}
	return nil
}

// CompleteForApp closes one self-service job. On a succeeded build the job
// status update, the app build_count increment and the owner's
// free_builds_used increment commit or roll back together. A failed build
// only lands its own terminal state: no counter moves, so the free build is
// not spent and the user can retry.
func (r *appBuilderRepo) CompleteForApp(ctx context.Context, jobID, appID uuid.UUID, res domain.JobResult) error {
	if res == (domain.JobResult{}) {
		return fmt.Errorf("complete app build job: empty result")
	}
	return entx.WithTx2(ctx, r.db, func(tx *db.Tx) error {
		row, err := tx.AppBuildJob.Query().
			Where(appbuildjob.ID(jobID), appbuildjob.AppIDEQ(appID)).
			Only(ctx)
		if err != nil {
			return fmt.Errorf("get app build job: %w", err)
		}

		updateJob := tx.AppBuildJob.UpdateOneID(row.ID).
			SetStatus(res.Status).
			SetNillableFinishedAt(timePtr(time.Now())).
			SetDurationMs(res.DurationMS).
			SetArtifactSize(res.ArtifactSize)
		if res.Error != "" {
			updateJob = updateJob.SetError(res.Error)
		}
		if res.ExitCode != nil {
			updateJob = updateJob.SetNillableExitCode(res.ExitCode)
		}
		if res.ArtifactPath != "" {
			updateJob = updateJob.SetArtifactPath(res.ArtifactPath)
		}
		if res.ArtifactSHA256 != "" {
			updateJob = updateJob.SetArtifactSha256(res.ArtifactSHA256)
		}
		if err := updateJob.Exec(ctx); err != nil {
			return fmt.Errorf("finish app build job: %w", err)
		}

		// Only a succeeded build advances the counters. A failed one lands in
		// its terminal state with the app's build_count and the user's
		// free_builds_used untouched, so the user can retry.
		if res.Status != domain.BuildStatusSucceeded {
			return nil
		}

		ent, err := tx.Entitlement.Query().Where(entitlement.UserIDEQ(row.UserID)).Only(ctx)
		if err != nil {
			return fmt.Errorf("get user entitlement: %w", err)
		}

		// The free tier grants exactly one successful build, so reaching that
		// count freezes the app for edits; higher tiers keep building and the
		// app stays unlocked.
		appUpdate := tx.App.UpdateOneID(appID).
			AddBuildCount(1).
			SetLastBuiltAt(time.Now()).
			SetLastBuildPlatform(row.Platform).
			SetLastBuildArtifactKey(res.ArtifactPath).
			Where(app.ID(appID))
		if consts.PlanTier(ent.Tier) == consts.PlanTierFree {
			appUpdate = appUpdate.SetLocked(true)
		}
		if err := appUpdate.Exec(ctx); err != nil {
			return fmt.Errorf("update app build stats: %w", err)
		}

		if err := tx.Entitlement.UpdateOneID(ent.ID).
			SetFreeBuildsUsed(ent.FreeBuildsUsed + 1).
			Exec(ctx); err != nil {
			return fmt.Errorf("increment user free builds used: %w", err)
		}
		return nil
	})
}
