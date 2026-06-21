package port

import "context"

// AppStateStore persists global application state that survives process restarts.
// The infrastructure layer provides a concrete implementation (e.g. jsonstore.AppStateStore).
type AppStateStore interface {
	// Load reads the last-opened workspace ID from the backing store.
	// If no state has been recorded yet, it returns ("", false, nil).
	// If a workspace ID has been recorded, it returns (id, true, nil).
	// An unknown (future) schema version is rejected with an error.
	Load(ctx context.Context) (workspaceID string, ok bool, err error)

	// SetLastOpened atomically writes workspaceID as the last-opened workspace.
	// Passing an empty string is valid; subsequent Load calls will return ok=false.
	SetLastOpened(ctx context.Context, workspaceID string) error
}
