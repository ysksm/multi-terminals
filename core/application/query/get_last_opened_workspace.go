package query

import (
	"context"
	"errors"
	"fmt"

	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain"
)

// GetLastOpenedWorkspaceHandler は直前に開いていた workspace を照会するクエリハンドラ。
type GetLastOpenedWorkspaceHandler struct {
	state port.AppStateStore
	repo  domain.WorkspaceRepository
}

// NewGetLastOpenedWorkspaceHandler は依存を注入して GetLastOpenedWorkspaceHandler を返す。
func NewGetLastOpenedWorkspaceHandler(state port.AppStateStore, repo domain.WorkspaceRepository) *GetLastOpenedWorkspaceHandler {
	return &GetLastOpenedWorkspaceHandler{state: state, repo: repo}
}

// Handle は直前に開いていた workspace の DTO を返す。
//
// - state が未記録（ok=false）: (WorkspaceDTO{}, false, nil)
// - state が記録済みで workspace が存在する: (dto, true, nil)
// - state が記録済みだが workspace が削除済み（ErrWorkspaceNotFound）: (WorkspaceDTO{}, false, nil)
//   壊れた参照は「前回なし」として扱う。
func (h *GetLastOpenedWorkspaceHandler) Handle(ctx context.Context) (WorkspaceDTO, bool, error) {
	wsIDStr, ok, err := h.state.Load(ctx)
	if err != nil {
		return WorkspaceDTO{}, false, fmt.Errorf("get last opened workspace: load state: %w", err)
	}
	if !ok {
		return WorkspaceDTO{}, false, nil
	}

	wsID, err := domain.NewWorkspaceId(wsIDStr)
	if err != nil {
		return WorkspaceDTO{}, false, fmt.Errorf("get last opened workspace: invalid workspace id %q: %w", wsIDStr, err)
	}

	w, err := h.repo.FindByID(ctx, wsID)
	if err != nil {
		if errors.Is(err, domain.ErrWorkspaceNotFound) {
			// The recorded workspace has been deleted; treat as "no last opened".
			return WorkspaceDTO{}, false, nil
		}
		return WorkspaceDTO{}, false, fmt.Errorf("get last opened workspace: %w", err)
	}

	return toWorkspaceDTO(w), true, nil
}
