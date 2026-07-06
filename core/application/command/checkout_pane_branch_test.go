package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
)

// setupGitPane はワークスペース + pane(/tmp/project)を作って ID を返す。
func setupGitPane(t *testing.T, repo *apptest.FakeRepo) (wsID, paneID string) {
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
		Directory:   "/tmp/project",
		Slot:        0,
	})
	if err != nil {
		t.Fatalf("add pane: %v", err)
	}
	return wsResult.WorkspaceID, paneResult.PaneID
}

func TestCheckoutPaneBranch(t *testing.T) {
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupGitPane(t, repo)
	git := apptest.NewFakeGitService()

	h := command.NewCheckoutPaneBranchHandler(repo, git)
	err := h.Handle(context.Background(), command.CheckoutPaneBranchCommand{
		WorkspaceID: wsID, PaneID: paneID, Branch: "feature",
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(git.Checkouts) != 1 || git.Checkouts[0].Dir != "/tmp/project" || git.Checkouts[0].Branch != "feature" {
		t.Errorf("Checkouts = %+v", git.Checkouts)
	}
}

func TestCheckoutPaneBranch_EmptyBranch(t *testing.T) {
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupGitPane(t, repo)
	git := apptest.NewFakeGitService()

	h := command.NewCheckoutPaneBranchHandler(repo, git)
	err := h.Handle(context.Background(), command.CheckoutPaneBranchCommand{
		WorkspaceID: wsID, PaneID: paneID, Branch: "  ",
	})
	var ve *apperr.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("expected ValidationError, got %v", err)
	}
}

func TestCheckoutPaneBranch_GitErrorIsValidation(t *testing.T) {
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupGitPane(t, repo)
	git := apptest.NewFakeGitService()
	git.CheckoutErr = errors.New("gitcli: switch: 未コミットの変更で失敗")

	h := command.NewCheckoutPaneBranchHandler(repo, git)
	err := h.Handle(context.Background(), command.CheckoutPaneBranchCommand{
		WorkspaceID: wsID, PaneID: paneID, Branch: "feature",
	})
	var ve *apperr.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("expected ValidationError (400 で UI にメッセージ表示), got %v", err)
	}
}
