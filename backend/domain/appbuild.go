package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// AppBuildService lets the user center hand packaging work to the app builder
// module. It is defined here instead of importing the app builder usecase so
// biz/app does not depend on biz/mobilebuild's packages; the implementation
// lives in biz/mobilebuild and is registered in its register.go. Every method
// takes owner and filters by user_id at the repository, so a cross-user request
// reads like "not found" instead of "exists but is someone else's".
type AppBuildService interface {
	// Enqueue starts a packaging job for one app. VersionCode of zero means
	// "assign the next legal code".
	Enqueue(ctx context.Context, in AppBuildInput) (*AppBuildJob, error)
	// Get returns one of the owner's own jobs, nil when absent.
	Get(ctx context.Context, owner, id uuid.UUID) (*AppBuildJob, error)
	// List returns the app's job history, newest first.
	List(ctx context.Context, owner, appID uuid.UUID) ([]*AppBuildJob, error)
	// Download resolves the artifact path of one of the owner's own jobs.
	Download(ctx context.Context, owner, id uuid.UUID) (AppBuildArtifact, error)
	// Publish promotes one of the owner's succeeded jobs to the update channel.
	Publish(ctx context.Context, owner, id uuid.UUID) (*AppBuildRelease, error)
	// Delete removes one of the owner's own jobs.
	Delete(ctx context.Context, owner, id uuid.UUID) error
	// CountToday counts the user's jobs across every app in the last 24 hours.
	CountToday(ctx context.Context, owner uuid.UUID) (int, error)
}

// AppBuildQuotaService is the user-center side of packaging: it decides
// whether a user may start a job and delegates the enqueue to the app builder.
type AppBuildQuotaService interface {
	// EnqueueBuild starts a job after the quota check passes.
	EnqueueBuild(ctx context.Context, uid, appID uuid.UUID, req *EnqueueAppBuildReq) (*AppBuildJob, error)
	// ListBuilds returns the app's job history.
	ListBuilds(ctx context.Context, uid, appID uuid.UUID) ([]*AppBuildJob, error)
	// GetBuild returns one job.
	GetBuild(ctx context.Context, uid, id uuid.UUID) (*AppBuildJob, error)
	// DownloadBuild resolves one job's artifact path.
	DownloadBuild(ctx context.Context, uid, id uuid.UUID) (AppBuildArtifact, error)
	// PublishBuild promotes one succeeded job to the update channel.
	PublishBuild(ctx context.Context, uid, id uuid.UUID) (*AppBuildRelease, error)
	// DeleteBuild removes one job.
	DeleteBuild(ctx context.Context, uid, id uuid.UUID) error
}

// AppBuildInput is the user center's request to start a packaging job.
type AppBuildInput struct {
	AppID       uuid.UUID
	Owner       uuid.UUID
	Platform    string
	Version     string
	VersionCode int
}

// EnqueueAppBuildReq starts a packaging job for one app.
type EnqueueAppBuildReq struct {
	Platform    string `json:"platform"`
	Version     string `json:"version" validate:"required"`
	VersionCode int    `json:"version_code"`
}

// AppBuildJob is the app builder job DTO surfaced to the user center.
type AppBuildJob struct {
	ID                  uuid.UUID  `json:"id"`
	AppID               uuid.UUID  `json:"app_id,omitempty"`
	Platform            string     `json:"platform"`
	TargetVersion       string     `json:"target_version"`
	VersionCode         int        `json:"version_code"`
	Status              string     `json:"status"`
	CurrentStep         string     `json:"current_step,omitempty"`
	Error               string     `json:"error,omitempty"`
	ExitCode            *int       `json:"exit_code,omitempty"`
	LogTail             string     `json:"log_tail,omitempty"`
	ArtifactSHA256      string     `json:"artifact_sha256,omitempty"`
	ArtifactSize        int64      `json:"artifact_size"`
	DurationMS          int64      `json:"duration_ms"`
	EnqueuedAt          time.Time  `json:"enqueued_at"`
	StartedAt           *time.Time `json:"started_at,omitempty"`
	FinishedAt          *time.Time `json:"finished_at,omitempty"`
	ArtifactDownloadURL string     `json:"artifact_download_url,omitempty"`
}

// AppBuildArtifact is the resolved artifact of one build job.
type AppBuildArtifact struct {
	Path        string
	FileName    string
	ContentType string
}

// AppBuildRelease describes the update channel after a publish.
type AppBuildRelease struct {
	Platform       string `json:"platform"`
	Version        string `json:"version"`
	URL            string `json:"url"`
	ArtifactURL    string `json:"artifact_url,omitempty"`
	ArtifactSHA256 string `json:"artifact_sha256,omitempty"`
}
