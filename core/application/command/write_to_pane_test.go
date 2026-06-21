package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/application/session"
)

func TestWriteToPaneHandler_Handle_Success(t *testing.T) {
	ctx := context.Background()
	reg := session.NewRegistry()

	fakeSess := apptest.NewFakeTerminalSession("pane-1")
	reg.Add("pane-1", fakeSess)

	handler := command.NewWriteToPaneHandler(reg)
	err := handler.Handle(ctx, command.WriteToPaneCommand{
		PaneID: "pane-1",
		Data:   []byte("hello\n"),
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// Verify the data was written to the session.
	writes := fakeSess.Writes
	if len(writes) != 1 {
		t.Fatalf("expected 1 write, got %d", len(writes))
	}
	if string(writes[0]) != "hello\n" {
		t.Errorf("expected written data %q, got %q", "hello\n", string(writes[0]))
	}
}

func TestWriteToPaneHandler_Handle_SessionNotFound(t *testing.T) {
	ctx := context.Background()
	reg := session.NewRegistry()

	handler := command.NewWriteToPaneHandler(reg)
	err := handler.Handle(ctx, command.WriteToPaneCommand{
		PaneID: "nonexistent",
		Data:   []byte("data"),
	})
	if !errors.Is(err, command.ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}
