// Package runner executes the mobile app build pipeline on this host.
package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/assetstore"
	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
)

// Config carries the paths and timeouts the pipeline needs.
type Config struct {
	Workspace  string
	AndroidSDK string
	JavaHome   string
	GradleTime time.Duration
	LogDir     string
	LogTail    time.Duration
	TailBytes  int
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
}

// Result is the outcome of a successful pipeline run.
type Result struct {
	ArtifactPath string
	SHA256       string
	Size         int64
	LogPath      string
}

// StepFunc reports a pipeline step transition.
type StepFunc func(step string) error

// TailFunc persists a log tail.
type TailFunc func(tail string) error

// Runner runs one pipeline. A Runner is created per job.
type Runner struct {
	cfg    Config
	assets assetstore.Store
	logger *slog.Logger
}

// NewRunner creates a pipeline runner.
func NewRunner(cfg Config, assets assetstore.Store, logger *slog.Logger) *Runner {
	if cfg.TailBytes <= 0 {
		cfg.TailBytes = 8192
	}
	if cfg.LogTail <= 0 {
		cfg.LogTail = 5 * time.Second
	}
	if cfg.GradleTime <= 0 {
		cfg.GradleTime = 45 * time.Minute
	}
	return &Runner{cfg: cfg, assets: assets, logger: logger.With("module", "runner")}
}

// Run executes every pipeline step. It returns a *Result on success.
func (r *Runner) Run(ctx context.Context, jobID uuid.UUID, onStep StepFunc, onTail TailFunc) (*Result, error) {
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

	fail := func(step, msg string) (*Result, error) {
		return nil, fmt.Errorf("%s: %s", step, msg)
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
	res.LogPath = logPath
	return res, nil
}

// step runs one pipeline stage, logging its start and duration.
func (r *Runner) step(
	ctx context.Context,
	name string,
	onStep StepFunc,
	log io.Writer,
	run func(context.Context, io.Writer) error,
) error {
	if err := onStep(name); err != nil {
		return fmt.Errorf("report step: %w", err)
	}
	if _, err := fmt.Fprintf(log, "=== %s started ===\n", name); err != nil {
		return fmt.Errorf("write build log: %w", err)
	}
	begin := time.Now()
	if err := run(ctx, log); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(log, "=== %s done in %s ===\n", name, time.Since(begin).Round(time.Millisecond)); err != nil {
		return fmt.Errorf("write build log: %w", err)
	}
	r.logger.InfoContext(ctx, "pipeline step done", "step", name, "duration", time.Since(begin))
	return nil
}

func (r *Runner) checkWorkspace() error {
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

func (r *Runner) buildEnv() []string {
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

func (r *Runner) expoBin() (string, []string) {
	local := filepath.Join(r.cfg.Workspace, "node_modules", ".bin", "expo")
	if fi, err := os.Stat(local); err == nil && fi.Mode().IsRegular() {
		return local, nil
	}
	return "npx", []string{"expo"}
}

// ExitError carries a non-zero exit code from an external command.
type ExitError struct {
	Command string
	Code    int
	Cause   error
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("%s exited %d", e.Command, e.Code)
}

func (e *ExitError) Unwrap() error {
	return e.Cause
}

// runCommand runs one external command, appending its output to log. Output is
// written straight to the log file: piping through tail would buffer it.
func (r *Runner) runCommand(ctx context.Context, dir, name string, args []string, log io.Writer) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = r.buildEnv()
	cmd.Stdout = log
	cmd.Stderr = log
	err := cmd.Run()
	if err == nil {
		return nil
	}
	code := -1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	}
	return &ExitError{Command: name, Code: code, Cause: err}
}

func (r *Runner) injectAssets(ctx context.Context, log io.Writer) error {
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
	return nil
}

// rewriteConfig patches app.json in place. Unknown keys are preserved because
// the file round-trips through map[string]any.
func (r *Runner) rewriteConfig(log io.Writer) error {
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

// atomicWriteFile replaces path by renaming a sibling temp file over it. Rename
// swaps the directory entry instead of truncating the target's inode, so a
// hard-linked copy of the same file -- which is how user build workspaces
// share the template through `cp -al` -- keeps its own content untouched.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func (r *Runner) prebuild(ctx context.Context, log io.Writer) error {
	bin, prefix := r.expoBin()
	args := append(prefix, "prebuild", "--platform", "android", "--no-install")
	if err := r.runCommand(ctx, r.cfg.Workspace, bin, args, log); err != nil {
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
func (r *Runner) writeLocalProperties(log io.Writer) error {
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
func (r *Runner) tuneGradleProperties(log io.Writer) error {
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

func (r *Runner) assemble(ctx context.Context, log io.Writer) error {
	buildCtx, cancel := context.WithTimeout(ctx, r.cfg.GradleTime)
	defer cancel()
	androidDir := filepath.Join(r.cfg.Workspace, "android")
	err := r.runCommand(buildCtx, androidDir, "./gradlew",
		[]string{"assembleRelease", "--no-daemon"}, log)
	if err != nil {
		if errors.Is(buildCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("gradle exceeded %s", r.cfg.GradleTime)
		}
		return fmt.Errorf("gradle assembleRelease: %w", err)
	}
	return nil
}

func (r *Runner) artifactPath() string {
	return filepath.Join(
		r.cfg.Workspace, "android", "app", "build", "outputs", "apk", "release", "app-release.apk")
}

func (r *Runner) verify(log io.Writer) error {
	res, err := r.collectArtifact()
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(log, "[verify] %s sha256=%s size=%d\n", res.ArtifactPath, res.SHA256, res.Size); err != nil {
		return err
	}
	return nil
}

func (r *Runner) collectArtifact() (*Result, error) {
	p := r.artifactPath()
	fi, err := os.Stat(p)
	if err != nil {
		return nil, fmt.Errorf("artifact missing: %w", err)
	}
	if fi.Size() == 0 {
		return nil, fmt.Errorf("artifact %s is empty", p)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("read artifact: %w", err)
	}
	sum := sha256.Sum256(data)
	return &Result{
		ArtifactPath: p,
		SHA256:       hex.EncodeToString(sum[:]),
		Size:         fi.Size(),
	}, nil
}

func (r *Runner) startTailLoop(ctx context.Context, logPath string, onTail TailFunc) context.CancelFunc {
	tailCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(r.cfg.LogTail)
		defer ticker.Stop()
		for {
			select {
			case <-tailCtx.Done():
				return
			case <-ticker.C:
				tail, err := readTail(logPath, r.cfg.TailBytes)
				if err != nil {
					continue
				}
				if err = onTail(tail); err != nil {
					r.logger.WarnContext(ctx, "persist log tail failed", "error", err)
				}
			}
		}
	}()
	return cancel
}

func readTail(path string, n int) (string, error) {
	if n <= 0 {
		n = 8192
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) <= n {
		return string(data), nil
	}
	return string(data[len(data)-n:]), nil
}
