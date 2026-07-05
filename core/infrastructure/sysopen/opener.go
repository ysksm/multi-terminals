// Package sysopen は port.DirectoryOpener の OS 実装を提供する。
// ディレクトリを Finder（ファイルマネージャ）や VS Code で開く。
package sysopen

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/ysksm/multi-terminals/core/application/port"
)

// コンパイル時インターフェース適合確認
var _ port.DirectoryOpener = (*Opener)(nil)

// Opener は実行中の OS のコマンドでディレクトリを開く DirectoryOpener 実装。
type Opener struct{}

// New は Opener を返す。
func New() *Opener {
	return &Opener{}
}

// fileManagerArgs は OS のファイルマネージャでディレクトリを開くコマンドを返す。
func fileManagerArgs(goos, dir string) []string {
	switch goos {
	case "darwin":
		return []string{"open", dir}
	case "windows":
		return []string{"explorer", dir}
	default:
		return []string{"xdg-open", dir}
	}
}

// editorArgs は VS Code でディレクトリを開くコマンドを返す。
// GUI 起動のプロセスは PATH に code が無いことがあるため、macOS は open -a を使う。
func editorArgs(goos, dir string) []string {
	switch goos {
	case "darwin":
		return []string{"open", "-a", "Visual Studio Code", dir}
	default:
		return []string{"code", dir}
	}
}

// run はコマンドを実行して終了を待つ。open/xdg-open/code はいずれも
// 対象アプリへ引き渡して即終了するため、同期実行でブロックしない。
func run(argv []string) error {
	cmd := exec.Command(argv[0], argv[1:]...)
	if err := cmd.Run(); err != nil {
		// Windows の explorer は成功時も非ゼロ終了することがあるため、
		// 起動できた（ExitError）場合は成功扱いにする。
		var exitErr *exec.ExitError
		if runtime.GOOS == "windows" && argv[0] == "explorer" && errors.As(err, &exitErr) {
			return nil
		}
		return fmt.Errorf("sysopen: %s: %w", argv[0], err)
	}
	return nil
}

// RevealInFileManager は OS のファイルマネージャでディレクトリを開く。
func (o *Opener) RevealInFileManager(dir string) error {
	return run(fileManagerArgs(runtime.GOOS, dir))
}

// OpenInEditor は VS Code でディレクトリを開く。
func (o *Opener) OpenInEditor(dir string) error {
	return run(editorArgs(runtime.GOOS, dir))
}

// urlArgs は既定のブラウザで URL を開くコマンドを返す。
func urlArgs(goos, url string) []string {
	switch goos {
	case "darwin":
		return []string{"open", url}
	case "windows":
		return []string{"rundll32", "url.dll,FileProtocolHandler", url}
	default:
		return []string{"xdg-open", url}
	}
}

// OpenURL は既定のブラウザで URL を開く。
func (o *Opener) OpenURL(url string) error {
	return run(urlArgs(runtime.GOOS, url))
}
