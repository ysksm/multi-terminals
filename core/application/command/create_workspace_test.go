package command_test

import (
	"context"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/domain"
)

func TestCreateWorkspaceHandler_Handle_Success(t *testing.T) {
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-001")
	h := command.NewCreateWorkspaceHandler(repo, idgen)

	result, err := h.Handle(context.Background(), command.CreateWorkspaceCommand{
		Name:   "My Workspace",
		Layout: "single",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.WorkspaceID != "ws-001" {
		t.Errorf("expected WorkspaceID %q, got %q", "ws-001", result.WorkspaceID)
	}

	// verify saved in repo using domain VO
	wsID, err := domain.NewWorkspaceId("ws-001")
	if err != nil {
		t.Fatalf("failed to create WorkspaceId: %v", err)
	}
	saved, err := repo.FindByID(context.Background(), wsID)
	if err != nil {
		t.Fatalf("expected workspace to be saved, got error: %v", err)
	}
	if saved.Name().String() != "My Workspace" {
		t.Errorf("expected name %q, got %q", "My Workspace", saved.Name().String())
	}
	if repo.SaveCallCount != 1 {
		t.Errorf("expected SaveCallCount 1, got %d", repo.SaveCallCount)
	}
	if repo.LastSavedID != "ws-001" {
		t.Errorf("expected LastSavedID %q, got %q", "ws-001", repo.LastSavedID)
	}
}

func TestCreateWorkspaceHandler_Handle_EmptyName(t *testing.T) {
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-001")
	h := command.NewCreateWorkspaceHandler(repo, idgen)

	_, err := h.Handle(context.Background(), command.CreateWorkspaceCommand{
		Name:   "",
		Layout: "single",
	})
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestCreateWorkspaceHandler_Handle_InvalidLayout(t *testing.T) {
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-001")
	h := command.NewCreateWorkspaceHandler(repo, idgen)

	_, err := h.Handle(context.Background(), command.CreateWorkspaceCommand{
		Name:   "My Workspace",
		Layout: "invalid_layout",
	})
	if err == nil {
		t.Fatal("expected error for invalid layout, got nil")
	}
}
