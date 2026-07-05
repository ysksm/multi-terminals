package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/domain"
)

// setupWorkspaceWithPane はワークスペース1つと pane 1つを作成して ID を返す。
func setupWorkspaceWithPane(t *testing.T, repo *apptest.FakeRepo, dir string) (wsID, paneID string) {
	t.Helper()
	ctx := context.Background()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")

	createWS := command.NewCreateWorkspaceHandler(repo, idgen)
	wsResult, err := createWS.Handle(ctx, command.CreateWorkspaceCommand{Name: "WS", Layout: "split_vertical"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	addPane := command.NewAddPaneHandler(repo, idgen)
	paneResult, err := addPane.Handle(ctx, command.AddPaneCommand{
		WorkspaceID: wsResult.WorkspaceID,
		Directory:   dir,
		Slot:        0,
	})
	if err != nil {
		t.Fatalf("add pane: %v", err)
	}
	return wsResult.WorkspaceID, paneResult.PaneID
}

func TestOpenPaneInHandler_Handle_Finder(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupWorkspaceWithPane(t, repo, "/tmp/project")
	opener := apptest.NewFakeDirectoryOpener()

	handler := command.NewOpenPaneInHandler(repo, opener, apptest.NewFakeGitService())
	err := handler.Handle(ctx, command.OpenPaneInCommand{
		WorkspaceID: wsID,
		PaneID:      paneID,
		Target:      command.OpenTargetFinder,
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got := opener.RevealedDirs; len(got) != 1 || got[0] != "/tmp/project" {
		t.Errorf("RevealedDirs = %v, want [/tmp/project]", got)
	}
	if len(opener.EditorDirs) != 0 {
		t.Errorf("EditorDirs = %v, want empty", opener.EditorDirs)
	}
}

func TestOpenPaneInHandler_Handle_VSCode(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupWorkspaceWithPane(t, repo, "/tmp/project")
	opener := apptest.NewFakeDirectoryOpener()

	handler := command.NewOpenPaneInHandler(repo, opener, apptest.NewFakeGitService())
	err := handler.Handle(ctx, command.OpenPaneInCommand{
		WorkspaceID: wsID,
		PaneID:      paneID,
		Target:      command.OpenTargetVSCode,
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got := opener.EditorDirs; len(got) != 1 || got[0] != "/tmp/project" {
		t.Errorf("EditorDirs = %v, want [/tmp/project]", got)
	}
	if len(opener.RevealedDirs) != 0 {
		t.Errorf("RevealedDirs = %v, want empty", opener.RevealedDirs)
	}
}

func TestOpenPaneInHandler_Handle_GitHub(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupWorkspaceWithPane(t, repo, "/tmp/project")
	opener := apptest.NewFakeDirectoryOpener()
	git := apptest.NewFakeGitService()
	git.Remotes["/tmp/project"] = "git@github.com:user/repo.git"

	handler := command.NewOpenPaneInHandler(repo, opener, git)
	err := handler.Handle(ctx, command.OpenPaneInCommand{
		WorkspaceID: wsID,
		PaneID:      paneID,
		Target:      command.OpenTargetGitHub,
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got := opener.OpenedURLs; len(got) != 1 || got[0] != "https://github.com/user/repo" {
		t.Errorf("OpenedURLs = %v, want [https://github.com/user/repo]", got)
	}
}

func TestOpenPaneInHandler_Handle_GitHub_NoRemote(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupWorkspaceWithPane(t, repo, "/tmp/project")
	opener := apptest.NewFakeDirectoryOpener()

	handler := command.NewOpenPaneInHandler(repo, opener, apptest.NewFakeGitService())
	err := handler.Handle(ctx, command.OpenPaneInCommand{
		WorkspaceID: wsID,
		PaneID:      paneID,
		Target:      command.OpenTargetGitHub,
	})
	if err == nil {
		t.Fatal("expected error when remote is missing, got nil")
	}
	if len(opener.OpenedURLs) != 0 {
		t.Errorf("OpenedURLs = %v, want empty", opener.OpenedURLs)
	}
}

func TestRemoteWebURL(t *testing.T) {
	tests := []struct {
		remote string
		want   string
		ok     bool
	}{
		{"git@github.com:user/repo.git", "https://github.com/user/repo", true},
		{"https://github.com/user/repo.git", "https://github.com/user/repo", true},
		{"https://github.com/user/repo", "https://github.com/user/repo", true},
		{"ssh://git@github.com/user/repo.git", "https://github.com/user/repo", true},
		{"", "", false},
		{"/local/path/repo", "", false},
	}
	for _, tt := range tests {
		got, err := command.RemoteWebURL(tt.remote)
		if tt.ok && (err != nil || got != tt.want) {
			t.Errorf("RemoteWebURL(%q) = %q, %v; want %q", tt.remote, got, err, tt.want)
		}
		if !tt.ok && err == nil {
			t.Errorf("RemoteWebURL(%q) expected error, got %q", tt.remote, got)
		}
	}
}

func TestOpenPaneInHandler_Handle_UnknownTarget(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupWorkspaceWithPane(t, repo, "/tmp/project")
	opener := apptest.NewFakeDirectoryOpener()

	handler := command.NewOpenPaneInHandler(repo, opener, apptest.NewFakeGitService())
	err := handler.Handle(ctx, command.OpenPaneInCommand{
		WorkspaceID: wsID,
		PaneID:      paneID,
		Target:      "sublime",
	})
	if err == nil {
		t.Fatal("expected error for unknown target, got nil")
	}
}

func TestOpenPaneInHandler_Handle_WorkspaceNotFound(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	opener := apptest.NewFakeDirectoryOpener()

	handler := command.NewOpenPaneInHandler(repo, opener, apptest.NewFakeGitService())
	err := handler.Handle(ctx, command.OpenPaneInCommand{
		WorkspaceID: "nonexistent",
		PaneID:      "pane-1",
		Target:      command.OpenTargetFinder,
	})
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestOpenPaneInHandler_Handle_PaneNotFound(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	wsID, _ := setupWorkspaceWithPane(t, repo, "/tmp/project")
	opener := apptest.NewFakeDirectoryOpener()

	handler := command.NewOpenPaneInHandler(repo, opener, apptest.NewFakeGitService())
	err := handler.Handle(ctx, command.OpenPaneInCommand{
		WorkspaceID: wsID,
		PaneID:      "no-such-pane",
		Target:      command.OpenTargetFinder,
	})
	if err == nil {
		t.Fatal("expected error for non-existent pane, got nil")
	}
}

func TestOpenPaneInHandler_Handle_OpenerError(t *testing.T) {
	ctx := context.Background()
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupWorkspaceWithPane(t, repo, "/tmp/project")
	opener := apptest.NewFakeDirectoryOpener()
	opener.Err = errors.New("app not installed")

	handler := command.NewOpenPaneInHandler(repo, opener, apptest.NewFakeGitService())
	err := handler.Handle(ctx, command.OpenPaneInCommand{
		WorkspaceID: wsID,
		PaneID:      paneID,
		Target:      command.OpenTargetVSCode,
	})
	if err == nil || !errors.Is(err, opener.Err) {
		t.Errorf("expected opener error to propagate, got %v", err)
	}
}
