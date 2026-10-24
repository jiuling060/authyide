package usecase

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/samber/do"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
	"github.com/chaitin/MonkeyCode/backend/db"
	appdomain "github.com/chaitin/MonkeyCode/backend/domain"
	"github.com/chaitin/MonkeyCode/backend/errcode"
)

const (
	appIconContentType = "image/png"
	appIconFileName    = "icon.png"
	appBuildPathBase   = "/api/v1/users/apps"
	appBuildListLimit  = 100
)

// appBuildService is the user-center side of the app builder. Every method is
// owner-scoped at the repository, so a cross-user request reads like "not
// found" instead of "exists but belongs to someone else".
type appBuildService struct {
	uc     Usecase
	repo   domain.AppBuilderRepo
	tasks  taskReader
	logger *slog.Logger
}

// taskReader is the only part of the task store the user-center build view
// needs: resolving a source task id back to a session title.
type taskReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*db.Task, error)
}

// NewAppBuildService exposes the app builder to the user center.
func NewAppBuildService(i *do.Injector) (appdomain.AppBuildService, error) {
	uc, err := NewUsecase(i)
	if err != nil {
		return nil, err
	}
	return &appBuildService{
		uc:     uc,
		repo:   do.MustInvoke[domain.AppBuilderRepo](i),
		tasks:  do.MustInvoke[appdomain.TaskRepo](i),
		logger: do.MustInvoke[*slog.Logger](i).With("component", "app-builds"),
	}, nil
}

func (s *appBuildService) Enqueue(ctx context.Context, in appdomain.AppBuildInput) (*appdomain.AppBuildJob, error) {
	if !s.uc.Enabled() {
		return nil, errcode.ErrBrandingInvalid
	}
	job, err := s.uc.EnqueueBuildForApp(ctx, in.Owner, in.AppID, &domain.EnqueueBuildReq{
		Platform:         in.Platform,
		Version:          in.Version,
		VersionCode:      in.VersionCode,
		SourceSnapshotID: in.SourceSnapshotID,
		SourceTaskID:     in.SourceTaskID,
	})
	if err != nil {
		return nil, err
	}
	return s.fromJob(ctx, job, nil), nil
}

func (s *appBuildService) Get(ctx context.Context, owner, id uuid.UUID) (*appdomain.AppBuildJob, error) {
	job, err := s.repo.GetOwnedJob(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, errcode.ErrBuildNotFound
	}
	return s.fromJob(ctx, job, nil), nil
}

func (s *appBuildService) List(ctx context.Context, owner, appID uuid.UUID) ([]*appdomain.AppBuildJob, error) {
	jobs, err := s.repo.ListByApp(ctx, owner, appID, appBuildListLimit)
	if err != nil {
		return nil, err
	}
	titles := s.sourceTaskTitles(ctx, jobs)
	out := make([]*appdomain.AppBuildJob, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, s.fromJob(ctx, job, titles))
	}
	return out, nil
}

// ListAll is the admin history of one app's jobs; it does not filter the
// owner, so the caller must sit behind admin auth.
func (s *appBuildService) ListAll(ctx context.Context, appID uuid.UUID) ([]*appdomain.AppBuildJob, error) {
	jobs, err := s.repo.ListByAppAll(ctx, appID, appBuildListLimit)
	if err != nil {
		return nil, err
	}
	titles := s.sourceTaskTitles(ctx, jobs)
	out := make([]*appdomain.AppBuildJob, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, s.fromJob(ctx, job, titles))
	}
	return out, nil
}

func (s *appBuildService) Download(ctx context.Context, owner, id uuid.UUID, channel string) (appdomain.AppBuildArtifact, error) {
	path, name, err := s.uc.DownloadOwned(ctx, owner, id, channel)
	if err != nil {
		return appdomain.AppBuildArtifact{}, err
	}
	return appdomain.AppBuildArtifact{
		Path:        path,
		FileName:    name,
		ContentType: domain.ContentTypeOf(name),
	}, nil
}

func (s *appBuildService) Publish(ctx context.Context, owner, id uuid.UUID, channel string) (*appdomain.AppBuildRelease, error) {
	st, err := s.uc.PublishOwned(ctx, owner, id, channel)
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

// Platforms reports one app's build targets and why each is or is not
// buildable.
func (s *appBuildService) Platforms(ctx context.Context, owner, appID uuid.UUID, desktopLicensed bool) ([]appdomain.AppBuildPlatform, error) {
	list, err := s.uc.Platforms(ctx, owner, appID, desktopLicensed)
	if err != nil {
		return nil, err
	}
	out := make([]appdomain.AppBuildPlatform, 0, len(list))
	for _, st := range list {
		out = append(out, appdomain.AppBuildPlatform{
			Platform:        st.Platform,
			Enabled:         st.Enabled,
			Reason:          st.Reason,
			UpdaterEndpoint: st.UpdaterEndpoint,
		})
	}
	return out, nil
}

// GetDesktopBranding returns one app's desktop identity with app defaults.
func (s *appBuildService) GetDesktopBranding(ctx context.Context, owner, appID uuid.UUID) (*appdomain.AppDesktopBranding, error) {
	b, err := s.uc.GetAppDesktopBranding(ctx, owner, appID)
	if err != nil {
		return nil, err
	}
	return &appdomain.AppDesktopBranding{
		ProductName:      b.ProductName,
		BundleIdentifier: b.BundleIdentifier,
	}, nil
}

// SaveDesktopBranding stores one app's desktop identity.
func (s *appBuildService) SaveDesktopBranding(ctx context.Context, owner, appID uuid.UUID, req *appdomain.AppUpdateDesktopBrandingReq) (*appdomain.AppDesktopBranding, error) {
	var inner *domain.UpdateAppDesktopBrandingReq
	if req != nil {
		inner = &domain.UpdateAppDesktopBrandingReq{
			ProductName:      req.ProductName,
			BundleIdentifier: req.BundleIdentifier,
		}
	}
	b, err := s.uc.SaveAppDesktopBranding(ctx, owner, appID, inner)
	if err != nil {
		return nil, err
	}
	return &appdomain.AppDesktopBranding{
		ProductName:      b.ProductName,
		BundleIdentifier: b.BundleIdentifier,
	}, nil
}

func (s *appBuildService) Delete(ctx context.Context, owner, id uuid.UUID) error {
	return s.repo.DeleteOwnedJob(ctx, owner, id)
}

func (s *appBuildService) CountToday(ctx context.Context, owner uuid.UUID) (int, error) {
	return s.repo.CountJobsToday(ctx, owner)
}

func (s *appBuildService) UploadIcon(ctx context.Context, owner, appID uuid.UUID, data []byte) error {
	return s.uc.UploadAppIcon(ctx, owner, appID, data)
}

func (s *appBuildService) ClearIcon(ctx context.Context, owner, appID uuid.UUID) error {
	return s.uc.ClearAppIcon(ctx, owner, appID)
}

func (s *appBuildService) DownloadIcon(ctx context.Context, owner, appID uuid.UUID) (appdomain.AppBuildArtifact, error) {
	path, err := s.uc.DownloadAppIcon(ctx, owner, appID)
	if err != nil {
		return appdomain.AppBuildArtifact{}, err
	}
	return appdomain.AppBuildArtifact{
		Path:        path,
		FileName:    appIconFileName,
		ContentType: appIconContentType,
	}, nil
}

// fromJob copies a build job into the user-center DTO. The download link is
// rewritten to the user-center route because the builder's own link sits
// behind admin auth, and it is only offered while the artifact exists.
// sourceTaskTitles is a pre-resolved lookup so a list call does one task query
// per distinct source session instead of one per job.
func (s *appBuildService) fromJob(ctx context.Context, job *domain.BuildJob, titles map[uuid.UUID]string) *appdomain.AppBuildJob {
	if job == nil {
		return nil
	}
	dto := &appdomain.AppBuildJob{
		ID:               job.ID,
		AppID:            job.AppID,
		Platform:         job.Platform,
		TargetVersion:    job.TargetVersion,
		VersionCode:      job.VersionCode,
		Status:           job.Status,
		CurrentStep:      job.CurrentStep,
		Error:            job.Error,
		ExitCode:         job.ExitCode,
		LogTail:          job.LogTail,
		ArtifactSHA256:   job.ArtifactSHA256,
		ArtifactSize:     job.ArtifactSize,
		DurationMS:       job.DurationMS,
		EnqueuedAt:       job.EnqueuedAt,
		StartedAt:        job.StartedAt,
		FinishedAt:       job.FinishedAt,
		SourceSnapshotID: job.SourceSnapshotID,
		SourceTaskID:     job.SourceTaskID,
	}
	if job.SourceTaskID != nil {
		dto.SourceTaskTitle = titles[*job.SourceTaskID]
	}
	if job.Status == domain.BuildStatusSucceeded && job.ArtifactPath != "" {
		dto.ArtifactDownloadURL = appBuildDownloadURL(job)
	}
	if job.Status == domain.BuildStatusSucceeded {
		dto.Artifacts = s.jobArtifacts(ctx, job)
	}
	return dto
}

// jobArtifacts projects one job's channels into user-center download actions.
// ListByApp already carries the artifact rows; GetOwnedJob does not, so it is
// fetched on demand for the single-job view.
func (s *appBuildService) jobArtifacts(ctx context.Context, job *domain.BuildJob) []appdomain.AppBuildArtifactInfo {
	if len(job.Artifacts) == 0 {
		artifacts, err := s.repo.ListArtifacts(ctx, job.ID)
		if err != nil {
			s.logger.WarnContext(ctx, "list build artifacts", "job_id", job.ID, "error", err)
			return nil
		}
		job.Artifacts = artifacts
	}
	out := make([]appdomain.AppBuildArtifactInfo, 0, len(job.Artifacts))
	for _, a := range job.Artifacts {
		if a == nil || strings.TrimSpace(a.FileName) == "" {
			continue
		}
		channel := strings.TrimSpace(a.Channel)
		out = append(out, appdomain.AppBuildArtifactInfo{
			Channel:     channel,
			FileName:    a.FileName,
			Size:        a.Size,
			SHA256:      a.SHA256,
			DownloadURL: appBuildChannelDownloadURL(job, channel),
			Publishable: domain.ChannelPublishable(channel),
		})
	}
	return out
}

// sourceTaskTitles resolves every distinct source session of jobs to its title.
// A missing session leaves the entry absent, which renders as an empty title.
func (s *appBuildService) sourceTaskTitles(ctx context.Context, jobs []*domain.BuildJob) map[uuid.UUID]string {
	titles := map[uuid.UUID]string{}
	if s.tasks == nil {
		return titles
	}
	for _, job := range jobs {
		if job.SourceTaskID == nil || titles[*job.SourceTaskID] != "" {
			continue
		}
		task, err := s.tasks.GetByID(ctx, *job.SourceTaskID)
		if err != nil || task == nil {
			continue
		}
		titles[*job.SourceTaskID] = task.Title
	}
	return titles
}

func appBuildDownloadURL(job *domain.BuildJob) string {
	// The route is job-scoped, not app-scoped: ownership is enforced by the
	// user id behind the session, so the app id never appears in the path.
	return appBuildPathBase + "/builds/" + job.ID.String() + "/download"
}

// appBuildChannelDownloadURL appends the channel only when it is not the
// default, so a plain download link keeps working for the primary channel.
func appBuildChannelDownloadURL(job *domain.BuildJob, channel string) string {
	if channel == "" {
		return appBuildDownloadURL(job)
	}
	return appBuildDownloadURL(job) + "?channel=" + url.QueryEscape(channel)
}
