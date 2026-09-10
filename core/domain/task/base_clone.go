package task

import (
	"errors"
	"strings"
	"time"
)

// BaseClone はコピー元として保持する clone。既定ブランチに置いたまま定期的に
// fetch/pull し、タスクの作業フォルダへはツリーコピーで配る。
type BaseClone struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	URL           string    `json:"url"`
	Path          string    `json:"path"`
	DefaultBranch string    `json:"defaultBranch"`
	LastFetchedAt time.Time `json:"lastFetchedAt"`
	SizeBytes     int64     `json:"sizeBytes"`
	CreatedAt     time.Time `json:"createdAt"`
}

// GetID は Repository[T] 用の識別子。
func (b *BaseClone) GetID() string { return b.ID }

// Validate は不変条件を検査する。
func (b *BaseClone) Validate() error {
	if strings.TrimSpace(b.ID) == "" {
		return errors.New("base clone id must not be empty")
	}
	if !repoNameRe.MatchString(b.Name) {
		return errors.New("base clone name must be a single path segment ([A-Za-z0-9._-])")
	}
	if strings.TrimSpace(b.URL) == "" {
		return errors.New("base clone url must not be empty")
	}
	if strings.TrimSpace(b.Path) == "" {
		return errors.New("base clone path must not be empty")
	}
	return nil
}

// MarkFetched は更新完了を記録する。
func (b *BaseClone) MarkFetched(defaultBranch string, size int64, now time.Time) {
	if defaultBranch != "" {
		b.DefaultBranch = defaultBranch
	}
	b.SizeBytes = size
	b.LastFetchedAt = now
}

// Settings はタスク環境の全体設定(単一レコード)。
type Settings struct {
	AutoFetchBeforeSetup bool `json:"autoFetchBeforeSetup"` // セットアップ直前にベースを更新
	IncludeNodeModules   bool `json:"includeNodeModules"`   // コピー時に node_modules も含める
	// LocalTask はローカルタスクの採番とステータス定義。旧データで空なら既定値で補う。
	LocalTask LocalTaskSettings `json:"localTask"`
}

// DefaultSettings は既定値。
func DefaultSettings() Settings {
	return Settings{AutoFetchBeforeSetup: true, IncludeNodeModules: true, LocalTask: DefaultLocalTaskSettings()}
}

// Normalized は旧データ(LocalTask 未設定)に既定値を補った設定を返す。
func (s Settings) Normalized() Settings {
	if s.LocalTask.Prefix == "" && len(s.LocalTask.Statuses) == 0 {
		s.LocalTask = DefaultLocalTaskSettings()
	}
	return s
}

// JiraKind は接続先の種類。
type JiraKind string

const (
	JiraCloud  JiraKind = "cloud"  // xxx.atlassian.net。メール + API トークン(Basic)
	JiraServer JiraKind = "server" // Server / Data Center。PAT(Bearer)
)

// JiraConfig は Jira 接続設定。トークンは含めない(別ファイルに 0600 で保存)。
type JiraConfig struct {
	Kind    JiraKind `json:"kind"`
	BaseURL string   `json:"baseUrl"`
	Email   string   `json:"email,omitempty"` // cloud のみ
	JQL     string   `json:"jql,omitempty"`   // 自動取り込み(手動同期時)
	// FieldIDs は名前 → customfield ID のキャッシュ(Sprint / Epic Link など)。
	FieldIDs map[string]string `json:"fieldIds,omitempty"`
}

// IsConfigured は接続に必要な項目が揃っているか(トークンは別途)。
func (c JiraConfig) IsConfigured() bool {
	if strings.TrimSpace(c.BaseURL) == "" {
		return false
	}
	switch c.Kind {
	case JiraCloud:
		return strings.TrimSpace(c.Email) != ""
	case JiraServer:
		return true
	}
	return false
}

// Validate は設定値を検査する。
func (c JiraConfig) Validate() error {
	if c.Kind != JiraCloud && c.Kind != JiraServer {
		return errors.New("jira kind must be cloud or server")
	}
	u := strings.TrimSpace(c.BaseURL)
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return errors.New("jira baseUrl must start with http:// or https://")
	}
	if c.Kind == JiraCloud && strings.TrimSpace(c.Email) == "" {
		return errors.New("jira cloud requires email")
	}
	return nil
}
