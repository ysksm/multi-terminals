package command

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/domain"
)

// SetPaneRemoteHostCommand は pane のリモート実行ホスト変更コマンドの入力 DTO。
// RemoteHost は空でローカル実行に戻す。
type SetPaneRemoteHostCommand struct {
	WorkspaceID string
	PaneID      string
	RemoteHost  string
}

// SetPaneRemoteHostHandler は pane リモートホスト変更コマンドを処理するハンドラ。
type SetPaneRemoteHostHandler struct {
	repo domain.WorkspaceRepository
}

// NewSetPaneRemoteHostHandler は依存を注入して SetPaneRemoteHostHandler を返す。
func NewSetPaneRemoteHostHandler(repo domain.WorkspaceRepository) *SetPaneRemoteHostHandler {
	return &SetPaneRemoteHostHandler{repo: repo}
}

// Handle は指定ワークスペースの指定 pane のリモート実行ホストを変更して保存する。
func (h *SetPaneRemoteHostHandler) Handle(ctx context.Context, cmd SetPaneRemoteHostCommand) error {
	wsID, err := domain.NewWorkspaceId(cmd.WorkspaceID)
	if err != nil {
		return apperr.Validation(fmt.Errorf("set pane remote host: invalid workspace id: %w", err))
	}

	w, err := h.repo.FindByID(ctx, wsID)
	if err != nil {
		return err
	}

	paneID, err := domain.NewPaneId(cmd.PaneID)
	if err != nil {
		return apperr.Validation(fmt.Errorf("set pane remote host: invalid pane id: %w", err))
	}

	host, err := domain.NewRemoteHost(cmd.RemoteHost)
	if err != nil {
		return apperr.Validation(fmt.Errorf("set pane remote host: invalid remote host: %w", err))
	}

	if err := w.SetPaneRemoteHost(paneID, host); err != nil {
		return apperr.Validation(fmt.Errorf("set pane remote host: %w", err))
	}

	if err := h.repo.Save(ctx, w); err != nil {
		return fmt.Errorf("set pane remote host: save: %w", err)
	}

	return nil
}
