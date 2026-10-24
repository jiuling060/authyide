package runner

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
)

// entrySpec describes one tar entry written by writeArchive. Typeflag zero
// means a regular file.
type entrySpec struct {
	typeflag byte
	name     string
	body     []byte
}

func writeArchive(t *testing.T, path string, specs ...entrySpec) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, s := range specs {
		hdr := &tar.Header{
			Name:     s.name,
			Mode:     0o644,
			Size:     int64(len(s.body)),
			ModTime:  time.Unix(0, 0),
			Typeflag: s.typeflag,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %s: %v", s.name, err)
		}
		if _, err := tw.Write(s.body); err != nil {
			t.Fatalf("write body %s: %v", s.name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
}

func overlayRunner(t *testing.T) (*Runner, string) {
	t.Helper()
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	return &Runner{cfg: Config{Workspace: workspace}}, workspace
}

func readPath(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return body
}

// listFiles walks root and returns every file as a path-to-body map, so a test
// can prove the workspace was left untouched.
func listFiles(t *testing.T, root string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		out[rel] = body
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}

func TestExtractSnapshotMergesOntoTemplate(t *testing.T) {
	r, ws := overlayRunner(t)
	writeArchiveTo(t, ws, map[string][]byte{
		"index.html":           []byte("template page"),
		"static/keep.js":       []byte("keep me"),
		"static/override.js":   []byte("old copy"),
		"only-in-template.txt": []byte("template only"),
	})

	archive := filepath.Join(t.TempDir(), "snap.tar.gz")
	writeArchive(t, archive,
		entrySpec{name: "index.html", body: []byte("built page")},
		entrySpec{name: "static/", typeflag: tar.TypeDir},
		entrySpec{name: "static/override.js", body: []byte("new copy")},
		entrySpec{name: "static/new.css", body: []byte(".a{}")},
		entrySpec{name: "./assets/logo.png", body: []byte("png")},
	)

	n, err := r.extractSnapshot(archive)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if n != 5 {
		t.Fatalf("entries = %d, want 5", n)
	}

	checks := map[string]string{
		"index.html":           "built page",
		"static/override.js":   "new copy",
		"static/keep.js":       "keep me",
		"static/new.css":       ".a{}",
		"assets/logo.png":      "png",
		"only-in-template.txt": "template only",
	}
	for rel, want := range checks {
		if got := string(readPath(t, filepath.Join(ws, rel))); got != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}
}

// Every rejection below must fail during validation, before a single byte lands
// on the workspace copy.
func expectRejected(t *testing.T, specs []entrySpec, want string) {
	t.Helper()
	r, ws := overlayRunner(t)
	writeArchiveTo(t, ws, map[string][]byte{
		"index.html": []byte("template page"),
	})

	before := listFiles(t, ws)
	archive := filepath.Join(t.TempDir(), "snap.tar.gz")
	writeArchive(t, archive, specs...)

	if _, err := r.extractSnapshot(archive); err == nil {
		t.Fatalf("expected an error containing %q", want)
	} else if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}

	after := listFiles(t, ws)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("workspace mutated during validation: %v -> %v", before, after)
	}
}

func TestExtractSnapshotRejectsEscapingPath(t *testing.T) {
	expectRejected(t, []entrySpec{{name: "../evil.txt", body: []byte("x")}}, "escapes the workspace")
}

func TestExtractSnapshotRejectsAbsolutePath(t *testing.T) {
	expectRejected(t, []entrySpec{{name: "/etc/passwd", body: []byte("x")}}, "absolute path")
}

func TestExtractSnapshotRejectsNestedDotDot(t *testing.T) {
	expectRejected(t, []entrySpec{{name: "a/../../evil.txt", body: []byte("x")}}, "escapes the workspace")
}

func TestExtractSnapshotRejectsSymlink(t *testing.T) {
	expectRejected(t, []entrySpec{{name: "link", typeflag: tar.TypeSymlink}}, "which is not allowed")
}

func TestExtractSnapshotRejectsHardLink(t *testing.T) {
	expectRejected(t, []entrySpec{{name: "link", typeflag: tar.TypeLink}}, "which is not allowed")
}

func TestExtractSnapshotRejectsEmptyName(t *testing.T) {
	expectRejected(t, []entrySpec{{name: "./"}}, "has no path")
}

func TestExtractSnapshotRejectsOversizedEntry(t *testing.T) {
	// The header claims more than the archive holds. archive/tar refuses to
	// write such a header, so build the block by hand.
	r, ws := overlayRunner(t)
	writeArchiveTo(t, ws, map[string][]byte{"index.html": []byte("template page")})

	archive := filepath.Join(t.TempDir(), "snap.tar.gz")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	gz := gzip.NewWriter(f)
	if _, err := gz.Write(tarBlock("big.bin", OverlayMaxBytes+1)); err != nil {
		t.Fatalf("write block: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}

	before := listFiles(t, ws)
	if _, err := r.extractSnapshot(archive); err == nil {
		t.Fatal("expected an error for an oversized entry")
	} else if !strings.Contains(err.Error(), "over the") {
		t.Fatalf("error %q does not mention the size limit", err.Error())
	}
	after := listFiles(t, ws)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("workspace mutated during validation: %v -> %v", before, after)
	}
}

// tarBlock encodes one v7 tar header with a body larger than the archive holds,
// which archive/tar will not write.
func tarBlock(name string, size int64) []byte {
	block := make([]byte, 512)
	copy(block[0:100], []byte(name))
	putOctal := func(off int, v uint64) {
		copy(block[off:off+11], []byte(fmt.Sprintf("%011o", v)))
		block[off+11] = 0
	}
	putOctal(100, 0o644)
	putOctal(108, 0)
	putOctal(116, 0)
	putOctal(124, uint64(size))
	putOctal(136, 0)
	copy(block[257:262], "ustar")
	block[263] = '0'
	copy(block[148:156], []byte("        "))
	var sum uint32
	for _, b := range block {
		sum += uint32(b)
	}
	copy(block[148:154], []byte(fmt.Sprintf("%06o", sum)))
	block[154] = 0
	block[155] = ' '
	return block
}

func TestExtractSnapshotRejectsTooManyEntries(t *testing.T) {
	r, ws := overlayRunner(t)
	archive := filepath.Join(t.TempDir(), "snap.tar.gz")

	f, err := os.Create(archive)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for i := 0; i <= OverlayMaxFiles; i++ {
		hdr := &tar.Header{
			Name:    fmt.Sprintf("f/%06d.txt", i),
			Mode:    0o644,
			Size:    0,
			ModTime: time.Unix(0, 0),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %d: %v", i, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}

	before := listFiles(t, ws)
	if _, err := r.extractSnapshot(archive); err == nil {
		t.Fatal("expected an error for an over-sized archive")
	} else if !strings.Contains(err.Error(), "over the") {
		t.Fatalf("error %q does not mention the entry limit", err.Error())
	}
	after := listFiles(t, ws)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("workspace mutated during validation: %v -> %v", before, after)
	}
}

func TestExtractSnapshotRejectsEmptyArchive(t *testing.T) {
	r, ws := overlayRunner(t)
	writeArchiveTo(t, ws, map[string][]byte{"index.html": []byte("template page")})

	archive := filepath.Join(t.TempDir(), "snap.tar.gz")
	writeArchive(t, archive)

	if _, err := r.extractSnapshot(archive); err == nil {
		t.Fatal("expected an error for an empty archive")
	} else if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("error %q does not mention empty archive", err.Error())
	}
}

func TestSafeJoin(t *testing.T) {
	root := "/tmp/ws/"
	cases := map[string]string{
		"index.html":    "/tmp/ws/index.html",
		"./a/b.txt":     "/tmp/ws/a/b.txt",
		"../x.txt":      "",
		"/etc/passwd":   "",
		"a/../../x.txt": "",
	}
	for name, want := range cases {
		got, err := safeJoin(root, name)
		if want == "" {
			if err == nil {
				t.Errorf("safeJoin(%q) accepted an escaping path: %s", name, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("safeJoin(%q) failed: %v", name, err)
			continue
		}
		if got != want {
			t.Errorf("safeJoin(%q) = %s, want %s", name, got, want)
		}
	}
}

// writeArchiveTo lays out a template copy for the overlay tests.
func writeArchiveTo(t *testing.T, root string, files map[string][]byte) {
	t.Helper()
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, rel := range keys {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", rel, err)
		}
		if err := os.WriteFile(p, files[rel], 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}

// fakeObjectSource serves one archive body for the whole test, standing in for
// the object store so overlaySource can be driven without cloud credentials.
type fakeObjectSource struct{ body []byte }

func (f *fakeObjectSource) GetObject(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(f.body)), nil
}

func overlaySourceRunner(t *testing.T, workspace, archive string, digest string) *Runner {
	t.Helper()
	logDir := filepath.Join(t.TempDir(), "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	body, err := os.ReadFile(archive)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	cfg := Config{
		SharedConfig: SharedConfig{LogDir: logDir},
		Workspace:    workspace,
		AppName:      "Session Brand",
		PackageID:    "com.session.brand",
		Version:      "2.1.0",
		VersionCode:  17,
		ServerURL:    "https://api.example.test",
		SourceSnapshot: &domain.SourceSnapshot{
			ObjectKey: "snapshots/snap.tar.gz",
			SHA256:    digest,
		},
	}
	return NewRunner(cfg, nil, nil, &fakeObjectSource{body: body}, slog.New(slog.DiscardHandler))
}

// Correctness property 1: the App record stays the authority for brand fields
// even when the session ships its own app.json. Session files that only the
// session has must still reach the build.
func TestOverlayThenRewriteKeepsAppRecordAsBrandAuthority(t *testing.T) {
	workspace := t.TempDir()
	templateAppJSON := `{"expo":{"name":"Template App","slug":"template","version":"0.0.0","android":{"package":"com.template.app"}}}`
	writeArchiveTo(t, workspace, map[string][]byte{
		"app.json":                         []byte(templateAppJSON),
		"src/App.tsx":                      []byte("template App"),
		"android/gradlew":                  []byte("#!/bin/sh\n"),
		"plugins/withAlipayLogin/index.ts": []byte("template plugin"),
	})

	sessionAppJSON := `{"expo":{"name":"Session App","slug":"session","version":"9.9.9","android":{"package":"com.session.app"}}}`
	archive := filepath.Join(t.TempDir(), "snap.tar.gz")
	writeArchive(t, archive,
		entrySpec{name: "app.json", body: []byte(sessionAppJSON)},
		entrySpec{name: "src/pages/SessionOnly.tsx", body: []byte("session page")},
	)
	digest, err := fileSHA256(archive)
	if err != nil {
		t.Fatalf("hash archive: %v", err)
	}

	r := overlaySourceRunner(t, workspace, archive, digest)
	if err := r.overlaySource(context.Background(), io.Discard); err != nil {
		t.Fatalf("overlaySource: %v", err)
	}
	if err := r.rewriteConfig(io.Discard); err != nil {
		t.Fatalf("rewriteConfig: %v", err)
	}

	if _, err := readPathErr(filepath.Join(workspace, "src/pages/SessionOnly.tsx")); err != nil {
		t.Fatalf("session-only file was not carried into the build: %v", err)
	}
	if _, err := readPathErr(filepath.Join(workspace, "plugins/withAlipayLogin/index.ts")); err != nil {
		t.Fatalf("template-only file was lost by the overlay: %v", err)
	}
	if _, err := readPathErr(filepath.Join(workspace, "android/gradlew")); err != nil {
		t.Fatalf("template build script was lost by the overlay: %v", err)
	}

	doc := map[string]any{}
	body := readPath(t, filepath.Join(workspace, "app.json"))
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("parse app.json: %v", err)
	}
	expo := doc["expo"].(map[string]any)
	if expo["name"] != "Session Brand" {
		t.Errorf("app name = %v, want the App record value", expo["name"])
	}
	if expo["slug"] != "com.session.brand" {
		t.Errorf("slug = %v, want the App record package id", expo["slug"])
	}
	if expo["version"] != "2.1.0" {
		t.Errorf("version = %v, want the App record value", expo["version"])
	}
	android := expo["android"].(map[string]any)
	if android["package"] != "com.session.brand" {
		t.Errorf("android.package = %v, want the App record package id", android["package"])
	}
	if android["versionCode"] != float64(17) {
		t.Errorf("android.versionCode = %v, want 17", android["versionCode"])
	}
	extra := expo["extra"].(map[string]any)
	if extra["baseUrl"] != "https://api.example.test" {
		t.Errorf("extra.baseUrl = %v, want the configured server url", extra["baseUrl"])
	}
}

// Correctness property 5 (fail closed): a snapshot whose digest does not match
// never touches the workspace copy.
func TestOverlaySourceFailsClosedOnDigestMismatch(t *testing.T) {
	workspace := t.TempDir()
	writeArchiveTo(t, workspace, map[string][]byte{
		"app.json":    []byte(`{"expo":{"name":"Template App","slug":"template"}}`),
		"src/App.tsx": []byte("template App"),
	})
	before := listFiles(t, workspace)

	archive := filepath.Join(t.TempDir(), "snap.tar.gz")
	writeArchive(t, archive, entrySpec{name: "src/App.tsx", body: []byte("tampered")})

	r := overlaySourceRunner(t, workspace, archive, "0000000000000000000000000000000000000000000000000000000000000000")
	err := r.overlaySource(context.Background(), io.Discard)
	if err == nil {
		t.Fatal("expected a digest mismatch error")
	} else if !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("error %q does not mention the digest mismatch", err.Error())
	}

	after := listFiles(t, workspace)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("workspace changed before the snapshot was validated:\nbefore=%v\nafter=%v", before, after)
	}
}

func readPathErr(path string) ([]byte, error) {
	return os.ReadFile(path)
}
