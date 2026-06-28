# ペイン（コンソール）タイトル機能 設計書

- 日付: 2026-06-28
- 対象ブランチ: feat/build-and-serve（または派生ブランチ）
- ステータス: 承認済み（実装計画へ）

## 目的

複数のコンソール（ペイン）を扱う際にどれがどれか分からなくなる問題を解消するため、**各ペインに任意のタイトルを付与・表示**できるようにする。タイトルはワークスペースに保存され、翌日のレイアウト再現時にも復元される。

## 決定事項（ブレインストーミング結果）

1. **既定表示**: タイトル未設定なら現状どおり作業ディレクトリを表示。設定されていればタイトルを表示（ディレクトリは tooltip に残す）。
2. **編集場所**: ペイン追加フォームに任意のタイトル入力欄を追加し、作成後はヘッダーのタイトルをクリックしてインライン編集。
3. **永続化**: タイトルはワークスペースに保存し、再現時に復元する。

## 前提（既存コードの確認結果）

- `Pane`（`core/domain/pane.go`）は `id, directory, slot, commands` を持ち、状態変更は集約境界のため非公開メソッド（`setDirectory`/`setCommands`）。公開ミューテータを追加しない方針。
- 既存の `SetPaneDirectory` がエンドツーエンドの参照実装:
  - 値オブジェクト `DirectoryPath`（`value_objects.go:49`）
  - `Workspace.SetPaneDirectory(id, dir)`（`workspace.go:114`）
  - `command.SetPaneDirectoryHandler`（`set_pane_directory.go`）
  - Web `PUT /api/workspaces/{id}/panes/{paneId}/directory`
  - フロント `api.setPaneDirectory`、`App.svelte` のヘッダー表示（`:238` が `cell.pane.directory` を表示）
- ペイン追加フォームは `App.svelte:265-` にあり、作業ディレクトリ等を入力する。
- 永続化は `core/infrastructure/jsonstore`。ドメイン↔レコードのマッパと `ReconstituteWorkspace` でペインを復元する。

## アーキテクチャ

`SetPaneDirectory` と同型の追加を、ドメイン→アプリ→インフラ→Web→フロントの各層に施す。タイトル専用の値オブジェクトを導入し、空文字を「未設定」として許容する。

### 1. ドメイン層

**新しい値オブジェクト `PaneTitle`**（`core/domain/value_objects.go`）:
- 内部 `value string`。
- `NewPaneTitle(value string) (PaneTitle, error)`:
  - 前後空白をトリム。
  - **空文字は許容**（空＝未設定）。
  - 最大長 100 文字（ルーン単位）を超えたらエラー。
  - 改行・制御文字（`\n`, `\r`, `\t` および Unicode 制御文字）を含む場合はエラー。
- `String() string` で値を返す。`IsZero()`（空判定）。

**`Pane`**（`core/domain/pane.go`）:
- フィールド `title PaneTitle` を追加。
- `NewPane(id PaneId, directory DirectoryPath, slot SlotIndex, title PaneTitle, commands []StartupCommand) (*Pane, error)` … 引数に `title` を追加（既存呼び出し箇所を更新）。
- `Title() PaneTitle` ゲッター、非公開 `setTitle(t PaneTitle)`。

**`Workspace`**（`core/domain/workspace.go`）:
- `SetPaneTitle(id PaneId, title PaneTitle) error`（`SetPaneDirectory` と同型）: 対象 pane を探し、無ければエラー、あれば `p.setTitle(title)`。

**再構成**（`ReconstituteWorkspace` / pane 復元ヘルパ）:
- 復元時に `title` を受け取り `NewPane(...)` に渡す。

### 2. アプリケーション層

**`SetPaneTitleCommand` / `SetPaneTitleHandler`**（`core/application/command/set_pane_title.go`、`set_pane_directory.go` のコピー構造）:
- 入力 DTO `SetPaneTitleCommand{WorkspaceID, PaneID, Title string}`。
- `Handle`: workspace id 検証 → `FindByID` → pane id 検証 → `NewPaneTitle` 検証 → `w.SetPaneTitle(...)` → `repo.Save`。検証失敗は `apperr.Validation`。

**`AddPane`**（既存 `AddPaneHandler`）:
- `AddPaneCommand` に任意 `Title string` を追加。
- ハンドラ内で `NewPaneTitle(cmd.Title)` を検証して `NewPane(...)` に渡す。

**クエリ DTO**（ペイン DTO、`core/application/query` のワークスペース取得結果）:
- ペイン DTO に `Title string` を追加し、ドメイン→DTO 変換で `pane.Title().String()` を設定。

**配線**（`apps/web/app.go` の `BuildDeps`）:
- `SetTitle: command.NewSetPaneTitleHandler(repo)` を `Deps` に追加。

### 3. インフラ（jsonstore）

**永続化レコード**（ペインの DTO/record）:
- `Title string `json:"title,omitempty"`` を追加。
- マッパ ドメイン→レコード: `pane.Title().String()`。レコード→ドメイン: `NewPaneTitle(record.Title)` を `NewPane(...)` へ。
- **後方互換**: 既存ファイルに `title` が無い場合、JSON デコードで空文字となり `NewPaneTitle("")` は成功（未設定）。追加フィールドのみのためスキーマ `version` は据え置き。

### 4. Web API

**新エンドポイント**（`apps/web/server.go` ルート + ハンドラ、ディレクトリ版と同型）:
- `PUT /api/workspaces/{id}/panes/{paneId}/title`、body `{"title": "..."}`。
- ハンドラは `SetPaneTitleCommand` を構築して `d.SetTitle.Handle(...)` を呼ぶ。検証エラーは 400、その他は既存方針に従う。

**ペイン追加エンドポイント**（既存 `POST /api/workspaces/{id}/panes`）:
- リクエスト body に任意 `title` を受理し `AddPaneCommand.Title` へ。

**レスポンス DTO**:
- ペインのレスポンス JSON に `title` を含める（クエリ DTO 経由で自動的に反映）。

### 5. フロントエンド

**`frontend/src/lib/api.js`**:
- `setPaneTitle: (id, paneId, title) => req('PUT', `/api/workspaces/${id}/panes/${paneId}/title`, { title })` を追加。
- `addPane(...)` の body に `title` を追加（任意）。

**`frontend/src/App.svelte`**:
- ペインヘッダー（`:238` 付近）の表示を `cell.pane.title || cell.pane.directory` に変更。`title`（HTML tooltip）属性には作業ディレクトリを設定し、ディレクトリも確認できるようにする。
- **インライン編集**: ヘッダーのラベルをクリックすると入力欄（`<input>`）に切り替わり、Enter/blur で `api.setPaneTitle` を呼んでワークスペースを再取得。Esc でキャンセル。
- **追加フォーム**（`:265-` 付近）に任意の「タイトル」入力欄を追加し、`addPane` に渡す。

## データフロー

```
[作成時]
追加フォーム(title任意) -> POST /panes {directory, slot, commands, title}
  -> AddPaneCommand.Title -> NewPaneTitle -> NewPane -> Workspace.AddPane -> Save

[編集時]
ヘッダーtitleクリック -> input -> PUT /panes/{paneId}/title {title}
  -> SetPaneTitleCommand -> NewPaneTitle -> Workspace.SetPaneTitle -> Save
  -> フロントが getWorkspace で再取得し再描画

[表示時]
getWorkspace -> pane DTO {..., title} -> ヘッダーに (title || directory)
```

## エラーハンドリング

- `NewPaneTitle`: 最大長超過・制御文字は検証エラー。空は正常（未設定）。
- `SetPaneTitleHandler` / `AddPaneHandler`: id・title の検証失敗は `apperr.Validation`（Web 層で 400）。
- 存在しない pane への `SetPaneTitle` はエラー（404/400 は既存方針に合わせる）。
- フロント: 保存失敗時は既存の `error` 表示機構に従い、編集をキャンセルして元の表示へ戻す。

## テスト戦略

- **ドメイン**: `PaneTitle` の VO テスト（空許可、トリム、最大長境界、制御文字拒否）。`Workspace.SetPaneTitle`（成功・pane 不在）。`NewPane`/再構成が title を保持。
- **アプリ**: `SetPaneTitleHandler`（成功で Save 呼出し、検証エラー）。`AddPane` が title を保存。クエリ DTO に title が載る。
- **インフラ**: title 込みのラウンドトリップ。`title` 欠落の既存ファイルを読み込んで空タイトル（後方互換）。
- **Web**: `PUT .../title` ハンドラ（200/400）。ペイン追加で title が保存され GET に現れる。
- **フロント**: 手動スモーク（ヘッダー表示の title||directory、インライン編集、追加フォーム）。既存 `go test ./...` 緑。

## スコープ外（YAGNI）

- タイトルの自動生成（実行コマンド名やプロセスからの推定）。
- 色分け・アイコン・タブ UI の刷新。
- タイトルの一意性制約や検索。

## 成果物一覧

- `core/domain/value_objects.go`（PaneTitle 追加）、`core/domain/pane.go`、`core/domain/workspace.go`、再構成ヘルパ
- `core/application/command/set_pane_title.go`（+テスト）、`add_pane.go`（title 対応）、クエリ DTO
- `core/infrastructure/jsonstore`（レコード + マッパ + テスト）
- `apps/web/server.go`（ルート/ハンドラ）、`apps/web/app.go`（Deps 配線）、関連テスト
- `frontend/src/lib/api.js`、`frontend/src/App.svelte`
- 既存 `NewPane` 呼び出し箇所すべての更新
