package runner

import (
	"archive/tar"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
)

func hasEntry(entries []string, name string) bool {
	for _, s := range entries {
		if s == name {
			return true
		}
	}
	return false
}

func stepNames(r *desktopRunner) []string {
	steps := r.steps()
	out := make([]string, 0, len(steps))
	for _, s := range steps {
		out = append(out, s.name)
	}
	return out
}

func desktopRunnerWith(bound bool) *desktopRunner {
	cfg := DesktopConfig{}
	if bound {
		cfg.SourceSnapshot = &domain.SourceSnapshot{ObjectKey: "snapshots/snap.tar.gz"}
	}
	return NewDesktopRunner(cfg, nil, slog.New(slog.DiscardHandler))
}

func newTestOverlay(t *testing.T, workspace string, skipRoots ...string) *sourceOverlay {
	return newSourceOverlay(nil, workspace,
		&domain.SourceSnapshot{ObjectKey: "snapshots/snap.tar.gz"}, t.TempDir(), skipRoots...)
}

// The overlay step only exists for jobs bound to a snapshot. It sits after the
// shell checkout so the reset cannot wipe it, and before bundle so the overlaid
// source is what gets compiled.
func TestDesktopStepsIncludeOverlayOnlyWhenBound(t *testing.T) {
	bound := stepNames(desktopRunnerWith(true))
	wantBound := []string{
		domain.StepUINext, domain.StepEngine, domain.StepBrowserExtension, domain.StepShell,
		domain.StepOverlaySource, domain.StepBundle, domain.StepVerify,
	}
	if !reflect.DeepEqual(bound, wantBound) {
		t.Fatalf("bound steps = %v, want %v", bound, wantBound)
	}

	unbound := stepNames(desktopRunnerWith(false))
	wantUnbound := []string{
		domain.StepUINext, domain.StepEngine, domain.StepBrowserExtension, domain.StepShell,
		domain.StepBundle, domain.StepVerify,
	}
	if !reflect.DeepEqual(unbound, wantUnbound) {
		t.Fatalf("unbound steps = %v, want %v", unbound, wantUnbound)
	}
	if hasEntry(unbound, domain.StepOverlaySource) {
		t.Fatal("an unbound job must not run the overlay")
	}
}

// A session file inside the sidecar tree is dropped instead of overwriting an
// artifact this runner staged, which would break the bundle at package time.
func TestOverlayDropsRunnerOwnedPaths(t *testing.T) {
	workspace := t.TempDir()
	writeArchiveTo(t, workspace, map[string][]byte{
		"ui-next/src/App.tsx":                             []byte("template App"),
		filepath.Join(desktopExtStaging, "extension.zip"): []byte("staged zip"),
	})

	archive := filepath.Join(t.TempDir(), "snap.tar.gz")
	writeArchive(t, archive,
		entrySpec{name: "ui-next/src/App.tsx", body: []byte("session App")},
		entrySpec{name: "ui-next/src/Session.tsx", body: []byte("session only")},
		entrySpec{name: "binaries", typeflag: tar.TypeDir},
		entrySpec{name: "binaries/browser-extension/extension.zip", body: []byte("session zip")},
	)

	overlay := newTestOverlay(t, workspace, desktopSidecarDir)
	paths, n, skipped, err := overlay.extract(archive)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if n != 2 {
		t.Errorf("overlaid entries = %d, want 2", n)
	}
	if skipped != 2 {
		t.Errorf("skipped entries = %d, want 2", skipped)
	}
	if body := string(readPath(t, filepath.Join(workspace, "ui-next/src/App.tsx"))); body != "session App" {
		t.Errorf("App.tsx = %q, want the session copy", body)
	}
	if body := string(readPath(t, filepath.Join(workspace, filepath.Join(desktopExtStaging, "extension.zip")))); body != "staged zip" {
		t.Errorf("extension zip = %q, want the staged copy", body)
	}
	// Only the file the session added is recorded: the overwritten one stays
	// managed by git, and the skipped sidecar entries never count.
	if want := []string{filepath.Join(workspace, "ui-next/src/Session.tsx")}; !reflect.DeepEqual(paths, want) {
		t.Errorf("recorded paths = %v, want %v", paths, want)
	}

	if _, err := overlay.Apply(context.Background(), io.Discard); err == nil {
		t.Fatal("Apply without an object store must fail")
	}
}

// Rollback removes only the paths the overlay created, so session-only files
// cannot leak into the next job. A file the session overwrote keeps the session
// content: deleting it would destroy the template copy, and the next job's git
// checkout restores tracked files itself.
func TestRollbackOverlayRestoresTemplate(t *testing.T) {
	workspace := t.TempDir()
	writeArchiveTo(t, workspace, map[string][]byte{
		"src/App.tsx":    []byte("template App"),
		"static/keep.js": []byte("template only"),
		"static/shared":  []byte("template copy"),
	})

	archive := filepath.Join(t.TempDir(), "snap.tar.gz")
	writeArchive(t, archive,
		entrySpec{name: "src/App.tsx", body: []byte("session App")},
		entrySpec{name: "static/shared", body: []byte("session copy")},
		entrySpec{name: "static/new.css", body: []byte(".a{}")},
		entrySpec{name: "features/", typeflag: tar.TypeDir},
		entrySpec{name: "features/Login.tsx", body: []byte("session feature")},
	)

	overlay := newTestOverlay(t, workspace)
	paths, n, _, err := overlay.extract(archive)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if n != 5 {
		t.Fatalf("entries = %d, want 5", n)
	}
	if body := string(readPath(t, filepath.Join(workspace, "static/shared"))); body != "session copy" {
		t.Fatalf("the overlay wrote nothing: static/shared = %q", body)
	}
	created := []string{
		filepath.Join(workspace, "static/new.css"),
		filepath.Join(workspace, "features"),
		filepath.Join(workspace, "features/Login.tsx"),
	}
	if !reflect.DeepEqual(paths, created) {
		t.Fatalf("recorded paths = %v, want %v", paths, created)
	}

	rollbackOverlay(paths)

	for _, p := range []string{filepath.Join(workspace, "features"), filepath.Join(workspace, "static/new.css")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("the session-only path %s survived rollback", p)
		}
	}
	if body := string(readPath(t, filepath.Join(workspace, "static/keep.js"))); body != "template only" {
		t.Errorf("keep.js = %q, want the template copy", body)
	}
	if body := string(readPath(t, filepath.Join(workspace, "src/App.tsx"))); body != "session App" {
		t.Errorf("App.tsx = %q, want the session copy left for git checkout", body)
	}
}

// A directory the template already had is not recorded as created, so rollback
// leaves it and its other files in place.
func TestRollbackOverlayKeepsTemplateDirectories(t *testing.T) {
	workspace := t.TempDir()
	writeArchiveTo(t, workspace, map[string][]byte{
		"static/keep.js": []byte("template only"),
	})
	before := listFiles(t, workspace)

	archive := filepath.Join(t.TempDir(), "snap.tar.gz")
	writeArchive(t, archive, entrySpec{name: "static/new.css", body: []byte(".a{}")})

	overlay := newTestOverlay(t, workspace)
	paths, _, _, err := overlay.extract(archive)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if want := filepath.Join(workspace, "static"); hasEntry(paths, want) {
		t.Fatalf("a directory the template already had was recorded as created: %v", paths)
	}
	if len(paths) != 1 {
		t.Fatalf("recorded paths = %v, want the single new file", paths)
	}

	rollbackOverlay(paths)
	if got := listFiles(t, workspace); !reflect.DeepEqual(before, got) {
		t.Fatalf("workspace not restored:\nbefore=%v\nafter=%v", before, got)
	}
}

func TestDepthOf(t *testing.T) {
	cases := map[string]int{
		"/tmp/ws":         2,
		"/tmp/ws/a":       3,
		"/tmp/ws/a/b.txt": 4,
	}
	for p, want := range cases {
		if got := depthOf(p); got != want {
			t.Errorf("depthOf(%q) = %d, want %d", p, got, want)
		}
	}
}

func TestSkipSummary(t *testing.T) {
	if got := skipSummary(0); got != "" {
		t.Errorf("skipSummary(0) = %q, want empty", got)
	}
	if got := skipSummary(2); !strings.Contains(got, "skipped 2") {
		t.Errorf("skipSummary(2) = %q, want it to mention the count", got)
	}
}
