package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/application/session"
)

func TestResizePaneHandler_Handle_Success(t *testing.T) {
	ctx := context.Background()
	reg := session.NewRegistry()

	fakeSess := apptest.NewFakeTerminalSession("pane-1")
	hub := session.NewSession(fakeSess)
	reg.Add("pane-1", hub)

	handler := command.NewResizePaneHandler(reg)
	err := handler.Handle(ctx, command.ResizePaneCommand{
		PaneID: "pane-1",
		Cols:   120,
		Rows:   40,
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// Verify the resize was applied to the inner fake session.
	if fakeSess.LastCols != 120 {
		t.Errorf("expected LastCols=120, got %d", fakeSess.LastCols)
	}
	if fakeSess.LastRows != 40 {
		t.Errorf("expected LastRows=40, got %d", fakeSess.LastRows)
	}
}

func TestResizePaneHandler_Handle_SessionNotFound(t *testing.T) {
	ctx := context.Background()
	reg := session.NewRegistry()

	handler := command.NewResizePaneHandler(reg)
	err := handler.Handle(ctx, command.ResizePaneCommand{
		PaneID: "nonexistent",
		Cols:   80,
		Rows:   24,
	})
	if !errors.Is(err, command.ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}
