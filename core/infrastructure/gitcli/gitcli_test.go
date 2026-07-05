package gitcli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
