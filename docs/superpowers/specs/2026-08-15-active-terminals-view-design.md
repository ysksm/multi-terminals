# アクティブなターミナル集約ビュー 設計

日付: 2026-08-15
状態: 実装済み

## 要件（ユーザー確定事項）

- 「アクティブなターミナルだけ集めて表示」したい
- 「アクティブ」の定義は**両方**とし、フィルタで切り替える
  - 既定: **起動中**（ライブセッションを持つ）ペインを全ワークスペース横断で集約
  - 絞り込み: **claude / codex 稼働中**（実行中 ● / 許可待ち ⏸）のペインのみ
- 表示は**新しい「アクティブ」ビュー**として独立させる。既存のワークスペース表示・
  レイアウトプリセット・保存内容には手を入れない

## 実現できる根拠（既存資産）

バックエンドの変更は不要。必要な材料は既に揃っている。

- `GET /api/panes/{paneId}/io`（WebSocket）は **paneId だけで接続でき、ワークスペースに
  紐づかない**。`session.Registry` も paneId をキーにグローバルにライブセッションを保持する
  ため、**別ワークスペースのターミナルを、切り替えずに同一画面へ描画できる**
- `GET /api/sessions` … 生存セッションの paneId 一覧
- `GET /api/workspaces` … 全ワークスペースの pane 情報（id / title / directory / slot / remoteHost）。
  paneId → 表示メタデータの逆引きに使う
- `GET /api/agent-status` + SSE … pane 単位の `[{tool, state}]`。既存の
  `aggregateByWorkspace()` はこれをワークスペース単位に畳んでいるだけなので、
  **畳まずに pane 単位で並べれば「稼働中のターミナル一覧」になる**
- 1 セッションに複数購読者が付けるため（`session.Session.Subscribe`）、ビュー切替で
  端末が再接続してもサーバ側のシェルは生き続け、スクロールバックが復元される

## 集約ロジック（フロント・純関数）

`frontend/src/lib/activeTerminals.js`（node の assert でテスト。`activeTerminals.node.test.mjs`）

- `collectActiveTerminals({workspaces, livePaneIds, agentPanes, agentOnly})`
  - live な paneId を持つペインだけを全ワークスペースから集め、ワークスペース名・
    タイトル・ディレクトリ・エージェント状態を付けて 1 本のリストにする
  - 並び順: **許可待ち(wait) → 実行中(active) → その他(idle)**、同順位は
    ワークスペース名 → スロット → paneId。**ユーザーの操作待ちで止まっているものが常に先頭**
  - `agentOnly` でエージェントの居ないペイン（idle）を除外
- `gridDimensions(count)` … 件数に応じた可変グリッド。列は `ceil(sqrt(n))`（最大 4 列）、
  行は残り。既存のレイアウトプリセット（最大 4 ペイン）とは別物で、永続化しない

## 表示（フロント）

- サイドバー先頭に「🗂 ワークスペース / ⚡ アクティブ」の切替。`Ctrl+Shift+A` でも切替。
  選択は `localStorage`（`mt.viewMode` / `mt.activeAgentOnly`）に保存
- 各セルのヘッダに **ワークスペース名タグ + タイトル（無ければディレクトリ）+
  リモートバッジ + エージェントバッジ（● / ⏸）+ ↗ ボタン**
  - ↗ = そのペインを従来のワークスペース表示で開く（該当ワークスペースへ移動しアクティブ化）
- グリッドは `grid-auto-rows: minmax(240px, 1fr)` + 縦スクロール。件数が増えても
  1 セルが潰れない
- ビュー表示中は 3 秒周期で `GET /api/workspaces` と `GET /api/sessions` を取り直す
  （他ワークスペースでの起動/終了を拾うため）。エージェント状態は既存の SSE で届く

## スコープ外・既知の制約

- `Terminal.svelte` は 1 ペイン 1 xterm インスタンス。大量に並べると重くなるため、
  可変グリッドは最大 4 列 + 縦スクロールに留めている（仮想化はしない）
- エージェント基準の絞り込みは `procscan` が `ps` に依存するため **macOS / Linux 限定**
  （Windows は常に 0 件。「起動中」基準は OS を問わず動く）
- リモートペイン（`remoteterm`）はローカル PID を持たずエージェント検出の対象外だが、
  「起動中」としては集約ビューに並ぶ
- ビューを切り替えるたびに端末は WebSocket を張り直す（セッション resume で
  スクロールバックは復元される）
