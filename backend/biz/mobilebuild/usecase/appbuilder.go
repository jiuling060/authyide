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
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/samber/do"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/assetstore"
	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/runner"
	"github.com/chaitin/MonkeyCode/backend/config"
	"github.com/chaitin/MonkeyCode/backend/errcode"
)

const (
	manifestDir  = "app-version"
	otaDir       = "ota"
	releaseDir   = "mobile"
	artifactName = "authyide.apk"

	// desktopReleaseDir holds the desktop artifacts and the updater manifest the
	// Tauri client polls. The updater appends the platform name to the endpoint,
	// so an endpoint of .../desktop/latest.json reads latest-linux.json.
	desktopReleaseDir   = "desktop"
	desktopManifestName = "latest-linux.json"

	defaultStaticDir  = "/opt/authyide/data/static"
	defaultMaxAsset   = 4 << 20
	defaultAppVersion = 1

	// prereqProbeTimeout caps every host prerequisite probe so a hung tool
	// cannot stall the status endpoint.
	prereqProbeTimeout = 3 * time.Second
)

var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// bundleIDRe is the reverse-domain form Tauri requires from an identifier.
var bundleIDRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*((\.[A-Za-z][A-Za-z0-9]*)+)$`)

// Usecase is the app builder service boundary.
type Usecase interface {
	Enabled() bool
	Status(ctx context.Context) (*domain.StatusResp, error)

	GetBranding(ctx context.Context) (*domain.Branding, error)
	SaveBranding(ctx context.Context, req *domain.UpdateBrandingReq) (*domain.Branding, error)
	UploadAsset(ctx context.Context, slot string, data []byte) (*domain.Branding, error)
	ClearAsset(ctx context.Context, slot string) (*domain.Branding, error)

	UploadAppIcon(ctx context.Context, owner, appID uuid.UUID, data []byte) error
	ClearAppIcon(ctx context.Context, owner, appID uuid.UUID) error
	DownloadAppIcon(ctx context.Context, owner, appID uuid.UUID) (string, error)

	GetAppDesktopBranding(ctx context.Context, owner, appID uuid.UUID) (*domain.AppDesktopBranding, error)
	SaveAppDesktopBranding(ctx context.Context, owner, appID uuid.UUID, req *domain.UpdateAppDesktopBrandingReq) (*domain.AppDesktopBranding, error)
	Platforms(ctx context.Context, owner, appID uuid.UUID, desktopLicensed bool) ([]*domain.AppBuildPlatform, error)

	GetDesktopBranding(ctx context.Context) (*domain.DesktopBranding, error)
	SaveDesktopBranding(ctx context.Context, req *domain.UpdateDesktopBrandingReq) (*domain.DesktopBranding, error)
	UploadDesktopAsset(ctx context.Context, data []byte) (*domain.DesktopBranding, error)
	ClearDesktopAsset(ctx context.Context) (*domain.DesktopBranding, error)

	EnqueueBuild(ctx context.Context, userID uuid.UUID, req *domain.EnqueueBuildReq) (*domain.BuildJob, error)
	EnqueueBuildForApp(ctx context.Context, owner, appID uuid.UUID, req *domain.EnqueueBuildReq) (*domain.BuildJob, error)
	ListJobs(ctx context.Context, limit int) ([]*domain.BuildJob, error)
	GetJob(ctx context.Context, id uuid.UUID) (*domain.BuildJob, error)
	DownloadArtifact(ctx context.Context, id uuid.UUID, channel string) (string, string, error)
	DownloadOwned(ctx context.Context, owner, id uuid.UUID, channel string) (string, string, error)
	Publish(ctx context.Context, id uuid.UUID, channel string) (*domain.ReleaseStatus, error)
	PublishOwned(ctx context.Context, owner, id uuid.UUID, channel string) (*domain.ReleaseStatus, error)
	Releases(ctx context.Context) (*domain.ReleasesResp, error)

	OtaStatus(ctx context.Context) (*domain.OtaStatus, error)
	PublishOta(ctx context.Context, req *domain.PublishOtaReq) (*domain.OtaStatus, error)
}

type usecase struct {
	cfg       *config.Config
	repo      domain.AppBuilderRepo
	assets    assetstore.Store
	appAssets assetstore.AppStore
	logger    *slog.Logger
}

// NewUsecase creates the app builder service.
func NewUsecase(i *do.Injector) (Usecase, error) {
	cfg := do.MustInvoke[*config.Config](i)
	repo := do.MustInvoke[domain.AppBuilderRepo](i)
	assets := do.MustInvoke[assetstore.Store](i)
	appAssets := do.MustInvoke[assetstore.AppStore](i)
	logger := do.MustInvoke[*slog.Logger](i)
	return &usecase{
		cfg:       cfg,
		repo:      repo,
		assets:    assets,
		appAssets: appAssets,
		logger:    logger.With("component", "app-builder"),
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

	resp := &domain.StatusResp{
		Platforms: []*domain.PlatformStatus{
			u.androidStatus(ctx, mc, nextVersionCode),
			u.desktopStatus(ctx, mc),
		},
		VersionPattern:  `^\d{6,8}$`,
		AssetSlots:      slots,
		MaxAssetBytes:   maxAsset,
		NextVersionCode: nextVersionCode,
	}
	if mc.PlatformEnabled(domain.PlatformDesktopLinux) {
		resp.Desktop = &domain.DesktopStatus{
			PublishableChannels: desktopPublishableChannels(),
			Unsigned:            mc.UpdaterSigningKeyPath == "",
		}
	}
	return resp, nil
}

func (u *usecase) androidStatus(ctx context.Context, mc config.MobileBuildConfig, nextVersionCode int) *domain.PlatformStatus {
	st := &domain.PlatformStatus{
		Platform:       domain.PlatformAndroid,
		Enabled:        u.Enabled(),
		MissingPrereqs: u.missingPrereqs(ctx, domain.PlatformAndroid),
		BuildSteps:     domain.BuildSteps,
		Channels:       []string{domain.ChannelAPK},
		Workspace:      mc.Workspace,
	}
	if !st.Enabled {
		st.Reason = errcode.ErrPlatformDisabled.Error()
	}
	return st
}

func (u *usecase) desktopStatus(ctx context.Context, mc config.MobileBuildConfig) *domain.PlatformStatus {
	st := &domain.PlatformStatus{
		Platform:       domain.PlatformDesktopLinux,
		Enabled:        mc.PlatformEnabled(domain.PlatformDesktopLinux),
		MissingPrereqs: u.missingPrereqs(ctx, domain.PlatformDesktopLinux),
		BuildSteps:     domain.DesktopBuildSteps,
		Channels:       domain.DesktopChannels,
		Workspace:      mc.DesktopWorkspace,
	}
	if !st.Enabled {
		st.Reason = errcode.ErrPlatformDisabled.Error()
	}
	return st
}

// desktopPublishableChannels is the subset of desktop channels the Tauri
// updater protocol can consume.
func desktopPublishableChannels() []string {
	out := make([]string, 0, 1)
	for _, c := range domain.DesktopChannels {
		if domain.ChannelPublishable(c) {
			out = append(out, c)
		}
	}
	return out
}

// missingPrereqs reports the host prerequisites platform lacks.
func (u *usecase) missingPrereqs(ctx context.Context, platform string) []string {
	pctx, cancel := context.WithTimeout(ctx, prereqProbeTimeout)
	defer cancel()
	return runner.MissingPrereqs(pctx, platform, runner.PrereqProbe{
		AndroidSDK:       u.cfg.MobileBuild.AndroidSDK,
		JavaHome:         u.cfg.MobileBuild.JavaHome,
		DesktopWorkspace: u.cfg.MobileBuild.DesktopWorkspace,
	})
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

// ---------------------------------------------------------------------------
// Desktop branding
// ---------------------------------------------------------------------------

// GetDesktopBranding returns the saved desktop branding, or the values the
// checked-in Tauri shell ships with when the admin has not saved anything yet.
func (u *usecase) GetDesktopBranding(ctx context.Context) (*domain.DesktopBranding, error) {
	b, err := u.repo.GetDesktopBranding(ctx)
	if err != nil {
		return nil, err
	}
	if b == nil {
		b = u.defaultDesktopBranding()
	}
	return b, nil
}

// defaultDesktopBranding reads the shell's tauri.conf.json so the form starts
// from the values the next build would actually use.
func (u *usecase) defaultDesktopBranding() *domain.DesktopBranding {
	b := &domain.DesktopBranding{
		ProductName:      "AuthyIDE",
		BundleIdentifier: "com.authyide.app",
	}
	data, err := os.ReadFile(filepath.Join(u.desktopWorkspace(), "tauri.conf.json"))
	if err != nil {
		u.logger.Warn("read tauri.conf.json failed", "error", err)
		return b
	}
	var doc struct {
		ProductName string `json:"productName"`
		Identifier  string `json:"identifier"`
		Plugins     struct {
			Updater struct {
				Endpoints []string `json:"endpoints"`
			} `json:"updater"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		u.logger.Warn("parse tauri.conf.json failed", "error", err)
		return b
	}
	if doc.ProductName != "" {
		b.ProductName = doc.ProductName
	}
	if doc.Identifier != "" {
		b.BundleIdentifier = doc.Identifier
	}
	if len(doc.Plugins.Updater.Endpoints) > 0 {
		b.UpdaterEndpoint = doc.Plugins.Updater.Endpoints[0]
	}
	return b
}

// SaveDesktopBranding merges the partial update and persists it once the merged
// value is a valid desktop identity. The icon key is not touched here: the
// shell icon travels through the asset endpoints.
func (u *usecase) SaveDesktopBranding(ctx context.Context, req *domain.UpdateDesktopBrandingReq) (*domain.DesktopBranding, error) {
	if req == nil {
		return nil, errcode.ErrBrandingInvalid
	}
	current, err := u.GetDesktopBranding(ctx)
	if err != nil {
		return nil, err
	}
	merged := *current
	if req.ProductName != nil {
		merged.ProductName = strings.TrimSpace(*req.ProductName)
	}
	if req.BundleIdentifier != nil {
		merged.BundleIdentifier = strings.TrimSpace(*req.BundleIdentifier)
	}
	if req.UpdaterEndpoint != nil {
		merged.UpdaterEndpoint = strings.TrimSpace(*req.UpdaterEndpoint)
	}
	if err := validateDesktopBranding(&merged); err != nil {
		return nil, errcode.ErrBrandingInvalid.Wrap(err)
	}

	// The merged value is written in full, so a first save of a single field
	// still leaves the identity usable by the worker.
	full := &domain.UpdateDesktopBrandingReq{
		ProductName:      &merged.ProductName,
		BundleIdentifier: &merged.BundleIdentifier,
		UpdaterEndpoint:  &merged.UpdaterEndpoint,
	}
	return u.repo.SaveDesktopBranding(ctx, full)
}

func validateDesktopBranding(b *domain.DesktopBranding) error {
	if b == nil {
		return errors.New("desktop branding is empty")
	}
	if strings.TrimSpace(b.ProductName) == "" || len(b.ProductName) > 100 {
		return errors.New("product_name must be 1..100 characters")
	}
	if !bundleIDRe.MatchString(b.BundleIdentifier) || len(b.BundleIdentifier) > 255 {
		return fmt.Errorf("bundle_identifier %q must be a dotted reverse domain", b.BundleIdentifier)
	}
	if b.UpdaterEndpoint != "" {
		if err := validateHTTPURL(b.UpdaterEndpoint); err != nil {
			return fmt.Errorf("updater_endpoint: %w", err)
		}
	}
	return nil
}

func (u *usecase) desktopWorkspace() string {
	if dir := strings.TrimSpace(u.cfg.MobileBuild.DesktopWorkspace); dir != "" {
		return dir
	}
	return runner.DefaultDesktopPath
}

// UploadDesktopAsset replaces the icon the desktop shell bundles. The request
// carries only the file, so the slot is fixed to the single desktop slot.
func (u *usecase) UploadDesktopAsset(ctx context.Context, data []byte) (*domain.DesktopBranding, error) {
	slot := domain.SlotDesktopIcon
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
	b, err := u.repo.SetDesktopIconKey(ctx, slot)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// ClearDesktopAsset removes the custom desktop icon so the shell falls back to
// its bundled default.
func (u *usecase) ClearDesktopAsset(ctx context.Context) (*domain.DesktopBranding, error) {
	slot := domain.SlotDesktopIcon
	if err := u.assets.Delete(ctx, slot); err != nil {
		return nil, fmt.Errorf("delete asset %s: %w", slot, err)
	}
	return u.repo.ClearDesktopIconKey(ctx)
}

// UploadAppIcon stores one app's custom icon and marks it on the apps table.
func (u *usecase) UploadAppIcon(ctx context.Context, owner, appID uuid.UUID, data []byte) error {
	if err := u.verifyOwnership(ctx, owner, appID); err != nil {
		return err
	}
	limit := u.cfg.MobileBuild.MaxAssetBytes
	if limit <= 0 {
		limit = defaultMaxAsset
	}
	if len(data) == 0 || int64(len(data)) > limit {
		return errcode.ErrAssetInvalid.Wrap(fmt.Errorf("asset must be 1..%d bytes", limit))
	}
	if !hasPNGSignature(data) {
		return errcode.ErrAssetInvalid.Wrap(errors.New("asset must be a PNG image"))
	}
	if err := u.appAssets.Put(ctx, appID, data); err != nil {
		return fmt.Errorf("store app icon: %w", err)
	}
	return u.repo.SetAppIconKey(ctx, appID, appID.String())
}

// ClearAppIcon removes one app's custom icon and falls back to the instance default.
func (u *usecase) ClearAppIcon(ctx context.Context, owner, appID uuid.UUID) error {
	if err := u.verifyOwnership(ctx, owner, appID); err != nil {
		return err
	}
	if err := u.appAssets.Delete(ctx, appID); err != nil {
		return fmt.Errorf("delete app icon: %w", err)
	}
	return u.repo.SetAppIconKey(ctx, appID, "")
}

// DownloadAppIcon resolves one app's icon for preview. It reports
// ErrBuildArtifactMissing when the app has no custom icon, so the caller falls
// back to the template's built-in default.
func (u *usecase) DownloadAppIcon(ctx context.Context, owner, appID uuid.UUID) (string, error) {
	if err := u.verifyOwnership(ctx, owner, appID); err != nil {
		return "", err
	}
	path, err := u.appAssets.Path(ctx, appID)
	if err != nil {
		if errors.Is(err, assetstore.ErrEmpty) {
			return "", errcode.ErrBuildArtifactMissing
		}
		return "", fmt.Errorf("resolve app icon: %w", err)
	}
	return path, nil
}

// defaultAppDesktopBranding derives one app's desktop identity from the app
// row. package_id is always a dotted reverse domain on mobile, so appending a
// fixed suffix keeps the Tauri identifier legal without user input.
func defaultAppDesktopBranding(app *domain.AppBranding) *domain.AppDesktopBranding {
	d := domain.DefaultDesktopBranding(app)
	return &domain.AppDesktopBranding{
		AppID:            app.ID,
		ProductName:      d.ProductName,
		BundleIdentifier: d.BundleIdentifier,
	}
}

// GetAppDesktopBranding returns one app's desktop identity with the app row as
// the fallback. Saving is what makes the identity survive a rename of the app.
func (u *usecase) GetAppDesktopBranding(ctx context.Context, owner, appID uuid.UUID) (*domain.AppDesktopBranding, error) {
	if err := u.verifyOwnership(ctx, owner, appID); err != nil {
		return nil, err
	}
	b, err := u.repo.GetAppDesktopBranding(ctx, appID)
	if err != nil {
		return nil, err
	}
	if b != nil {
		return b, nil
	}
	app, err := u.repo.GetAppBranding(ctx, appID)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, errcode.ErrAppNotFound
	}
	return defaultAppDesktopBranding(app), nil
}

// SaveAppDesktopBranding merges the partial update and writes the merged value
// in full, so a first save of one field still leaves a usable identity.
func (u *usecase) SaveAppDesktopBranding(ctx context.Context, owner, appID uuid.UUID, req *domain.UpdateAppDesktopBrandingReq) (*domain.AppDesktopBranding, error) {
	if err := u.verifyOwnership(ctx, owner, appID); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, errcode.ErrBrandingInvalid
	}
	current, err := u.GetAppDesktopBranding(ctx, owner, appID)
	if err != nil {
		return nil, err
	}
	merged := *current
	if req.ProductName != nil {
		merged.ProductName = strings.TrimSpace(*req.ProductName)
	}
	if req.BundleIdentifier != nil {
		merged.BundleIdentifier = strings.TrimSpace(*req.BundleIdentifier)
	}
	if err := validateAppDesktopBranding(&merged); err != nil {
		return nil, errcode.ErrBrandingInvalid.Wrap(err)
	}
	full := &domain.UpdateAppDesktopBrandingReq{
		ProductName:      &merged.ProductName,
		BundleIdentifier: &merged.BundleIdentifier,
	}
	return u.repo.SaveAppDesktopBranding(ctx, appID, full)
}

func validateAppDesktopBranding(b *domain.AppDesktopBranding) error {
	if b == nil {
		return errors.New("app desktop branding is empty")
	}
	if strings.TrimSpace(b.ProductName) == "" || len(b.ProductName) > 100 {
		return errors.New("product_name must be 1..100 characters")
	}
	if !bundleIDRe.MatchString(b.BundleIdentifier) || len(b.BundleIdentifier) > 255 {
		return fmt.Errorf("bundle_identifier %q must be a dotted reverse domain", b.BundleIdentifier)
	}
	return nil
}

// desktopIdentityFor resolves the desktop identity one job builds with. An app
// builds under its own identity; an admin-console build uses the instance
// singleton. The app's updater endpoint is derived from its id so each app
// publishes into its own update source.
func (u *usecase) desktopIdentityFor(ctx context.Context, owner uuid.UUID, app *domain.AppBranding) (*domain.DesktopBranding, error) {
	if app != nil {
		b, err := u.GetAppDesktopBranding(ctx, owner, app.ID)
		if err != nil {
			return nil, err
		}
		if b != nil && strings.TrimSpace(b.ProductName) != "" && strings.TrimSpace(b.BundleIdentifier) != "" {
			return &domain.DesktopBranding{
				ProductName:      b.ProductName,
				BundleIdentifier: b.BundleIdentifier,
				UpdaterEndpoint:  u.appUpdaterEndpoint(app.ID),
			}, nil
		}
	}
	b, err := u.repo.GetDesktopBranding(ctx)
	if err != nil {
		return nil, err
	}
	if b == nil || strings.TrimSpace(b.ProductName) == "" || strings.TrimSpace(b.BundleIdentifier) == "" {
		return nil, errcode.ErrBrandingInvalid.Wrap(errors.New("desktop branding is not configured"))
	}
	return b, nil
}

// Platforms lists every build target of one app with the reason each is or is
// not buildable. desktopLicensed comes from the user's entitlement, which this
// module does not read: the entitlement belongs to the app module.
func (u *usecase) Platforms(ctx context.Context, owner, appID uuid.UUID, desktopLicensed bool) ([]*domain.AppBuildPlatform, error) {
	if err := u.verifyOwnership(ctx, owner, appID); err != nil {
		return nil, err
	}
	app, err := u.repo.GetAppBranding(ctx, appID)
	if err != nil {
		return nil, err
	}
	if app == nil {
		return nil, errcode.ErrAppNotFound
	}

	out := make([]*domain.AppBuildPlatform, 0, len(domain.BuildPlatforms))
	for _, platform := range domain.BuildPlatforms {
		st := &domain.AppBuildPlatform{Platform: platform}
		switch reason := u.platformReason(platform, app, desktopLicensed); {
		case reason == "":
			st.Enabled = true
		default:
			st.Reason = reason
		}
		if platform == domain.PlatformDesktopLinux {
			st.UpdaterEndpoint = u.appUpdaterEndpoint(appID)
		}
		out = append(out, st)
	}
	return out, nil
}

// platformReason returns the human-readable blocker of one platform, empty when
// the platform is buildable. The order is the order a user should fix things in.
func (u *usecase) platformReason(platform string, app *domain.AppBranding, desktopLicensed bool) string {
	mc := u.cfg.MobileBuild
	if !mc.Enabled {
		return errcode.ErrPlatformDisabled.Error()
	}
	if !mc.PlatformEnabled(platform) {
		return errcode.ErrPlatformDisabled.Error()
	}
	if !platformEnabled(app.EnabledPlatforms, platform) {
		return errcode.ErrAppInvalidPlatform.Error()
	}
	if platform == domain.PlatformDesktopLinux && !desktopLicensed {
		return errcode.ErrAppDesktopUnlicensed.Error()
	}
	return ""
}

// verifyOwnership checks that one app belongs to one user.
func (u *usecase) verifyOwnership(ctx context.Context, owner, appID uuid.UUID) error {
	row, err := u.repo.GetAppBranding(ctx, appID)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}
	if row == nil {
		return errcode.ErrAppNotFound
	}
	if row.UserID != owner {
		return errcode.ErrAppNotFound
	}
	return nil
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
	// The request may name the app vocabulary (desktop) or the builder
	// vocabulary (desktop-linux); everything downstream speaks canonical.
	req.Platform = domain.NormalizePlatform(req.Platform)
	if req.Platform == "" {
		req.Platform = domain.PlatformAndroid
	}
	if !domain.PlatformValid(req.Platform) {
		return nil, errcode.ErrPlatformInvalid.Wrap(
			fmt.Errorf("platform %q is not buildable", req.Platform))
	}
	if !u.cfg.MobileBuild.PlatformEnabled(req.Platform) {
		return nil, errcode.ErrPlatformDisabled.Wrap(
			fmt.Errorf("platform %q is not enabled on this instance", req.Platform))
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

	if req.Platform == domain.PlatformDesktopLinux {
		if _, err := u.desktopIdentityFor(ctx, userID, app); err != nil {
			return nil, err
		}
	} else {
		current, err := u.repo.GetBranding(ctx)
		if err != nil {
			return nil, err
		}
		if current == nil {
			current = u.defaultBranding()
		}
		if strings.TrimSpace(current.Name) == "" || strings.TrimSpace(current.ServerURL) == "" {
			return nil, errcode.ErrBrandingInvalid.Wrap(errors.New("mobile branding is not configured"))
		}
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
	var err error
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

// platformEnabled reports whether platform is inside one app's target set. Both
// sides are normalized so a set stored in the app vocabulary (desktop) matches
// the builder vocabulary (desktop-linux). An empty set accepts any platform.
func platformEnabled(enabled []string, platform string) bool {
	if len(enabled) == 0 {
		return true
	}
	want := domain.NormalizePlatform(platform)
	for _, p := range enabled {
		if domain.NormalizePlatform(p) == want {
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
	// The detail view is the only place the channel set is exposed: desktop
	// jobs render one download and publish button per channel.
	artifacts, err := u.repo.ListArtifacts(ctx, id)
	if err != nil {
		return nil, err
	}
	job.Artifacts = artifacts
	return withDownloadURL(job), nil
}

// DownloadArtifact resolves the artifact of a succeeded job. The path is
// validated so the handler can stream it without a second check, and the file
// name drives the Content-Disposition header. An empty channel resolves to the
// job's primary channel.
func (u *usecase) DownloadArtifact(ctx context.Context, id uuid.UUID, channel string) (string, string, error) {
	job, err := u.repo.GetJob(ctx, id)
	if err != nil {
		return "", "", err
	}
	if job == nil {
		return "", "", errcode.ErrBuildNotFound
	}
	return u.resolveArtifact(ctx, job, channel)
}

// DownloadOwned resolves the artifact of one of the owner's jobs.
func (u *usecase) DownloadOwned(ctx context.Context, owner, id uuid.UUID, channel string) (string, string, error) {
	job, err := u.repo.GetOwnedJob(ctx, owner, id)
	if err != nil {
		return "", "", err
	}
	if job == nil {
		return "", "", errcode.ErrBuildNotFound
	}
	return u.resolveArtifact(ctx, job, channel)
}

// resolveArtifact maps one job channel to a file the handler can serve. The
// primary channel is recorded on the job row and every other channel in the
// artifact table, and all of them land in the job's own artifact directory.
func (u *usecase) resolveArtifact(ctx context.Context, job *domain.BuildJob, channel string) (string, string, error) {
	if job == nil {
		return "", "", errcode.ErrBuildNotFound
	}
	if job.Status != domain.BuildStatusSucceeded {
		return "", "", errcode.ErrBuildNotSucceeded
	}
	primary := domain.PrimaryChannelOf(job.Platform)
	channel = strings.TrimSpace(channel)
	if channel == "" {
		channel = primary
	}
	if !domain.ChannelValid(channel) {
		return "", "", errcode.ErrChannelInvalid.Wrap(
			fmt.Errorf("channel %q is not a build channel", channel))
	}

	var name string
	var path string
	if channel == primary {
		path = job.ArtifactPath
		name = filepath.Base(path)
		if name == "" {
			name = artifactName
		}
	} else {
		info, err := u.repo.GetArtifact(ctx, job.ID, channel)
		if err != nil {
			return "", "", err
		}
		if info == nil {
			return "", "", errcode.ErrBuildArtifactMissing
		}
		name = info.FileName
		path = info.Path
	}
	if path == "" || name == "" {
		return "", "", errcode.ErrBuildArtifactMissing
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Size() == 0 {
		return "", "", errcode.ErrBuildArtifactMissing
	}
	return path, name, nil
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

// Publish promotes a succeeded job to the update channel of its platform. The
// artifact copy and the manifest write are rolled back together, so a partial
// release never gets served.
func (u *usecase) Publish(ctx context.Context, id uuid.UUID, channel string) (*domain.ReleaseStatus, error) {
	job, err := u.repo.GetJob(ctx, id)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, errcode.ErrBuildNotFound
	}
	return u.publishJob(ctx, job, channel)
}

// PublishOwned publishes one of the owner's jobs to the release channel.
func (u *usecase) PublishOwned(ctx context.Context, owner, id uuid.UUID, channel string) (*domain.ReleaseStatus, error) {
	job, err := u.repo.GetOwnedJob(ctx, owner, id)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, errcode.ErrBuildNotFound
	}
	return u.publishJob(ctx, job, channel)
}

func (u *usecase) publishJob(ctx context.Context, job *domain.BuildJob, channel string) (*domain.ReleaseStatus, error) {
	if job == nil {
		return nil, errcode.ErrBuildNotFound
	}
	if job.Status != domain.BuildStatusSucceeded {
		return nil, errcode.ErrBuildNotSucceeded
	}
	if job.Platform == domain.PlatformDesktopLinux {
		return u.publishDesktopJob(ctx, job, channel)
	}
	return u.publishMobileJob(ctx, job)
}

// publishMobileJob copies the APK into the static tree and points the native
// app-version manifest at it.
func (u *usecase) publishMobileJob(ctx context.Context, job *domain.BuildJob) (*domain.ReleaseStatus, error) {
	if job.ArtifactPath == "" {
		return nil, errcode.ErrBuildArtifactMissing
	}
	if fi, err := os.Stat(job.ArtifactPath); err != nil || fi.Size() == 0 {
		return nil, errcode.ErrBuildArtifactMissing
	}

	version := job.TargetVersion
	current, err := u.readJobManifest(job)
	if err != nil {
		return nil, err
	}
	if current != nil && !versionGreater(version, current.Version) {
		return nil, errcode.ErrReleaseVersionTooOld.Wrap(
			fmt.Errorf("manifest already at %s", current.Version))
	}

	dest := u.mobileArtifactDest(job, version)
	if err := copyFile(job.ArtifactPath, dest); err != nil {
		return nil, err
	}
	rollback := func() {
		os.Remove(dest)
		os.Remove(filepath.Dir(dest))
	}

	manifest := domain.ReleaseManifest{Version: version, URL: u.mobileArtifactPublicURL(job, version)}
	if err := writeManifestAtomic(u.mobileManifestPath(job), manifest); err != nil {
		rollback()
		return nil, err
	}
	if err := u.repo.MarkJobPublished(ctx, job.ID, job.ArtifactSHA256); err != nil {
		rollback()
		return nil, err
	}

	u.logger.InfoContext(ctx, "native release published",
		"version", version, "version_code", job.VersionCode, "url", manifest.URL)
	return u.jobReleaseStatus(job, manifest.URL, job.ArtifactSHA256)
}

// publishDesktopJob promotes one desktop channel to the Tauri updater source.
// Only AppImage carries the updater protocol; deb and rpm are upgraded by the
// package manager, so they stay download-only.
func (u *usecase) publishDesktopJob(ctx context.Context, job *domain.BuildJob, channel string) (*domain.ReleaseStatus, error) {
	if !u.cfg.MobileBuild.PlatformEnabled(job.Platform) {
		return nil, errcode.ErrPlatformDisabled.Wrap(errors.New("desktop builds are disabled"))
	}
	channel = strings.TrimSpace(channel)
	if channel == "" {
		channel = domain.ChannelAppImage
	}
	if !domain.ChannelPublishable(channel) {
		return nil, errcode.ErrChannelNotPublishable.Wrap(
			fmt.Errorf("channel %q is not an update source", channel))
	}

	path, name, err := u.resolveArtifact(ctx, job, channel)
	if err != nil {
		return nil, err
	}
	version := job.TargetVersion

	current, err := u.readJobDesktopManifest(job)
	if err != nil {
		return nil, err
	}
	if current != nil && !versionGreater(version, current.Version) {
		return nil, errcode.ErrReleaseVersionTooOld.Wrap(
			fmt.Errorf("manifest already at %s", current.Version))
	}

	dest := u.desktopArtifactDest(job, version, name)
	if err := copyFile(path, dest); err != nil {
		return nil, err
	}
	rollback := func() {
		os.Remove(dest)
		os.Remove(filepath.Dir(dest))
	}

	sha := ""
	if sum, err := sha256File(path); err == nil {
		sha = sum
	}
	manifest := domain.DesktopReleaseManifest{
		Version:   version,
		PubDate:   time.Now().UTC().Format(time.RFC3339),
		URL:       u.desktopArtifactPublicURL(job, version, name),
		Signature: "",
	}
	if u.cfg.MobileBuild.UpdaterSigningKeyPath == "" {
		// An unsigned manifest is kept as a version record and a download
		// entry; the client refuses to update without a valid signature.
		u.logger.WarnContext(ctx, "desktop release published unsigned; clients will not update",
			"version", version)
	}
	if err := writeDesktopManifestAtomic(u.desktopManifestPath(job), manifest); err != nil {
		rollback()
		return nil, err
	}
	if err := u.repo.MarkJobPublished(ctx, job.ID, sha); err != nil {
		rollback()
		return nil, err
	}

	u.logger.InfoContext(ctx, "desktop release published",
		"version", version, "channel", channel, "url", manifest.URL, "unsigned", manifest.Signature == "")
	return u.jobReleaseStatus(job, manifest.URL, sha)
}

// jobReleaseStatus reports the state of one job's release train, resolving the
// artifact URL it just published rather than re-reading the manifest.
func (u *usecase) jobReleaseStatus(job *domain.BuildJob, artifactURL, sha256sum string) (*domain.ReleaseStatus, error) {
	platform := domain.PlatformAndroid
	enabled := u.Enabled() && u.cfg.MobileBuild.PlatformEnabled(platform)
	if job != nil && job.Platform == domain.PlatformDesktopLinux {
		platform = domain.PlatformDesktopLinux
	}
	return &domain.ReleaseStatus{
		Platform:        platform,
		Enabled:         enabled,
		Manifest:        &domain.ReleaseManifest{Version: job.TargetVersion, URL: artifactURL},
		ArtifactURL:     artifactURL,
		ArtifactPresent: true,
		ArtifactSHA256:  sha256sum,
	}, nil
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
	resp := &domain.ReleasesResp{Native: []*domain.ReleaseStatus{st}, Ota: ota}
	if u.cfg.MobileBuild.PlatformEnabled(domain.PlatformDesktopLinux) {
		desktop, err := u.DesktopReleaseStatus(ctx)
		if err != nil {
			return nil, err
		}
		resp.Desktop = desktop
	}
	return resp, nil
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
	st := &domain.ReleaseStatus{
		Platform: platform,
		Enabled:  u.Enabled() && u.cfg.MobileBuild.PlatformEnabled(platform),
	}
	if manifest == nil {
		return st, nil
	}
	st.Manifest = manifest
	st.ArtifactURL = manifest.URL
	present, sum := u.servedArtifactInfo(manifest.URL)
	st.ArtifactPresent = present
	st.ArtifactSHA256 = sum
	return st, nil
}

// DesktopReleaseStatus reads the Tauri updater manifest and verifies the
// artifact it points at is actually served from disk.
func (u *usecase) DesktopReleaseStatus(ctx context.Context) (*domain.DesktopReleaseStatus, error) {
	st := &domain.DesktopReleaseStatus{
		Platform: domain.PlatformDesktopLinux,
		Enabled:  u.cfg.MobileBuild.PlatformEnabled(domain.PlatformDesktopLinux),
		Unsigned: u.cfg.MobileBuild.UpdaterSigningKeyPath == "",
	}
	manifest, err := u.ReadDesktopManifest(ctx)
	if err != nil {
		return nil, err
	}
	if manifest == nil {
		return st, nil
	}
	st.Manifest = manifest
	st.ArtifactPresent, st.ArtifactSHA256 = u.servedArtifactInfo(manifest.URL)
	return st, nil
}

// servedArtifactInfo maps a public artifact URL back to disk and reports what
// the client actually downloads.
func (u *usecase) servedArtifactInfo(rawURL string) (bool, string) {
	local := u.localArtifactPath(rawURL)
	if local == "" {
		return false, ""
	}
	fi, err := os.Stat(local)
	if err != nil || fi.Size() == 0 {
		return false, ""
	}
	sum, err := sha256File(local)
	if err != nil {
		return true, ""
	}
	return true, sum
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

func readDesktopManifestFile(path string) (*domain.DesktopReleaseManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read desktop release manifest: %w", err)
	}
	var m domain.DesktopReleaseManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse desktop release manifest: %w", err)
	}
	if m.Version == "" || m.URL == "" {
		return nil, nil
	}
	return &m, nil
}

func (u *usecase) ReadOtaManifest(ctx context.Context) (*domain.ReleaseManifest, error) {
	return readManifestFile(filepath.Join(u.staticDir(), otaDir, domain.PlatformAndroid+".json"))
}

// ReadDesktopManifest returns the Tauri updater record, or nil when the
// desktop channel has never been published.
func (u *usecase) ReadDesktopManifest(ctx context.Context) (*domain.DesktopReleaseManifest, error) {
	return readDesktopManifestFile(filepath.Join(u.staticDir(), desktopReleaseDir, desktopManifestName))
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
	return u.publicBase() + prefix + domain.MobileArtifactRelPath("", version)
}

// desktopArtifactURL is the public address of one desktop artifact.
func (u *usecase) desktopArtifactURL(version, name string) string {
	prefix := strings.TrimRight(strings.TrimSpace(u.cfg.StaticFiles.RoutePrefix), "/")
	return u.publicBase() + prefix + domain.DesktopArtifactRelPath("", version, name)
}

// appUpdaterEndpoint is the URL the desktop client of one app polls. It is
// derived from the app id rather than stored, so an app can never be pointed
// at another app's update source.
func (u *usecase) appUpdaterEndpoint(appID uuid.UUID) string {
	return domain.DesktopUpdaterEndpoint(u.publicBase(), u.cfg.StaticFiles.RoutePrefix, appID.String())
}

// mobileManifestPath is the on-disk native update manifest of one job.
func (u *usecase) mobileManifestPath(job *domain.BuildJob) string {
	appID := appIDString(job)
	return filepath.Join(u.staticDir(), domain.MobileManifestRelPath(appID, domain.PlatformAndroid))
}

// desktopManifestPath is the on-disk Tauri updater manifest of one job.
func (u *usecase) desktopManifestPath(job *domain.BuildJob) string {
	return filepath.Join(u.staticDir(), domain.DesktopManifestRelPath(appIDString(job)))
}

// mobileArtifactDest is the on-disk destination of one job's APK.
func (u *usecase) mobileArtifactDest(job *domain.BuildJob, version string) string {
	return filepath.Join(u.staticDir(), domain.MobileArtifactRelPath(appIDString(job), version))
}

// desktopArtifactDest is the on-disk destination of one job's desktop package.
func (u *usecase) desktopArtifactDest(job *domain.BuildJob, version, name string) string {
	return filepath.Join(u.staticDir(), domain.DesktopArtifactRelPath(appIDString(job), version, name))
}

// mobileArtifactPublicURL is the download address of one job's APK.
func (u *usecase) mobileArtifactPublicURL(job *domain.BuildJob, version string) string {
	return u.publicBase() + u.cfg.StaticFiles.RoutePrefix +
		domain.MobileArtifactRelPath(appIDString(job), version)
}

// desktopArtifactPublicURL is the download address of one job's desktop
// package.
func (u *usecase) desktopArtifactPublicURL(job *domain.BuildJob, version, name string) string {
	return u.publicBase() + u.cfg.StaticFiles.RoutePrefix +
		domain.DesktopArtifactRelPath(appIDString(job), version, name)
}

// readJobManifest reads the native update manifest of one job's release train.
func (u *usecase) readJobManifest(job *domain.BuildJob) (*domain.ReleaseManifest, error) {
	return readManifestFile(u.mobileManifestPath(job))
}

// readJobDesktopManifest reads the Tauri updater manifest of one job's release
// train. It reports nil when no manifest exists yet.
func (u *usecase) readJobDesktopManifest(job *domain.BuildJob) (*domain.DesktopReleaseManifest, error) {
	return readDesktopManifestFile(u.desktopManifestPath(job))
}

// appIDString is the release-train key of one job; empty for admin-console
// builds, which publish into the instance train.
func appIDString(job *domain.BuildJob) string {
	if job == nil || job.AppID == uuid.Nil {
		return ""
	}
	return job.AppID.String()
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

// writeJSONAtomic writes to a temp file and renames, so a reader never
// observes a half-written manifest.
func writeJSONAtomic(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create manifest dir: %w", err)
	}
	data, err := json.Marshal(v)
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

func writeManifestAtomic(path string, manifest domain.ReleaseManifest) error {
	return writeJSONAtomic(path, manifest)
}

func writeDesktopManifestAtomic(path string, manifest domain.DesktopReleaseManifest) error {
	return writeJSONAtomic(path, manifest)
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
