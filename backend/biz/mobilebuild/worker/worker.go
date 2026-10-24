// Package worker runs queued app builds, one at a time.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/samber/do"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/assetstore"
	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/runner"
	"github.com/chaitin/MonkeyCode/backend/config"
)

const (
	orphanRecoveryDelay = 5 * time.Second
	// androidArtifactName is the fixed download name of the mobile artifact,
	// which keeps an existing install URL stable across rebuilds.
	androidArtifactName = "authyide.apk"
	// prereqProbeTime caps every prerequisite probe so a hung tool cannot
	// stall the status endpoint.
	prereqProbeTime = 3 * time.Second
)

// Worker is the single build executor. Only one job runs at a time.
type Worker struct {
	cfg       *config.Config
	repo      domain.AppBuilderRepo
	assets    assetstore.Store
	appAssets assetstore.AppStore
	objects   domain.ObjectSource
	logger    *slog.Logger
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	workerID  string
}

// NewWorker creates the build worker and starts its executor goroutine.
func NewWorker(i *do.Injector) (*Worker, error) {
	cfg := do.MustInvoke[*config.Config](i)
	repo := do.MustInvoke[domain.AppBuilderRepo](i)
	assets := do.MustInvoke[assetstore.Store](i)
	appAssets := do.MustInvoke[assetstore.AppStore](i)
	objects := do.MustInvoke[domain.ObjectSource](i)
	logger := do.MustInvoke[*slog.Logger](i)

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "worker"
	}
	w := &Worker{
		cfg:       cfg,
		repo:      repo,
		assets:    assets,
		appAssets: appAssets,
		objects:   objects,
		logger:    logger.With("module", "mobilebuild.worker"),
		workerID:  fmt.Sprintf("%s-%d", hostname, os.Getpid()),
	}
	w.Start(context.Background())
	return w, nil
}

// Start launches the executor goroutine. It is a no-op when the module is
// disabled.
func (w *Worker) Start(ctx context.Context) {
	if !w.cfg.MobileBuild.Enabled {
		w.logger.Info("mobile build worker disabled")
		return
	}
	ctx, w.cancel = context.WithCancel(ctx)
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer func() {
			if p := recover(); p != nil {
				w.logger.Error("worker panic", "panic", p)
			}
		}()
		w.recoverOrphans(ctx)
		w.loop(ctx)
	}()
	w.logger.Info("mobile build worker started",
		"workspace", w.cfg.MobileBuild.Workspace,
		"worker_id", w.workerID,
	)
}

// Close waits for the executor to finish.
func (w *Worker) Close() {
	if w.cancel == nil {
		return
	}
	w.logger.Info("mobile build worker stopping")
	w.cancel()
	w.wg.Wait()
}

// recoverOrphans marks every job that was running when the process died.
func (w *Worker) recoverOrphans(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(orphanRecoveryDelay):
	}
	if err := w.repo.FailAllRunning(ctx, "build interrupted by server restart"); err != nil {
		w.logger.WarnContext(ctx, "failed to recover orphan builds", "error", err)
	}
}

func (w *Worker) loop(ctx context.Context) error {
	interval := w.cfg.MobileBuild.PollIntervalDuration()
	for {
		if err := w.poll(ctx); err != nil {
			w.logger.WarnContext(ctx, "build poll failed", "error", err)
		}
		if err := interruptibleSleep(ctx, interval); err != nil {
			return err
		}
	}
}

func (w *Worker) poll(ctx context.Context) error {
	running, err := w.repo.AnyRunning(ctx)
	if err != nil {
		return err
	}
	if running {
		return nil
	}
	job, err := w.repo.PickOldestQueued(ctx)
	if err != nil {
		return err
	}
	if job == nil {
		return nil
	}
	claimed, err := w.repo.ClaimJob(ctx, job.ID, w.workerID)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	return w.execute(ctx, job)
}

func (w *Worker) execute(ctx context.Context, job *domain.BuildJob) error {
	begin := time.Now()
	mc := w.cfg.MobileBuild

	// The three refusals below are permanent: the platform is unknown, it is
	// switched off on this instance, or its toolchain is missing. Retrying them
	// would only burn the single build slot again, so they fail the job.
	if !domain.PlatformValid(job.Platform) {
		return w.fail(ctx, job.ID, begin, nil, nil,
			fmt.Errorf("platform %q is not buildable", job.Platform))
	}
	if !mc.PlatformEnabled(job.Platform) {
		return w.fail(ctx, job.ID, begin, nil, nil,
			fmt.Errorf("platform %q is not enabled on this instance", job.Platform))
	}
	if missing := w.missingPrereqs(ctx, job.Platform); len(missing) > 0 {
		return w.fail(ctx, job.ID, begin, nil, nil,
			fmt.Errorf("platform %q is not ready on this host: missing %s", job.Platform, strings.Join(missing, ", ")))
	}
	if err := w.checkResources(); err != nil {
		return w.fail(ctx, job.ID, begin, nil, nil, fmt.Errorf("resource check: %w", err))
	}

	logPath := filepath.Join(mc.LogDir, job.ID.String()+".log")
	if err := w.repo.SetJobLogPath(ctx, job.ID, logPath); err != nil {
		w.logger.WarnContext(ctx, "failed to record log path", "error", err)
	}

	work, cleanup, err := w.workspaceFor(job.Platform, job.ID)
	if err != nil {
		return w.fail(ctx, job.ID, begin, nil, nil, fmt.Errorf("prepare workspace: %w", err))
	}
	defer cleanup()

	source, err := w.repo.GetJobSourceSnapshot(ctx, job.ID)
	if err != nil {
		return w.fail(ctx, job.ID, begin, nil, nil, fmt.Errorf("read source snapshot: %w", err))
	}

	b, app, err := w.runnerFor(ctx, job.Platform, work, job, source)
	if err != nil {
		return w.fail(ctx, job.ID, begin, nil, nil, err)
	}

	result, runErr := b.Run(ctx, job.ID,
		func(step string) error { return w.repo.SetJobStep(ctx, job.ID, step) },
		func(tail string) error { return w.repo.SetJobLogTail(ctx, job.ID, tail) },
	)
	if runErr != nil {
		return w.fail(ctx, job.ID, begin, exitCodeOf(runErr), app, runErr)
	}
	if len(result.Artifacts) == 0 {
		return w.fail(ctx, job.ID, begin, nil, app, fmt.Errorf("build produced no artifact"))
	}

	moved, err := w.storeArtifacts(job.Platform, job.ID, result.Artifacts)
	if err != nil {
		return w.fail(ctx, job.ID, begin, nil, app, err)
	}
	primary := moved[0]
	res := domain.JobResult{
		Status:         domain.BuildStatusSucceeded,
		ExitCode:       intPtr(0),
		ArtifactPath:   primary.Path,
		ArtifactSHA256: primary.SHA256,
		ArtifactSize:   primary.Size,
		DurationMS:     time.Since(begin).Milliseconds(),
		Artifacts:      moved,
	}
	if err := w.complete(ctx, job.ID, app, res); err != nil {
		return err
	}
	w.logger.InfoContext(ctx, "build succeeded",
		"job_id", job.ID, "platform", job.Platform,
		"version", job.TargetVersion, "artifacts", len(moved))
	return nil
}

// runnerFor builds the runner of one job and returns the app branding it used.
// app is non-nil only for self-service mobile builds.
func (w *Worker) runnerFor(ctx context.Context, platform, work string, job *domain.BuildJob, source *domain.SourceSnapshot) (runner.BuildRunner, *domain.AppBranding, error) {
	if platform == domain.PlatformDesktopLinux {
		// The app id comes from the job so an app builds under its own identity
		// while an admin-console build uses the instance singleton.
		var app *domain.AppBranding
		if job.AppID != uuid.Nil {
			app, err := w.repo.GetAppBranding(ctx, job.AppID)
			if err != nil {
				return nil, nil, fmt.Errorf("read app branding: %w", err)
			}
			if app == nil {
				return nil, nil, fmt.Errorf("app %s does not exist", job.AppID.String())
			}
		}
		dbg, err := w.desktopIdentity(ctx, job, app)
		if err != nil {
			return nil, nil, err
		}
		return runner.NewDesktopRunner(w.desktopConfig(ctx, work, job, dbg, app, source), w.objects, w.logger), nil, nil
	}

	branding, err := w.repo.GetBranding(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("read branding: %w", err)
	}
	if branding == nil || branding.Name == "" || branding.ServerURL == "" {
		return nil, nil, fmt.Errorf("mobile app branding is not configured")
	}
	var app *domain.AppBranding
	if job.AppID != uuid.Nil {
		app, err = w.repo.GetAppBranding(ctx, job.AppID)
		if err != nil {
			return nil, nil, fmt.Errorf("read app branding: %w", err)
		}
		if app == nil {
			return nil, nil, fmt.Errorf("app %s does not exist", job.AppID.String())
		}
	}
	return runner.NewAndroidRunner(
		w.androidConfig(work, job, branding, app, source), w.assets, w.appAssets, w.objects, w.logger), app, nil
}

func (w *Worker) androidConfig(workspace string, job *domain.BuildJob, branding *domain.Branding, app *domain.AppBranding, source *domain.SourceSnapshot) runner.AndroidConfig {
	mc := w.cfg.MobileBuild
	cfg := runner.AndroidConfig{
		SharedConfig: runner.SharedConfig{
			BuildTimeout: mc.GradleTimeoutDuration(),
			LogDir:       mc.LogDir,
			LogTail:      mc.LogTailRefreshDuration(),
			TailBytes:    mc.LogTailBytes,
		},
		Workspace:      workspace,
		AndroidSDK:     mc.AndroidSDK,
		JavaHome:       mc.JavaHome,
		GradleTime:     mc.GradleTimeoutDuration(),
		AppName:        branding.Name,
		Version:        job.TargetVersion,
		VersionCode:    job.VersionCode,
		ServerURL:      branding.ServerURL,
		UpdatesURL:     branding.UpdatesServer,
		SourceSnapshot: source,
	}
	if app != nil {
		if app.Name != "" {
			cfg.AppName = app.Name
		}
		cfg.PackageID = app.PackageID
		cfg.AdNetwork = app.AdNetwork
		cfg.AdAppID = app.AdAppID
		cfg.AppID = app.ID
		cfg.IconKey = app.IconKey
	}
	return cfg
}

func (w *Worker) desktopConfig(ctx context.Context, workspace string, job *domain.BuildJob, branding *domain.DesktopBranding, app *domain.AppBranding, source *domain.SourceSnapshot) runner.DesktopConfig {
	mc := w.cfg.MobileBuild
	return runner.DesktopConfig{
		SharedConfig: runner.SharedConfig{
			LogDir:       mc.LogDir,
			BuildTimeout: mc.BuildTimeoutDuration(),
			CargoTimeout: mc.CargoTimeoutDuration(),
			LogTail:      mc.LogTailRefreshDuration(),
			TailBytes:    mc.LogTailBytes,
		},
		DesktopWorkspace: workspace,
		EngineSrc:        mc.EngineSrc,
		ExtensionDir:     mc.ExtensionDir,
		SkillsDir:        mc.SkillsDir,
		ProductName:      branding.ProductName,
		BundleID:         branding.BundleIdentifier,
		Version:          job.TargetVersion,
		UpdaterEndpoint:  branding.UpdaterEndpoint,
		IconPath:         w.appIconPath(ctx, app),
		SourceSnapshot:   source,
	}
}

// appIconPath is the source PNG of one app's desktop icon, or empty when the
// app has none and the shell's own icon should be bundled.
func (w *Worker) appIconPath(ctx context.Context, app *domain.AppBranding) string {
	if app == nil || app.IconKey == "" {
		return ""
	}
	p, err := w.appAssets.Path(ctx, app.ID)
	if err != nil {
		w.logger.Warn("app icon not readable", "app_id", app.ID, "error", err)
		return ""
	}
	return p
}

// desktopIdentity resolves the identity one desktop job builds with. An app
// builds under its own product name and bundle id; an admin-console build uses
// the instance singleton.
func (w *Worker) desktopIdentity(ctx context.Context, job *domain.BuildJob, app *domain.AppBranding) (*domain.DesktopBranding, error) {
	if app != nil {
		b, err := w.repo.GetAppDesktopBranding(ctx, app.ID)
		if err != nil {
			return nil, fmt.Errorf("read app desktop branding: %w", err)
		}
		endpoint := domain.DesktopUpdaterEndpoint(w.cfg.Server.BaseURL, w.cfg.StaticFiles.RoutePrefix, app.ID.String())
		if b == nil {
			// The owner has never saved a desktop branding row. The app row
			// still gives enough to build with, so an unsaved app is not stuck.
			d := domain.DefaultDesktopBranding(app)
			return &domain.DesktopBranding{
				ProductName:      d.ProductName,
				BundleIdentifier: d.BundleIdentifier,
				UpdaterEndpoint:  endpoint,
			}, nil
		}
		return &domain.DesktopBranding{
			ProductName:      b.ProductName,
			BundleIdentifier: b.BundleIdentifier,
			UpdaterEndpoint:  endpoint,
		}, nil
	}
	b, err := w.repo.GetDesktopBranding(ctx)
	if err != nil {
		return nil, fmt.Errorf("read desktop branding: %w", err)
	}
	if b == nil || strings.TrimSpace(b.ProductName) == "" || strings.TrimSpace(b.BundleIdentifier) == "" {
		return nil, fmt.Errorf("desktop branding is not configured")
	}
	return b, nil
}

// workspaceFor returns the working directory of one job. Mobile jobs get their
// own hard-linked copy of the template; the desktop shell is built in place,
// because its target/ tree takes about half an hour to recompile from scratch.
func (w *Worker) workspaceFor(platform string, jobID uuid.UUID) (string, func(), error) {
	mc := w.cfg.MobileBuild
	if platform == domain.PlatformDesktopLinux {
		if mc.DesktopWorkspace == "" {
			return "", nil, fmt.Errorf("desktop_workspace is not configured")
		}
		return mc.DesktopWorkspace, func() {}, nil
	}
	dst := filepath.Join(mc.AssetRoot, "workspaces", jobID.String())
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", nil, err
	}
	out, err := copyTemplate(mc.Workspace, dst)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %s", err, out)
	}
	return dst, func() { os.RemoveAll(dst) }, nil
}

// runCopy shells out to cp. A test can swap it to make the hard-link attempt
// fail without needing real mount points.
var runCopy = func(args ...string) ([]byte, error) {
	return exec.Command("cp", args...).CombinedOutput()
}

// copyTemplate clones the build template into dst. Hard links are the cheap
// path: no bytes move and the build does not need to re-read the template.
// But cp exits EXDEV across a mount boundary, and that boundary is ordinary
// here - the template lives on the source volume while asset root is its own
// mount. Device numbers cannot be trusted to predict it either: two overlay
// mounts over the same backing storage report the same st_dev while still
// refusing to link between layers. So the copy is attempted with hard links
// and redone as a real copy if cp refuses, which is what matters.
func copyTemplate(src, dst string) ([]byte, error) {
	out, err := runCopy("-al", src, dst)
	if err == nil {
		return out, nil
	}
	// A refused cp -al leaves a partial tree of links behind. It must go: cp -a
	// into an existing directory copies into it instead of creating dst.
	_ = os.RemoveAll(dst)
	out2, err2 := runCopy("-a", src, dst)
	if err2 != nil {
		return append(out, out2...), err2
	}
	return out2, nil
}

// storeArtifacts moves every produced artifact into the job's own directory.
// Every mobile job owns its workspace copy, so the files must be moved out
// before cleanup deletes it. The primary channel comes first so the job row's
// single-value artifact columns mirror the artifact the client downloads.
func (w *Worker) storeArtifacts(platform string, jobID uuid.UUID, artifacts []runner.Artifact) ([]runner.Artifact, error) {
	dstDir := filepath.Join(w.cfg.MobileBuild.AssetRoot, "artifacts", jobID.String())
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return nil, fmt.Errorf("create artifact dir: %w", err)
	}
	primary := domain.PrimaryChannelOf(platform)
	out := make([]runner.Artifact, 0, len(artifacts))
	for _, a := range artifacts {
		name := a.FileName
		if name == "" {
			name = androidArtifactName
		}
		a.FileName = name
		dst := filepath.Join(dstDir, name)
		if err := os.Rename(a.Path, dst); err != nil {
			return nil, fmt.Errorf("move artifact %s: %w", name, err)
		}
		a.Path = dst
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return channelOrder(primary, out[i].Channel) < channelOrder(primary, out[j].Channel)
	})
	return out, nil
}

// channelOrder puts the primary channel of one platform first so the job row
// and the artifact table agree on which file is the default download.
func channelOrder(primary, channel string) int {
	if primary != "" && channel == primary {
		return 0
	}
	return 1
}

// missingPrereqs reports the host prerequisites platform lacks.
func (w *Worker) missingPrereqs(ctx context.Context, platform string) []string {
	pctx, cancel := context.WithTimeout(ctx, prereqProbeTime)
	defer cancel()
	return runner.MissingPrereqs(pctx, platform, runner.PrereqProbe{
		AndroidSDK:       w.cfg.MobileBuild.AndroidSDK,
		JavaHome:         w.cfg.MobileBuild.JavaHome,
		DesktopWorkspace: w.cfg.MobileBuild.DesktopWorkspace,
	})
}

// checkResources refuses a job the host cannot hold. Disk and memory are
// checked against configured floors; a missing floor means the platform cannot
// start, because a half-built artifact is worse than a clear failure.
func (w *Worker) checkResources() error {
	mc := w.cfg.MobileBuild
	needDisk := mc.MinFreeDisk()
	if needDisk > 0 {
		root := mc.AssetRoot
		if root == "" {
			root = "/"
		}
		var st syscall.Statfs_t
		if err := syscall.Statfs(root, &st); err != nil {
			return fmt.Errorf("statfs %s: %w", root, err)
		}
		free := int64(st.Bavail) * int64(st.Bsize)
		if free < needDisk {
			return fmt.Errorf("only %d bytes free in %s, need %d", free, root, needDisk)
		}
	}
	needMem := mc.MinFreeMem()
	if needMem > 0 {
		free, ok := memAvailableBytes()
		if !ok {
			return fmt.Errorf("could not read available memory")
		}
		if free < needMem {
			return fmt.Errorf("only %d bytes available memory, need %d", free, needMem)
		}
	}
	return nil
}

// memAvailableBytes reads MemAvailable from /proc/meminfo.
func memAvailableBytes() (int64, bool) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "MemAvailable:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, false
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			return 0, false
		}
		return kb * 1024, true
	}
	return 0, false
}

func (w *Worker) complete(ctx context.Context, id uuid.UUID, app *domain.AppBranding, res domain.JobResult) error {
	if app == nil {
		return w.repo.FinishJob(ctx, id, res)
	}
	return w.repo.CompleteForApp(ctx, id, app.ID, res)
}

func (w *Worker) fail(ctx context.Context, id uuid.UUID, begin time.Time, code *int, app *domain.AppBranding, cause error) error {
	res := domain.JobResult{
		Status:     domain.BuildStatusFailed,
		ExitCode:   code,
		Error:      cause.Error(),
		DurationMS: time.Since(begin).Milliseconds(),
	}
	if err := w.complete(ctx, id, app, res); err != nil {
		return fmt.Errorf("record build failure: %w", err)
	}
	w.logger.WarnContext(ctx, "build failed", "job_id", id, "error", cause)
	return nil
}

func exitCodeOf(err error) *int {
	var exitErr *runner.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.Code
		return &code
	}
	return nil
}

func intPtr(v int) *int {
	return &v
}

func interruptibleSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		d = time.Second
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}
