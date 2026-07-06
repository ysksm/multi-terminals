package gitcli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/port"
)

// initRepo は tmp 配下に main ブランチの git リポジトリを作り、1コミット入れる。
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
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
	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

func TestInfo_NotARepo(t *testing.T) {
	s := New()
	info, err := s.Info(t.TempDir())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.IsRepo {
		t.Errorf("expected IsRepo=false, got %+v", info)
	}
}

func TestInfo_CleanRepo(t *testing.T) {
	dir := initRepo(t)
	s := New()
	info, err := s.Info(dir)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if !info.IsRepo || info.Branch != "main" || info.Dirty {
		t.Errorf("expected {IsRepo:true Branch:main Dirty:false}, got %+v", info)
	}
}

func TestInfo_DirtyRepo(t *testing.T) {
	dir := initRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New()
	info, err := s.Info(dir)
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if !info.IsRepo || !info.Dirty {
		t.Errorf("expected dirty repo, got %+v", info)
	}
}

func TestRemoteURL(t *testing.T) {
	dir := initRepo(t)
	cmd := exec.Command("git", "-C", dir, "remote", "add", "origin", "git@github.com:user/repo.git")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("remote add: %v\n%s", err, out)
	}
	s := New()
	url, err := s.RemoteURL(dir)
	if err != nil {
		t.Fatalf("RemoteURL: %v", err)
	}
	if url != "git@github.com:user/repo.git" {
		t.Errorf("RemoteURL = %q", url)
	}
}

func TestRemoteURL_NoRemote(t *testing.T) {
	dir := initRepo(t)
	s := New()
	if _, err := s.RemoteURL(dir); err == nil {
		t.Fatal("expected error for missing remote, got nil")
	}
}

func TestClone(t *testing.T) {
	src := initRepo(t)
	dest := filepath.Join(t.TempDir(), "cloned")
	s := New()
	path, err := s.Clone(src, dest)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if path != dest {
		t.Errorf("Clone path = %q, want %q", path, dest)
	}
	if _, err := os.Stat(filepath.Join(dest, "a.txt")); err != nil {
		t.Errorf("cloned file missing: %v", err)
	}
}

func TestClone_ExistingRepoReused(t *testing.T) {
	src := initRepo(t)
	dest := initRepo(t) // 既に clone 済み相当のリポジトリ
	marker := filepath.Join(dest, "local-change.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New()
	path, err := s.Clone(src, dest)
	if err != nil {
		t.Fatalf("Clone into existing repo: %v", err)
	}
	if path != dest {
		t.Errorf("Clone path = %q, want %q", path, dest)
	}
	// clone は実行されず、既存の内容が保持される
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("existing repo content lost: %v", err)
	}
}

func TestClone_ExistingRepoReused_TildeExpanded(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	repo := filepath.Join(home, "src", "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", repo, "init", "-b", "main")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	s := New()
	path, err := s.Clone("git@example.com:user/repo.git", "~/src/repo")
	if err != nil {
		t.Fatalf("Clone with tilde dest: %v", err)
	}
	if path != repo {
		t.Errorf("Clone path = %q, want %q", path, repo)
	}
}

func TestClone_DestExists(t *testing.T) {
	src := initRepo(t)
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, "x"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New()
	if _, err := s.Clone(src, dest); err == nil {
		t.Fatal("expected error for non-empty dest, got nil")
	}
}

func TestExpandTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := expandTilde("~/src/repo")
	if err != nil {
		t.Fatalf("expandTilde: %v", err)
	}
	if got != filepath.Join(home, "src", "repo") {
		t.Errorf("expandTilde = %q", got)
	}
	// チルダ無しはそのまま
	if got, _ := expandTilde("/abs/path"); got != "/abs/path" {
		t.Errorf("expandTilde(/abs/path) = %q", got)
	}
}

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
