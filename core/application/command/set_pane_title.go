package command

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/domain"
)

// SetPaneTitleCommand は pane のタイトル変更コマンドの入力 DTO。
type SetPaneTitleCommand struct {
	WorkspaceID string
	PaneID      string
	Title       string
}

// SetPaneTitleHandler は pane タイトル変更コマンドを処理するハンドラ。
type SetPaneTitleHandler struct {
	repo domain.WorkspaceRepository
}

// NewSetPaneTitleHandler は依存を注入して SetPaneTitleHandler を返す。
func NewSetPaneTitleHandler(repo domain.WorkspaceRepository) *SetPaneTitleHandler {
	return &SetPaneTitleHandler{repo: repo}
}

// Handle は指定ワークスペースの指定 pane のタイトルを変更して保存する。
func (h *SetPaneTitleHandler) Handle(ctx context.Context, cmd SetPaneTitleCommand) error {
	wsID, err := domain.NewWorkspaceId(cmd.WorkspaceID)
	if err != nil {
		return apperr.Validation(fmt.Errorf("set pane title: invalid workspace id: %w", err))
	}

	w, err := h.repo.FindByID(ctx, wsID)
	if err != nil {
		return err
	}

	paneID, err := domain.NewPaneId(cmd.PaneID)
	if err != nil {
		return apperr.Validation(fmt.Errorf("set pane title: invalid pane id: %w", err))
	}

	title, err := domain.NewPaneTitle(cmd.Title)
	if err != nil {
		return apperr.Validation(fmt.Errorf("set pane title: invalid title: %w", err))
	}

	if err := w.SetPaneTitle(paneID, title); err != nil {
		return apperr.Validation(fmt.Errorf("set pane title: %w", err))
	}

	if err := h.repo.Save(ctx, w); err != nil {
		return fmt.Errorf("set pane title: save: %w", err)
	}

	return nil
}
