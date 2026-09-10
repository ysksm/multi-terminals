package port

import (
	"context"
	"errors"

	"github.com/ysksm/multi-terminals/core/domain/task"
)

// Jira クライアントのエラー種別。HTTP 層は Validation 相当(400)として返し、
// UI はメッセージをそのまま出す。
var (
	ErrJiraNotConfigured = errors.New("jira is not configured")
	ErrJiraAuth          = errors.New("jira authentication failed")
	ErrJiraNotFound      = errors.New("jira issue not found")
	ErrJiraUnavailable   = errors.New("jira is unreachable")
)

// JiraIssue は取得した issue の必要項目だけの写し。Description はプレーンテキスト
// (Cloud の ADF はテキストノードを抽出、Server は wiki 記法のまま)。
type JiraIssue struct {
	Key            string `json:"key"`
	Summary        string `json:"summary"`
	Status         string `json:"status"`
	StatusCategory string `json:"statusCategory"` // new | indeterminate | done
	Assignee       string `json:"assignee"`
	Priority       string `json:"priority"`
	Epic           string `json:"epic"`
	Sprint         string `json:"sprint"`
	Description    string `json:"description"`
	URL            string `json:"url"`
}

// JiraUser は接続テストで得る自分の情報。
type JiraUser struct {
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
}

// JiraClient は Jira REST API の読み取り専用ポート。書き込みは行わない。
type JiraClient interface {
	// TestConnection は /myself を叩いて認証を確認する。
	TestConnection(ctx context.Context) (JiraUser, error)
	// FetchIssues は指定番号の issue を取得する。見つからない番号があれば
	// ErrJiraNotFound を返す(見つかった分だけ返すことはしない)。
	FetchIssues(ctx context.Context, keys []string) ([]JiraIssue, error)
	// Search は JQL で検索する。max は上限件数。
	Search(ctx context.Context, jql string, max int) ([]JiraIssue, error)
}

// JiraClientFactory は設定とトークンからクライアントを作る。設定変更のたびに
// 作り直せるようファクトリで受ける。
type JiraClientFactory func(cfg task.JiraConfig, token string) JiraClient
