// Package gitcli は git コマンドラインを実行する port.GitService 実装を提供する。
package gitcli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
