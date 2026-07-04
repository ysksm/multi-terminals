# リポジトリ clone の任意化（既存フォルダ利用）設計

日付: 2026-07-04
ブランチ: fix/wails-color-and-sidebar-collapse

## 背景 / 目的

ペイン追加フォームの「リポジトリ URL から clone」機能は、現状 URL を入力すると必ず
`git clone` を実行する。clone 先ディレクトリが既に存在すると git がエラーになるため、
**既に clone 済みのフォルダを使いたい**ケースで機能しない。

リポジトリ URL の設定は残しつつ、clone を必須にしない。

## 決定した仕様

URL 指定時、clone 先ディレクトリ（`~` 展開後）の状態で分岐する:

| clone 先の状態 | 動作 |
|---|---|
| 存在しない / 空ディレクトリ | 従来どおり `git clone` |
| 既存の git リポジトリ | clone をスキップし、そのパスをそのまま使う |
| 既存の非 git フォルダ（中身あり） | エラー（`git clone` 自身の「already exists and is not an empty directory」を UI に表示） |

- リモート URL の一致検証は行わない（SSH/HTTPS 表記差での誤検知を避ける。
  既存リポジトリはユーザーの指定を信頼する）。
- 判定は `rev-parse --is-inside-work-tree` によるため、リポジトリ配下の
  サブディレクトリを指定した場合も「既存リポジトリ」としてそのまま使う
  （モノレポの一部をペインにする用途を許容）。
- URL 未入力時の動作（ディレクトリ直接指定）は変更なし。

## 実装方針（A 案: gitcli.Clone を冪等にする）

判定ロジックはファイルシステム知識を持つ infrastructure 層に置き、
application 層・API・フロントエンドのロジックは変更しない。

### core/infrastructure/gitcli

`Clone(url, dest)` を冪等化:

1. dest を `expandTilde` で展開（既存処理）
2. `git -C <expanded> rev-parse --is-inside-work-tree` が成功したら、
   clone せず expanded パスを返す
3. それ以外は従来どおり `git clone -- url expanded` を実行

### core/application/port

`GitService.Clone` のドキュメントコメントを更新:
「dest が既に git リポジトリの場合は clone せず、そのパスを返す」。

### ハンドラ / API / フロントエンド

- `CloneRepositoryHandler`、`POST /api/repos/clone`、`api.cloneRepo` はロジック変更なし。
- UI 文言のみ調整（App.svelte）:
  - フィールドラベル: 「リポジトリ URL から clone（任意）」→
    「リポジトリ URL（任意・未 clone なら自動 clone）」
  - 送信ボタン: 「Clone して追加」→「追加」に統一

## テスト

`core/infrastructure/gitcli/gitcli_test.go` に追加:

1. **既存リポジトリ**: initRepo で作ったリポジトリを dest に指定 → clone されず
   展開済みパスが返る（エラーなし、既存コミットが保持される）
2. **既存の非 git フォルダ**: ファイル入りの一時ディレクトリを dest に指定 →
   エラーが返る

既存の handler テスト（fake GitService 使用）は変更不要。

## エラーハンドリング

非 git フォルダ指定時は gitcli のエラー → `apperr.Validation` → `mapErr` →
フロントの `guard` という既存経路でそのまま UI に表示される。新規経路なし。
