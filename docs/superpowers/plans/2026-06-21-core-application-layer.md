# Core Application Layer Implementation Plan

> Execution: this plan is driven by a Workflow (sequential TDD implementers + parallel review). Steps use checkbox syntax for human tracking; the workflow handles progression.

**Goal:** `core/application` の CQRS コマンド/クエリハンドラ、`IDGenerator` ポート、読み取り DTO、および domain の永続化復元用コンストラクタを TDD で実装する。

**Architecture:** クリーンアーキテクチャの application 層。domain のみに依存し、永続化は `domain.WorkspaceRepository` ポート越し。Command と Query を別パッケージに分離（CQRS）。各コマンドハンドラは「repo で集約をロード → ドメインメソッド呼び出し → repo に保存」、各クエリハンドラは「repo から読み取り → DTO に変換」。

**Tech Stack:** Go 1.26、標準ライブラリのみ（`context`, `errors`, `fmt`）。テストは標準 `testing` + 共有フェイク（`apptest` パッケージ）。

## Global Constraints

- Go module: `github.com/ysksm/multi-terminals`、go directive `1.26`
- 外部依存ゼロ（標準ライブラリのみ）
- パッケージ構成:
  - `core/application/port` — ポート interface（`IDGenerator`）
  - `core/application/apptest` — テスト用共有フェイク（`FakeRepo`, `FakeIDGen`）
  - `core/application/command` — コマンド入力 DTO + ハンドラ
  - `core/application/query` — クエリ入力 + 読み取り DTO + ハンドラ
- ハンドラは入力に**プリミティブ型の DTO**を受け取り、ハンドラ内で domain の Value Object を構築する（VO の構築エラーはハンドラがそのまま返す）。UI/アダプタが domain 型を直接組み立てなくて済むようにする。
- コマンドハンドラは構造体 + `Handle(ctx context.Context, cmd XxxCommand) error`（生成系のみ `(Result, error)`）。依存はコンストラクタ `NewXxxHandler(deps...) *XxxHandler` で注入。
- 1 ファイル 1 ハンドラ（テストは同名 `_test.go`）。
- TDD: 失敗するテスト → 失敗確認 → 最小実装 → 通過確認 → コミット。
- 各タスクのコミットメッセージ末尾に必ず:
  ```
  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
  ```

## 設計上の決定（レビュー申し送りへの回答）

1. **永続化復元コンストラクタ**: `domain.ReconstituteWorkspace(...)` を追加（Task 7）。JSON リポジトリ（次計画）が検証済みの永続データから集約を再構築するのに使う。不変条件は通常の生成時と同様に検証する。
2. **version フィールド**: 集約には持たせない。永続化 DTO（infra 層）にのみ持たせる方針。本計画ではコード追加なし。
3. **TerminalRunner ポート / ランタイムコマンド（OpenWorkspace/Write/Resize/Close）**: 本計画のスコープ外。実際に使う infra/PTY 計画で定義する（YAGNI）。本計画は永続化系の CQRS に集中する。

## 既存コードの前提（domain パッケージ・確定済み）

- `domain.Workspace`: `NewWorkspace(id WorkspaceId, name WorkspaceName, layout LayoutPreset) (*Workspace, error)`、メソッド `ID()`, `Name()`, `Layout()`, `Panes() []*Pane`, `Rename(WorkspaceName)`, `ChangeLayout(LayoutPreset) error`, `AddPane(*Pane) error`, `RemovePane(PaneId) error`, `SetPaneDirectory(PaneId, DirectoryPath) error`, `SetPaneStartupCommands(PaneId, []StartupCommand) error`, `MaximizePane(PaneId) error`, `RestoreLayout()`, `SetLastActivePane(PaneId) error`, `LastActivePaneId() (PaneId, bool)`, `MaximizedPaneId() (PaneId, bool)`
- `domain.Pane`: `NewPane(id PaneId, directory DirectoryPath, slot SlotIndex, commands []StartupCommand) (*Pane, error)`、`ID()`, `Directory()`, `Slot()`, `Commands() []StartupCommand`
- VO コンストラクタ: `NewWorkspaceId`, `NewPaneId`, `NewWorkspaceName`, `NewDirectoryPath`, `NewSlotIndex(int)`, `NewStartupCommand(string, bool)`（すべて `(T, error)`）。`String()`/`Int()`/`Equals`/`IsZero` あり。
- `domain.WorkspaceRepository` interface: `Save(ctx, *Workspace) error`, `FindByID(ctx, WorkspaceId) (*Workspace, error)`, `List(ctx) ([]*Workspace, error)`, `Delete(ctx, WorkspaceId) error`。sentinel `domain.ErrWorkspaceNotFound`。
- `domain.LayoutPreset` 値: `LayoutSingle`/`LayoutSplitVertical`/`LayoutSplitHorizontal`/`LayoutGrid2x2`、`Capacity()`, `IsValid()`。レイアウト文字列値: `"single"`, `"split_vertical"`, `"split_horizontal"`, `"grid_2x2"`。

---

## Task 1: port パッケージ + apptest フェイク + CreateWorkspace（最初の縦切り）

**Files:**
- Create: `core/application/port/id_generator.go` — `type IDGenerator interface { NewID() string }`
- Create: `core/application/apptest/fakes.go` —
  - `FakeIDGen`: `port.IDGenerator` 実装。`NewFakeIDGen(ids ...string) *FakeIDGen` で固定 ID 列を順に返す（尽きたら連番 `"id-N"`）。
  - `FakeRepo`: `domain.WorkspaceRepository` 実装（インメモリ map）。`NewFakeRepo() *FakeRepo`。`Save`/`FindByID`/`List`/`Delete`、未存在は `domain.ErrWorkspaceNotFound`。コンパイル時アサーション `var _ domain.WorkspaceRepository = (*FakeRepo)(nil)`。
- Create: `core/application/command/create_workspace.go`
  - `type CreateWorkspaceCommand struct { Name string; Layout string }`
  - `type CreateWorkspaceResult struct { WorkspaceID string }`
  - `type CreateWorkspaceHandler struct { repo domain.WorkspaceRepository; idgen port.IDGenerator }`
  - `func NewCreateWorkspaceHandler(repo domain.WorkspaceRepository, idgen port.IDGenerator) *CreateWorkspaceHandler`
  - `func (h *CreateWorkspaceHandler) Handle(ctx context.Context, cmd CreateWorkspaceCommand) (CreateWorkspaceResult, error)` — idgen.NewID → NewWorkspaceId → NewWorkspaceName(cmd.Name) → NewWorkspace(id, name, LayoutPreset(cmd.Layout)) → repo.Save → return id。VO/集約構築のエラーはラップして返す。
- Test: `core/application/command/create_workspace_test.go` — apptest を使い、(1) 正常系: 生成され repo に保存され Result.ID が idgen の値、(2) 空名でエラー、(3) 不正レイアウトでエラー。

**TDD steps:** 失敗テスト→失敗確認(`go test ./core/application/...`)→port/apptest/handler 実装→通過確認→コミット `feat(application): add IDGenerator port, test fakes, CreateWorkspace handler`。

---

## Task 2: RenameWorkspace + ChangeLayout ハンドラ

**Files (each: handler + test):**
- `core/application/command/rename_workspace.go` — `RenameWorkspaceCommand{ WorkspaceID, Name string }`、`RenameWorkspaceHandler{ repo }`、`NewRenameWorkspaceHandler(repo)`、`Handle`: NewWorkspaceId → repo.FindByID（未存在はそのまま `domain.ErrWorkspaceNotFound` を返す）→ NewWorkspaceName → w.Rename → repo.Save。
- `core/application/command/change_layout.go` — `ChangeLayoutCommand{ WorkspaceID, Layout string }`、`Handle`: FindByID → w.ChangeLayout(LayoutPreset(cmd.Layout))（エラーはそのまま返す）→ repo.Save。
- Tests: 正常系（保存後の状態を repo.FindByID で検証）、未存在ワークスペースでエラー、不正値でエラー。

**Commit:** `feat(application): add RenameWorkspace and ChangeLayout handlers`

---

## Task 3: AddPane + RemovePane ハンドラ

**Files:**
- `core/application/command/add_pane.go`:
  - `type StartupCommandInput struct { Command string; AutoRun bool }`（command パッケージ共有の入力型。ここで定義）
  - `AddPaneCommand{ WorkspaceID string; Directory string; Slot int; Commands []StartupCommandInput }`
  - `AddPaneResult{ PaneID string }`
  - `AddPaneHandler{ repo; idgen }`、`NewAddPaneHandler(repo, idgen)`
  - `Handle`: FindByID → idgen.NewID→NewPaneId → NewDirectoryPath → NewSlotIndex(cmd.Slot) → 各 Commands を NewStartupCommand で構築 → NewPane → w.AddPane（エラーはそのまま）→ repo.Save → return PaneID。
- `core/application/command/remove_pane.go`: `RemovePaneCommand{ WorkspaceID, PaneID string }`、`Handle`: FindByID → NewPaneId → w.RemovePane → repo.Save。
- Tests: AddPane 正常系（PaneID 返却、保存後 Panes に反映）、容量超過/不正 slot/不正ディレクトリでエラー、未存在 WS でエラー。RemovePane 正常系・未存在 pane でエラー。

**Commit:** `feat(application): add AddPane and RemovePane handlers`

---

## Task 4: SetPaneDirectory + SetPaneStartupCommands ハンドラ

**Files:**
- `core/application/command/set_pane_directory.go`: `SetPaneDirectoryCommand{ WorkspaceID, PaneID, Directory string }`、`Handle`: FindByID → NewPaneId → NewDirectoryPath → w.SetPaneDirectory → repo.Save。
- `core/application/command/set_pane_startup_commands.go`: `SetPaneStartupCommandsCommand{ WorkspaceID, PaneID string; Commands []StartupCommandInput }`（Task 3 の `StartupCommandInput` を再利用）、`Handle`: FindByID → NewPaneId → 各 NewStartupCommand → w.SetPaneStartupCommands → repo.Save。
- Tests: 正常系（保存後 pane の状態を検証）、未存在 pane/WS でエラー、不正値でエラー。

**Commit:** `feat(application): add SetPaneDirectory and SetPaneStartupCommands handlers`

---

## Task 5: MaximizePane + RestoreLayout + SetLastActivePane ハンドラ

**Files:**
- `core/application/command/maximize_pane.go`: `MaximizePaneCommand{ WorkspaceID, PaneID string }`、`Handle`: FindByID → NewPaneId → w.MaximizePane → repo.Save。
- `core/application/command/restore_layout.go`: `RestoreLayoutCommand{ WorkspaceID string }`、`Handle`: FindByID → w.RestoreLayout() → repo.Save。
- `core/application/command/set_last_active_pane.go`: `SetLastActivePaneCommand{ WorkspaceID, PaneID string }`、`Handle`: FindByID → NewPaneId → w.SetLastActivePane → repo.Save。
- Tests: 正常系（保存後 MaximizedPaneId/LastActivePaneId を検証）、未存在 pane/WS でエラー。

**Commit:** `feat(application): add MaximizePane, RestoreLayout, SetLastActivePane handlers`

---

## Task 6: query パッケージ — DTO + GetWorkspace + ListWorkspaces

**Files:**
- `core/application/query/dto.go`:
  - `type StartupCommandDTO struct { Command string; AutoRun bool }`
  - `type PaneDTO struct { ID string; Directory string; Slot int; Commands []StartupCommandDTO }`
  - `type WorkspaceDTO struct { ID string; Name string; Layout string; Panes []PaneDTO; LastActivePaneID *string; MaximizedPaneID *string }`
  - 非公開ヘルパ `toWorkspaceDTO(w *domain.Workspace) WorkspaceDTO`（Panes を SlotIndex 昇順に並べる。LastActive/Maximized は設定ありのとき `*string`、なければ nil）。
- `core/application/query/get_workspace.go`: `GetWorkspaceQuery{ WorkspaceID string }`、`GetWorkspaceHandler{ repo }`、`Handle(ctx, q) (WorkspaceDTO, error)`: NewWorkspaceId → repo.FindByID → toWorkspaceDTO。未存在は `domain.ErrWorkspaceNotFound`。
- `core/application/query/list_workspaces.go`: `ListWorkspacesHandler{ repo }`、`Handle(ctx) ([]WorkspaceDTO, error)`: repo.List → 各 toWorkspaceDTO（空なら空スライス）。
- Tests: DTO 変換（pane 並び順、active/maximized の有無）、GetWorkspace 正常/未存在、ListWorkspaces 空/複数。

**Commit:** `feat(application): add query DTOs, GetWorkspace and ListWorkspaces handlers`

---

## Task 7: domain 永続化復元コンストラクタ

**Files:**
- Modify: `core/domain/workspace.go` — 追加:
  - `func ReconstituteWorkspace(id WorkspaceId, name WorkspaceName, layout LayoutPreset, panes []*Pane, lastActive *PaneId, maximized *PaneId) (*Workspace, error)`
  - 検証: id 非ゼロ、layout 有効、pane 数 ≤ capacity、slot 重複なし・範囲内、pane id 重複なし、lastActive/maximized は与えた panes のいずれかを指す（または nil）。既存の `AddPane` のチェックロジックと整合させる（重複ロジックは小さな非公開ヘルパに切り出して `AddPane` と共有してよい）。
  - 与えられた `lastActive`/`maximized` はコピーして保持（呼び出し側ポインタをエイリアスしない）。
- Modify: `core/domain/workspace_test.go` — 追加テスト: 正常復元（全フィールドが復元される）、容量超過でエラー、slot 重複でエラー、存在しない pane を指す lastActive/maximized でエラー、nil ポインタは OK。

**Commit:** `feat(domain): add ReconstituteWorkspace for repository hydration`

---

## 完了基準

- `go build ./...`、`go vet ./...` クリーン、`go test ./...` 全 PASS。
- application パッケージ群が domain のみに依存（外部依存ゼロ）。
- 全コマンド/クエリハンドラがフェイク repo に対してテスト済み。

## 次の計画（本計画のスコープ外）

- infrastructure 層: JSON `WorkspaceRepository` 実装（`ReconstituteWorkspace` を使用、`version` フィールドを DTO に）、`app-state.json`、`creack/pty` の `TerminalRunner` 実装、ランタイムコマンド（OpenWorkspace 等）。
- apps/web（WebSocket）+ frontend（Svelte/xterm.js）、apps/wails。
