// Package domain holds the mobile app builder DTOs, constants and interfaces.
package domain

import (
	"context"
	"regexp"
	"time"

	"github.com/google/uuid"
)

// Build statuses.
const (
	BuildStatusQueued    = "queued"
	BuildStatusRunning   = "running"
	BuildStatusSucceeded = "succeeded"
	BuildStatusFailed    = "failed"
)

// Build pipeline steps, in execution order.
const (
	StepInjectAssets  = "inject-assets"
	StepRewriteConfig = "rewrite-config"
	StepPrebuild      = "prebuild"
	StepAssemble      = "assemble"
	StepVerify        = "verify"
)

// BuildSteps lists the pipeline steps in execution order.
var BuildSteps = []string{
	StepInjectAssets,
	StepRewriteConfig,
	StepPrebuild,
	StepAssemble,
	StepVerify,
}

// Asset slots, whitelisted for upload and never used in path joining.
const (
	SlotLogoLight    = "logo-light"
	SlotLogoDark     = "logo-dark"
	SlotIcon         = "icon"
	SlotIconDark     = "icon-dark"
	SlotAdaptiveIcon = "adaptive-icon"
	SlotSplash       = "splash"
	SlotSplashDark   = "splash-dark"
)

// AssetSlots lists every accepted asset slot.
var AssetSlots = []string{
	SlotLogoLight,
	SlotLogoDark,
	SlotIcon,
	SlotIconDark,
	SlotAdaptiveIcon,
	SlotSplash,
	SlotSplashDark,
}

// AssetSlotValid reports whether slot is one of the whitelisted slots.
func AssetSlotValid(slot string) bool {
	for _, s := range AssetSlots {
		if s == slot {
			return true
		}
	}
	return false
}

// PlatformAndroid is the only platform this deployment builds.
const PlatformAndroid = "android"

// versionPattern must match the APP_VERSION_RE in mobile/src/updates/useOtaUpdate.ts
// so a published native version is recognised by the client.
var versionPattern = regexp.MustCompile(`^\d{6,8}$`)

// VersionValid reports whether version is a legal expo version string.
func VersionValid(version string) bool {
	return versionPattern.MatchString(version)
}

// StatusResp describes the capabilities of the app builder. The frontend uses
// it to hide the entry and render the upload UI without hard-coding values.
type StatusResp struct {
	Enabled        bool            `json:"enabled"`
	Workspace      string          `json:"workspace"`
	Platforms      []string        `json:"platforms"`
	AssetSlots     []AssetSlotInfo `json:"asset_slots"`
	BuildSteps     []string        `json:"build_steps"`
	VersionPattern string          `json:"version_pattern"`
	MaxAssetBytes  int64           `json:"max_asset_bytes"`
	GradleTimeout  string          `json:"gradle_timeout"`
	// NextVersionCode is the smallest legal version code, so the frontend can
	// pre-fill the build form.
	NextVersionCode int `json:"next_version_code"`
}

// Branding is the single mobile app branding of this deployment.
type Branding struct {
	Name          string          `json:"name"`
	Version       string          `json:"version"`
	VersionCode   int             `json:"version_code"`
	ServerURL     string          `json:"server_url"`
	UpdatesServer string          `json:"updates_server,omitempty"`
	Assets        []AssetSlotInfo `json:"assets"`
}

// AssetSlotInfo describes one asset slot and whether it has been customised.
type AssetSlotInfo struct {
	Slot  string `json:"slot"`
	Key   string `json:"key,omitempty"`
	URL   string `json:"url,omitempty"`
	Bytes int64  `json:"bytes"`
	Set   bool   `json:"set"`
}

// UpdateBrandingReq carries the editable branding fields. Nil means unchanged.
type UpdateBrandingReq struct {
	Name          *string `json:"name,omitempty"`
	Version       *string `json:"version,omitempty"`
	VersionCode   *int    `json:"version_code,omitempty"`
	ServerURL     *string `json:"server_url,omitempty"`
	UpdatesServer *string `json:"updates_server,omitempty"`
}

// EnqueueBuildReq starts a packaging job.
type EnqueueBuildReq struct {
	Platform    string `json:"platform"`
	Version     string `json:"version" validate:"required"`
	VersionCode int    `json:"version_code"`
	// AppID is set for self-service packaging and empty for admin-console jobs.
	AppID uuid.UUID `json:"app_id,omitempty"`
}

// BuildJob is one packaging request.
type BuildJob struct {
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
	ArtifactPath        string     `json:"artifact_path,omitempty"`
	ArtifactSHA256      string     `json:"artifact_sha256,omitempty"`
	ArtifactSize        int64      `json:"artifact_size"`
	DurationMS          int64      `json:"duration_ms"`
	EnqueuedAt          time.Time  `json:"enqueued_at"`
	StartedAt           *time.Time `json:"started_at,omitempty"`
	FinishedAt          *time.Time `json:"finished_at,omitempty"`
	PublishedAt         *time.Time `json:"published_at,omitempty"`
	ArtifactDownloadURL string     `json:"artifact_download_url,omitempty"`
}

// ReleaseManifest is the app-version/<platform>.json contract consumed by
// mobile checkAppUpdate().
type ReleaseManifest struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// ReleaseStatus describes the current native release of a platform.
type ReleaseStatus struct {
	Platform        string           `json:"platform"`
	Enabled         bool             `json:"enabled"`
	Manifest        *ReleaseManifest `json:"manifest,omitempty"`
	ArtifactURL     string           `json:"artifact_url,omitempty"`
	ArtifactPresent bool             `json:"artifact_present"`
	ArtifactSHA256  string           `json:"artifact_sha256,omitempty"`
}

// ReleasesResp aggregates every release channel of one deployment.
type ReleasesResp struct {
	Native []*ReleaseStatus `json:"native"`
	Ota    *OtaStatus       `json:"ota,omitempty"`
}

// OtaStatus tells the frontend whether OTA publishing is available.
type OtaStatus struct {
	Enabled  bool             `json:"enabled"`
	URL      string           `json:"url,omitempty"`
	Manifest *ReleaseManifest `json:"manifest,omitempty"`
}

// PublishOtaReq publishes an OTA update.
type PublishOtaReq struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// JobResult carries the outcome of one pipeline run.
type JobResult struct {
	Status         string
	Error          string
	ExitCode       *int
	ArtifactPath   string
	ArtifactSHA256 string
	ArtifactSize   int64
	DurationMS     int64
}

// AppBranding carries the per-app build inputs of one self-service job.
// ServerURL and UpdatesServer come from the instance branding instead.
type AppBranding struct {
	ID               uuid.UUID
	Name             string
	PackageID        string
	AdNetwork        string
	AdAppID          string
	EnabledPlatforms []string
}

// AppBuilderRepo is the persistence boundary of the app builder.
type AppBuilderRepo interface {
	GetBranding(ctx context.Context) (*Branding, error)
	SaveBranding(ctx context.Context, req *UpdateBrandingReq) (*Branding, error)
	SetAssetKey(ctx context.Context, slot, key string) (*Branding, error)
	ClearAssetKey(ctx context.Context, slot string) (*Branding, error)
	// GetAppBranding returns the per-app build inputs of one user-center app.
	GetAppBranding(ctx context.Context, appID uuid.UUID) (*AppBranding, error)

	CreateJob(ctx context.Context, userID uuid.UUID, req *EnqueueBuildReq) (*BuildJob, error)
	ListJobs(ctx context.Context, limit int) ([]*BuildJob, error)
	GetJob(ctx context.Context, id uuid.UUID) (*BuildJob, error)
	AnyRunning(ctx context.Context) (bool, error)
	PickOldestQueued(ctx context.Context) (*BuildJob, error)
	ClaimJob(ctx context.Context, id uuid.UUID, workerID string) (bool, error)
	SetJobLogPath(ctx context.Context, id uuid.UUID, path string) error
	SetJobStep(ctx context.Context, id uuid.UUID, step string) error
	SetJobLogTail(ctx context.Context, id uuid.UUID, tail string) error
	FinishJob(ctx context.Context, id uuid.UUID, res JobResult) error
	FailAllRunning(ctx context.Context, reason string) error
	MaxVersionCode(ctx context.Context) (int, error)
	MarkJobPublished(ctx context.Context, id uuid.UUID, artifactSHA256 string) error

	// ListByApp returns one user's jobs for one app, newest first.
	ListByApp(ctx context.Context, userID, appID uuid.UUID, limit int) ([]*BuildJob, error)
	// GetOwnedJob returns one of the user's own jobs, nil when absent.
	GetOwnedJob(ctx context.Context, userID, id uuid.UUID) (*BuildJob, error)
	// HasActiveJobForUser reports whether the user has a queued or running job.
	HasActiveJobForUser(ctx context.Context, userID uuid.UUID) (bool, error)
	// CountJobsToday counts one user's jobs across every app in the last 24 hours.
	CountJobsToday(ctx context.Context, userID uuid.UUID) (int, error)
	// CountQueued counts every queued job, for the queue depth cap.
	CountQueued(ctx context.Context) (int, error)
	// MaxVersionCodeForApp returns the highest version code one app has used.
	MaxVersionCodeForApp(ctx context.Context, appID uuid.UUID) (int, error)
	// DeleteOwnedJob removes one of the user's own jobs.
	DeleteOwnedJob(ctx context.Context, userID, id uuid.UUID) error
	// CompleteForApp closes a self-service job and updates its app's and
	// owner's usage counters in one transaction.
	CompleteForApp(ctx context.Context, jobID, appID uuid.UUID, res JobResult) error
}
