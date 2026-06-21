package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/domain"
)

func TestRestoreLayoutHandler_Handle_Success(t *testing.T) {
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

	// maximize pane first
	maximizeHandler := command.NewMaximizePaneHandler(repo)
	err = maximizeHandler.Handle(ctx, command.MaximizePaneCommand{
		WorkspaceID: wsResult.WorkspaceID,
		PaneID:      paneResult.PaneID,
	})
	if err != nil {
		t.Fatalf("maximize pane: %v", err)
	}

	// restore layout
	handler := command.NewRestoreLayoutHandler(repo)
	err = handler.Handle(ctx, command.RestoreLayoutCommand{
		WorkspaceID: wsResult.WorkspaceID,
	})
	if err != nil {
		t.Fatalf("restore layout: %v", err)
	}

	// verify MaximizedPaneId is cleared
	wsID, _ := domain.NewWorkspaceId(wsResult.WorkspaceID)
	w, err := repo.FindByID(ctx, wsID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	_, ok := w.MaximizedPaneId()
	if ok {
		t.Error("expected MaximizedPaneId to be cleared after RestoreLayout")
	}
	// 1 for create + 1 for add pane + 1 for maximize + 1 for restore
	if repo.SaveCallCount != 4 {
		t.Errorf("expected SaveCallCount 4, got %d", repo.SaveCallCount)
	}
	if repo.LastSavedID != wsResult.WorkspaceID {
		t.Errorf("expected LastSavedID %q, got %q", wsResult.WorkspaceID, repo.LastSavedID)
	}
}

// TestRestoreLayoutHandler_Handle_Idempotent は最大化されていないワークスペースに対して
// RestoreLayout を呼び出してもエラーが返らず、MaximizedPaneId が未設定のままであることを確認する。
func TestRestoreLayoutHandler_Handle_Idempotent(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1")

	// create workspace (no maximize)
	createWS := command.NewCreateWorkspaceHandler(repo, idgen)
	wsResult, err := createWS.Handle(ctx, command.CreateWorkspaceCommand{Name: "WS", Layout: "single"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	// restore layout without prior maximize
	handler := command.NewRestoreLayoutHandler(repo)
	err = handler.Handle(ctx, command.RestoreLayoutCommand{
		WorkspaceID: wsResult.WorkspaceID,
	})
	if err != nil {
		t.Fatalf("restore layout on non-maximized workspace: %v", err)
	}

	// verify MaximizedPaneId is still unset
	wsID, _ := domain.NewWorkspaceId(wsResult.WorkspaceID)
	w, err := repo.FindByID(ctx, wsID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	_, ok := w.MaximizedPaneId()
	if ok {
		t.Error("expected MaximizedPaneId to be unset")
	}
	// 1 for create + 1 for restore
	if repo.SaveCallCount != 2 {
		t.Errorf("expected SaveCallCount 2, got %d", repo.SaveCallCount)
	}
}

func TestRestoreLayoutHandler_Handle_WorkspaceNotFound(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()

	handler := command.NewRestoreLayoutHandler(repo)
	err := handler.Handle(ctx, command.RestoreLayoutCommand{
		WorkspaceID: "nonexistent",
	})
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}
