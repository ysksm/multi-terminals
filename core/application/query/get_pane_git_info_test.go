package query_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/application/query"
	"github.com/ysksm/multi-terminals/core/domain"
)

func setupPane(t *testing.T, repo *apptest.FakeRepo, dir string) (wsID, paneID string) {
	t.Helper()
	ctx := context.Background()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")
	createWS := command.NewCreateWorkspaceHandler(repo, idgen)
	wsResult, err := createWS.Handle(ctx, command.CreateWorkspaceCommand{Name: "WS", Layout: "single"})
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

func TestGetPaneGitInfoHandler_Repo(t *testing.T) {
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupPane(t, repo, "/tmp/project")
	git := apptest.NewFakeGitService()
	git.Infos["/tmp/project"] = port.GitInfo{IsRepo: true, Branch: "main", Dirty: true}

	h := query.NewGetPaneGitInfoHandler(repo, git)
	dto, err := h.Handle(context.Background(), query.GetPaneGitInfoQuery{WorkspaceID: wsID, PaneID: paneID})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if !dto.IsRepo || dto.Branch != "main" || !dto.Dirty {
		t.Errorf("dto = %+v, want {IsRepo:true Branch:main Dirty:true}", dto)
	}
}

func TestGetPaneGitInfoHandler_NotARepo(t *testing.T) {
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupPane(t, repo, "/tmp/plain")
	git := apptest.NewFakeGitService()

	h := query.NewGetPaneGitInfoHandler(repo, git)
	dto, err := h.Handle(context.Background(), query.GetPaneGitInfoQuery{WorkspaceID: wsID, PaneID: paneID})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if dto.IsRepo {
		t.Errorf("dto = %+v, want IsRepo=false", dto)
	}
}

func TestGetPaneGitInfoHandler_WorkspaceNotFound(t *testing.T) {
	repo := apptest.NewFakeRepo()
	git := apptest.NewFakeGitService()
	h := query.NewGetPaneGitInfoHandler(repo, git)
	_, err := h.Handle(context.Background(), query.GetPaneGitInfoQuery{WorkspaceID: "nope", PaneID: "p"})
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestGetPaneGitInfoHandler_PaneNotFound(t *testing.T) {
	repo := apptest.NewFakeRepo()
	wsID, _ := setupPane(t, repo, "/tmp/project")
	git := apptest.NewFakeGitService()
	h := query.NewGetPaneGitInfoHandler(repo, git)
	_, err := h.Handle(context.Background(), query.GetPaneGitInfoQuery{WorkspaceID: wsID, PaneID: "nope"})
	if err == nil {
		t.Fatal("expected error for missing pane, got nil")
	}
}
