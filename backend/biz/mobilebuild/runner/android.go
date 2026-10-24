// Package runner executes the app build pipelines on this host.
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/assetstore"
	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
)

// overlaySource downloads the bound session snapshot and overlays its files
// onto the workspace copy. A download failure, size or digest mismatch, illegal
// entry path or entry-limit breach fails the job before any file is written.
func (r *androidRunner) overlaySource(ctx context.Context, log io.Writer) error {
	_, err := r.overlay().Apply(ctx, log)
	return err
}

// overlay builds the session snapshot overlay for this job's workspace copy.
// The mobile workspace is deleted with the job, so the created paths are not
// rolled back.
func (r *androidRunner) overlay() *sourceOverlay {
	return newSourceOverlay(r.objects, r.cfg.Workspace, r.cfg.SourceSnapshot, r.cfg.LogDir)
}

// extractSnapshot validates archive and only then unpacks it onto the workspace
// copy, so a bad entry aborts the job before the copy is touched at all.
// Nothing is ever deleted, so files and directories that only the template has
// survive the overlay.
func (r *androidRunner) extractSnapshot(archive string) (int, error) {
	_, n, _, err := r.overlay().extract(archive)
	return n, err
}

func (r *androidRunner) injectAssets(ctx context.Context, log io.Writer) error {
	assetsDir := filepath.Join(r.cfg.Workspace, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return fmt.Errorf("create assets dir: %w", err)
	}
	changed := 0
	for _, slot := range domain.AssetSlots {
		data, err := r.assets.Read(ctx, slot)
		if err != nil {
			if errors.Is(err, assetstore.ErrEmpty) {
				continue
			}
			return fmt.Errorf("read asset %s: %w", slot, err)
		}
		target := filepath.Join(assetsDir, slot+".png")
		if err = atomicWriteFile(target, data, 0o644); err != nil {
			return fmt.Errorf("write asset %s: %w", slot, err)
		}
		changed++
		if _, err = fmt.Fprintf(log, "[inject-assets] %s <- %d bytes\n", target, len(data)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(log, "[inject-assets] %d custom asset(s) injected\n", changed); err != nil {
		return err
	}
	if err := r.injectAppIcon(ctx, log); err != nil {
		return err
	}
	return nil
}

// injectAppIcon overlays the per-app icon on top of the instance assets. When
// the app has no icon of its own the instance asset (or the template's built-in
// default) is left untouched.
func (r *androidRunner) injectAppIcon(ctx context.Context, log io.Writer) error {
	if r.appAssets == nil || r.cfg.AppID == uuid.Nil || r.cfg.IconKey == "" {
		return nil
	}
	data, err := r.appAssets.Read(ctx, r.cfg.AppID)
	if err != nil {
		if errors.Is(err, assetstore.ErrEmpty) {
			return nil
		}
		return fmt.Errorf("read app icon: %w", err)
	}
	target := filepath.Join(r.cfg.Workspace, "assets", domain.SlotIcon+".png")
	if err = atomicWriteFile(target, data, 0o644); err != nil {
		return fmt.Errorf("write app icon: %w", err)
	}
	if _, err = fmt.Fprintf(log, "[inject-assets] app %s icon overwritten, %d bytes\n",
		r.cfg.AppID.String(), len(data)); err != nil {
		return err
	}
	return nil
}

// rewriteConfig patches app.json in place. Unknown keys are preserved because
// the file round-trips through map[string]any.
func (r *androidRunner) rewriteConfig(log io.Writer) error {
	appJSON := filepath.Join(r.cfg.Workspace, "app.json")
	data, err := os.ReadFile(appJSON)
	if err != nil {
		return fmt.Errorf("read app.json: %w", err)
	}
	var doc map[string]any
	if err = json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("parse app.json: %w", err)
	}
	expo, ok := doc["expo"].(map[string]any)
	if !ok {
		return fmt.Errorf("app.json: missing expo object")
	}
	setStr(expo, "name", r.cfg.AppName)
	setStr(expo, "slug", r.cfg.PackageID)
	setStr(expo, "version", r.cfg.Version)

	android, ok := expo["android"].(map[string]any)
	if !ok {
		android = map[string]any{}
		expo["android"] = android
	}
	android["versionCode"] = r.cfg.VersionCode
	setStr(android, "package", r.cfg.PackageID)

	extra, ok := expo["extra"].(map[string]any)
	if !ok {
		extra = map[string]any{}
		expo["extra"] = extra
	}
	setStr(extra, "baseUrl", r.cfg.ServerURL)
	setStr(extra, "updatesServer", r.cfg.UpdatesURL)
	setStr(extra, "adNetwork", r.cfg.AdNetwork)
	setStr(extra, "adAppID", r.cfg.AdAppID)

	// JS over-the-air delivery is deliberately left off: this deployment has
	// neither HTTPS nor signed update manifests, and a configured updates.url
	// would make expo-updates fail its launch fetch. The native manifest check
	// reads extra.updatesServer directly, so it keeps working untouched.
	updates, ok := expo["updates"].(map[string]any)
	if !ok {
		updates = map[string]any{}
		expo["updates"] = updates
	}
	updates["enabled"] = false
	delete(updates, "url")

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("encode app.json: %w", err)
	}
	out = append(out, '\n')
	if err = atomicWriteFile(appJSON, out, 0o644); err != nil {
		return fmt.Errorf("write app.json: %w", err)
	}
	if _, err = fmt.Fprintf(log, "[rewrite-config] name=%s slug=%s version=%s versionCode=%d baseUrl=%s updatesURL=%s adNetwork=%s\n",
		r.cfg.AppName, r.cfg.PackageID, r.cfg.Version, r.cfg.VersionCode, r.cfg.ServerURL, r.cfg.UpdatesURL, r.cfg.AdNetwork); err != nil {
		return err
	}
	return nil
}

func setStr(m map[string]any, key, value string) {
	if value != "" {
		m[key] = value
	}
}
func (r *androidRunner) prebuild(ctx context.Context, log io.Writer) error {
	bin, prefix := r.expoBin()
	args := append(prefix, "prebuild", "--platform", "android", "--no-install")
	if err := runCommand(ctx, r.cfg.Workspace, r.androidEnv(), bin, args, log); err != nil {
		return fmt.Errorf("expo prebuild: %w", err)
	}
	if err := r.writeLocalProperties(log); err != nil {
		return err
	}
	if err := r.tuneGradleProperties(log); err != nil {
		return err
	}
	return nil
}

// writeLocalProperties pins the SDK location after prebuild regenerates it.
func (r *androidRunner) writeLocalProperties(log io.Writer) error {
	p := filepath.Join(r.cfg.Workspace, "android", "local.properties")
	if err := atomicWriteFile(p, []byte("sdk.dir="+r.cfg.AndroidSDK+"\n"), 0o644); err != nil {
		return fmt.Errorf("write local.properties: %w", err)
	}
	if _, err := fmt.Fprintf(log, "[prebuild] %s\n", p); err != nil {
		return err
	}
	return nil
}

// tuneGradleProperties lowers gradle's memory footprint after prebuild resets
// the file, and narrows the native build to arm64-v8a.
func (r *androidRunner) tuneGradleProperties(log io.Writer) error {
	p := filepath.Join(r.cfg.Workspace, "android", "gradle.properties")
	data, err := os.ReadFile(p)
	if err != nil {
		return fmt.Errorf("read gradle.properties: %w", err)
	}
	tune := []string{
		"reactNativeArchitectures=arm64-v8a",
		"org.gradle.parallel=false",
		"org.gradle.workers.max=2",
		"kotlin.daemon.jvmargs=-Xmx1536m -XX:MaxMetaspaceSize=384m",
	}
	out := strings.TrimRight(string(data), "\n")
	for _, line := range tune {
		key := strings.SplitN(line, "=", 2)[0]
		out = removeProperty(out, key)
		out = out + "\n" + line
	}
	if err = atomicWriteFile(p, []byte(out+"\n"), 0o644); err != nil {
		return fmt.Errorf("write gradle.properties: %w", err)
	}
	if _, err = fmt.Fprintf(log, "[prebuild] tuned %s\n", p); err != nil {
		return err
	}
	return nil
}

func removeProperty(content, key string) string {
	var kept []string
	for _, line := range strings.Split(content, "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, key+"=") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimRight(strings.Join(kept, "\n"), "\n")
}

func (r *androidRunner) assemble(ctx context.Context, log io.Writer) error {
	buildCtx, cancel := context.WithTimeout(ctx, r.cfg.GradleTime)
	defer cancel()
	androidDir := filepath.Join(r.cfg.Workspace, "android")
	err := runCommand(buildCtx, androidDir, r.androidEnv(), "./gradlew",
		[]string{"assembleRelease", "--no-daemon"}, log)
	if err != nil {
		if errors.Is(buildCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("gradle exceeded %s", r.cfg.GradleTime)
		}
		return fmt.Errorf("gradle assembleRelease: %w", err)
	}
	return nil
}

func (r *androidRunner) artifactPath() string {
	return filepath.Join(
		r.cfg.Workspace, "android", "app", "build", "outputs", "apk", "release", "app-release.apk")
}

func (r *androidRunner) verify(log io.Writer) error {
	res, err := r.collectArtifact()
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(log, "[verify] %s sha256=%s size=%d\n", res.Path, res.SHA256, res.Size); err != nil {
		return err
	}
	return nil
}

func (r *androidRunner) collectArtifact() (*Artifact, error) {
	p := r.artifactPath()
	fi, err := os.Stat(p)
	if err != nil {
		return nil, fmt.Errorf("artifact missing: %w", err)
	}
	if fi.Size() == 0 {
		return nil, fmt.Errorf("artifact %s is empty", p)
	}
	sum, err := fileSHA256(p)
	if err != nil {
		return nil, fmt.Errorf("hash artifact: %w", err)
	}
	return &Artifact{
		Channel:  domain.ChannelAPK,
		FileName: filepath.Base(p),
		Path:     p,
		SHA256:   sum,
		Size:     fi.Size(),
	}, nil
}

// AndroidConfig is the mobile (Expo/Android) pipeline's configuration.
type AndroidConfig struct {
	SharedConfig
	Workspace  string
	AndroidSDK string
	JavaHome   string
	// GradleTime caps assembleRelease. Kept as its own knob so the long
	// mobile build is not bound to the shorter desktop step timeout.
	GradleTime time.Duration
	AppName    string
	// PackageID is the app's bundle identifier; empty for admin-console builds,
	// which keep the workspace template's own slug.
	PackageID   string
	Version     string
	VersionCode int
	ServerURL   string
	UpdatesURL  string
	AdNetwork   string
	AdAppID     string
	// AppID selects the per-app icon; uuid.Nil for admin-console builds, which
	// use the instance branding assets only.
	AppID uuid.UUID
	// IconKey marks the app as having a custom icon; empty skips the store read.
	IconKey string
	// SourceSnapshot is the session source archive to overlay onto the
	// workspace copy before assets are injected. Nil builds the template source.
	SourceSnapshot *domain.SourceSnapshot
}

// AndroidRunner runs one mobile pipeline. A runner is created per job.
type androidRunner struct {
	baseRunner
	cfg       AndroidConfig
	assets    assetstore.Store
	appAssets assetstore.AppStore
	objects   domain.ObjectSource
}

// NewAndroidRunner creates a mobile pipeline runner. appAssets may be nil when
// no per-app icon is in play; objects may be nil when no session snapshot is
// overlaid.
func NewAndroidRunner(cfg AndroidConfig, assets assetstore.Store, appAssets assetstore.AppStore, objects domain.ObjectSource, logger *slog.Logger) *androidRunner {
	if cfg.TailBytes <= 0 {
		cfg.TailBytes = DefaultTailBytes
	}
	if cfg.LogTail <= 0 {
		cfg.LogTail = DefaultLogTail
	}
	if cfg.BuildTimeout <= 0 {
		cfg.BuildTimeout = DefaultBuildTime
	}
	if cfg.GradleTime <= 0 {
		cfg.GradleTime = cfg.BuildTimeout
	}
	return &androidRunner{
		baseRunner: baseRunnerOf(cfg.SharedConfig, logger),
		cfg:        cfg,
		assets:     assets,
		appAssets:  appAssets,
		objects:    objects,
	}
}

func (r *androidRunner) Platform() string {
	return domain.PlatformAndroid
}

// Precheck reports host prerequisites the mobile pipeline cannot run without.
func (r *androidRunner) Precheck(ctx context.Context) []string {
	missing := make([]string, 0, 2)
	for _, label := range []struct {
		label string
		path  string
	}{{"android-sdk", r.cfg.AndroidSDK}, {"java-home", r.cfg.JavaHome}} {
		fi, err := os.Stat(label.path)
		if err != nil || !fi.IsDir() {
			missing = append(missing, label.label)
		}
	}
	return missing
}

// Compatibility shims: the pipeline was single-platform before the runner split,
// so existing callers and tests keep their mobile-only names.
type (
	Runner = androidRunner
	Config = AndroidConfig
)

// NewRunner keeps the pre-split constructor name for the mobile pipeline.
func NewRunner(cfg AndroidConfig, assets assetstore.Store, appAssets assetstore.AppStore, objects domain.ObjectSource, logger *slog.Logger) *androidRunner {
	return NewAndroidRunner(cfg, assets, appAssets, objects, logger)
}

// checkWorkspace verifies the mobile template and the toolchain are in place
// before a job is claimed as running.
func (r *androidRunner) checkWorkspace() error {
	gradlew := filepath.Join(r.cfg.Workspace, "android", "gradlew")
	if fi, err := os.Stat(gradlew); err != nil {
		return fmt.Errorf("workspace %s: missing android/gradlew", r.cfg.Workspace)
	} else if !fi.Mode().IsRegular() {
		return fmt.Errorf("workspace %s: android/gradlew is not a file", r.cfg.Workspace)
	}
	if _, err := os.Stat(filepath.Join(r.cfg.Workspace, "app.json")); err != nil {
		return fmt.Errorf("workspace %s: missing app.json", r.cfg.Workspace)
	}
	if fi, err := os.Stat(r.cfg.AndroidSDK); err != nil {
		return fmt.Errorf("android sdk %s: %w", r.cfg.AndroidSDK, err)
	} else if !fi.IsDir() {
		return fmt.Errorf("android sdk %s: not a directory", r.cfg.AndroidSDK)
	}
	if fi, err := os.Stat(r.cfg.JavaHome); err != nil {
		return fmt.Errorf("java home %s: %w", r.cfg.JavaHome, err)
	} else if !fi.IsDir() {
		return fmt.Errorf("java home %s: not a directory", r.cfg.JavaHome)
	}
	return nil
}

// expoBin prefers the workspace's own expo binary so the build does not depend
// on a global install.
func (r *androidRunner) expoBin() (string, []string) {
	local := filepath.Join(r.cfg.Workspace, "node_modules", ".bin", "expo")
	if fi, err := os.Stat(local); err == nil && fi.Mode().IsRegular() {
		return local, nil
	}
	return "npx", []string{"expo"}
}

// androidEnv is the PATH and toolchain environment every mobile step runs with.
func (r *androidRunner) androidEnv() []string {
	path := filepath.Join(r.cfg.JavaHome, "bin")
	return []string{
		"PATH=" + path + ":/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"ANDROID_HOME=" + r.cfg.AndroidSDK,
		"JAVA_HOME=" + r.cfg.JavaHome,
		"HOME=/root",
		"CI=1",
		"LANG=C.UTF-8",
		"NO_COLOR=1",
	}
}

// Run executes every mobile pipeline step. It returns one artifact.
func (r *androidRunner) Run(ctx context.Context, jobID uuid.UUID, onStep StepFunc, onTail TailFunc) (*RunResult, error) {
	started := time.Now()
	if err := r.checkWorkspace(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(r.cfg.LogDir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	logPath := filepath.Join(r.cfg.LogDir, jobID.String()+".log")
	logF, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open build log: %w", err)
	}
	defer logF.Close()
	if _, err = fmt.Fprintf(logF, "=== app build %s started %s ===\n", jobID.String(), started.UTC().Format(time.RFC3339)); err != nil {
		return nil, fmt.Errorf("write build log: %w", err)
	}

	cancelTail := r.startTailLoop(ctx, logPath, onTail)
	defer cancelTail()
	r.cleanupOlder(DefaultLogRetention, logPath)

	fail := func(step string, msg string) (*RunResult, error) {
		return nil, fmt.Errorf("%s: %s", step, msg)
	}
	// overlay-source runs only for jobs bound to a session snapshot. It is
	// placed before inject-assets on purpose: assets and app.json written by
	// later steps always win over any file of the same name carried by the
	// session, so the App record stays the single source of brand and version.
	if r.cfg.SourceSnapshot != nil {
		if err := r.step(ctx, domain.StepOverlaySource, onStep, logF, func(ctx context.Context, log io.Writer) error {
			return r.overlaySource(ctx, log)
		}); err != nil {
			return fail(domain.StepOverlaySource, err.Error())
		}
	}
	if err := r.step(ctx, domain.StepInjectAssets, onStep, logF, func(ctx context.Context, log io.Writer) error {
		return r.injectAssets(ctx, log)
	}); err != nil {
		return fail(domain.StepInjectAssets, err.Error())
	}
	if err := r.step(ctx, domain.StepRewriteConfig, onStep, logF, func(ctx context.Context, log io.Writer) error {
		return r.rewriteConfig(log)
	}); err != nil {
		return fail(domain.StepRewriteConfig, err.Error())
	}
	if err := r.step(ctx, domain.StepPrebuild, onStep, logF, func(ctx context.Context, log io.Writer) error {
		return r.prebuild(ctx, log)
	}); err != nil {
		return fail(domain.StepPrebuild, err.Error())
	}
	if err := r.step(ctx, domain.StepAssemble, onStep, logF, func(ctx context.Context, log io.Writer) error {
		return r.assemble(ctx, log)
	}); err != nil {
		return fail(domain.StepAssemble, err.Error())
	}
	if err := r.step(ctx, domain.StepVerify, onStep, logF, func(ctx context.Context, log io.Writer) error {
		return r.verify(log)
	}); err != nil {
		return fail(domain.StepVerify, err.Error())
	}

	res, err := r.collectArtifact()
	if err != nil {
		return nil, err
	}
	return &RunResult{Artifacts: []Artifact{*res}, LogPath: logPath}, nil
}
