# Infrastructure: JSON Persistence Implementation Plan

> Execution: driven by a Workflow (sequential TDD implementers + parallel review). Checkbox steps are for human tracking.

**Goal:** `core/infrastructure/jsonstore` に JSON ファイルベースの `domain.WorkspaceRepository` 実装と `AppStateStore`（前回開いた workspace の記録）を実装し、application 層に `GetLastOpenedWorkspace` クエリと `AppStateStore` ポートを追加する。

**Architecture:** クリーンアーキテクチャの infrastructure 層。domain のポート（`WorkspaceRepository`）と application のポート（`AppStateStore`）を実装する。永続化 DTO（`version` 付き）と domain 集約の相互変換はマッパーで行い、ロード時は `domain.ReconstituteWorkspace` で不変条件を再検証する。ファイル I/O は標準ライブラリのみ。

**Tech Stack:** Go 1.26、標準ライブラリのみ（`encoding/json`, `os`, `path/filepath`, `sort`, `sync`, `context`, `errors`, `fmt`）。テストは `t.TempDir()` を使う。

## Global Constraints

- Go module `github.com/ysksm/multi-terminals`、go directive `1.26`
- **このフェーズは外部依存を導入しない**（PTY の `creack/pty` は次フェーズ）。標準ライブラリのみ。
- パッケージ:
  - `core/infrastructure/jsonstore` — `WorkspaceRepository` 実装、`AppStateStore` 実装、永続化 DTO、マッパー
  - 追加: `core/application/port`（`AppStateStore` ポート）、`core/application/query`（`GetLastOpenedWorkspace`）、`core/application/apptest`（`FakeAppStateStore`）
- 永続化 DTO は `version int` を必ず持つ（現行 `CurrentSchemaVersion = 1`）。`version` は集約には持たせず DTO のみ（前フェーズの設計判断どおり）。
- ファイル書き込みは**アトミック**にする（`<name>.tmp` に書いて `os.Rename`）。
- リポジトリは並行アクセス安全にする（`sync.RWMutex`）。
- ロード時、未知（将来）バージョンは明確なエラーで拒否する。
- TDD: 失敗テスト→失敗確認→最小実装→通過確認→コミット。`t.TempDir()` で実ファイル I/O をテスト。
- コミットメッセージ末尾に必ず:
  ```
  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
  ```

## 既存コードの前提（確定済み）

- `domain.ReconstituteWorkspace(id WorkspaceId, name WorkspaceName, layout LayoutPreset, panes []*Pane, lastActive *PaneId, maximized *PaneId) (*Workspace, error)` — 永続データからの再構築用。全不変条件を検証。
- `domain.Workspace` getter: `ID() WorkspaceId`, `Name() WorkspaceName`, `Layout() LayoutPreset`, `Panes() []*Pane`, `LastActivePaneId() (PaneId, bool)`, `MaximizedPaneId() (PaneId, bool)`。
- `domain.Pane` getter: `ID() PaneId`, `Directory() DirectoryPath`, `Slot() SlotIndex`, `Commands() []StartupCommand`。
- VO: `NewWorkspaceId`/`NewPaneId`/`NewWorkspaceName`/`NewDirectoryPath`/`NewSlotIndex(int)`/`NewStartupCommand(string,bool)`（すべて `(T,error)`）、`String()`/`Int()`、`StartupCommand.Command()`/`AutoRun()`。
- `domain.WorkspaceRepository`: `Save`/`FindByID`/`List`/`Delete`、sentinel `domain.ErrWorkspaceNotFound`。
- レイアウト文字列値: `"single"`/`"split_vertical"`/`"split_horizontal"`/`"grid_2x2"`（`domain.LayoutPreset` はこの文字列が下層値）。
- application 層: `core/application/query` に `WorkspaceDTO` と非公開 `toWorkspaceDTO(*domain.Workspace) WorkspaceDTO` あり。`core/application/apptest` に `FakeRepo`。`core/application/port` に `IDGenerator`。

---

## Task 1: 永続化 DTO とスキーマバージョン

**Files:**
- Create: `core/infrastructure/jsonstore/schema.go`
  - `const CurrentSchemaVersion = 1`
  - `type startupCommandRecord struct { Command string \`json:"command"\`; AutoRun bool \`json:"autoRun"\` }`
  - `type paneRecord struct { ID string \`json:"id"\`; Directory string \`json:"directory"\`; Slot int \`json:"slot"\`; Commands []startupCommandRecord \`json:"commands"\` }`
  - `type workspaceRecord struct { Version int \`json:"version"\`; ID string \`json:"id"\`; Name string \`json:"name"\`; Layout string \`json:"layout"\`; Panes []paneRecord \`json:"panes"\`; LastActivePaneID *string \`json:"lastActivePaneId,omitempty"\`; MaximizedPaneID *string \`json:"maximizedPaneId,omitempty"\` }`
  - `type appStateRecord struct { Version int \`json:"version"\`; LastOpenedWorkspaceID string \`json:"lastOpenedWorkspaceId"\` }`
- Test: `core/infrastructure/jsonstore/schema_test.go` — `workspaceRecord` を `json.Marshal`→`json.Unmarshal` してフィールドが往復することを確認（特に `omitempty` で nil ポインタが欠落し、設定時に出力されること）。

**Commit:** `feat(jsonstore): add persistence DTOs and schema version`

---

## Task 2: マッパー（domain ⇔ record）

**Files:**
- Create: `core/infrastructure/jsonstore/mapper.go`
  - `func toRecord(w *domain.Workspace) workspaceRecord` — getter から全フィールドを詰める。Panes は `w.Panes()` をそのまま（順序は保存時のまま）。`LastActivePaneId()`/`MaximizedPaneId()` が ok のとき `*string` を設定、なければ nil。`Version = CurrentSchemaVersion`。
  - `func toDomain(rec workspaceRecord) (*domain.Workspace, error)` — まず `rec.Version` を検証（`> CurrentSchemaVersion` なら `fmt.Errorf("unsupported schema version %d", rec.Version)`、`< 1` もエラー）。VO を構築（`NewWorkspaceId`/`NewWorkspaceName`、`domain.LayoutPreset(rec.Layout)`）、各 `paneRecord` を VO + `domain.NewPane` で `*Pane` に変換、`LastActivePaneID`/`MaximizedPaneID` を `*PaneId` に変換（nil は nil）、最後に `domain.ReconstituteWorkspace(...)` を呼ぶ。いずれかの構築エラーはラップして返す。
- Test: `core/infrastructure/jsonstore/mapper_test.go` — domain で `Workspace`（pane 複数 + maximized + lastActive 設定）を作り `toRecord`→`toDomain` の往復で同値（ID/Name/Layout/pane 数・各 pane の dir/slot/commands・active/maximized）を確認。未知バージョン（999）で `toDomain` がエラー、バージョン 0 でエラー。不正レイアウト文字列でエラー。

**Commit:** `feat(jsonstore): add domain<->record mapper using ReconstituteWorkspace`

---

## Task 3: WorkspaceRepository — Save / FindByID（アトミック書き込み）

**Files:**
- Create: `core/infrastructure/jsonstore/workspace_repository.go`
  - `type WorkspaceRepository struct { dir string; mu sync.RWMutex }`（`dir` は workspaces ディレクトリ）
  - `func NewWorkspaceRepository(baseDir string) (*WorkspaceRepository, error)` — `dir = filepath.Join(baseDir, "workspaces")` を `os.MkdirAll(dir, 0o755)` で用意。
  - `func DefaultBaseDir() (string, error)` — `os.UserConfigDir()` + `"multi-terminals"`（adapter が使う既定パス）。
  - 非公開 `func (r *WorkspaceRepository) pathFor(id string) string` → `filepath.Join(r.dir, id+".json")`。
  - `func (r *WorkspaceRepository) Save(ctx context.Context, w *domain.Workspace) error` — `toRecord`→`json.MarshalIndent`→アトミック書き込み（`pathFor+".tmp"` に `os.WriteFile`（0o644）→`os.Rename`）。`mu.Lock`。
  - `func (r *WorkspaceRepository) FindByID(ctx context.Context, id domain.WorkspaceId) (*domain.Workspace, error)` — `os.ReadFile`（`os.IsNotExist` → `domain.ErrWorkspaceNotFound`）→`json.Unmarshal`→`toDomain`。`mu.RLock`。
  - コンパイル時アサーション `var _ domain.WorkspaceRepository = (*WorkspaceRepository)(nil)`（残り 2 メソッドは Task 4 で実装するまでスタブにせず、Task 4 と同時に満たす想定 → このタスクでは List/Delete も最小シグネチャだけ用意して `panic("not implemented")` ではなく **Task 4 で実装**。アサーションは Task 4 で追加する）。
- Test: `core/infrastructure/jsonstore/workspace_repository_test.go` — `t.TempDir()` で repo を作り、Save した workspace を FindByID で取得し同値を確認。存在しない ID で `errors.Is(err, domain.ErrWorkspaceNotFound)`。`.json` ファイルが実際に作られること、`.tmp` が残らないことを確認。

注: コンパイル時アサーションは List/Delete が未実装だと型を満たさずビルドできない。よって **このタスクでは List/Delete を空実装（正しいシグネチャで `return nil, nil` 等）として置き**、Task 4 で本実装に差し替える。アサーション行は Task 3 で入れてよい。

**Commit:** `feat(jsonstore): add WorkspaceRepository Save and FindByID with atomic writes`

---

## Task 4: WorkspaceRepository — List / Delete

**Files:**
- Modify: `core/infrastructure/jsonstore/workspace_repository.go` — Task 3 の空実装を本実装に差し替え:
  - `func (r *WorkspaceRepository) List(ctx context.Context) ([]*domain.Workspace, error)` — `os.ReadDir(r.dir)`、`.json` 拡張子のみ対象、各ファイルを読み `toDomain`。`mu.RLock`。ディレクトリが空なら空スライス。1 ファイルの破損/変換エラーは全体を失敗させる（`fmt.Errorf` でファイル名を含める）。
  - `func (r *WorkspaceRepository) Delete(ctx context.Context, id domain.WorkspaceId) error` — `os.Remove(pathFor)`、`os.IsNotExist` → `domain.ErrWorkspaceNotFound`。`mu.Lock`。
- Modify test: List 空/複数、Delete 正常・未存在で `ErrWorkspaceNotFound`、Delete 後の FindByID が `ErrWorkspaceNotFound`。

**Commit:** `feat(jsonstore): implement WorkspaceRepository List and Delete`

---

## Task 5: 堅牢性テスト（破損・未知バージョン・往復網羅）

**Files:**
- Modify: `core/infrastructure/jsonstore/workspace_repository_test.go` — 追加テスト:
  - 不正な JSON ファイルを `dir` に直接書き、`FindByID`/`List` がエラーを返す（パニックしない）。
  - `version: 999` のレコードを書き、`FindByID` が「unsupported schema version」を含むエラー。
  - `.json` 以外のファイル（例 `README.txt`）が `dir` にあっても `List` が無視する。
  - 複数 pane + maximized + lastActive を持つ workspace の Save→FindByID 完全往復で全フィールド一致。

**Commit:** `test(jsonstore): cover corrupt files, unknown version, and full round-trip`

---

## Task 6: AppStateStore（前回開いた workspace の記録）

**Files:**
- Create: `core/infrastructure/jsonstore/app_state.go`
  - `type AppStateStore struct { path string; mu sync.RWMutex }`
  - `func NewAppStateStore(baseDir string) *AppStateStore` — `path = filepath.Join(baseDir, "app-state.json")`。
  - `func (s *AppStateStore) Load(ctx context.Context) (workspaceID string, ok bool, err error)` — ファイルが無ければ `("", false, nil)`。あれば読み込み、`LastOpenedWorkspaceID` が空なら `ok=false`、非空なら `(id, true, nil)`。未知バージョンはエラー。
  - `func (s *AppStateStore) SetLastOpened(ctx context.Context, workspaceID string) error` — `appStateRecord{Version: CurrentSchemaVersion, LastOpenedWorkspaceID: workspaceID}` をアトミック書き込み。
- Test: `core/infrastructure/jsonstore/app_state_test.go` — `t.TempDir()`：未作成時 Load は `ok=false`、SetLastOpened 後 Load が `(id, true)`、上書き、空文字 set 後は `ok=false`。

**Commit:** `feat(jsonstore): add AppStateStore for last-opened workspace`

---

## Task 7: application 層の GetLastOpenedWorkspace クエリ + AppStateStore ポート

**Files:**
- Create: `core/application/port/app_state_store.go`
  - `type AppStateStore interface { Load(ctx context.Context) (workspaceID string, ok bool, err error); SetLastOpened(ctx context.Context, workspaceID string) error }`
  - （infra の `jsonstore.AppStateStore` がこのインターフェースを満たす。jsonstore 側のコンパイル時アサーションは循環 import を避けるため application 側テストで担保する。）
- Modify: `core/application/apptest/fakes.go` — `FakeAppStateStore` を追加（インメモリ。`NewFakeAppStateStore()`、`Load`/`SetLastOpened`）。コンパイル時アサーション `var _ port.AppStateStore = (*FakeAppStateStore)(nil)`。
- Create: `core/application/query/get_last_opened_workspace.go`
  - `type GetLastOpenedWorkspaceHandler struct { state port.AppStateStore; repo domain.WorkspaceRepository }`
  - `func NewGetLastOpenedWorkspaceHandler(state port.AppStateStore, repo domain.WorkspaceRepository) *GetLastOpenedWorkspaceHandler`
  - `func (h *GetLastOpenedWorkspaceHandler) Handle(ctx context.Context) (WorkspaceDTO, bool, error)` — `state.Load`：`ok=false` なら `(WorkspaceDTO{}, false, nil)`。`ok=true` なら `NewWorkspaceId`→`repo.FindByID`→`toWorkspaceDTO`→`(dto, true, nil)`。`repo` が `ErrWorkspaceNotFound`（記録された WS が削除済み）の場合も `(WorkspaceDTO{}, false, nil)` を返す（壊れた参照は「前回なし」として扱う）。
- Create: `core/application/query/get_last_opened_workspace_test.go` — `apptest.FakeAppStateStore` + `apptest.FakeRepo` を使い：未設定で `ok=false`、設定済みで該当 DTO を返す、記録先が削除済みなら `ok=false`。
- Modify: `core/infrastructure/jsonstore/app_state_test.go` または新規 — `var _ port.AppStateStore = (*AppStateStore)(nil)` を **infra のテストファイル**に置き（import application/port はテストのみ。循環 import にならないことを確認。ならない: infra→application/port は一方向）、infra の `AppStateStore` がポートを満たすことを保証。

**Commit:** `feat(application): add GetLastOpenedWorkspace query and AppStateStore port`

---

## 完了基準

- `go build ./...`、`go vet ./...` クリーン、`go test ./...` 全 PASS（infra テストは `t.TempDir()` 使用）。
- 外部依存ゼロを維持（このフェーズでは新規 import なし）。
- `jsonstore.WorkspaceRepository` が `domain.WorkspaceRepository` を、`jsonstore.AppStateStore` が `port.AppStateStore` を満たす（コンパイル時アサーション）。

## 次の計画（スコープ外）

- ターミナル/PTY: `port.TerminalRunner` 定義、`creack/pty` 実装、ランタイムコマンド（OpenWorkspace/WriteToPane/ResizePane/CloseSession）、`TerminalSession`。**apps/web アダプタと対で実装**して end-to-end 検証する。
- apps/web（HTTP+WebSocket）+ frontend（Svelte/xterm.js）、apps/wails。
