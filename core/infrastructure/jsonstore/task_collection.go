package jsonstore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/ysksm/multi-terminals/core/domain/task"
)

// taskCollectionSchemaVersion はタスク管理系レコードのエンベロープ版数。
// 後方互換のない形式変更をしたときに上げる。
const taskCollectionSchemaVersion = 1

// envelope はレコードを版数付きで包む永続化 DTO。将来の形式変更を検出するために
// レコード本体とは別に version を持つ。
type envelope[T any] struct {
	Version int `json:"version"`
	Record  T   `json:"record"`
}

// Collection は「1 レコード 1 JSON ファイル」方式の task.Repository[T] 実装。
// 各レコードは dir 配下の <id>.json に保存する。書き込みは <file>.tmp に書いてから
// rename する原子的操作。全メソッドは RWMutex で並行安全。
type Collection[T task.Identified] struct {
	dir string
	mu  sync.RWMutex
}

// コンパイル時インターフェース適合確認
var (
	_ task.TaskRepository      = (*Collection[*task.Task])(nil)
	_ task.TemplateRepository  = (*Collection[*task.Template])(nil)
	_ task.BaseCloneRepository = (*Collection[*task.BaseClone])(nil)
	_ task.SetupRunRepository  = (*Collection[*task.SetupRun])(nil)
)

// newCollection は filepath.Join(baseDir, subdir) をルートとする Collection を返す。
// ディレクトリが無ければ作る。
func newCollection[T task.Identified](baseDir, subdir string) (*Collection[T], error) {
	dir := filepath.Join(baseDir, subdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating %s directory %q: %w", subdir, dir, err)
	}
	return &Collection[T]{dir: dir}, nil
}

// NewTaskRepository は tasks/ 配下にタスクを保存するリポジトリを返す。
func NewTaskRepository(baseDir string) (*Collection[*task.Task], error) {
	return newCollection[*task.Task](baseDir, "tasks")
}

// NewTemplateRepository は templates/ 配下に環境テンプレートを保存するリポジトリを返す。
func NewTemplateRepository(baseDir string) (*Collection[*task.Template], error) {
	return newCollection[*task.Template](baseDir, "templates")
}

// NewBaseCloneRepository は base_clones/ 配下にベースクローン定義を保存するリポジトリを返す。
func NewBaseCloneRepository(baseDir string) (*Collection[*task.BaseClone], error) {
	return newCollection[*task.BaseClone](baseDir, "base_clones")
}

// NewSetupRunRepository は setup_runs/ 配下にセットアップ実行を保存するリポジトリを返す。
func NewSetupRunRepository(baseDir string) (*Collection[*task.SetupRun], error) {
	return newCollection[*task.SetupRun](baseDir, "setup_runs")
}

// pathFor は id に対応するファイルパスを返す。id がパス区切りや ".." を含むなど
// 単一のパスセグメントとして安全でない場合はエラー。
func (c *Collection[T]) pathFor(id string) (string, error) {
	if id == "" || id == "." || id == ".." ||
		strings.ContainsRune(id, os.PathSeparator) ||
		strings.ContainsRune(id, '/') ||
		strings.Contains(id, "..") {
		return "", fmt.Errorf("invalid record id %q", id)
	}
	return filepath.Join(c.dir, id+".json"), nil
}

// Save はレコードをエンベロープで包んで JSON 化し、原子的に書き込む。
// 同じ ID の既存ファイルは上書きされる。
func (c *Collection[T]) Save(_ context.Context, rec T) error {
	path, err := c.pathFor(rec.GetID())
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(envelope[T]{Version: taskCollectionSchemaVersion, Record: rec}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling record %q: %w", rec.GetID(), err)
	}
	tmp := path + ".tmp"

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("writing tmp file %q: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming %q to %q: %w", tmp, path, err)
	}
	return nil
}

// decode は 1 ファイルを読んでレコードに戻す。版数が未対応ならエラー。
func (c *Collection[T]) decode(path, name string) (T, error) {
	var zero T
	data, err := os.ReadFile(path)
	if err != nil {
		return zero, err
	}
	var env envelope[T]
	if err := json.Unmarshal(data, &env); err != nil {
		return zero, fmt.Errorf("unmarshalling record file %q: %w", name, err)
	}
	if env.Version < 1 || env.Version > taskCollectionSchemaVersion {
		return zero, fmt.Errorf("unsupported schema version %d in %q: only versions up to %d are supported", env.Version, name, taskCollectionSchemaVersion)
	}
	return env.Record, nil
}

// FindByID は id のレコードを読み出す。無ければ task.ErrNotFound。
func (c *Collection[T]) FindByID(_ context.Context, id string) (T, error) {
	var zero T
	path, err := c.pathFor(id)
	if err != nil {
		return zero, err
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	rec, err := c.decode(path, filepath.Base(path))
	if err != nil {
		if os.IsNotExist(err) {
			return zero, task.ErrNotFound
		}
		return zero, err
	}
	return rec, nil
}

// List は dir 配下の全レコードを ID 昇順で返す。".json" 以外は無視する。
// 空なら空の(nil でない)スライス。1 ファイルでも壊れていれば全体をエラーにする。
func (c *Collection[T]) List(_ context.Context) ([]T, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %q: %w", c.dir, err)
	}

	out := make([]T, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		rec, err := c.decode(filepath.Join(c.dir, entry.Name()), entry.Name())
		if err != nil {
			return nil, fmt.Errorf("reading record file %q: %w", entry.Name(), err)
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GetID() < out[j].GetID() })
	return out, nil
}

// Delete は id のファイルを削除する。無ければ task.ErrNotFound。
func (c *Collection[T]) Delete(_ context.Context, id string) error {
	path, err := c.pathFor(id)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return task.ErrNotFound
		}
		return fmt.Errorf("deleting record file %q: %w", path, err)
	}
	return nil
}
