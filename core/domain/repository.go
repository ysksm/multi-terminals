package domain

import (
	"context"
	"errors"
)

// ErrWorkspaceNotFound は対象 workspace が存在しないことを示す。
var ErrWorkspaceNotFound = errors.New("workspace not found")

// WorkspaceRepository は Workspace 集約の永続化ポート。
// 実装は infrastructure 層が提供する（JSON 実装は別計画）。
type WorkspaceRepository interface {
	Save(ctx context.Context, w *Workspace) error
	FindByID(ctx context.Context, id WorkspaceId) (*Workspace, error)
	List(ctx context.Context) ([]*Workspace, error)
	Delete(ctx context.Context, id WorkspaceId) error
}
