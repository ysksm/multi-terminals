package command

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/domain"
)

// ChangeLayoutCommand はワークスペースのレイアウト変更コマンドの入力 DTO。
type ChangeLayoutCommand struct {
	WorkspaceID string
	Layout      string
}

// ChangeLayoutHandler はワークスペースのレイアウト変更コマンドを処理するハンドラ。
type ChangeLayoutHandler struct {
	repo domain.WorkspaceRepository
}

// NewChangeLayoutHandler は依存を注入して ChangeLayoutHandler を返す。
func NewChangeLayoutHandler(repo domain.WorkspaceRepository) *ChangeLayoutHandler {
	return &ChangeLayoutHandler{repo: repo}
}

// Handle はワークスペースを取得してレイアウトを変更し、リポジトリに保存する。
func (h *ChangeLayoutHandler) Handle(ctx context.Context, cmd ChangeLayoutCommand) error {
	id, err := domain.NewWorkspaceId(cmd.WorkspaceID)
	if err != nil {
		return fmt.Errorf("change layout: invalid id: %w", err)
	}

	w, err := h.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}

	if err := w.ChangeLayout(domain.LayoutPreset(cmd.Layout)); err != nil {
		return fmt.Errorf("change layout: %w", err)
	}

	if err := h.repo.Save(ctx, w); err != nil {
		return fmt.Errorf("change layout: save: %w", err)
	}

	return nil
}
