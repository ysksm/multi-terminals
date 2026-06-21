package command

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/application/session"
)

// ResizePaneCommand is the input DTO for resizing a terminal pane.
type ResizePaneCommand struct {
	PaneID string
	Cols   uint16
	Rows   uint16
}

// ResizePaneHandler handles the ResizePaneCommand.
type ResizePaneHandler struct {
	registry *session.Registry
}

// NewResizePaneHandler constructs a ResizePaneHandler.
func NewResizePaneHandler(registry *session.Registry) *ResizePaneHandler {
	return &ResizePaneHandler{registry: registry}
}

// Handle resizes the terminal session identified by cmd.PaneID.
// Returns ErrSessionNotFound if no session is registered for that ID.
func (h *ResizePaneHandler) Handle(ctx context.Context, cmd ResizePaneCommand) error {
	sess, ok := h.registry.Get(cmd.PaneID)
	if !ok {
		return ErrSessionNotFound
	}
	if err := sess.Resize(cmd.Cols, cmd.Rows); err != nil {
		return fmt.Errorf("resize pane %s: %w", cmd.PaneID, err)
	}
	return nil
}
