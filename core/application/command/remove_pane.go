package command

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/domain"
)

// RemovePaneCommand は pane 削除コマンドの入力 DTO。
type RemovePaneCommand struct {
	WorkspaceID string
	PaneID      string
}

// RemovePaneHandler は pane 削除コマンドを処理するハンドラ。
type RemovePaneHandler struct {
	repo domain.WorkspaceRepository
}

// NewRemovePaneHandler は依存を注入して RemovePaneHandler を返す。
func NewRemovePaneHandler(repo domain.WorkspaceRepository) *RemovePaneHandler {
	return &RemovePaneHandler{repo: repo}
}

// Handle は指定ワークスペースから pane を削除して保存する。
func (h *RemovePaneHandler) Handle(ctx context.Context, cmd RemovePaneCommand) error {
	wsID, err := domain.NewWorkspaceId(cmd.WorkspaceID)
	if err != nil {
		return fmt.Errorf("remove pane: invalid workspace id: %w", err)
	}

	w, err := h.repo.FindByID(ctx, wsID)
	if err != nil {
		return err
	}

	paneID, err := domain.NewPaneId(cmd.PaneID)
	if err != nil {
		return fmt.Errorf("remove pane: invalid pane id: %w", err)
	}

	if err := w.RemovePane(paneID); err != nil {
		return fmt.Errorf("remove pane: %w", err)
	}

	if err := h.repo.Save(ctx, w); err != nil {
		return fmt.Errorf("remove pane: save: %w", err)
	}

	return nil
}
