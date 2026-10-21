// Package worker runs queued mobile app builds, one at a time.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
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
	apkFileName         = "authyide.apk"
)

// Worker is the single build executor. Only one job runs at a time.
type Worker struct {
	cfg      *config.Config
	repo     domain.AppBuilderRepo
	assets   assetstore.Store
	logger   *slog.Logger
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	workerID string
}

// NewWorker creates the build worker and starts its executor goroutine.
func NewWorker(i *do.Injector) (*Worker, error) {
	cfg := do.MustInvoke[*config.Config](i)
	repo := do.MustInvoke[domain.AppBuilderRepo](i)
	assets := do.MustInvoke[assetstore.Store](i)
	logger := do.MustInvoke[*slog.Logger](i)

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "worker"
	}
	w := &Worker{
		cfg:      cfg,
		repo:     repo,
		assets:   assets,
		logger:   logger.With("module", "mobilebuild.worker"),
		workerID: fmt.Sprintf("%s-%d", hostname, os.Getpid()),
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

func (w *Worker) loop(ctx context.Context) {
	interval := w.cfg.MobileBuild.PollIntervalDuration()
	for {
		if err := w.poll(ctx); err != nil {
			w.logger.WarnContext(ctx, "build poll failed", "error", err)
		}
		if err := interruptibleSleep(ctx, interval); err != nil {
			return
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
	logPath := filepath.Join(w.cfg.MobileBuild.LogDir, job.ID.String()+".log")
	if err := w.repo.SetJobLogPath(ctx, job.ID, logPath); err != nil {
		w.logger.WarnContext(ctx, "failed to record log path", "error", err)
	}

	var app *domain.AppBranding
	isolated := false
	if job.AppID != uuid.Nil {
		var err error
		app, err = w.repo.GetAppBranding(ctx, job.AppID)
		if err != nil {
			return w.fail(ctx, job.ID, begin, nil, nil, fmt.Errorf("read app branding: %w", err))
		}
		if app == nil {
			return w.fail(ctx, job.ID, begin, nil, nil,
				fmt.Errorf("app %s does not exist", job.AppID.String()))
		}
		isolated = true
	}

	branding, err := w.repo.GetBranding(ctx)
	if err != nil {
		return w.fail(ctx, job.ID, begin, nil, app, fmt.Errorf("read branding: %w", err))
	}
	if branding == nil || branding.Name == "" || branding.ServerURL == "" {
		return w.fail(ctx, job.ID, begin, nil, app, fmt.Errorf("mobile app branding is not configured"))
	}

	work, cleanup, err := w.workspaceFor(job.ID, isolated)
	if err != nil {
		return w.fail(ctx, job.ID, begin, nil, app, fmt.Errorf("prepare workspace: %w", err))
	}
	defer cleanup()

	r := runner.NewRunner(w.runnerConfig(work, job, branding, app), w.assets, w.logger)
	result, runErr := r.Run(ctx, job.ID,
		func(step string) error { return w.repo.SetJobStep(ctx, job.ID, step) },
		func(tail string) error { return w.repo.SetJobLogTail(ctx, job.ID, tail) },
	)
	if runErr != nil {
		return w.fail(ctx, job.ID, begin, exitCodeOf(runErr), app, runErr)
	}

	if isolated {
		dst := filepath.Join(w.cfg.MobileBuild.AssetRoot, "artifacts", job.ID.String(), apkFileName)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return w.fail(ctx, job.ID, begin, nil, app, fmt.Errorf("create artifact dir: %w", err))
		}
		if err := os.Rename(result.ArtifactPath, dst); err != nil {
			return w.fail(ctx, job.ID, begin, nil, app, fmt.Errorf("move artifact: %w", err))
		}
		result.ArtifactPath = dst
	}

	zero := 0
	res := domain.JobResult{
		Status:         domain.BuildStatusSucceeded,
		ExitCode:       &zero,
		ArtifactPath:   result.ArtifactPath,
		ArtifactSHA256: result.SHA256,
		ArtifactSize:   result.Size,
		DurationMS:     time.Since(begin).Milliseconds(),
	}
	if err := w.complete(ctx, job.ID, app, res); err != nil {
		return err
	}
	w.logger.InfoContext(ctx, "build succeeded",
		"job_id", job.ID, "version", job.TargetVersion, "version_code", job.VersionCode)
	return nil
}

// workspaceFor returns the working directory of one job. Jobs of a user-center
// app get a hard-linked copy of the template so their generated files cannot
// leak into the next build; admin builds reuse the template because the worker
// never runs two of them at the same time.
func (w *Worker) workspaceFor(jobID uuid.UUID, isolated bool) (string, func(), error) {
	if !isolated {
		return w.cfg.MobileBuild.Workspace, func() {}, nil
	}
	dst := filepath.Join(w.cfg.MobileBuild.AssetRoot, "workspaces", jobID.String())
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", nil, err
	}
	out, err := exec.Command("cp", "-al", w.cfg.MobileBuild.Workspace, dst).CombinedOutput()
	if err != nil {
		return "", nil, fmt.Errorf("%w: %s", err, out)
	}
	return dst, func() { os.RemoveAll(dst) }, nil
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

func (w *Worker) runnerConfig(workspace string, job *domain.BuildJob, branding *domain.Branding, app *domain.AppBranding) runner.Config {
	mc := w.cfg.MobileBuild
	cfg := runner.Config{
		Workspace:   workspace,
		AndroidSDK:  mc.AndroidSDK,
		JavaHome:    mc.JavaHome,
		GradleTime:  mc.GradleTimeoutDuration(),
		LogDir:      mc.LogDir,
		LogTail:     mc.LogTailRefreshDuration(),
		TailBytes:   mc.LogTailBytes,
		AppName:     branding.Name,
		Version:     job.TargetVersion,
		VersionCode: job.VersionCode,
		ServerURL:   branding.ServerURL,
		UpdatesURL:  branding.UpdatesServer,
	}
	if app != nil {
		if app.Name != "" {
			cfg.AppName = app.Name
		}
		cfg.PackageID = app.PackageID
		cfg.AdNetwork = app.AdNetwork
		cfg.AdAppID = app.AdAppID
	}
	return cfg
}

func exitCodeOf(err error) *int {
	var exitErr *runner.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.Code
		return &code
	}
	return nil
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
