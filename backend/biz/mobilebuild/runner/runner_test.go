package runner

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// atomicWriteFile replaces the directory entry instead of truncating in place.
// That is what keeps a cp -al hard link to the original pointing at the old
// content, so verify the behaviour through the hard link rather than through
// the inode number.
func TestAtomicWriteFileKeepsHardLinkedTemplateUntouched(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "app.json")
	template := filepath.Join(dir, "template.json")

	if err := atomicWriteFile(template, []byte(`{"old":true}`), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	if err := os.Link(template, target); err != nil {
		t.Fatalf("link template: %v", err)
	}

	if err := atomicWriteFile(target, []byte(`{"new":true}`), 0o600); err != nil {
		t.Fatalf("atomic write: %v", err)
	}

	targetBody, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(targetBody) != `{"new":true}` {
		t.Fatalf("target body = %q", targetBody)
	}

	templateBody, err := os.ReadFile(template)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	if string(templateBody) != `{"old":true}` {
		t.Fatalf("template was mutated: %q", templateBody)
	}
}

func TestAtomicWriteFileLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "index.html")

	if err := atomicWriteFile(target, []byte("<html/>"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := atomicWriteFile(target, []byte("<html/>"), 0o600); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "tmp_") {
			t.Fatalf("temp file left behind: %s", entry.Name())
		}
	}
}

func TestAtomicWriteFilePreservesOriginalOnFailure(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "missing", "child.html")

	if err := atomicWriteFile(nested, []byte("<html/>"), 0o600); err == nil {
		t.Fatal("expected an error writing into a missing directory")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("leftover entries after failed write: %v", entries)
	}
}

func tuneRunner(t *testing.T, workspace string) *androidRunner {
	t.Helper()
	cfg := AndroidConfig{
		Workspace:    workspace,
		SharedConfig: SharedConfig{BuildTimeout: time.Minute},
	}
	return NewAndroidRunner(cfg, nil, nil, nil, slog.Default())
}

func readGradleProperties(t *testing.T, workspace string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(workspace, "android", "gradle.properties"))
	if err != nil {
		t.Fatalf("read gradle.properties: %v", err)
	}
	return string(body)
}

func writeGradleProperties(t *testing.T, workspace, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(workspace, "android"), 0o755); err != nil {
		t.Fatalf("mkdir android: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "android", "gradle.properties"), []byte(body), 0o600); err != nil {
		t.Fatalf("seed gradle.properties: %v", err)
	}
}

func TestTuneGradlePropertiesAddsNativeLibPickFirst(t *testing.T) {
	workspace := t.TempDir()
	writeGradleProperties(t, workspace, "org.gradle.jvmargs=-Xmx2048m -XX:MaxMetaspaceSize=512m\n")

	if err := tuneRunner(t, workspace).tuneGradleProperties(os.Stdout); err != nil {
		t.Fatalf("tune: %v", err)
	}

	body := readGradleProperties(t, workspace)
	if !strings.Contains(body, "android.packagingOptions.pickFirsts=**/*.so") {
		t.Fatalf("pickFirsts not tuned:\n%s", body)
	}
	for _, want := range []string{
		"org.gradle.jvmargs=-Xmx2048m -XX:MaxMetaspaceSize=512m",
		"reactNativeArchitectures=arm64-v8a",
		"org.gradle.parallel=false",
		"org.gradle.workers.max=2",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q:\n%s", want, body)
		}
	}
}

func TestTuneGradlePropertiesReplacesExistingPickFirsts(t *testing.T) {
	workspace := t.TempDir()
	writeGradleProperties(t, workspace,
		"reactNativeArchitectures=armeabi-v7a,arm64-v8a\n"+
			"android.packagingOptions.pickFirsts=/LICENSE,**/picasa.ini\n")

	if err := tuneRunner(t, workspace).tuneGradleProperties(os.Stdout); err != nil {
		t.Fatalf("tune: %v", err)
	}

	body := readGradleProperties(t, workspace)
	if strings.Contains(body, "picasa.ini") {
		t.Fatalf("stale pickFirsts survived:\n%s", body)
	}
	if strings.Count(body, "android.packagingOptions.pickFirsts=") != 1 {
		t.Fatalf("expected exactly one pickFirsts line:\n%s", body)
	}
	if !strings.Contains(body, "android.packagingOptions.pickFirsts=**/*.so") {
		t.Fatalf("pickFirsts not tuned:\n%s", body)
	}
}

func TestRemovePropertyKeepsSiblingPackagingOptions(t *testing.T) {
	body := strings.Join([]string{
		"android.packagingOptions.pickFirsts=a.so",
		"android.packagingOptions.excludes=b.so",
		"android.packagingOptions.pickFirstsExtra=c.so",
	}, "\n")

	out := removeProperty(body, "android.packagingOptions.pickFirsts")

	if strings.Contains(out, "pickFirsts=") {
		t.Fatalf("target property survived:\n%s", out)
	}
	for _, want := range []string{
		"android.packagingOptions.excludes=b.so",
		"android.packagingOptions.pickFirstsExtra=c.so",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("sibling property dropped:\n%s", out)
		}
	}
}
