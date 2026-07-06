package query_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/application/query"
	"github.com/ysksm/multi-terminals/core/domain"
)

func TestListPaneBranchesHandler(t *testing.T) {
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupPane(t, repo, "/tmp/project")
	git := apptest.NewFakeGitService()
	git.BranchLists["/tmp/project"] = []port.BranchInfo{
		{Name: "main", IsCurrent: true},
		{Name: "feature", IsRemote: true},
	}

	h := query.NewListPaneBranchesHandler(repo, git)
	dtos, err := h.Handle(context.Background(), query.ListPaneBranchesQuery{WorkspaceID: wsID, PaneID: paneID})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(dtos) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(dtos), dtos)
	}
	if dtos[0].Name != "main" || !dtos[0].IsCurrent || dtos[0].IsRemote {
		t.Errorf("dtos[0] = %+v", dtos[0])
	}
	if dtos[1].Name != "feature" || dtos[1].IsCurrent || !dtos[1].IsRemote {
		t.Errorf("dtos[1] = %+v", dtos[1])
	}
}

func TestListPaneBranchesHandler_EmptyIsNotNil(t *testing.T) {
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupPane(t, repo, "/tmp/plain")
	git := apptest.NewFakeGitService()

	h := query.NewListPaneBranchesHandler(repo, git)
	dtos, err := h.Handle(context.Background(), query.ListPaneBranchesQuery{WorkspaceID: wsID, PaneID: paneID})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if dtos == nil {
		t.Error("dtos は nil でなく空 slice を返す(JSON で null にしない)")
	}
}

func TestListPaneBranchesHandler_WorkspaceNotFound(t *testing.T) {
	repo := apptest.NewFakeRepo()
	git := apptest.NewFakeGitService()
	h := query.NewListPaneBranchesHandler(repo, git)
	_, err := h.Handle(context.Background(), query.ListPaneBranchesQuery{WorkspaceID: "nope", PaneID: "p"})
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestListPaneBranchesHandler_PaneNotFound(t *testing.T) {
	repo := apptest.NewFakeRepo()
	wsID, _ := setupPane(t, repo, "/tmp/project")
	git := apptest.NewFakeGitService()
	h := query.NewListPaneBranchesHandler(repo, git)
	_, err := h.Handle(context.Background(), query.ListPaneBranchesQuery{WorkspaceID: wsID, PaneID: "nope"})
	if err == nil {
		t.Fatal("expected error for missing pane, got nil")
	}
}
