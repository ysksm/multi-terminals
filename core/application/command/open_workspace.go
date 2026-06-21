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
// If a live session for a pane is already registered, it is resumed (not restarted)
// and autoRun commands are not re-sent.
// If starting any new session fails, all sessions started in this call are closed and removed.
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

	// allPaneIDs collects all pane IDs (resumed + newly started) for the result.
	var allPaneIDs []string
	// newlyOpened tracks sessions started in this call so we can clean them up on failure.
	var newlyOpened []string

	for _, pane := range panes {
		paneID := pane.ID().String()

		// RESUME: if a live session already exists for this pane, do not restart it.
		if _, ok := h.registry.Get(paneID); ok {
			allPaneIDs = append(allPaneIDs, paneID)
			continue
		}

		// Start a new terminal session for this pane.
		req := port.TerminalStartRequest{
			SessionID: paneID,
			Dir:       pane.Directory().String(),
			Shell:     h.shell,
			Cols:      h.cols,
			Rows:      h.rows,
		}
		inner, err := h.runner.Start(ctx, req)
		if err != nil {
			// Clean up all sessions started in this call.
			for _, openedID := range newlyOpened {
				if s, ok := h.registry.Get(openedID); ok {
					_ = s.Close()
					h.registry.Remove(openedID)
				}
			}
			return OpenWorkspaceResult{}, fmt.Errorf("open workspace: start pane %s: %w", paneID, err)
		}

		hub := session.NewSession(inner)
		h.registry.Add(paneID, hub)
		newlyOpened = append(newlyOpened, paneID)
		allPaneIDs = append(allPaneIDs, paneID)

		// Reap the session from the registry when its PTY exits naturally.
		go func(id string, s *session.Session) {
			<-s.Done()
			h.registry.Remove(id)
		}(paneID, hub)

		// Send autoRun startup commands.
		for _, sc := range pane.Commands() {
			if sc.AutoRun() {
				if err := hub.Write([]byte(sc.Command() + "\n")); err != nil {
					_ = err
				}
			}
		}
	}

	// Record the last-opened workspace.
	if err := h.state.SetLastOpened(ctx, cmd.WorkspaceID); err != nil {
		// Clean up only the sessions opened in this call.
		for _, openedID := range newlyOpened {
			if s, ok := h.registry.Get(openedID); ok {
				_ = s.Close()
				h.registry.Remove(openedID)
			}
		}
		return OpenWorkspaceResult{}, fmt.Errorf("open workspace: set last opened: %w", err)
	}

	result := OpenWorkspaceResult{Panes: make([]OpenedPane, len(allPaneIDs))}
	for i, id := range allPaneIDs {
		result.Panes[i] = OpenedPane{PaneID: id}
	}
	return result, nil
}
