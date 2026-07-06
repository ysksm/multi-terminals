// Package gitcli は git コマンドラインを実行する port.GitService 実装を提供する。
package gitcli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ysksm/multi-terminals/core/application/port"
)

// コンパイル時インターフェース適合確認
var _ port.GitService = (*Service)(nil)

// Service は git CLI を使う GitService 実装。
type Service struct{}

// New は Service を返す。
func New() *Service {
	return &Service{}
}

// git は dir で git サブコマンドを実行し、trim した標準出力を返す。
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(errBuf.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// Info はディレクトリの git 状態を返す。リポジトリでない場合は IsRepo=false。
func (s *Service) Info(dir string) (port.GitInfo, error) {
	if _, err := git(dir, "rev-parse", "--is-inside-work-tree"); err != nil {
		return port.GitInfo{}, nil
	}

	// unborn branch（初回コミット前）でも動く symbolic-ref を優先し、
	// detached HEAD では短縮コミット表記にフォールバックする。
	branch, err := git(dir, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil || branch == "" {
		if sha, shaErr := git(dir, "rev-parse", "--short", "HEAD"); shaErr == nil {
			branch = "(" + sha + ")"
		} else {
			branch = "(no branch)"
		}
	}

	status, err := git(dir, "status", "--porcelain")
	if err != nil {
		return port.GitInfo{}, fmt.Errorf("gitcli: status: %w", err)
	}

	return port.GitInfo{IsRepo: true, Branch: branch, Dirty: status != ""}, nil
}

// RemoteURL は origin リモートの URL を返す。
func (s *Service) RemoteURL(dir string) (string, error) {
	url, err := git(dir, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("gitcli: remote get-url: %w", err)
	}
	return url, nil
}

// expandTilde は先頭の "~/" をホームディレクトリに展開する。
func expandTilde(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("gitcli: home dir: %w", err)
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
	}
	return path, nil
}

// Clone は url を dest（~/ 展開あり）に clone して clone 先パスを返す。
// dest が既に git リポジトリの場合は clone せず、そのパスをそのまま返す
// （clone 済みフォルダの再利用。リモート URL の一致検証は行わない）。
func (s *Service) Clone(url, dest string) (string, error) {
	expanded, err := expandTilde(dest)
	if err != nil {
		return "", err
	}
	if _, err := git(expanded, "rev-parse", "--is-inside-work-tree"); err == nil {
		return expanded, nil
	}
	cmd := exec.Command("git", "clone", "--", url, expanded)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gitcli: clone: %w: %s", err, strings.TrimSpace(errBuf.String()))
	}
	return expanded, nil
}

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
		// スラッシュを含まない ref はリモートの symbolic HEAD なので除外
		// (%(refname:short) は origin/HEAD を裸のリモート名 "origin" で出力する)
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
