package command

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/domain"
)

// RenameWorkspaceCommand はワークスペースのリネームコマンドの入力 DTO。
type RenameWorkspaceCommand struct {
	WorkspaceID string
	Name        string
}

// RenameWorkspaceHandler はワークスペースのリネームコマンドを処理するハンドラ。
type RenameWorkspaceHandler struct {
	repo domain.WorkspaceRepository
}

// NewRenameWorkspaceHandler は依存を注入して RenameWorkspaceHandler を返す。
func NewRenameWorkspaceHandler(repo domain.WorkspaceRepository) *RenameWorkspaceHandler {
	return &RenameWorkspaceHandler{repo: repo}
}

// Handle はワークスペースを取得してリネームし、リポジトリに保存する。
func (h *RenameWorkspaceHandler) Handle(ctx context.Context, cmd RenameWorkspaceCommand) error {
	id, err := domain.NewWorkspaceId(cmd.WorkspaceID)
	if err != nil {
		return fmt.Errorf("rename workspace: invalid id: %w", err)
	}

	w, err := h.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}

	name, err := domain.NewWorkspaceName(cmd.Name)
	if err != nil {
		return fmt.Errorf("rename workspace: invalid name: %w", err)
	}

	w.Rename(name)

	if err := h.repo.Save(ctx, w); err != nil {
		return fmt.Errorf("rename workspace: save: %w", err)
	}

	return nil
}
