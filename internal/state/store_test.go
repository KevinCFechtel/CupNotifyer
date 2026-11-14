package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileStorePersistsTargetState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	store, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore() error = %v", err)
	}
	if store.IsSent("pushover:1", "server|image", "version:v2") {
		t.Fatal("new store unexpectedly contains notification state")
	}
	if err := store.MarkSent("pushover:1", "server|image", "version:v2"); err != nil {
		t.Fatalf("MarkSent() error = %v", err)
	}
	if !store.IsSent("pushover:1", "server|image", "version:v2") {
		t.Fatal("state was not updated")
	}
	if store.IsSent("pushover:2", "server|image", "version:v2") {
		t.Fatal("state leaked into another target")
	}

	reloaded, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("reloading store: %v", err)
	}
	if !reloaded.IsSent("pushover:1", "server|image", "version:v2") {
		t.Fatal("persisted state was not reloaded")
	}
}

func TestFileStoreRejectsCorruptState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStore(path); err == nil {
		t.Fatal("NewFileStore() error = nil, want an error")
	}
}
