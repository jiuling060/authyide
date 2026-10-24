package assetstore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// AppStore persists per-app icons. The instance-wide Store is keyed by a
// whitelisted slot, which cannot express "this App's icon", so app icons live
// in their own subdirectory keyed by app id. The id is validated, so the
// resulting path stays inside the root.
type AppStore interface {
	Put(ctx context.Context, appID uuid.UUID, data []byte) error
	Read(ctx context.Context, appID uuid.UUID) ([]byte, error)
	// Path returns the on-disk location of one app's icon, or ErrEmpty when the
	// app has none. Handlers stream it instead of copying bytes.
	Path(ctx context.Context, appID uuid.UUID) (string, error)
	Delete(ctx context.Context, appID uuid.UUID) error
}

// NewAppStore returns an AppStore rooted at dir.
func NewAppStore(dir string) AppStore {
	return &appStore{root: filepath.Clean(dir)}
}

type appStore struct {
	root string
}

func (s *appStore) path(appID uuid.UUID) string {
	if appID == uuid.Nil {
		return ""
	}
	return filepath.Join(s.root, appID.String()+".png")
}

func (s *appStore) Put(ctx context.Context, appID uuid.UUID, data []byte) error {
	p := s.path(appID)
	if p == "" {
		return fmt.Errorf("invalid app id")
	}
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return fmt.Errorf("create app asset dir: %w", err)
	}
	tmp, err := os.CreateTemp(s.root, appID.String()+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp app asset: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write app asset: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync app asset: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close app asset: %w", err)
	}
	if err = os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("chmod app asset: %w", err)
	}
	if err = os.Rename(tmpName, p); err != nil {
		return fmt.Errorf("replace app asset: %w", err)
	}
	return nil
}

func (s *appStore) Path(ctx context.Context, appID uuid.UUID) (string, error) {
	p := s.path(appID)
	if p == "" {
		return "", fmt.Errorf("invalid app id")
	}
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return "", ErrEmpty
		}
		return "", fmt.Errorf("stat app asset: %w", err)
	}
	return p, nil
}

func (s *appStore) Read(ctx context.Context, appID uuid.UUID) ([]byte, error) {
	p := s.path(appID)
	if p == "" {
		return nil, fmt.Errorf("invalid app id")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrEmpty
		}
		return nil, fmt.Errorf("read app asset: %w", err)
	}
	return data, nil
}

func (s *appStore) Delete(ctx context.Context, appID uuid.UUID) error {
	p := s.path(appID)
	if p == "" {
		return fmt.Errorf("invalid app id")
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove app asset: %w", err)
	}
	return nil
}
