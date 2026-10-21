// Package assetstore persists customised mobile app branding assets.
//
// Assets are small PNGs and are built on the same host as the gradle run, so
// they live on local disk next to the build logs instead of in object storage:
// the pipeline then only copies files locally and never needs a network call
// during a build.
package assetstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chaitin/MonkeyCode/backend/biz/mobilebuild/domain"
)

// ErrEmpty indicates a slot has no stored asset.
var ErrEmpty = errors.New("asset slot is empty")

// Store persists branding assets keyed by asset slot.
type Store interface {
	Put(ctx context.Context, slot string, data []byte) error
	Read(ctx context.Context, slot string) ([]byte, error)
	Size(ctx context.Context, slot string) (int64, error)
	Exists(ctx context.Context, slot string) bool
	Delete(ctx context.Context, slot string) error
}

// NewFileStore returns a Store rooted at dir. Slot names are whitelisted and
// never joined with user input, so the resulting paths stay inside dir.
func NewFileStore(dir string) Store {
	return &fileStore{root: filepath.Clean(dir)}
}

type fileStore struct {
	root string
}

func (s *fileStore) path(slot string) (string, error) {
	if !domain.AssetSlotValid(slot) {
		return "", fmt.Errorf("invalid asset slot %q", slot)
	}
	return filepath.Join(s.root, slot+".png"), nil
}

func (s *fileStore) Put(ctx context.Context, slot string, data []byte) error {
	p, err := s.path(slot)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return fmt.Errorf("create asset dir: %w", err)
	}
	tmp, err := os.CreateTemp(s.root, slot+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp asset: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write asset: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync asset: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close asset: %w", err)
	}
	if err = os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("chmod asset: %w", err)
	}
	if err = os.Rename(tmpName, p); err != nil {
		return fmt.Errorf("replace asset: %w", err)
	}
	return nil
}

func (s *fileStore) Read(ctx context.Context, slot string) ([]byte, error) {
	p, err := s.path(slot)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrEmpty
		}
		return nil, fmt.Errorf("read asset: %w", err)
	}
	return data, nil
}

func (s *fileStore) Size(ctx context.Context, slot string) (int64, error) {
	p, err := s.path(slot)
	if err != nil {
		return 0, err
	}
	fi, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, ErrEmpty
		}
		return 0, fmt.Errorf("stat asset: %w", err)
	}
	return fi.Size(), nil
}

func (s *fileStore) Exists(ctx context.Context, slot string) bool {
	p, err := s.path(slot)
	if err != nil {
		return false
	}
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

func (s *fileStore) Delete(ctx context.Context, slot string) error {
	p, err := s.path(slot)
	if err != nil {
		return err
	}
	if err = os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove asset: %w", err)
	}
	return nil
}
