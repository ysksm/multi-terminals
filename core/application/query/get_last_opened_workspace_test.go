package query_test

import (
	"context"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/application/query"
	"github.com/ysksm/multi-terminals/core/domain"
)

// TestGetLastOpenedWorkspaceHandler_NotSet は state が未設定のとき ok=false を返すことを確認する。
func TestGetLastOpenedWorkspaceHandler_NotSet(t *testing.T) {
	state := apptest.NewFakeAppStateStore()
	repo := apptest.NewFakeRepo()
	h := query.NewGetLastOpenedWorkspaceHandler(state, repo)

	dto, ok, err := h.Handle(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Errorf("expected ok=false when no last opened workspace, got ok=true with dto=%+v", dto)
	}
}

// TestGetLastOpenedWorkspaceHandler_Found は設定済みの workspace が DTO として返ることを確認する。
func TestGetLastOpenedWorkspaceHandler_Found(t *testing.T) {
	state := apptest.NewFakeAppStateStore()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1")

	// Create a workspace
	createHandler := command.NewCreateWorkspaceHandler(repo, idgen)
	result, err := createHandler.Handle(context.Background(), command.CreateWorkspaceCommand{
		Name:   "My WS",
		Layout: "single",
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	wsID := result.WorkspaceID

	// Record it as last opened
	if err := state.SetLastOpened(context.Background(), wsID); err != nil {
		t.Fatalf("SetLastOpened: %v", err)
	}

	h := query.NewGetLastOpenedWorkspaceHandler(state, repo)
	dto, ok, err := h.Handle(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true after SetLastOpened, got false")
	}
	if dto.ID != wsID {
		t.Errorf("dto.ID: got %q, want %q", dto.ID, wsID)
	}
	if dto.Name != "My WS" {
		t.Errorf("dto.Name: got %q, want %q", dto.Name, "My WS")
	}
	if dto.Layout != "single" {
		t.Errorf("dto.Layout: got %q, want %q", dto.Layout, "single")
	}
}

// TestGetLastOpenedWorkspaceHandler_DeletedWorkspace は参照先が削除済みのとき ok=false を返すことを確認する。
func TestGetLastOpenedWorkspaceHandler_DeletedWorkspace(t *testing.T) {
	state := apptest.NewFakeAppStateStore()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-deleted")

	// Create workspace
	createHandler := command.NewCreateWorkspaceHandler(repo, idgen)
	result, err := createHandler.Handle(context.Background(), command.CreateWorkspaceCommand{
		Name:   "To Be Deleted",
		Layout: "single",
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	wsID := result.WorkspaceID

	// Record it as last opened
	if err := state.SetLastOpened(context.Background(), wsID); err != nil {
		t.Fatalf("SetLastOpened: %v", err)
	}

	// Delete the workspace directly via repo
	wsid, err := domain.NewWorkspaceId(wsID)
	if err != nil {
		t.Fatalf("NewWorkspaceId: %v", err)
	}
	if err := repo.Delete(context.Background(), wsid); err != nil {
		t.Fatalf("repo.Delete: %v", err)
	}

	h := query.NewGetLastOpenedWorkspaceHandler(state, repo)
	dto, ok, err := h.Handle(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Errorf("expected ok=false when referenced workspace is deleted, got ok=true with dto=%+v", dto)
	}
}
