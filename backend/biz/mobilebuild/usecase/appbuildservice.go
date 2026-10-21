package usecase

import (
	"context"

	"github.com/google/uuid"
	"github.com/samber/do"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
	appdomain "github.com/chaitin/MonkeyCode/backend/domain"
	"github.com/chaitin/MonkeyCode/backend/errcode"
)

const (
	appBuildContentType = "application/vnd.android.package-archive"
	appBuildPathBase    = "/api/v1/users/apps"
	appBuildListLimit   = 100
)

// appBuildService is the user-center side of the app builder. Every method is
// owner-scoped at the repository, so a cross-user request reads like "not
// found" instead of "exists but belongs to someone else".
type appBuildService struct {
	uc   Usecase
	repo domain.AppBuilderRepo
}

// NewAppBuildService exposes the app builder to the user center.
func NewAppBuildService(i *do.Injector) (appdomain.AppBuildService, error) {
	uc, err := NewUsecase(i)
	if err != nil {
		return nil, err
	}
	return &appBuildService{
		uc:   uc,
		repo: do.MustInvoke[domain.AppBuilderRepo](i),
	}, nil
}

func (s *appBuildService) Enqueue(ctx context.Context, in appdomain.AppBuildInput) (*appdomain.AppBuildJob, error) {
	if !s.uc.Enabled() {
		return nil, errcode.ErrBrandingInvalid
	}
	job, err := s.uc.EnqueueBuildForApp(ctx, in.Owner, in.AppID, &domain.EnqueueBuildReq{
		Platform:    in.Platform,
		Version:     in.Version,
		VersionCode: in.VersionCode,
	})
	if err != nil {
		return nil, err
	}
	return fromJob(job), nil
}

func (s *appBuildService) Get(ctx context.Context, owner, id uuid.UUID) (*appdomain.AppBuildJob, error) {
	job, err := s.repo.GetOwnedJob(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, errcode.ErrBuildNotFound
	}
	return fromJob(job), nil
}

func (s *appBuildService) List(ctx context.Context, owner, appID uuid.UUID) ([]*appdomain.AppBuildJob, error) {
	jobs, err := s.repo.ListByApp(ctx, owner, appID, appBuildListLimit)
	if err != nil {
		return nil, err
	}
	out := make([]*appdomain.AppBuildJob, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, fromJob(job))
	}
	return out, nil
}

func (s *appBuildService) Download(ctx context.Context, owner, id uuid.UUID) (appdomain.AppBuildArtifact, error) {
	path, err := s.uc.DownloadOwned(ctx, owner, id)
	if err != nil {
		return appdomain.AppBuildArtifact{}, err
	}
	return appdomain.AppBuildArtifact{
		Path:        path,
		FileName:    artifactName,
		ContentType: appBuildContentType,
	}, nil
}

func (s *appBuildService) Publish(ctx context.Context, owner, id uuid.UUID) (*appdomain.AppBuildRelease, error) {
	st, err := s.uc.PublishOwned(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	rel := &appdomain.AppBuildRelease{Platform: st.Platform}
	if st.Manifest != nil {
		rel.Version = st.Manifest.Version
		rel.URL = st.Manifest.URL
	}
	rel.ArtifactURL = st.ArtifactURL
	rel.ArtifactSHA256 = st.ArtifactSHA256
	return rel, nil
}

func (s *appBuildService) Delete(ctx context.Context, owner, id uuid.UUID) error {
	return s.repo.DeleteOwnedJob(ctx, owner, id)
}

func (s *appBuildService) CountToday(ctx context.Context, owner uuid.UUID) (int, error) {
	return s.repo.CountJobsToday(ctx, owner)
}

// fromJob copies a build job into the user-center DTO. The download link is
// rewritten to the user-center route because the builder's own link sits
// behind admin auth, and it is only offered while the artifact exists.
func fromJob(job *domain.BuildJob) *appdomain.AppBuildJob {
	if job == nil {
		return nil
	}
	dto := &appdomain.AppBuildJob{
		ID:             job.ID,
		AppID:          job.AppID,
		Platform:       job.Platform,
		TargetVersion:  job.TargetVersion,
		VersionCode:    job.VersionCode,
		Status:         job.Status,
		CurrentStep:    job.CurrentStep,
		Error:          job.Error,
		ExitCode:       job.ExitCode,
		LogTail:        job.LogTail,
		ArtifactSHA256: job.ArtifactSHA256,
		ArtifactSize:   job.ArtifactSize,
		DurationMS:     job.DurationMS,
		EnqueuedAt:     job.EnqueuedAt,
		StartedAt:      job.StartedAt,
		FinishedAt:     job.FinishedAt,
	}
	if job.Status == domain.BuildStatusSucceeded && job.ArtifactPath != "" {
		dto.ArtifactDownloadURL = appBuildDownloadURL(job)
	}
	return dto
}

func appBuildDownloadURL(job *domain.BuildJob) string {
	return appBuildPathBase + "/" + job.AppID.String() + "/builds/" + job.ID.String() + "/download"
}
