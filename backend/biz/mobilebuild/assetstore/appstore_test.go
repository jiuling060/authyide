package assetstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestAppStorePutReadRoundTrip(t *testing.T) {
	store := NewAppStore(t.TempDir())
	ctx := context.Background()
	id := uuid.New()
	body := []byte{0x89, 'P', 'N', 'G'}

	if err := store.Put(ctx, id, body); err != nil {
		t.Fatalf("put: %v", err)
	}

	got, err := store.Read(ctx, id)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("read body = %q, want %q", got, body)
	}
}

func TestAppStoreReadMissingReturnsErrEmpty(t *testing.T) {
	store := NewAppStore(t.TempDir())
	if _, err := store.Read(context.Background(), uuid.New()); !errors.Is(err, ErrEmpty) {
		t.Fatalf("read missing = %v, want ErrEmpty", err)
	}
}

func TestAppStorePathMissingReturnsErrEmpty(t *testing.T) {
	store := NewAppStore(t.TempDir())
	if _, err := store.Path(context.Background(), uuid.New()); !errors.Is(err, ErrEmpty) {
		t.Fatalf("path missing = %v, want ErrEmpty", err)
	}
}

func TestAppStoreRejectsNilID(t *testing.T) {
	store := NewAppStore(t.TempDir())
	ctx := context.Background()

	if err := store.Put(ctx, uuid.Nil, []byte("x")); err == nil {
		t.Fatal("put with nil id succeeded")
	}
	if _, err := store.Read(ctx, uuid.Nil); err == nil {
		t.Fatal("read with nil id succeeded")
	}
	if _, err := store.Path(ctx, uuid.Nil); err == nil {
		t.Fatal("path with nil id succeeded")
	}
	if err := store.Delete(ctx, uuid.Nil); err == nil {
		t.Fatal("delete with nil id succeeded")
	}
}

func TestAppStoreDeleteMissingIsIdempotent(t *testing.T) {
	store := NewAppStore(t.TempDir())
	if err := store.Delete(context.Background(), uuid.New()); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
}

func TestAppStoreAtomicPutLeavesNoTempFiles(t *testing.T) {
	store := NewAppStore(t.TempDir())
	ctx := context.Background()
	id := uuid.New()

	if err := store.Put(ctx, id, []byte("one")); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := store.Put(ctx, id, []byte("two")); err != nil {
		t.Fatalf("rewrite: %v", err)
	}

	got, err := store.Read(ctx, id)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "two" {
		t.Fatalf("read body = %q", got)
	}

	entries, err := os.ReadDir(filepath.Dir(store.(*appStore).path(id)))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Fatalf("temp file left behind: %s", entry.Name())
		}
	}
}
