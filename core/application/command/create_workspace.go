package command

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain"
)

// CreateWorkspaceCommand はワークスペース生成コマンドの入力 DTO。
type CreateWorkspaceCommand struct {
	Name   string
	Layout string
}

// CreateWorkspaceResult はワークスペース生成コマンドの結果。
type CreateWorkspaceResult struct {
	WorkspaceID string
}

// CreateWorkspaceHandler はワークスペース生成コマンドを処理するハンドラ。
type CreateWorkspaceHandler struct {
	repo  domain.WorkspaceRepository
	idgen port.IDGenerator
}

// NewCreateWorkspaceHandler は依存を注入して CreateWorkspaceHandler を返す。
func NewCreateWorkspaceHandler(repo domain.WorkspaceRepository, idgen port.IDGenerator) *CreateWorkspaceHandler {
	return &CreateWorkspaceHandler{repo: repo, idgen: idgen}
}

// Handle はワークスペースを生成してリポジトリに保存し、生成した ID を返す。
func (h *CreateWorkspaceHandler) Handle(ctx context.Context, cmd CreateWorkspaceCommand) (CreateWorkspaceResult, error) {
	rawID := h.idgen.NewID()
	id, err := domain.NewWorkspaceId(rawID)
	if err != nil {
		return CreateWorkspaceResult{}, apperr.Validation(fmt.Errorf("create workspace: invalid id: %w", err))
	}

	name, err := domain.NewWorkspaceName(cmd.Name)
	if err != nil {
		return CreateWorkspaceResult{}, apperr.Validation(fmt.Errorf("create workspace: invalid name: %w", err))
	}

	layout := domain.LayoutPreset(cmd.Layout)
	w, err := domain.NewWorkspace(id, name, layout)
	if err != nil {
		return CreateWorkspaceResult{}, apperr.Validation(fmt.Errorf("create workspace: %w", err))
	}

	if err := h.repo.Save(ctx, w); err != nil {
		return CreateWorkspaceResult{}, fmt.Errorf("create workspace: save: %w", err)
	}

	return CreateWorkspaceResult{WorkspaceID: id.String()}, nil
}
