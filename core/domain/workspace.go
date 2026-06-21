package domain

import (
	"errors"
	"fmt"
)

// Workspace は永続化対象の集約ルート。レイアウトと pane 群の不変条件を強制する。
type Workspace struct {
	id        WorkspaceId
	name      WorkspaceName
	layout    LayoutPreset
	panes     []*Pane
	lastActive *PaneId
	maximized  *PaneId
}

func NewWorkspace(id WorkspaceId, name WorkspaceName, layout LayoutPreset) (*Workspace, error) {
	if id.IsZero() {
		return nil, errors.New("workspace id must not be empty")
	}
	if !layout.IsValid() {
		return nil, fmt.Errorf("invalid layout preset: %q", layout)
	}
	return &Workspace{id: id, name: name, layout: layout}, nil
}

func (w *Workspace) ID() WorkspaceId     { return w.id }
func (w *Workspace) Name() WorkspaceName { return w.name }
func (w *Workspace) Layout() LayoutPreset { return w.layout }

// Panes は内部スライスの防御的コピーを返す（要素 *Pane 自体は共有）。
func (w *Workspace) Panes() []*Pane {
	return append([]*Pane(nil), w.panes...)
}

func (w *Workspace) Rename(name WorkspaceName) {
	w.name = name
}

// ChangeLayout はレイアウトを変更する。既存 pane 数・slot が新容量に収まらない場合はエラー。
func (w *Workspace) ChangeLayout(layout LayoutPreset) error {
	if !layout.IsValid() {
		return fmt.Errorf("invalid layout preset: %q", layout)
	}
	if len(w.panes) > layout.Capacity() {
		return fmt.Errorf("cannot change layout: %d panes exceed capacity %d", len(w.panes), layout.Capacity())
	}
	for _, p := range w.panes {
		if p.slot.Int() >= layout.Capacity() {
			return fmt.Errorf("pane slot %d out of range for layout capacity %d", p.slot.Int(), layout.Capacity())
		}
	}
	w.layout = layout
	return nil
}

// findPane は id に一致する pane を返す。なければ nil。
func (w *Workspace) findPane(id PaneId) *Pane {
	for _, p := range w.panes {
		if p.id.Equals(id) {
			return p
		}
	}
	return nil
}
