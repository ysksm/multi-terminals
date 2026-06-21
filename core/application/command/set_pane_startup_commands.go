package command

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/domain"
)

// SetPaneStartupCommandsCommand は pane の起動コマンド列変更コマンドの入力 DTO。
type SetPaneStartupCommandsCommand struct {
	WorkspaceID string
	PaneID      string
	Commands    []StartupCommandInput
}

// SetPaneStartupCommandsHandler は pane 起動コマンド変更コマンドを処理するハンドラ。
type SetPaneStartupCommandsHandler struct {
	repo domain.WorkspaceRepository
}

// NewSetPaneStartupCommandsHandler は依存を注入して SetPaneStartupCommandsHandler を返す。
func NewSetPaneStartupCommandsHandler(repo domain.WorkspaceRepository) *SetPaneStartupCommandsHandler {
	return &SetPaneStartupCommandsHandler{repo: repo}
}

// Handle は指定ワークスペースの指定 pane の起動コマンド列を置き換えて保存する。
func (h *SetPaneStartupCommandsHandler) Handle(ctx context.Context, cmd SetPaneStartupCommandsCommand) error {
	wsID, err := domain.NewWorkspaceId(cmd.WorkspaceID)
	if err != nil {
		return apperr.Validation(fmt.Errorf("set pane startup commands: invalid workspace id: %w", err))
	}

	w, err := h.repo.FindByID(ctx, wsID)
	if err != nil {
		return err
	}

	paneID, err := domain.NewPaneId(cmd.PaneID)
	if err != nil {
		return apperr.Validation(fmt.Errorf("set pane startup commands: invalid pane id: %w", err))
	}

	startupCmds := make([]domain.StartupCommand, 0, len(cmd.Commands))
	for _, c := range cmd.Commands {
		sc, err := domain.NewStartupCommand(c.Command, c.AutoRun)
		if err != nil {
			return apperr.Validation(fmt.Errorf("set pane startup commands: invalid startup command: %w", err))
		}
		startupCmds = append(startupCmds, sc)
	}

	if err := w.SetPaneStartupCommands(paneID, startupCmds); err != nil {
		return apperr.Validation(fmt.Errorf("set pane startup commands: %w", err))
	}

	if err := h.repo.Save(ctx, w); err != nil {
		return fmt.Errorf("set pane startup commands: save: %w", err)
	}

	return nil
}
