package jsonstore

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ysksm/multi-terminals/core/domain/task"
)

func TestSettingsStore_DefaultWhenMissing(t *testing.T) {
	s := NewSettingsStore(t.TempDir())
	got, err := s.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, task.DefaultSettings()) {
		t.Errorf("expected defaults %+v, got %+v", task.DefaultSettings(), got)
	}
}

func TestSettingsStore_RoundTrip(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	s := NewSettingsStore(base)
	want := task.Settings{AutoFetchBeforeSetup: false, IncludeNodeModules: false}
	if err := s.Save(ctx, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %+v, got %+v", want, got)
	}
	if _, err := os.Stat(filepath.Join(base, "task_settings.json")); err != nil {
		t.Errorf("settings file not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "task_settings.json.tmp")); !os.IsNotExist(err) {
		t.Errorf("tmp file should be removed, stat err=%v", err)
	}
}

func TestSettingsStore_CreatesParentDir(t *testing.T) {
	base := filepath.Join(t.TempDir(), "nested", "dir")
	s := NewSettingsStore(base)
	if err := s.Save(context.Background(), task.DefaultSettings()); err != nil {
		t.Fatalf("Save into missing dir: %v", err)
	}
}

func TestSettingsStore_RejectsFutureVersion(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "task_settings.json"), []byte(`{"version": 42, "record": {}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSettingsStore(base).Load(context.Background()); err == nil {
		t.Error("expected error for version 42")
	}
}
