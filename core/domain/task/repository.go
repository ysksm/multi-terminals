package task

import "context"

// Identified は Repository[T] が扱うレコード(ID を持つ)。
type Identified interface {
	GetID() string
}

// Repository は「1 レコード 1 JSON ファイル」方式の集約永続化ポート。
// Task / Template / BaseClone / SetupRun で共通。存在しない ID は ErrNotFound。
type Repository[T Identified] interface {
	Save(ctx context.Context, rec T) error
	FindByID(ctx context.Context, id string) (T, error)
	List(ctx context.Context) ([]T, error)
	Delete(ctx context.Context, id string) error
}

// TaskRepository / TemplateRepository / BaseCloneRepository / SetupRunRepository は
// 型ごとの別名。実装は jsonstore が提供する。
type (
	TaskRepository      = Repository[*Task]
	TemplateRepository  = Repository[*Template]
	BaseCloneRepository = Repository[*BaseClone]
	SetupRunRepository  = Repository[*SetupRun]
)

// SettingsStore は Settings(単一レコード)の永続化ポート。未保存なら DefaultSettings。
type SettingsStore interface {
	Load(ctx context.Context) (Settings, error)
	Save(ctx context.Context, s Settings) error
}

// JiraConfigStore は Jira 接続設定とトークンの永続化ポート。トークンは設定 JSON と
// 別ファイル(0600)に置く。Load の hasToken はトークンの有無だけを返し、値は
// Token() でしか読めない(HTTP レスポンスに載せないため)。
type JiraConfigStore interface {
	Load(ctx context.Context) (cfg JiraConfig, hasToken bool, err error)
	SaveConfig(ctx context.Context, cfg JiraConfig) error
	// SaveToken は空文字列でトークンを削除する。
	SaveToken(ctx context.Context, token string) error
	Token(ctx context.Context) (string, error)
}
