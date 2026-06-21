# Multi-Terminals 設計仕様書

- **日付**: 2026-06-21
- **ステータス**: 承認済み（実装計画フェーズへ）

## 1. 目的とゴール

1 ウィンドウ内で複数のターミナルを扱えるターミナルアプリを作る。

中心的な価値は **「翻日でもすぐにターミナル環境を再現できる」** こと:

- フォルダ（作業ディレクトリ）を覚える
- 起動コマンドを保存しておく
- 1 画面 / 2 分割 / 4 分割などのレイアウトで開く
- どのスロットにどのパスのターミナルを開くかを指定できる
- 前回開いた状態（ワークスペース・レイアウト・最大化状態）で開ける

UI は GUI 操作を伴うため、**Svelte + Go** で **Web アプリ** と **Wails アプリ**の両方を提供する。
内部は **CQRS（Command / Query 分離）**、適用箇所は **DDD レイヤードアーキテクチャ優先 + クリーンアーキテクチャ**で構成する。

## 2. 主要な意思決定（確定事項）

| 項目 | 決定 |
| --- | --- |
| アーキテクチャ | 共有 Go コア + 薄い 2 アダプタ（モジュラーモノリス） |
| PTY 実行場所 | Wails = ローカル、Web = サーバ側 |
| フロントエンド | Svelte で UI を 1 つ作り、通信層だけ Wails/Web で差し替え（UI 共有） |
| 永続化 | JSON ファイル（抽象 Repository 越し。将来 SQLite 差し替え可能） |
| 分割モデル | プリセット中心（1 / 2 分割 / 4 分割）＋ アクティブターミナルの最大化機能 |
| 起動コマンド挙動 | ペイン毎に「自動実行 / 手動」を選択 |
| 実装順 | core ドメイン層から（テスト駆動でエンドツーエンド検証） |

## 3. アーキテクチャ（クリーンアーキテクチャ）

依存方向は内向きのみ。`infrastructure` は port を実装して注入する（依存性逆転）。

```
domain          … Entities, Value Objects, Domain Services, Repository/Port インターフェース（依存ゼロ）
application     … Command Handler / Query Handler（CQRS）、DTO、ポート定義
infrastructure  … JSON リポジトリ実装、PTY ランナー実装、IDGen / Clock
interface       … Wails バインディング / HTTP+WebSocket ハンドラ（薄いアダプタ）
frontend        … Svelte + xterm.js（Wails/Web 共有、通信層だけ差し替え）
```

- `domain` と `application` は Go の純粋パッケージ。テスト駆動で固める。
- PTY もファイル I/O も `application` 層の **port インターフェース**越しにしか触れない（テスト時はモック注入）。

## 4. ドメインモデル（DDD）

### 4.1 集約ルート: `Workspace`（永続化対象）

**Value Objects**

- `WorkspaceId` — 一意識別子
- `WorkspaceName` — 表示名（非空）
- `LayoutPreset` — `Single` / `SplitVertical` / `SplitHorizontal` / `Grid2x2`
- `PaneId` — pane 識別子
- `DirectoryPath` — 作業ディレクトリ（存在確認はアプリ層の責務）
- `StartupCommand` — `{ command: string, autoRun: bool }`
- `SlotIndex` — レイアウト内の位置（0 始まり）

**Entity: `Pane`**

- `PaneId`
- `DirectoryPath`
- `[]StartupCommand`
- `SlotIndex`

**`Workspace` が保持する状態**

- `name`
- `layoutPreset`
- `panes[]`
- `lastActivePaneId`
- `maximizedPaneId`（最大化中の pane。なければ空）

**不変条件**

- pane 数は `layoutPreset` の許容スロット数以下（Single=1, Split*=2, Grid2x2=4）
- `SlotIndex` は workspace 内で重複しない、かつ許容範囲内
- `maximizedPaneId` / `lastActivePaneId` は存在する pane を指す（または空）

### 4.2 ランタイム集約: `TerminalSession`（永続化しない）

PTY プロセスの生存管理を担う。

- `SessionId`
- `PaneId`（どの pane に紐づくか）
- プロセス状態（起動中 / 終了）
- I/O ストリームの参照

スクロールバック等のターミナル内容は永続化しない（再現するのは環境であって履歴ではない）。

## 5. CQRS（Command / Query 分離）

### 5.1 永続化 Command（`Workspace` 集約を Repository 経由で更新）

- `CreateWorkspace`
- `RenameWorkspace`
- `ChangeLayout`
- `AddPane`
- `RemovePane`
- `SetPaneDirectory`
- `SetPaneStartupCommands`
- `MaximizePane` / `RestoreLayout`（最大化解除）
- `SetLastActivePane`

### 5.2 ランタイム Command（`TerminalSession`）

- `OpenWorkspace` — 各 pane に対し PTY を生成し、`autoRun: true` の `StartupCommand` を順次送信
- `WriteToPane` — キー入力を PTY に書き込む
- `ResizePane` — 端末サイズ変更
- `CloseSession` — PTY 終了

### 5.3 Query（読み取り専用、副作用なし）

- `ListWorkspaces`
- `GetWorkspace`
- `GetLastOpenedWorkspace`（「前回開いた状態で開く」用）

### 5.4 規約

- ハンドラは 1 ファイル 1 責務。
- Command と Query で別ポート（書き込み用 / 読み取り用）を定義し、依存を分離する。

## 6. 永続化（JSON、抽象 Repository 越し）

`domain` に `WorkspaceRepository` インターフェースを定義:

```
Save(ctx, workspace) error
FindByID(ctx, id) (Workspace, error)
List(ctx) ([]Workspace, error)
Delete(ctx, id) error
```

`infrastructure` が JSON 実装を提供する。

- 保存先: `~/.config/multi-terminals/workspaces/<id>.json`
- アプリ状態: `~/.config/multi-terminals/app-state.json`（`lastOpenedWorkspaceId` を保持）
- 翻日再現: workspace JSON にフォルダ・起動コマンド・レイアウト・最大化状態を保存。起動時に `app-state.json` から前回の workspace を復元。
- 各 JSON に `version` フィールドを持たせ、将来 SQLite 実装に差し替えてもインターフェースは不変に保つ。

## 7. ターミナル / PTY と通信

### 7.1 ポート

`application` に `TerminalRunner` ポートを定義:

```
Start(dir DirectoryPath) (Session, error)
Write(sessionId, data []byte) error
Resize(sessionId, cols, rows int) error
OnOutput(sessionId, callback func([]byte))
Close(sessionId) error
```

### 7.2 実装

- `infrastructure`: `github.com/creack/pty` でローカル PTY を起動。
- **Web アダプタ**: WebSocket で pane ごとに双方向ストリーム（入力↑ / 出力↓）。サーバ側で PTY 実行。
- **Wails アダプタ**: 同じ core を import。Go ↔ JS バインディング + イベントで I/O。ローカル PTY 実行。

### 7.3 フロントエンド

- `xterm.js` で端末描画。
- `TerminalTransport` インターフェースを定義し、Wails / Web で実装を差し替える。UI 本体は共有。

## 8. プロジェクト構成

```
multi-terminals/
  core/                    # Go: domain + application + infrastructure（テスト駆動）
    domain/
    application/
      command/
      query/
    infrastructure/
      persistence/
      terminal/
  apps/web/                # Go HTTP+WebSocket アダプタ
  apps/wails/              # Wails アダプタ
  frontend/                # Svelte + xterm.js（共有 UI）
  docs/superpowers/specs/
```

## 9. 実装順序

1. `core/domain` — VO・Entity・不変条件をテスト駆動で実装
2. `core/application` — Command / Query ハンドラ、port 定義 ＋ JSON Repository ＋ PTY ランナー実装
3. `apps/web` アダプタ（WebSocket）＋ `frontend`（Svelte / xterm.js）でエンドツーエンド検証
4. `apps/wails` アダプタを同じ core に載せる

各ステップは独立した spec → plan → 実装サイクルにできる。最初の実装計画は **core**（ステップ 1–2）を対象とする。

## 10. スコープ外（YAGNI）

- 任意のツリー分割（VSCode/tmux 的な再帰分割）— プリセット + 最大化で要件を満たす
- ターミナルスクロールバックの永続化
- マルチユーザー / 認証 / リモート共有（Web は当面ローカル利用前提）
- SQLite 実装（インターフェースのみ用意し、実装は将来）
