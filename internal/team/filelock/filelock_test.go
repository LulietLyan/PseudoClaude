package filelock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.lock")
	release, err := Acquire(context.Background(), path, Options{Attempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock not created: %v", err)
	}
	release()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock not removed: %v", err)
	}
}

func TestAcquireFailsWhenLocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.lock")
	release, err := Acquire(context.Background(), path, Options{Attempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := Acquire(context.Background(), path, Options{Attempts: 1}); !errors.Is(err, os.ErrExist) {
		t.Fatalf("Acquire err = %v, want exists", err)
	}
}

func TestAcquireRemovesStaleLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.lock")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	release, err := Acquire(context.Background(), path, Options{Attempts: 2, StaleAge: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	release()
}
