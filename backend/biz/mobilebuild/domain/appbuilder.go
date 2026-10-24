// Package domain holds the mobile app builder DTOs, constants and interfaces.
package domain

import (
	"context"
	"io"
	"path/filepath"
	"regexp"
	"strings"
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

// Build pipeline steps, in execution order. overlay-source runs only when the
// job is bound to a session source snapshot; the runner reports it then. Both
// pipelines share it: the mobile pipeline overlays the session onto the app
// template, the desktop pipeline onto the shell checkout.
const (
	StepOverlaySource = "overlay-source"
	StepInjectAssets  = "inject-assets"
	StepRewriteConfig = "rewrite-config"
	StepPrebuild      = "prebuild"
	StepAssemble      = "assemble"
	StepVerify        = "verify"

	StepUINext           = "ui-next"
	StepEngine           = "engine"
	StepBrowserExtension = "browser-extension"
	StepShell            = "shell"
	StepBundle           = "bundle"
)

// BuildSteps lists the pipeline steps in execution order.
var BuildSteps = []string{
	StepOverlaySource,
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
	// SlotDesktopIcon is the icon the desktop shell bundles into its window,
	// taskbar and installer artwork.
	SlotDesktopIcon = "desktop-icon"
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
	SlotDesktopIcon,
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

// PlatformAndroid and PlatformDesktopLinux are the build targets this
// deployment can actually compile.
const (
	PlatformAndroid      = "android"
	PlatformDesktopLinux = "desktop-linux"
)

// PlatformDesktop is the user-facing alias for the desktop build target. The
// app record stores platform names from the app vocabulary (web/android/ios/
// harmonyos/desktop), while the builder speaks android/desktop-linux; the two
// vocabularies are reconciled at the boundary with NormalizePlatform so a
// request may name either form.
const PlatformDesktop = "desktop"

// BuildPlatforms lists every platform the builder can queue a job for.
var BuildPlatforms = []string{PlatformAndroid, PlatformDesktopLinux}

// NormalizePlatform canonicalizes a platform name onto the builder vocabulary.
func NormalizePlatform(platform string) string {
	switch p := strings.TrimSpace(platform); p {
	case PlatformDesktop:
		return PlatformDesktopLinux
	default:
		return p
	}
}

// PlatformValid reports whether platform is a buildable target.
func PlatformValid(platform string) bool {
	return PlatformValidRaw(NormalizePlatform(platform))
}

// PlatformValidRaw checks an already canonical platform name.
func PlatformValidRaw(platform string) bool {
	for _, p := range BuildPlatforms {
		if p == platform {
			return true
		}
	}
	return false
}

// ContentTypeOf maps an artifact file name to the media type a browser or a
// native updater needs to treat it as a downloadable package.
func ContentTypeOf(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".apk":
		return "application/vnd.android.package-archive"
	case ".deb":
		return "application/vnd.debian.binary-package"
	case ".rpm":
		return "application/x-rpm"
	case ".appimage":
		return "application/x-appimage"
	case ".png":
		return "image/png"
	default:
		return "application/octet-stream"
	}
}

// Static release layout. The usecase writes these paths and the worker derives
// the updater endpoint from the same constants, so the client address and the
// on-disk tree cannot drift apart.
const (
	StaticManifestDir     = "app-version"
	StaticMobileDir       = "mobile"
	StaticDesktopDir      = "desktop"
	StaticDesktopManifest = "latest-linux.json"
	// DesktopEndpointLeaf is the URL the desktop client polls. The updater
	// appends the platform name, so the manifest lands on disk as
	// StaticDesktopManifest instead of "latest.json".
	DesktopEndpointLeaf = "latest.json"
	MobileArtifactName  = "authyide.apk"
	DesktopAppSuffix    = ".desktop"
)

// JoinStatic joins a public base URL with a route prefix and path segments.
func JoinStatic(baseURL, prefix string, segments ...string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	p := strings.TrimRight(strings.TrimSpace(prefix), "/")
	for _, s := range segments {
		p += "/" + strings.Trim(s, "/")
	}
	return base + p
}

// MobileManifestRelPath is the release-manifest path of one release train.
// An empty appID gives the admin-console train.
func MobileManifestRelPath(appID, platform string) string {
	if appID == "" {
		return JoinStatic("", "", StaticManifestDir, platform+".json")
	}
	return JoinStatic("", "", StaticManifestDir, appID, platform+".json")
}

// DesktopManifestRelPath is the on-disk updater manifest of one release train.
func DesktopManifestRelPath(appID string) string {
	if appID == "" {
		return JoinStatic("", "", StaticDesktopDir, StaticDesktopManifest)
	}
	return JoinStatic("", "", StaticDesktopDir, appID, StaticDesktopManifest)
}

// DesktopUpdaterEndpoint is the URL the desktop client of one release train
// polls. An empty appID gives the admin-console train.
func DesktopUpdaterEndpoint(baseURL, prefix, appID string) string {
	segments := []string{StaticDesktopDir}
	if appID != "" {
		segments = append(segments, appID)
	}
	segments = append(segments, DesktopEndpointLeaf)
	return JoinStatic(baseURL, prefix, segments...)
}

// MobileArtifactRelPath is the public path of one release train's APK.
func MobileArtifactRelPath(appID, version string) string {
	segments := []string{StaticMobileDir}
	if appID != "" {
		segments = append(segments, appID)
	}
	segments = append(segments, version, MobileArtifactName)
	return JoinStatic("", "", segments...)
}

// DesktopArtifactRelPath is the public path of one release train's desktop
// package.
func DesktopArtifactRelPath(appID, version, name string) string {
	segments := []string{StaticDesktopDir}
	if appID != "" {
		segments = append(segments, appID)
	}
	segments = append(segments, version, name)
	return JoinStatic("", "", segments...)
}

// DefaultDesktopBranding is the desktop identity one app builds with until the
// owner has saved a desktop branding row. Package id is always a dotted
// reverse domain on mobile, so the fixed suffix keeps the Tauri identifier
// legal without user input.
func DefaultDesktopBranding(app *AppBranding) DesktopBranding {
	return DesktopBranding{
		ProductName:      app.Name,
		BundleIdentifier: app.PackageID + DesktopAppSuffix,
	}
}

// DesktopBuildSteps lists the desktop pipeline steps in execution order.
var DesktopBuildSteps = []string{
	StepUINext,
	StepEngine,
	StepBrowserExtension,
	StepShell,
	StepOverlaySource,
	StepBundle,
	StepVerify,
}

// Artifact channels. A desktop job emits one row per channel; the mobile job
// emits a single apk row so download and publish have one code path.
const (
	ChannelAPK      = "apk"
	ChannelDeb      = "deb"
	ChannelRpm      = "rpm"
	ChannelAppImage = "appimage"
)

// ArtifactChannels lists every channel the builder can produce.
var ArtifactChannels = []string{ChannelAPK, ChannelDeb, ChannelRpm, ChannelAppImage}

// DesktopChannels are the channels one desktop-linux job collects.
var DesktopChannels = []string{ChannelDeb, ChannelRpm, ChannelAppImage}

// ChannelValid reports whether channel is a known artifact channel.
func ChannelValid(channel string) bool {
	for _, c := range ArtifactChannels {
		if c == channel {
			return true
		}
	}
	return false
}

// ChannelPublishable reports whether channel can be promoted to an update
// source. Only AppImage carries the Tauri updater protocol; deb and rpm are
// upgraded by the package manager, matching the shell's updater_supported.
func ChannelPublishable(channel string) bool {
	return channel == ChannelAppImage
}

// PrimaryChannelOf returns the single channel used when a download request omits
// the channel query. Empty when the platform needs an explicit channel.
func PrimaryChannelOf(platform string) string {
	switch platform {
	case PlatformAndroid:
		return ChannelAPK
	case PlatformDesktopLinux:
		return ChannelAppImage
	default:
		return ""
	}
}

// versionPattern must match the APP_VERSION_RE in mobile/src/updates/useOtaUpdate.ts
// so a published native version is recognised by the client.
var versionPattern = regexp.MustCompile(`^\d{6,8}$`)

// VersionValid reports whether version is a legal expo version string.
func VersionValid(version string) bool {
	return versionPattern.MatchString(version)
}

// StatusResp describes the capabilities of the app builder. The frontend uses
// it to hide entries and render forms without hard-coding platform behaviour.
type StatusResp struct {
	// Platforms lists every platform the deployment declares, keyed with its
	// availability so the frontend can disable the button and explain why.
	Platforms []*PlatformStatus `json:"platforms"`
	// VersionPattern is the shared mobile and desktop version rule.
	VersionPattern string `json:"version_pattern"`
	// AssetSlots is mobile-only; desktop ships its icon through branding.
	AssetSlots    []AssetSlotInfo `json:"asset_slots,omitempty"`
	MaxAssetBytes int64           `json:"max_asset_bytes"`
	// NextVersionCode is the smallest legal version code, so the frontend can
	// pre-fill the build form.
	NextVersionCode int            `json:"next_version_code"`
	Desktop         *DesktopStatus `json:"desktop,omitempty"`
}

// PlatformStatus is one platform's capability and preflight result.
type PlatformStatus struct {
	Platform       string   `json:"platform"`
	Enabled        bool     `json:"enabled"`
	Reason         string   `json:"reason,omitempty"`
	MissingPrereqs []string `json:"missing_prereqs,omitempty"`
	BuildSteps     []string `json:"build_steps"`
	Channels       []string `json:"channels,omitempty"`
	Workspace      string   `json:"workspace"`
}

// DesktopStatus carries the desktop-only knobs the frontend needs.
type DesktopStatus struct {
	PublishableChannels []string `json:"publishable_channels"`
	// Unsigned is true when no updater signing key is configured; the client
	// will refuse such updates, so the frontend flags the publish entry.
	Unsigned bool `json:"unsigned"`
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
	// SourceSnapshotID and SourceTaskID are written once at enqueue time and
	// never change. Empty means the job builds the template source.
	SourceSnapshotID *uuid.UUID `json:"source_snapshot_id,omitempty"`
	SourceTaskID     *uuid.UUID `json:"source_task_id,omitempty"`
}

// SourceSnapshot is the session source archive one job overlays onto the
// template workspace. ObjectKey is the full object key; SHA256 and SizeBytes
// are the values recorded when the snapshot was promoted, used to verify the
// download before any file is written.
type SourceSnapshot struct {
	ObjectKey string
	SHA256    string
	SizeBytes int64
}

// ObjectSource reads one object of object storage. *pkg/oss.Client implements
// it implicitly; declared here so the runner's tests can inject an in-memory
// fake without pulling in cloud SDK dependencies.
type ObjectSource interface {
	GetObject(ctx context.Context, key string) (io.ReadCloser, error)
}

// BuildJob is one packaging request.
type BuildJob struct {
	ID    uuid.UUID `json:"id"`
	AppID uuid.UUID `json:"app_id,omitempty"`
	// SourceSnapshotID and SourceTaskID point at the session snapshot this job
	// built from; nil when the template source was used.
	SourceSnapshotID    *uuid.UUID `json:"source_snapshot_id,omitempty"`
	SourceTaskID        *uuid.UUID `json:"source_task_id,omitempty"`
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
	// Artifacts is every channel of this job. Populated for job detail
	// responses; empty for list rows.
	Artifacts []*ArtifactInfo `json:"artifacts,omitempty"`
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
	Native  []*ReleaseStatus      `json:"native"`
	Desktop *DesktopReleaseStatus `json:"desktop,omitempty"`
	Ota     *OtaStatus            `json:"ota,omitempty"`
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
	// Artifacts is the full multi-channel set of one run. The single-value
	// fields above mirror Artifacts[0], the primary artifact, so the existing
	// job table columns and log display keep working unchanged.
	Artifacts []Artifact
}

// Artifact is one distributable file of a build, per channel.
type Artifact struct {
	Channel  string `json:"channel"`
	FileName string `json:"file_name"`
	Path     string `json:"artifact_path"`
	SHA256   string `json:"artifact_sha256"`
	Size     int64  `json:"artifact_size"`
}

// ArtifactInfo is the JSON projection of one artifact row for API responses.
type ArtifactInfo struct {
	Channel    string    `json:"channel"`
	FileName   string    `json:"file_name"`
	Size       int64     `json:"size"`
	SHA256     string    `json:"sha256"`
	DurationMS int64     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
	// Path is the server-side location of the file. It stays out of API
	// responses so a job detail never leaks the deployment filesystem.
	Path string `json:"-"`
}

// DesktopBranding is the single desktop branding of this deployment. It shares
// no field with the mobile Branding on purpose: the two clients evolve their
// identity independently.
type DesktopBranding struct {
	ID               uuid.UUID `json:"id"`
	ProductName      string    `json:"product_name"`
	BundleIdentifier string    `json:"bundle_identifier"`
	UpdaterEndpoint  string    `json:"updater_endpoint"`
	IconKey          string    `json:"icon_key,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// UpdateDesktopBrandingReq carries the editable desktop branding fields. Nil
// means unchanged.
type UpdateDesktopBrandingReq struct {
	ProductName      *string `json:"product_name,omitempty"`
	BundleIdentifier *string `json:"bundle_identifier,omitempty"`
	UpdaterEndpoint  *string `json:"updater_endpoint,omitempty"`
}

// AppDesktopBranding is the per-app identity of the desktop shell. The icon is
// deliberately absent: a desktop app shares apps.icon_key with its mobile
// build, so the two clients carry one asset.
type AppDesktopBranding struct {
	ID               uuid.UUID `json:"id"`
	AppID            uuid.UUID `json:"app_id"`
	ProductName      string    `json:"product_name"`
	BundleIdentifier string    `json:"bundle_identifier"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// UpdateAppDesktopBrandingReq carries the editable per-app desktop branding
// fields. Nil means unchanged.
type UpdateAppDesktopBrandingReq struct {
	ProductName      *string `json:"product_name,omitempty"`
	BundleIdentifier *string `json:"bundle_identifier,omitempty"`
}

// AppBuildPlatform describes one build target available to one app. enabled is
// false when the target is unreachable, and reason carries the reason the UI
// shows next to it.
type AppBuildPlatform struct {
	Platform string `json:"platform"`
	Enabled  bool   `json:"enabled"`
	Reason   string `json:"reason,omitempty"`
	// UpdaterEndpoint is the URL the desktop client of this app polls. It is
	// derived from the app id, so each app owns its update source.
	UpdaterEndpoint string `json:"updater_endpoint,omitempty"`
}

// DesktopReleaseManifest is the Tauri updater protocol record the desktop
// client polls. signature is empty when no updater signing key is configured;
// the client refuses to update in that case, so unsigned instances keep the
// manifest only as a version record and a download entry.
type DesktopReleaseManifest struct {
	Version   string `json:"version"`
	Notes     string `json:"notes"`
	PubDate   string `json:"pub_date"`
	URL       string `json:"url"`
	Signature string `json:"signature"`
}

// DesktopReleaseStatus describes the current desktop update release.
type DesktopReleaseStatus struct {
	Platform        string                  `json:"platform"`
	Enabled         bool                    `json:"enabled"`
	Manifest        *DesktopReleaseManifest `json:"manifest,omitempty"`
	ArtifactPresent bool                    `json:"artifact_present"`
	ArtifactSHA256  string                  `json:"artifact_sha256,omitempty"`
	Unsigned        bool                    `json:"unsigned"`
}

// AppBranding carries the per-app build inputs of one self-service job.
// ServerURL and UpdatesServer come from the instance branding instead.
type AppBranding struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	Name             string
	PackageID        string
	IconKey          string
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

	// GetAppDesktopBranding returns one app's desktop identity, or nil until the
	// owner has saved any. Defaults come from the app row in the usecase.
	GetAppDesktopBranding(ctx context.Context, appID uuid.UUID) (*AppDesktopBranding, error)
	// SaveAppDesktopBranding upserts one app's desktop identity.
	SaveAppDesktopBranding(ctx context.Context, appID uuid.UUID, req *UpdateAppDesktopBrandingReq) (*AppDesktopBranding, error)

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
	// ListByAppAll returns every job of one app, newest first. The owner is not
	// filtered, so only an admin may call it.
	ListByAppAll(ctx context.Context, appID uuid.UUID, limit int) ([]*BuildJob, error)
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

	// SetAppIconKey stores one app's custom icon marker on the apps table.
	SetAppIconKey(ctx context.Context, appID uuid.UUID, key string) error

	// GetJobSourceSnapshot returns the session snapshot one job overlays, nil
	// when the job builds the template source.
	GetJobSourceSnapshot(ctx context.Context, id uuid.UUID) (*SourceSnapshot, error)

	// Desktop branding lives in a second singleton table, independent of the
	// mobile branding: the two clients evolve their identity separately.
	GetDesktopBranding(ctx context.Context) (*DesktopBranding, error)
	SaveDesktopBranding(ctx context.Context, req *UpdateDesktopBrandingReq) (*DesktopBranding, error)
	SetDesktopIconKey(ctx context.Context, key string) (*DesktopBranding, error)
	ClearDesktopIconKey(ctx context.Context) (*DesktopBranding, error)

	// SaveArtifacts writes the full channel set of one job. It replaces any
	// earlier rows of the same job so a retried worker keeps the table in
	// step with the job table's primary artifact columns.
	SaveArtifacts(ctx context.Context, jobID uuid.UUID, platform string, artifacts []Artifact) error
	ListArtifacts(ctx context.Context, jobID uuid.UUID) ([]*ArtifactInfo, error)
	// GetArtifact returns one channel row of one job, nil when absent.
	GetArtifact(ctx context.Context, jobID uuid.UUID, channel string) (*ArtifactInfo, error)
}
