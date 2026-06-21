package jsonstore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ysksm/multi-terminals/core/domain"
)

// WorkspaceRepository is a JSON-file-backed implementation of domain.WorkspaceRepository.
// Each workspace is stored as a separate JSON file named <id>.json under dir.
// File writes are atomic: data is written to <file>.tmp then renamed to <file>.
// All methods are safe for concurrent use via an RWMutex.
type WorkspaceRepository struct {
	dir string
	mu  sync.RWMutex
}

// Compile-time assertion: WorkspaceRepository must satisfy domain.WorkspaceRepository.
var _ domain.WorkspaceRepository = (*WorkspaceRepository)(nil)

// NewWorkspaceRepository creates a WorkspaceRepository rooted at
// filepath.Join(baseDir, "workspaces"). The subdirectory is created if it
// does not exist.
func NewWorkspaceRepository(baseDir string) (*WorkspaceRepository, error) {
	dir := filepath.Join(baseDir, "workspaces")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating workspaces directory %q: %w", dir, err)
	}
	return &WorkspaceRepository{dir: dir}, nil
}

// DefaultBaseDir returns the OS-appropriate user configuration directory
// joined with "multi-terminals". Adapters use this as the default baseDir.
func DefaultBaseDir() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("getting user config dir: %w", err)
	}
	return filepath.Join(cfgDir, "multi-terminals"), nil
}

// pathFor returns the expected file path for a workspace with the given id string.
// It returns an error if id is not a safe single path segment (e.g. contains path separators or "..").
func (r *WorkspaceRepository) pathFor(id string) (string, error) {
	if id == "" || id == "." || id == ".." ||
		strings.ContainsRune(id, os.PathSeparator) ||
		strings.ContainsRune(id, '/') ||
		strings.Contains(id, "..") {
		return "", fmt.Errorf("invalid workspace id %q", id)
	}
	return filepath.Join(r.dir, id+".json"), nil
}

// Save marshals w to JSON and writes it atomically to pathFor(w.ID()).
// An existing file for the same ID is overwritten.
func (r *WorkspaceRepository) Save(ctx context.Context, w *domain.Workspace) error {
	rec := toRecord(w)
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling workspace %q: %w", w.ID().String(), err)
	}

	path, err := r.pathFor(w.ID().String())
	if err != nil {
		return err
	}
	tmp := path + ".tmp"

	r.mu.Lock()
	defer r.mu.Unlock()

	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("writing tmp file %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		// Best-effort cleanup of the tmp file.
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming %q to %q: %w", tmp, path, err)
	}
	return nil
}

// FindByID loads the workspace with the given id from disk.
// Returns domain.ErrWorkspaceNotFound if no file exists for that id.
func (r *WorkspaceRepository) FindByID(ctx context.Context, id domain.WorkspaceId) (*domain.Workspace, error) {
	path, err := r.pathFor(id.String())
	if err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, domain.ErrWorkspaceNotFound
		}
		return nil, fmt.Errorf("reading workspace file %q: %w", path, err)
	}

	var rec workspaceRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("unmarshalling workspace file %q: %w", path, err)
	}

	ws, err := toDomain(rec)
	if err != nil {
		return nil, fmt.Errorf("converting record from %q to domain: %w", path, err)
	}
	return ws, nil
}

// List returns all workspaces stored under r.dir.
// Only files with a ".json" extension are considered. An empty directory returns
// an empty (non-nil) slice. A single file that cannot be parsed fails the whole
// call with an error that includes the filename.
func (r *WorkspaceRepository) List(ctx context.Context) ([]*domain.Workspace, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, fmt.Errorf("reading workspaces directory %q: %w", r.dir, err)
	}

	workspaces := make([]*domain.Workspace, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(r.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading workspace file %q: %w", entry.Name(), err)
		}

		var rec workspaceRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return nil, fmt.Errorf("unmarshalling workspace file %q: %w", entry.Name(), err)
		}

		ws, err := toDomain(rec)
		if err != nil {
			return nil, fmt.Errorf("converting record from %q to domain: %w", entry.Name(), err)
		}

		workspaces = append(workspaces, ws)
	}

	return workspaces, nil
}

// Delete removes the workspace file for the given id.
// Returns domain.ErrWorkspaceNotFound if no file exists for that id.
func (r *WorkspaceRepository) Delete(ctx context.Context, id domain.WorkspaceId) error {
	path, err := r.pathFor(id.String())
	if err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return domain.ErrWorkspaceNotFound
		}
		return fmt.Errorf("deleting workspace file %q: %w", path, err)
	}
	return nil
}
