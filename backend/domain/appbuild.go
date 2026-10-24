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
	// ListAll returns every job of one app regardless of owner; only an admin
	// may call it.
	ListAll(ctx context.Context, appID uuid.UUID) ([]*AppBuildJob, error)
	// Download resolves the artifact of one channel of one of the owner's own
	// jobs. An empty channel resolves to the job's primary channel.
	Download(ctx context.Context, owner, id uuid.UUID, channel string) (AppBuildArtifact, error)
	// Publish promotes one of the owner's succeeded jobs to the update channel.
	Publish(ctx context.Context, owner, id uuid.UUID, channel string) (*AppBuildRelease, error)
	// Delete removes one of the owner's own jobs.
	Delete(ctx context.Context, owner, id uuid.UUID) error
	// CountToday counts the user's jobs across every app in the last 24 hours.
	CountToday(ctx context.Context, owner uuid.UUID) (int, error)
	// UploadIcon stores one app's custom icon.
	UploadIcon(ctx context.Context, owner, appID uuid.UUID, data []byte) error
	// ClearIcon removes one app's custom icon.
	ClearIcon(ctx context.Context, owner, appID uuid.UUID) error
	// DownloadIcon resolves one app's custom icon for preview.
	DownloadIcon(ctx context.Context, owner, appID uuid.UUID) (AppBuildArtifact, error)
	// Platforms lists every build target of one app with the reason each is or
	// is not buildable.
	Platforms(ctx context.Context, owner, appID uuid.UUID, desktopLicensed bool) ([]AppBuildPlatform, error)
	// GetDesktopBranding returns one app's desktop identity.
	GetDesktopBranding(ctx context.Context, owner, appID uuid.UUID) (*AppDesktopBranding, error)
	// SaveDesktopBranding stores one app's desktop identity.
	SaveDesktopBranding(ctx context.Context, owner, appID uuid.UUID, req *AppUpdateDesktopBrandingReq) (*AppDesktopBranding, error)
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
	// DownloadBuild resolves one job's artifact for one channel.
	DownloadBuild(ctx context.Context, uid, id uuid.UUID, channel string) (AppBuildArtifact, error)
	// PublishBuild promotes one succeeded job's channel to the update channel.
	PublishBuild(ctx context.Context, uid, id uuid.UUID, channel string) (*AppBuildRelease, error)
	// DeleteBuild removes one job.
	DeleteBuild(ctx context.Context, uid, id uuid.UUID) error
	// UploadIcon stores one app's custom icon.
	UploadIcon(ctx context.Context, uid, appID uuid.UUID, data []byte) error
	// ClearIcon removes one app's custom icon.
	ClearIcon(ctx context.Context, uid, appID uuid.UUID) error
	// DownloadIcon resolves one app's custom icon for preview.
	DownloadIcon(ctx context.Context, uid, appID uuid.UUID) (AppBuildArtifact, error)
	// Platforms lists every build target of one app with the reason each is or
	// is not buildable.
	Platforms(ctx context.Context, uid, appID uuid.UUID, desktopLicensed bool) ([]AppBuildPlatform, error)
	// GetDesktopBranding returns one app's desktop identity.
	GetDesktopBranding(ctx context.Context, uid, appID uuid.UUID) (*AppDesktopBranding, error)
	// SaveDesktopBranding stores one app's desktop identity.
	SaveDesktopBranding(ctx context.Context, uid, appID uuid.UUID, req *AppUpdateDesktopBrandingReq) (*AppDesktopBranding, error)
	// ListAppBuilds returns every build of one app with its owning app, so an
	// admin sees all tenants' history without an owner check.
	ListAppBuilds(ctx context.Context, appID uuid.UUID) ([]*AppBuildRecord, error)
}

// AppBuildInput is the user center's request to start a packaging job.
type AppBuildInput struct {
	AppID       uuid.UUID
	Owner       uuid.UUID
	Platform    string
	Version     string
	VersionCode int
	// SourceSnapshotID and SourceTaskID are the app's binding at enqueue time;
	// nil builds the template source.
	SourceSnapshotID *uuid.UUID
	SourceTaskID     *uuid.UUID
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
	// Artifacts lists every channel of a succeeded job so the caller can offer
	// one download and one publish action per package.
	Artifacts        []AppBuildArtifactInfo `json:"artifacts,omitempty"`
	SourceSnapshotID *uuid.UUID             `json:"source_snapshot_id,omitempty"`
	SourceTaskID     *uuid.UUID             `json:"source_task_id,omitempty"`
	SourceTaskTitle  string                 `json:"source_task_title,omitempty"`
}

// AppBuildArtifactInfo is one channel of a build job, projected for the API.
type AppBuildArtifactInfo struct {
	Channel     string `json:"channel"`
	FileName    string `json:"file_name"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	DownloadURL string `json:"download_url"`
	Publishable bool   `json:"publishable"`
}

// AppBuildPlatform describes one build target of one app.
type AppBuildPlatform struct {
	Platform string `json:"platform"`
	Enabled  bool   `json:"enabled"`
	Reason   string `json:"reason,omitempty"`
	// UpdaterEndpoint is the URL the desktop client of this app polls. It is
	// derived from the app id, so each app owns its update source.
	UpdaterEndpoint string `json:"updater_endpoint,omitempty"`
}

// AppDesktopBranding is the per-app identity of the desktop shell.
type AppDesktopBranding struct {
	ProductName      string `json:"product_name"`
	BundleIdentifier string `json:"bundle_identifier"`
}

// AppUpdateDesktopBrandingReq carries the editable per-app desktop branding
// fields. Nil means unchanged.
type AppUpdateDesktopBrandingReq struct {
	ProductName      *string `json:"product_name,omitempty"`
	BundleIdentifier *string `json:"bundle_identifier,omitempty"`
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

// AppBuildRecord is one row of the admin build listing: a job together with the
// app it built, so a listing does not need a second request for app names.
type AppBuildRecord struct {
	Job *AppBuildJob `json:"job"`
	// AppName and PackageID come from the owning app row; both are empty when
	// the app was removed after the job was enqueued.
	AppName   string    `json:"app_name,omitempty"`
	PackageID string    `json:"package_id,omitempty"`
	OwnerID   uuid.UUID `json:"owner_id"`
}
