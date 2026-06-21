# Terminal/PTY + Web Backend Implementation Plan

> Execution: driven by a Workflow (sequential TDD implementers + parallel review incl. a concurrency lens). Checkbox steps are for human tracking.

**Goal:** ライブターミナル（PTY）を扱う `TerminalRunner` ポートとランタイムコマンド（OpenWorkspace/WriteToPane/ResizePane/ClosePane）、`creack/pty` 実装、そして `apps/web`（HTTP JSON API + WebSocket でのペイン I/O）を実装し、ブラウザから到達可能な Go サーバを完成させる。

**Architecture:** クリーンアーキテクチャ。ランタイムは `port.TerminalRunner`/`port.TerminalSession` で抽象化し、application 層のランタイムコマンドハンドラがオーケストレーション（ワークスペースを開く→各ペインに PTY 起動→autoRun の起動コマンド送信→前回開いた記録）を行う。ライブセッションは `session.Registry` が保持。infra 層が `creack/pty` で実装。`apps/web` は薄いアダプタ（HTTP ルーティング + WebSocket ブリッジ）で、CQRS ハンドラを呼ぶだけ。

**Tech Stack:** Go 1.26。新規外部依存: `github.com/creack/pty`（infra/terminal のみ）、`github.com/gorilla/websocket`（apps/web のみ）。HTTP ルーティングは標準 `net/http` の Go 1.22+ メソッド付きパターン（ルータ依存なし）。

## Global Constraints

- Go module `github.com/ysksm/multi-terminals`、go directive `1.26`。
- **依存の境界**: `core/domain`・`core/application`（port/command/query/session/apptest を含む）は引き続き**標準ライブラリのみ**。外部依存を使ってよいのは `core/infrastructure/terminal`（`creack/pty`）と `apps/web`（`gorilla/websocket`）だけ。ランタイムハンドラは `port.TerminalRunner` インターフェース越しにしか PTY に触れない。
- 並行安全を徹底: `session.Registry` と PTY セッションは複数 goroutine から触られる（HTTP ハンドラ + 出力ポンプ）。`sync.Mutex`/`RWMutex` と channel で保護し、二重 Close・チャネルへの送信後 close のパニックを避ける。
- ストリーミング出力は channel（`<-chan []byte`）で公開し、プロセス終了/Close 時に必ず close する。
- TDD: 失敗テスト→失敗確認→最小実装→通過確認→コミット。PTY 実装テストは実シェル（`/bin/sh`）を起動して echo を検証（タイムアウト付き）。WebSocket はフェイクランナー + `httptest` + gorilla dialer で往復テスト。
- 各タスクのコミットメッセージ末尾に必ず:
  ```
  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
  ```

## 既存コードの前提（確定済み）

- `domain.Workspace`: `Panes() []*Pane`（要素は `*domain.Pane`）。`domain.Pane`: `ID() PaneId`, `Directory() DirectoryPath`, `Slot() SlotIndex`, `Commands() []StartupCommand`。`StartupCommand`: `Command() string`, `AutoRun() bool`。VO は `String()`/`Int()`。
- `domain.WorkspaceRepository`（`FindByID` 等）、`domain.ErrWorkspaceNotFound`。
- `core/application/port`: `IDGenerator`, `AppStateStore`（`Load`/`SetLastOpened`）。
- `core/application/command`: 既存の永続化ハンドラ群（Create/Rename/ChangeLayout/AddPane/RemovePane/SetPaneDirectory/SetPaneStartupCommands/Maximize/Restore/SetLastActivePane）。各 `XxxCommand`/`NewXxxHandler`/`Handle`。`StartupCommandInput{Command string; AutoRun bool}`。
- `core/application/query`: `WorkspaceDTO`/`PaneDTO`/`StartupCommandDTO`、`GetWorkspaceHandler`/`ListWorkspacesHandler`/`GetLastOpenedWorkspaceHandler`。
- `core/application/apptest`: `FakeRepo`(+`SaveCallCount`/`LastSavedID`), `FakeIDGen`, `FakeAppStateStore`。
- `core/infrastructure/jsonstore`: `NewWorkspaceRepository(baseDir)`, `DefaultBaseDir()`, `NewAppStateStore(baseDir)`。
- `scripts/dev.sh web` は `go run ./apps/web` を呼ぶ（apps/web が存在すれば起動）。

---

## Task 1: TerminalRunner ポート + セッションレジストリ + フェイク

**Files:**
- Create: `core/application/port/terminal_runner.go`
  - `type TerminalStartRequest struct { SessionID string; Dir string; Shell string; Cols uint16; Rows uint16 }`
  - `type TerminalSession interface { ID() string; Write(data []byte) error; Resize(cols, rows uint16) error; Output() <-chan []byte; Done() <-chan struct{}; Close() error }`
    - `Output()`: プロセス出力のチャンク。プロセス終了/Close で close される。
    - `Done()`: セッション完全終了で close される。
  - `type TerminalRunner interface { Start(ctx context.Context, req TerminalStartRequest) (TerminalSession, error) }`
- Create: `core/application/session/registry.go`
  - `type Registry struct { mu sync.RWMutex; sessions map[string]port.TerminalSession }`
  - `func NewRegistry() *Registry`
  - `func (r *Registry) Add(id string, s port.TerminalSession)`、`Get(id string) (port.TerminalSession, bool)`、`Remove(id string)`、`IDs() []string`、`CloseAll()`（全 Close + クリア）。
- Create: `core/application/apptest/terminal_fakes.go`
  - `FakeTerminalSession`: `port.TerminalSession` 実装。`NewFakeTerminalSession(id string) *FakeTerminalSession`。`Write` は受信データを記録（`Writes [][]byte`、`mu` 保護）し、さらに `out` チャネルへエコー（テストで出力検証可能に）。`Resize` は最後の cols/rows を記録（`LastCols`/`LastRows`）。`Output()` は `out`、`Done()` は `done`。`Close()` は冪等で `out`/`done` を close。
  - `FakeTerminalRunner`: `port.TerminalRunner` 実装。`NewFakeTerminalRunner() *FakeTerminalRunner`。`Start` は `FakeTerminalSession`(id=req.SessionID) を生成して `Started []TerminalStartRequest` に記録し返す。`StartErr error` を設定すると Start がそれを返す。
  - コンパイル時アサーション `var _ port.TerminalSession = (*FakeTerminalSession)(nil)`、`var _ port.TerminalRunner = (*FakeTerminalRunner)(nil)`。
- Test: `core/application/session/registry_test.go` — Add/Get/Remove/IDs/CloseAll の挙動、並行 Add/Get が race detector で安全（`go test -race` 前提の単純な並行アクセス）。

**Commit:** `feat(application): add TerminalRunner port, session Registry, terminal fakes`

---

## Task 2: ランタイムコマンドハンドラ

**Files:**
- Create: `core/application/command/runtime_errors.go` — `var ErrSessionNotFound = errors.New("terminal session not found")`。
- Create: `core/application/command/open_workspace.go`
  - `type OpenWorkspaceCommand struct { WorkspaceID string }`
  - `type OpenedPane struct { PaneID string }`
  - `type OpenWorkspaceResult struct { Panes []OpenedPane }`
  - `type OpenWorkspaceHandler struct { repo domain.WorkspaceRepository; runner port.TerminalRunner; registry *session.Registry; state port.AppStateStore; shell string; cols, rows uint16 }`
  - `func NewOpenWorkspaceHandler(repo, runner, registry, state port.AppStateStore, shell string) *OpenWorkspaceHandler`（cols/rows は既定 80x24 を内部設定）
  - `Handle(ctx, cmd) (OpenWorkspaceResult, error)`: `repo.FindByID`（未存在は `ErrWorkspaceNotFound`）→ pane を slot 昇順で走査 → 各 pane: 既存セッションがあれば `Close`+`Remove` → `runner.Start(ctx, TerminalStartRequest{SessionID: paneID, Dir: pane.Directory().String(), Shell: h.shell, Cols: h.cols, Rows: h.rows})` → `registry.Add(paneID, sess)` → autoRun な StartupCommand を順に `sess.Write([]byte(cmd.Command()+"\n"))` → 結果に paneID を追加。全 pane 後 `state.SetLastOpened(ctx, cmd.WorkspaceID)`。Start エラー時はそれまで開いたセッションを Close してエラー返却。
- Create: `core/application/command/write_to_pane.go` — `WriteToPaneCommand{ PaneID string; Data []byte }`、`WriteToPaneHandler{ registry }`、`Handle`: `registry.Get`→`Write`、未登録は `ErrSessionNotFound`。
- Create: `core/application/command/resize_pane.go` — `ResizePaneCommand{ PaneID string; Cols, Rows uint16 }`、`Handle`: Get→`Resize`、未登録は `ErrSessionNotFound`。
- Create: `core/application/command/close_pane.go` — `ClosePaneCommand{ PaneID string }`、`Handle`: Get→`Close`+`registry.Remove`、未登録は `ErrSessionNotFound`。
- Tests（各 `_test.go`、`apptest.FakeTerminalRunner`/`FakeRepo`/`FakeAppStateStore` 使用）:
  - OpenWorkspace: 各 pane に Start が呼ばれ registry に登録、autoRun コマンドのみが `Write` される（autoRun=false は送られない）、`state` に last-opened が記録、未存在 WS でエラー。
  - WriteToPane/ResizePane/ClosePane: 正常系（フェイクセッションの記録を検証）、未登録 pane で `ErrSessionNotFound`。

**Commit:** `feat(application): add runtime command handlers (Open/Write/Resize/Close)`

---

## Task 3: creack/pty による TerminalRunner 実装

**Files:**
- 依存追加: `go get github.com/creack/pty@latest`（`go.mod`/`go.sum` 更新）。
- Create: `core/infrastructure/terminal/pty_runner.go`
  - `package terminal`
  - `type Runner struct { defaultShell string }`、`func NewRunner() *Runner`（defaultShell = `os.Getenv("SHELL")`、空なら `/bin/sh`）。
  - `func (r *Runner) Start(ctx context.Context, req port.TerminalStartRequest) (port.TerminalSession, error)`:
    - shell = req.Shell || r.defaultShell。`cmd := exec.CommandContext(ctx, shell)`、`cmd.Dir = req.Dir`（空なら未設定）。
    - `f, err := pty.Start(cmd)`。`pty.Setsize(f, &pty.Winsize{Cols: req.Cols, Rows: req.Rows})`（cols/rows が 0 のときは設定スキップ）。
    - `ptySession` を生成（下記）。出力ポンプ goroutine: `f.Read` ループ→`out` チャネルへ。EOF/エラーで `out` を close、プロセス `Wait` 後 `done` を close。
  - `type ptySession struct {...}` が `port.TerminalSession` を実装:
    - `ID() string`（req.SessionID）。
    - `Write(data) error`: `f.Write`（`mu` で Close と排他）。
    - `Resize(cols, rows) error`: `pty.Setsize`。
    - `Output() <-chan []byte` / `Done() <-chan struct{}`。
    - `Close() error`: 冪等（`sync.Once`）。`cmd.Process.Kill()` + `f.Close()`。出力ポンプ goroutine が `out`/`done` を閉じる責務を持つ（二重 close 回避）。
  - コンパイル時アサーション `var _ port.TerminalSession = (*ptySession)(nil)`、`var _ port.TerminalRunner = (*Runner)(nil)`。
- Test: `core/infrastructure/terminal/pty_runner_test.go`
  - `TestRunnerEchoRoundTrip`: `NewRunner().Start(ctx, req{SessionID:"s1", Dir: t.TempDir(), Shell:"/bin/sh", Cols:80, Rows:24})` → `Write([]byte("echo hello123\n"))` → `Output()` から `select` でタイムアウト（例 5s）付きで読み、累積バイトに `hello123` が含まれたら成功 → `Close()`。`Done()` が閉じることも確認。
  - `TestRunnerSetsDir`: `Dir` を tempdir にして `Write("pwd\n")`、出力に tempdir パス（の basename）が現れることを確認（macOS の `/private` シンボリックリンク差異に配慮し basename で照合）。
  - 注: これらは実シェル統合テスト。CI/macOS/Linux で動く前提。

**Commit:** `feat(terminal): add creack/pty TerminalRunner implementation`

---

## Task 4: apps/web — HTTP 基盤 + ワークスペース系エンドポイント

**Files:**
- Create: `apps/web/server.go`
  - `type Deps struct { Create *command.CreateWorkspaceHandler; Rename *command.RenameWorkspaceHandler; ChangeLayout *command.ChangeLayoutHandler; Maximize *command.MaximizePaneHandler; Restore *command.RestoreLayoutHandler; SetLastActive *command.SetLastActivePaneHandler; Get *query.GetWorkspaceHandler; List *query.ListWorkspacesHandler; GetLastOpened *query.GetLastOpenedWorkspaceHandler; AddPane *command.AddPaneHandler; RemovePane *command.RemovePaneHandler; SetDir *command.SetPaneDirectoryHandler; SetCmds *command.SetPaneStartupCommandsHandler; Open *command.OpenWorkspaceHandler; Write *command.WriteToPaneHandler; Resize *command.ResizePaneHandler; ClosePane *command.ClosePaneHandler; Registry *session.Registry }`
  - `func NewMux(d Deps) *http.ServeMux` — 全ルートを登録（Task 4/5/6 で順次埋める）。
  - 非公開ヘルパ `writeJSON(w, status, v)`、`readJSON(r, &v) error`、`mapErr(w, err)`（`domain.ErrWorkspaceNotFound`→404、`command.ErrSessionNotFound`→404、VO/検証エラー→400、その他→500、JSON `{ "error": "..." }`）。
  - 本タスクで登録するルート（`net/http` メソッド付きパターン）:
    - `GET /api/workspaces` → List → `[]WorkspaceDTO`
    - `POST /api/workspaces` → body `{name, layout}` → Create → 201 `{id}`
    - `GET /api/workspaces/{id}` → Get → `WorkspaceDTO`（404 可）
    - `PATCH /api/workspaces/{id}` → body `{name?, layout?}` → Rename/ChangeLayout（与えられたフィールドのみ適用）→ 204
    - `POST /api/workspaces/{id}/maximize` → body `{paneId}` → Maximize → 204
    - `POST /api/workspaces/{id}/restore` → Restore → 204
    - `POST /api/workspaces/{id}/active-pane` → body `{paneId}` → SetLastActive → 204
    - `GET /api/last-opened` → GetLastOpened → 200 `{found: bool, workspace?: WorkspaceDTO}`
- Test: `apps/web/server_test.go` — `httptest` で `NewMux` に**フェイク/インメモリ依存**（`jsonstore` を `t.TempDir()` で実体化、`apptest.FakeIDGen`、`apptest.FakeTerminalRunner`、`session.NewRegistry`、`apptest.FakeAppStateStore` または実 `jsonstore.AppStateStore`）を渡してハンドラを組み立て、create→list→get の往復、404、PATCH によるリネーム、最後に各エンドポイントのステータスコードを検証。

**Commit:** `feat(web): add HTTP server, JSON helpers, workspace endpoints`

---

## Task 5: apps/web — ペイン/ランタイム系エンドポイント

**Files:**
- Modify: `apps/web/server.go`（`NewMux` にルート追加）:
  - `POST /api/workspaces/{id}/panes` → body `{directory, slot, commands:[{command,autoRun}]}` → AddPane → 201 `{paneId}`
  - `DELETE /api/workspaces/{id}/panes/{paneId}` → RemovePane → 204
  - `PUT /api/workspaces/{id}/panes/{paneId}/directory` → body `{directory}` → SetPaneDirectory → 204
  - `PUT /api/workspaces/{id}/panes/{paneId}/commands` → body `{commands:[{command,autoRun}]}` → SetPaneStartupCommands → 204
  - `POST /api/workspaces/{id}/open` → Open → 200 `{panes:[{paneId}]}`
- Test: `apps/web/server_test.go`（追加）— add pane→get で反映、remove→get で消える、set directory/commands の反映、open のレスポンス、不正 body で 400、未存在で 404。

**Commit:** `feat(web): add pane and runtime (open) endpoints`

---

## Task 6: apps/web — WebSocket ペイン I/O

**Files:**
- 依存追加: `go get github.com/gorilla/websocket@latest`。
- Create: `apps/web/ws.go`
  - `func (d Deps) handlePaneIO(w http.ResponseWriter, r *http.Request)` — `{paneId}` を取得、`Registry.Get(paneId)`（無ければ 404）、`websocket.Upgrader` でアップグレード。
  - 接続後:
    - 出力ポンプ goroutine: `for chunk := range sess.Output() { ws.WriteMessage(websocket.BinaryMessage, chunk) }`、チャネル close でループ終了→ws を閉じる。
    - 入力ループ: `ws.ReadMessage()`。`TextMessage` の JSON `{ "type":"input", "data":"<base64 or raw>" }` → `WriteToPaneCommand`（data は文字列をそのままバイト列に）；`{ "type":"resize", "cols":N, "rows":M }` → `ResizePaneCommand`。読み取りエラー/切断でループ終了。
    - 終了時に goroutine をクリーンに停止（`sess.Done()` or context で連動）。二重 Close・close 済みチャネルへの書き込みを避ける。
  - `NewMux` に `GET /api/panes/{paneId}/io` を登録。
- Test: `apps/web/ws_test.go` — `httptest.NewServer(NewMux(deps))`、`apptest.FakeTerminalRunner` でセッションを registry に登録（または open 経由）、`gorilla/websocket` の Dialer で接続、`{type:"input","data":"hi"}` を送信→フェイクセッションが out にエコー→クライアントが受信し `hi` を確認。`{type:"resize",cols,rows}` 送信→フェイクセッションの `LastCols/LastRows` が更新されることを別経路（registry 経由でセッション取得）で確認。

**Commit:** `feat(web): add WebSocket pane I/O bridge`

---

## Task 7: apps/web — 組み立て + main + スモーク

**Files:**
- Create: `apps/web/app.go`
  - `func BuildDeps(baseDir string) (Deps, error)` — `jsonstore.NewWorkspaceRepository(baseDir)`、`jsonstore.NewAppStateStore(baseDir)`、`terminal.NewRunner()`、`session.NewRegistry()`、`uuidIDGen`（下記）を生成し全ハンドラを wire して `Deps` を返す。
  - 簡易 ID 生成 `type uuidIDGen struct{}`（`port.IDGenerator`）— 衝突しにくい ID を返す（`crypto/rand` で 16 バイト→hex、stdlib のみ）。`core/infrastructure` でなく apps/web 内のローカル実装でよい。
- Create: `apps/web/main.go`
  - `package main`。`func main()`: `baseDir, _ := jsonstore.DefaultBaseDir()`（環境変数 `MULTI_TERMINALS_DIR` で上書き可）、`BuildDeps`、`NewMux`、`addr := ":" + portFromEnv("8080")`、`http.ListenAndServe`。起動ログ出力。
- Test: `apps/web/app_test.go` — `BuildDeps(t.TempDir())` が成功し、`NewMux` 経由で `GET /api/workspaces` が 200 + `[]` を返すスモーク。`POST /api/workspaces` → `GET /api/workspaces/{id}` の実体往復（jsonstore がファイルに書く）。
- 動作確認（手動・計画外）: `scripts/dev.sh web` で起動できること。

**Commit:** `feat(web): wire dependencies, add main entrypoint and smoke test`

---

## 完了基準

- `go build ./...`、`go vet ./...`、`go test ./...`（`-race` 推奨）全 PASS。
- `core/domain`・`core/application` は依然 stdlib のみ。外部依存は `creack/pty`（terminal）と `gorilla/websocket`（web）に限定。
- `go run ./apps/web` でサーバが起動し、`GET /api/workspaces` が応答する。

## 次の計画（スコープ外）

- frontend（Svelte + Vite + xterm.js）: 共有 UI、`TerminalTransport`（WebSocket 実装）、レイアウト/ペイン/最大化 UI。ブラウザで end-to-end 検証（chrome-devtools）。
- apps/wails: 同じ core を Go バインディングで。
