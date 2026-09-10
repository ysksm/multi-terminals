// Package setupexec は環境セットアップで使うファイル操作とコマンド実行の
// port.SetupExecutor 実装を提供する。
package setupexec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ysksm/multi-terminals/core/application/port"
)

// コンパイル時インターフェース適合確認
var _ port.SetupExecutor = (*Executor)(nil)

// Executor は OS のファイルシステムとシェルを使う SetupExecutor 実装。
type Executor struct{}

// New は Executor を返す。
func New() *Executor {
	return &Executor{}
}

// MkdirAll は path を親ごと作る。既にあれば何もしない。
func (e *Executor) MkdirAll(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("setupexec: mkdir %s: %w", path, err)
	}
	return nil
}

// Exists は path が存在するか。
func (e *Executor) Exists(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("setupexec: stat %s: %w", path, err)
	}
	return true, nil
}

// progressEvery はコピー進捗をログに出す間隔(ファイル数)。
const progressEvery = 500

// CopyTree は src 配下を dst にコピーする。exclude に含まれる名前のディレクトリは
// 配下ごとスキップする(node_modules など)。シンボリックリンクはリンクのまま再作成し、
// 通常ファイルはモードを保って内容をコピーする。dst が既にあって空でなければ
// エラーにする(無関係なフォルダへ混ぜないため)。
func (e *Executor) CopyTree(ctx context.Context, src, dst string, exclude []string, log io.Writer) error {
	if log == nil {
		log = io.Discard
	}
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("setupexec: copy: src %s: %w", src, err)
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("setupexec: copy: src %s is not a directory", src)
	}
	if entries, err := os.ReadDir(dst); err == nil {
		if len(entries) > 0 {
			return fmt.Errorf("setupexec: copy: dst %s already exists and is not empty", dst)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("setupexec: copy: dst %s: %w", dst, err)
	}

	excluded := make(map[string]bool, len(exclude))
	for _, name := range exclude {
		if name != "" {
			excluded[name] = true
		}
	}

	files, dirs := 0, 0
	err = filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			if path != src && excluded[d.Name()] {
				return filepath.SkipDir
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if err := os.MkdirAll(target, info.Mode().Perm()|0o700); err != nil {
				return err
			}
			dirs++
			return nil
		}

		if d.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.Symlink(link, target); err != nil {
				return err
			}
			files++
		} else if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			if err := copyFile(path, target, info.Mode().Perm()); err != nil {
				return err
			}
			files++
		} else {
			// ソケット・デバイス等は対象外(スキップ)
			return nil
		}
		if files%progressEvery == 0 {
			fmt.Fprintf(log, "... %d files copied\n", files)
		}
		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("setupexec: copy %s: %w", src, ctx.Err())
		}
		return fmt.Errorf("setupexec: copy %s: %w", src, err)
	}
	fmt.Fprintf(log, "copied %d files, %d dirs\n", files, dirs)
	return nil
}

// copyFile は通常ファイルを mode 付きでコピーする。
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

// RunCommand は dir で command をシェル経由で実行する。stdout/stderr は log へ。
// ctx のキャンセルでプロセス(unix ではプロセスグループごと)を殺す。
func (e *Executor) RunCommand(ctx context.Context, dir, command string, env []string, log io.Writer) error {
	if log == nil {
		log = io.Discard
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	}
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), "GIT_TERMINAL_PROMPT=0"), env...)
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.Stdin = nil
	setProcessGroup(cmd)

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("setupexec: command %q cancelled: %w", command, ctx.Err())
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("setupexec: command %q exited with code %d", command, exitErr.ExitCode())
		}
		return fmt.Errorf("setupexec: command %q: %w", command, err)
	}
	return nil
}

// RemoveAll は path を配下ごと削除する。空・ルート・ホーム・浅すぎるパスは
// 安全のため拒否する。
func (e *Executor) RemoveAll(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("setupexec: remove: path is empty")
	}
	clean := filepath.Clean(path)
	if clean == string(filepath.Separator) || clean == "." || filepath.VolumeName(clean)+string(filepath.Separator) == clean {
		return fmt.Errorf("setupexec: remove: refusing to delete %q", path)
	}
	if home, err := os.UserHomeDir(); err == nil && filepath.Clean(home) == clean {
		return fmt.Errorf("setupexec: remove: refusing to delete home directory %q", path)
	}
	rel := strings.TrimPrefix(clean, filepath.VolumeName(clean))
	parts := strings.Split(strings.Trim(rel, string(filepath.Separator)), string(filepath.Separator))
	if len(parts) < 2 || parts[0] == "" {
		return fmt.Errorf("setupexec: remove: path %q is too shallow", path)
	}
	if err := os.RemoveAll(clean); err != nil {
		return fmt.Errorf("setupexec: remove %s: %w", path, err)
	}
	return nil
}

// DirSize は path 配下の通常ファイルの合計バイト数を返す。読めない項目は飛ばす。
func (e *Executor) DirSize(path string) (int64, error) {
	if _, err := os.Stat(path); err != nil {
		return 0, fmt.Errorf("setupexec: size %s: %w", path, err)
	}
	var total int64
	_ = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total, nil
}

// ExpandPath は先頭の ~ をホームに展開し、絶対パスにする。
func (e *Executor) ExpandPath(path string) (string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", errors.New("setupexec: path is empty")
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("setupexec: home dir: %w", err)
		}
		p = filepath.Join(home, p[1:])
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("setupexec: abs %s: %w", path, err)
	}
	return abs, nil
}
