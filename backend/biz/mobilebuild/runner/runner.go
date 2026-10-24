// Package runner executes the app build pipelines on this host. runner.go
// holds only the layer both platforms share; the per-platform pipelines live in
// android.go and desktop.go.
package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
)

// Defaults shared by every pipeline.
const (
	DefaultBuildTime    = 45 * time.Minute
	DefaultCargoTime    = 90 * time.Minute
	DefaultLogTail      = 5 * time.Second
	DefaultTailBytes    = 8192
	DefaultLogRetention = 30 * 24 * time.Hour
)

// Default on-disk locations.
const (
	DefaultLogDir      = "/opt/mobilebuild/logs"
	DefaultWorkspace   = "/opt/mobilebuild/workspaces"
	DefaultDesktopPath = "/opt/mobilebuild/desktop"
)

// StepFunc reports a pipeline step transition.
type StepFunc func(step string) error

// TailFunc persists a log tail.
type TailFunc func(tail string) error

// Artifact is the per-channel record of one produced build file. The runner
// reuses the domain type so a job's result can flow into the repository without
// field-by-field copying.
type Artifact = domain.Artifact

// RunResult is everything a runner produced for one job.
type RunResult struct {
	Artifacts []Artifact
	LogPath   string
}

// BuildRunner is one platform's build pipeline.
type BuildRunner interface {
	// Platform returns the platform key used across the API and DB.
	Platform() string
	// Precheck lists missing host prerequisites. Empty means ready to run.
	Precheck(ctx context.Context) []string
	// Run executes the pipeline and returns every artifact it produced.
	Run(ctx context.Context, jobID uuid.UUID, onStep StepFunc, onTail TailFunc) (*RunResult, error)
}

// SharedConfig holds the knobs both platforms share.
type SharedConfig struct {
	// LogDir holds the per-job build logs.
	LogDir string
	// BuildTimeout caps short steps (npm ci, cargo check, tauri build).
	BuildTimeout time.Duration
	// CargoTimeout caps the rust-engine build, which dominates wall clock.
	CargoTimeout time.Duration
	// LogTail is how often the persisted tail is pushed up to the job row.
	LogTail time.Duration
	// TailBytes is the persisted tail length.
	TailBytes int
}

// baseRunner carries the logger and the log policy shared by every runner.
type baseRunner struct {
	logger    *slog.Logger
	logDir    string
	logTail   time.Duration
	tailBytes int
}

func baseRunnerOf(cfg SharedConfig, logger *slog.Logger) baseRunner {
	return baseRunner{
		logger:    defaultLogger(logger).With("module", "runner"),
		logDir:    cfg.LogDir,
		logTail:   cfg.LogTail,
		tailBytes: cfg.TailBytes,
	}
}

func defaultLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.New(slog.DiscardHandler)
	}
	return logger
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

// runCommand runs one external command in dir, appending its output to log.
// Output is written straight to the log file: piping through tail would buffer
// it. A timed-out step is reported as exit 124 so the job row can tell a
// timeout apart from a build failure.
func runCommand(ctx context.Context, dir string, env []string, name string, args []string, log io.Writer) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
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
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		code = 124
	}
	return &ExitError{Command: name, Code: code, Cause: err}
}

// runStep runs one external command under its own deadline.
func runStep(ctx context.Context, dir string, env []string, name string, args []string, timeout time.Duration, log io.Writer) error {
	if timeout <= 0 {
		timeout = DefaultBuildTime
	}
	stepCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return runCommand(stepCtx, dir, env, name, args, log)
}

func (r *baseRunner) step(
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

// startTailLoop keeps a goroutine pushing the tail of logPath up to the job
// row so a client polling progress still sees something while a long step runs.
func (r *baseRunner) startTailLoop(ctx context.Context, logPath string, onTail TailFunc) context.CancelFunc {
	tailCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(r.logTail)
		defer ticker.Stop()
		for {
			select {
			case <-tailCtx.Done():
				return
			case <-ticker.C:
				tail, err := readTail(logPath, r.tailBytes)
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

// fileSHA256 returns the hex digest of path.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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
	wroteOK := false
	defer func() {
		if !wroteOK {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync temp: %w", err)
	}
	if err = tmp.Chmod(perm); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if err = os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename into place: %w", err)
	}
	wroteOK = true
	return nil
}

// safeJoin joins root and name, refusing absolute paths, parent traversal and
// any result that would escape root.
func safeJoin(root, name string) (string, error) {
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("absolute path %q is not allowed", name)
	}
	cleaned := filepath.Clean(name)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes the workspace", name)
	}
	dst := filepath.Join(root, cleaned)
	rel, err := filepath.Rel(root, dst)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path %q escapes the workspace", name)
	}
	return dst, nil
}

// copyFile copies src onto dst, creating intermediate directories. Ownership of
// the destination tree is not otherwise touched, so a hard-linked workspace
// keeps its links.
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// buildDir returns dir if it is non-empty, otherwise the current working
// directory.
func buildDir(dir string) string {
	if dir == "" {
		if wd, err := os.Getwd(); err == nil {
			return wd
		}
		return "."
	}
	return dir
}

// envKV returns only the entries whose name is non-empty, so a missing override
// leaves the inherited value untouched.
func envKV(pairs ...[2]string) []string {
	out := make([]string, 0, len(pairs))
	for _, kv := range pairs {
		if kv[0] == "" {
			continue
		}
		out = append(out, kv[0]+"="+kv[1])
	}
	return out
}

// cleanupOlder deletes logs older than retention. The job's own log is skipped,
// so a running build cannot delete the file it is writing to.
func (r *baseRunner) cleanupOlder(retention time.Duration, keep string) {
	go func() {
		entries, err := os.ReadDir(r.logDir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			full := filepath.Join(r.logDir, e.Name())
			if full == keep || time.Since(info.ModTime()) <= retention {
				continue
			}
			if err := os.Remove(full); err != nil {
				r.logger.Warn("log cleanup failed", "path", full, "error", err)
			}
		}
	}()
}
