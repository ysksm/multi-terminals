package jsonstore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// AppStateStore persists global application state (e.g. last-opened workspace)
// in a single JSON file. Reads and writes are safe for concurrent use.
type AppStateStore struct {
	path string
	mu   sync.RWMutex
}

// NewAppStateStore returns an AppStateStore that reads and writes
// filepath.Join(baseDir, "app-state.json").
func NewAppStateStore(baseDir string) *AppStateStore {
	return &AppStateStore{
		path: filepath.Join(baseDir, "app-state.json"),
	}
}

// Load reads the application state from disk.
// If the file does not exist, it returns ("", false, nil).
// If the file exists and LastOpenedWorkspaceID is non-empty, it returns (id, true, nil).
// If the file exists but LastOpenedWorkspaceID is empty, it returns ("", false, nil).
// An unknown (future) schema version is rejected with an error.
func (s *AppStateStore) Load(_ context.Context) (workspaceID string, ok bool, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("reading app state file %q: %w", s.path, err)
	}

	var rec appStateRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return "", false, fmt.Errorf("unmarshalling app state file %q: %w", s.path, err)
	}

	if rec.Version < 1 || rec.Version > CurrentSchemaVersion {
		return "", false, fmt.Errorf("unsupported schema version %d in %q", rec.Version, s.path)
	}

	if rec.LastOpenedWorkspaceID == "" {
		return "", false, nil
	}
	return rec.LastOpenedWorkspaceID, true, nil
}

// SetLastOpened atomically writes the given workspaceID as the last-opened workspace.
// An empty workspaceID is valid and will cause subsequent Load calls to return ok=false.
func (s *AppStateStore) SetLastOpened(_ context.Context, workspaceID string) error {
	rec := appStateRecord{
		Version:               CurrentSchemaVersion,
		LastOpenedWorkspaceID: workspaceID,
	}

	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling app state: %w", err)
	}

	tmp := s.path + ".tmp"

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing tmp app state file %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming %q to %q: %w", tmp, s.path, err)
	}
	return nil
}
