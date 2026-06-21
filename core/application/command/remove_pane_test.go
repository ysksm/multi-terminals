package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/domain"
)

func TestRemovePaneHandler_Handle_Success(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")

	// workspace を作成
	createHandler := command.NewCreateWorkspaceHandler(repo, idgen)
	wsResult, err := createHandler.Handle(ctx, command.CreateWorkspaceCommand{Name: "Test", Layout: "split_vertical"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	// pane を追加
	addHandler := command.NewAddPaneHandler(repo, idgen)
	addResult, err := addHandler.Handle(ctx, command.AddPaneCommand{
		WorkspaceID: wsResult.WorkspaceID,
		Directory:   "/tmp",
		Slot:        0,
	})
	if err != nil {
		t.Fatalf("add pane: %v", err)
	}

	// pane を削除
	removeHandler := command.NewRemovePaneHandler(repo)
	err = removeHandler.Handle(ctx, command.RemovePaneCommand{
		WorkspaceID: wsResult.WorkspaceID,
		PaneID:      addResult.PaneID,
	})
	if err != nil {
		t.Fatalf("remove pane: %v", err)
	}

	// repo から取得して pane が削除されているか確認
	wsID, err := domain.NewWorkspaceId(wsResult.WorkspaceID)
	if err != nil {
		t.Fatalf("NewWorkspaceId: %v", err)
	}
	w, err := repo.FindByID(ctx, wsID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if len(w.Panes()) != 0 {
		t.Errorf("expected 0 panes after removal, got %d", len(w.Panes()))
	}
	// 1 for create + 1 for add pane + 1 for remove pane
	if repo.SaveCallCount != 3 {
		t.Errorf("expected SaveCallCount 3, got %d", repo.SaveCallCount)
	}
	if repo.LastSavedID != wsResult.WorkspaceID {
		t.Errorf("expected LastSavedID %q, got %q", wsResult.WorkspaceID, repo.LastSavedID)
	}
}

func TestRemovePaneHandler_Handle_WorkspaceNotFound(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()

	handler := command.NewRemovePaneHandler(repo)
	err := handler.Handle(ctx, command.RemovePaneCommand{
		WorkspaceID: "nonexistent",
		PaneID:      "pane-1",
	})
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestRemovePaneHandler_Handle_PaneNotFound(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1")

	// workspace を作成（pane は追加しない）
	createHandler := command.NewCreateWorkspaceHandler(repo, idgen)
	wsResult, err := createHandler.Handle(ctx, command.CreateWorkspaceCommand{Name: "Test", Layout: "split_vertical"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	handler := command.NewRemovePaneHandler(repo)
	err = handler.Handle(ctx, command.RemovePaneCommand{
		WorkspaceID: wsResult.WorkspaceID,
		PaneID:      "pane-nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent pane, got nil")
	}
}
