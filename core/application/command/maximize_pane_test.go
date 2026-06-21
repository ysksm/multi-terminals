package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/domain"
)

func TestMaximizePaneHandler_Handle_Success(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")

	// create workspace with split_vertical (capacity 2)
	createWS := command.NewCreateWorkspaceHandler(repo, idgen)
	wsResult, err := createWS.Handle(ctx, command.CreateWorkspaceCommand{Name: "WS", Layout: "split_vertical"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	// add pane
	addPane := command.NewAddPaneHandler(repo, idgen)
	paneResult, err := addPane.Handle(ctx, command.AddPaneCommand{
		WorkspaceID: wsResult.WorkspaceID,
		Directory:   "/tmp",
		Slot:        0,
	})
	if err != nil {
		t.Fatalf("add pane: %v", err)
	}

	// maximize pane
	handler := command.NewMaximizePaneHandler(repo)
	err = handler.Handle(ctx, command.MaximizePaneCommand{
		WorkspaceID: wsResult.WorkspaceID,
		PaneID:      paneResult.PaneID,
	})
	if err != nil {
		t.Fatalf("maximize pane: %v", err)
	}

	// verify via repo
	wsID, _ := domain.NewWorkspaceId(wsResult.WorkspaceID)
	w, err := repo.FindByID(ctx, wsID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	maxID, ok := w.MaximizedPaneId()
	if !ok {
		t.Fatal("expected MaximizedPaneId to be set")
	}
	if maxID.String() != paneResult.PaneID {
		t.Errorf("MaximizedPaneId mismatch: got %q, want %q", maxID.String(), paneResult.PaneID)
	}
}

func TestMaximizePaneHandler_Handle_WorkspaceNotFound(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()

	handler := command.NewMaximizePaneHandler(repo)
	err := handler.Handle(ctx, command.MaximizePaneCommand{
		WorkspaceID: "nonexistent",
		PaneID:      "pane-1",
	})
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestMaximizePaneHandler_Handle_PaneNotFound(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1")

	createWS := command.NewCreateWorkspaceHandler(repo, idgen)
	wsResult, err := createWS.Handle(ctx, command.CreateWorkspaceCommand{Name: "WS", Layout: "single"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	handler := command.NewMaximizePaneHandler(repo)
	err = handler.Handle(ctx, command.MaximizePaneCommand{
		WorkspaceID: wsResult.WorkspaceID,
		PaneID:      "no-such-pane",
	})
	if err == nil {
		t.Fatal("expected error for non-existent pane, got nil")
	}
}
