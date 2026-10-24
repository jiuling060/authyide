// Package runner: the session source overlay shared by both platform pipelines.
package runner

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
)

const (
	// OverlayMaxFiles caps the number of tar entries one snapshot may hold, so
	// a runaway archive cannot exhaust the disk before the assemble step.
	OverlayMaxFiles = 20000
	// OverlayMaxBytes caps the decompressed size of one regular entry.
	OverlayMaxBytes = 512 << 20
	// OverlayTotalMaxBytes caps the decompressed total, so a compressed bomb
	// cannot outgrow the workspace copy.
	OverlayTotalMaxBytes = 2 << 30
)

// sourceOverlay applies one session snapshot onto a workspace root. It verifies
// size and digest, validates every entry, and only then writes, so a bad
// archive fails the job before the workspace is touched. Nothing is deleted, so
// files only the workspace template has survive the overlay.
type sourceOverlay struct {
	objects   domain.ObjectSource
	snapshot  *domain.SourceSnapshot
	workspace string
	logDir    string
	// skipRoots are workspace-relative directories one pipeline stages on its
	// own. Entries under them are dropped so a session archive cannot overwrite
	// a staged artifact and break the bundle.
	skipRoots []string
}

func newSourceOverlay(objects domain.ObjectSource, workspace string, snap *domain.SourceSnapshot, logDir string, skipRoots ...string) *sourceOverlay {
	return &sourceOverlay{
		objects:   objects,
		snapshot:  snap,
		workspace: filepath.Clean(workspace),
		logDir:    logDir,
		skipRoots: skipRoots,
	}
}

// Apply overlays the snapshot and returns the paths it created, so a caller that
// shares the workspace with the next job can roll the overlay back.
func (o *sourceOverlay) Apply(ctx context.Context, log io.Writer) ([]string, error) {
	if o.objects == nil {
		return nil, fmt.Errorf("object store is not configured")
	}
	snap := o.snapshot

	tmp, err := os.CreateTemp(o.logDir, "snapshot-*.tar.gz")
	if err != nil {
		return nil, fmt.Errorf("create snapshot buffer: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	body, err := o.objects.GetObject(ctx, snap.ObjectKey)
	if err != nil {
		tmp.Close()
		return nil, fmt.Errorf("download snapshot %s: %w", snap.ObjectKey, err)
	}
	written, err := io.Copy(tmp, body)
	_ = body.Close()
	if err != nil {
		tmp.Close()
		return nil, fmt.Errorf("download snapshot %s: %w", snap.ObjectKey, err)
	}
	if err = tmp.Close(); err != nil {
		return nil, fmt.Errorf("close snapshot buffer: %w", err)
	}
	if snap.SizeBytes > 0 && written != snap.SizeBytes {
		return nil, fmt.Errorf("snapshot %s size mismatch: want %d got %d", snap.ObjectKey, snap.SizeBytes, written)
	}
	if snap.SHA256 != "" {
		got, err := fileSHA256(tmpName)
		if err != nil {
			return nil, fmt.Errorf("hash snapshot %s: %w", snap.ObjectKey, err)
		}
		if got != strings.ToLower(snap.SHA256) {
			return nil, fmt.Errorf("snapshot %s digest mismatch: want %s got %s", snap.ObjectKey, snap.SHA256, got)
		}
	}

	paths, n, skipped, err := o.extract(tmpName)
	if err != nil {
		return nil, err
	}
	if _, err = fmt.Fprintf(log, "[overlay-source] snapshot %s overlaid %d entr(ies)%s\n",
		snap.ObjectKey, n, skipSummary(skipped)); err != nil {
		return nil, fmt.Errorf("write build log: %w", err)
	}
	return paths, nil
}

// root is the overlay root with a trailing separator, so filepath.Join and
// filepath.Rel agree about the root itself.
func (o *sourceOverlay) root() string {
	return o.workspace + string(os.PathSeparator)
}

// skips reports whether one entry lands in a directory the pipeline owns.
func (o *sourceOverlay) skips(name string) bool {
	cleaned := filepath.ToSlash(name)
	for _, r := range o.skipRoots {
		if cleaned == r || strings.HasPrefix(cleaned, r+"/") {
			return true
		}
	}
	return false
}

// extract validates the archive and only then unpacks it onto the workspace
// copy, so a bad entry aborts the job before the copy is touched at all.
func (o *sourceOverlay) extract(archive string) ([]string, int, int, error) {
	if err := o.validate(archive); err != nil {
		return nil, 0, 0, err
	}
	return o.write(archive)
}

// validate walks every header without writing, rejecting empty names, paths
// that escape the workspace, link and device entries, oversized files and entry
// counts.
func (o *sourceOverlay) validate(archive string) error {
	f, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("open snapshot archive: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open snapshot gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	workspace := o.root()
	entries := 0
	var totalBytes int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read snapshot archive: %w", err)
		}
		entries++
		if entries > OverlayMaxFiles {
			return fmt.Errorf("snapshot has %d entries, over the %d limit", entries, OverlayMaxFiles)
		}
		name, err := cleanSnapshotName(hdr)
		if err != nil {
			return err
		}
		if o.skips(name) {
			continue
		}
		if _, err = safeJoin(workspace, name); err != nil {
			return fmt.Errorf("snapshot entry %q: %w", hdr.Name, err)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
		case tar.TypeReg, tar.TypeRegA:
			if hdr.Size < 0 {
				return fmt.Errorf("snapshot entry %q has a negative size", name)
			}
			if hdr.Size > OverlayMaxBytes {
				return fmt.Errorf("snapshot entry %q is %d bytes, over the %d byte limit",
					name, hdr.Size, OverlayMaxBytes)
			}
			totalBytes += hdr.Size
			if totalBytes > OverlayTotalMaxBytes {
				return fmt.Errorf("snapshot expands to %d bytes, over the %d byte limit",
					totalBytes, OverlayTotalMaxBytes)
			}
		case tar.TypeSymlink, tar.TypeLink, tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
			return fmt.Errorf("snapshot entry %q uses type %q, which is not allowed", name, string(hdr.Typeflag))
		default:
			// PAX extended headers and similar metadata entries carry no payload.
		}
	}
	if entries == 0 {
		return fmt.Errorf("snapshot archive is empty")
	}
	return nil
}

// write is the second pass: every entry already passed validate, so the only
// failures left are I/O errors. It records only the files and directories it
// created; a file the snapshot overwrote is left out, because deleting it on
// rollback would destroy the template copy that a later git checkout restores.
func (o *sourceOverlay) write(archive string) ([]string, int, int, error) {
	f, err := os.Open(archive)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("open snapshot archive: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("open snapshot gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	workspace := o.root()
	created := make([]string, 0, 256)
	entries := 0
	skipped := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, 0, fmt.Errorf("read snapshot archive: %w", err)
		}
		name, err := cleanSnapshotName(hdr)
		if err != nil {
			return nil, 0, 0, err
		}
		if o.skips(name) {
			skipped++
			continue
		}
		dst, err := safeJoin(workspace, name)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("snapshot entry %q: %w", hdr.Name, err)
		}
		entries++

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err = mkdirNew(dst, &created); err != nil {
				return nil, 0, 0, err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err = mkdirNew(filepath.Dir(dst), &created); err != nil {
				return nil, 0, 0, err
			}
			_, statErr := os.Stat(dst)
			fresh := os.IsNotExist(statErr)
			if !fresh && statErr != nil {
				return nil, 0, 0, fmt.Errorf("stat %s: %w", dst, statErr)
			}
			if err = copyTarEntry(tr, dst); err != nil {
				return nil, 0, 0, fmt.Errorf("write %s: %w", name, err)
			}
			if fresh {
				created = append(created, dst)
			}
		default:
			// PAX extended headers and similar metadata entries carry no payload.
		}
	}
	return created, entries, skipped, nil
}

// mkdirNew creates the missing ancestors of dir one level at a time and records
// each one, so the caller can tell a directory it created from one the template
// already had.
func mkdirNew(dir string, created *[]string) error {
	for d := filepath.Clean(dir); ; d = filepath.Dir(d) {
		_, err := os.Stat(d)
		if err == nil {
			return nil
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat %s: %w", d, err)
		}
		if err = os.Mkdir(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
		*created = append(*created, d)
		if filepath.Dir(d) == d {
			return nil
		}
	}
}

// rollbackOverlay removes the paths one Apply created, deepest path first, so a
// workspace shared with the next job returns to its own tree. A directory that
// still holds a template file stays put.
func rollbackOverlay(paths []string) {
	sort.Slice(paths, func(i, j int) bool {
		return depthOf(paths[i]) > depthOf(paths[j])
	})
	for _, p := range paths {
		_ = os.Remove(p)
	}
}

func depthOf(p string) int {
	return strings.Count(filepath.Clean(p), string(os.PathSeparator))
}

func skipSummary(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(", skipped %d entr(ies) under runner-owned paths", n)
}

// cleanSnapshotName normalises one tar entry name and refuses empty ones.
func cleanSnapshotName(hdr *tar.Header) (string, error) {
	name := strings.TrimPrefix(hdr.Name, "./")
	if name == "" {
		return "", fmt.Errorf("snapshot entry %q has no path", hdr.Name)
	}
	return name, nil
}

// copyTarEntry streams one regular entry to dst through a temp file and renames
// it over the target. The rename drops any hard link the target had to the
// shared template inode, so a job's overlay can never mutate the template.
func copyTarEntry(src io.Reader, dst string) error {
	tmp, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp for %s: %w", dst, err)
	}
	tmpName := tmp.Name()
	if _, err = io.Copy(tmp, src); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write %s: %w", dst, err)
	}
	if err = tmp.Chmod(0o644); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err = tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err = os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("replace %s: %w", dst, err)
	}
	return nil
}
