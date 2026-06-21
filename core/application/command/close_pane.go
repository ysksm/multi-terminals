package command

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/application/session"
)

// ClosePaneCommand is the input DTO for closing a terminal pane.
type ClosePaneCommand struct {
	PaneID string
}

// ClosePaneHandler handles the ClosePaneCommand.
type ClosePaneHandler struct {
	registry *session.Registry
}

// NewClosePaneHandler constructs a ClosePaneHandler.
func NewClosePaneHandler(registry *session.Registry) *ClosePaneHandler {
	return &ClosePaneHandler{registry: registry}
}

// Handle closes the terminal session identified by cmd.PaneID and removes it from the registry.
// Returns ErrSessionNotFound if no session is registered for that ID.
func (h *ClosePaneHandler) Handle(ctx context.Context, cmd ClosePaneCommand) error {
	sess, ok := h.registry.Get(cmd.PaneID)
	if !ok {
		return ErrSessionNotFound
	}
	if err := sess.Close(); err != nil {
		return fmt.Errorf("close pane %s: %w", cmd.PaneID, err)
	}
	h.registry.Remove(cmd.PaneID)
	return nil
}
