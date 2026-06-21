package command

import (
	"context"
	"fmt"
	"sort"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/application/session"
	"github.com/ysksm/multi-terminals/core/domain"
)

// OpenWorkspaceCommand is the input DTO for opening (starting) a workspace.
type OpenWorkspaceCommand struct {
	WorkspaceID string
}

// OpenedPane holds the pane ID of a successfully started terminal session.
type OpenedPane struct {
	PaneID string
}

// OpenWorkspaceResult is the result of opening a workspace.
type OpenWorkspaceResult struct {
	Panes []OpenedPane
}

// OpenWorkspaceHandler handles the OpenWorkspaceCommand.
// It finds the workspace, starts a TerminalSession for each pane (ordered by slot),
// sends autoRun startup commands, and records the last-opened workspace.
type OpenWorkspaceHandler struct {
	repo     domain.WorkspaceRepository
	runner   port.TerminalRunner
	registry *session.Registry
	state    port.AppStateStore
	shell    string
	cols     uint16
	rows     uint16
}

// NewOpenWorkspaceHandler constructs an OpenWorkspaceHandler with default terminal size 80x24.
func NewOpenWorkspaceHandler(
	repo domain.WorkspaceRepository,
	runner port.TerminalRunner,
	registry *session.Registry,
	state port.AppStateStore,
	shell string,
) *OpenWorkspaceHandler {
	return &OpenWorkspaceHandler{
		repo:     repo,
		runner:   runner,
		registry: registry,
		state:    state,
		shell:    shell,
		cols:     80,
		rows:     24,
	}
}

// Handle opens the workspace by starting a terminal session for each pane.
// Panes are processed in ascending slot order.
// If an existing session for a pane is registered, it is closed before starting a new one.
// If starting any session fails, all sessions started in this call are closed and removed.
// On full success, state.SetLastOpened is called with the workspace ID.
func (h *OpenWorkspaceHandler) Handle(ctx context.Context, cmd OpenWorkspaceCommand) (OpenWorkspaceResult, error) {
	wsID, err := domain.NewWorkspaceId(cmd.WorkspaceID)
	if err != nil {
		return OpenWorkspaceResult{}, apperr.Validation(fmt.Errorf("open workspace: invalid workspace id: %w", err))
	}

	w, err := h.repo.FindByID(ctx, wsID)
	if err != nil {
		return OpenWorkspaceResult{}, err
	}

	// Sort panes by slot index (ascending).
	panes := w.Panes()
	sort.Slice(panes, func(i, j int) bool {
		return panes[i].Slot().Int() < panes[j].Slot().Int()
	})

	// Track sessions opened in this call for cleanup on failure.
	var opened []string

	for _, pane := range panes {
		paneID := pane.ID().String()

		// Close and remove any existing session for this pane.
		if existing, ok := h.registry.Get(paneID); ok {
			_ = existing.Close()
			h.registry.Remove(paneID)
		}

		// Start a new terminal session for this pane.
		req := port.TerminalStartRequest{
			SessionID: paneID,
			Dir:       pane.Directory().String(),
			Shell:     h.shell,
			Cols:      h.cols,
			Rows:      h.rows,
		}
		sess, err := h.runner.Start(ctx, req)
		if err != nil {
			// Clean up all sessions started in this call.
			for _, openedID := range opened {
				if s, ok := h.registry.Get(openedID); ok {
					_ = s.Close()
					h.registry.Remove(openedID)
				}
			}
			return OpenWorkspaceResult{}, fmt.Errorf("open workspace: start pane %s: %w", paneID, err)
		}

		h.registry.Add(paneID, sess)
		opened = append(opened, paneID)

		// A6.1: reap the session from the registry when its PTY exits naturally
		// (e.g. the user types "exit"). The goroutine captures the session and
		// paneID by value so it is not affected by loop variable reuse.
		go func(id string, s port.TerminalSession) {
			<-s.Done()
			h.registry.Remove(id)
		}(paneID, sess)

		// Send autoRun startup commands.
		for _, sc := range pane.Commands() {
			if sc.AutoRun() {
				if err := sess.Write([]byte(sc.Command() + "\n")); err != nil {
					// Non-fatal: log would go here in production; continue.
					_ = err
				}
			}
		}
	}

	// Record the last-opened workspace.
	if err := h.state.SetLastOpened(ctx, cmd.WorkspaceID); err != nil {
		// A5: clean up all sessions opened in this call so they are not leaked.
		for _, openedID := range opened {
			if s, ok := h.registry.Get(openedID); ok {
				_ = s.Close()
				h.registry.Remove(openedID)
			}
		}
		return OpenWorkspaceResult{}, fmt.Errorf("open workspace: set last opened: %w", err)
	}

	result := OpenWorkspaceResult{Panes: make([]OpenedPane, len(opened))}
	for i, id := range opened {
		result.Panes[i] = OpenedPane{PaneID: id}
	}
	return result, nil
}
