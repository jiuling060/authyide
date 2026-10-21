// Package usecase implements the business rules of the mobile app builder.
package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/samber/do"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/assetstore"
	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
	"github.com/chaitin/MonkeyCode/backend/config"
	"github.com/chaitin/MonkeyCode/backend/errcode"
)

const (
	manifestDir  = "app-version"
	otaDir       = "ota"
	releaseDir   = "mobile"
	artifactName = "authyide.apk"

	defaultStaticDir  = "/opt/authyide/data/static"
	defaultMaxAsset   = 4 << 20
	defaultAppVersion = 1
)

var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// Usecase is the app builder service boundary.
type Usecase interface {
	Enabled() bool
	Status(ctx context.Context) (*domain.StatusResp, error)

	GetBranding(ctx context.Context) (*domain.Branding, error)
	SaveBranding(ctx context.Context, req *domain.UpdateBrandingReq) (*domain.Branding, error)
	UploadAsset(ctx context.Context, slot string, data []byte) (*domain.Branding, error)
	ClearAsset(ctx context.Context, slot string) (*domain.Branding, error)

	EnqueueBuild(ctx context.Context, userID uuid.UUID, req *domain.EnqueueBuildReq) (*domain.BuildJob, error)
	EnqueueBuildForApp(ctx context.Context, owner, appID uuid.UUID, req *domain.EnqueueBuildReq) (*domain.BuildJob, error)
	ListJobs(ctx context.Context, limit int) ([]*domain.BuildJob, error)
	GetJob(ctx context.Context, id uuid.UUID) (*domain.BuildJob, error)
	DownloadArtifact(ctx context.Context, id uuid.UUID) (string, error)
	DownloadOwned(ctx context.Context, owner, id uuid.UUID) (string, error)
	Publish(ctx context.Context, id uuid.UUID) (*domain.ReleaseStatus, error)
	PublishOwned(ctx context.Context, owner, id uuid.UUID) (*domain.ReleaseStatus, error)
	Releases(ctx context.Context) (*domain.ReleasesResp, error)

	OtaStatus(ctx context.Context) (*domain.OtaStatus, error)
	PublishOta(ctx context.Context, req *domain.PublishOtaReq) (*domain.OtaStatus, error)
}

type usecase struct {
	cfg    *config.Config
	repo   domain.AppBuilderRepo
	assets assetstore.Store
	logger *slog.Logger
}

// NewUsecase creates the app builder service.
func NewUsecase(i *do.Injector) (Usecase, error) {
	cfg := do.MustInvoke[*config.Config](i)
	repo := do.MustInvoke[domain.AppBuilderRepo](i)
	assets := do.MustInvoke[assetstore.Store](i)
	logger := do.MustInvoke[*slog.Logger](i)
	return &usecase{
		cfg:    cfg,
		repo:   repo,
		assets: assets,
		logger: logger.With("component", "app-builder"),
	}, nil
}

func (u *usecase) Enabled() bool {
	return u.cfg.MobileBuild.Enabled
}

// Status reports the capabilities the frontend needs to render the page.
func (u *usecase) Status(ctx context.Context) (*domain.StatusResp, error) {
	mc := u.cfg.MobileBuild
	maxAsset := mc.MaxAssetBytes
	if maxAsset <= 0 {
		maxAsset = defaultMaxAsset
	}
	branding, err := u.GetBranding(ctx)
	if err != nil {
		return nil, err
	}
	slots := emptyAssets()
	if branding != nil && len(branding.Assets) > 0 {
		slots = branding.Assets
	}
	nextVersionCode := 1
	if max, err := u.repo.MaxVersionCode(ctx); err == nil && max+1 > nextVersionCode {
		nextVersionCode = max + 1
	}
	return &domain.StatusResp{
		Enabled:         u.Enabled(),
		Workspace:       mc.Workspace,
		Platforms:       []string{domain.PlatformAndroid},
		AssetSlots:      slots,
		BuildSteps:      domain.BuildSteps,
		VersionPattern:  `^\d{6,8}$`,
		MaxAssetBytes:   maxAsset,
		GradleTimeout:   mc.GradleTimeoutDuration().String(),
		NextVersionCode: nextVersionCode,
	}, nil
}

// ---------------------------------------------------------------------------
// Branding
// ---------------------------------------------------------------------------

// GetBranding returns the saved branding, or the values currently set in the
// mobile app.json when the admin has not saved anything yet.
func (u *usecase) GetBranding(ctx context.Context) (*domain.Branding, error) {
	b, err := u.repo.GetBranding(ctx)
	if err != nil {
		return nil, err
	}
	if b == nil {
		b = u.defaultBranding()
	}
	return u.fillAssetMeta(ctx, b), nil
}

func (u *usecase) defaultBranding() *domain.Branding {
	b := &domain.Branding{
		Name:        "AuthyIDE",
		ServerURL:   u.publicBase(),
		VersionCode: defaultAppVersion,
	}
	data, err := os.ReadFile(filepath.Join(u.cfg.MobileBuild.Workspace, "app.json"))
	if err != nil {
		u.logger.Warn("read app.json failed", "error", err)
		return b
	}
	var doc struct {
		Expo struct {
			Name    string `json:"name"`
			Version string `json:"version"`
			Extra   struct {
				BaseURL string `json:"baseUrl"`
			} `json:"extra"`
			Android struct {
				VersionCode int `json:"versionCode"`
			} `json:"android"`
		} `json:"expo"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		u.logger.Warn("parse app.json failed", "error", err)
		return b
	}
	if doc.Expo.Name != "" {
		b.Name = doc.Expo.Name
	}
	b.Version = strings.TrimSpace(doc.Expo.Version)
	if doc.Expo.Android.VersionCode > 0 {
		b.VersionCode = doc.Expo.Android.VersionCode
	}
	if doc.Expo.Extra.BaseURL != "" {
		b.ServerURL = strings.TrimSpace(doc.Expo.Extra.BaseURL)
	}
	return b
}

// SaveBranding merges the partial update into the current branding and only
// persists the row once the merged value is valid.
func (u *usecase) SaveBranding(ctx context.Context, req *domain.UpdateBrandingReq) (*domain.Branding, error) {
	if req == nil {
		return nil, errcode.ErrBrandingInvalid
	}
	current, err := u.GetBranding(ctx)
	if err != nil {
		return nil, err
	}
	if current == nil {
		current = u.defaultBranding()
	}

	merged := *current
	if req.Name != nil {
		merged.Name = strings.TrimSpace(*req.Name)
	}
	if req.Version != nil {
		merged.Version = strings.TrimSpace(*req.Version)
	}
	if req.VersionCode != nil {
		merged.VersionCode = *req.VersionCode
	}
	if req.ServerURL != nil {
		merged.ServerURL = strings.TrimSpace(*req.ServerURL)
	}
	if req.UpdatesServer != nil {
		merged.UpdatesServer = strings.TrimSpace(*req.UpdatesServer)
	}
	if err := validateBranding(&merged); err != nil {
		return nil, errcode.ErrBrandingInvalid.Wrap(err)
	}

	b, err := u.repo.SaveBranding(ctx, req)
	if err != nil {
		return nil, err
	}
	return u.fillAssetMeta(ctx, b), nil
}

func (u *usecase) fillAssetMeta(ctx context.Context, b *domain.Branding) *domain.Branding {
	if b == nil {
		return nil
	}
	if b.Assets == nil {
		b.Assets = emptyAssets()
	}
	for i := range b.Assets {
		slot := b.Assets[i].Slot
		if b.Assets[i].Key == "" || !domain.AssetSlotValid(slot) {
			continue
		}
		size, err := u.assets.Size(ctx, slot)
		if err == nil {
			b.Assets[i].Bytes = size
		}
	}
	return b
}

func emptyAssets() []domain.AssetSlotInfo {
	slots := make([]domain.AssetSlotInfo, 0, len(domain.AssetSlots))
	for _, slot := range domain.AssetSlots {
		slots = append(slots, domain.AssetSlotInfo{Slot: slot})
	}
	return slots
}

func (u *usecase) UploadAsset(ctx context.Context, slot string, data []byte) (*domain.Branding, error) {
	if !domain.AssetSlotValid(slot) {
		return nil, errcode.ErrAssetSlotInvalid
	}
	limit := u.cfg.MobileBuild.MaxAssetBytes
	if limit <= 0 {
		limit = defaultMaxAsset
	}
	if len(data) == 0 || int64(len(data)) > limit {
		return nil, errcode.ErrAssetInvalid.Wrap(fmt.Errorf("asset must be 1..%d bytes", limit))
	}
	if !hasPNGSignature(data) {
		return nil, errcode.ErrAssetInvalid.Wrap(errors.New("asset must be a PNG image"))
	}
	if err := u.assets.Put(ctx, slot, data); err != nil {
		return nil, fmt.Errorf("store asset %s: %w", slot, err)
	}
	b, err := u.repo.SetAssetKey(ctx, slot, slot)
	if err != nil {
		return nil, err
	}
	return u.fillAssetMeta(ctx, b), nil
}

func (u *usecase) ClearAsset(ctx context.Context, slot string) (*domain.Branding, error) {
	if !domain.AssetSlotValid(slot) {
		return nil, errcode.ErrAssetSlotInvalid
	}
	if err := u.assets.Delete(ctx, slot); err != nil {
		return nil, fmt.Errorf("delete asset %s: %w", slot, err)
	}
	b, err := u.repo.ClearAssetKey(ctx, slot)
	if err != nil {
		return nil, err
	}
	return u.fillAssetMeta(ctx, b), nil
}

func validateBranding(b *domain.Branding) error {
	if b == nil {
		return errors.New("branding is empty")
	}
	if strings.TrimSpace(b.Name) == "" {
		return errors.New("name is required")
	}
	if !domain.VersionValid(b.Version) {
		return fmt.Errorf("version %q must match ^\\d{6,8}$", b.Version)
	}
	if b.VersionCode < 1 {
		return errors.New("version_code must be >= 1")
	}
	if err := validateHTTPURL(b.ServerURL); err != nil {
		return fmt.Errorf("server_url: %w", err)
	}
	if b.UpdatesServer != "" {
		if err := validateHTTPURL(b.UpdatesServer); err != nil {
			return fmt.Errorf("updates_server: %w", err)
		}
	}
	return nil
}

func validateHTTPURL(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return errors.New("url is empty")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return err
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("must be an http or https url")
	}
	return nil
}

func hasPNGSignature(data []byte) bool {
	return len(data) >= len(pngSignature) && string(data[:len(pngSignature)]) == string(pngSignature)
}

// ---------------------------------------------------------------------------
// Builds
// ---------------------------------------------------------------------------

func (u *usecase) EnqueueBuild(ctx context.Context, userID uuid.UUID, req *domain.EnqueueBuildReq) (*domain.BuildJob, error) {
	return u.enqueueBuild(ctx, userID, req, nil)
}

// EnqueueBuildForApp is the self-service enqueue path. The app is resolved and
// validated before enqueue so a request against someone else's app reads as a
// missing app instead of a successful build.
func (u *usecase) EnqueueBuildForApp(ctx context.Context, owner, appID uuid.UUID, req *domain.EnqueueBuildReq) (*domain.BuildJob, error) {
	app, err := u.repo.GetAppBranding(ctx, appID)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, errcode.ErrAppNotFound
	}
	return u.enqueueBuild(ctx, owner, req, app)
}

// enqueueBuild is the shared enqueue path. app is nil for admin-console builds,
// which hold the whole worker; user-center builds hold one user's queue slot.
func (u *usecase) enqueueBuild(
	ctx context.Context,
	userID uuid.UUID,
	req *domain.EnqueueBuildReq,
	app *domain.AppBranding,
) (*domain.BuildJob, error) {
	if req == nil {
		return nil, errcode.ErrBrandingInvalid
	}
	req.Platform = strings.TrimSpace(req.Platform)
	if req.Platform == "" {
		req.Platform = domain.PlatformAndroid
	}
	if req.Platform != domain.PlatformAndroid {
		return nil, errcode.ErrBrandingInvalid.Wrap(fmt.Errorf("unsupported platform %q", req.Platform))
	}
	if app != nil && !platformEnabled(app.EnabledPlatforms, req.Platform) {
		return nil, errcode.ErrAppInvalidPlatform.Wrap(
			fmt.Errorf("%q is not in the app's platform set", req.Platform))
	}
	req.Version = strings.TrimSpace(req.Version)
	if !domain.VersionValid(req.Version) {
		return nil, errcode.ErrBrandingInvalid.Wrap(
			fmt.Errorf("version %q must match ^\\d{6,8}$", req.Version))
	}

	if app == nil {
		running, err := u.repo.AnyRunning(ctx)
		if err != nil {
			return nil, err
		}
		if running {
			return nil, errcode.ErrBuildBusy
		}
	} else {
		busy, err := u.repo.HasActiveJobForUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		if busy {
			return nil, errcode.ErrBuildBusy
		}
	}

	current, err := u.repo.GetBranding(ctx)
	if err != nil {
		return nil, err
	}
	if current == nil {
		current = u.defaultBranding()
	}
	if strings.TrimSpace(current.Name) == "" || strings.TrimSpace(current.ServerURL) == "" {
		return nil, errcode.ErrBrandingInvalid.Wrap(errors.New("branding is not configured"))
	}

	if app != nil {
		req.AppID = app.ID
		if n, err := u.repo.CountQueued(ctx); err != nil {
			return nil, err
		} else if u.cfg.MobileBuild.QueueDepth > 0 && n >= u.cfg.MobileBuild.QueueDepth {
			return nil, errcode.ErrBuildQueueFull
		}
	}

	var maxCode int
	if app != nil {
		maxCode, err = u.repo.MaxVersionCodeForApp(ctx, app.ID)
	} else {
		maxCode, err = u.repo.MaxVersionCode(ctx)
	}
	if err != nil {
		return nil, err
	}
	if req.VersionCode == 0 {
		req.VersionCode = maxCode + 1
	}
	if req.VersionCode <= maxCode {
		return nil, errcode.ErrReleaseVersionTooOld.Wrap(
			fmt.Errorf("version_code %d must be greater than %d", req.VersionCode, maxCode))
	}

	job, err := u.repo.CreateJob(ctx, userID, req)
	if err != nil {
		return nil, err
	}
	u.logger.InfoContext(ctx, "build enqueued",
		"job_id", job.ID, "version", job.TargetVersion, "version_code", job.VersionCode)
	return withDownloadURL(job), nil
}

// platformEnabled reports whether platform is in the app's platform set; an
// empty set accepts the default platform.
func platformEnabled(enabled []string, platform string) bool {
	if len(enabled) == 0 {
		return true
	}
	for _, p := range enabled {
		if p == platform {
			return true
		}
	}
	return false
}

func (u *usecase) ListJobs(ctx context.Context, limit int) ([]*domain.BuildJob, error) {
	jobs, err := u.repo.ListJobs(ctx, limit)
	if err != nil {
		return nil, err
	}
	for _, job := range jobs {
		withDownloadURL(job)
	}
	return jobs, nil
}

func (u *usecase) GetJob(ctx context.Context, id uuid.UUID) (*domain.BuildJob, error) {
	job, err := u.repo.GetJob(ctx, id)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, errcode.ErrBuildNotFound
	}
	return withDownloadURL(job), nil
}

// DownloadArtifact resolves the artifact of a succeeded job. The path is
// validated so the handler can stream it without a second check.
func (u *usecase) DownloadArtifact(ctx context.Context, id uuid.UUID) (string, error) {
	job, err := u.repo.GetJob(ctx, id)
	if err != nil {
		return "", err
	}
	if job == nil {
		return "", errcode.ErrBuildNotFound
	}
	return u.downloadJob(job)
}

// DownloadOwned resolves the artifact of one of the owner's jobs.
func (u *usecase) DownloadOwned(ctx context.Context, owner, id uuid.UUID) (string, error) {
	job, err := u.repo.GetOwnedJob(ctx, owner, id)
	if err != nil {
		return "", err
	}
	if job == nil {
		return "", errcode.ErrBuildNotFound
	}
	return u.downloadJob(job)
}

func (u *usecase) downloadJob(job *domain.BuildJob) (string, error) {
	if job == nil {
		return "", errcode.ErrBuildNotFound
	}
	if job.Status != domain.BuildStatusSucceeded {
		return "", errcode.ErrBuildNotSucceeded
	}
	if job.ArtifactPath == "" {
		return "", errcode.ErrBuildArtifactMissing
	}
	fi, err := os.Stat(job.ArtifactPath)
	if err != nil || fi.Size() == 0 {
		return "", errcode.ErrBuildArtifactMissing
	}
	return job.ArtifactPath, nil
}

// withDownloadURL exposes the download endpoint only for jobs whose artifact
// exists, so a queued job never advertises a 404 link.
func withDownloadURL(job *domain.BuildJob) *domain.BuildJob {
	if job != nil && job.Status == domain.BuildStatusSucceeded && job.ArtifactPath != "" {
		job.ArtifactDownloadURL = buildDownloadPath(job.ID)
	}
	return job
}

func buildDownloadPath(id uuid.UUID) string {
	return "/api/v1/admin/app-builder/builds/" + id.String() + "/download"
}

// ---------------------------------------------------------------------------
// Releases
// ---------------------------------------------------------------------------

// Publish copies the artifact into the static tree and points the native
// update manifest at it. Both side effects are rolled back if either one
// fails, so a partial release never gets served.
func (u *usecase) Publish(ctx context.Context, id uuid.UUID) (*domain.ReleaseStatus, error) {
	job, err := u.repo.GetJob(ctx, id)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, errcode.ErrBuildNotFound
	}
	return u.publishJob(ctx, job)
}

// PublishOwned publishes one of the owner's jobs to the release channel.
func (u *usecase) PublishOwned(ctx context.Context, owner, id uuid.UUID) (*domain.ReleaseStatus, error) {
	job, err := u.repo.GetOwnedJob(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, errcode.ErrBuildNotFound
	}
	return u.publishJob(ctx, job)
}

func (u *usecase) publishJob(ctx context.Context, job *domain.BuildJob) (*domain.ReleaseStatus, error) {
	if job == nil {
		return nil, errcode.ErrBuildNotFound
	}
	if job.Status != domain.BuildStatusSucceeded {
		return nil, errcode.ErrBuildNotSucceeded
	}
	if job.ArtifactPath == "" {
		return nil, errcode.ErrBuildArtifactMissing
	}
	if fi, err := os.Stat(job.ArtifactPath); err != nil || fi.Size() == 0 {
		return nil, errcode.ErrBuildArtifactMissing
	}

	version := job.TargetVersion
	current, err := u.ReadManifest(ctx, domain.PlatformAndroid)
	if err != nil {
		return nil, err
	}
	if current != nil && !versionGreater(version, current.Version) {
		return nil, errcode.ErrReleaseVersionTooOld.Wrap(
			fmt.Errorf("manifest already at %s", current.Version))
	}

	dest := filepath.Join(u.staticDir(), releaseDir, version, artifactName)
	if err := copyFile(job.ArtifactPath, dest); err != nil {
		return nil, err
	}
	rollback := func() {
		os.Remove(dest)
		os.Remove(filepath.Dir(dest))
	}

	manifest := domain.ReleaseManifest{Version: version, URL: u.artifactURL(version)}
	manifestPath := filepath.Join(u.staticDir(), manifestDir, domain.PlatformAndroid+".json")
	if err := writeManifestAtomic(manifestPath, manifest); err != nil {
		rollback()
		return nil, err
	}
	if err := u.repo.MarkJobPublished(ctx, job.ID, job.ArtifactSHA256); err != nil {
		rollback()
		return nil, err
	}

	u.logger.InfoContext(ctx, "native release published",
		"version", version, "version_code", job.VersionCode, "url", manifest.URL)
	return u.ReleaseStatus(ctx, domain.PlatformAndroid)
}

func (u *usecase) Releases(ctx context.Context) (*domain.ReleasesResp, error) {
	st, err := u.ReleaseStatus(ctx, domain.PlatformAndroid)
	if err != nil {
		return nil, err
	}
	ota, err := u.OtaStatus(ctx)
	if err != nil {
		return nil, err
	}
	return &domain.ReleasesResp{Native: []*domain.ReleaseStatus{st}, Ota: ota}, nil
}

// ReleaseStatus reads the native manifest and verifies the artifact it points
// at is actually served from disk.
func (u *usecase) ReleaseStatus(ctx context.Context, platform string) (*domain.ReleaseStatus, error) {
	if platform == "" {
		platform = domain.PlatformAndroid
	}
	manifest, err := u.ReadManifest(ctx, platform)
	if err != nil {
		return nil, err
	}
	st := &domain.ReleaseStatus{Platform: platform, Enabled: u.cfg.MobileBuild.Enabled}
	if manifest == nil {
		return st, nil
	}
	st.Manifest = manifest
	st.ArtifactURL = manifest.URL

	local := u.localArtifactPath(manifest.URL)
	if local == "" {
		return st, nil
	}
	fi, err := os.Stat(local)
	if err != nil || fi.Size() == 0 {
		return st, nil
	}
	st.ArtifactPresent = true
	sum, err := sha256File(local)
	if err == nil {
		st.ArtifactSHA256 = sum
	}
	return st, nil
}

func (u *usecase) ReadManifest(ctx context.Context, platform string) (*domain.ReleaseManifest, error) {
	if platform == "" {
		platform = domain.PlatformAndroid
	}
	return readManifestFile(filepath.Join(u.staticDir(), manifestDir, platform+".json"))
}

func readManifestFile(path string) (*domain.ReleaseManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read release manifest: %w", err)
	}
	var m domain.ReleaseManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse release manifest: %w", err)
	}
	if m.Version == "" || m.URL == "" {
		return nil, nil
	}
	return &m, nil
}

func (u *usecase) ReadOtaManifest(ctx context.Context) (*domain.ReleaseManifest, error) {
	return readManifestFile(filepath.Join(u.staticDir(), otaDir, domain.PlatformAndroid+".json"))
}

// ---------------------------------------------------------------------------
// OTA
// ---------------------------------------------------------------------------

func (u *usecase) OtaStatus(ctx context.Context) (*domain.OtaStatus, error) {
	st := &domain.OtaStatus{
		Enabled: u.cfg.MobileBuild.OtaEnabled,
		URL:     strings.TrimSpace(u.cfg.MobileBuild.UpdatesURL),
	}
	if !st.Enabled {
		return st, nil
	}
	manifest, err := u.ReadOtaManifest(ctx)
	if err != nil {
		return nil, err
	}
	st.Manifest = manifest
	return st, nil
}

// PublishOta writes the OTA manifest. The native APK release is untouched:
// OTA distributes JS bundles and native distributes APKs.
func (u *usecase) PublishOta(ctx context.Context, req *domain.PublishOtaReq) (*domain.OtaStatus, error) {
	if !u.cfg.MobileBuild.OtaEnabled {
		return nil, errcode.ErrOtaDisabled
	}
	if req == nil || !domain.VersionValid(strings.TrimSpace(req.Version)) {
		return nil, errcode.ErrBrandingInvalid.Wrap(
			errors.New("version must match ^\\d{6,8}$"))
	}
	req.Version = strings.TrimSpace(req.Version)
	if err := validateHTTPURL(req.URL); err != nil {
		return nil, errcode.ErrBrandingInvalid.Wrap(fmt.Errorf("url: %w", err))
	}

	current, err := u.ReadOtaManifest(ctx)
	if err != nil {
		return nil, err
	}
	if current != nil && !versionGreater(req.Version, current.Version) {
		return nil, errcode.ErrReleaseVersionTooOld.Wrap(
			fmt.Errorf("ota manifest already at %s", current.Version))
	}

	path := filepath.Join(u.staticDir(), otaDir, domain.PlatformAndroid+".json")
	if err := writeManifestAtomic(path, domain.ReleaseManifest{Version: req.Version, URL: req.URL}); err != nil {
		return nil, err
	}
	u.logger.InfoContext(ctx, "ota manifest published", "version", req.Version, "url", req.URL)
	return u.OtaStatus(ctx)
}

// ---------------------------------------------------------------------------
// Filesystem helpers
// ---------------------------------------------------------------------------

func (u *usecase) staticDir() string {
	if dir := strings.TrimSpace(u.cfg.StaticFiles.Dir); dir != "" {
		return dir
	}
	return defaultStaticDir
}

func (u *usecase) publicBase() string {
	return strings.TrimRight(strings.TrimSpace(u.cfg.Server.BaseURL), "/")
}

// artifactURL is the public address the native client downloads the APK from.
func (u *usecase) artifactURL(version string) string {
	prefix := strings.TrimRight(strings.TrimSpace(u.cfg.StaticFiles.RoutePrefix), "/")
	return u.publicBase() + prefix + "/" + releaseDir + "/" + version + "/" + artifactName
}

// localArtifactPath maps a public static URL back to the local file so the
// release status can verify what the client actually downloads.
func (u *usecase) localArtifactPath(rawURL string) string {
	prefix := strings.TrimRight(strings.TrimSpace(u.cfg.StaticFiles.RoutePrefix), "/")
	base := u.publicBase() + prefix
	if !strings.HasPrefix(rawURL, base) {
		return ""
	}
	rel := strings.TrimPrefix(rawURL, base)
	if rel == "" {
		return ""
	}
	return filepath.Join(u.staticDir(), strings.TrimLeft(rel, "/"))
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create release dir: %w", err)
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open artifact: %w", err)
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dst), ".tmp-*")
	if err != nil {
		return fmt.Errorf("create release temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return fmt.Errorf("copy artifact: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync artifact: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close artifact: %w", err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("chmod artifact: %w", err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("replace artifact: %w", err)
	}
	return nil
}

// writeManifestAtomic writes to a temp file and renames, so a reader never
// observes a half-written manifest.
func writeManifestAtomic(path string, manifest domain.ReleaseManifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create manifest dir: %w", err)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create manifest temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write manifest: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync manifest: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close manifest: %w", err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("chmod manifest: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace manifest: %w", err)
	}
	return nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// versionGreater compares expo version strings. Versions are 6-8 digit
// integers so they compare numerically rather than lexicographically.
func versionGreater(candidate, current string) bool {
	c, cerr := strconv.ParseUint(candidate, 10, 64)
	u, uerr := strconv.ParseUint(current, 10, 64)
	if cerr != nil || uerr != nil {
		return candidate > current
	}
	return c > u
}
