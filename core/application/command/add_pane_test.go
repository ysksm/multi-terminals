package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/domain"
)

func TestAddPaneHandler_Handle_Success(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")

	// 先に workspace を作成して保存
	createHandler := command.NewCreateWorkspaceHandler(repo, idgen)
	result, err := createHandler.Handle(ctx, command.CreateWorkspaceCommand{Name: "Test", Layout: "split_vertical"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	wsID := result.WorkspaceID

	addHandler := command.NewAddPaneHandler(repo, idgen)
	addResult, err := addHandler.Handle(ctx, command.AddPaneCommand{
		WorkspaceID: wsID,
		Directory:   "/tmp",
		Slot:        1,
		Commands:    []command.StartupCommandInput{{Command: "echo hello", AutoRun: true}},
	})
	if err != nil {
		t.Fatalf("add pane: %v", err)
	}
	if addResult.PaneID == "" {
		t.Fatal("expected non-empty PaneID")
	}

	// repo から取得して pane が反映されているか確認
	wsID2, err := domain.NewWorkspaceId(wsID)
	if err != nil {
		t.Fatalf("NewWorkspaceId: %v", err)
	}
	w, err := repo.FindByID(ctx, wsID2)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	panes := w.Panes()
	if len(panes) != 1 {
		t.Fatalf("expected 1 pane, got %d", len(panes))
	}
	if panes[0].ID().String() != addResult.PaneID {
		t.Errorf("pane ID mismatch: got %q, want %q", panes[0].ID().String(), addResult.PaneID)
	}
}

func TestAddPaneHandler_Handle_WorkspaceNotFound(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen()

	handler := command.NewAddPaneHandler(repo, idgen)
	_, err := handler.Handle(ctx, command.AddPaneCommand{
		WorkspaceID: "nonexistent",
		Directory:   "/tmp",
		Slot:        0,
	})
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestAddPaneHandler_Handle_CapacityExceeded(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")

	// single layout: capacity 1
	createHandler := command.NewCreateWorkspaceHandler(repo, idgen)
	result, err := createHandler.Handle(ctx, command.CreateWorkspaceCommand{Name: "Test", Layout: "single"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	// pane-1 を追加
	addHandler := command.NewAddPaneHandler(repo, idgen)
	_, err = addHandler.Handle(ctx, command.AddPaneCommand{
		WorkspaceID: result.WorkspaceID,
		Directory:   "/tmp",
		Slot:        0,
	})
	if err != nil {
		t.Fatalf("first add pane: %v", err)
	}

	// 2 つ目は容量超過
	_, err = addHandler.Handle(ctx, command.AddPaneCommand{
		WorkspaceID: result.WorkspaceID,
		Directory:   "/tmp",
		Slot:        1,
	})
	if err == nil {
		t.Fatal("expected capacity exceeded error, got nil")
	}
}

func TestAddPaneHandler_Handle_InvalidSlot(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")

	createHandler := command.NewCreateWorkspaceHandler(repo, idgen)
	result, err := createHandler.Handle(ctx, command.CreateWorkspaceCommand{Name: "Test", Layout: "split_vertical"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	addHandler := command.NewAddPaneHandler(repo, idgen)
	_, err = addHandler.Handle(ctx, command.AddPaneCommand{
		WorkspaceID: result.WorkspaceID,
		Directory:   "/tmp",
		Slot:        -1, // negative slot
	})
	if err == nil {
		t.Fatal("expected error for invalid slot, got nil")
	}
}

func TestAddPaneHandler_Handle_InvalidDirectory(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")

	createHandler := command.NewCreateWorkspaceHandler(repo, idgen)
	result, err := createHandler.Handle(ctx, command.CreateWorkspaceCommand{Name: "Test", Layout: "split_vertical"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	addHandler := command.NewAddPaneHandler(repo, idgen)
	_, err = addHandler.Handle(ctx, command.AddPaneCommand{
		WorkspaceID: result.WorkspaceID,
		Directory:   "", // empty directory
		Slot:        0,
	})
	if err == nil {
		t.Fatal("expected error for invalid directory, got nil")
	}
}
