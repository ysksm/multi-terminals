# エージェント稼働状況インジケータ 設計

日付: 2026-07-07
状態: 承認済み（2026-07-07 のセッションでブレインストーミング実施、要件確定）

## 要件（ユーザー確定事項）

- ワークスペース一覧（左サイドバー）に、claude code / codex が稼働中かどうかを表示する
- 状態も表示する: **active** = 実行中、**wait** = 許可プロンプトを表示して停止中
- ツール別 × 状態別の件数を表示する（例: `claude ●1 ⏸1  codex ●1`）
- 更新はリアルタイム push（変化時に配信）

## 検出の考え方（サーバ側）

端末出力だけでは「claude が動いているか」は確実に分からないため、**プロセス走査**を主信号にする。

- サーバは 1 回の `ps -axo pid=,ppid=,command=`（1 プロセス起動）で全プロセスを取得し、ppid で木を構築
- 各ペインの **シェル PID の子孫**に、コマンド行が `claude` / `codex` にマッチするプロセスがあれば「そのツールが稼働中」
  - claude はネイティブバイナリ（`claude`）でも node 実行（`... .claude ...`）でも拾えるようコマンド行マッチ
- PID の取得: `ptySession` に `Pid()` を実装し、`session.Session` は内側セッションが `Pid()` を持つ場合のみ委譲（**オプショナルインターフェース**。`port.TerminalSession` は変更しない）。リモートペイン（remoteterm）はローカル PID を持たないため対象外（Pid=0 → スキップ）

## 状態モデル（active / wait）

- **wait** = 稼働中 **かつ** スクロールバック末尾（現在画面付近、末尾 2KB）に既知の許可プロンプト文字列があり、**かつ** 直近 2 秒以上新規出力が無い（idle）
  - 「過去にプロンプトが出ただけ」の誤検出を避けるための複合条件
  - パターンは拡張可能なリスト（claude: `Do you want`, `❯ 1. Yes` / codex: `Allow command`, `Approve`）
- **active** = 稼働中で wait でない
- プロセスなしのペインは表示対象外
- 実装のため `session.Session` に `Tail(n)`（末尾バイト取得）と `LastOutputAt()`（最終出力時刻）を追加。drain 時に時刻を記録

## 配信（リアルタイム push）

- **SSE**（`GET /api/agent-status/stream`, `text/event-stream`）を新設。既存 WS はペイン I/O 専用のまま
- 監視ゴルーチン（`agentstatus.Watcher`）が 1.5 秒ごとにスナップショットを算出し、**前回から変化したときだけ**全購読者へ push（購読開始時は即時 1 回）
- スナップショット取得用に `GET /api/agent-status`（プレーン JSON）も用意。SSE が使えない環境（Wails の AssetServer 等）ではフロントがポーリングにフォールバック
- ペイロードは pane 単位（サーバはワークスペースを意識しない = 疎結合）:

```json
{ "panes": { "<paneId>": [ { "tool": "claude", "state": "wait" } ] } }
```

## 表示（フロント）

- フロントは `EventSource` で購読（失敗時は 3 秒間隔ポーリング）
- ワークスペース一覧は既に全ワークスペースの panes を持つ（`GET /api/workspaces` の DTO）ので、**フロント側で pane → workspace 集計**
- 各行にツール別バッジを追加。`●` = active、`⏸` = wait。0 件のツールは出さない。稼働ゼロのワークスペースはバッジなし

```
my-workspace  [4分割]  claude ●1 ⏸1   codex ●1
```

## レイヤ配置

- `core/application/agentstatus` … 検出・状態判定・Watcher（stdlib のみ。`Proc` 型とスキャナ関数型をここで定義 = ポート）
- `core/infrastructure/procscan` … `ps` 実行と出力パース（`agentstatus.Proc` を返す実装）
- `apps/web` … `/api/agent-status`（snapshot）と `/api/agent-status/stream`（SSE）
- `frontend/src/lib/agentStatus.js` … 購読（SSE + ポーリングフォールバック）と workspace 集計（純関数・node:test でテスト）

## スコープ

- 検出対象 OS は Unix（macOS / Linux）。Windows は `procscan` が空を返し、バッジ非表示（エラーにしない）
- リモートペインは対象外（将来の拡張余地）
