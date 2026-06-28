package command_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/application/session"
	"github.com/ysksm/multi-terminals/core/domain"
)

func TestDeleteWorkspaceHandler_Handle_Success(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")
	reg := session.NewRegistry()

	// Create workspace
	createWS := command.NewCreateWorkspaceHandler(repo, idgen)
	wsResult, err := createWS.Handle(ctx, command.CreateWorkspaceCommand{Name: "WS", Layout: "single"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	// Add a pane
	addPane := command.NewAddPaneHandler(repo, idgen)
	paneResult, err := addPane.Handle(ctx, command.AddPaneCommand{
		WorkspaceID: wsResult.WorkspaceID,
		Directory:   "/tmp",
		Slot:        0,
	})
	if err != nil {
		t.Fatalf("add pane: %v", err)
	}

	// Register a fake session for the pane
	fakeSess := apptest.NewFakeTerminalSession(paneResult.PaneID)
	hub := session.NewSession(fakeSess)
	reg.Add(paneResult.PaneID, hub)

	// Delete the workspace
	handler := command.NewDeleteWorkspaceHandler(repo, reg)
	err = handler.Handle(ctx, command.DeleteWorkspaceCommand{WorkspaceID: wsResult.WorkspaceID})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	// Workspace should no longer be in the repo
	wsID, _ := domain.NewWorkspaceId(wsResult.WorkspaceID)
	_, findErr := repo.FindByID(ctx, wsID)
	if !errors.Is(findErr, domain.ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound after delete, got %v", findErr)
	}

	// Session should be removed from the registry
	if _, ok := reg.Get(paneResult.PaneID); ok {
		t.Error("expected session to be removed from registry after workspace delete")
	}

	// Hub Done channel should be closed after Close propagates
	select {
	case <-hub.Done():
		// ok
	case <-time.After(3 * time.Second):
		t.Error("expected hub Done channel to be closed after session Close")
	}
}

func TestDeleteWorkspaceHandler_Handle_NoLiveSessions(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")
	reg := session.NewRegistry()

	// Create workspace + pane but register no session
	createWS := command.NewCreateWorkspaceHandler(repo, idgen)
	wsResult, err := createWS.Handle(ctx, command.CreateWorkspaceCommand{Name: "WS", Layout: "single"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	addPane := command.NewAddPaneHandler(repo, idgen)
	if _, err := addPane.Handle(ctx, command.AddPaneCommand{
		WorkspaceID: wsResult.WorkspaceID,
		Directory:   "/tmp",
		Slot:        0,
	}); err != nil {
		t.Fatalf("add pane: %v", err)
	}

	handler := command.NewDeleteWorkspaceHandler(repo, reg)
	if err := handler.Handle(ctx, command.DeleteWorkspaceCommand{WorkspaceID: wsResult.WorkspaceID}); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	wsID, _ := domain.NewWorkspaceId(wsResult.WorkspaceID)
	_, findErr := repo.FindByID(ctx, wsID)
	if !errors.Is(findErr, domain.ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound after delete, got %v", findErr)
	}
}

func TestDeleteWorkspaceHandler_Handle_InvalidID(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	reg := session.NewRegistry()

	handler := command.NewDeleteWorkspaceHandler(repo, reg)
	err := handler.Handle(ctx, command.DeleteWorkspaceCommand{WorkspaceID: ""})
	if err == nil {
		t.Fatal("expected validation error for empty workspace ID, got nil")
	}
	var ve *apperr.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestDeleteWorkspaceHandler_Handle_WorkspaceNotFound(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	reg := session.NewRegistry()

	handler := command.NewDeleteWorkspaceHandler(repo, reg)
	err := handler.Handle(ctx, command.DeleteWorkspaceCommand{WorkspaceID: "nonexistent"})
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}
