package query

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/domain"
)

// GetWorkspaceQuery はワークスペース取得クエリの入力 DTO。
type GetWorkspaceQuery struct {
	WorkspaceID string
}

// GetWorkspaceHandler はワークスペース取得クエリを処理するハンドラ。
type GetWorkspaceHandler struct {
	repo domain.WorkspaceRepository
}

// NewGetWorkspaceHandler は依存を注入して GetWorkspaceHandler を返す。
func NewGetWorkspaceHandler(repo domain.WorkspaceRepository) *GetWorkspaceHandler {
	return &GetWorkspaceHandler{repo: repo}
}

// Handle は指定した WorkspaceID のワークスペースを DTO として返す。
// 未存在のとき domain.ErrWorkspaceNotFound を返す。
func (h *GetWorkspaceHandler) Handle(ctx context.Context, q GetWorkspaceQuery) (WorkspaceDTO, error) {
	wsID, err := domain.NewWorkspaceId(q.WorkspaceID)
	if err != nil {
		return WorkspaceDTO{}, fmt.Errorf("get workspace: invalid id: %w", err)
	}

	w, err := h.repo.FindByID(ctx, wsID)
	if err != nil {
		return WorkspaceDTO{}, err
	}

	return toWorkspaceDTO(w), nil
}
