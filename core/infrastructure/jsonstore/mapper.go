package jsonstore

import (
	"fmt"

	"github.com/ysksm/multi-terminals/core/domain"
)

// toRecord converts a domain.Workspace aggregate to a workspaceRecord persistence DTO.
// The Version field is always set to CurrentSchemaVersion.
// LastActivePaneID and MaximizedPaneID are set to *string when present, nil otherwise.
func toRecord(w *domain.Workspace) workspaceRecord {
	panes := w.Panes()
	paneRecs := make([]paneRecord, len(panes))
	for i, p := range panes {
		cmds := p.Commands()
		cmdRecs := make([]startupCommandRecord, len(cmds))
		for j, c := range cmds {
			cmdRecs[j] = startupCommandRecord{
				Command: c.Command(),
				AutoRun: c.AutoRun(),
			}
		}
		paneRecs[i] = paneRecord{
			ID:        p.ID().String(),
			Directory: p.Directory().String(),
			Slot:      p.Slot().Int(),
			Commands:  cmdRecs,
		}
	}

	rec := workspaceRecord{
		Version: CurrentSchemaVersion,
		ID:      w.ID().String(),
		Name:    w.Name().String(),
		Layout:  string(w.Layout()),
		Panes:   paneRecs,
	}

	if id, ok := w.LastActivePaneId(); ok {
		s := id.String()
		rec.LastActivePaneID = &s
	}

	if id, ok := w.MaximizedPaneId(); ok {
		s := id.String()
		rec.MaximizedPaneID = &s
	}

	return rec
}

// toDomain converts a workspaceRecord persistence DTO back to a domain.Workspace aggregate.
// It validates the schema version, constructs all Value Objects, and calls
// domain.ReconstituteWorkspace to re-validate all invariants.
func toDomain(rec workspaceRecord) (*domain.Workspace, error) {
	// Validate schema version
	if rec.Version < 1 {
		return nil, fmt.Errorf("unsupported schema version %d: version must be >= 1", rec.Version)
	}
	if rec.Version > CurrentSchemaVersion {
		return nil, fmt.Errorf("unsupported schema version %d: only versions up to %d are supported", rec.Version, CurrentSchemaVersion)
	}

	// Build workspace ID
	wsID, err := domain.NewWorkspaceId(rec.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid workspace id %q: %w", rec.ID, err)
	}

	// Build workspace name
	wsName, err := domain.NewWorkspaceName(rec.Name)
	if err != nil {
		return nil, fmt.Errorf("invalid workspace name %q: %w", rec.Name, err)
	}

	// Build layout preset and validate
	layout := domain.LayoutPreset(rec.Layout)
	if !layout.IsValid() {
		return nil, fmt.Errorf("invalid layout preset %q", rec.Layout)
	}

	// Build panes
	panes := make([]*domain.Pane, 0, len(rec.Panes))
	for _, pr := range rec.Panes {
		paneID, err := domain.NewPaneId(pr.ID)
		if err != nil {
			return nil, fmt.Errorf("invalid pane id %q: %w", pr.ID, err)
		}

		dir, err := domain.NewDirectoryPath(pr.Directory)
		if err != nil {
			return nil, fmt.Errorf("invalid directory path %q for pane %q: %w", pr.Directory, pr.ID, err)
		}

		slot, err := domain.NewSlotIndex(pr.Slot)
		if err != nil {
			return nil, fmt.Errorf("invalid slot index %d for pane %q: %w", pr.Slot, pr.ID, err)
		}

		cmds := make([]domain.StartupCommand, 0, len(pr.Commands))
		for _, cr := range pr.Commands {
			cmd, err := domain.NewStartupCommand(cr.Command, cr.AutoRun)
			if err != nil {
				return nil, fmt.Errorf("invalid startup command %q for pane %q: %w", cr.Command, pr.ID, err)
			}
			cmds = append(cmds, cmd)
		}

		pane, err := domain.NewPane(paneID, dir, slot, cmds)
		if err != nil {
			return nil, fmt.Errorf("cannot build pane %q: %w", pr.ID, err)
		}
		panes = append(panes, pane)
	}

	// Build optional PaneId pointers
	var lastActive *domain.PaneId
	if rec.LastActivePaneID != nil {
		id, err := domain.NewPaneId(*rec.LastActivePaneID)
		if err != nil {
			return nil, fmt.Errorf("invalid lastActivePaneId %q: %w", *rec.LastActivePaneID, err)
		}
		lastActive = &id
	}

	var maximized *domain.PaneId
	if rec.MaximizedPaneID != nil {
		id, err := domain.NewPaneId(*rec.MaximizedPaneID)
		if err != nil {
			return nil, fmt.Errorf("invalid maximizedPaneId %q: %w", *rec.MaximizedPaneID, err)
		}
		maximized = &id
	}

	// Reconstitute the aggregate — this re-validates all invariants
	w, err := domain.ReconstituteWorkspace(wsID, wsName, layout, panes, lastActive, maximized)
	if err != nil {
		return nil, fmt.Errorf("cannot reconstitute workspace %q: %w", rec.ID, err)
	}

	return w, nil
}
