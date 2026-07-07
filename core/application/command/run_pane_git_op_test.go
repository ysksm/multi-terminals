package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
)

func TestRunPaneGitOp_AllOps(t *testing.T) {
	for _, op := range []string{command.GitOpPull, command.GitOpPush, command.GitOpFetch} {
		repo := apptest.NewFakeRepo()
		wsID, paneID := setupGitPane(t, repo)
		git := apptest.NewFakeGitService()

		h := command.NewRunPaneGitOpHandler(repo, git)
		err := h.Handle(context.Background(), command.RunPaneGitOpCommand{
			WorkspaceID: wsID, PaneID: paneID, Op: op,
		})
		if err != nil {
			t.Fatalf("handle(%s): %v", op, err)
		}
		if len(git.GitOps) != 1 || git.GitOps[0].Op != op || git.GitOps[0].Dir != "/tmp/project" {
			t.Errorf("GitOps(%s) = %+v", op, git.GitOps)
		}
	}
}

func TestRunPaneGitOp_UnknownOp(t *testing.T) {
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupGitPane(t, repo)
	git := apptest.NewFakeGitService()

	h := command.NewRunPaneGitOpHandler(repo, git)
	err := h.Handle(context.Background(), command.RunPaneGitOpCommand{
		WorkspaceID: wsID, PaneID: paneID, Op: "rebase",
	})
	var ve *apperr.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("expected ValidationError, got %v", err)
	}
	if len(git.GitOps) != 0 {
		t.Errorf("未知 op で git が呼ばれた: %+v", git.GitOps)
	}
}

func TestRunPaneGitOp_GitErrorIsValidation(t *testing.T) {
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupGitPane(t, repo)
	git := apptest.NewFakeGitService()
	git.OpErr = errors.New("git pull: 認証エラー")

	h := command.NewRunPaneGitOpHandler(repo, git)
	err := h.Handle(context.Background(), command.RunPaneGitOpCommand{
		WorkspaceID: wsID, PaneID: paneID, Op: command.GitOpPull,
	})
	var ve *apperr.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("expected ValidationError, got %v", err)
	}
}
