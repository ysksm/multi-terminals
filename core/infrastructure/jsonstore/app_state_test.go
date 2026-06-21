package jsonstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAppStateStore_LoadNotExist(t *testing.T) {
	dir := t.TempDir()
	s := NewAppStateStore(dir)
	ctx := context.Background()

	id, ok, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("expected no error on missing file, got %v", err)
	}
	if ok {
		t.Errorf("expected ok=false when file doesn't exist, got ok=true with id=%q", id)
	}
}

func TestAppStateStore_SetAndLoad(t *testing.T) {
	dir := t.TempDir()
	s := NewAppStateStore(dir)
	ctx := context.Background()

	const wantID = "workspace-123"

	if err := s.SetLastOpened(ctx, wantID); err != nil {
		t.Fatalf("SetLastOpened: %v", err)
	}

	gotID, ok, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true after SetLastOpened, got false")
	}
	if gotID != wantID {
		t.Errorf("Load returned id %q, want %q", gotID, wantID)
	}
}

func TestAppStateStore_Overwrite(t *testing.T) {
	dir := t.TempDir()
	s := NewAppStateStore(dir)
	ctx := context.Background()

	if err := s.SetLastOpened(ctx, "first-id"); err != nil {
		t.Fatalf("first SetLastOpened: %v", err)
	}
	if err := s.SetLastOpened(ctx, "second-id"); err != nil {
		t.Fatalf("second SetLastOpened: %v", err)
	}

	gotID, ok, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true, got false")
	}
	if gotID != "second-id" {
		t.Errorf("Load returned %q, want %q", gotID, "second-id")
	}
}

func TestAppStateStore_EmptyIDSetOkFalse(t *testing.T) {
	dir := t.TempDir()
	s := NewAppStateStore(dir)
	ctx := context.Background()

	// Set a non-empty ID first.
	if err := s.SetLastOpened(ctx, "some-id"); err != nil {
		t.Fatalf("SetLastOpened: %v", err)
	}

	// Now set empty ID; Load should return ok=false.
	if err := s.SetLastOpened(ctx, ""); err != nil {
		t.Fatalf("SetLastOpened empty: %v", err)
	}

	_, ok, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ok {
		t.Errorf("expected ok=false when LastOpenedWorkspaceID is empty, got true")
	}
}

func TestAppStateStore_UnknownVersionError(t *testing.T) {
	dir := t.TempDir()
	s := NewAppStateStore(dir)
	ctx := context.Background()

	// Write a file with a future schema version directly to the expected path.
	statePath := filepath.Join(dir, "app-state.json")
	data := []byte(`{"version":999,"lastOpenedWorkspaceId":"ws-1"}`)
	if err := os.WriteFile(statePath, data, 0o644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	_, _, err := s.Load(ctx)
	if err == nil {
		t.Fatal("expected error for unknown schema version, got nil")
	}
}
