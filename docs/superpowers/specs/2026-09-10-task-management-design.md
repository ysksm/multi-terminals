# タスク管理（Jira 連携 + 環境セットアップ）設計

日付: 2026-09-10
状態: 実装済み（2026-09-10）
モック: `docs/mockups/2026-09-10-task-management-ui.html`

## 要件（ユーザー確定事項）

- モックに載っているタスク機能が欲しい
- Jira は **Cloud と Server / Data Center の両方**に対応（設定画面で種類を選ぶ）
- リポジトリ取得は **git clone と「ベースクローンからコピー」の両方**を第 1 段階で入れる
- Jira への書き戻しは **読み取り専用**（ステータス変更は「Jira で開く」に任せる）
- 第 1 段階の範囲は **タスク一覧 / Jira 取り込み / タスク詳細 / ワークスペースのタスク帯 /
  Jira 接続設定 + 環境テンプレート + セットアップ実行**（ベースクローン管理も含む）
- サイドバーのタスク欄は進行中（作業中 / 環境あり）だけ最大 5 件。全件はメインの一覧
- 設定は右上の ⚙ メニューに集約（実装済みのアプリバーに「タスク環境」グループを足す）
- 実装は **1 つの PR にまとめる**（分割しない）
- ブランチ名の `{slug}` は概要から **ASCII 英数字だけを抽出**（小文字ハイフン区切り、最大 30 文字）。
  空なら省略して `feature/PROJ-1301` のように末尾の `-` を落とす。タスク詳細で編集可
- ローカル状態「完了」は **手動のみ**（Jira の状態からは自動で変えない）
- **Jira に依存しないタスク（ローカルタスク）も同じ一覧・同じ環境セットアップで扱う**
  （モック: `docs/mockups/2026-09-10-local-tasks-ui.html`）。取込元 `source = jira | local` で区別し、
  番号は設定の接頭辞 + 連番で自動採番（`T-001`）。概要・説明・優先度・ラベル・リンク・ステータスは
  アプリ内で編集し、ステータスは設定で名前と分類を定義する。詳細の「Jira に紐付ける」で
  あとから Jira タスクに切り替えられる（環境はそのまま）。逆方向はしない
- Jira 同期は **手動のみ**（「⟳ Jira を同期」ボタン。起動時・定期の自動同期はしない）

## 前提（確定）

| 項目 | 提案 |
| --- | --- |
| Task と Workspace の関係 | Task は別集約。Workspace は `taskId` を任意で持つだけ（既存機能・保存形式は不変。`taskId` は追加フィールドで schema version 据え置き） |
| API トークンの保存先 | `MULTI_TERMINALS_DIR/jira_token`（権限 0600）。OS キーチェーン対応は後続 |
| 作業フォルダの削除 | ユーザーが一覧の「作業フォルダを削除」を押したときだけ。自動削除しない。ワークスペース削除でもフォルダは残す |
| ベースクローンの置き場 | 既定 `MULTI_TERMINALS_DIR/base/<name>`。パスは登録時に変更可 |
| 自動取り込み JQL | 設定していれば手動同期のたびに該当 issue を「未準備」で追加（未登録のものだけ） |
| 説明文の表示 | プレーンテキスト（Cloud の ADF はテキストノードを抽出、Server は wiki 記法のまま）。HTML は描画しない |
| セットアップコマンドの実行 | `sh -c`（Windows は `cmd /C`）。環境変数 `TASK_KEY` / `TASK_DIR` / `TASK_BRANCH` を渡す |

## 画面（モックとの対応）

1. **タスク一覧**（ホーム。`#tasks`）
   - 列: Jira 番号 / 概要（担当・優先度・スプリント）/ Jira ステータス / ローカル状態 /
     リポジトリ / ワークスペース / 更新 / 操作
   - 状態チップでフィルタ（すべて / 作業中 / 環境あり / 未準備 / 完了）、並び替え、
     20 件/頁のページング（サーバ側ではなくフロントで行う。数百件でも一覧は 1 リクエスト）
   - 操作: 作業中・環境あり → 「開く」（ワークスペースへ）/ 未準備 → 「環境を作る」/
     完了 → 「作業フォルダを削除」。全行に「詳細」
2. **Jira から取り込む**（モーダル）
   - 番号入力（カンマ区切り複数、URL 貼り付け可。`[A-Z][A-Z0-9_]+-\d+` を抽出）→ 取得 →
     プレビュー → テンプレート選択（「なし」可）→ 「取り込む（環境は後で）」または
     「取り込んでセットアップ」
   - 「作成されるもの」欄はテンプレートと番号からフロントで計算して表示
3. **タスク詳細**（`#tasks/<KEY>`）
   - 左: Jira 情報（読み取り専用）+ ローカルメモ（自動保存）
   - 右: 作業環境の編集（テンプレート / 作業フォルダ / ブランチ / リポジトリ / ペイン割当）。
     未準備なら「環境をセットアップ」、環境ありなら「ワークスペースを開く」
4. **環境セットアップ**（`#tasks/<KEY>/setup`）
   - ステップ一覧 + 進捗バー + 各ステップのログ。失敗ステップで停止し
     「このステップから再実行」「スキップして続行」。「バックグラウンドで続行」で
     一覧へ戻れる（一覧のローカル状態は「準備中 ⟳」）
5. **ワークスペース**（既存画面）
   - `taskId` があるときだけ上部にタスク帯（番号 / 概要 / Jira ステータス / ブランチ /
     タスク詳細 / Jira で開く）。ステータス変更ボタンは読み取り専用のため置かない
6. **設定**（`#settings/<panel>`、⚙ メニューの「タスク環境」から）
   - 環境テンプレート / ベースクローン / Jira 接続 の 3 パネル。既存のリモート設定・
     ショートカット一覧はモーダルのまま

サイドバーは実装済みの縦積み（タスク → ワークスペース → アクティブ）の先頭に
「タスク · 進行中」欄を足す。

## ドメインモデル（`core/domain`）

```
Task
  ID            TaskId（内部 ID）
  JiraKey       "PROJ-1301"（一意。取り込み時の重複はエラー。ローカルタスクは採番した "T-007"）
  Source        jira | local（空は jira）
  Local         {Labels[], Links[], StatusID}   ※ local のみ。概要・説明・優先度は Jira と同じ欄に入れる
  Jira          JiraSnapshot{Summary, Status, StatusCategory, Assignee, Priority,
                Epic, Sprint, Description, URL, FetchedAt}
  Note          string（ローカルメモ）
  LocalState    none | preparing | ready | done   ※ working は保存しない（後述）
  Env           TaskEnv{TemplateID?, WorkDir, Branch, Repos []TaskRepo}
  WorkspaceID   *WorkspaceId
  CreatedAt / UpdatedAt

TaskRepo        {Name, Path, Source RepoSource}
RepoSource      Kind: clone | baseCopy, URL, BaseCloneID

EnvTemplate
  ID, Name, IsDefault
  WorkDirPattern  "~/work/{KEY}"
  BranchPattern   "feature/{KEY}-{slug}"
  Layout          既存 LayoutPreset
  Repos           []TemplateRepo{Name, Source RepoSource, SetupCommands []string}
  Panes           []TemplatePane{Slot, RepoName（"" = 作業フォルダ直下）, Commands []StartupCommand}

BaseClone
  ID, Name, URL, Path, DefaultBranch, LastFetchedAt

TaskSettings（単一レコード）
  AutoFetchBeforeSetup  bool（既定 true）
  IncludeNodeModules    bool（既定 true。コピー時に node_modules を含める）
  LocalTask             {Prefix "T", Digits 3, NextSeq, Statuses[{ID, Name, Category new|indeterminate|done}]}

SetupRun
  ID, TaskID, State pending|running|failed|done|aborted
  Steps []SetupStep{Index, Kind, Label, Command, Dir, State pending|running|done|failed|skipped,
                    StartedAt, Duration, Log}
  StartedAt, FinishedAt

JiraConfig（設定。secret は別ファイル）
  Kind cloud | server, BaseURL, Email（cloud のみ）, JQL
```

- **`working` は導出**: `LocalState == ready` かつ紐付くワークスペースのペインに
  ライブセッションがある（`session.Registry`）。一覧クエリで付ける
- `Workspace` に `TaskID *TaskId` を追加（JSON は `taskId,omitempty`）
- `Workspace` 削除時は Task 側の `WorkspaceID` を外し、`LocalState` は `ready` のまま
  （フォルダはある）。一覧では「ワークスペースなし」と出し「ワークスペースを作り直す」を出す

## 永続化（`core/infrastructure/jsonstore`）

`MULTI_TERMINALS_DIR` 配下に既存と同じ「1 レコード 1 JSON ファイル」方式。

```
tasks/<taskId>.json
templates/<templateId>.json
base_clones/<id>.json
setup_runs/<runId>.json        … 直近の実行だけ残す（Task ごとに 1 件。再実行で上書き）
task_settings.json
jira.json                      … kind / baseUrl / email / jql
jira_token                     … 0600。API トークンまたは PAT
base/<name>/                   … ベースクローン本体（既定パス）
```

## Jira クライアント（`core/application/port.JiraClient` / `core/infrastructure/jirahttp`）

```go
type JiraClient interface {
    TestConnection(ctx) (JiraUser, error)           // /myself
    FetchIssues(ctx, keys []string) ([]JiraIssue, error)
    Search(ctx, jql string) ([]JiraIssue, error)     // 自動取り込み用。最大 200 件
}
```

| | Cloud | Server / DC |
| --- | --- | --- |
| 認証 | `Authorization: Basic base64(email:token)` | `Authorization: Bearer <PAT>` |
| API | `/rest/api/3/search/jql`, `/rest/api/3/issue/{key}` | `/rest/api/2/search`, `/rest/api/2/issue/{key}` |
| 説明文 | ADF → テキストノード抽出 | wiki 記法の文字列 |
| スプリント / エピック | `customfield_*` はサイトごとに ID が違うため、`/rest/api/{v}/field` を 1 度取得して名前（`Sprint` / `Epic Link` / `Parent`）から解決し、`jira.json` にキャッシュ | 同左 |

- `net/http` のみで実装（依存追加なし）。タイムアウト 15 秒
- 失敗は `apperr` で種類分け（認証失敗 / 見つからない / 通信失敗）し、UI にそのまま出す
- テストは `httptest.Server` でスタブ

## セットアップ実行（`core/application/setup`）

Task + EnvTemplate から**ステップ列を決定的に生成**し（pure。テスト対象）、
ランナーが順に実行する。

```
1. mkdir            作業フォルダ作成
2. repo:acquire     各リポジトリ: baseCopy → Go でツリーコピー（IncludeNodeModules=false なら除外）
                                   clone    → 既存 GitService.Clone
3. repo:branch      各リポジトリ: fetch --prune → ブランチが既にあれば switch、無ければ
                                   origin/<default> から switch -c
4. repo:setup       各リポジトリ: SetupCommands を順に sh -c で実行（stdout/stderr をログへ）
5. workspace        既存 CreateWorkspace / AddPane / SetPaneStartupCommands ハンドラで作成し
                    Task.WorkspaceID と Workspace.TaskID を相互に設定
```

- 実行は goroutine。進捗は `setup_runs/<id>.json` に都度保存しつつ、
  `GET /api/setup-runs/{id}/stream`（SSE。既存 agent-status と同じ方式）で配信
- 失敗したステップで停止（`State=failed`）。`POST …/retry`（そのステップから）、
  `POST …/skip`（スキップして次へ）、`POST …/abort`
- 実行中の Task は `LocalState=preparing`。完了で `ready`
- 「ベースの鮮度」= BaseClone.LastFetchedAt。`AutoFetchBeforeSetup` なら手順 2 の前に
  ベースを `fetch` + `pull --ff-only`（既定ブランチ上）
- 同時実行は Task ごとに 1 本。別 Task は並列可

## API（`apps/web`）

```
GET    /api/tasks                          一覧（working を導出して返す。local はステータス名/分類を設定から埋める）
POST   /api/tasks                          手動作成 {summary, description, priority, labels, links, statusId, key?, templateId?, setup}
GET    /api/tasks/next-key                 次に採番される番号
POST   /api/tasks/{id}/link-jira           {key} → Jira から取得して source=jira に切替
POST   /api/tasks/import                   {keys[], templateId?, setup bool} → 取り込み（+ 実行開始）
POST   /api/tasks/preview                  {keys[]} → Jira から取得して返すだけ（保存しない）
GET    /api/tasks/{id}
PATCH  /api/tasks/{id}                     note / env（workDir, branch, repos, templateId）/ localState（done ↔ ready の手動切替）/ local（ローカルタスクの編集項目）
DELETE /api/tasks/{id}                     タスクを削除（フォルダは残す）
POST   /api/tasks/{id}/sync                Jira を再取得
POST   /api/tasks/{id}/setup               セットアップ開始 → {runId}
POST   /api/tasks/{id}/workdir/delete      作業フォルダ削除（確認は UI）
POST   /api/tasks/sync                     全件を再取得 + JQL 自動取り込み（手動ボタンからのみ）

GET    /api/setup-runs/{id}
GET    /api/setup-runs/{id}/stream         SSE
POST   /api/setup-runs/{id}/retry | skip | abort

GET/POST/PUT/DELETE /api/templates[/{id}]
GET/POST/DELETE     /api/base-clones[/{id}]
POST   /api/base-clones/{id}/update        fetch + pull --ff-only（/api/base-clones/update で全件）
GET/PUT /api/task-settings
GET/PUT /api/jira/config                   token は PUT のみ受け付け、GET では「設定済み」フラグだけ返す
POST   /api/jira/test
```

`GET /api/workspaces` のレスポンスに `taskId` を追加（タスク帯・サイドバー用）。

## フロント

- 画面切替は `App.svelte` の `screen` 状態（`workspace | tasks | task | setup | settings`）+
  URL ハッシュ。既存の `viewMode`（集約表示）はワークスペース画面の中のモードとして残す
- App.svelte が既に大きいため、新画面は `frontend/src/lib/tasks/` にコンポーネント分割
  （`TaskList.svelte` / `TaskImport.svelte` / `TaskDetail.svelte` / `SetupRun.svelte` /
  `TaskSettings.svelte`）。App.svelte は配線だけ
- 純関数は `frontend/src/lib/tasks.js` に置き node の assert でテスト:
  `parseJiraKeys(text)` / `expandPattern(pattern, {KEY, slug})` / `slugify(summary)` /
  `localStateOf(task, livePaneIds)` / `filterAndSortTasks()` / `paginate()` /
  `planFromTemplate(template, key, summary)`（「作成されるもの」表示用。サーバ側の
  ステップ生成と同じ結果になることをテストで固定）
- ⚙ メニューに「タスク環境」グループ（環境テンプレート / ベースクローン / Jira 接続）を追加
- サイドバーのタスク欄: `working` → `ready` の順、最大 5 件、「すべてのタスク N →」

## スコープ外・既知の制約

- Jira への書き込み（遷移・コメント）は行わない
- 手動タスク（Jira なし）、複数 Jira サイト、OAuth は対象外
- `git worktree` 方式は対象外（将来 `RepoSource.Kind` を増やして対応できる構造にする）
- セットアップコマンドは対話できない（入力待ちになるコマンドはタイムアウトで失敗扱い。
  既定 10 分）
- ベースクローンのサイズ表示は更新時に計算してキャッシュ（毎回 `du` しない）
- Windows: コピー・コマンド実行は動くが、エージェント検出（既存制約）は対象外のまま
