# multi-terminals ゼロベース再実装 仕様書（機能一覧 / 技術仕様 / タスク一覧）

- 日付: 2026-06-28
- 目的: 現行実装（v0.2.0）の機能・設計を棚卸しし、**ゼロから作り直す前提**で「何を・どの順序で・どの技術で作るか」を一枚にまとめる。
- 想定読者: 再実装を担当する開発者 / レビュアー。

---

## 0. プロダクト概要

複数のターミナルを 1 ウィンドウで管理し、**フォルダ・起動コマンド・レイアウト・タイトルを保存して翌日でもすぐに環境を再現できる**デスクトップ / Web アプリ。

中心価値: 「翌日でもすぐにターミナル環境を再現できる」こと。

- 1画面 / 左右2分割 / 上下2分割 / 4分割のプリセットレイアウト + アクティブペイン最大化
- ペインごとに作業ディレクトリ・起動コマンド（自動実行/手動）・タイトルを保存
- 「前回開いた状態」を起動時に復元
- 同一の Go コアを **Web 版（単一バイナリ）** と **Wails デスクトップ版** の 2 アダプタで共有
- 端末は OS ネイティブ PTY（Unix=PTY / Windows=ConPTY）

---

## 1. 機能一覧

### 1.1 ワークスペース管理（永続化）
| # | 機能 | 説明 |
| --- | --- | --- |
| F-01 | ワークスペース作成 | 名前 + レイアウトプリセットを指定して新規作成 |
| F-02 | ワークスペース一覧 | 保存済みワークスペースを一覧表示（ID 昇順） |
| F-03 | ワークスペース取得 | 1件の詳細（ペイン・レイアウト・最大化/アクティブ状態）取得 |
| F-04 | ワークスペース改名 | 表示名を変更 |
| F-05 | レイアウト変更 | プリセット変更。既存ペインが新容量・スロット範囲に収まる場合のみ許可 |
| F-06 | ワークスペース削除 | 削除時に起動中の PTY セッションも閉じる。選択中なら表示クリア（インライン2段階確認 UI） |
| F-07 | 前回状態の復元 | 起動時に `lastOpenedWorkspaceId` から前回のワークスペースを復元 |

### 1.2 ペイン管理（永続化）
| # | 機能 | 説明 |
| --- | --- | --- |
| F-08 | ペイン追加 | スロット・作業ディレクトリ・起動コマンド・タイトル（任意）を指定して追加 |
| F-09 | ペイン削除 | ワークスペースから除去（最大化/アクティブ参照も整合的にクリア） |
| F-10 | 作業ディレクトリ編集 | ペインの作業ディレクトリをインライン編集 |
| F-11 | 起動コマンド編集 | コマンド列 + 自動実行/手動フラグをインライン編集 |
| F-12 | ペインタイトル | タイトル付与・ヘッダー表示・インライン編集・保存復元。未設定時はディレクトリ表示（ディレクトリは tooltip に残す） |

### 1.3 レイアウト / フォーカス操作
| # | 機能 | 説明 |
| --- | --- | --- |
| F-13 | プリセットレイアウト | single(1) / split_vertical(2) / split_horizontal(2) / grid_2x2(4) |
| F-14 | アクティブペイン最大化 | 1ペインを最大化表示、元に戻す（restore） |
| F-15 | アクティブペイン記録 | `lastActivePaneId` をサーバーに記録（最大化/復元の基準） |
| F-16 | ペイン間フォーカス移動 | `Ctrl+Shift+矢印` で隣接ペインへフォーカス移動（位置入替ではない）。最大化中・端は無移動 |
| F-17 | サイドバー折りたたみ | トグルで開閉。状態を localStorage に保存し再読込後も維持。折りたたみ時は workspace を全幅化（グリッドを `1fr` に上書きし空トラックを残さない） |

### 1.4 ターミナル（ランタイム）
| # | 機能 | 説明 |
| --- | --- | --- |
| F-18 | ワークスペースを開く | 全ペインに対し PTY 起動。`autoRun:true` の起動コマンドを順次送信。`lastOpened` を更新 |
| F-19 | 端末入出力 | xterm.js ↔ PTY の双方向ストリーム（入力↑ / 出力↓）。GUI 起動でも色が出るよう PTY 子プロセスへ `TERM=xterm-256color`/`COLORTERM=truecolor` を既定補完（呼出側設定値は保持） |
| F-20 | 端末リサイズ | 端末サイズ（cols/rows）変更を PTY に反映 |
| F-21 | セッション継続 | セッションはサーバー側で保持。UI を閉じても継続し、再オープンで復帰 |
| F-22 | スクロールバック復元 | 再接続時に直近の出力（リングバッファ、既定 256KB）をスナップショット送信 |
| F-23 | セッション一覧 | 起動中ペイン ID の一覧取得（再接続用） |
| F-24 | 複数同時接続 | 同一ペインへ複数クライアントが購読可能（ファンアウト、遅い購読者はドロップ） |

### 1.5 配布 / 実行形態
| # | 機能 | 説明 |
| --- | --- | --- |
| F-25 | 単一バイナリ（Web版） | フロントを Go バイナリに埋め込み、UI+API を 1 ポートで配信 |
| F-26 | Wails デスクトップ版 | ネットワークポート不使用。端末 I/O は Go↔JS バインディング + イベント |
| F-27 | クロスプラットフォーム | Windows（ConPTY/PowerShell 既定）/ macOS / Linux |
| F-28 | 環境変数設定 | `PORT` / `MULTI_TERMINALS_DIR` / `MULTI_TERMINALS_SHELL` / `SHELL` / `VITE_API_TARGET` |

---

## 2. 技術仕様

### 2.1 技術スタック
| 区分 | 採用 |
| --- | --- |
| バックエンド言語 | Go 1.26+ |
| アーキテクチャ | DDD レイヤード + クリーンアーキテクチャ + CQRS（モジュラーモノリス） |
| PTY | `github.com/aymanbagabas/go-pty`（Unix=PTY / Windows=ConPTY） |
| Web 通信 | `net/http`（REST）+ `github.com/gorilla/websocket`（端末 I/O） |
| デスクトップ | `github.com/wailsapp/wails/v2`（v2.12.0） |
| フロントエンド | Svelte 5 + Vite 8 |
| 端末描画 | `@xterm/xterm` v6 + `@xterm/addon-fit` |
| 永続化 | JSON ファイル（抽象 Repository 越し。将来 SQLite 差し替え可能） |
| 配布 | 単一バイナリ（go:embed）+ Wails `.app`/`.exe`。CI は GitHub Actions（ネイティブランナー） |

### 2.2 レイヤー構成と依存方向（内向きのみ）
```
core/domain          … Entities, Value Objects, 集約, Repository/ポート（依存ゼロ・stdlib のみ）
core/application     … CQRS Command/Query ハンドラ, ポート, DTO, session ランタイム（stdlib のみ）
core/infrastructure  … JSON 永続化(jsonstore) / ターミナル(go-pty)
apps/web             … net/http REST + gorilla/websocket（薄いアダプタ）+ go:embed SPA
apps/wails           … Wails バインディング（同 core / 同 mux を在プロセス利用）
frontend             … Svelte + xterm.js（Web/Wails 共有、通信層だけ差し替え）
```
- `infrastructure`/`apps` のみ外部依存を持つ。`core/domain`・`core/application` は標準ライブラリのみ。
- PTY もファイル I/O も application 層の **port インターフェース**越しにしか触れない（テスト時はフェイク注入）。

### 2.3 ドメインモデル（DDD）

**集約ルート `Workspace`（永続化対象）**
- 値オブジェクト: `WorkspaceId`, `WorkspaceName`(非空), `LayoutPreset`, `PaneId`, `DirectoryPath`(非空), `StartupCommand{command, autoRun}`, `SlotIndex`(>=0), `PaneTitle`(空許容/最大100ルーン/制御文字禁止/トリム)
- エンティティ `Pane`: `id, directory, slot, title, commands[]`（状態変更は集約内部の非公開セッターのみ）
- `Workspace` 保持状態: `name, layout, panes[], lastActivePaneId(任意), maximizedPaneId(任意)`
- **不変条件**:
  - pane 数 ≤ レイアウト容量（Single=1 / Split*=2 / Grid2x2=4）
  - `SlotIndex` は workspace 内で一意かつ範囲内
  - `lastActivePaneId` / `maximizedPaneId` は存在する pane を指す（または空）
- 主メソッド: `Rename`, `ChangeLayout`(容量検証), `AddPane`, `RemovePane`(参照クリア), `SetPaneDirectory`, `SetPaneTitle`, `SetPaneStartupCommands`, `SetLastActivePane`, `MaximizePane`, `RestoreLayout`
- 復元: `ReconstituteWorkspace(...)` で全不変条件を再検証（永続化からの再構成専用）

**レイアウトプリセット**: `single`(1) / `split_vertical`(2) / `split_horizontal`(2) / `grid_2x2`(4)。`Capacity()` / `IsValid()` を持つ。

**ランタイム集約 `TerminalSession`（永続化しない）**
- PTY プロセスの生存管理。スクロールバック等の端末内容は永続化しない（再現するのは環境であって履歴ではない）。

### 2.4 CQRS（Command / Query 分離・1ファイル1責務）

**永続化 Command（Repository 経由で Workspace を更新）**
`CreateWorkspace` / `RenameWorkspace` / `DeleteWorkspace` / `ChangeLayout` / `AddPane` / `RemovePane` / `SetPaneDirectory` / `SetPaneTitle` / `SetPaneStartupCommands` / `SetLastActivePane` / `MaximizePane` / `RestoreLayout`
（`CreateWorkspace`・`AddPane` は `IDGenerator` ポートで ID 生成）

**ランタイム Command（TerminalSession / Registry）**
- `OpenWorkspace` — pane を slot 昇順に PTY 起動。既存ライブセッションは再利用（autoRun 再送なし）、新規のみ autoRun 送信。失敗時は新規分をロールバック。成功で `SetLastOpened`。既定端末サイズ 80x24
- `WriteToPane` — キー入力を PTY へ（未起動なら `ErrSessionNotFound`）
- `ResizePane` — 端末サイズ変更
- `ClosePane` — `Get→Close→Remove`

**Query（読み取り専用・副作用なし）**
- `ListWorkspaces`（ID 昇順、空でも非 nil スライス）
- `GetWorkspace`
- `GetLastOpenedWorkspace`（前回が削除済みなら「無し」を返す）

### 2.5 ポート（application が定義、infrastructure/adapters が実装）
- `WorkspaceRepository`: `Save / FindByID / List / Delete`（`ErrWorkspaceNotFound`）
- `TerminalRunner`: `Start(ctx, TerminalStartRequest{SessionID,Dir,Shell,Cols,Rows}) (TerminalSession, error)`
- `TerminalSession`: `ID / Write / Resize / Output()<-chan / Done()<-chan / Close()`（Close 冪等）
- `IDGenerator`: `NewID() string`
- `AppStateStore`: `Load() (workspaceID, ok, err) / SetLastOpened(id)`（未知スキーマ version はエラー）
- エラーモデル `apperr.ValidationError`（ドメイン/不変条件違反 → Web で 400）

### 2.6 ランタイム・セッション管理（`core/application/session`）
- `Session`: `TerminalSession` をラップし **スクロールバック・リングバッファ（既定 256KB）** と着脱可能な購読者を保持
  - `Subscribe() -> (snapshot []byte, *Subscription)` 現在のスクロールバックコピー + ライブ購読
  - `Unsubscribe`（冪等） / `Done()` / `Close()`
  - ドレイン goroutine が PTY 出力を読み、リングへ追記しつつ購読者へブロードキャスト。遅い購読者はドロップ（再接続でスクロールバック再生）
- `Registry`: pane ID → Session のスレッドセーフ管理。`Add / Get / Remove / IDs / CloseAll`

### 2.7 永続化（JSON / `core/infrastructure/jsonstore`）
- 保存先: `MULTI_TERMINALS_DIR`（既定: OS ユーザー設定ディレクトリ配下 `multi-terminals/`）
  - ワークスペース: `<baseDir>/workspaces/<id>.json`（1ファイル1ワークスペース）
  - アプリ状態: `<baseDir>/app-state.json`（`lastOpenedWorkspaceId`）
- **スキーマ version フィールド**（現行 `CurrentSchemaVersion = 1`）。未知 version は読み込みエラー
- **アトミック書き込み**（`.tmp` へ書いて rename）、RWMutex でスレッドセーフ
- ID はパスとして安全か検証（区切り文字・`..` 拒否）
- マッパ `toRecord`/`toDomain`。読み込みは `ReconstituteWorkspace` で不変条件を再検証
- **後方互換**: 追加フィールド（例: `title`）は `omitempty`、欠落時は空値で復元（version 据え置き）

### 2.8 ターミナル / PTY（`core/infrastructure/terminal`）
- `go-pty` の `xpty.New()` + `ptmx.Command()` で起動（`CommandContext` は使わない＝セッションがリクエストより長命）
- シェルは `exec.LookPath` で絶対パス解決（go-pty の制約回避）
- 既定シェル: Unix=`$SHELL`→`/bin/sh`、Windows=`$MULTI_TERMINALS_SHELL`→`powershell.exe`
- **端末環境変数の補完** `ensureTerminalEnv(os.Environ())`: `TERM`/`COLORTERM` が未設定なら `xterm-256color`/`truecolor` を既定付与（呼出側設定値は保持）。GUI 起動（Finder/Wails 等）で親に `TERM` が無く dumb-terminal にフォールバックして色が消える問題への対処（既存セッションには非反映＝再起動が必要）
- `pump()` goroutine が PTY 出力を 4KB チャンクで `Output()` チャネルへ。遅い購読側は select default でドロップ。EOF/エラーで `out`/`done` をクローズ（`sync.Once`）
- `Close()` は冪等（kill + PTY クローズ）

### 2.9 Web アダプタ（`apps/web`）
- 設定: `PORT`(既定8080) / `MULTI_TERMINALS_DIR`。SIGINT/SIGTERM で `Registry.CloseAll()` + 10s graceful shutdown
- SPA は `//go:embed all:dist` で埋め込み配信。未ビルド時フォールバック HTML。ディープリンクは index.html へ
- **REST エンドポイント**:

| Method | Path | 対応 |
| --- | --- | --- |
| GET | `/api/workspaces` | ListWorkspaces |
| POST | `/api/workspaces` | CreateWorkspace `{name, layout}` |
| GET | `/api/workspaces/{id}` | GetWorkspace |
| PATCH | `/api/workspaces/{id}` | Rename / ChangeLayout `{name?, layout?}` |
| DELETE | `/api/workspaces/{id}` | DeleteWorkspace（204） |
| POST | `/api/workspaces/{id}/maximize` | MaximizePane `{paneId}` |
| POST | `/api/workspaces/{id}/restore` | RestoreLayout |
| POST | `/api/workspaces/{id}/active-pane` | SetLastActivePane `{paneId}` |
| POST | `/api/workspaces/{id}/open` | OpenWorkspace → `{panes:[{paneId}]}` |
| POST | `/api/workspaces/{id}/panes` | AddPane `{directory, slot, title?, commands[]}` |
| DELETE | `/api/workspaces/{id}/panes/{paneId}` | RemovePane |
| PUT | `/api/workspaces/{id}/panes/{paneId}/directory` | SetPaneDirectory |
| PUT | `/api/workspaces/{id}/panes/{paneId}/title` | SetPaneTitle |
| PUT | `/api/workspaces/{id}/panes/{paneId}/commands` | SetPaneStartupCommands |
| GET | `/api/last-opened` | GetLastOpenedWorkspace → `{found, workspace?}` |
| GET | `/api/sessions` | 起動中 pane ID 一覧 → `{paneIds:[]}` |
| GET (WS) | `/api/panes/{paneId}/io` | 端末 I/O（WebSocket アップグレード） |

- **WebSocket プロトコル** `/api/panes/{paneId}/io`:
  - 接続時に hub を購読 → 非空ならスナップショットを 1 バイナリフレーム送信 → 出力ポンプ開始
  - Client→Server（JSON）: `{"type":"input","data":"<utf8>"}` / `{"type":"resize","cols":N,"rows":N}`
  - Server→Client: 生の端末出力をバイナリフレーム。セッション終了で Close フレーム（1000 "session ended"）
  - 複数クライアント可。`sync.Once` で冪等クリーンアップ

### 2.10 Wails アダプタ（`apps/wails`）
- 同一 REST mux + SPA を **在プロセス**配信（AssetServer、ネットワークポート不使用）
- 端末 I/O は Go↔JS バインディング + ランタイムイベント:
  - バインディング: `PaneSubscribe(paneId)` / `PaneUnsubscribe(paneId)` / `PaneWrite(paneId, data)` / `PaneResize(paneId, cols, rows)`
  - イベント: `pane:{paneId}`（base64 チャンク、購読時スナップショット含む）/ `pane:{paneId}:done`（終了通知）
- `startup()` で Wails context 取得、`shutdown()` で PTY セッションクローズ
- ウィンドウ 1200×800

### 2.11 フロントエンド（`frontend`、Web/Wails 共有）
- スタック: Svelte 5（`$state`/`$effect`）+ Vite 8 + `@xterm/xterm` v6 + `@xterm/addon-fit`
- `vite.config.js`: `/api` を `VITE_API_TARGET`(既定 `http://localhost:8080`) へプロキシ（`ws:true`）
- 主要モジュール:
  - `lib/api.js`: REST クライアント（`listWorkspaces`/`createWorkspace`/`getWorkspace`/`patchWorkspace`/`maximizePane`/`restoreLayout`/`setActivePane`/`lastOpened`/`listSessions`/`addPane`/`removePane`/`setPaneDirectory`/`setPaneTitle`/`setPaneCommands`/`open`/`deleteWorkspace`）。`LAYOUTS` 定義 `{value,label,capacity,cols,rows}` と `layoutOf()`
  - `lib/termTransport.js`: 通信層抽象。`isDesktop()` で Wails/ブラウザ判定。`connectPane(paneId,{onData,onClose,onError})` が統一 IF `{send,resize,close}` を返す。`paneWsURL()` / `b64ToBytes()`
  - `lib/Terminal.svelte`: xterm.js 統合。ResizeObserver で `fit.fit()`→`resize` 送信。`active` で `term.focus()`、`onActivate` でクリック同期。status バッジ
  - `lib/paneNav.js`: 純粋関数 `neighborSlot(slot, cols, rows, direction)`（範囲外は null）
  - `App.svelte`: サイドバー（一覧/作成/2段階削除/折りたたみ）、ツールバー（レイアウト選択/開く/復元/ペイン数）、ペイングリッド（タイトル/ディレクトリ/コマンドのインライン編集・追加フォーム・最大化・未起動プレースホルダ）、`Ctrl+Shift+矢印` ナビ、起動時の last-opened 復元 + ライブセッション再接続
- キーボード: `Ctrl+Shift+矢印`（フォーカス移動）/ タイトル編集の Enter(確定)・Esc(取消)・blur(自動確定)

### 2.12 ビルド / CI / テスト
- スクリプト: `scripts/dev.sh`(web/frontend/build/start/check) / `build-all.sh`(web クロス + ネイティブ Wails) / `build-wails.sh` / `build-mac.sh` / `build.bat`
- Web 単一バイナリ: `frontend` を `apps/web/webui/dist` へビルド埋め込み
- Wails: クロスコンパイル不可（Windows は Windows 上、macOS は macOS 上）
- CI（`.github/workflows/build.yml`）: tag `v*`/手動。macOS/Windows ネイティブランナーで `build-all.sh all`、`release/*` を成果物アップロード
- テスト方針: `go test ./...`（`-race` 含む）。ドメイン/アプリは TDD。端末テストは実シェル起動。フロントは `*.node.test.mjs`（`paneNav` / `termTransport` ヘルパ）

### 2.13 スコープ外（YAGNI）
- 任意のツリー分割（tmux/VSCode 的な再帰分割）— プリセット + 最大化で充足
- 端末スクロールバックの永続化
- マルチユーザー / 認証 / リモート共有
- SQLite 実装（インターフェースのみ用意、実装は将来）
- ペイン位置入替・サイドバー幅ドラッグ・ショートカットのカスタマイズ

---

## 3. タスク一覧（再実装の順序・依存つき）

実装はクリーンアーキテクチャの内側から外側へ。各フェーズはテスト緑を完了条件とする。

### フェーズ 0: プロジェクト基盤
- [ ] T0-1 リポジトリ初期化（`go.mod` go 1.26、ライセンス、`.gitignore`、ディレクトリ雛形 `core/`・`apps/`・`frontend/`・`docs/`・`scripts/`）
- [ ] T0-2 依存追加（`go-pty` / `gorilla/websocket` / `wails/v2`）
- [ ] T0-3 開発スクリプト雛形（`scripts/dev.sh` の web/frontend/build/start/check）

### フェーズ 1: core/domain（TDD・依存ゼロ）
- [ ] T1-1 値オブジェクト: `WorkspaceId`/`PaneId`/`WorkspaceName`/`DirectoryPath`/`SlotIndex`/`StartupCommand`/`PaneTitle`（検証ルール込み・各テスト）
- [ ] T1-2 `LayoutPreset`（4プリセット + `Capacity()`/`IsValid()` + テスト）
- [ ] T1-3 エンティティ `Pane`（非公開セッター、防御的コピー + テスト）
- [ ] T1-4 集約 `Workspace`（全メソッド + 不変条件 + テスト）
- [ ] T1-5 `ReconstituteWorkspace`（再検証 + テスト）
- [ ] T1-6 `WorkspaceRepository` インターフェース + `ErrWorkspaceNotFound`

### フェーズ 2: core/application（CQRS・ポート・TDD）
- [ ] T2-1 ポート定義: `TerminalRunner`/`TerminalSession`/`IDGenerator`/`AppStateStore`
- [ ] T2-2 `apperr.ValidationError`
- [ ] T2-3 永続化 Command（12件）: Create/Rename/Delete/ChangeLayout/AddPane/RemovePane/SetPaneDirectory/SetPaneTitle/SetPaneStartupCommands/SetLastActivePane/MaximizePane/RestoreLayout（各テスト）
- [ ] T2-4 Query（3件）+ DTO: ListWorkspaces/GetWorkspace/GetLastOpenedWorkspace（各テスト）
- [ ] T2-5 ランタイム `session.Session`（スクロールバック・購読 + テスト）+ `session.Registry`（+ テスト）
- [ ] T2-6 ランタイム Command: OpenWorkspace（slot 順・再利用・autoRun・ロールバック・SetLastOpened）/ WriteToPane / ResizePane / ClosePane + `ErrSessionNotFound`（各テスト、フェイク Runner）
- [ ] T2-7 テスト用フェイク（fake repo / fake terminal / fake idgen / fake appstate）

### フェーズ 3: core/infrastructure（ポート実装）
- [ ] T3-1 `jsonstore`: スキーマ/record/version 定義（+ テスト）
- [ ] T3-2 `jsonstore.WorkspaceRepository`（アトミック書込・ID 安全検証・List + テスト）
- [ ] T3-3 `jsonstore.AppStateStore`（Load/SetLastOpened + version 検証 + テスト）
- [ ] T3-4 `jsonstore` マッパ（toRecord/toDomain + ラウンドトリップ + 後方互換テスト）
- [ ] T3-5 `terminal.Runner`（go-pty 起動・LookPath・pump goroutine・`ensureTerminalEnv`(TERM/COLORTERM 補完) + 実シェルテスト）
- [ ] T3-6 既定シェル解決（`shell_unix.go`/`shell_windows.go` ビルドタグ + テスト）
- [ ] T3-7 `IDGenerator` 実装（uuid 等）

### フェーズ 4: apps/web アダプタ
- [ ] T4-1 依存配線 `BuildDeps`（repo/registry/runner/idgen/appstate + 全ハンドラ）
- [ ] T4-2 REST ルーティング + ハンドラ（全16ルート、`mapErr`/`writeJSON`、検証エラー→400）
- [ ] T4-3 WebSocket `/api/panes/{paneId}/io`（購読・スナップショット・input/resize・close + テスト）
- [ ] T4-4 SPA 埋め込み配信 `webui.go`（go:embed・フォールバック・ディープリンク）
- [ ] T4-5 `cmd/main.go`（PORT/DIR・graceful shutdown・CloseAll）
- [ ] T4-6 サーバーテスト（REST + WS + e2e スモーク）

### フェーズ 5: frontend（Svelte / xterm.js）
- [ ] T5-1 プロジェクト初期化（Svelte5 + Vite8 + xterm、`vite.config.js` プロキシ）
- [ ] T5-2 `lib/api.js`（全 REST 関数 + `LAYOUTS`/`layoutOf` + 任意のユニットテスト）
- [ ] T5-3 `lib/termTransport.js`（Web/Wails 切替・`connectPane` + `*.node.test.mjs`）
- [ ] T5-4 `lib/Terminal.svelte`（xterm + FitAddon + ResizeObserver + active/onActivate）
- [ ] T5-5 `lib/paneNav.js`（`neighborSlot` + `*.node.test.mjs`）
- [ ] T5-6 `App.svelte`: サイドバー（一覧/作成/折りたたみ/2段階削除）+ 折りたたみ時グリッド全幅化 CSS（`.app.sidebar-collapsed { grid-template-columns: 1fr }`）
- [ ] T5-7 `App.svelte`: ペイングリッド + レイアウト + 最大化/復元
- [ ] T5-8 `App.svelte`: ペイン追加フォーム + ディレクトリ/コマンド/タイトルのインライン編集
- [ ] T5-9 `App.svelte`: `Ctrl+Shift+矢印` フォーカス移動 + アクティブ表示
- [ ] T5-10 `App.svelte`: 起動時 last-opened 復元 + ライブセッション再接続

### フェーズ 6: apps/wails アダプタ
- [ ] T6-1 `main.go`/`app.go`（同 mux 在プロセス配信、startup/shutdown、wails.json）
- [ ] T6-2 バインディング `PaneSubscribe/Unsubscribe/Write/Resize` + イベント `pane:{id}` / `pane:{id}:done`
- [ ] T6-3 Wails 経路の e2e（フェイク端末でバインディング/イベントのテスト）

### フェーズ 7: ビルド / CI / リリース
- [ ] T7-1 `build-all.sh`（frontend ビルド→埋め込み→web クロス + ネイティブ Wails）
- [ ] T7-2 `build-wails.sh` / `build-mac.sh` / `build.bat`
- [ ] T7-3 GitHub Actions（macOS/Windows ネイティブ、tag `v*`、`release/*` アップロード）
- [ ] T7-4 README / リリースノート / 環境変数表

### フェーズ 8: 仕上げ
- [ ] T8-1 `go test -race ./...` 緑、`scripts/dev.sh check`（build+vet+test）緑
- [ ] T8-2 手動スモーク（Web 単一バイナリ起動 / Wails 起動 / 翌日再現シナリオ）
- [ ] T8-3 後方互換確認（旧 JSON 読み込み）

### 推奨実装順序（マイルストーン）
1. **M1 コア完成**: フェーズ 1–3（ドメイン〜インフラ、`go test` 緑）
2. **M2 Web E2E**: フェーズ 4–5（ブラウザで作成→開く→入力→再現が通る）
3. **M3 デスクトップ**: フェーズ 6（同 UI が Wails で動く）
4. **M4 配布**: フェーズ 7–8（単一バイナリ/Wails 成果物 + CI）

---

## 4. 受け入れ基準（Definition of Done）
- ワークスペースを作成→ペイン追加（ディレクトリ/コマンド/タイトル）→開く→端末入力ができ、アプリ再起動後に **前回状態が復元** される。
- 4 レイアウト + 最大化/復元 + `Ctrl+Shift+矢印` 移動が動作する。
- 同一バイナリで Web 配信、Wails で同 UI がポート無しに動作する。
- `go test -race ./...` と frontend の node テストが緑。
- 旧スキーマ（`title` 無し等）の JSON を後方互換で読み込める。
</content>
</invoke>
