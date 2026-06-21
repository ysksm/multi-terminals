package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/application/session"
)

func TestClosePaneHandler_Handle_Success(t *testing.T) {
	ctx := context.Background()
	reg := session.NewRegistry()

	fakeSess := apptest.NewFakeTerminalSession("pane-1")
	reg.Add("pane-1", fakeSess)

	handler := command.NewClosePaneHandler(reg)
	err := handler.Handle(ctx, command.ClosePaneCommand{PaneID: "pane-1"})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// Session should be removed from registry.
	if _, ok := reg.Get("pane-1"); ok {
		t.Error("expected session to be removed from registry after close")
	}

	// Session's Done channel should be closed.
	select {
	case <-fakeSess.Done():
		// ok
	default:
		t.Error("expected session Done channel to be closed")
	}
}

func TestClosePaneHandler_Handle_SessionNotFound(t *testing.T) {
	ctx := context.Background()
	reg := session.NewRegistry()

	handler := command.NewClosePaneHandler(reg)
	err := handler.Handle(ctx, command.ClosePaneCommand{PaneID: "nonexistent"})
	if !errors.Is(err, command.ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}
