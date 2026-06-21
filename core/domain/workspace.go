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

// AddPane は pane を追加する。容量超過・slot 範囲外・slot/id 重複はエラー。
func (w *Workspace) AddPane(p *Pane) error {
	if p == nil {
		return errors.New("pane must not be nil")
	}
	if len(w.panes) >= w.layout.Capacity() {
		return fmt.Errorf("cannot add pane: layout capacity %d reached", w.layout.Capacity())
	}
	if p.slot.Int() >= w.layout.Capacity() {
		return fmt.Errorf("pane slot %d out of range for layout capacity %d", p.slot.Int(), w.layout.Capacity())
	}
	for _, existing := range w.panes {
		if existing.id.Equals(p.id) {
			return fmt.Errorf("pane id %s already exists", p.id)
		}
		if existing.slot.Equals(p.slot) {
			return fmt.Errorf("slot %d already occupied", p.slot.Int())
		}
	}
	w.panes = append(w.panes, p)
	return nil
}

// RemovePane は pane を削除する。lastActive / maximized が指していたら解除する。
func (w *Workspace) RemovePane(id PaneId) error {
	idx := -1
	for i, p := range w.panes {
		if p.id.Equals(id) {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("pane %s not found", id)
	}
	w.panes = append(w.panes[:idx], w.panes[idx+1:]...)
	if w.lastActive != nil && w.lastActive.Equals(id) {
		w.lastActive = nil
	}
	if w.maximized != nil && w.maximized.Equals(id) {
		w.maximized = nil
	}
	return nil
}

// SetPaneDirectory は指定 pane の作業ディレクトリを変更する。
func (w *Workspace) SetPaneDirectory(id PaneId, dir DirectoryPath) error {
	p := w.findPane(id)
	if p == nil {
		return fmt.Errorf("pane %s not found", id)
	}
	p.setDirectory(dir)
	return nil
}

// SetPaneStartupCommands は指定 pane の起動コマンド列を置き換える。
func (w *Workspace) SetPaneStartupCommands(id PaneId, commands []StartupCommand) error {
	p := w.findPane(id)
	if p == nil {
		return fmt.Errorf("pane %s not found", id)
	}
	p.setCommands(commands)
	return nil
}
