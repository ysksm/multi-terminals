package command

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain"
)

// StartupCommandInput はコマンドパッケージ共有の起動コマンド入力型。
type StartupCommandInput struct {
	Command string
	AutoRun bool
}

// AddPaneCommand は pane 追加コマンドの入力 DTO。
type AddPaneCommand struct {
	WorkspaceID string
	Directory   string
	Slot        int
	Commands    []StartupCommandInput
}

// AddPaneResult は pane 追加コマンドの結果。
type AddPaneResult struct {
	PaneID string
}

// AddPaneHandler は pane 追加コマンドを処理するハンドラ。
type AddPaneHandler struct {
	repo  domain.WorkspaceRepository
	idgen port.IDGenerator
}

// NewAddPaneHandler は依存を注入して AddPaneHandler を返す。
func NewAddPaneHandler(repo domain.WorkspaceRepository, idgen port.IDGenerator) *AddPaneHandler {
	return &AddPaneHandler{repo: repo, idgen: idgen}
}

// Handle は指定ワークスペースに pane を追加して保存し、生成した PaneID を返す。
func (h *AddPaneHandler) Handle(ctx context.Context, cmd AddPaneCommand) (AddPaneResult, error) {
	wsID, err := domain.NewWorkspaceId(cmd.WorkspaceID)
	if err != nil {
		return AddPaneResult{}, fmt.Errorf("add pane: invalid workspace id: %w", err)
	}

	w, err := h.repo.FindByID(ctx, wsID)
	if err != nil {
		return AddPaneResult{}, err
	}

	rawPaneID := h.idgen.NewID()
	paneID, err := domain.NewPaneId(rawPaneID)
	if err != nil {
		return AddPaneResult{}, fmt.Errorf("add pane: invalid pane id: %w", err)
	}

	dir, err := domain.NewDirectoryPath(cmd.Directory)
	if err != nil {
		return AddPaneResult{}, fmt.Errorf("add pane: invalid directory: %w", err)
	}

	slot, err := domain.NewSlotIndex(cmd.Slot)
	if err != nil {
		return AddPaneResult{}, fmt.Errorf("add pane: invalid slot: %w", err)
	}

	startupCmds := make([]domain.StartupCommand, 0, len(cmd.Commands))
	for _, c := range cmd.Commands {
		sc, err := domain.NewStartupCommand(c.Command, c.AutoRun)
		if err != nil {
			return AddPaneResult{}, fmt.Errorf("add pane: invalid startup command: %w", err)
		}
		startupCmds = append(startupCmds, sc)
	}

	pane, err := domain.NewPane(paneID, dir, slot, startupCmds)
	if err != nil {
		return AddPaneResult{}, fmt.Errorf("add pane: create pane: %w", err)
	}

	if err := w.AddPane(pane); err != nil {
		return AddPaneResult{}, fmt.Errorf("add pane: %w", err)
	}

	if err := h.repo.Save(ctx, w); err != nil {
		return AddPaneResult{}, fmt.Errorf("add pane: save: %w", err)
	}

	return AddPaneResult{PaneID: paneID.String()}, nil
}
