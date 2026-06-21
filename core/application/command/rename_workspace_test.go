package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/domain"
)

func TestRenameWorkspaceHandler_Handle_Success(t *testing.T) {
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-001")
	createH := command.NewCreateWorkspaceHandler(repo, idgen)
	_, err := createH.Handle(context.Background(), command.CreateWorkspaceCommand{
		Name:   "Original Name",
		Layout: "single",
	})
	if err != nil {
		t.Fatalf("setup: create workspace: %v", err)
	}

	h := command.NewRenameWorkspaceHandler(repo)
	err = h.Handle(context.Background(), command.RenameWorkspaceCommand{
		WorkspaceID: "ws-001",
		Name:        "New Name",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// verify saved state
	wsID, _ := domain.NewWorkspaceId("ws-001")
	saved, err := repo.FindByID(context.Background(), wsID)
	if err != nil {
		t.Fatalf("expected workspace in repo: %v", err)
	}
	if saved.Name().String() != "New Name" {
		t.Errorf("expected name %q, got %q", "New Name", saved.Name().String())
	}
	// 1 for create + 1 for rename
	if repo.SaveCallCount != 2 {
		t.Errorf("expected SaveCallCount 2, got %d", repo.SaveCallCount)
	}
	if repo.LastSavedID != "ws-001" {
		t.Errorf("expected LastSavedID %q, got %q", "ws-001", repo.LastSavedID)
	}
}

func TestRenameWorkspaceHandler_Handle_NotFound(t *testing.T) {
	repo := apptest.NewFakeRepo()
	h := command.NewRenameWorkspaceHandler(repo)
	err := h.Handle(context.Background(), command.RenameWorkspaceCommand{
		WorkspaceID: "nonexistent",
		Name:        "Whatever",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent workspace, got nil")
	}
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestRenameWorkspaceHandler_Handle_InvalidName(t *testing.T) {
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-001")
	createH := command.NewCreateWorkspaceHandler(repo, idgen)
	_, err := createH.Handle(context.Background(), command.CreateWorkspaceCommand{
		Name:   "Original Name",
		Layout: "single",
	})
	if err != nil {
		t.Fatalf("setup: create workspace: %v", err)
	}

	h := command.NewRenameWorkspaceHandler(repo)
	err = h.Handle(context.Background(), command.RenameWorkspaceCommand{
		WorkspaceID: "ws-001",
		Name:        "",
	})
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}
