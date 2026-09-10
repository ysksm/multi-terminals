package jsonstore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ysksm/multi-terminals/core/domain/task"
)

// JiraConfigStore は Jira 接続設定を jira.json に、トークンを jira_token(0600)に
// 分けて保存する task.JiraConfigStore 実装。トークンは設定 JSON に載せない。
type JiraConfigStore struct {
	configPath string
	tokenPath  string
	mu         sync.RWMutex
}

// コンパイル時インターフェース適合確認
var _ task.JiraConfigStore = (*JiraConfigStore)(nil)

// NewJiraConfigStore は baseDir 配下の jira.json / jira_token を読み書きする
// JiraConfigStore を返す。
func NewJiraConfigStore(baseDir string) *JiraConfigStore {
	return &JiraConfigStore{
		configPath: filepath.Join(baseDir, "jira.json"),
		tokenPath:  filepath.Join(baseDir, "jira_token"),
	}
}

// Load は設定とトークンの有無を返す。どちらのファイルも無ければゼロ値・false・nil。
func (s *JiraConfigStore) Load(_ context.Context) (task.JiraConfig, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var cfg task.JiraConfig
	data, err := os.ReadFile(s.configPath)
	if err != nil && !os.IsNotExist(err) {
		return task.JiraConfig{}, false, fmt.Errorf("reading jira config file %q: %w", s.configPath, err)
	}
	if err == nil {
		var env envelope[task.JiraConfig]
		if err := json.Unmarshal(data, &env); err != nil {
			return task.JiraConfig{}, false, fmt.Errorf("unmarshalling jira config file %q: %w", s.configPath, err)
		}
		if env.Version < 1 || env.Version > taskCollectionSchemaVersion {
			return task.JiraConfig{}, false, fmt.Errorf("unsupported schema version %d in %q", env.Version, s.configPath)
		}
		cfg = env.Record
	}

	hasToken, err := s.tokenExists()
	if err != nil {
		return task.JiraConfig{}, false, err
	}
	return cfg, hasToken, nil
}

// tokenExists はトークンファイルが存在し空でないか。呼び出し側でロックする。
func (s *JiraConfigStore) tokenExists() (bool, error) {
	tok, err := s.readToken()
	if err != nil {
		return false, err
	}
	return tok != "", nil
}

// readToken はトークンファイルを読む。無ければ空文字列。末尾の改行は落とす。
func (s *JiraConfigStore) readToken() (string, error) {
	data, err := os.ReadFile(s.tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("reading jira token file %q: %w", s.tokenPath, err)
	}
	return strings.TrimRight(string(data), "\r\n"), nil
}

// SaveConfig は設定を原子的に書き込む。トークンには触れない。
func (s *JiraConfigStore) SaveConfig(_ context.Context, cfg task.JiraConfig) error {
	data, err := json.MarshalIndent(envelope[task.JiraConfig]{Version: taskCollectionSchemaVersion, Record: cfg}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling jira config: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return writeAtomic(s.configPath, data, 0o644)
}

// SaveToken はトークンを 0600 で書き込む。空文字列ならファイルを削除する。
func (s *JiraConfigStore) SaveToken(_ context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if token == "" {
		if err := os.Remove(s.tokenPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing jira token file %q: %w", s.tokenPath, err)
		}
		return nil
	}
	if err := writeAtomic(s.tokenPath, []byte(token), 0o600); err != nil {
		return err
	}
	// WriteFile の perm は umask の影響を受けるため念のため明示的に絞る。
	if err := os.Chmod(s.tokenPath, 0o600); err != nil {
		return fmt.Errorf("chmod jira token file %q: %w", s.tokenPath, err)
	}
	return nil
}

// Token はトークンを返す。無ければ空文字列と nil。
func (s *JiraConfigStore) Token(_ context.Context) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readToken()
}
