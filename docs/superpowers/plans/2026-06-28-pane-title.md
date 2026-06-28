# ペイン（コンソール）タイトル機能 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 各ペインに任意のタイトルを付与・表示・編集でき、ワークスペースに保存されて再現されるようにする。

**Architecture:** 既存 `SetPaneDirectory` のエンドツーエンドパターンを踏襲し、ドメイン→アプリ→インフラ→Web→フロントの各層に `PaneTitle`（空許容の値オブジェクト）を追加する。表示はタイトル優先・未設定時はディレクトリにフォールバック。

**Tech Stack:** Go 1.26（DDD レイヤード）、Svelte 5 + Vite フロント、JSON ファイル永続化（jsonstore）。

## Global Constraints

- Go 1.26。新規 Go 依存は追加しない（標準ライブラリのみ）。
- ドメインの集約境界を維持: Pane の状態変更は非公開メソッド（`setTitle`）経由、変更は `Workspace.SetPaneTitle` から行う。公開ミューテータを Pane に追加しない。
- `PaneTitle` は**空文字を許容**（空＝未設定）。前後空白トリム、最大 100 文字（ルーン単位）、制御文字（`unicode.IsControl`）禁止。
- 永続化は**後方互換**: 既存ファイルに `title` が無くても読み込めること（空タイトル扱い）。追加フィールドのみのため schema `version` は据え置き。
- 表示は「タイトルがあればタイトル、なければディレクトリ」。ディレクトリは tooltip に残す。web 版・既存 API の他挙動は不変。

確定済みシグネチャ（参照実装と既存コード）:
- `domain.NewDirectoryPath(string) (DirectoryPath, error)`、`(DirectoryPath).String() string`。
- 現状 `domain.NewPane(id PaneId, directory DirectoryPath, slot SlotIndex, commands []StartupCommand) (*Pane, error)` を **`title PaneTitle` 追加**へ変更。呼び出し箇所: `core/application/command/add_pane.go:79`、`core/infrastructure/jsonstore/mapper.go:110`、および各 `*_test.go`。
- `domain.Workspace.SetPaneDirectory(id PaneId, dir DirectoryPath) error`（`workspace.go:114`）が `SetPaneTitle` の雛形。
- `command.SetPaneDirectoryHandler`（`set_pane_directory.go`）が `SetPaneTitleHandler` の雛形。`apperr.Validation(...)` を使用。
- `command.AddPaneCommand{WorkspaceID, Directory, Slot, Commands []StartupCommandInput}`、`AddPaneHandler.Handle` 内で `NewPane(paneID, dir, slot, startupCmds)`。
- `jsonstore` `paneRecord{ID, Directory, Slot, Commands}`（`schema.go:14`）、マッパ書き出し `mapper.go:24`、読み込み `mapper.go:85-114`。
- `query.PaneDTO{ID, Directory, Slot, Commands}`（`dto.go:16`）、`toWorkspaceDTO`（`dto.go:35-`）。
- `web.Deps` フィールド `SetDir *command.SetPaneDirectoryHandler`（`server.go:30`）、`BuildDeps`（`apps/web/app.go`）で各ハンドラを配線。
- Web ルート `PUT /api/workspaces/{id}/panes/{paneId}/directory` → `handleSetPaneDirectory`（`server.go:293`）、`POST /api/workspaces/{id}/panes` → `handleAddPane`（`server.go:249`）。
- フロント `api.js`: `addPane(id, directory, slot, commands)`、`setPaneDirectory(id, paneId, directory)`。`App.svelte` ヘッダー `:238`、追加フォーム `:265-`、`submitAddPane`（`:97`）。

## File Structure

- `core/domain/value_objects.go` — `PaneTitle` 値オブジェクト追加。
- `core/domain/pane.go` — `title` フィールド + `Title()`/`setTitle()`、`NewPane` 署名変更。
- `core/domain/workspace.go` — `SetPaneTitle` 追加。
- `core/application/command/set_pane_title.go`（新規）+ `set_pane_title_test.go`（新規）。
- `core/application/command/add_pane.go` — `AddPaneCommand.Title` 対応。
- `core/application/query/dto.go` — `PaneDTO.Title` + 変換。
- `core/infrastructure/jsonstore/schema.go` / `mapper.go` — `title` フィールド + マッピング。
- `apps/web/server.go` — ルート + `handleSetPaneTitle`、`handleAddPane` の title 受理。
- `apps/web/app.go` — `Deps.SetTitle` 配線。
- `frontend/src/lib/api.js` / `frontend/src/App.svelte` — API + 表示/編集 UI。

---

### Task 1: ドメイン — PaneTitle 値オブジェクト + Pane.title + Workspace.SetPaneTitle

**Files:**
- Modify: `core/domain/value_objects.go`, `core/domain/pane.go`, `core/domain/workspace.go`
- Modify (caller fix to keep build green): `core/application/command/add_pane.go:79`, `core/infrastructure/jsonstore/mapper.go:110`, および全 `NewPane(` 呼び出し（テスト含む）
- Test: `core/domain/value_objects_test.go`（または既存テストファイル）, `core/domain/workspace_test.go`

**Interfaces:**
- Produces:
  - `domain.NewPaneTitle(value string) (PaneTitle, error)`、`(PaneTitle).String() string`、`(PaneTitle).IsZero() bool`、`const MaxPaneTitleLen = 100`
  - `domain.NewPane(id PaneId, directory DirectoryPath, slot SlotIndex, title PaneTitle, commands []StartupCommand) (*Pane, error)`（署名変更）
  - `(*Pane).Title() PaneTitle`
  - `(*Workspace).SetPaneTitle(id PaneId, title PaneTitle) error`

- [ ] **Step 1: PaneTitle の失敗するテストを書く**

`core/domain/value_objects_test.go` に追記（無ければ新規・`package domain`）:
```go
func TestNewPaneTitle(t *testing.T) {
	// 空は許容（未設定）
	if pt, err := NewPaneTitle(""); err != nil || !pt.IsZero() {
		t.Fatalf("empty title: got (%q, %v), want empty/no-error", pt.String(), err)
	}
	// 前後空白はトリム
	if pt, err := NewPaneTitle("  build  "); err != nil || pt.String() != "build" {
		t.Fatalf("trim: got (%q, %v), want \"build\"", pt.String(), err)
	}
	// 最大長ちょうどは可、超過は不可
	ok := strings.Repeat("a", MaxPaneTitleLen)
	if _, err := NewPaneTitle(ok); err != nil {
		t.Fatalf("max length boundary: unexpected error %v", err)
	}
	if _, err := NewPaneTitle(strings.Repeat("a", MaxPaneTitleLen+1)); err == nil {
		t.Fatal("over max length: expected error, got nil")
	}
	// 制御文字（改行など）は不可
	if _, err := NewPaneTitle("a\nb"); err == nil {
		t.Fatal("control char: expected error, got nil")
	}
}
```
（`value_objects_test.go` が無く新規作成する場合は `import ("strings"; "testing")` を付ける。既存ファイルに追記する場合は `strings` が import 済みか確認し、無ければ追加。）

- [ ] **Step 2: テストを実行して失敗を確認**

Run: `go test ./core/domain/ -run TestNewPaneTitle -v`
Expected: FAIL（`NewPaneTitle`/`MaxPaneTitleLen` 未定義のコンパイルエラー）。

- [ ] **Step 3: PaneTitle を実装**

`core/domain/value_objects.go` に追記（ファイル末尾付近）:
```go
// MaxPaneTitleLen は PaneTitle の最大長（ルーン単位）。
const MaxPaneTitleLen = 100

// PaneTitle はペインの表示名。空は「未設定」を表す。
type PaneTitle struct{ value string }

// NewPaneTitle は前後空白をトリムして PaneTitle を生成する。
// 空は許容（未設定）。最大長超過・制御文字を含む場合はエラー。
func NewPaneTitle(value string) (PaneTitle, error) {
	v := strings.TrimSpace(value)
	if utf8.RuneCountInString(v) > MaxPaneTitleLen {
		return PaneTitle{}, fmt.Errorf("pane title must be at most %d characters", MaxPaneTitleLen)
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return PaneTitle{}, errors.New("pane title must not contain control characters")
		}
	}
	return PaneTitle{value: v}, nil
}

func (t PaneTitle) String() string { return t.value }
func (t PaneTitle) IsZero() bool   { return t.value == "" }
```
`value_objects.go` の import に `"fmt"`, `"unicode"`, `"unicode/utf8"` を追加（`strings`, `errors` は既存）。

- [ ] **Step 4: PaneTitle テストを実行して成功を確認**

Run: `go test ./core/domain/ -run TestNewPaneTitle -v`
Expected: PASS

- [ ] **Step 5: Pane に title を追加し NewPane 署名を変更**

`core/domain/pane.go`:
```go
type Pane struct {
	id        PaneId
	directory DirectoryPath
	slot      SlotIndex
	title     PaneTitle
	commands  []StartupCommand
}

// NewPane は Pane を生成する。commands は防御的にコピーされる。
func NewPane(id PaneId, directory DirectoryPath, slot SlotIndex, title PaneTitle, commands []StartupCommand) (*Pane, error) {
	if id.IsZero() {
		return nil, errors.New("pane id must not be empty")
	}
	return &Pane{
		id:        id,
		directory: directory,
		slot:      slot,
		title:     title,
		commands:  append([]StartupCommand(nil), commands...),
	}, nil
}
```
さらにゲッター/セッターを追加（`Directory()` の近く）:
```go
func (p *Pane) Title() PaneTitle { return p.title }

func (p *Pane) setTitle(t PaneTitle) { p.title = t }
```

- [ ] **Step 6: Workspace.SetPaneTitle の失敗するテストを書く**

`core/domain/workspace_test.go` に追記:
```go
func TestWorkspaceSetPaneTitle(t *testing.T) {
	w := newTestWorkspaceWithOnePane(t) // 既存のヘルパがあれば利用。無ければ下の注記参照。
	panes := w.Panes()
	id := panes[0].ID()

	title, _ := NewPaneTitle("API server")
	if err := w.SetPaneTitle(id, title); err != nil {
		t.Fatalf("SetPaneTitle: %v", err)
	}
	if got := w.Panes()[0].Title().String(); got != "API server" {
		t.Fatalf("title not set: got %q", got)
	}

	// 存在しない pane はエラー
	missing, _ := NewPaneId("does-not-exist")
	if err := w.SetPaneTitle(missing, title); err == nil {
		t.Fatal("SetPaneTitle on missing pane: expected error, got nil")
	}
}
```
注記: ワークスペース+ペイン生成のヘルパが既存テストに無い場合は、同ファイル内の既存テストと同じ手順（`NewWorkspace`→`NewPane(... , PaneTitle{}, ...)`→`AddPane`）でインラインに用意すること。`NewPane` 呼び出しには新引数の `PaneTitle{}`（空）を渡す。

- [ ] **Step 7: テストを実行して失敗を確認**

Run: `go test ./core/domain/ -run TestWorkspaceSetPaneTitle -v`
Expected: FAIL（`SetPaneTitle` 未定義）。

- [ ] **Step 8: Workspace.SetPaneTitle を実装**

`core/domain/workspace.go` に `SetPaneDirectory` の直後へ追加:
```go
// SetPaneTitle は指定 pane の表示名を変更する。
func (w *Workspace) SetPaneTitle(id PaneId, title PaneTitle) error {
	p := w.findPane(id)
	if p == nil {
		return fmt.Errorf("pane %s not found", id)
	}
	p.setTitle(title)
	return nil
}
```
（`SetPaneDirectory` と同じ `findPane`/エラーパターン。`fmt` は既存 import。）

- [ ] **Step 9: 既存の NewPane 呼び出しをすべて更新（ビルドを通す）**

全呼び出し箇所に `PaneTitle` 引数を挿入する。本番コードは空タイトル `domain.PaneTitle{}`（後続タスクで実値に差し替え）:
- `core/application/command/add_pane.go:79`:
  ```go
  pane, err := domain.NewPane(paneID, dir, slot, domain.PaneTitle{}, startupCmds)
  ```
- `core/infrastructure/jsonstore/mapper.go:110`:
  ```go
  pane, err := domain.NewPane(paneID, dir, slot, domain.PaneTitle{}, cmds)
  ```
- テスト内の `NewPane(` 呼び出し（`grep -rn "NewPane(" --include="*_test.go" core` で列挙）はすべて第4引数に `PaneTitle{}` を挿入。

Run（列挙）: `grep -rn "NewPane(" core --include="*.go" | grep -v "func NewPane"`
すべて4引数になるよう修正。

- [ ] **Step 10: ドメイン層と全体のビルド・テストを確認**

Run:
```bash
go build ./...
go test ./core/...
```
Expected: ビルド成功、`core/...` テスト PASS（新規 2 テスト含む）。

- [ ] **Step 11: コミット**

```bash
git add core/domain core/application/command/add_pane.go core/infrastructure/jsonstore/mapper.go
git commit -m "feat(domain): add PaneTitle value object and Workspace.SetPaneTitle"
```

---

### Task 2: アプリケーション — SetPaneTitleHandler + AddPane.Title + PaneDTO.Title

**Files:**
- Create: `core/application/command/set_pane_title.go`, `core/application/command/set_pane_title_test.go`
- Modify: `core/application/command/add_pane.go`, `core/application/query/dto.go`
- Test: `core/application/query/dto_test.go`（既存があれば追記）

**Interfaces:**
- Consumes: `domain.NewPaneTitle`, `domain.Workspace.SetPaneTitle`, `domain.NewPane(..., title, ...)`（Task 1）
- Produces:
  - `command.SetPaneTitleCommand{WorkspaceID, PaneID, Title string}`
  - `command.NewSetPaneTitleHandler(repo domain.WorkspaceRepository) *command.SetPaneTitleHandler`
  - `(*SetPaneTitleHandler).Handle(ctx, SetPaneTitleCommand) error`
  - `command.AddPaneCommand.Title string`（追加フィールド）
  - `query.PaneDTO.Title string`

- [ ] **Step 1: SetPaneTitleHandler の失敗するテストを書く**

`core/application/command/set_pane_title_test.go`（同パッケージの既存テストのフェイク repo を流用。`set_pane_directory_test.go` を参照して同型に書く）:
```go
package command

import (
	"context"
	"testing"

	"github.com/ysksm/multi-terminals/core/domain"
)

func TestSetPaneTitleHandler(t *testing.T) {
	// set_pane_directory_test.go と同じ方式でワークスペース+ペインを用意した
	// フェイク WorkspaceRepository を構築する（既存のテストヘルパ/フェイクを再利用）。
	repo, wsID, paneID := newRepoWithOnePane(t) // 既存ヘルパが無ければ set_pane_directory_test.go の手順をコピー
	h := NewSetPaneTitleHandler(repo)

	err := h.Handle(context.Background(), SetPaneTitleCommand{
		WorkspaceID: wsID,
		PaneID:      paneID,
		Title:       "web server",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}

	w, _ := repo.FindByID(context.Background(), mustWsID(t, wsID))
	if got := w.Panes()[0].Title().String(); got != "web server" {
		t.Fatalf("title not persisted: got %q", got)
	}

	// 不正なワークスペース id は検証エラー
	if err := h.Handle(context.Background(), SetPaneTitleCommand{WorkspaceID: "", PaneID: paneID, Title: "x"}); err == nil {
		t.Fatal("invalid workspace id: expected error")
	}
}
```
注記: `newRepoWithOnePane` / `mustWsID` は **`set_pane_directory_test.go` に存在するヘルパ名に合わせる**こと。同等のものが無ければ、その隣接テストが使っているフェイク repo 構築コードをこのテスト用にインラインで複製する（同パッケージなので非公開ヘルパを共有可能）。テストはハンドラ経由でタイトルが保存されることを検証する。

- [ ] **Step 2: テストを実行して失敗を確認**

Run: `go test ./core/application/command/ -run TestSetPaneTitleHandler -v`
Expected: FAIL（`SetPaneTitleCommand`/`NewSetPaneTitleHandler` 未定義）。

- [ ] **Step 3: SetPaneTitleHandler を実装**

`core/application/command/set_pane_title.go`（`set_pane_directory.go` のコピー構造）:
```go
package command

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/domain"
)

// SetPaneTitleCommand は pane のタイトル変更コマンドの入力 DTO。
type SetPaneTitleCommand struct {
	WorkspaceID string
	PaneID      string
	Title       string
}

// SetPaneTitleHandler は pane タイトル変更コマンドを処理するハンドラ。
type SetPaneTitleHandler struct {
	repo domain.WorkspaceRepository
}

// NewSetPaneTitleHandler は依存を注入して SetPaneTitleHandler を返す。
func NewSetPaneTitleHandler(repo domain.WorkspaceRepository) *SetPaneTitleHandler {
	return &SetPaneTitleHandler{repo: repo}
}

// Handle は指定ワークスペースの指定 pane のタイトルを変更して保存する。
func (h *SetPaneTitleHandler) Handle(ctx context.Context, cmd SetPaneTitleCommand) error {
	wsID, err := domain.NewWorkspaceId(cmd.WorkspaceID)
	if err != nil {
		return apperr.Validation(fmt.Errorf("set pane title: invalid workspace id: %w", err))
	}

	w, err := h.repo.FindByID(ctx, wsID)
	if err != nil {
		return err
	}

	paneID, err := domain.NewPaneId(cmd.PaneID)
	if err != nil {
		return apperr.Validation(fmt.Errorf("set pane title: invalid pane id: %w", err))
	}

	title, err := domain.NewPaneTitle(cmd.Title)
	if err != nil {
		return apperr.Validation(fmt.Errorf("set pane title: invalid title: %w", err))
	}

	if err := w.SetPaneTitle(paneID, title); err != nil {
		return apperr.Validation(fmt.Errorf("set pane title: %w", err))
	}

	if err := h.repo.Save(ctx, w); err != nil {
		return fmt.Errorf("set pane title: save: %w", err)
	}

	return nil
}
```

- [ ] **Step 4: SetPaneTitleHandler テストを実行して成功を確認**

Run: `go test ./core/application/command/ -run TestSetPaneTitleHandler -v`
Expected: PASS

- [ ] **Step 5: AddPane に Title を追加**

`core/application/command/add_pane.go`:
- `AddPaneCommand` に追加:
  ```go
  type AddPaneCommand struct {
  	WorkspaceID string
  	Directory   string
  	Slot        int
  	Title       string
  	Commands    []StartupCommandInput
  }
  ```
- ハンドラ内、`NewPane` 呼び出し前に title を検証し、呼び出しを差し替え:
  ```go
  title, err := domain.NewPaneTitle(cmd.Title)
  if err != nil {
  	return AddPaneResult{}, apperr.Validation(fmt.Errorf("add pane: invalid title: %w", err))
  }

  pane, err := domain.NewPane(paneID, dir, slot, title, startupCmds)
  ```

- [ ] **Step 6: PaneDTO に Title を追加**

`core/application/query/dto.go`:
- `PaneDTO` に `Title string `json:"title"`` を追加（`Directory` の隣）。
- `toWorkspaceDTO` の `PaneDTO{...}` 構築に `Title: p.Title().String(),` を追加。

- [ ] **Step 7: AddPane の title 保存テストを追記（任意だが推奨）**

`add_pane` の既存テストファイルに、title を渡すと保存されることを確認する短いテストを追記（既存テストの構築手順を流用、`AddPaneCommand` に `Title: "db"` を設定し、保存後の pane の `Title().String()` を検証）。`go test ./core/application/...` が緑であること。

- [ ] **Step 8: アプリ層のビルド・テスト**

Run:
```bash
go build ./...
go test ./core/application/...
```
Expected: PASS

- [ ] **Step 9: コミット**

```bash
git add core/application
git commit -m "feat(application): SetPaneTitleHandler, AddPane title, PaneDTO.Title"
```

---

### Task 3: 永続化（jsonstore）— title の保存/復元 + 後方互換

**Files:**
- Modify: `core/infrastructure/jsonstore/schema.go`, `core/infrastructure/jsonstore/mapper.go`
- Test: `core/infrastructure/jsonstore/mapper_test.go`（既存）

**Interfaces:**
- Consumes: `domain.Pane.Title()`, `domain.NewPaneTitle`, `domain.NewPane(..., title, ...)`（Task 1）
- Produces: `paneRecord.Title` フィールド、ラウンドトリップでの title 保持

- [ ] **Step 1: 後方互換 + ラウンドトリップの失敗するテストを書く**

`core/infrastructure/jsonstore/mapper_test.go` に追記（既存テストの構築方式に合わせる）:
```go
func TestPaneTitleRoundTripAndBackwardCompat(t *testing.T) {
	// ラウンドトリップ: title を持つ pane を record 化→ドメイン復元で保持される
	title, _ := domain.NewPaneTitle("API")
	dir, _ := domain.NewDirectoryPath("/tmp")
	slot, _ := domain.NewSlotIndex(0)
	pid, _ := domain.NewPaneId("p1")
	pane, _ := domain.NewPane(pid, dir, slot, title, nil)
	wsID, _ := domain.NewWorkspaceId("w1")
	name, _ := domain.NewWorkspaceName("ws")
	w, _ := domain.NewWorkspace(wsID, name, domain.LayoutSingle) // 既存の生成 API に合わせる
	_ = w.AddPane(pane)

	rec := toRecord(w)                 // 既存のドメイン→record 関数名に合わせる
	got, err := toDomain(rec)          // 既存の record→ドメイン関数名に合わせる
	if err != nil {
		t.Fatalf("toDomain: %v", err)
	}
	if got.Panes()[0].Title().String() != "API" {
		t.Fatalf("round-trip lost title: %q", got.Panes()[0].Title().String())
	}

	// 後方互換: title フィールドが無い JSON を読み込んでも空タイトルで成功する
	const legacy = `{"version":1,"id":"w1","name":"ws","layout":"single","panes":[{"id":"p1","directory":"/tmp","slot":0,"commands":[]}]}`
	var lrec workspaceRecord
	if err := json.Unmarshal([]byte(legacy), &lrec); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	lw, err := toDomain(lrec)
	if err != nil {
		t.Fatalf("toDomain legacy: %v", err)
	}
	if !lw.Panes()[0].Title().IsZero() {
		t.Fatalf("legacy pane should have empty title, got %q", lw.Panes()[0].Title().String())
	}
}
```
注記: `toRecord`/`toDomain`/`NewWorkspace`/`LayoutSingle`/`NewWorkspaceName` は **mapper.go・既存テストで実際に使われている関数/値の名前に合わせる**こと（このリポジトリの実名を `grep` で確認してから書く）。テストの意図は「title がラウンドトリップで保持される」「title 欠落の旧 JSON が空 title で読める」の 2 点。

- [ ] **Step 2: テストを実行して失敗を確認**

Run: `go test ./core/infrastructure/jsonstore/ -run TestPaneTitleRoundTripAndBackwardCompat -v`
Expected: FAIL（`paneRecord` に Title が無く、復元時に title が空のまま → ラウンドトリップ assertion で失敗）。

- [ ] **Step 3: schema と mapper に title を追加**

`core/infrastructure/jsonstore/schema.go` の `paneRecord`:
```go
type paneRecord struct {
	ID        string                 `json:"id"`
	Directory string                 `json:"directory"`
	Slot      int                    `json:"slot"`
	Title     string                 `json:"title,omitempty"`
	Commands  []startupCommandRecord `json:"commands"`
}
```
`core/infrastructure/jsonstore/mapper.go`:
- 書き出し（`paneRecord{...}` 構築、`mapper.go:24` 付近）に `Title: p.Title().String(),` を追加。
- 読み込み（`mapper.go:110` 付近）で title を復元:
  ```go
  title, err := domain.NewPaneTitle(pr.Title)
  if err != nil {
  	return nil, fmt.Errorf("invalid title %q for pane %q: %w", pr.Title, pr.ID, err)
  }

  pane, err := domain.NewPane(paneID, dir, slot, title, cmds)
  ```
  （Task 1 で入れた一時的な `domain.PaneTitle{}` を置き換える。）

- [ ] **Step 4: テストを実行して成功を確認**

Run: `go test ./core/infrastructure/jsonstore/ -run TestPaneTitleRoundTripAndBackwardCompat -v`
Expected: PASS

- [ ] **Step 5: jsonstore 全体のテスト**

Run: `go test ./core/infrastructure/jsonstore/`
Expected: PASS（既存テストも緑）

- [ ] **Step 6: コミット**

```bash
git add core/infrastructure/jsonstore
git commit -m "feat(jsonstore): persist pane title (backward-compatible)"
```

---

### Task 4: Web — タイトル更新エンドポイント + AddPane の title 受理 + Deps 配線

**Files:**
- Modify: `apps/web/app.go`（`Deps.SetTitle` 配線）, `apps/web/server.go`（`Deps` フィールド・ルート・ハンドラ）
- Test: `apps/web/server_test.go` または `apps/web/app_test.go`（既存のハンドラテスト方式に合わせる）

**Interfaces:**
- Consumes: `command.NewSetPaneTitleHandler`, `command.SetPaneTitleCommand`, `command.AddPaneCommand.Title`（Task 2）
- Produces:
  - `web.Deps.SetTitle *command.SetPaneTitleHandler`
  - ルート `PUT /api/workspaces/{id}/panes/{paneId}/title` → `handleSetPaneTitle`
  - `handleAddPane` が body の `title` を受理

- [ ] **Step 1: ハンドラの失敗するテストを書く**

`apps/web/server_test.go`（既存のテスト方式・ヘルパに合わせて追記）:
```go
func TestSetPaneTitleEndpoint(t *testing.T) {
	// 既存のサーバーテストと同じ方式で Deps+mux を組み、ワークスペース+ペインを用意する。
	// （例: BuildDeps(t.TempDir()) → NewMux → ワークスペース作成 → ペイン追加）
	deps, mux := newTestServer(t)            // 既存ヘルパ名に合わせる
	wsID := createWorkspace(t, mux)          // 既存ヘルパ
	paneID := addPane(t, mux, wsID, "/tmp")  // 既存ヘルパ

	// PUT title
	body := `{"title":"My Server"}`
	req := httptest.NewRequest("PUT", "/api/workspaces/"+wsID+"/panes/"+paneID+"/title", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PUT title: got %d, want 204; body=%s", rec.Code, rec.Body.String())
	}

	// GET workspace shows the title
	w2 := getWorkspace(t, mux, wsID) // 既存ヘルパ、JSON デコード結果
	if w2.Panes[0].Title != "My Server" {
		t.Fatalf("title not reflected: %q", w2.Panes[0].Title)
	}

	_ = deps
}
```
注記: `newTestServer`/`createWorkspace`/`addPane`/`getWorkspace` は **既存のサーバーテストに存在するヘルパ名/手順に合わせる**こと。無ければ既存テストの該当処理をインラインで再現する。GET 結果の pane JSON に `Title` フィールド（`json:"title"`）が必要（Task 2 で追加済み）。

- [ ] **Step 2: テストを実行して失敗を確認**

Run: `go test ./apps/web/ -run TestSetPaneTitleEndpoint -v`
Expected: FAIL（ルート未登録のため 404、または `SetTitle`/フィールド未定義のコンパイルエラー）。

- [ ] **Step 3: Deps フィールドと配線を追加**

`apps/web/server.go` の `Deps` 構造体に追加（`SetDir` の隣）:
```go
	SetTitle      *command.SetPaneTitleHandler
```
`apps/web/app.go` の `BuildDeps` の返却 `Deps{...}` に追加（`SetDir:` の隣）:
```go
		SetTitle:      command.NewSetPaneTitleHandler(repo),
```

- [ ] **Step 4: ルートとハンドラを追加**

`apps/web/server.go` のルート登録（`directory` の隣、`server.go:61` 付近）:
```go
	mux.HandleFunc("PUT /api/workspaces/{id}/panes/{paneId}/title", d.handleSetPaneTitle)
```
ハンドラ（`handleSetPaneDirectory` の隣に追加）:
```go
func (d Deps) handleSetPaneTitle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	paneID := r.PathValue("paneId")
	var body struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := d.SetTitle.Handle(r.Context(), command.SetPaneTitleCommand{
		WorkspaceID: id,
		PaneID:      paneID,
		Title:       body.Title,
	}); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 5: handleAddPane で title を受理**

`apps/web/server.go` の `handleAddPane`（`:249`）:
- リクエスト body 構造体に `Title string `json:"title"`` を追加（`Slot` の隣）。
- `AddPaneCommand{...}` 構築に `Title: body.Title,` を追加。

- [ ] **Step 6: テストを実行して成功を確認**

Run: `go test ./apps/web/ -run TestSetPaneTitleEndpoint -v`
Expected: PASS

- [ ] **Step 7: web 全体のテスト**

Run: `go test ./apps/web/`
Expected: PASS（既存テストも緑）

- [ ] **Step 8: コミット**

```bash
git add apps/web
git commit -m "feat(web): add PUT pane title endpoint and accept title on add-pane"
```

---

### Task 5: フロントエンド — タイトル表示・インライン編集・追加フォーム入力

**Files:**
- Modify: `frontend/src/lib/api.js`, `frontend/src/App.svelte`

**Interfaces:**
- Consumes: `PUT /api/workspaces/{id}/panes/{paneId}/title`、pane JSON の `title`、`POST .../panes` の `title`（Task 4）
- Produces: ヘッダーのタイトル表示（`title || directory`）、インライン編集、追加フォームのタイトル欄

- [ ] **Step 1: api.js に setPaneTitle と addPane の title を追加**

`frontend/src/lib/api.js`:
- `addPane` を title 対応に変更:
  ```js
  addPane: (id, directory, slot, commands, title) =>
    req('POST', `/api/workspaces/${id}/panes`, { directory, slot, commands, title }),
  ```
- `setPaneDirectory` の隣に追加:
  ```js
  setPaneTitle: (id, paneId, title) =>
    req('PUT', `/api/workspaces/${id}/panes/${paneId}/title`, { title }),
  ```

- [ ] **Step 2: 追加フォームにタイトル入力を追加**

`frontend/src/App.svelte`:
- スクリプト先頭付近のフォーム状態（`paneDir` 等の宣言箇所）に `let paneTitle = $state('')` を追加。
- `startAddPane(slot)` で `paneTitle = ''` をリセット。
- `submitAddPane()` の `api.addPane(...)` 呼び出しに `paneTitle.trim()` を渡す:
  ```js
  await api.addPane(current.id, paneDir.trim(), addingSlot, commands, paneTitle.trim())
  ```
- 追加フォームのマークアップ（`作業ディレクトリ` の前）にタイトル欄を追加:
  ```svelte
  <label>タイトル（任意）
    <input placeholder="例: API サーバー" bind:value={paneTitle} />
  </label>
  ```

- [ ] **Step 3: ヘッダー表示をタイトル優先 + インライン編集に変更**

`frontend/src/App.svelte`:
- スクリプトにインライン編集の状態と関数を追加:
  ```js
  let editingTitlePaneId = $state(null)
  let titleDraft = $state('')

  function startEditTitle(pane) {
    editingTitlePaneId = pane.id
    titleDraft = pane.title || ''
  }
  function cancelEditTitle() {
    editingTitlePaneId = null
  }
  function commitEditTitle(paneId) {
    const next = titleDraft.trim()
    editingTitlePaneId = null
    guard(async () => {
      await api.setPaneTitle(current.id, paneId, next)
      await reloadCurrent()
    })
  }
  ```
- ヘッダー（`:238` 付近）の `<span class="dir" ...>` を、編集状態で切り替わる表示に変更:
  ```svelte
  {#if editingTitlePaneId === cell.pane.id}
    <input
      class="title-edit"
      bind:value={titleDraft}
      onkeydown={(e) => {
        if (e.key === 'Enter') commitEditTitle(cell.pane.id)
        else if (e.key === 'Escape') cancelEditTitle()
      }}
      onblur={() => commitEditTitle(cell.pane.id)}
      autofocus
    />
  {:else}
    <span
      class="dir"
      title={cell.pane.directory}
      role="button"
      tabindex="0"
      onclick={() => startEditTitle(cell.pane)}
      onkeydown={(e) => { if (e.key === 'Enter') startEditTitle(cell.pane) }}
    >{cell.pane.title || cell.pane.directory}</span>
  {/if}
  ```
  （`reloadCurrent`/`guard`/`current` は既存のものを使用。`.title-edit` の最小スタイルを `<style>` に追加してよいが必須ではない。）

- [ ] **Step 4: フロントのビルドを確認**

Run: `(cd frontend && npm run build)`
Expected: ビルド成功（エラーなし）。`frontend/package-lock.json` が変化したら戻す（`git checkout -- frontend/package-lock.json`）。

- [ ] **Step 5: エンドツーエンドのスモーク**

Run:
```bash
rm -rf apps/web/webui/dist && mkdir -p apps/web/webui/dist && touch apps/web/webui/dist/.gitkeep
cp -R frontend/dist/. apps/web/webui/dist/
go build -o bin/multi-terminals ./apps/web/cmd
PORT=18140 ./bin/multi-terminals >/tmp/mt-title.log 2>&1 &
sleep 2
# ワークスペース作成→ペイン追加(タイトル付き)→タイトル更新→取得 を curl で確認
WS=$(curl -s -XPOST localhost:18140/api/workspaces -d '{"name":"t","layout":"single"}' | sed -E 's/.*"id":"([^"]+)".*/\1/')
PANE=$(curl -s -XPOST localhost:18140/api/workspaces/$WS/panes -d '{"directory":"/tmp","slot":0,"title":"first","commands":[]}' | sed -E 's/.*"paneId":"([^"]+)".*/\1/')
curl -s -o /dev/null -w "PUT title -> %{http_code}\n" -XPUT localhost:18140/api/workspaces/$WS/panes/$PANE/title -d '{"title":"renamed"}'
curl -s localhost:18140/api/workspaces/$WS | grep -o '"title":"renamed"' && echo "title reflected OK"
kill %1 2>/dev/null || true
```
Expected: `PUT title -> 204`、`"title":"renamed"` が GET 結果に現れる、`title reflected OK`。

- [ ] **Step 6: コミット**

```bash
git add frontend/src/lib/api.js frontend/src/App.svelte
git commit -m "feat(frontend): per-pane title display, inline edit, and add-form field"
```

---

## Self-Review

**1. Spec coverage:**
- PaneTitle VO（空許容/トリム/最大長/制御文字）→ Task 1 ✓
- Pane.title + Workspace.SetPaneTitle + NewPane 署名 → Task 1 ✓
- SetPaneTitleHandler / AddPane.Title / PaneDTO.Title → Task 2 ✓
- 永続化 + 後方互換 → Task 3 ✓
- Web エンドポイント + AddPane title + Deps 配線 → Task 4 ✓
- フロント 表示(title||directory)/インライン編集/追加フォーム → Task 5 ✓
- スコープ外（自動生成・色/アイコン・タブ刷新）→ どのタスクにも含めない ✓

**2. Placeholder scan:** すべてのコード/コマンドは実体記載。テストヘルパ名は「既存名に合わせる」と明示（リポジトリ固有のため実名は実装時に grep 確認）。TBD なし。

**3. Type consistency:**
- `NewPane(id, directory, slot, title, commands)` の新署名は Task 1 で定義し、Task 2(add_pane)・Task 3(mapper) で同一順序で使用。Task 1 は一時的に `domain.PaneTitle{}` を渡し、Task 2/3 が実値へ置換。
- `SetPaneTitleCommand{WorkspaceID, PaneID, Title}`・`NewSetPaneTitleHandler`・`Deps.SetTitle` は Task 2→Task 4 で一致。
- `PaneDTO.Title`（`json:"title"`）は Task 2 で追加、Task 4 のテストと Task 5 のフロントが参照。
- フロント `api.setPaneTitle(id, paneId, title)`・`addPane(..., title)` は Task 5 内で一貫。

## 既知の注意点
- テストヘルパ（フェイク repo / サーバーテスト補助）の実名はリポジトリ固有。各テストタスクの Step は「隣接する既存テスト（`set_pane_directory_test.go`、既存の `apps/web` テスト、`mapper_test.go`）の方式に合わせる」前提。実装者は対象パッケージの既存テストを先に読むこと。
- `NewPane` 署名変更は全呼び出し箇所（テスト含む）に波及する。Task 1 Step 9 の grep で漏れなく更新する。
