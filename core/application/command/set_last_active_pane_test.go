package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/domain"
)

func TestSetLastActivePaneHandler_Handle_Success(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")

	// create workspace
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

	// set last active pane
	handler := command.NewSetLastActivePaneHandler(repo)
	err = handler.Handle(ctx, command.SetLastActivePaneCommand{
		WorkspaceID: wsResult.WorkspaceID,
		PaneID:      paneResult.PaneID,
	})
	if err != nil {
		t.Fatalf("set last active pane: %v", err)
	}

	// verify via repo
	wsID, _ := domain.NewWorkspaceId(wsResult.WorkspaceID)
	w, err := repo.FindByID(ctx, wsID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	lastID, ok := w.LastActivePaneId()
	if !ok {
		t.Fatal("expected LastActivePaneId to be set")
	}
	if lastID.String() != paneResult.PaneID {
		t.Errorf("LastActivePaneId mismatch: got %q, want %q", lastID.String(), paneResult.PaneID)
	}
	// 1 for create + 1 for add pane + 1 for set last active pane
	if repo.SaveCallCount != 3 {
		t.Errorf("expected SaveCallCount 3, got %d", repo.SaveCallCount)
	}
	if repo.LastSavedID != wsResult.WorkspaceID {
		t.Errorf("expected LastSavedID %q, got %q", wsResult.WorkspaceID, repo.LastSavedID)
	}
}

func TestSetLastActivePaneHandler_Handle_WorkspaceNotFound(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()

	handler := command.NewSetLastActivePaneHandler(repo)
	err := handler.Handle(ctx, command.SetLastActivePaneCommand{
		WorkspaceID: "nonexistent",
		PaneID:      "pane-1",
	})
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestSetLastActivePaneHandler_Handle_PaneNotFound(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1")

	createWS := command.NewCreateWorkspaceHandler(repo, idgen)
	wsResult, err := createWS.Handle(ctx, command.CreateWorkspaceCommand{Name: "WS", Layout: "single"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	handler := command.NewSetLastActivePaneHandler(repo)
	err = handler.Handle(ctx, command.SetLastActivePaneCommand{
		WorkspaceID: wsResult.WorkspaceID,
		PaneID:      "no-such-pane",
	})
	if err == nil {
		t.Fatal("expected error for non-existent pane, got nil")
	}
}
