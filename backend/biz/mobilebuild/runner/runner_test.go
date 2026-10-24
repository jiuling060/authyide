package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
