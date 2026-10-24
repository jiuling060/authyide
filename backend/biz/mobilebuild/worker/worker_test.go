package worker

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/google/uuid"

	"github.com/chaitin/MonkeyCode/backend/config"
)

func TestStatSameDevice(t *testing.T) {
	t.Run("same device", func(t *testing.T) {
		if !statSameDevice(&syscall.Stat_t{Dev: 7}, &syscall.Stat_t{Dev: 7}) {
			t.Fatal("identical devices should compare equal")
		}
	})
	t.Run("different device", func(t *testing.T) {
		if statSameDevice(&syscall.Stat_t{Dev: 7}, &syscall.Stat_t{Dev: 8}) {
			t.Fatal("different devices should not compare equal")
		}
	})
	t.Run("unrelated types", func(t *testing.T) {
		if statSameDevice("not-a-stat", &syscall.Stat_t{Dev: 7}) {
			t.Fatal("a non-stat payload must not be treated as the same device")
		}
	})
}

func TestSameFilesystem(t *testing.T) {
	root := t.TempDir()
	one := filepath.Join(root, "a.txt")
	two := filepath.Join(root, "b.txt")
	if err := os.WriteFile(one, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(two, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	same, err := sameFilesystem(one, two)
	if err != nil {
		t.Fatal(err)
	}
	if !same {
		t.Fatal("two files inside one directory share a filesystem")
	}

	missing, err := sameFilesystem(one, filepath.Join(root, "absent"))
	if err == nil {
		t.Fatal("a missing path should surface a stat error")
	}
	if missing {
		t.Fatal("a stat error must not be reported as the same filesystem")
	}
}

func TestCopyTemplate(t *testing.T) {
	src := t.TempDir()
	dstParent := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "gradlew"), []byte("#!/bin/sh\necho build\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(src, "android", "app")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inner, "build.gradle"), []byte("plugins {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dstParent, "workspace")
	if _, err := copyTemplate(src, dst); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dst, "gradlew"))
	if err != nil {
		t.Fatalf("copied workspace is incomplete: %v", err)
	}
	if string(got) != "#!/bin/sh\necho build\n" {
		t.Fatalf("unexpected gradlew contents: %q", got)
	}
	info, err := os.Stat(filepath.Join(dst, "gradlew"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("gradlew should be a regular file, got %v", info.Mode())
	}
	if err := os.WriteFile(filepath.Join(dst, "scratch"), []byte("job scratch"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(src, "scratch")); !os.IsNotExist(err) {
		t.Fatal("mutating a job workspace must not touch the template")
	}
}

func TestWorkspaceForMobileJob(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, ".npmrc"), []byte("registry=https://registry.npmjs.org/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
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
