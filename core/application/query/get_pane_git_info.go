package query

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain"
)

// PaneGitInfoDTO は pane の作業ディレクトリの git 状態の読み取りモデル。
type PaneGitInfoDTO struct {
	IsRepo bool   `json:"isRepo"`
	Branch string `json:"branch"`
	Dirty  bool   `json:"dirty"`
}

// GetPaneGitInfoQuery は pane の git 情報取得クエリの入力 DTO。
type GetPaneGitInfoQuery struct {
	WorkspaceID string
	PaneID      string
}

// GetPaneGitInfoHandler は pane の作業ディレクトリの git 情報を返すハンドラ。
type GetPaneGitInfoHandler struct {
	repo domain.WorkspaceRepository
	git  port.GitService
}

// NewGetPaneGitInfoHandler は依存を注入して GetPaneGitInfoHandler を返す。
func NewGetPaneGitInfoHandler(repo domain.WorkspaceRepository, git port.GitService) *GetPaneGitInfoHandler {
	return &GetPaneGitInfoHandler{repo: repo, git: git}
}

// Handle は指定 pane のディレクトリの git 状態を返す。
func (h *GetPaneGitInfoHandler) Handle(ctx context.Context, q GetPaneGitInfoQuery) (PaneGitInfoDTO, error) {
	wsID, err := domain.NewWorkspaceId(q.WorkspaceID)
	if err != nil {
		return PaneGitInfoDTO{}, apperr.Validation(fmt.Errorf("get pane git info: invalid workspace id: %w", err))
	}

	w, err := h.repo.FindByID(ctx, wsID)
	if err != nil {
		return PaneGitInfoDTO{}, err
	}

	paneID, err := domain.NewPaneId(q.PaneID)
	if err != nil {
		return PaneGitInfoDTO{}, apperr.Validation(fmt.Errorf("get pane git info: invalid pane id: %w", err))
	}

	var dir string
	for _, p := range w.Panes() {
		if p.ID().Equals(paneID) {
			dir = p.Directory().String()
			break
		}
	}
	if dir == "" {
		return PaneGitInfoDTO{}, apperr.Validation(fmt.Errorf("get pane git info: pane not found: %s", q.PaneID))
	}

	info, err := h.git.Info(dir)
	if err != nil {
		return PaneGitInfoDTO{}, fmt.Errorf("get pane git info: %w", err)
	}
	return PaneGitInfoDTO{IsRepo: info.IsRepo, Branch: info.Branch, Dirty: info.Dirty}, nil
}
