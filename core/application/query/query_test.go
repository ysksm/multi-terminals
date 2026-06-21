package query_test

import (
	"context"
	"sort"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/application/query"
	"github.com/ysksm/multi-terminals/core/domain"
)

// helper: ワークスペースを FakeRepo に直接保存するユーティリティ
func mustSaveWorkspace(t *testing.T, repo domain.WorkspaceRepository, w *domain.Workspace) {
	t.Helper()
	if err := repo.Save(context.Background(), w); err != nil {
		t.Fatalf("save workspace: %v", err)
	}
}

// helper: シンプルなワークスペースを生成して保存する
func mustCreateWorkspace(t *testing.T, repo domain.WorkspaceRepository, idgen interface{ NewID() string }, name, layout string) string {
	t.Helper()
	createHandler := command.NewCreateWorkspaceHandler(repo, idgen)
	result, err := createHandler.Handle(context.Background(), command.CreateWorkspaceCommand{
		Name:   name,
		Layout: layout,
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	return result.WorkspaceID
}

// TestGetWorkspaceHandler_Found は存在するワークスペースを取得できることを確認する。
func TestGetWorkspaceHandler_Found(t *testing.T) {
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1")
	wsID := mustCreateWorkspace(t, repo, idgen, "My WS", "single")

	h := query.NewGetWorkspaceHandler(repo)
	dto, err := h.Handle(context.Background(), query.GetWorkspaceQuery{WorkspaceID: wsID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if dto.ID != wsID {
		t.Errorf("ID: got %q, want %q", dto.ID, wsID)
	}
	if dto.Name != "My WS" {
		t.Errorf("Name: got %q, want %q", dto.Name, "My WS")
	}
	if dto.Layout != "single" {
		t.Errorf("Layout: got %q, want %q", dto.Layout, "single")
	}
	if len(dto.Panes) != 0 {
		t.Errorf("Panes: got %d, want 0", len(dto.Panes))
	}
	if dto.LastActivePaneID != nil {
		t.Errorf("LastActivePaneID: got %v, want nil", dto.LastActivePaneID)
	}
	if dto.MaximizedPaneID != nil {
		t.Errorf("MaximizedPaneID: got %v, want nil", dto.MaximizedPaneID)
	}
}

// TestGetWorkspaceHandler_NotFound は存在しないワークスペースで ErrWorkspaceNotFound が返ることを確認する。
func TestGetWorkspaceHandler_NotFound(t *testing.T) {
	repo := apptest.NewFakeRepo()
	h := query.NewGetWorkspaceHandler(repo)
	_, err := h.Handle(context.Background(), query.GetWorkspaceQuery{WorkspaceID: "non-existent"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestGetWorkspaceHandler_InvalidID は無効な ID でエラーが返ることを確認する。
func TestGetWorkspaceHandler_InvalidID(t *testing.T) {
	repo := apptest.NewFakeRepo()
	h := query.NewGetWorkspaceHandler(repo)
	_, err := h.Handle(context.Background(), query.GetWorkspaceQuery{WorkspaceID: ""})
	if err == nil {
		t.Fatal("expected error for empty workspace id")
	}
}

// TestListWorkspacesHandler_Empty は空のリポジトリで空スライスが返ることを確認する。
func TestListWorkspacesHandler_Empty(t *testing.T) {
	repo := apptest.NewFakeRepo()
	h := query.NewListWorkspacesHandler(repo)
	dtos, err := h.Handle(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dtos == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(dtos) != 0 {
		t.Errorf("expected 0 DTOs, got %d", len(dtos))
	}
}

// TestListWorkspacesHandler_Multiple は複数ワークスペースが全て返ることを確認する。
func TestListWorkspacesHandler_Multiple(t *testing.T) {
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "ws-2")
	mustCreateWorkspace(t, repo, idgen, "Workspace A", "single")
	mustCreateWorkspace(t, repo, idgen, "Workspace B", "split_vertical")

	h := query.NewListWorkspacesHandler(repo)
	dtos, err := h.Handle(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dtos) != 2 {
		t.Errorf("expected 2 DTOs, got %d", len(dtos))
	}
}

// TestToWorkspaceDTO_PaneOrder は pane が SlotIndex 昇順で並ぶことを確認する。
func TestToWorkspaceDTO_PaneOrder(t *testing.T) {
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-a", "pane-b")
	wsID := mustCreateWorkspace(t, repo, idgen, "WS", "split_vertical")

	// slot 1 を先に追加、その後 slot 0 を追加
	addPaneHandler := command.NewAddPaneHandler(repo, idgen)
	_, err := addPaneHandler.Handle(context.Background(), command.AddPaneCommand{
		WorkspaceID: wsID,
		Directory:   "/tmp",
		Slot:        1,
	})
	if err != nil {
		t.Fatalf("add pane slot 1: %v", err)
	}
	_, err = addPaneHandler.Handle(context.Background(), command.AddPaneCommand{
		WorkspaceID: wsID,
		Directory:   "/home",
		Slot:        0,
	})
	if err != nil {
		t.Fatalf("add pane slot 0: %v", err)
	}

	h := query.NewGetWorkspaceHandler(repo)
	dto, err := h.Handle(context.Background(), query.GetWorkspaceQuery{WorkspaceID: wsID})
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}

	if len(dto.Panes) != 2 {
		t.Fatalf("expected 2 panes, got %d", len(dto.Panes))
	}
	// Slot 昇順で並んでいることを確認
	if !sort.SliceIsSorted(dto.Panes, func(i, j int) bool {
		return dto.Panes[i].Slot < dto.Panes[j].Slot
	}) {
		t.Errorf("panes not sorted by slot: %+v", dto.Panes)
	}
	if dto.Panes[0].Slot != 0 {
		t.Errorf("first pane slot: got %d, want 0", dto.Panes[0].Slot)
	}
	if dto.Panes[1].Slot != 1 {
		t.Errorf("second pane slot: got %d, want 1", dto.Panes[1].Slot)
	}
}

// TestToWorkspaceDTO_WithLastActiveAndMaximized は LastActivePaneID / MaximizedPaneID が正しく変換されることを確認する。
func TestToWorkspaceDTO_WithLastActiveAndMaximized(t *testing.T) {
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")
	wsID := mustCreateWorkspace(t, repo, idgen, "WS", "single")

	addPaneHandler := command.NewAddPaneHandler(repo, idgen)
	addResult, err := addPaneHandler.Handle(context.Background(), command.AddPaneCommand{
		WorkspaceID: wsID,
		Directory:   "/tmp",
		Slot:        0,
	})
	if err != nil {
		t.Fatalf("add pane: %v", err)
	}
	paneID := addResult.PaneID

	// SetLastActivePane
	setLastActiveHandler := command.NewSetLastActivePaneHandler(repo)
	if err := setLastActiveHandler.Handle(context.Background(), command.SetLastActivePaneCommand{
		WorkspaceID: wsID,
		PaneID:      paneID,
	}); err != nil {
		t.Fatalf("set last active pane: %v", err)
	}

	// MaximizePane
	maximizePaneHandler := command.NewMaximizePaneHandler(repo)
	if err := maximizePaneHandler.Handle(context.Background(), command.MaximizePaneCommand{
		WorkspaceID: wsID,
		PaneID:      paneID,
	}); err != nil {
		t.Fatalf("maximize pane: %v", err)
	}

	h := query.NewGetWorkspaceHandler(repo)
	dto, err := h.Handle(context.Background(), query.GetWorkspaceQuery{WorkspaceID: wsID})
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}

	if dto.LastActivePaneID == nil {
		t.Error("LastActivePaneID: expected non-nil")
	} else if *dto.LastActivePaneID != paneID {
		t.Errorf("LastActivePaneID: got %q, want %q", *dto.LastActivePaneID, paneID)
	}

	if dto.MaximizedPaneID == nil {
		t.Error("MaximizedPaneID: expected non-nil")
	} else if *dto.MaximizedPaneID != paneID {
		t.Errorf("MaximizedPaneID: got %q, want %q", *dto.MaximizedPaneID, paneID)
	}
}

// TestToWorkspaceDTO_PaneFields は PaneDTO のフィールドが正しく変換されることを確認する。
func TestToWorkspaceDTO_PaneFields(t *testing.T) {
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")
	wsID := mustCreateWorkspace(t, repo, idgen, "WS", "single")

	addPaneHandler := command.NewAddPaneHandler(repo, idgen)
	addResult, err := addPaneHandler.Handle(context.Background(), command.AddPaneCommand{
		WorkspaceID: wsID,
		Directory:   "/usr/local",
		Slot:        0,
		Commands: []command.StartupCommandInput{
			{Command: "echo hello", AutoRun: true},
			{Command: "ls -la", AutoRun: false},
		},
	})
	if err != nil {
		t.Fatalf("add pane: %v", err)
	}

	h := query.NewGetWorkspaceHandler(repo)
	dto, err := h.Handle(context.Background(), query.GetWorkspaceQuery{WorkspaceID: wsID})
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}

	if len(dto.Panes) != 1 {
		t.Fatalf("expected 1 pane, got %d", len(dto.Panes))
	}

	pane := dto.Panes[0]
	if pane.ID != addResult.PaneID {
		t.Errorf("PaneDTO.ID: got %q, want %q", pane.ID, addResult.PaneID)
	}
	if pane.Directory != "/usr/local" {
		t.Errorf("PaneDTO.Directory: got %q, want %q", pane.Directory, "/usr/local")
	}
	if pane.Slot != 0 {
		t.Errorf("PaneDTO.Slot: got %d, want 0", pane.Slot)
	}
	if len(pane.Commands) != 2 {
		t.Fatalf("PaneDTO.Commands: got %d, want 2", len(pane.Commands))
	}
	if pane.Commands[0].Command != "echo hello" || !pane.Commands[0].AutoRun {
		t.Errorf("Commands[0]: got %+v", pane.Commands[0])
	}
	if pane.Commands[1].Command != "ls -la" || pane.Commands[1].AutoRun {
		t.Errorf("Commands[1]: got %+v", pane.Commands[1])
	}
}
