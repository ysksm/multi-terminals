package command

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/domain"
)

// SetLastActivePaneCommand は最後にアクティブだった pane 設定コマンドの入力 DTO。
type SetLastActivePaneCommand struct {
	WorkspaceID string
	PaneID      string
}

// SetLastActivePaneHandler は最後にアクティブだった pane 設定コマンドを処理するハンドラ。
type SetLastActivePaneHandler struct {
	repo domain.WorkspaceRepository
}

// NewSetLastActivePaneHandler は依存を注入して SetLastActivePaneHandler を返す。
func NewSetLastActivePaneHandler(repo domain.WorkspaceRepository) *SetLastActivePaneHandler {
	return &SetLastActivePaneHandler{repo: repo}
}

// Handle は指定ワークスペースの最後にアクティブだった pane を設定して保存する。
func (h *SetLastActivePaneHandler) Handle(ctx context.Context, cmd SetLastActivePaneCommand) error {
	wsID, err := domain.NewWorkspaceId(cmd.WorkspaceID)
	if err != nil {
		return fmt.Errorf("set last active pane: invalid workspace id: %w", err)
	}

	w, err := h.repo.FindByID(ctx, wsID)
	if err != nil {
		return err
	}

	paneID, err := domain.NewPaneId(cmd.PaneID)
	if err != nil {
		return fmt.Errorf("set last active pane: invalid pane id: %w", err)
	}

	if err := w.SetLastActivePane(paneID); err != nil {
		return fmt.Errorf("set last active pane: %w", err)
	}

	if err := h.repo.Save(ctx, w); err != nil {
		return fmt.Errorf("set last active pane: save: %w", err)
	}

	return nil
}
