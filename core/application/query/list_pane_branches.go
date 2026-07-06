package query

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain"
)

// PaneBranchDTO は pane の作業ディレクトリの 1 ブランチの読み取りモデル。
type PaneBranchDTO struct {
	Name      string `json:"name"`
	IsCurrent bool   `json:"isCurrent"`
	IsRemote  bool   `json:"isRemote"`
}

// ListPaneBranchesQuery は pane のブランチ一覧取得クエリの入力 DTO。
type ListPaneBranchesQuery struct {
	WorkspaceID string
	PaneID      string
}

// ListPaneBranchesHandler は pane の作業ディレクトリのブランチ一覧を返すハンドラ。
type ListPaneBranchesHandler struct {
	repo domain.WorkspaceRepository
	git  port.GitService
}

// NewListPaneBranchesHandler は依存を注入して ListPaneBranchesHandler を返す。
func NewListPaneBranchesHandler(repo domain.WorkspaceRepository, git port.GitService) *ListPaneBranchesHandler {
	return &ListPaneBranchesHandler{repo: repo, git: git}
}

// Handle は指定 pane のディレクトリのブランチ一覧を返す。
func (h *ListPaneBranchesHandler) Handle(ctx context.Context, q ListPaneBranchesQuery) ([]PaneBranchDTO, error) {
	wsID, err := domain.NewWorkspaceId(q.WorkspaceID)
	if err != nil {
		return nil, apperr.Validation(fmt.Errorf("list pane branches: invalid workspace id: %w", err))
	}

	w, err := h.repo.FindByID(ctx, wsID)
	if err != nil {
		return nil, err
	}

	paneID, err := domain.NewPaneId(q.PaneID)
	if err != nil {
		return nil, apperr.Validation(fmt.Errorf("list pane branches: invalid pane id: %w", err))
	}

	var dir string
	for _, p := range w.Panes() {
		if p.ID().Equals(paneID) {
			dir = p.Directory().String()
			break
		}
	}
	if dir == "" {
		return nil, apperr.Validation(fmt.Errorf("list pane branches: pane not found: %s", q.PaneID))
	}

	branches, err := h.git.Branches(dir)
	if err != nil {
		return nil, fmt.Errorf("list pane branches: %w", err)
	}
	dtos := make([]PaneBranchDTO, len(branches))
	for i, b := range branches {
		dtos[i] = PaneBranchDTO{Name: b.Name, IsCurrent: b.IsCurrent, IsRemote: b.IsRemote}
	}
	return dtos, nil
}
