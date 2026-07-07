# ペイン git 操作(ブランチ切替・pull・push・fetch)設計

日付: 2026-07-07
ステータス: 承認済み

## 目的

ペインヘッダの git バッジから、簡易的な git 操作(ブランチ切替・pull・push・fetch)を
実行できるようにする。ショートカットキーにも対応する。

## 決定事項(ブレインストーミング結果)

- UI: git バッジ(`⎇ branch`)クリックでドロップダウンメニュー。
- ブランチ切替対象: ローカル + リモート追跡ブランチ(リモートのみは追跡ブランチを自動作成して切替 = `git switch` の既定挙動)。新規ブランチ作成は対象外(YAGNI)。
- dirty 時の切替: git にそのまま任せ、失敗したら git のエラーメッセージをメニューに表示。自動 stash・事前確認ダイアログはしない。
- 実行方式: REST 同期実行(案A)。非同期ジョブや PTY へのコマンド送信は不採用。
- ショートカット: メニュー開閉キーのみグローバルに割当(`Ctrl+Shift+B`)。各操作への直接キーは割り当てない。メニュー内キー操作あり。

## バックエンド

### ポート拡張 — `core/application/port/git_service.go`

```go
// BranchInfo は 1 ブランチの情報。
type BranchInfo struct {
    Name      string // ローカル名。リモートのみの場合も origin/ プレフィックスを除いた名前
    IsCurrent bool
    IsRemote  bool   // リモートにのみ存在(ローカル未チェックアウト)
}

type GitService interface {
    // 既存: Info / RemoteURL / Clone
    Branches(dir string) ([]BranchInfo, error)
    Checkout(dir, branch string) error // git switch <branch>
    Pull(dir string) error
    Push(dir string) error
    Fetch(dir string) error // git fetch --prune
}
```

### gitcli 実装 — `core/infrastructure/gitcli/gitcli.go`

- ネットワーク系(Pull/Push/Fetch)は `exec.CommandContext` + 60 秒タイムアウト。
  環境変数 `GIT_TERMINAL_PROMPT=0` を付与し、認証プロンプトでハングせず即エラーにする。
- Checkout/Branches はローカル操作なので既存 `git()` ヘルパを流用(タイムアウトなし)。
- `Branches` は `git branch --format='%(refname:short)'` と
  `git branch -r --format='%(refname:short)'` を実行して Go 側でマージ。
  `origin/HEAD` は除外。ローカルに同名があるリモートブランチは重複除去(ローカル優先)。
- Push は upstream 未設定なら git のエラーをそのまま返す(`--set-upstream` 案内が
  メッセージに含まれるためユーザーに伝わる)。

### application 層(CQRS)

- query: `ListPaneBranches` — paneId → Pane.Directory 解決 → `GitService.Branches`。
- command: `CheckoutPaneBranch{WorkspaceID, PaneID, Branch}`。
- command: `RunPaneGitOp{WorkspaceID, PaneID, Op}` — Op は `pull|push|fetch` の 3 値。
  3 コマンドに分けると重複が多いため 1 つに集約。
- いずれも既存 `GetPaneGitInfo` / `OpenPaneIn` と同様、Directory が空・非リポジトリの
  場合は `apperr` で 4xx 相当を返す。

### REST API — `apps/web/server.go`

```
GET  /api/workspaces/{id}/panes/{paneId}/git/branches → {branches:[{name,isCurrent,isRemote}]}
POST /api/workspaces/{id}/panes/{paneId}/git/checkout {branch} → 200 / エラー JSON
POST /api/workspaces/{id}/panes/{paneId}/git/{op}     op=pull|push|fetch → 200 / エラー JSON
```

エラー時は git の stderr をメッセージに含めた JSON を返し、UI はそのまま表示する。

## フロントエンド

### git メニュー — 新コンポーネント `frontend/src/lib/GitMenu.svelte`

- git バッジクリックでバッジ直下にドロップダウン表示。
- 構成(上から): ①操作行 `⬇ Pull` / `⬆ Push` / `⟳ Fetch`、②区切り線、
  ③ブランチ一覧(スクロール可。現在ブランチに ✓。リモートのみのブランチは
  `origin/` 表記を薄字で付記)。ブランチクリックで checkout。
- 開いた時に branches API を取得。操作実行中は該当ボタン/行にスピナーを出し
  他操作を無効化。成功後はブランチ一覧と git バッジ(`paneGit`)を再取得して
  メニューは開いたまま反映(fetch 後は新リモートブランチが現れるため一覧再取得必須)。
- エラーは git stderr をメニュー下部に赤字表示(閉じるまで保持)。
- 外側クリック / Esc で閉じる。

### ショートカットキー — `frontend/src/lib/shortcuts.js`

- `Ctrl+Shift+B`: アクティブペインの git メニューを開閉(非リポジトリペインでは無反応)。
  `PANE_ACTIONS` に `b: 'gitmenu'` を追加、`SHORTCUT_GROUPS` にも記載。
- メニュー内: `↑↓` ブランチ選択、`Enter` checkout、`p`/`u`/`f` で pull/push/fetch、
  `Esc` で閉じる。

## テスト

- gitcli: 実 git でテンポラリの bare リモート + clone を作り
  Branches/Checkout/Pull/Push/Fetch を実機検証(既存 gitcli_test.go 流儀)。
  認証プロンプトが必要な URL への fetch が `GIT_TERMINAL_PROMPT=0` で即エラーに
  なることも確認。
- application 層: `apptest/git_fakes.go` を拡張しフェイクで command/query 単体テスト。
- web: httptest で正常系 / 異常系(非リポジトリ、pane 不在)。
- frontend: メニューのキーボード操作ロジックを pure 関数に切り出し
  `*.node.test.mjs`(既存流儀)。最後にブラウザで実機確認。

## スコープ外

- 新規ブランチ作成、stash、commit、merge/rebase、コンフリクト解決 UI
- 操作の進捗ストリーミング(完了/失敗のみ)
- 認証情報の入力 UI(credential helper / ssh-agent 前提。無ければエラー表示)
