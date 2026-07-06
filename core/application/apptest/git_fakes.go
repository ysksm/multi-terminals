package apptest

import (
	"fmt"
	"sync"

	"github.com/ysksm/multi-terminals/core/application/port"
)

// コンパイル時インターフェース適合確認
var _ port.GitService = (*FakeGitService)(nil)

// CloneCall は FakeGitService.Clone の呼び出し記録。
type CloneCall struct {
	URL  string
	Dest string
}

// CheckoutCall は FakeGitService.Checkout の呼び出し記録。
type CheckoutCall struct {
	Dir    string
	Branch string
}

// GitOpCall は FakeGitService の Pull/Push/Fetch 呼び出し記録。
type GitOpCall struct {
	Dir string
	Op  string // "pull" | "push" | "fetch"
}

// FakeGitService は port.GitService のテスト用実装。
// Infos / Remotes に登録した値を返し、Clone は呼び出しを記録して Dest を返す。
type FakeGitService struct {
	mu          sync.Mutex
	Infos       map[string]port.GitInfo       // dir -> info（未登録は IsRepo=false）
	Remotes     map[string]string             // dir -> remote URL（未登録はエラー）
	Clones      []CloneCall
	CloneErr    error
	BranchLists map[string][]port.BranchInfo // dir -> branches
	Checkouts   []CheckoutCall
	CheckoutErr error
	GitOps      []GitOpCall
	OpErr       error // Pull/Push/Fetch 共通のエラー注入
}

// NewFakeGitService は空の FakeGitService を返す。
func NewFakeGitService() *FakeGitService {
	return &FakeGitService{
		Infos:       make(map[string]port.GitInfo),
		Remotes:     make(map[string]string),
		BranchLists: make(map[string][]port.BranchInfo),
	}
}

func (f *FakeGitService) Info(dir string) (port.GitInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Infos[dir], nil
}

func (f *FakeGitService) RemoteURL(dir string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	url, ok := f.Remotes[dir]
	if !ok {
		return "", fmt.Errorf("fake git: no remote for %s", dir)
	}
	return url, nil
}

func (f *FakeGitService) Clone(url, dest string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.CloneErr != nil {
		return "", f.CloneErr
	}
	f.Clones = append(f.Clones, CloneCall{URL: url, Dest: dest})
	return dest, nil
}

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

// recordOp は Pull/Push/Fetch の呼び出しを記録する。
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
