package jsonstore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ysksm/multi-terminals/core/domain/task"
)

// SettingsStore はタスク環境の全体設定(単一レコード)を task_settings.json に保存する
// task.SettingsStore 実装。並行安全。
type SettingsStore struct {
	path string
	mu   sync.RWMutex
}

// コンパイル時インターフェース適合確認
var _ task.SettingsStore = (*SettingsStore)(nil)

// NewSettingsStore は filepath.Join(baseDir, "task_settings.json") を読み書きする
// SettingsStore を返す。
func NewSettingsStore(baseDir string) *SettingsStore {
	return &SettingsStore{path: filepath.Join(baseDir, "task_settings.json")}
}

// Load は設定を読み出す。ファイルが無ければ task.DefaultSettings() を返す。
func (s *SettingsStore) Load(_ context.Context) (task.Settings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return task.DefaultSettings(), nil
		}
		return task.Settings{}, fmt.Errorf("reading task settings file %q: %w", s.path, err)
	}
	var env envelope[task.Settings]
	if err := json.Unmarshal(data, &env); err != nil {
		return task.Settings{}, fmt.Errorf("unmarshalling task settings file %q: %w", s.path, err)
	}
	if env.Version < 1 || env.Version > taskCollectionSchemaVersion {
		return task.Settings{}, fmt.Errorf("unsupported schema version %d in %q", env.Version, s.path)
	}
	return env.Record, nil
}

// Save は設定を原子的に書き込む。親ディレクトリは無ければ作る。
func (s *SettingsStore) Save(_ context.Context, settings task.Settings) error {
	data, err := json.MarshalIndent(envelope[task.Settings]{Version: taskCollectionSchemaVersion, Record: settings}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling task settings: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return writeAtomic(s.path, data, 0o644)
}

// writeAtomic は path.tmp に書いてから rename する。親ディレクトリは無ければ作る。
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating directory for %q: %w", path, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("writing tmp file %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming %q to %q: %w", tmp, path, err)
	}
	return nil
}
