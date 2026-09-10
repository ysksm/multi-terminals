package apptest

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain/task"
)

// コンパイル時インターフェース適合確認
var (
	_ task.TaskRepository    = (*MemRepo[*task.Task])(nil)
	_ task.SettingsStore     = (*FakeSettingsStore)(nil)
	_ task.JiraConfigStore   = (*FakeJiraStore)(nil)
	_ port.JiraClient        = (*FakeJiraClient)(nil)
	_ port.SetupExecutor     = (*FakeExecutor)(nil)
	_ port.JiraClientFactory = (&FakeJiraClient{}).Factory
)

// MemRepo は task.Repository[T] のインメモリ実装。Save はポインタをそのまま保持する。
type MemRepo[T task.Identified] struct {
	mu    sync.RWMutex
	store map[string]T
	// SaveCount は Save の呼び出し回数。
	SaveCount int
}

// NewMemRepo は空の MemRepo を返す。
func NewMemRepo[T task.Identified]() *MemRepo[T] {
	return &MemRepo[T]{store: make(map[string]T)}
}

func (r *MemRepo[T]) Save(_ context.Context, rec T) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.store[rec.GetID()] = rec
	r.SaveCount++
	return nil
}

func (r *MemRepo[T]) FindByID(_ context.Context, id string) (T, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.store[id]
	if !ok {
		var zero T
		return zero, task.ErrNotFound
	}
	return rec, nil
}

// List は ID 昇順で返す。
func (r *MemRepo[T]) List(_ context.Context) ([]T, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]T, 0, len(r.store))
	for _, rec := range r.store {
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GetID() < out[j].GetID() })
	return out, nil
}

func (r *MemRepo[T]) Delete(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.store[id]; !ok {
		return task.ErrNotFound
	}
	delete(r.store, id)
	return nil
}

// FakeSettingsStore は task.SettingsStore のインメモリ実装。未保存なら既定値。
type FakeSettingsStore struct {
	mu    sync.Mutex
	set   bool
	value task.Settings
}

// NewFakeSettingsStore は既定値を返す FakeSettingsStore を返す。
func NewFakeSettingsStore() *FakeSettingsStore { return &FakeSettingsStore{} }

func (s *FakeSettingsStore) Load(_ context.Context) (task.Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.set {
		return task.DefaultSettings(), nil
	}
	return s.value, nil
}

func (s *FakeSettingsStore) Save(_ context.Context, v task.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.value, s.set = v, true
	return nil
}

// FakeJiraStore は task.JiraConfigStore のインメモリ実装。Cfg / Tok を直接設定できる。
type FakeJiraStore struct {
	mu  sync.Mutex
	Cfg task.JiraConfig
	Tok string
}

// NewFakeJiraStore は接続済み(cloud)の設定を持つ FakeJiraStore を返す。
func NewFakeJiraStore() *FakeJiraStore {
	return &FakeJiraStore{
		Cfg: task.JiraConfig{Kind: task.JiraCloud, BaseURL: "https://example.atlassian.net", Email: "me@example.com"},
		Tok: "token",
	}
}

func (s *FakeJiraStore) Load(_ context.Context) (task.JiraConfig, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Cfg, s.Tok != "", nil
}

func (s *FakeJiraStore) SaveConfig(_ context.Context, cfg task.JiraConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Cfg = cfg
	return nil
}

func (s *FakeJiraStore) SaveToken(_ context.Context, tok string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Tok = tok
	return nil
}

func (s *FakeJiraStore) Token(_ context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Tok, nil
}

// FakeJiraClient は port.JiraClient のテスト用実装。Issues に登録した issue を返す。
type FakeJiraClient struct {
	mu            sync.Mutex
	Issues        map[string]port.JiraIssue
	SearchResults []port.JiraIssue
	Err           error // 全メソッド共通のエラー注入
	User          port.JiraUser
	FetchCalls    [][]string
	SearchCalls   []string
	// FactoryCalls は Factory 経由で生成された回数。
	FactoryCalls int
}

// NewFakeJiraClient は空の FakeJiraClient を返す。
func NewFakeJiraClient() *FakeJiraClient {
	return &FakeJiraClient{Issues: make(map[string]port.JiraIssue)}
}

// Factory は自身を返す port.JiraClientFactory。
func (c *FakeJiraClient) Factory(_ task.JiraConfig, _ string) port.JiraClient {
	c.mu.Lock()
	c.FactoryCalls++
	c.mu.Unlock()
	return c
}

func (c *FakeJiraClient) TestConnection(_ context.Context) (port.JiraUser, error) {
	if c.Err != nil {
		return port.JiraUser{}, c.Err
	}
	return c.User, nil
}

func (c *FakeJiraClient) FetchIssues(_ context.Context, keys []string) ([]port.JiraIssue, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.FetchCalls = append(c.FetchCalls, append([]string(nil), keys...))
	if c.Err != nil {
		return nil, c.Err
	}
	var out []port.JiraIssue
	var missing []string
	for _, k := range keys {
		is, ok := c.Issues[k]
		if !ok {
			missing = append(missing, k)
			continue
		}
		out = append(out, is)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%w: %s", port.ErrJiraNotFound, strings.Join(missing, ", "))
	}
	return out, nil
}

func (c *FakeJiraClient) Search(_ context.Context, jql string, max int) ([]port.JiraIssue, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.SearchCalls = append(c.SearchCalls, jql)
	if c.Err != nil {
		return nil, c.Err
	}
	out := c.SearchResults
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return append([]port.JiraIssue(nil), out...), nil
}

// CopyCall は FakeExecutor.CopyTree の呼び出し記録。
type CopyCall struct {
	Src     string
	Dst     string
	Exclude []string
}

// CommandCall は FakeExecutor.RunCommand の呼び出し記録。
type CommandCall struct {
	Dir     string
	Command string
	Env     []string
}

// FakeExecutor は port.SetupExecutor のテスト用実装。ファイルは触らず呼び出しを記録する。
type FakeExecutor struct {
	mu          sync.Mutex
	Mkdirs      []string
	Copies      []CopyCall
	Commands    []CommandCall
	Removed     []string
	ExistsPaths map[string]bool
	Sizes       map[string]int64
	// CommandErr はコマンドごとのエラー注入フック。
	CommandErr func(command string) error
	// CommandOutput は RunCommand が log へ書く内容。
	CommandOutput string
	// Block が非 nil なら RunCommand は Block が閉じるか ctx が終わるまで待つ。
	Block chan struct{}
	// MkdirErr / CopyErr / RemoveErr はエラー注入。
	MkdirErr  error
	CopyErr   error
	RemoveErr error
}

// NewFakeExecutor は空の FakeExecutor を返す。
func NewFakeExecutor() *FakeExecutor {
	return &FakeExecutor{ExistsPaths: make(map[string]bool), Sizes: make(map[string]int64)}
}

func (e *FakeExecutor) MkdirAll(path string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.MkdirErr != nil {
		return e.MkdirErr
	}
	e.Mkdirs = append(e.Mkdirs, path)
	e.ExistsPaths[path] = true
	return nil
}

func (e *FakeExecutor) Exists(path string) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ExistsPaths[path], nil
}

func (e *FakeExecutor) CopyTree(_ context.Context, src, dst string, exclude []string, log io.Writer) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.CopyErr != nil {
		return e.CopyErr
	}
	e.Copies = append(e.Copies, CopyCall{Src: src, Dst: dst, Exclude: append([]string(nil), exclude...)})
	e.ExistsPaths[dst] = true
	fmt.Fprintf(log, "copied %s -> %s\n", src, dst)
	return nil
}

func (e *FakeExecutor) RunCommand(ctx context.Context, dir, command string, env []string, log io.Writer) error {
	e.mu.Lock()
	e.Commands = append(e.Commands, CommandCall{Dir: dir, Command: command, Env: append([]string(nil), env...)})
	block := e.Block
	hook := e.CommandErr
	out := e.CommandOutput
	e.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return fmt.Errorf("fake exec: %w", ctx.Err())
		}
	}
	if out != "" {
		_, _ = io.WriteString(log, out)
	}
	if hook != nil {
		return hook(command)
	}
	return nil
}

func (e *FakeExecutor) RemoveAll(path string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.RemoveErr != nil {
		return e.RemoveErr
	}
	e.Removed = append(e.Removed, path)
	delete(e.ExistsPaths, path)
	return nil
}

func (e *FakeExecutor) DirSize(path string) (int64, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.Sizes[path], nil
}

// ExpandPath は先頭の "~" を /home/test に置き換える。それ以外はそのまま。
func (e *FakeExecutor) ExpandPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("fake exec: empty path")
	}
	if path == "~" {
		return "/home/test", nil
	}
	if strings.HasPrefix(path, "~/") {
		return "/home/test" + path[1:], nil
	}
	return path, nil
}

// CommandList は記録したコマンド文字列だけを返す(アサート用)。
func (e *FakeExecutor) CommandList() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, len(e.Commands))
	for i, c := range e.Commands {
		out[i] = c.Command
	}
	return out
}
