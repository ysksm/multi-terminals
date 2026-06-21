# Core Domain Layer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Multi-Terminals の `core/domain` パッケージ（Value Object・Entity・集約 `Workspace`・Repository ポート）をテスト駆動で実装する。

**Architecture:** クリーンアーキテクチャの最内層。依存ゼロの純粋 Go パッケージ。集約ルート `Workspace` が全不変条件を強制し、外部からの状態変更はすべて集約のメソッド経由で行う。永続化・PTY などの I/O は本計画の対象外（ポートのインターフェースのみ定義）。

**Tech Stack:** Go 1.26、標準ライブラリのみ（`testing`、`errors`、`fmt`、`strings`）。テストフレームワークは標準 `testing`。

## Global Constraints

- Go module path: `github.com/ysksm/multi-terminals`
- `go.mod` の go ディレクティブ: `1.26`
- パッケージ名: `domain`（ディレクトリ `core/domain/`）
- 外部依存ゼロ（標準ライブラリのみ）
- Value Object はすべて非公開フィールド + コンストラクタ `NewXxx(...) (Xxx, error)` でバリデーション。ディレクトリ存在確認など I/O を伴う検証はドメイン層では行わない（非空などの構文的検証のみ）。
- 集約外部からの `Workspace` / `Pane` 内部状態の変更は集約のメソッド経由のみ。`Pane` の状態変更メソッドは非公開（同一パッケージの `Workspace` からのみ呼ぶ）。
- LayoutPreset の容量: `Single`=1, `SplitVertical`=2, `SplitHorizontal`=2, `Grid2x2`=4
- すべての識別子比較は `Equals` メソッド経由（`==` で構造体比較せず、意図を明示）
- コミットメッセージ末尾に必ず以下を付与:
  ```
  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
  ```

---

### Task 1: モジュール初期化と LayoutPreset

**Files:**
- Create: `go.mod`
- Create: `core/domain/layout_preset.go`
- Test: `core/domain/layout_preset_test.go`

**Interfaces:**
- Consumes: なし
- Produces:
  - 型 `LayoutPreset string`
  - 定数 `LayoutSingle`, `LayoutSplitVertical`, `LayoutSplitHorizontal`, `LayoutGrid2x2`
  - `func (l LayoutPreset) Capacity() int`
  - `func (l LayoutPreset) IsValid() bool`

- [ ] **Step 1: Go モジュールを初期化**

Run:
```bash
cd /Users/kasamatsu/src/github/multi-terminals
go mod init github.com/ysksm/multi-terminals
```
Expected: `go.mod` が作成される。中身の go ディレクティブが `1.26` であることを確認（異なる場合は手で `1.26` に修正）。

- [ ] **Step 2: 失敗するテストを書く**

Create `core/domain/layout_preset_test.go`:
```go
package domain

import "testing"

func TestLayoutPresetCapacity(t *testing.T) {
	cases := []struct {
		preset LayoutPreset
		want   int
	}{
		{LayoutSingle, 1},
		{LayoutSplitVertical, 2},
		{LayoutSplitHorizontal, 2},
		{LayoutGrid2x2, 4},
	}
	for _, c := range cases {
		if got := c.preset.Capacity(); got != c.want {
			t.Errorf("%s.Capacity() = %d, want %d", c.preset, got, c.want)
		}
	}
}

func TestLayoutPresetIsValid(t *testing.T) {
	if !LayoutGrid2x2.IsValid() {
		t.Error("LayoutGrid2x2 should be valid")
	}
	if LayoutPreset("unknown").IsValid() {
		t.Error("unknown preset should be invalid")
	}
}
```

- [ ] **Step 3: テストが失敗することを確認**

Run: `go test ./core/domain/ -run TestLayoutPreset -v`
Expected: コンパイルエラー（`undefined: LayoutSingle` など）で FAIL。

- [ ] **Step 4: 最小実装を書く**

Create `core/domain/layout_preset.go`:
```go
package domain

// LayoutPreset はワークスペースの画面分割プリセットを表す Value Object。
type LayoutPreset string

const (
	LayoutSingle          LayoutPreset = "single"
	LayoutSplitVertical   LayoutPreset = "split_vertical"
	LayoutSplitHorizontal LayoutPreset = "split_horizontal"
	LayoutGrid2x2         LayoutPreset = "grid_2x2"
)

// Capacity はこのプリセットが収容できる pane の最大数を返す。
// 未知のプリセットは 0 を返す。
func (l LayoutPreset) Capacity() int {
	switch l {
	case LayoutSingle:
		return 1
	case LayoutSplitVertical, LayoutSplitHorizontal:
		return 2
	case LayoutGrid2x2:
		return 4
	default:
		return 0
	}
}

// IsValid は既知のプリセットかどうかを返す。
func (l LayoutPreset) IsValid() bool {
	return l.Capacity() > 0
}
```

- [ ] **Step 5: テストが通ることを確認**

Run: `go test ./core/domain/ -run TestLayoutPreset -v`
Expected: PASS

- [ ] **Step 6: コミット**

```bash
git add go.mod core/domain/layout_preset.go core/domain/layout_preset_test.go
git commit -m "feat(domain): add LayoutPreset value object

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: 識別子・基本 Value Object 群

**Files:**
- Create: `core/domain/value_objects.go`
- Test: `core/domain/value_objects_test.go`

**Interfaces:**
- Consumes: なし
- Produces:
  - `WorkspaceId` — `NewWorkspaceId(string) (WorkspaceId, error)`, `(WorkspaceId) String() string`, `(WorkspaceId) IsZero() bool`, `(WorkspaceId) Equals(WorkspaceId) bool`
  - `PaneId` — `NewPaneId(string) (PaneId, error)`, `(PaneId) String() string`, `(PaneId) IsZero() bool`, `(PaneId) Equals(PaneId) bool`
  - `WorkspaceName` — `NewWorkspaceName(string) (WorkspaceName, error)`, `(WorkspaceName) String() string`
  - `DirectoryPath` — `NewDirectoryPath(string) (DirectoryPath, error)`, `(DirectoryPath) String() string`
  - `SlotIndex` — `NewSlotIndex(int) (SlotIndex, error)`, `(SlotIndex) Int() int`, `(SlotIndex) Equals(SlotIndex) bool`
  - `StartupCommand` — `NewStartupCommand(command string, autoRun bool) (StartupCommand, error)`, `(StartupCommand) Command() string`, `(StartupCommand) AutoRun() bool`

- [ ] **Step 1: 失敗するテストを書く**

Create `core/domain/value_objects_test.go`:
```go
package domain

import "testing"

func TestNewWorkspaceId(t *testing.T) {
	id, err := NewWorkspaceId("ws-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.String() != "ws-1" {
		t.Errorf("String() = %q, want %q", id.String(), "ws-1")
	}
	if id.IsZero() {
		t.Error("id should not be zero")
	}
	if _, err := NewWorkspaceId("   "); err == nil {
		t.Error("blank id should error")
	}
	var zero WorkspaceId
	if !zero.IsZero() {
		t.Error("zero value should report IsZero")
	}
}

func TestWorkspaceIdEquals(t *testing.T) {
	a, _ := NewWorkspaceId("ws-1")
	b, _ := NewWorkspaceId("ws-1")
	c, _ := NewWorkspaceId("ws-2")
	if !a.Equals(b) {
		t.Error("equal ids should be Equals")
	}
	if a.Equals(c) {
		t.Error("different ids should not be Equals")
	}
}

func TestNewPaneId(t *testing.T) {
	id, err := NewPaneId("pane-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id.String() != "pane-1" {
		t.Errorf("String() = %q", id.String())
	}
	if _, err := NewPaneId(""); err == nil {
		t.Error("empty pane id should error")
	}
}

func TestNewWorkspaceName(t *testing.T) {
	n, err := NewWorkspaceName("My WS")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n.String() != "My WS" {
		t.Errorf("String() = %q", n.String())
	}
	if _, err := NewWorkspaceName(" "); err == nil {
		t.Error("blank name should error")
	}
}

func TestNewDirectoryPath(t *testing.T) {
	p, err := NewDirectoryPath("/home/me/proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.String() != "/home/me/proj" {
		t.Errorf("String() = %q", p.String())
	}
	if _, err := NewDirectoryPath(""); err == nil {
		t.Error("empty path should error")
	}
}

func TestNewSlotIndex(t *testing.T) {
	s, err := NewSlotIndex(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Int() != 0 {
		t.Errorf("Int() = %d", s.Int())
	}
	if _, err := NewSlotIndex(-1); err == nil {
		t.Error("negative slot should error")
	}
	a, _ := NewSlotIndex(2)
	b, _ := NewSlotIndex(2)
	if !a.Equals(b) {
		t.Error("equal slots should be Equals")
	}
}

func TestNewStartupCommand(t *testing.T) {
	c, err := NewStartupCommand("npm run dev", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Command() != "npm run dev" {
		t.Errorf("Command() = %q", c.Command())
	}
	if !c.AutoRun() {
		t.Error("AutoRun() should be true")
	}
	if _, err := NewStartupCommand("  ", false); err == nil {
		t.Error("blank command should error")
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./core/domain/ -run 'TestNew|TestWorkspaceIdEquals' -v`
Expected: コンパイルエラー（`undefined: NewWorkspaceId` など）で FAIL。

- [ ] **Step 3: 最小実装を書く**

Create `core/domain/value_objects.go`:
```go
package domain

import (
	"errors"
	"strings"
)

// WorkspaceId はワークスペースの一意識別子。
type WorkspaceId struct{ value string }

func NewWorkspaceId(value string) (WorkspaceId, error) {
	if strings.TrimSpace(value) == "" {
		return WorkspaceId{}, errors.New("workspace id must not be empty")
	}
	return WorkspaceId{value: value}, nil
}

func (id WorkspaceId) String() string            { return id.value }
func (id WorkspaceId) IsZero() bool               { return id.value == "" }
func (id WorkspaceId) Equals(o WorkspaceId) bool  { return id.value == o.value }

// PaneId は pane の一意識別子。
type PaneId struct{ value string }

func NewPaneId(value string) (PaneId, error) {
	if strings.TrimSpace(value) == "" {
		return PaneId{}, errors.New("pane id must not be empty")
	}
	return PaneId{value: value}, nil
}

func (id PaneId) String() string        { return id.value }
func (id PaneId) IsZero() bool           { return id.value == "" }
func (id PaneId) Equals(o PaneId) bool   { return id.value == o.value }

// WorkspaceName はワークスペースの表示名（非空）。
type WorkspaceName struct{ value string }

func NewWorkspaceName(value string) (WorkspaceName, error) {
	if strings.TrimSpace(value) == "" {
		return WorkspaceName{}, errors.New("workspace name must not be empty")
	}
	return WorkspaceName{value: value}, nil
}

func (n WorkspaceName) String() string { return n.value }

// DirectoryPath は pane の作業ディレクトリ（非空。存在確認はアプリ層の責務）。
type DirectoryPath struct{ value string }

func NewDirectoryPath(value string) (DirectoryPath, error) {
	if strings.TrimSpace(value) == "" {
		return DirectoryPath{}, errors.New("directory path must not be empty")
	}
	return DirectoryPath{value: value}, nil
}

func (p DirectoryPath) String() string { return p.value }

// SlotIndex はレイアウト内の位置（0 始まり）。
type SlotIndex struct{ value int }

func NewSlotIndex(value int) (SlotIndex, error) {
	if value < 0 {
		return SlotIndex{}, errors.New("slot index must be >= 0")
	}
	return SlotIndex{value: value}, nil
}

func (s SlotIndex) Int() int             { return s.value }
func (s SlotIndex) Equals(o SlotIndex) bool { return s.value == o.value }

// StartupCommand は pane を開いたときに実行する候補コマンドと自動実行フラグ。
type StartupCommand struct {
	command string
	autoRun bool
}

func NewStartupCommand(command string, autoRun bool) (StartupCommand, error) {
	if strings.TrimSpace(command) == "" {
		return StartupCommand{}, errors.New("startup command must not be empty")
	}
	return StartupCommand{command: command, autoRun: autoRun}, nil
}

func (c StartupCommand) Command() string { return c.command }
func (c StartupCommand) AutoRun() bool   { return c.autoRun }
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./core/domain/ -run 'TestNew|TestWorkspaceIdEquals' -v`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add core/domain/value_objects.go core/domain/value_objects_test.go
git commit -m "feat(domain): add core value objects

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Pane エンティティ

**Files:**
- Create: `core/domain/pane.go`
- Test: `core/domain/pane_test.go`

**Interfaces:**
- Consumes: `PaneId`, `DirectoryPath`, `SlotIndex`, `StartupCommand`（Task 2）
- Produces:
  - `func NewPane(id PaneId, directory DirectoryPath, slot SlotIndex, commands []StartupCommand) (*Pane, error)`
  - `func (p *Pane) ID() PaneId`
  - `func (p *Pane) Directory() DirectoryPath`
  - `func (p *Pane) Slot() SlotIndex`
  - `func (p *Pane) Commands() []StartupCommand`（防御的コピーを返す）
  - 非公開: `func (p *Pane) setDirectory(DirectoryPath)`, `func (p *Pane) setCommands([]StartupCommand)`（同一パッケージの `Workspace` からのみ呼ぶ）

- [ ] **Step 1: 失敗するテストを書く**

Create `core/domain/pane_test.go`:
```go
package domain

import "testing"

func mustPaneId(t *testing.T, v string) PaneId {
	t.Helper()
	id, err := NewPaneId(v)
	if err != nil {
		t.Fatalf("NewPaneId(%q): %v", v, err)
	}
	return id
}

func mustDir(t *testing.T, v string) DirectoryPath {
	t.Helper()
	d, err := NewDirectoryPath(v)
	if err != nil {
		t.Fatalf("NewDirectoryPath(%q): %v", v, err)
	}
	return d
}

func mustSlot(t *testing.T, v int) SlotIndex {
	t.Helper()
	s, err := NewSlotIndex(v)
	if err != nil {
		t.Fatalf("NewSlotIndex(%d): %v", v, err)
	}
	return s
}

func TestNewPane(t *testing.T) {
	cmd, _ := NewStartupCommand("ls", false)
	p, err := NewPane(mustPaneId(t, "p1"), mustDir(t, "/tmp"), mustSlot(t, 0), []StartupCommand{cmd})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.ID().Equals(mustPaneId(t, "p1")) {
		t.Error("ID mismatch")
	}
	if p.Directory().String() != "/tmp" {
		t.Errorf("Directory = %q", p.Directory().String())
	}
	if p.Slot().Int() != 0 {
		t.Errorf("Slot = %d", p.Slot().Int())
	}
	if len(p.Commands()) != 1 || p.Commands()[0].Command() != "ls" {
		t.Errorf("Commands = %#v", p.Commands())
	}
}

func TestNewPaneRejectsZeroId(t *testing.T) {
	var zero PaneId
	if _, err := NewPane(zero, mustDir(t, "/tmp"), mustSlot(t, 0), nil); err == nil {
		t.Error("zero pane id should error")
	}
}

func TestPaneCommandsIsDefensiveCopy(t *testing.T) {
	cmd, _ := NewStartupCommand("ls", false)
	src := []StartupCommand{cmd}
	p, _ := NewPane(mustPaneId(t, "p1"), mustDir(t, "/tmp"), mustSlot(t, 0), src)
	// 入力スライスを書き換えても内部状態は不変であること
	other, _ := NewStartupCommand("rm -rf /", true)
	src[0] = other
	if p.Commands()[0].Command() != "ls" {
		t.Error("Pane must not share backing array with input slice")
	}
	// 返り値を書き換えても内部状態は不変であること
	got := p.Commands()
	got[0] = other
	if p.Commands()[0].Command() != "ls" {
		t.Error("Commands() must return a defensive copy")
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./core/domain/ -run TestPane -v`
Expected: コンパイルエラー（`undefined: NewPane`）で FAIL。

- [ ] **Step 3: 最小実装を書く**

Create `core/domain/pane.go`:
```go
package domain

import "errors"

// Pane はワークスペース内の 1 つのターミナル枠を表すエンティティ。
type Pane struct {
	id        PaneId
	directory DirectoryPath
	slot      SlotIndex
	commands  []StartupCommand
}

// NewPane は Pane を生成する。commands は防御的にコピーされる。
func NewPane(id PaneId, directory DirectoryPath, slot SlotIndex, commands []StartupCommand) (*Pane, error) {
	if id.IsZero() {
		return nil, errors.New("pane id must not be empty")
	}
	return &Pane{
		id:        id,
		directory: directory,
		slot:      slot,
		commands:  append([]StartupCommand(nil), commands...),
	}, nil
}

func (p *Pane) ID() PaneId               { return p.id }
func (p *Pane) Directory() DirectoryPath { return p.directory }
func (p *Pane) Slot() SlotIndex          { return p.slot }

// Commands は内部スライスの防御的コピーを返す。
func (p *Pane) Commands() []StartupCommand {
	return append([]StartupCommand(nil), p.commands...)
}

func (p *Pane) setDirectory(d DirectoryPath) { p.directory = d }

func (p *Pane) setCommands(c []StartupCommand) {
	p.commands = append([]StartupCommand(nil), c...)
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./core/domain/ -run TestPane -v`
Expected: PASS

注: 非公開メソッド `setDirectory` / `setCommands` はこの時点で未使用のため、`go vet` は通るが「未使用」警告は出ない（メソッドは未使用でもコンパイルエラーにならない）。Task 5 で `Workspace` から使用する。

- [ ] **Step 5: コミット**

```bash
git add core/domain/pane.go core/domain/pane_test.go
git commit -m "feat(domain): add Pane entity

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Workspace 集約 — 生成・改名・レイアウト変更

**Files:**
- Create: `core/domain/workspace.go`
- Test: `core/domain/workspace_test.go`

**Interfaces:**
- Consumes: `WorkspaceId`, `WorkspaceName`, `LayoutPreset`, `Pane`, `PaneId`, `SlotIndex`（Task 1–3）
- Produces:
  - `func NewWorkspace(id WorkspaceId, name WorkspaceName, layout LayoutPreset) (*Workspace, error)`
  - `func (w *Workspace) ID() WorkspaceId`
  - `func (w *Workspace) Name() WorkspaceName`
  - `func (w *Workspace) Layout() LayoutPreset`
  - `func (w *Workspace) Panes() []*Pane`（内部スライスの防御的コピー）
  - `func (w *Workspace) Rename(name WorkspaceName)`
  - `func (w *Workspace) ChangeLayout(layout LayoutPreset) error`
  - 非公開ヘルパ: `func (w *Workspace) findPane(id PaneId) *Pane`

- [ ] **Step 1: 失敗するテストを書く**

Create `core/domain/workspace_test.go`:
```go
package domain

import "testing"

func mustWorkspaceId(t *testing.T, v string) WorkspaceId {
	t.Helper()
	id, err := NewWorkspaceId(v)
	if err != nil {
		t.Fatalf("NewWorkspaceId(%q): %v", v, err)
	}
	return id
}

func mustName(t *testing.T, v string) WorkspaceName {
	t.Helper()
	n, err := NewWorkspaceName(v)
	if err != nil {
		t.Fatalf("NewWorkspaceName(%q): %v", v, err)
	}
	return n
}

func newTestWorkspace(t *testing.T, layout LayoutPreset) *Workspace {
	t.Helper()
	w, err := NewWorkspace(mustWorkspaceId(t, "ws1"), mustName(t, "WS"), layout)
	if err != nil {
		t.Fatalf("NewWorkspace: %v", err)
	}
	return w
}

func TestNewWorkspace(t *testing.T) {
	w := newTestWorkspace(t, LayoutGrid2x2)
	if !w.ID().Equals(mustWorkspaceId(t, "ws1")) {
		t.Error("ID mismatch")
	}
	if w.Name().String() != "WS" {
		t.Errorf("Name = %q", w.Name().String())
	}
	if w.Layout() != LayoutGrid2x2 {
		t.Errorf("Layout = %q", w.Layout())
	}
	if len(w.Panes()) != 0 {
		t.Errorf("new workspace should have no panes, got %d", len(w.Panes()))
	}
}

func TestNewWorkspaceRejectsInvalidLayout(t *testing.T) {
	if _, err := NewWorkspace(mustWorkspaceId(t, "ws1"), mustName(t, "WS"), LayoutPreset("bogus")); err == nil {
		t.Error("invalid layout should error")
	}
}

func TestNewWorkspaceRejectsZeroId(t *testing.T) {
	var zero WorkspaceId
	if _, err := NewWorkspace(zero, mustName(t, "WS"), LayoutSingle); err == nil {
		t.Error("zero workspace id should error")
	}
}

func TestWorkspaceRename(t *testing.T) {
	w := newTestWorkspace(t, LayoutSingle)
	w.Rename(mustName(t, "Renamed"))
	if w.Name().String() != "Renamed" {
		t.Errorf("Name = %q", w.Name().String())
	}
}

func TestWorkspaceChangeLayout(t *testing.T) {
	w := newTestWorkspace(t, LayoutGrid2x2)
	if err := w.ChangeLayout(LayoutSplitVertical); err != nil {
		t.Fatalf("ChangeLayout: %v", err)
	}
	if w.Layout() != LayoutSplitVertical {
		t.Errorf("Layout = %q", w.Layout())
	}
	if err := w.ChangeLayout(LayoutPreset("bogus")); err == nil {
		t.Error("invalid layout should error")
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./core/domain/ -run TestNewWorkspace -v`
Expected: コンパイルエラー（`undefined: NewWorkspace`）で FAIL。

- [ ] **Step 3: 最小実装を書く**

Create `core/domain/workspace.go`:
```go
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
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./core/domain/ -run TestNewWorkspace -v` および `go test ./core/domain/ -run TestWorkspace -v`
Expected: いずれも PASS

注: `lastActive` / `maximized` フィールドと `findPane` ヘルパはこの時点で未使用。Go ではフィールド未使用はコンパイルエラーにならず、非公開メソッド未使用も問題ない。Task 5–6 で使用する。

- [ ] **Step 5: コミット**

```bash
git add core/domain/workspace.go core/domain/workspace_test.go
git commit -m "feat(domain): add Workspace aggregate with layout management

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Workspace 集約 — pane 管理

**Files:**
- Modify: `core/domain/workspace.go`（メソッド追加）
- Modify: `core/domain/workspace_test.go`（テスト追加）

**Interfaces:**
- Consumes: Task 4 の `Workspace`、`Pane`、`DirectoryPath`、`StartupCommand`、`PaneId`
- Produces:
  - `func (w *Workspace) AddPane(p *Pane) error`
  - `func (w *Workspace) RemovePane(id PaneId) error`
  - `func (w *Workspace) SetPaneDirectory(id PaneId, dir DirectoryPath) error`
  - `func (w *Workspace) SetPaneStartupCommands(id PaneId, commands []StartupCommand) error`

- [ ] **Step 1: 失敗するテストを書く**

Append to `core/domain/workspace_test.go`:
```go
func newPaneAt(t *testing.T, id string, slot int) *Pane {
	t.Helper()
	p, err := NewPane(mustPaneId(t, id), mustDir(t, "/tmp"), mustSlot(t, slot), nil)
	if err != nil {
		t.Fatalf("NewPane: %v", err)
	}
	return p
}

func TestWorkspaceAddPane(t *testing.T) {
	w := newTestWorkspace(t, LayoutSplitVertical)
	if err := w.AddPane(newPaneAt(t, "p0", 0)); err != nil {
		t.Fatalf("AddPane: %v", err)
	}
	if err := w.AddPane(newPaneAt(t, "p1", 1)); err != nil {
		t.Fatalf("AddPane: %v", err)
	}
	if len(w.Panes()) != 2 {
		t.Fatalf("expected 2 panes, got %d", len(w.Panes()))
	}
}

func TestWorkspaceAddPaneRejectsOverCapacity(t *testing.T) {
	w := newTestWorkspace(t, LayoutSingle)
	if err := w.AddPane(newPaneAt(t, "p0", 0)); err != nil {
		t.Fatalf("AddPane: %v", err)
	}
	if err := w.AddPane(newPaneAt(t, "p1", 0)); err == nil {
		t.Error("adding beyond capacity should error")
	}
}

func TestWorkspaceAddPaneRejectsSlotOutOfRange(t *testing.T) {
	w := newTestWorkspace(t, LayoutSplitVertical)
	if err := w.AddPane(newPaneAt(t, "p0", 2)); err == nil {
		t.Error("slot >= capacity should error")
	}
}

func TestWorkspaceAddPaneRejectsDuplicateSlot(t *testing.T) {
	w := newTestWorkspace(t, LayoutGrid2x2)
	_ = w.AddPane(newPaneAt(t, "p0", 0))
	if err := w.AddPane(newPaneAt(t, "p1", 0)); err == nil {
		t.Error("duplicate slot should error")
	}
}

func TestWorkspaceAddPaneRejectsDuplicateId(t *testing.T) {
	w := newTestWorkspace(t, LayoutGrid2x2)
	_ = w.AddPane(newPaneAt(t, "p0", 0))
	if err := w.AddPane(newPaneAt(t, "p0", 1)); err == nil {
		t.Error("duplicate pane id should error")
	}
}

func TestWorkspaceRemovePane(t *testing.T) {
	w := newTestWorkspace(t, LayoutSplitVertical)
	_ = w.AddPane(newPaneAt(t, "p0", 0))
	_ = w.AddPane(newPaneAt(t, "p1", 1))
	if err := w.RemovePane(mustPaneId(t, "p0")); err != nil {
		t.Fatalf("RemovePane: %v", err)
	}
	if len(w.Panes()) != 1 {
		t.Fatalf("expected 1 pane, got %d", len(w.Panes()))
	}
	if err := w.RemovePane(mustPaneId(t, "missing")); err == nil {
		t.Error("removing missing pane should error")
	}
}

func TestWorkspaceSetPaneDirectory(t *testing.T) {
	w := newTestWorkspace(t, LayoutSingle)
	_ = w.AddPane(newPaneAt(t, "p0", 0))
	if err := w.SetPaneDirectory(mustPaneId(t, "p0"), mustDir(t, "/var/log")); err != nil {
		t.Fatalf("SetPaneDirectory: %v", err)
	}
	if w.Panes()[0].Directory().String() != "/var/log" {
		t.Errorf("Directory = %q", w.Panes()[0].Directory().String())
	}
	if err := w.SetPaneDirectory(mustPaneId(t, "missing"), mustDir(t, "/x")); err == nil {
		t.Error("missing pane should error")
	}
}

func TestWorkspaceSetPaneStartupCommands(t *testing.T) {
	w := newTestWorkspace(t, LayoutSingle)
	_ = w.AddPane(newPaneAt(t, "p0", 0))
	cmd, _ := NewStartupCommand("npm run dev", true)
	if err := w.SetPaneStartupCommands(mustPaneId(t, "p0"), []StartupCommand{cmd}); err != nil {
		t.Fatalf("SetPaneStartupCommands: %v", err)
	}
	got := w.Panes()[0].Commands()
	if len(got) != 1 || got[0].Command() != "npm run dev" || !got[0].AutoRun() {
		t.Errorf("Commands = %#v", got)
	}
	if err := w.SetPaneStartupCommands(mustPaneId(t, "missing"), nil); err == nil {
		t.Error("missing pane should error")
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./core/domain/ -run 'TestWorkspaceAddPane|TestWorkspaceRemovePane|TestWorkspaceSetPane' -v`
Expected: コンパイルエラー（`undefined: ... AddPane`）で FAIL。

- [ ] **Step 3: 最小実装を書く**

Append to `core/domain/workspace.go`（末尾にメソッド追加）:
```go
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
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./core/domain/ -v`
Expected: 全テスト PASS

- [ ] **Step 5: コミット**

```bash
git add core/domain/workspace.go core/domain/workspace_test.go
git commit -m "feat(domain): add pane management to Workspace aggregate

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Workspace 集約 — アクティブ / 最大化状態

**Files:**
- Modify: `core/domain/workspace.go`（メソッド追加）
- Modify: `core/domain/workspace_test.go`（テスト追加）

**Interfaces:**
- Consumes: Task 4–5 の `Workspace`、`PaneId`
- Produces:
  - `func (w *Workspace) SetLastActivePane(id PaneId) error`
  - `func (w *Workspace) LastActivePaneId() (PaneId, bool)`（bool は設定有無）
  - `func (w *Workspace) MaximizePane(id PaneId) error`
  - `func (w *Workspace) RestoreLayout()`（最大化解除）
  - `func (w *Workspace) MaximizedPaneId() (PaneId, bool)`

- [ ] **Step 1: 失敗するテストを書く**

Append to `core/domain/workspace_test.go`:
```go
func TestWorkspaceSetLastActivePane(t *testing.T) {
	w := newTestWorkspace(t, LayoutSplitVertical)
	_ = w.AddPane(newPaneAt(t, "p0", 0))
	if _, ok := w.LastActivePaneId(); ok {
		t.Error("new workspace should have no last active pane")
	}
	if err := w.SetLastActivePane(mustPaneId(t, "p0")); err != nil {
		t.Fatalf("SetLastActivePane: %v", err)
	}
	got, ok := w.LastActivePaneId()
	if !ok || !got.Equals(mustPaneId(t, "p0")) {
		t.Errorf("LastActivePaneId = %v, ok=%v", got, ok)
	}
	if err := w.SetLastActivePane(mustPaneId(t, "missing")); err == nil {
		t.Error("missing pane should error")
	}
}

func TestWorkspaceMaximizeAndRestore(t *testing.T) {
	w := newTestWorkspace(t, LayoutGrid2x2)
	_ = w.AddPane(newPaneAt(t, "p0", 0))
	_ = w.AddPane(newPaneAt(t, "p1", 1))
	if _, ok := w.MaximizedPaneId(); ok {
		t.Error("new workspace should not be maximized")
	}
	if err := w.MaximizePane(mustPaneId(t, "p1")); err != nil {
		t.Fatalf("MaximizePane: %v", err)
	}
	got, ok := w.MaximizedPaneId()
	if !ok || !got.Equals(mustPaneId(t, "p1")) {
		t.Errorf("MaximizedPaneId = %v, ok=%v", got, ok)
	}
	w.RestoreLayout()
	if _, ok := w.MaximizedPaneId(); ok {
		t.Error("RestoreLayout should clear maximized")
	}
	if err := w.MaximizePane(mustPaneId(t, "missing")); err == nil {
		t.Error("maximizing missing pane should error")
	}
}

func TestRemovePaneClearsActiveAndMaximized(t *testing.T) {
	w := newTestWorkspace(t, LayoutGrid2x2)
	_ = w.AddPane(newPaneAt(t, "p0", 0))
	_ = w.SetLastActivePane(mustPaneId(t, "p0"))
	_ = w.MaximizePane(mustPaneId(t, "p0"))
	if err := w.RemovePane(mustPaneId(t, "p0")); err != nil {
		t.Fatalf("RemovePane: %v", err)
	}
	if _, ok := w.LastActivePaneId(); ok {
		t.Error("removing active pane should clear lastActive")
	}
	if _, ok := w.MaximizedPaneId(); ok {
		t.Error("removing maximized pane should clear maximized")
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./core/domain/ -run 'TestWorkspaceSetLastActivePane|TestWorkspaceMaximizeAndRestore|TestRemovePaneClears' -v`
Expected: コンパイルエラー（`undefined: ... SetLastActivePane`）で FAIL。

- [ ] **Step 3: 最小実装を書く**

Append to `core/domain/workspace.go`（末尾にメソッド追加）:
```go
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
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./core/domain/ -v`
Expected: 全テスト PASS

- [ ] **Step 5: コミット**

```bash
git add core/domain/workspace.go core/domain/workspace_test.go
git commit -m "feat(domain): add active/maximized pane state to Workspace

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: WorkspaceRepository ポート

**Files:**
- Create: `core/domain/repository.go`
- Test: `core/domain/repository_test.go`

**Interfaces:**
- Consumes: `Workspace`, `WorkspaceId`（Task 1–6）
- Produces:
  - `var ErrWorkspaceNotFound = errors.New("workspace not found")`
  - インターフェース `WorkspaceRepository`:
    - `Save(ctx context.Context, w *Workspace) error`
    - `FindByID(ctx context.Context, id WorkspaceId) (*Workspace, error)`
    - `List(ctx context.Context) ([]*Workspace, error)`
    - `Delete(ctx context.Context, id WorkspaceId) error`

このタスクの「テスト」は、インターフェースが満たせる形であることをコンパイル時に保証するための最小の fake 実装で行う（インフラ実装は別計画）。

- [ ] **Step 1: 失敗するテストを書く**

Create `core/domain/repository_test.go`:
```go
package domain

import (
	"context"
	"errors"
	"testing"
)

// fakeRepo は WorkspaceRepository を満たすことをコンパイル時に検証するための最小実装。
type fakeRepo struct {
	store map[string]*Workspace
}

func newFakeRepo() *fakeRepo { return &fakeRepo{store: map[string]*Workspace{}} }

func (r *fakeRepo) Save(_ context.Context, w *Workspace) error {
	r.store[w.ID().String()] = w
	return nil
}

func (r *fakeRepo) FindByID(_ context.Context, id WorkspaceId) (*Workspace, error) {
	w, ok := r.store[id.String()]
	if !ok {
		return nil, ErrWorkspaceNotFound
	}
	return w, nil
}

func (r *fakeRepo) List(_ context.Context) ([]*Workspace, error) {
	out := make([]*Workspace, 0, len(r.store))
	for _, w := range r.store {
		out = append(out, w)
	}
	return out, nil
}

func (r *fakeRepo) Delete(_ context.Context, id WorkspaceId) error {
	if _, ok := r.store[id.String()]; !ok {
		return ErrWorkspaceNotFound
	}
	delete(r.store, id.String())
	return nil
}

// コンパイル時アサーション
var _ WorkspaceRepository = (*fakeRepo)(nil)

func TestWorkspaceRepositoryContract(t *testing.T) {
	ctx := context.Background()
	repo := newFakeRepo()

	id, _ := NewWorkspaceId("ws1")
	name, _ := NewWorkspaceName("WS")
	w, _ := NewWorkspace(id, name, LayoutSingle)

	if err := repo.Save(ctx, w); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.FindByID(ctx, id)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if !got.ID().Equals(id) {
		t.Error("FindByID returned wrong workspace")
	}
	list, err := repo.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("List: %v len=%d", err, len(list))
	}
	if err := repo.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.FindByID(ctx, id); !errors.Is(err, ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./core/domain/ -run TestWorkspaceRepositoryContract -v`
Expected: コンパイルエラー（`undefined: WorkspaceRepository` / `undefined: ErrWorkspaceNotFound`）で FAIL。

- [ ] **Step 3: 最小実装を書く**

Create `core/domain/repository.go`:
```go
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
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./core/domain/ -v`
Expected: 全テスト PASS

- [ ] **Step 5: パッケージ全体の健全性確認**

Run:
```bash
go vet ./core/domain/
go test ./core/domain/ -cover
```
Expected: `go vet` は無出力（問題なし）。`go test` は全 PASS、カバレッジが表示される。

- [ ] **Step 6: コミット**

```bash
git add core/domain/repository.go core/domain/repository_test.go
git commit -m "feat(domain): add WorkspaceRepository port

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## 次の計画（本計画のスコープ外）

- **application 層**: Command/Query ハンドラ、`TerminalRunner` ポート、ID 生成・Clock ポート、DTO
- **infrastructure 層**: JSON `WorkspaceRepository` 実装（`~/.config/multi-terminals/`）、`creack/pty` による `TerminalRunner` 実装、`app-state.json`（`lastOpenedWorkspaceId`）
- **apps/web アダプタ + frontend**: HTTP+WebSocket、Svelte + xterm.js でエンドツーエンド検証
- **apps/wails アダプタ**

これらはそれぞれ独立した spec→plan→実装サイクルで進める。
