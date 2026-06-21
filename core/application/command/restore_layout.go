package command

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/domain"
)

// RestoreLayoutCommand はレイアウト復元コマンドの入力 DTO。
type RestoreLayoutCommand struct {
	WorkspaceID string
}

// RestoreLayoutHandler はレイアウト復元コマンドを処理するハンドラ。
type RestoreLayoutHandler struct {
	repo domain.WorkspaceRepository
}

// NewRestoreLayoutHandler は依存を注入して RestoreLayoutHandler を返す。
func NewRestoreLayoutHandler(repo domain.WorkspaceRepository) *RestoreLayoutHandler {
	return &RestoreLayoutHandler{repo: repo}
}

// Handle は指定ワークスペースの最大化状態を解除して保存する。
func (h *RestoreLayoutHandler) Handle(ctx context.Context, cmd RestoreLayoutCommand) error {
	wsID, err := domain.NewWorkspaceId(cmd.WorkspaceID)
	if err != nil {
		return apperr.Validation(fmt.Errorf("restore layout: invalid workspace id: %w", err))
	}

	w, err := h.repo.FindByID(ctx, wsID)
	if err != nil {
		return err
	}

	w.RestoreLayout()

	if err := h.repo.Save(ctx, w); err != nil {
		return fmt.Errorf("restore layout: save: %w", err)
	}

	return nil
}
