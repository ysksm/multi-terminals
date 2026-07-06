package jsonstore

// CurrentSchemaVersion is the version written into every persisted record.
// Increment this when the on-disk format changes in a backward-incompatible way.
const CurrentSchemaVersion = 1

// startupCommandRecord is the JSON DTO for a single startup command.
type startupCommandRecord struct {
	Command string `json:"command"`
	AutoRun bool   `json:"autoRun"`
}

// paneRecord is the JSON DTO for a single pane.
// RemoteHost is empty for local panes; the field is additive and old records
// without it load as local panes, so the schema version is unchanged.
type paneRecord struct {
	ID         string                 `json:"id"`
	Directory  string                 `json:"directory"`
	Slot       int                    `json:"slot"`
	Title      string                 `json:"title,omitempty"`
	RemoteHost string                 `json:"remoteHost,omitempty"`
	Commands   []startupCommandRecord `json:"commands"`
}

// workspaceRecord is the JSON DTO for a workspace aggregate.
// Version must always be written so the loader can detect future formats.
type workspaceRecord struct {
	Version          int          `json:"version"`
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	Layout           string       `json:"layout"`
	Panes            []paneRecord `json:"panes"`
	LastActivePaneID *string      `json:"lastActivePaneId,omitempty"`
	MaximizedPaneID  *string      `json:"maximizedPaneId,omitempty"`
}

// appStateRecord is the JSON DTO for the global application state file.
type appStateRecord struct {
	Version               int    `json:"version"`
	LastOpenedWorkspaceID string `json:"lastOpenedWorkspaceId"`
}
