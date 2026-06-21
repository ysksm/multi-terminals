package command

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/application/session"
)

// WriteToPaneCommand is the input DTO for writing data to a terminal pane.
type WriteToPaneCommand struct {
	PaneID string
	Data   []byte
}

// WriteToPaneHandler handles the WriteToPaneCommand.
type WriteToPaneHandler struct {
	registry *session.Registry
}

// NewWriteToPaneHandler constructs a WriteToPaneHandler.
func NewWriteToPaneHandler(registry *session.Registry) *WriteToPaneHandler {
	return &WriteToPaneHandler{registry: registry}
}

// Handle writes data to the terminal session identified by cmd.PaneID.
// Returns ErrSessionNotFound if no session is registered for that ID.
func (h *WriteToPaneHandler) Handle(ctx context.Context, cmd WriteToPaneCommand) error {
	sess, ok := h.registry.Get(cmd.PaneID)
	if !ok {
		return ErrSessionNotFound
	}
	if err := sess.Write(cmd.Data); err != nil {
		return fmt.Errorf("write to pane %s: %w", cmd.PaneID, err)
	}
	return nil
}
