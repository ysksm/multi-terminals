# サイドバー折りたたみ + ワークスペース削除 + ペイン間フォーカス移動 設計書

- 日付: 2026-06-28
- 対象ブランチ: feat/build-and-serve（または派生ブランチ）
- ステータス: 承認済み（実装計画へ）

## 目的

複数コンソール運用時の使い勝手を改善する 3 つの UI/CRUD 機能を追加する。

1. 左サイドバーを折りたたみ可能にする（作業領域を広げる）。
2. 作成したワークスペース（ビュー）を削除できるようにする。
3. `Ctrl+Shift+矢印` でペイン間をフォーカス移動できるようにする。

## 決定事項（ブレインストーミング結果）

1. **折りたたみ**: トグルボタンで開閉。状態は `localStorage`（キー `mt.sidebarCollapsed`）に保存し再読込後も維持。
2. **削除**: 一覧の各項目に削除ボタン。**インライン2段階確認**（✕→「削除？」→確定）。削除時は起動中の PTY セッションも閉じる。選択中を削除したら表示をクリア。
3. **移動**: フォーカス移動（位置入替ではない）。`Ctrl+Shift+矢印`で矢印方向の隣ペインをアクティブにし端末へフォーカス。

## 前提（既存コードの確認結果）

- フロントは `frontend/src/App.svelte`（`<aside class="sidebar">` ＋ `<main class="workspace">`）、`frontend/src/lib/api.js`、`frontend/src/lib/Terminal.svelte`。
- レイアウト定義は `api.js` の `LAYOUTS`: `single(1x1)`, `split_vertical(cols2,rows1)`, `split_horizontal(cols1,rows2)`, `grid_2x2(cols2,rows2)`。各要素 `{value,label,capacity,cols,rows}`。`layoutOf(value)` ヘルパあり。
- ペインは `slot`（0 始まり）を持ち、グリッドは slot 昇順に描画。`(row,col) = (slot/cols, slot%cols)`。
- アクティブ/最大化: バックエンドに `SetActivePane`（`lastActivePaneId`）・`MaximizePane`（`maximizedPaneId`）。フロントは `maximized = current?.maximizedPaneId` を派生。現状フロントにペインのアクティブ表示は無い。
- 削除の土台: `domain.WorkspaceRepository.Delete(ctx, id)`（`repository.go:17`）と jsonstore 実装（`workspace_repository.go:164`）は存在。**上位（コマンド/Web/フロント）が未実装**。
- セッション: `session.Registry` に `Get(id)`, `Remove(id)`, `Session.Close()`。`command.ClosePaneHandler` が `Get→Close→Remove` の参照実装。
- `web.Deps`（`server.go`）に各ハンドラを保持、`BuildDeps`（`apps/web/app.go`）で配線。`mapErr`/`writeJSON` ヘルパあり。
- `Terminal.svelte` は `term`（xterm）をローカル保持。`@xterm/xterm` の `Terminal` は `focus()` を持つ。

## アーキテクチャ

機能1・3 はフロントのみ。機能2 はアプリ→Web→フロントのフルスタック（ドメイン/インフラは既存 `Delete` を利用）。

### 機能1: サイドバー折りたたみ（フロントのみ）

`frontend/src/App.svelte`:
- 状態 `let sidebarCollapsed = $state(localStorage.getItem('mt.sidebarCollapsed') === '1')`。
- `function toggleSidebar()` で反転し `localStorage.setItem('mt.sidebarCollapsed', sidebarCollapsed ? '1' : '0')`。
- ルート要素 `<div class="app">` に `class:sidebar-collapsed={sidebarCollapsed}` を付与。
- トグルボタン（`☰`）をメインのツールバー左端に常時配置（折りたたみ中もメイン側にあるので押せる）。`aria-label="サイドバー切替"`、`aria-expanded={!sidebarCollapsed}`。
- CSS: `.app.sidebar-collapsed .sidebar { display: none }`。メインは既存レイアウトのまま全幅化（`.app` が flex/grid なら sidebar 非表示で main が伸びる）。

### 機能2: ワークスペース削除（フルスタック）

**アプリケーション層**（`core/application/command/delete_workspace.go`）:
- `DeleteWorkspaceCommand{WorkspaceID string}`。
- `DeleteWorkspaceHandler{repo domain.WorkspaceRepository; registry *session.Registry}`、`NewDeleteWorkspaceHandler(repo, registry)`。
- `Handle(ctx, cmd) error`:
  1. `domain.NewWorkspaceId` で検証（失敗は `apperr.Validation`）。
  2. `repo.FindByID` で取得（存在しなければそのエラーを返す）。
  3. 各 pane について `registry.Get(paneID)` が見つかれば `Close()`＋`registry.Remove(paneID)`（起動中シェルを閉じ孤児化を防ぐ。`Close` エラーは握りつぶさずログ相当の扱いはせず処理継続でよい—ClosePane と整合）。
  4. `repo.Delete(ctx, id)`。エラーはラップして返す。
- 注: last-opened が消えても `GetLastOpenedWorkspace` は存在チェック済みのため追加対応不要。

**Web 層**:
- `web.Deps` に `DeleteWorkspace *command.DeleteWorkspaceHandler` を追加。`BuildDeps` で `command.NewDeleteWorkspaceHandler(repo, reg)` を配線（`reg` は既存の `session.NewRegistry()`）。
- ルート `DELETE /api/workspaces/{id}` → `handleDeleteWorkspace`：`DeleteWorkspaceCommand{WorkspaceID: id}` を実行。成功は `204 No Content`、失敗は `mapErr`。

**フロント**:
- `api.js`: `deleteWorkspace: (id) => req('DELETE', `/api/workspaces/${id}`)`。
- `App.svelte`: 一覧の各 `<li>` に削除コントロール。状態 `let confirmingDeleteId = $state(null)`。
  - 通常は小さな `✕` ボタン。クリックで `confirmingDeleteId = w.id`（その項目だけ「削除？」確定ボタン＋取消に切替）。
  - 確定で `guard(async () => { await api.deleteWorkspace(w.id); if (current?.id === w.id) current = null; confirmingDeleteId = null; await reloadWorkspaces() })`。
  - 別項目をクリック/取消で `confirmingDeleteId = null`。
  - `reloadWorkspaces`（既存の一覧再取得処理に合わせる。無ければ `listWorkspaces` 再呼び出し）。

### 機能3: Ctrl+Shift+矢印 ペイン間フォーカス移動（フロントのみ）

**純粋関数**（`frontend/src/lib/paneNav.js`、テスト可能に分離）:
```
neighborSlot(slot, cols, rows, direction): number | null
```
- `(row,col) = (Math.floor(slot/cols), slot%cols)`。
- direction に応じ row/col を ±1。範囲外（`row<0||row>=rows||col<0||col>=cols`）は `null`。
- それ以外は `row*cols + col` を返す。

**`App.svelte`**:
- 状態 `let activePaneId = $state(null)`。ワークスペース選択/再取得時に `current.lastActivePaneId ?? current.panes[0]?.id ?? null` で初期化。
- アクティブセルをハイライト（`class:active-cell={cell.pane && cell.pane.id === activePaneId}`、CSS で枠線）。
- `onMount` で `window.addEventListener('keydown', onKey, true)`（capture フェーズ）、`onDestroy`/cleanup で解除。
- `onKey(e)`:
  - 条件: `e.ctrlKey && e.shiftKey && !e.altKey && !e.metaKey` かつ `e.key` が `ArrowLeft/Right/Up/Down`。
  - `current` が無い、または最大化中（`maximized`）は無視。
  - `e.preventDefault(); e.stopPropagation()`（xterm にキーを渡さない）。
  - アクティブペインの slot を求め、`layoutOf(current.layout)` の `cols/rows` と direction で `neighborSlot` を計算。
  - 隣 slot に存在するペインを `current.panes.find(p => p.slot === target)` で探し、あれば `activePaneId = pane.id`（起動済みなら当該 `Terminal` がフォーカスを受ける）。
- 各 `Terminal` に `active={cell.pane.id === activePaneId}` と `onActivate={() => activePaneId = cell.pane.id}` を渡す。

**`Terminal.svelte`**:
- プロップに `active`（boolean, 既定 false）と `onActivate`（関数, 任意）を追加。
- `$effect(() => { if (active && term) term.focus() })`（起動済みのとき active で xterm にフォーカス）。
- 端末初期化時に `host`（または xterm のラッパ要素）の focus を検知して `onActivate?.()` を呼ぶ（クリックでアクティブ同期）。最小実装として xterm の `textarea` への `focus` を `term.textarea?.addEventListener('focus', ...)`、または `term.onSelectionChange` ではなく単純に `host` の `focusin` を購読。

## データフロー

```
[折りたたみ] ☰クリック -> sidebarCollapsed 反転 -> localStorage 保存 -> .app クラス切替

[削除] ✕ -> confirmingDeleteId 設定 -> 確定 -> DELETE /api/workspaces/{id}
  -> DeleteWorkspaceHandler: FindByID -> 各pane registry.Close+Remove -> repo.Delete
  -> 204 -> フロント: 選択中ならクリア + 一覧再取得

[移動] Ctrl+Shift+矢印 (capture) -> neighborSlot 計算 -> activePaneId 更新
  -> Terminal active=true -> term.focus()
```

## エラーハンドリング

- `DeleteWorkspaceHandler`: id 検証失敗は `apperr.Validation`（Web で 400）。`FindByID` の not-found は既存方針（`mapErr`）。`Session.Close` 失敗で処理を中断しない（残りも閉じ、最終的に `repo.Delete` を試みる）。
- フロント削除失敗時は既存 `error` 表示で通知し、`confirmingDeleteId` をリセット。
- 移動: 隣が無い/範囲外は何もしない。最大化中は無視。`preventDefault` で端末への誤入力を防止。

## テスト戦略

- **アプリ**: `DeleteWorkspaceHandler` テスト（fake repo + `session.Registry` に fake `port.TerminalSession` を登録 → 削除で「repo から消える」「登録セッションが Close+Remove される」「id 検証エラー」）。`apps/wails/app_test.go` の `fakeTerm` と同型のフェイクを利用可。
- **Web**: `DELETE /api/workspaces/{id}`（204、GET 一覧から消える、未知 id の挙動）。
- **フロント**: `paneNav.js` の `neighborSlot` を node テストで検証（各レイアウト・各方向・端の非移動）。折りたたみ永続化・2段階削除・キー移動は手動スモーク。
- 既存 `go test ./...` 緑、`npm run build` 成功、web スモーク（`GET /`=200）。

## スコープ外（YAGNI）

- サイドバー幅のドラッグ調整、複数選択一括削除、削除アンドゥ/ゴミ箱。
- ペイン位置の入れ替え（今回はフォーカス移動のみ）。
- アクティブペインのサーバー永続化（`setActivePane` 連携は今回行わない。フロント内状態とフォーカスのみ）。
- ショートカットのカスタマイズ設定。

## 成果物一覧

- `core/application/command/delete_workspace.go`（+テスト）
- `apps/web/server.go`（ルート/ハンドラ/Deps フィールド）、`apps/web/app.go`（配線）、関連テスト
- `frontend/src/lib/api.js`（deleteWorkspace）
- `frontend/src/lib/paneNav.js`（+ node テスト）
- `frontend/src/lib/Terminal.svelte`（active/onActivate）
- `frontend/src/App.svelte`（折りたたみ、削除UI、アクティブ表示、キーハンドラ）
