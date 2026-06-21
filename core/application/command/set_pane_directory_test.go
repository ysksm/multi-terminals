package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/domain"
)

func TestSetPaneDirectoryHandler_Handle_Success(t *testing.T) {
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

	// set pane directory
	handler := command.NewSetPaneDirectoryHandler(repo)
	err = handler.Handle(ctx, command.SetPaneDirectoryCommand{
		WorkspaceID: wsResult.WorkspaceID,
		PaneID:      paneResult.PaneID,
		Directory:   "/home/user",
	})
	if err != nil {
		t.Fatalf("set pane directory: %v", err)
	}

	// verify via repo
	wsID, _ := domain.NewWorkspaceId(wsResult.WorkspaceID)
	w, err := repo.FindByID(ctx, wsID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	panes := w.Panes()
	if len(panes) != 1 {
		t.Fatalf("expected 1 pane, got %d", len(panes))
	}
	if panes[0].Directory().String() != "/home/user" {
		t.Errorf("directory mismatch: got %q, want %q", panes[0].Directory().String(), "/home/user")
	}
}

func TestSetPaneDirectoryHandler_Handle_WorkspaceNotFound(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()

	handler := command.NewSetPaneDirectoryHandler(repo)
	err := handler.Handle(ctx, command.SetPaneDirectoryCommand{
		WorkspaceID: "nonexistent",
		PaneID:      "pane-1",
		Directory:   "/tmp",
	})
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestSetPaneDirectoryHandler_Handle_PaneNotFound(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1")

	createWS := command.NewCreateWorkspaceHandler(repo, idgen)
	wsResult, err := createWS.Handle(ctx, command.CreateWorkspaceCommand{Name: "WS", Layout: "single"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	handler := command.NewSetPaneDirectoryHandler(repo)
	err = handler.Handle(ctx, command.SetPaneDirectoryCommand{
		WorkspaceID: wsResult.WorkspaceID,
		PaneID:      "no-such-pane",
		Directory:   "/tmp",
	})
	if err == nil {
		t.Fatal("expected error for non-existent pane, got nil")
	}
}

func TestSetPaneDirectoryHandler_Handle_InvalidDirectory(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")

	createWS := command.NewCreateWorkspaceHandler(repo, idgen)
	wsResult, err := createWS.Handle(ctx, command.CreateWorkspaceCommand{Name: "WS", Layout: "split_vertical"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	addPane := command.NewAddPaneHandler(repo, idgen)
	paneResult, err := addPane.Handle(ctx, command.AddPaneCommand{
		WorkspaceID: wsResult.WorkspaceID,
		Directory:   "/tmp",
		Slot:        0,
	})
	if err != nil {
		t.Fatalf("add pane: %v", err)
	}

	handler := command.NewSetPaneDirectoryHandler(repo)
	err = handler.Handle(ctx, command.SetPaneDirectoryCommand{
		WorkspaceID: wsResult.WorkspaceID,
		PaneID:      paneResult.PaneID,
		Directory:   "", // invalid
	})
	if err == nil {
		t.Fatal("expected error for empty directory, got nil")
	}
}
