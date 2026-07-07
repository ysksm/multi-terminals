package command

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain"
)

// RunPaneGitOp の操作種別。
const (
	GitOpPull  = "pull"
	GitOpPush  = "push"
	GitOpFetch = "fetch"
)

// RunPaneGitOpCommand は pane の作業ディレクトリで pull/push/fetch を実行する
// コマンドの入力 DTO。
type RunPaneGitOpCommand struct {
	WorkspaceID string
	PaneID      string
	Op          string // GitOpPull | GitOpPush | GitOpFetch
}

// RunPaneGitOpHandler は pane のディレクトリで git のリモート操作を実行するハンドラ。
type RunPaneGitOpHandler struct {
	repo domain.WorkspaceRepository
	git  port.GitService
}

// NewRunPaneGitOpHandler は依存を注入して RunPaneGitOpHandler を返す。
func NewRunPaneGitOpHandler(repo domain.WorkspaceRepository, git port.GitService) *RunPaneGitOpHandler {
	return &RunPaneGitOpHandler{repo: repo, git: git}
}

// paneDirForGit は workspace/pane ID を検証して pane の作業ディレクトリを返す。
// git 系コマンドハンドラ共通の前処理。
func paneDirForGit(ctx context.Context, repo domain.WorkspaceRepository, workspaceID, paneID, opName string) (string, error) {
	wsID, err := domain.NewWorkspaceId(workspaceID)
	if err != nil {
		return "", apperr.Validation(fmt.Errorf("%s: invalid workspace id: %w", opName, err))
	}
	w, err := repo.FindByID(ctx, wsID)
	if err != nil {
		return "", err
	}
	pID, err := domain.NewPaneId(paneID)
	if err != nil {
		return "", apperr.Validation(fmt.Errorf("%s: invalid pane id: %w", opName, err))
	}
	for _, p := range w.Panes() {
		if p.ID().Equals(pID) {
			return p.Directory().String(), nil
		}
	}
	return "", apperr.Validation(fmt.Errorf("%s: pane not found: %s", opName, paneID))
}

// Handle は指定 pane のディレクトリで Op を実行する。git の失敗は
// ValidationError として返し、UI にそのまま表示する。
func (h *RunPaneGitOpHandler) Handle(ctx context.Context, cmd RunPaneGitOpCommand) error {
	dir, err := paneDirForGit(ctx, h.repo, cmd.WorkspaceID, cmd.PaneID, "run pane git op")
	if err != nil {
		return err
	}

	var opErr error
	switch cmd.Op {
	case GitOpPull:
		opErr = h.git.Pull(dir)
	case GitOpPush:
		opErr = h.git.Push(dir)
	case GitOpFetch:
		opErr = h.git.Fetch(dir)
	default:
		return apperr.Validation(fmt.Errorf("run pane git op: unknown op: %q", cmd.Op))
	}
	if opErr != nil {
		return apperr.Validation(fmt.Errorf("run pane git op: %s: %w", cmd.Op, opErr))
	}
	return nil
}
