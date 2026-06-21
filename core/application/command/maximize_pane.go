package command

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/domain"
)

// MaximizePaneCommand は pane 最大化コマンドの入力 DTO。
type MaximizePaneCommand struct {
	WorkspaceID string
	PaneID      string
}

// MaximizePaneHandler は pane 最大化コマンドを処理するハンドラ。
type MaximizePaneHandler struct {
	repo domain.WorkspaceRepository
}

// NewMaximizePaneHandler は依存を注入して MaximizePaneHandler を返す。
func NewMaximizePaneHandler(repo domain.WorkspaceRepository) *MaximizePaneHandler {
	return &MaximizePaneHandler{repo: repo}
}

// Handle は指定ワークスペースの指定 pane を最大化して保存する。
func (h *MaximizePaneHandler) Handle(ctx context.Context, cmd MaximizePaneCommand) error {
	wsID, err := domain.NewWorkspaceId(cmd.WorkspaceID)
	if err != nil {
		return fmt.Errorf("maximize pane: invalid workspace id: %w", err)
	}

	w, err := h.repo.FindByID(ctx, wsID)
	if err != nil {
		return err
	}

	paneID, err := domain.NewPaneId(cmd.PaneID)
	if err != nil {
		return fmt.Errorf("maximize pane: invalid pane id: %w", err)
	}

	if err := w.MaximizePane(paneID); err != nil {
		return fmt.Errorf("maximize pane: %w", err)
	}

	if err := h.repo.Save(ctx, w); err != nil {
		return fmt.Errorf("maximize pane: save: %w", err)
	}

	return nil
}
