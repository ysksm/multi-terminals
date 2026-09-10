package port

import (
	"context"
	"io"
)

// SetupExecutor は環境セットアップで使うファイル操作とコマンド実行のポート。
// ランナー(application/taskmgmt)はこれと GitService だけでステップを実行する。
type SetupExecutor interface {
	// MkdirAll は path を親ごと作る。既にあれば何もしない。
	MkdirAll(path string) error
	// Exists は path が存在するか。
	Exists(path string) (bool, error)
	// CopyTree は src 配下を dst にコピーする。exclude はディレクトリ名
	// (例: node_modules)で、一致するディレクトリはスキップする。進捗は log へ。
	CopyTree(ctx context.Context, src, dst string, exclude []string, log io.Writer) error
	// RunCommand は dir で command をシェル経由(sh -c / cmd /C)で実行する。
	// env は "KEY=VALUE" 形式で親プロセスの環境に追加する。stdout/stderr は log へ。
	RunCommand(ctx context.Context, dir, command string, env []string, log io.Writer) error
	// RemoveAll は path を配下ごと削除する。存在しなければ何もしない。
	RemoveAll(path string) error
	// DirSize は path 配下の合計バイト数を返す。
	DirSize(path string) (int64, error)
	// ExpandPath は先頭の ~ をホームに展開し絶対パスにする。
	ExpandPath(path string) (string, error)
}
