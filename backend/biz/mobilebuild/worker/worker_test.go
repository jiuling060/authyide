package worker

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/chaitin/MonkeyCode/backend/config"
)

func execCommand(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// withRunCopy swaps runCopy for the duration of one test and restores it.
func withRunCopy(t *testing.T, fn func(args ...string) ([]byte, error)) {
	t.Helper()
	prev := runCopy
	runCopy = fn
	t.Cleanup(func() { runCopy = prev })
}

func writeTemplate(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, ".npmrc"), []byte("registry=https://registry.npmjs.org/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(src, "android", "app")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inner, "build.gradle"), []byte("plugins {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return src
}

func TestCopyTemplateUsesHardLinksWhenAllowed(t *testing.T) {
	src := writeTemplate(t)
	dst := filepath.Join(t.TempDir(), "workspace")

	var calls [][]string
	withRunCopy(t, func(args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		return execCommand("cp", args...)
	})

	if _, err := copyTemplate(src, dst); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 {
		t.Fatalf("a working hard-link copy should not be retried, got %d attempts: %v", len(calls), calls)
	}
	if calls[0][0] != "-al" {
		t.Fatalf("first attempt should use hard links, got %v", calls[0])
	}

	got, err := os.ReadFile(filepath.Join(dst, "android", "app", "build.gradle"))
	if err != nil {
		t.Fatalf("copied workspace is incomplete: %v", err)
	}
	if string(got) != "plugins {}" {
		t.Fatalf("unexpected build.gradle contents: %q", got)
	}
	if _, err := os.Stat(filepath.Join(src, "scratch")); !os.IsNotExist(err) {
		t.Fatal("mutating a job workspace must not touch the template")
	}
}

func TestCopyTemplateFallsBackWhenHardLinksAreRefused(t *testing.T) {
	src := writeTemplate(t)
	dst := filepath.Join(t.TempDir(), "workspace")

	var calls [][]string
	withRunCopy(t, func(args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		if len(args) > 0 && args[0] == "-al" {
			// Leave a partial tree behind, as a real refused cp -al would.
			partial := filepath.Join(dst, ".npmrc")
			if err := os.MkdirAll(dst, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(partial, []byte("stale"), 0o644); err != nil {
				t.Fatal(err)
			}
			return []byte("Invalid cross-device link"), errors.New("exit status 1")
		}
		return execCommand("cp", args...)
	})

	if _, err := copyTemplate(src, dst); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("a refused hard-link copy should fall back exactly once, got %d attempts: %v", len(calls), calls)
	}
	if calls[0][0] != "-al" || calls[1][0] != "-a" {
		t.Fatalf("expected a hard-link attempt followed by a real copy, got %v", calls)
	}
	// The partial tree from the refused attempt must be gone, otherwise cp -a
	// would have merged into it instead of recreating the workspace.
	got, err := os.ReadFile(filepath.Join(dst, ".npmrc"))
	if err != nil {
		t.Fatalf("retry did not produce the template: %v", err)
	}
	if string(got) != "registry=https://registry.npmjs.org/\n" {
		t.Fatalf("workspace kept a stale partial file: %q", got)
	}
	inner, err := os.ReadFile(filepath.Join(dst, "android", "app", "build.gradle"))
	if err != nil {
		t.Fatalf("retry did not copy the nested tree: %v", err)
	}
	if string(inner) != "plugins {}" {
		t.Fatalf("unexpected build.gradle contents: %q", inner)
	}
}

func TestCopyTemplateKeepsBothErrorsWhenBothAttemptsFail(t *testing.T) {
	src := writeTemplate(t)
	dst := filepath.Join(t.TempDir(), "workspace")

	withRunCopy(t, func(args ...string) ([]byte, error) {
		switch args[0] {
		case "-al":
			return []byte("first: cross-device link"), errors.New("first")
		case "-a":
			return []byte("second: no space left on device"), errors.New("second")
		default:
			return nil, nil
		}
	})

	out, err := copyTemplate(src, dst)
	if err == nil {
		t.Fatal("a failed real copy must not be reported as success")
	}
	if !strings.Contains(string(out), "cross-device link") || !strings.Contains(string(out), "no space left on device") {
		t.Fatalf("both attempts' output should be preserved for diagnosis, got %q", string(out))
	}
}

func TestWorkspaceForMobileJob(t *testing.T) {
	src := writeTemplate(t)
	assetRoot := t.TempDir()
	w := &Worker{cfg: &config.Config{MobileBuild: config.MobileBuildConfig{Workspace: src, AssetRoot: assetRoot}}}

	jobID := uuid.New()
	work, cleanup, err := w.workspaceFor("android", jobID)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(assetRoot, "workspaces", jobID.String()); work != want {
		t.Fatalf("workspace = %q, want %q", work, want)
	}
	if _, err := os.Stat(filepath.Join(work, ".npmrc")); err != nil {
		t.Fatalf("template file did not reach the job workspace: %v", err)
	}
	cleanup()
	if _, err := os.Stat(work); !os.IsNotExist(err) {
		t.Fatal("cleanup should remove the job workspace")
	}
}

func TestWorkspaceForMobileJobRefusesMissingTemplate(t *testing.T) {
	w := &Worker{cfg: &config.Config{MobileBuild: config.MobileBuildConfig{
		Workspace: filepath.Join(t.TempDir(), "absent"), AssetRoot: t.TempDir(),
	}}}
	if _, _, err := w.workspaceFor("android", uuid.New()); err == nil {
		t.Fatal("a missing template must fail the workspace setup")
	}
}
