package command

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/session"
	"github.com/ysksm/multi-terminals/core/domain"
)

// DeleteWorkspaceCommand is the input DTO for deleting a workspace.
type DeleteWorkspaceCommand struct {
	WorkspaceID string
}

// DeleteWorkspaceHandler handles the DeleteWorkspaceCommand.
type DeleteWorkspaceHandler struct {
	repo     domain.WorkspaceRepository
	registry *session.Registry
}

// NewDeleteWorkspaceHandler constructs a DeleteWorkspaceHandler.
func NewDeleteWorkspaceHandler(repo domain.WorkspaceRepository, registry *session.Registry) *DeleteWorkspaceHandler {
	return &DeleteWorkspaceHandler{repo: repo, registry: registry}
}

// Handle closes all live PTY sessions for the workspace's panes, then deletes the workspace.
func (h *DeleteWorkspaceHandler) Handle(ctx context.Context, cmd DeleteWorkspaceCommand) error {
	id, err := domain.NewWorkspaceId(cmd.WorkspaceID)
	if err != nil {
		return apperr.Validation(fmt.Errorf("delete workspace: invalid workspace id: %w", err))
	}

	w, err := h.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}

	for _, pane := range w.Panes() {
		paneID := pane.ID().String()
		if sess, ok := h.registry.Get(paneID); ok {
			_ = sess.Close()
			h.registry.Remove(paneID)
		}
	}

	if err := h.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete workspace: %w", err)
	}

	return nil
}
