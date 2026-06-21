package command

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/domain"
)

// SetPaneDirectoryCommand は pane の作業ディレクトリ変更コマンドの入力 DTO。
type SetPaneDirectoryCommand struct {
	WorkspaceID string
	PaneID      string
	Directory   string
}

// SetPaneDirectoryHandler は pane ディレクトリ変更コマンドを処理するハンドラ。
type SetPaneDirectoryHandler struct {
	repo domain.WorkspaceRepository
}

// NewSetPaneDirectoryHandler は依存を注入して SetPaneDirectoryHandler を返す。
func NewSetPaneDirectoryHandler(repo domain.WorkspaceRepository) *SetPaneDirectoryHandler {
	return &SetPaneDirectoryHandler{repo: repo}
}

// Handle は指定ワークスペースの指定 pane の作業ディレクトリを変更して保存する。
func (h *SetPaneDirectoryHandler) Handle(ctx context.Context, cmd SetPaneDirectoryCommand) error {
	wsID, err := domain.NewWorkspaceId(cmd.WorkspaceID)
	if err != nil {
		return apperr.Validation(fmt.Errorf("set pane directory: invalid workspace id: %w", err))
	}

	w, err := h.repo.FindByID(ctx, wsID)
	if err != nil {
		return err
	}

	paneID, err := domain.NewPaneId(cmd.PaneID)
	if err != nil {
		return apperr.Validation(fmt.Errorf("set pane directory: invalid pane id: %w", err))
	}

	dir, err := domain.NewDirectoryPath(cmd.Directory)
	if err != nil {
		return apperr.Validation(fmt.Errorf("set pane directory: invalid directory: %w", err))
	}

	if err := w.SetPaneDirectory(paneID, dir); err != nil {
		return apperr.Validation(fmt.Errorf("set pane directory: %w", err))
	}

	if err := h.repo.Save(ctx, w); err != nil {
		return fmt.Errorf("set pane directory: save: %w", err)
	}

	return nil
}
