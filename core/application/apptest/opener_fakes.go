package apptest

import (
	"sync"

	"github.com/ysksm/multi-terminals/core/application/port"
)

// コンパイル時インターフェース適合確認
var _ port.DirectoryOpener = (*FakeDirectoryOpener)(nil)

// FakeDirectoryOpener は port.DirectoryOpener のテスト用実装。
// 呼び出されたディレクトリを記録し、Err が設定されていればそれを返す。
type FakeDirectoryOpener struct {
	mu           sync.Mutex
	RevealedDirs []string
	EditorDirs   []string
	OpenedURLs   []string
	Err          error
}

// NewFakeDirectoryOpener は空の FakeDirectoryOpener を返す。
func NewFakeDirectoryOpener() *FakeDirectoryOpener {
	return &FakeDirectoryOpener{}
}

func (f *FakeDirectoryOpener) RevealInFileManager(dir string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.RevealedDirs = append(f.RevealedDirs, dir)
	return nil
}

func (f *FakeDirectoryOpener) OpenInEditor(dir string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.EditorDirs = append(f.EditorDirs, dir)
	return nil
}

func (f *FakeDirectoryOpener) OpenURL(url string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.OpenedURLs = append(f.OpenedURLs, url)
	return nil
}
