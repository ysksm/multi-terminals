package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain"
)

// CheckoutPaneBranchCommand は pane の作業ディレクトリのブランチ切替コマンドの入力 DTO。
type CheckoutPaneBranchCommand struct {
	WorkspaceID string
	PaneID      string
	Branch      string
}

// CheckoutPaneBranchHandler は pane のディレクトリでブランチを切り替えるハンドラ。
type CheckoutPaneBranchHandler struct {
	repo domain.WorkspaceRepository
	git  port.GitService
}

// NewCheckoutPaneBranchHandler は依存を注入して CheckoutPaneBranchHandler を返す。
func NewCheckoutPaneBranchHandler(repo domain.WorkspaceRepository, git port.GitService) *CheckoutPaneBranchHandler {
	return &CheckoutPaneBranchHandler{repo: repo, git: git}
}

// Handle は指定 pane のディレクトリで branch に切り替える。dirty で切り替え
// られない等の git の失敗は ValidationError として返し、UI にそのまま表示する。
func (h *CheckoutPaneBranchHandler) Handle(ctx context.Context, cmd CheckoutPaneBranchCommand) error {
	branch := strings.TrimSpace(cmd.Branch)
	if branch == "" {
		return apperr.Validation(fmt.Errorf("checkout pane branch: branch is required"))
	}

	dir, err := paneDirForGit(ctx, h.repo, cmd.WorkspaceID, cmd.PaneID, "checkout pane branch")
	if err != nil {
		return err
	}

	if err := h.git.Checkout(dir, branch); err != nil {
		return apperr.Validation(fmt.Errorf("checkout pane branch: %w", err))
	}
	return nil
}
