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

// SetLastActivePane は最後にアクティブだった pane を記録する。pane が存在しなければエラー。
func (w *Workspace) SetLastActivePane(id PaneId) error {
	if w.findPane(id) == nil {
		return fmt.Errorf("pane %s not found", id)
	}
	copied := id
	w.lastActive = &copied
	return nil
}

// LastActivePaneId は記録された pane id と設定有無を返す。
func (w *Workspace) LastActivePaneId() (PaneId, bool) {
	if w.lastActive == nil {
		return PaneId{}, false
	}
	return *w.lastActive, true
}

// MaximizePane は指定 pane を最大化状態にする。pane が存在しなければエラー。
func (w *Workspace) MaximizePane(id PaneId) error {
	if w.findPane(id) == nil {
		return fmt.Errorf("pane %s not found", id)
	}
	copied := id
	w.maximized = &copied
	return nil
}

// RestoreLayout は最大化状態を解除する。
func (w *Workspace) RestoreLayout() {
	w.maximized = nil
}

// MaximizedPaneId は最大化中の pane id と設定有無を返す。
func (w *Workspace) MaximizedPaneId() (PaneId, bool) {
	if w.maximized == nil {
		return PaneId{}, false
	}
	return *w.maximized, true
}

// hasPaneId は panes スライス中に id が存在するかを返す非公開ヘルパ。
func hasPaneId(panes []*Pane, id PaneId) bool {
	for _, p := range panes {
		if p.id.Equals(id) {
			return true
		}
	}
	return false
}

// ReconstituteWorkspace は永続化ストアから読み出した検証済みデータで集約を再構築する。
// 通常の生成時と同様に不変条件を検証する。
// lastActive / maximized は与えた panes のいずれかを指すか nil でなければならない。
// lastActive / maximized ポインタは値コピーして保持するため呼び出し側の変数とエイリアスしない。
// panes スライスの要素 (*Pane) は集約の通常の規約に従い共有される（防御的コピーは Panes() メソッドが担う）。
func ReconstituteWorkspace(
	id WorkspaceId,
	name WorkspaceName,
	layout LayoutPreset,
	panes []*Pane,
	lastActive *PaneId,
	maximized *PaneId,
) (*Workspace, error) {
	if id.IsZero() {
		return nil, errors.New("workspace id must not be empty")
	}
	if !layout.IsValid() {
		return nil, fmt.Errorf("invalid layout preset: %q", layout)
	}
	if len(panes) > layout.Capacity() {
		return nil, fmt.Errorf("cannot reconstitute workspace: %d panes exceed layout capacity %d", len(panes), layout.Capacity())
	}

	// validate pane slot ranges and uniqueness
	seenIds := make(map[string]bool, len(panes))
	seenSlots := make(map[int]bool, len(panes))
	for _, p := range panes {
		if p == nil {
			return nil, errors.New("pane must not be nil")
		}
		if p.slot.Int() >= layout.Capacity() {
			return nil, fmt.Errorf("pane slot %d out of range for layout capacity %d", p.slot.Int(), layout.Capacity())
		}
		if seenIds[p.id.String()] {
			return nil, fmt.Errorf("pane id %s already exists", p.id)
		}
		seenIds[p.id.String()] = true
		if seenSlots[p.slot.Int()] {
			return nil, fmt.Errorf("slot %d already occupied", p.slot.Int())
		}
		seenSlots[p.slot.Int()] = true
	}

	// validate lastActive / maximized pointers
	if lastActive != nil && !hasPaneId(panes, *lastActive) {
		return nil, fmt.Errorf("lastActive pane %s not found in provided panes", *lastActive)
	}
	if maximized != nil && !hasPaneId(panes, *maximized) {
		return nil, fmt.Errorf("maximized pane %s not found in provided panes", *maximized)
	}

	w := &Workspace{
		id:     id,
		name:   name,
		layout: layout,
		panes:  append([]*Pane(nil), panes...),
	}

	// copy pointers to avoid aliasing caller's variables
	if lastActive != nil {
		copied := *lastActive
		w.lastActive = &copied
	}
	if maximized != nil {
		copied := *maximized
		w.maximized = &copied
	}

	return w, nil
}
