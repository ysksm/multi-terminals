# ペイン git 操作(ブランチ切替・pull・push・fetch)実装プラン

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** ペインヘッダの git バッジから、ブランチ切替・pull・push・fetch を実行できるようにする(ショートカット Ctrl+Shift+B 対応)。

**Architecture:** 既存の GitService ポートにメソッドを追加 → gitcli 実装 → application 層 CQRS(query 1 + command 2) → REST エンドポイント → Svelte ドロップダウンメニュー、の積み上げ。spec: `docs/superpowers/specs/2026-07-07-pane-git-ops-design.md`

**Tech Stack:** Go(標準ライブラリ + git CLI)、Svelte 5 runes、Vite、node 素の assert スクリプトテスト

## Global Constraints

- Go テストは `go test ./core/... ./apps/...`、CI 相当は `./scripts/dev.sh check`(build+vet+test)。
- gitcli は git CLI 実行(go-git 不使用)。ネットワーク系 git は `GIT_TERMINAL_PROMPT=0` + 60 秒タイムアウト必須。
- コメント・エラーメッセージは既存流儀に合わせ日本語(エラー文言は `gitcli: xxx` 等の既存形式)。
- フロントの node テストは素のスクリプト(`node frontend/src/lib/<file>.node.test.mjs` で実行、成功時 `console.log('xxx: OK')`)。
- コミットメッセージ末尾: `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`

---

### Task 1: GitService ポート拡張(Branches/Checkout) + gitcli 実装

**Files:**
- Modify: `core/application/port/git_service.go`
- Modify: `core/application/apptest/git_fakes.go`
- Modify: `core/infrastructure/gitcli/gitcli.go`
- Test: `core/infrastructure/gitcli/gitcli_test.go`

**Interfaces:**
- Consumes: 既存 `port.GitService`、gitcli の `git(dir, args...)` ヘルパ。
- Produces: `port.BranchInfo{Name string, IsCurrent bool, IsRemote bool}`、`GitService.Branches(dir string) ([]BranchInfo, error)`、`GitService.Checkout(dir, branch string) error`。fake は `FakeGitService.BranchLists map[string][]port.BranchInfo`、`FakeGitService.Checkouts []CheckoutCall`、`FakeGitService.CheckoutErr error`。

- [ ] **Step 1: 失敗するテストを書く**

`core/infrastructure/gitcli/gitcli_test.go` の末尾に追加(既存の `initRepo` ヘルパを利用):

```go
// initClonePair は bare リモートとその clone を作り (bare, clone) を返す。
func initClonePair(t *testing.T) (bare, clone string) {
	t.Helper()
	src := initRepo(t)
	bare = filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "clone", "--bare", src, bare).CombinedOutput(); err != nil {
		t.Fatalf("clone --bare: %v\n%s", err, out)
	}
	clone = filepath.Join(t.TempDir(), "clone")
	if out, err := exec.Command("git", "clone", bare, clone).CombinedOutput(); err != nil {
		t.Fatalf("clone: %v\n%s", err, out)
	}
	return bare, clone
}

// runGit は dir で git を実行して失敗なら Fatal。
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestBranches_LocalAndRemote(t *testing.T) {
	_, clone := initClonePair(t)
	// ローカルブランチ feature を作り、リモートのみのブランチ remote-only を作る
	runGit(t, clone, "branch", "feature")
	runGit(t, clone, "push", "origin", "main:remote-only")
	runGit(t, clone, "fetch", "origin")

	s := New()
	branches, err := s.Branches(clone)
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}
	got := map[string]port.BranchInfo{}
	for _, b := range branches {
		got[b.Name] = b
	}
	if b := got["main"]; !b.IsCurrent || b.IsRemote {
		t.Errorf("main = %+v, want {IsCurrent:true IsRemote:false}", b)
	}
	if b := got["feature"]; b.IsCurrent || b.IsRemote {
		t.Errorf("feature = %+v, want {IsCurrent:false IsRemote:false}", b)
	}
	if b := got["remote-only"]; !b.IsRemote {
		t.Errorf("remote-only = %+v, want {IsRemote:true}", b)
	}
	if _, ok := got["HEAD"]; ok {
		t.Error("origin/HEAD が混入している")
	}
	// main はローカル優先で 1 件のみ(origin/main と重複しない)
	count := 0
	for _, b := range branches {
		if b.Name == "main" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("main が %d 件(重複除去されていない)", count)
	}
}

func TestCheckout_LocalBranch(t *testing.T) {
	dir := initRepo(t)
	runGit(t, dir, "branch", "feature")

	s := New()
	if err := s.Checkout(dir, "feature"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	info, err := s.Info(dir)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Branch != "feature" {
		t.Errorf("branch = %q, want feature", info.Branch)
	}
}

func TestCheckout_RemoteOnlyBranchCreatesTracking(t *testing.T) {
	_, clone := initClonePair(t)
	runGit(t, clone, "push", "origin", "main:remote-only")
	runGit(t, clone, "fetch", "origin")

	s := New()
	if err := s.Checkout(clone, "remote-only"); err != nil {
		t.Fatalf("Checkout(remote-only): %v", err)
	}
	info, err := s.Info(clone)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Branch != "remote-only" {
		t.Errorf("branch = %q, want remote-only", info.Branch)
	}
}

func TestCheckout_UnknownBranchFails(t *testing.T) {
	dir := initRepo(t)
	s := New()
	if err := s.Checkout(dir, "no-such-branch"); err == nil {
		t.Fatal("expected error for unknown branch, got nil")
	}
}
```

import に `"github.com/ysksm/multi-terminals/core/application/port"` を追加する。

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./core/infrastructure/gitcli/ -run 'TestBranches|TestCheckout' -v`
Expected: FAIL(`s.Branches undefined` / `s.Checkout undefined` のコンパイルエラー)

- [ ] **Step 3: ポート・fake・gitcli を実装**

`core/application/port/git_service.go` — `GitInfo` の後に追加し、interface にメソッドを足す:

```go
// BranchInfo は 1 ブランチの情報。
type BranchInfo struct {
	Name      string // ローカル名。リモートのみの場合も origin/ プレフィックスを除いた名前
	IsCurrent bool
	IsRemote  bool // リモートにのみ存在(ローカル未チェックアウト)
}
```

interface に追加:

```go
	// Branches はローカル + リモート追跡ブランチを返す。ローカルと同名の
	// リモートブランチはローカル優先で重複除去する。origin/HEAD は除外。
	Branches(dir string) ([]BranchInfo, error)

	// Checkout は branch に切り替える(git switch 相当。リモートのみの
	// ブランチは追跡ブランチを自動作成して切り替える)。
	Checkout(dir, branch string) error
```

`core/application/apptest/git_fakes.go` — 末尾に追加、struct にフィールド追加:

```go
// CheckoutCall は FakeGitService.Checkout の呼び出し記録。
type CheckoutCall struct {
	Dir    string
	Branch string
}
```

`FakeGitService` struct に追加:

```go
	BranchLists map[string][]port.BranchInfo // dir -> branches
	Checkouts   []CheckoutCall
	CheckoutErr error
```

`NewFakeGitService` の初期化に `BranchLists: make(map[string][]port.BranchInfo),` を追加。メソッド:

```go
func (f *FakeGitService) Branches(dir string) ([]port.BranchInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.BranchLists[dir], nil
}

func (f *FakeGitService) Checkout(dir, branch string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.CheckoutErr != nil {
		return f.CheckoutErr
	}
	f.Checkouts = append(f.Checkouts, CheckoutCall{Dir: dir, Branch: branch})
	return nil
}
```

`core/infrastructure/gitcli/gitcli.go` — 末尾に追加:

```go
// splitLines は出力を行に分割し、空行を除いて返す。
func splitLines(out string) []string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// Branches はローカル + リモート追跡ブランチを返す。ローカルと同名の
// リモートブランチはローカル優先で重複除去する。
func (s *Service) Branches(dir string) ([]port.BranchInfo, error) {
	// detached HEAD では current は空のまま(どの行も IsCurrent=false)
	current, _ := git(dir, "symbolic-ref", "--short", "-q", "HEAD")

	localOut, err := git(dir, "branch", "--format=%(refname:short)")
	if err != nil {
		return nil, fmt.Errorf("gitcli: branch: %w", err)
	}
	remoteOut, err := git(dir, "branch", "-r", "--format=%(refname:short)")
	if err != nil {
		return nil, fmt.Errorf("gitcli: branch -r: %w", err)
	}

	var branches []port.BranchInfo
	seen := map[string]bool{}
	for _, name := range splitLines(localOut) {
		// detached HEAD の擬似エントリ "(HEAD detached at ...)" は除外
		if strings.HasPrefix(name, "(") {
			continue
		}
		seen[name] = true
		branches = append(branches, port.BranchInfo{Name: name, IsCurrent: name == current})
	}
	for _, ref := range splitLines(remoteOut) {
		// "origin/HEAD -> origin/main" 表記と origin/HEAD を除外
		if strings.Contains(ref, " ") || strings.HasSuffix(ref, "/HEAD") {
			continue
		}
		slash := strings.Index(ref, "/")
		if slash < 0 {
			continue
		}
		name := ref[slash+1:]
		if seen[name] {
			continue
		}
		seen[name] = true
		branches = append(branches, port.BranchInfo{Name: name, IsRemote: true})
	}
	return branches, nil
}

// Checkout は branch に切り替える。リモートのみのブランチは git switch が
// 追跡ブランチを自動作成する。
func (s *Service) Checkout(dir, branch string) error {
	if _, err := git(dir, "switch", branch); err != nil {
		return fmt.Errorf("gitcli: switch: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./core/... ./apps/...`
Expected: PASS(全パッケージ。interface 拡張のコンパイル影響が fake/gitcli 両方で解消されていること)

- [ ] **Step 5: コミット**

```bash
git add core/application/port/git_service.go core/application/apptest/git_fakes.go core/infrastructure/gitcli/gitcli.go core/infrastructure/gitcli/gitcli_test.go
git commit -m "feat(git): add Branches/Checkout to GitService port and gitcli

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: GitService ポート拡張(Pull/Push/Fetch) + gitcli 実装

**Files:**
- Modify: `core/application/port/git_service.go`
- Modify: `core/application/apptest/git_fakes.go`
- Modify: `core/infrastructure/gitcli/gitcli.go`
- Test: `core/infrastructure/gitcli/gitcli_test.go`

**Interfaces:**
- Consumes: Task 1 の `initClonePair` / `runGit` テストヘルパ。
- Produces: `GitService.Pull(dir string) error`、`Push(dir string) error`、`Fetch(dir string) error`。fake は `FakeGitService.GitOps []GitOpCall`(`GitOpCall{Dir, Op string}`)、`FakeGitService.OpErr error`(Pull/Push/Fetch 共通のエラー注入)。

- [ ] **Step 1: 失敗するテストを書く**

`core/infrastructure/gitcli/gitcli_test.go` の末尾に追加:

```go
func TestPush(t *testing.T) {
	bare, clone := initClonePair(t)
	if err := os.WriteFile(filepath.Join(clone, "new.txt"), []byte("n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, clone, "add", ".")
	runGit(t, clone, "commit", "-m", "new")

	s := New()
	if err := s.Push(clone); err != nil {
		t.Fatalf("Push: %v", err)
	}
	// bare 側の main が clone の HEAD と一致する
	cmdOut := func(dir string, args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).Output()
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return strings.TrimSpace(string(out))
	}
	if cmdOut(bare, "rev-parse", "main") != cmdOut(clone, "rev-parse", "HEAD") {
		t.Error("push 後も bare の main が更新されていない")
	}
}

func TestPullAndFetch(t *testing.T) {
	bare, cloneA := initClonePair(t)
	cloneB := filepath.Join(t.TempDir(), "cloneB")
	if out, err := exec.Command("git", "clone", bare, cloneB).CombinedOutput(); err != nil {
		t.Fatalf("clone B: %v\n%s", err, out)
	}

	// A 側でコミットして push、新ブランチも push
	if err := os.WriteFile(filepath.Join(cloneA, "from-a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, cloneA, "add", ".")
	runGit(t, cloneA, "commit", "-m", "from A")
	runGit(t, cloneA, "push", "origin", "main")
	runGit(t, cloneA, "push", "origin", "main:new-branch")

	s := New()
	// Fetch で新リモートブランチが見える
	if err := s.Fetch(cloneB); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	branches, err := s.Branches(cloneB)
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}
	found := false
	for _, b := range branches {
		if b.Name == "new-branch" && b.IsRemote {
			found = true
		}
	}
	if !found {
		t.Errorf("fetch 後に new-branch が見えない: %+v", branches)
	}

	// Pull で from-a.txt が取り込まれる
	if err := s.Pull(cloneB); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cloneB, "from-a.txt")); err != nil {
		t.Errorf("pull 後に from-a.txt が無い: %v", err)
	}
}

func TestFetch_BadRemoteFails(t *testing.T) {
	dir := initRepo(t)
	runGit(t, dir, "remote", "add", "origin", filepath.Join(t.TempDir(), "no-such-repo"))
	s := New()
	if err := s.Fetch(dir); err == nil {
		t.Fatal("expected error for missing remote repo, got nil")
	}
}
```

import に `"strings"` を追加する(未追加の場合)。

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./core/infrastructure/gitcli/ -run 'TestPush|TestPullAndFetch|TestFetch_BadRemote' -v`
Expected: FAIL(`s.Push undefined` 等のコンパイルエラー)

- [ ] **Step 3: ポート・fake・gitcli を実装**

`core/application/port/git_service.go` の interface に追加:

```go
	// Pull は現在ブランチを pull する。認証が必要な場合は即エラー。
	Pull(dir string) error

	// Push は現在ブランチを push する。upstream 未設定は git のエラーを返す。
	Push(dir string) error

	// Fetch は全リモートを fetch --prune する。
	Fetch(dir string) error
```

`core/application/apptest/git_fakes.go` に追加:

```go
// GitOpCall は FakeGitService の Pull/Push/Fetch 呼び出し記録。
type GitOpCall struct {
	Dir string
	Op  string // "pull" | "push" | "fetch"
}
```

`FakeGitService` struct に追加:

```go
	GitOps []GitOpCall
	OpErr  error // Pull/Push/Fetch 共通のエラー注入
```

メソッド:

```go
func (f *FakeGitService) recordOp(dir, op string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.OpErr != nil {
		return f.OpErr
	}
	f.GitOps = append(f.GitOps, GitOpCall{Dir: dir, Op: op})
	return nil
}

func (f *FakeGitService) Pull(dir string) error  { return f.recordOp(dir, "pull") }
func (f *FakeGitService) Push(dir string) error  { return f.recordOp(dir, "push") }
func (f *FakeGitService) Fetch(dir string) error { return f.recordOp(dir, "fetch") }
```

`core/infrastructure/gitcli/gitcli.go` — import に `"context"`、`"time"` を追加し、末尾に追加:

```go
// netTimeout はネットワークを伴う git 操作の上限時間。
const netTimeout = 60 * time.Second

// gitNet は認証プロンプト無効(GIT_TERMINAL_PROMPT=0)・タイムアウト付きで
// git を実行する。pull/push/fetch などリモートと通信する操作に使う。
func gitNet(dir string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), netTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("git %s: %v でタイムアウトしました", args[0], netTimeout)
		}
		return fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(errBuf.String()))
	}
	return nil
}

// Pull は現在ブランチを pull する。
func (s *Service) Pull(dir string) error { return gitNet(dir, "pull") }

// Push は現在ブランチを push する。
func (s *Service) Push(dir string) error { return gitNet(dir, "push") }

// Fetch は全リモートを fetch --prune する。
func (s *Service) Fetch(dir string) error { return gitNet(dir, "fetch", "--prune") }
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./core/... ./apps/...`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
git add core/application/port/git_service.go core/application/apptest/git_fakes.go core/infrastructure/gitcli/gitcli.go core/infrastructure/gitcli/gitcli_test.go
git commit -m "feat(git): add Pull/Push/Fetch to GitService port and gitcli

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: application 層 query — ListPaneBranches

**Files:**
- Create: `core/application/query/list_pane_branches.go`
- Test: `core/application/query/list_pane_branches_test.go`

**Interfaces:**
- Consumes: Task 1 の `port.BranchInfo` / `GitService.Branches`、fake の `BranchLists`。既存 `domain.WorkspaceRepository`、`apperr.Validation`。テストは既存 `query_test.setupPane`(get_pane_git_info_test.go に定義済み、同パッケージなので利用可)。
- Produces: `query.PaneBranchDTO{Name, IsCurrent, IsRemote}`(json タグ `name`/`isCurrent`/`isRemote`)、`query.ListPaneBranchesQuery{WorkspaceID, PaneID string}`、`query.NewListPaneBranchesHandler(repo domain.WorkspaceRepository, git port.GitService) *ListPaneBranchesHandler`、`Handle(ctx, q) ([]PaneBranchDTO, error)`(nil でなく空 slice を返す)。

- [ ] **Step 1: 失敗するテストを書く**

`core/application/query/list_pane_branches_test.go`:

```go
package query_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/application/query"
	"github.com/ysksm/multi-terminals/core/domain"
)

func TestListPaneBranchesHandler(t *testing.T) {
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupPane(t, repo, "/tmp/project")
	git := apptest.NewFakeGitService()
	git.BranchLists["/tmp/project"] = []port.BranchInfo{
		{Name: "main", IsCurrent: true},
		{Name: "feature", IsRemote: true},
	}

	h := query.NewListPaneBranchesHandler(repo, git)
	dtos, err := h.Handle(context.Background(), query.ListPaneBranchesQuery{WorkspaceID: wsID, PaneID: paneID})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(dtos) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(dtos), dtos)
	}
	if dtos[0].Name != "main" || !dtos[0].IsCurrent || dtos[0].IsRemote {
		t.Errorf("dtos[0] = %+v", dtos[0])
	}
	if dtos[1].Name != "feature" || dtos[1].IsCurrent || !dtos[1].IsRemote {
		t.Errorf("dtos[1] = %+v", dtos[1])
	}
}

func TestListPaneBranchesHandler_EmptyIsNotNil(t *testing.T) {
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupPane(t, repo, "/tmp/plain")
	git := apptest.NewFakeGitService()

	h := query.NewListPaneBranchesHandler(repo, git)
	dtos, err := h.Handle(context.Background(), query.ListPaneBranchesQuery{WorkspaceID: wsID, PaneID: paneID})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if dtos == nil {
		t.Error("dtos は nil でなく空 slice を返す(JSON で null にしない)")
	}
}

func TestListPaneBranchesHandler_WorkspaceNotFound(t *testing.T) {
	repo := apptest.NewFakeRepo()
	git := apptest.NewFakeGitService()
	h := query.NewListPaneBranchesHandler(repo, git)
	_, err := h.Handle(context.Background(), query.ListPaneBranchesQuery{WorkspaceID: "nope", PaneID: "p"})
	if !errors.Is(err, domain.ErrWorkspaceNotFound) {
		t.Errorf("expected ErrWorkspaceNotFound, got %v", err)
	}
}

func TestListPaneBranchesHandler_PaneNotFound(t *testing.T) {
	repo := apptest.NewFakeRepo()
	wsID, _ := setupPane(t, repo, "/tmp/project")
	git := apptest.NewFakeGitService()
	h := query.NewListPaneBranchesHandler(repo, git)
	_, err := h.Handle(context.Background(), query.ListPaneBranchesQuery{WorkspaceID: wsID, PaneID: "nope"})
	if err == nil {
		t.Fatal("expected error for missing pane, got nil")
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./core/application/query/ -run TestListPaneBranches -v`
Expected: FAIL(コンパイルエラー: `query.NewListPaneBranchesHandler` 未定義)

- [ ] **Step 3: 実装**

`core/application/query/list_pane_branches.go`:

```go
package query

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain"
)

// PaneBranchDTO は pane の作業ディレクトリの 1 ブランチの読み取りモデル。
type PaneBranchDTO struct {
	Name      string `json:"name"`
	IsCurrent bool   `json:"isCurrent"`
	IsRemote  bool   `json:"isRemote"`
}

// ListPaneBranchesQuery は pane のブランチ一覧取得クエリの入力 DTO。
type ListPaneBranchesQuery struct {
	WorkspaceID string
	PaneID      string
}

// ListPaneBranchesHandler は pane の作業ディレクトリのブランチ一覧を返すハンドラ。
type ListPaneBranchesHandler struct {
	repo domain.WorkspaceRepository
	git  port.GitService
}

// NewListPaneBranchesHandler は依存を注入して ListPaneBranchesHandler を返す。
func NewListPaneBranchesHandler(repo domain.WorkspaceRepository, git port.GitService) *ListPaneBranchesHandler {
	return &ListPaneBranchesHandler{repo: repo, git: git}
}

// Handle は指定 pane のディレクトリのブランチ一覧を返す。
func (h *ListPaneBranchesHandler) Handle(ctx context.Context, q ListPaneBranchesQuery) ([]PaneBranchDTO, error) {
	wsID, err := domain.NewWorkspaceId(q.WorkspaceID)
	if err != nil {
		return nil, apperr.Validation(fmt.Errorf("list pane branches: invalid workspace id: %w", err))
	}

	w, err := h.repo.FindByID(ctx, wsID)
	if err != nil {
		return nil, err
	}

	paneID, err := domain.NewPaneId(q.PaneID)
	if err != nil {
		return nil, apperr.Validation(fmt.Errorf("list pane branches: invalid pane id: %w", err))
	}

	var dir string
	for _, p := range w.Panes() {
		if p.ID().Equals(paneID) {
			dir = p.Directory().String()
			break
		}
	}
	if dir == "" {
		return nil, apperr.Validation(fmt.Errorf("list pane branches: pane not found: %s", q.PaneID))
	}

	branches, err := h.git.Branches(dir)
	if err != nil {
		return nil, fmt.Errorf("list pane branches: %w", err)
	}
	dtos := make([]PaneBranchDTO, len(branches))
	for i, b := range branches {
		dtos[i] = PaneBranchDTO{Name: b.Name, IsCurrent: b.IsCurrent, IsRemote: b.IsRemote}
	}
	return dtos, nil
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./core/application/query/ -v -run TestListPaneBranches`
Expected: PASS(4 テスト)

- [ ] **Step 5: コミット**

```bash
git add core/application/query/list_pane_branches.go core/application/query/list_pane_branches_test.go
git commit -m "feat(app): add ListPaneBranches query

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: application 層 command — CheckoutPaneBranch / RunPaneGitOp

**Files:**
- Create: `core/application/command/checkout_pane_branch.go`
- Create: `core/application/command/run_pane_git_op.go`
- Test: `core/application/command/checkout_pane_branch_test.go`
- Test: `core/application/command/run_pane_git_op_test.go`

**Interfaces:**
- Consumes: Task 1/2 の fake(`Checkouts`/`CheckoutErr`/`GitOps`/`OpErr`)。command パッケージのテストには query パッケージの setupPane が無いので同等ヘルパをテスト内に定義する。
- Produces:
  - `command.CheckoutPaneBranchCommand{WorkspaceID, PaneID, Branch string}`、`NewCheckoutPaneBranchHandler(repo, git) *CheckoutPaneBranchHandler`、`Handle(ctx, cmd) error`
  - 定数 `command.GitOpPull = "pull"` / `GitOpPush = "push"` / `GitOpFetch = "fetch"`
  - `command.RunPaneGitOpCommand{WorkspaceID, PaneID, Op string}`、`NewRunPaneGitOpHandler(repo, git) *RunPaneGitOpHandler`、`Handle(ctx, cmd) error`
  - git 失敗は `apperr.Validation` で包む(clone_repository.go と同じ方針。HTTP では 400 になり UI がメッセージ表示)。

- [ ] **Step 1: 失敗するテストを書く**

`core/application/command/checkout_pane_branch_test.go`:

```go
package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
)

// setupGitPane はワークスペース + pane(/tmp/project)を作って ID を返す。
func setupGitPane(t *testing.T, repo *apptest.FakeRepo) (wsID, paneID string) {
	t.Helper()
	ctx := context.Background()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")
	createWS := command.NewCreateWorkspaceHandler(repo, idgen)
	wsResult, err := createWS.Handle(ctx, command.CreateWorkspaceCommand{Name: "WS", Layout: "single"})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	addPane := command.NewAddPaneHandler(repo, idgen)
	paneResult, err := addPane.Handle(ctx, command.AddPaneCommand{
		WorkspaceID: wsResult.WorkspaceID,
		Directory:   "/tmp/project",
		Slot:        0,
	})
	if err != nil {
		t.Fatalf("add pane: %v", err)
	}
	return wsResult.WorkspaceID, paneResult.PaneID
}

func TestCheckoutPaneBranch(t *testing.T) {
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupGitPane(t, repo)
	git := apptest.NewFakeGitService()

	h := command.NewCheckoutPaneBranchHandler(repo, git)
	err := h.Handle(context.Background(), command.CheckoutPaneBranchCommand{
		WorkspaceID: wsID, PaneID: paneID, Branch: "feature",
	})
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(git.Checkouts) != 1 || git.Checkouts[0].Dir != "/tmp/project" || git.Checkouts[0].Branch != "feature" {
		t.Errorf("Checkouts = %+v", git.Checkouts)
	}
}

func TestCheckoutPaneBranch_EmptyBranch(t *testing.T) {
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupGitPane(t, repo)
	git := apptest.NewFakeGitService()

	h := command.NewCheckoutPaneBranchHandler(repo, git)
	err := h.Handle(context.Background(), command.CheckoutPaneBranchCommand{
		WorkspaceID: wsID, PaneID: paneID, Branch: "  ",
	})
	var ve *apperr.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("expected ValidationError, got %v", err)
	}
}

func TestCheckoutPaneBranch_GitErrorIsValidation(t *testing.T) {
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupGitPane(t, repo)
	git := apptest.NewFakeGitService()
	git.CheckoutErr = errors.New("gitcli: switch: 未コミットの変更で失敗")

	h := command.NewCheckoutPaneBranchHandler(repo, git)
	err := h.Handle(context.Background(), command.CheckoutPaneBranchCommand{
		WorkspaceID: wsID, PaneID: paneID, Branch: "feature",
	})
	var ve *apperr.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("expected ValidationError (400 で UI にメッセージ表示), got %v", err)
	}
}
```

`core/application/command/run_pane_git_op_test.go`:

```go
package command_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
)

func TestRunPaneGitOp_AllOps(t *testing.T) {
	for _, op := range []string{command.GitOpPull, command.GitOpPush, command.GitOpFetch} {
		repo := apptest.NewFakeRepo()
		wsID, paneID := setupGitPane(t, repo)
		git := apptest.NewFakeGitService()

		h := command.NewRunPaneGitOpHandler(repo, git)
		err := h.Handle(context.Background(), command.RunPaneGitOpCommand{
			WorkspaceID: wsID, PaneID: paneID, Op: op,
		})
		if err != nil {
			t.Fatalf("handle(%s): %v", op, err)
		}
		if len(git.GitOps) != 1 || git.GitOps[0].Op != op || git.GitOps[0].Dir != "/tmp/project" {
			t.Errorf("GitOps(%s) = %+v", op, git.GitOps)
		}
	}
}

func TestRunPaneGitOp_UnknownOp(t *testing.T) {
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupGitPane(t, repo)
	git := apptest.NewFakeGitService()

	h := command.NewRunPaneGitOpHandler(repo, git)
	err := h.Handle(context.Background(), command.RunPaneGitOpCommand{
		WorkspaceID: wsID, PaneID: paneID, Op: "rebase",
	})
	var ve *apperr.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("expected ValidationError, got %v", err)
	}
	if len(git.GitOps) != 0 {
		t.Errorf("未知 op で git が呼ばれた: %+v", git.GitOps)
	}
}

func TestRunPaneGitOp_GitErrorIsValidation(t *testing.T) {
	repo := apptest.NewFakeRepo()
	wsID, paneID := setupGitPane(t, repo)
	git := apptest.NewFakeGitService()
	git.OpErr = errors.New("git pull: 認証エラー")

	h := command.NewRunPaneGitOpHandler(repo, git)
	err := h.Handle(context.Background(), command.RunPaneGitOpCommand{
		WorkspaceID: wsID, PaneID: paneID, Op: command.GitOpPull,
	})
	var ve *apperr.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("expected ValidationError, got %v", err)
	}
}
```

注意: `setupGitPane` が既存の command テストヘルパと名前衝突しないこと(`grep -rn "func setupGitPane" core/application/command/` が新規ファイルのみを返すこと)を確認。衝突する場合はテスト側の関数名を `setupGitOpsPane` に変える。

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./core/application/command/ -run 'TestCheckoutPaneBranch|TestRunPaneGitOp' -v`
Expected: FAIL(コンパイルエラー: ハンドラ未定義)

- [ ] **Step 3: 実装**

`core/application/command/checkout_pane_branch.go`:

```go
package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain"
)

// CheckoutPaneBranchCommand は pane の作業ディレクトリのブランチ切替コマンドの入力 DTO。
type CheckoutPaneBranchCommand struct {
	WorkspaceID string
	PaneID      string
	Branch      string
}

// CheckoutPaneBranchHandler は pane のディレクトリでブランチを切り替えるハンドラ。
type CheckoutPaneBranchHandler struct {
	repo domain.WorkspaceRepository
	git  port.GitService
}

// NewCheckoutPaneBranchHandler は依存を注入して CheckoutPaneBranchHandler を返す。
func NewCheckoutPaneBranchHandler(repo domain.WorkspaceRepository, git port.GitService) *CheckoutPaneBranchHandler {
	return &CheckoutPaneBranchHandler{repo: repo, git: git}
}

// Handle は指定 pane のディレクトリで branch に切り替える。dirty で切り替え
// られない等の git の失敗は ValidationError として返し、UI にそのまま表示する。
func (h *CheckoutPaneBranchHandler) Handle(ctx context.Context, cmd CheckoutPaneBranchCommand) error {
	branch := strings.TrimSpace(cmd.Branch)
	if branch == "" {
		return apperr.Validation(fmt.Errorf("checkout pane branch: branch is required"))
	}

	dir, err := paneDirForGit(ctx, h.repo, cmd.WorkspaceID, cmd.PaneID, "checkout pane branch")
	if err != nil {
		return err
	}

	if err := h.git.Checkout(dir, branch); err != nil {
		return apperr.Validation(fmt.Errorf("checkout pane branch: %w", err))
	}
	return nil
}
```

`core/application/command/run_pane_git_op.go`(pane→dir 解決の共通ヘルパ `paneDirForGit` はこちらに置く。checkout と gitop の 2 ハンドラで共有):

```go
package command

import (
	"context"
	"fmt"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain"
)

// RunPaneGitOp の操作種別。
const (
	GitOpPull  = "pull"
	GitOpPush  = "push"
	GitOpFetch = "fetch"
)

// RunPaneGitOpCommand は pane の作業ディレクトリで pull/push/fetch を実行する
// コマンドの入力 DTO。
type RunPaneGitOpCommand struct {
	WorkspaceID string
	PaneID      string
	Op          string // GitOpPull | GitOpPush | GitOpFetch
}

// RunPaneGitOpHandler は pane のディレクトリで git のリモート操作を実行するハンドラ。
type RunPaneGitOpHandler struct {
	repo domain.WorkspaceRepository
	git  port.GitService
}

// NewRunPaneGitOpHandler は依存を注入して RunPaneGitOpHandler を返す。
func NewRunPaneGitOpHandler(repo domain.WorkspaceRepository, git port.GitService) *RunPaneGitOpHandler {
	return &RunPaneGitOpHandler{repo: repo, git: git}
}

// paneDirForGit は workspace/pane ID を検証して pane の作業ディレクトリを返す。
// git 系コマンドハンドラ共通の前処理。
func paneDirForGit(ctx context.Context, repo domain.WorkspaceRepository, workspaceID, paneID, opName string) (string, error) {
	wsID, err := domain.NewWorkspaceId(workspaceID)
	if err != nil {
		return "", apperr.Validation(fmt.Errorf("%s: invalid workspace id: %w", opName, err))
	}
	w, err := repo.FindByID(ctx, wsID)
	if err != nil {
		return "", err
	}
	pID, err := domain.NewPaneId(paneID)
	if err != nil {
		return "", apperr.Validation(fmt.Errorf("%s: invalid pane id: %w", opName, err))
	}
	for _, p := range w.Panes() {
		if p.ID().Equals(pID) {
			return p.Directory().String(), nil
		}
	}
	return "", apperr.Validation(fmt.Errorf("%s: pane not found: %s", opName, paneID))
}

// Handle は指定 pane のディレクトリで Op を実行する。git の失敗は
// ValidationError として返し、UI にそのまま表示する。
func (h *RunPaneGitOpHandler) Handle(ctx context.Context, cmd RunPaneGitOpCommand) error {
	dir, err := paneDirForGit(ctx, h.repo, cmd.WorkspaceID, cmd.PaneID, "run pane git op")
	if err != nil {
		return err
	}

	var opErr error
	switch cmd.Op {
	case GitOpPull:
		opErr = h.git.Pull(dir)
	case GitOpPush:
		opErr = h.git.Push(dir)
	case GitOpFetch:
		opErr = h.git.Fetch(dir)
	default:
		return apperr.Validation(fmt.Errorf("run pane git op: unknown op: %q", cmd.Op))
	}
	if opErr != nil {
		return apperr.Validation(fmt.Errorf("run pane git op: %s: %w", cmd.Op, opErr))
	}
	return nil
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./core/application/command/ -run 'TestCheckoutPaneBranch|TestRunPaneGitOp' -v`
Expected: PASS(6 テスト)

- [ ] **Step 5: コミット**

```bash
git add core/application/command/checkout_pane_branch.go core/application/command/run_pane_git_op.go core/application/command/checkout_pane_branch_test.go core/application/command/run_pane_git_op_test.go
git commit -m "feat(app): add CheckoutPaneBranch and RunPaneGitOp commands

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: REST エンドポイント + 配線

**Files:**
- Modify: `apps/web/server.go`
- Modify: `apps/web/app.go`
- Test: `apps/web/server_gitops_test.go`(新規)

**Interfaces:**
- Consumes: Task 3/4 のハンドラ、既存 `mapErr`/`writeJSON`、テストヘルパ `doRequest`(server_test.go)・`createWorkspaceWithPane`(server_openin_test.go、同パッケージ web_test なので利用可)。
- Produces:
  - `GET  /api/workspaces/{id}/panes/{paneId}/git/branches` → 200 `{"branches":[{"name","isCurrent","isRemote"}]}`
  - `POST /api/workspaces/{id}/panes/{paneId}/git/checkout` body `{"branch":"..."}` → 204
  - `POST /api/workspaces/{id}/panes/{paneId}/git/{op}`(op=pull|push|fetch) → 204、未知 op は 400
  - `Deps.GitBranches *query.ListPaneBranchesHandler` / `Deps.GitCheckout *command.CheckoutPaneBranchHandler` / `Deps.GitOp *command.RunPaneGitOpHandler`
  - 注: Go 1.22 ServeMux はリテラル `/git/checkout` がワイルドカード `/git/{op}` より優先されるため両立できる。

- [ ] **Step 1: 失敗するテストを書く**

`apps/web/server_gitops_test.go`:

```go
package web_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ysksm/multi-terminals/apps/web"
	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/application/query"
)

// buildGitOpsDeps は git 操作エンドポイントのテストに必要な最小 Deps と fake を返す。
func buildGitOpsDeps(t *testing.T) (web.Deps, *apptest.FakeGitService) {
	t.Helper()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")
	git := apptest.NewFakeGitService()

	return web.Deps{
		Create:      command.NewCreateWorkspaceHandler(repo, idgen),
		AddPane:     command.NewAddPaneHandler(repo, idgen),
		GitBranches: query.NewListPaneBranchesHandler(repo, git),
		GitCheckout: command.NewCheckoutPaneBranchHandler(repo, git),
		GitOp:       command.NewRunPaneGitOpHandler(repo, git),
	}, git
}

func TestListPaneBranchesEndpoint(t *testing.T) {
	deps, git := buildGitOpsDeps(t)
	mux := web.NewMux(deps)
	wsID, paneID := createWorkspaceWithPane(t, mux, "/tmp/project")
	git.BranchLists["/tmp/project"] = []port.BranchInfo{
		{Name: "main", IsCurrent: true},
		{Name: "feature", IsRemote: true},
	}

	w := doRequest(mux, http.MethodGet,
		"/api/workspaces/"+wsID+"/panes/"+paneID+"/git/branches", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var res struct {
		Branches []struct {
			Name      string `json:"name"`
			IsCurrent bool   `json:"isCurrent"`
			IsRemote  bool   `json:"isRemote"`
		} `json:"branches"`
	}
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(res.Branches) != 2 || res.Branches[0].Name != "main" || !res.Branches[0].IsCurrent {
		t.Errorf("branches = %+v", res.Branches)
	}
}

func TestListPaneBranchesEndpoint_EmptyIsArray(t *testing.T) {
	deps, _ := buildGitOpsDeps(t)
	mux := web.NewMux(deps)
	wsID, paneID := createWorkspaceWithPane(t, mux, "/tmp/plain")

	w := doRequest(mux, http.MethodGet,
		"/api/workspaces/"+wsID+"/panes/"+paneID+"/git/branches", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); !json.Valid([]byte(body)) || body == "" {
		t.Fatalf("invalid body: %q", body)
	}
	var res map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(res["branches"]) == "null" {
		t.Error(`branches が null(空配列 [] を返すべき)`)
	}
}

func TestGitCheckoutEndpoint(t *testing.T) {
	deps, git := buildGitOpsDeps(t)
	mux := web.NewMux(deps)
	wsID, paneID := createWorkspaceWithPane(t, mux, "/tmp/project")

	w := doRequest(mux, http.MethodPost,
		"/api/workspaces/"+wsID+"/panes/"+paneID+"/git/checkout", `{"branch":"feature"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if len(git.Checkouts) != 1 || git.Checkouts[0].Branch != "feature" {
		t.Errorf("Checkouts = %+v", git.Checkouts)
	}
}

func TestGitOpEndpoints(t *testing.T) {
	for _, op := range []string{"pull", "push", "fetch"} {
		deps, git := buildGitOpsDeps(t)
		mux := web.NewMux(deps)
		wsID, paneID := createWorkspaceWithPane(t, mux, "/tmp/project")

		w := doRequest(mux, http.MethodPost,
			"/api/workspaces/"+wsID+"/panes/"+paneID+"/git/"+op, "")
		if w.Code != http.StatusNoContent {
			t.Fatalf("%s: expected 204, got %d: %s", op, w.Code, w.Body.String())
		}
		if len(git.GitOps) != 1 || git.GitOps[0].Op != op {
			t.Errorf("%s: GitOps = %+v", op, git.GitOps)
		}
	}
}

func TestGitOpEndpoint_UnknownOp(t *testing.T) {
	deps, _ := buildGitOpsDeps(t)
	mux := web.NewMux(deps)
	wsID, paneID := createWorkspaceWithPane(t, mux, "/tmp/project")

	w := doRequest(mux, http.MethodPost,
		"/api/workspaces/"+wsID+"/panes/"+paneID+"/git/rebase", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGitCheckoutEndpoint_GitError(t *testing.T) {
	deps, git := buildGitOpsDeps(t)
	mux := web.NewMux(deps)
	wsID, paneID := createWorkspaceWithPane(t, mux, "/tmp/project")
	git.CheckoutErr = errTest("gitcli: switch: local changes would be overwritten")

	w := doRequest(mux, http.MethodPost,
		"/api/workspaces/"+wsID+"/panes/"+paneID+"/git/checkout", `{"branch":"feature"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var res struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.Error == "" {
		t.Error("エラーメッセージが空(git の stderr を UI に届ける)")
	}
}

// errTest は文字列だけのシンプルな error。
type errTest string

func (e errTest) Error() string { return string(e) }
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./apps/web/ -run 'TestListPaneBranchesEndpoint|TestGitCheckoutEndpoint|TestGitOpEndpoint' -v`
Expected: FAIL(コンパイルエラー: `Deps.GitBranches` 未定義)

- [ ] **Step 3: 実装**

`apps/web/server.go` — `Deps` の `GetPaneGit` の直後にフィールド追加:

```go
	GitBranches     *query.ListPaneBranchesHandler
	GitCheckout     *command.CheckoutPaneBranchHandler
	GitOp           *command.RunPaneGitOpHandler
```

`NewMux` の `GET .../git` 登録行の直後にルート追加(リテラル checkout がワイルドカード {op} より優先される):

```go
	mux.HandleFunc("GET /api/workspaces/{id}/panes/{paneId}/git/branches", d.handleListPaneBranches)
	mux.HandleFunc("POST /api/workspaces/{id}/panes/{paneId}/git/checkout", d.handleGitCheckout)
	mux.HandleFunc("POST /api/workspaces/{id}/panes/{paneId}/git/{op}", d.handleGitOp)
```

`handleGetPaneGit` の直後にハンドラ追加:

```go
func (d Deps) handleListPaneBranches(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	paneID := r.PathValue("paneId")
	dtos, err := d.GitBranches.Handle(r.Context(), query.ListPaneBranchesQuery{
		WorkspaceID: id,
		PaneID:      paneID,
	})
	if err != nil {
		mapErr(w, err)
		return
	}
	if dtos == nil {
		dtos = []query.PaneBranchDTO{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"branches": dtos})
}

func (d Deps) handleGitCheckout(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	paneID := r.PathValue("paneId")
	var body struct {
		Branch string `json:"branch"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := d.GitCheckout.Handle(r.Context(), command.CheckoutPaneBranchCommand{
		WorkspaceID: id,
		PaneID:      paneID,
		Branch:      body.Branch,
	}); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (d Deps) handleGitOp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	paneID := r.PathValue("paneId")
	op := r.PathValue("op")
	if err := d.GitOp.Handle(r.Context(), command.RunPaneGitOpCommand{
		WorkspaceID: id,
		PaneID:      paneID,
		Op:          op,
	}); err != nil {
		mapErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

`apps/web/app.go` — `BuildDeps` の `GetPaneGit` 行の直後に追加:

```go
		GitBranches:     query.NewListPaneBranchesHandler(repo, git),
		GitCheckout:     command.NewCheckoutPaneBranchHandler(repo, git),
		GitOp:           command.NewRunPaneGitOpHandler(repo, git),
```

- [ ] **Step 4: テストが通ることを確認**

Run: `./scripts/dev.sh check`
Expected: `>> check: OK`(build + vet + 全テスト PASS)

- [ ] **Step 5: コミット**

```bash
git add apps/web/server.go apps/web/app.go apps/web/server_gitops_test.go
git commit -m "feat(web): add git branches/checkout/pull/push/fetch endpoints

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: フロントエンド — API クライアント・メニューキー操作ロジック・ショートカット定義

**Files:**
- Modify: `frontend/src/lib/api.js`
- Create: `frontend/src/lib/gitMenu.js`
- Test: `frontend/src/lib/gitMenu.node.test.mjs`
- Modify: `frontend/src/lib/shortcuts.js`
- Modify: `frontend/src/lib/shortcuts.node.test.mjs`

**Interfaces:**
- Consumes: Task 5 の REST API、既存 `req()`。
- Produces:
  - `api.paneGitBranches(id, paneId)` / `api.paneGitCheckout(id, paneId, branch)` / `api.paneGitOp(id, paneId, op)`
  - `menuKeyAction(key, {branchCount, selectedIndex})` → `{type:'move',index}` | `{type:'checkout'}` | `{type:'op',op}` | `{type:'close'}` | `null`
  - `paneShortcutAction` が `Ctrl+Shift+B` で `'gitmenu'` を返す

- [ ] **Step 1: 失敗するテストを書く**

`frontend/src/lib/gitMenu.node.test.mjs`:

```js
import assert from 'node:assert'
import { menuKeyAction } from './gitMenu.js'

const st = (branchCount, selectedIndex) => ({ branchCount, selectedIndex })

// 移動: 端でクランプ
assert.deepEqual(menuKeyAction('ArrowDown', st(3, 0)), { type: 'move', index: 1 }, '↓ で次へ')
assert.deepEqual(menuKeyAction('ArrowDown', st(3, 2)), { type: 'move', index: 2 }, '末尾で止まる')
assert.deepEqual(menuKeyAction('ArrowUp', st(3, 2)), { type: 'move', index: 1 }, '↑ で前へ')
assert.deepEqual(menuKeyAction('ArrowUp', st(3, 0)), { type: 'move', index: 0 }, '先頭で止まる')

// checkout: ブランチがあるときだけ
assert.deepEqual(menuKeyAction('Enter', st(3, 1)), { type: 'checkout' }, 'Enter で checkout')
assert.equal(menuKeyAction('Enter', st(0, 0)), null, 'ブランチ 0 件で Enter は無効')

// 操作キー(大文字小文字両対応)
assert.deepEqual(menuKeyAction('p', st(1, 0)), { type: 'op', op: 'pull' }, 'p → pull')
assert.deepEqual(menuKeyAction('P', st(1, 0)), { type: 'op', op: 'pull' }, 'P → pull')
assert.deepEqual(menuKeyAction('u', st(1, 0)), { type: 'op', op: 'push' }, 'u → push')
assert.deepEqual(menuKeyAction('f', st(1, 0)), { type: 'op', op: 'fetch' }, 'f → fetch')

// 閉じる・対象外
assert.deepEqual(menuKeyAction('Escape', st(1, 0)), { type: 'close' }, 'Esc で閉じる')
assert.equal(menuKeyAction('x', st(1, 0)), null, '対象外キーは null')

console.log('gitMenu: OK')
```

`frontend/src/lib/shortcuts.node.test.mjs` の `paneShortcutAction` 検証群(`ev('G')` の行の後)に追加:

```js
assert.equal(paneShortcutAction(ev('B')), 'gitmenu', 'Ctrl+Shift+B → gitmenu')
```

同ファイル末尾のヘルプ一覧検証のキーワード配列を `['最大化', 'Finder', 'VS Code', 'リモート', 'git メニュー']` に変更する。

- [ ] **Step 2: テストが失敗することを確認**

Run: `node frontend/src/lib/gitMenu.node.test.mjs; node frontend/src/lib/shortcuts.node.test.mjs`
Expected: 両方 FAIL(gitMenu.js が無い / 'gitmenu' が null)

- [ ] **Step 3: 実装**

`frontend/src/lib/gitMenu.js`:

```js
/**
 * git メニュー内のキーボード操作を action に対応付ける pure 関数。
 * @param {string} key KeyboardEvent.key
 * @param {{branchCount: number, selectedIndex: number}} state
 * @returns {{type:'move',index:number}|{type:'checkout'}|{type:'op',op:string}|{type:'close'}|null}
 */
export function menuKeyAction(key, { branchCount, selectedIndex }) {
  switch (key) {
    case 'ArrowDown':
      return { type: 'move', index: Math.min(selectedIndex + 1, Math.max(branchCount - 1, 0)) }
    case 'ArrowUp':
      return { type: 'move', index: Math.max(selectedIndex - 1, 0) }
    case 'Enter':
      return branchCount > 0 ? { type: 'checkout' } : null
    case 'Escape':
      return { type: 'close' }
    case 'p':
    case 'P':
      return { type: 'op', op: 'pull' }
    case 'u':
    case 'U':
      return { type: 'op', op: 'push' }
    case 'f':
    case 'F':
      return { type: 'op', op: 'fetch' }
    default:
      return null
  }
}
```

`frontend/src/lib/api.js` — `paneGit:` 行の直後に追加:

```js
  paneGitBranches: (id, paneId) => req('GET', `/api/workspaces/${id}/panes/${paneId}/git/branches`),
  paneGitCheckout: (id, paneId, branch) =>
    req('POST', `/api/workspaces/${id}/panes/${paneId}/git/checkout`, { branch }),
  paneGitOp: (id, paneId, op) => req('POST', `/api/workspaces/${id}/panes/${paneId}/git/${op}`),
```

`frontend/src/lib/shortcuts.js`:
- `PANE_ACTIONS` を `{ z: 'maximize', f: 'finder', v: 'vscode', g: 'github', b: 'gitmenu' }` に変更。
- JSDoc の returns 型に `'gitmenu'` を追記。
- `SHORTCUT_GROUPS` の「ペイン」グループに追加:

```js
      { keys: ['Ctrl', 'Shift', 'B'], desc: 'git メニューを開閉（リポジトリのみ。P/U/F=pull/push/fetch、↑↓+Enter=ブランチ切替）' },
```

- [ ] **Step 4: テストが通ることを確認**

Run: `node frontend/src/lib/gitMenu.node.test.mjs && node frontend/src/lib/shortcuts.node.test.mjs && node frontend/src/lib/paneNav.node.test.mjs`
Expected: `gitMenu: OK` / `shortcuts: OK` / paneNav も OK

- [ ] **Step 5: コミット**

```bash
git add frontend/src/lib/gitMenu.js frontend/src/lib/gitMenu.node.test.mjs frontend/src/lib/api.js frontend/src/lib/shortcuts.js frontend/src/lib/shortcuts.node.test.mjs
git commit -m "feat(ui): add git menu API client, key logic and Ctrl+Shift+B shortcut

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: GitMenu コンポーネント + App 統合 + 実機確認

**Files:**
- Create: `frontend/src/lib/GitMenu.svelte`
- Modify: `frontend/src/App.svelte`

**Interfaces:**
- Consumes: Task 6 の `api.paneGitBranches/paneGitCheckout/paneGitOp`、`menuKeyAction`、`paneShortcutAction`(→'gitmenu')。
- Produces: `<GitMenu workspaceId paneId onClose onChanged />`。`onChanged` は成功時(checkout/pull/push/fetch 後)に呼ばれ、App 側で `refreshGitInfo()` する。

- [ ] **Step 1: GitMenu.svelte を作成**

`frontend/src/lib/GitMenu.svelte`:

```svelte
<script>
  import { onMount } from 'svelte'
  import { api } from './api.js'
  import { menuKeyAction } from './gitMenu.js'

  let { workspaceId, paneId, onClose, onChanged } = $props()

  let branches = $state([])
  let loading = $state(true)
  let running = $state('') // '' | 'pull' | 'push' | 'fetch' | 'checkout'
  let errorMsg = $state('')
  let selectedIndex = $state(0)
  let root // メニュールート要素(フォーカス・外側クリック判定用)

  async function loadBranches() {
    loading = true
    try {
      const res = await api.paneGitBranches(workspaceId, paneId)
      branches = res?.branches || []
      const cur = branches.findIndex((b) => b.isCurrent)
      selectedIndex = cur >= 0 ? cur : 0
    } catch (e) {
      errorMsg = e.message || String(e)
    } finally {
      loading = false
    }
  }

  async function run(kind, fn) {
    if (running) return
    running = kind
    errorMsg = ''
    try {
      await fn()
      await loadBranches()
      onChanged?.()
    } catch (e) {
      errorMsg = e.message || String(e)
    } finally {
      running = ''
    }
  }

  const runOp = (op) => run(op, () => api.paneGitOp(workspaceId, paneId, op))
  const checkout = (branch) => run('checkout', () => api.paneGitCheckout(workspaceId, paneId, branch))

  function onKeydown(e) {
    const action = menuKeyAction(e.key, { branchCount: branches.length, selectedIndex })
    if (!action) return
    e.preventDefault()
    e.stopPropagation()
    if (action.type === 'close') onClose?.()
    else if (action.type === 'move') selectedIndex = action.index
    else if (action.type === 'checkout') checkout(branches[selectedIndex].name)
    else if (action.type === 'op') runOp(action.op)
  }

  // メニュー外クリックで閉じる(開いたクリック自体はバブリング完了後なので拾わない)
  function onWindowClick(e) {
    if (root && !root.contains(e.target)) onClose?.()
  }

  onMount(() => {
    loadBranches()
    root?.focus()
  })
</script>

<svelte:window onclick={onWindowClick} />

<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
<div class="git-menu" bind:this={root} tabindex="-1" onkeydown={onKeydown} role="menu">
  <div class="ops">
    <button onclick={() => runOp('pull')} disabled={!!running}>
      {running === 'pull' ? '…' : '⬇'} Pull
    </button>
    <button onclick={() => runOp('push')} disabled={!!running}>
      {running === 'push' ? '…' : '⬆'} Push
    </button>
    <button onclick={() => runOp('fetch')} disabled={!!running}>
      {running === 'fetch' ? '…' : '⟳'} Fetch
    </button>
  </div>
  <hr />
  {#if loading}
    <div class="muted">読み込み中…</div>
  {:else if branches.length === 0}
    <div class="muted">ブランチなし</div>
  {:else}
    <ul>
      {#each branches as b, i (b.name)}
        <li>
          <button
            class="branch"
            class:selected={i === selectedIndex}
            disabled={!!running || b.isCurrent}
            onclick={() => checkout(b.name)}
            onmouseenter={() => (selectedIndex = i)}
          >
            <span class="check">{b.isCurrent ? '✓' : ''}</span>
            {b.name}
            {#if b.isRemote}<span class="remote">origin</span>{/if}
            {#if running === 'checkout' && i === selectedIndex}…{/if}
          </button>
        </li>
      {/each}
    </ul>
  {/if}
  {#if errorMsg}
    <div class="error">{errorMsg}</div>
  {/if}
</div>

<style>
  .git-menu {
    position: absolute;
    top: 100%;
    left: 0;
    z-index: 30;
    min-width: 220px;
    max-width: 340px;
    padding: 6px;
    background: #24283b;
    border: 1px solid #414868;
    border-radius: 6px;
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.4);
    outline: none;
  }
  .ops {
    display: flex;
    gap: 4px;
  }
  .ops button {
    flex: 1;
    font-size: 12px;
    padding: 4px 6px;
  }
  hr {
    border: none;
    border-top: 1px solid #414868;
    margin: 6px 0;
  }
  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    max-height: 220px;
    overflow-y: auto;
  }
  .branch {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    text-align: left;
    background: none;
    border: none;
    padding: 4px 6px;
    border-radius: 4px;
    font-size: 12px;
    cursor: pointer;
  }
  .branch.selected {
    background: #364a82;
  }
  .branch:disabled {
    cursor: default;
    opacity: 0.7;
  }
  .check {
    width: 1em;
  }
  .remote {
    margin-left: auto;
    font-size: 10px;
    opacity: 0.6;
  }
  .muted {
    font-size: 12px;
    opacity: 0.6;
    padding: 4px 6px;
  }
  .error {
    margin-top: 6px;
    padding: 4px 6px;
    font-size: 11px;
    color: #f7768e;
    white-space: pre-wrap;
    word-break: break-all;
    border-top: 1px solid #414868;
  }
</style>
```

配色は App の既存ダークテーマ(`frontend/src/app.css` / App.svelte の style)を確認し、近い色変数・値があればそちらに合わせて調整してよい。

- [ ] **Step 2: App.svelte に統合**

`frontend/src/App.svelte` の変更点:

1. import に追加: `import GitMenu from './lib/GitMenu.svelte'`
2. state 追加(`paneGit` 宣言の直後):

```js
  // git メニューを開いているペイン(null = 閉)
  let gitMenuPaneId = $state(null)

  function toggleGitMenu(paneId) {
    gitMenuPaneId = gitMenuPaneId === paneId ? null : paneId
  }
```

3. `onKey` の paneAction ブロックを次のように変更(gitmenu 分岐を追加):

```js
    // Ctrl+Shift+Z/F/V/G/B: アクティブペインの最大化 / Finder / VS Code / リモート / git メニュー
    const paneAction = paneShortcutAction(e)
    if (paneAction) {
      const pane = current?.panes?.find((p) => p.id === activePaneId) || current?.panes?.[0]
      if (!pane) return
      // リポジトリでないペインでは 🌐 ボタン・git バッジ非表示と同様に無視する
      if ((paneAction === 'github' || paneAction === 'gitmenu') && !paneGit[pane.id]?.isRepo) return
      e.preventDefault()
      e.stopPropagation()
      if (paneAction === 'maximize') toggleMaximize(maximized || pane.id)
      else if (paneAction === 'gitmenu') toggleGitMenu(pane.id)
      else openPaneIn(pane.id, paneAction)
      return
    }
```

4. git バッジをクリック可能にし、メニューを直下に描画。既存のバッジ描画部:

```svelte
                {#if paneGit[cell.pane.id]?.isRepo}
                  <span
                    class="git-badge"
                    class:dirty={paneGit[cell.pane.id].dirty}
                    title={paneGit[cell.pane.id].dirty ? '未コミットの変更あり' : 'クリーン'}
                  >⎇ {paneGit[cell.pane.id].branch}{paneGit[cell.pane.id].dirty ? '*' : ''}</span>
                {/if}
```

を次に置き換える:

```svelte
                {#if paneGit[cell.pane.id]?.isRepo}
                  <span class="git-wrap">
                    <span
                      class="git-badge"
                      class:dirty={paneGit[cell.pane.id].dirty}
                      title={paneGit[cell.pane.id].dirty ? '未コミットの変更あり(クリックで git メニュー)' : 'クリックで git メニュー'}
                      role="button"
                      tabindex="0"
                      onclick={(e) => { e.stopPropagation(); toggleGitMenu(cell.pane.id) }}
                      onkeydown={(e) => { if (e.key === 'Enter') toggleGitMenu(cell.pane.id) }}
                    >⎇ {paneGit[cell.pane.id].branch}{paneGit[cell.pane.id].dirty ? '*' : ''}</span>
                    {#if gitMenuPaneId === cell.pane.id}
                      <GitMenu
                        workspaceId={current.id}
                        paneId={cell.pane.id}
                        onClose={() => (gitMenuPaneId = null)}
                        onChanged={refreshGitInfo}
                      />
                    {/if}
                  </span>
                {/if}
```

5. App.svelte の `<style>` に追加(既存 `.git-badge` 定義の近く):

```css
  .git-wrap {
    position: relative;
    display: inline-block;
  }
  .git-badge {
    cursor: pointer;
  }
```

注: バッジの `onclick` は `e.stopPropagation()` しているため、GitMenu の `svelte:window onclick`(外側クリックで閉じる)には届かず、トグルが二重に発火しない。

- [ ] **Step 3: 全テスト + ビルド確認**

Run: `./scripts/dev.sh check && (cd frontend && npm run build) && node frontend/src/lib/gitMenu.node.test.mjs && node frontend/src/lib/shortcuts.node.test.mjs`
Expected: `check: OK`、vite build 成功、node テスト OK

- [ ] **Step 4: ブラウザ実機確認**

1. バックエンド起動: `PORT=8099 MULTI_TERMINALS_DIR=$(mktemp -d) go run ./apps/web/cmd &`(ポートは PORT 環境変数。8080 は使わない。実リポジトリで試すため、起動後にワークスペース+ペイン(directory=このリポジトリのパス)を UI から作る)
2. フロント起動: `cd frontend && VITE_API_TARGET=http://localhost:8099 npm run dev`(プロキシ先は VITE_API_TARGET で指定)
3. chrome-devtools MCP でブラウザから確認:
   - git リポジトリのペインでバッジ `⎇ main` をクリック → メニューが開き、ブランチ一覧(ローカル+リモート、現在ブランチ ✓)が出る
   - 別ブランチをクリック → バッジのブランチ名が変わる
   - Fetch をクリック → エラーなく完了(またはリモート無しリポジトリでエラーメッセージが赤字表示される)
   - dirty な状態(適当なファイルを touch)で衝突するブランチへ切替 → git のエラーが赤字表示される
   - Ctrl+Shift+B でメニュー開閉、↑↓+Enter で切替、Esc で閉じる
   - メニュー外クリックで閉じる
4. 確認後、起動したプロセスを停止する。

- [ ] **Step 5: コミット**

```bash
git add frontend/src/lib/GitMenu.svelte frontend/src/App.svelte
git commit -m "feat(ui): git dropdown menu on pane badge (branch switch/pull/push/fetch)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## 完了条件

- `./scripts/dev.sh check` が OK
- `node frontend/src/lib/*.node.test.mjs` が全部 OK
- ブラウザ実機で Task 7 Step 4 の確認項目が全部通る
