package query

import (
	"context"
	"fmt"
	"sort"

	"github.com/ysksm/multi-terminals/core/domain"
)

// ListWorkspacesHandler はワークスペース一覧クエリを処理するハンドラ。
type ListWorkspacesHandler struct {
	repo domain.WorkspaceRepository
}

// NewListWorkspacesHandler は依存を注入して ListWorkspacesHandler を返す。
func NewListWorkspacesHandler(repo domain.WorkspaceRepository) *ListWorkspacesHandler {
	return &ListWorkspacesHandler{repo: repo}
}

// Handle は全ワークスペースを DTO のスライスとして返す。空のときは空スライス（nil でなく）を返す。
func (h *ListWorkspacesHandler) Handle(ctx context.Context) ([]WorkspaceDTO, error) {
	workspaces, err := h.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}

	dtos := make([]WorkspaceDTO, len(workspaces))
	for i, w := range workspaces {
		dtos[i] = toWorkspaceDTO(w)
	}

	sort.Slice(dtos, func(i, j int) bool {
		return dtos[i].ID < dtos[j].ID
	})

	return dtos, nil
}
