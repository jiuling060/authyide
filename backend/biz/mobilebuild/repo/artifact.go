package repo

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
	"github.com/chaitin/MonkeyCode/backend/db"
	"github.com/chaitin/MonkeyCode/backend/db/appbuildartifact"
	"github.com/chaitin/MonkeyCode/backend/db/appbuildjob"
	"github.com/chaitin/MonkeyCode/backend/pkg/entx"
)

// SaveArtifacts replaces the channel set of one job. Deletion is soft: the
// uq_app_build_artifacts_job_channel index is live-only, so a retried worker
// can re-insert the same (job_id, channel) pair without tripping the
// constraint.
func (r *appBuilderRepo) SaveArtifacts(ctx context.Context, jobID uuid.UUID, platform string, artifacts []domain.Artifact) error {
	return entx.WithTx2(ctx, r.db, func(tx *db.Tx) error {
		return r.saveArtifactsTx(ctx, tx, jobID, platform, artifacts)
	})
}

func (r *appBuilderRepo) saveArtifactsTx(ctx context.Context, tx *db.Tx, jobID uuid.UUID, platform string, artifacts []domain.Artifact) error {
	if len(artifacts) == 0 {
		return fmt.Errorf("save build artifacts: empty set")
	}
	if _, err := tx.AppBuildArtifact.Delete().Where(appbuildartifact.JobID(jobID)).Exec(ctx); err != nil {
		return fmt.Errorf("replace build artifacts: %w", err)
	}
	for _, art := range artifacts {
		if art.Channel == "" {
			return fmt.Errorf("save build artifacts: empty channel")
		}
		if _, err := tx.AppBuildArtifact.Create().
			SetJobID(jobID).
			SetPlatform(platform).
			SetChannel(art.Channel).
			SetFileName(art.FileName).
			SetArtifactPath(art.Path).
			SetArtifactSha256(art.SHA256).
			SetArtifactSize(art.Size).
			Save(ctx); err != nil {
			return fmt.Errorf("insert build artifact %s: %w", art.Channel, err)
		}
	}
	return nil
}

// ListArtifacts returns every channel of one job. DurationMS comes from the
// job: the artifact table records no per-channel timing.
func (r *appBuilderRepo) ListArtifacts(ctx context.Context, jobID uuid.UUID) ([]*domain.ArtifactInfo, error) {
	rows, err := r.db.AppBuildArtifact.Query().
		Where(appbuildartifact.JobID(jobID)).
		Order(db.Asc(appbuildartifact.FieldChannel)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list build artifacts: %w", err)
	}
	info, err := r.jobDuration(ctx, jobID)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.ArtifactInfo, 0, len(rows))
	for _, row := range rows {
		out = append(out, artifactInfo(row, info))
	}
	return out, nil
}

// GetArtifact returns one channel of one job, nil when absent.
func (r *appBuilderRepo) GetArtifact(ctx context.Context, jobID uuid.UUID, channel string) (*domain.ArtifactInfo, error) {
	row, err := r.db.AppBuildArtifact.Query().
		Where(appbuildartifact.JobID(jobID), appbuildartifact.Channel(channel)).
		Only(ctx)
	if err != nil && db.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get build artifact: %w", err)
	}
	info, err := r.jobDuration(ctx, jobID)
	if err != nil {
		return nil, err
	}
	return artifactInfo(row, info), nil
}

func (r *appBuilderRepo) jobDuration(ctx context.Context, jobID uuid.UUID) (int64, error) {
	job, err := r.db.AppBuildJob.Query().Where(appbuildjob.ID(jobID)).Only(ctx)
	if err != nil && db.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get build job: %w", err)
	}
	return job.DurationMs, nil
}

func artifactInfo(row *db.AppBuildArtifact, durationMS int64) *domain.ArtifactInfo {
	return &domain.ArtifactInfo{
		Channel:    row.Channel,
		FileName:   row.FileName,
		Size:       row.ArtifactSize,
		SHA256:     row.ArtifactSha256,
		DurationMS: durationMS,
		CreatedAt:  row.CreatedAt,
		Path:       row.ArtifactPath,
	}
}
