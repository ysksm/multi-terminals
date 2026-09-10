package taskmgmt

import (
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain/task"
)

// TaskDTO は一覧・詳細で返すタスク。保存されている Task に、導出した
// 「作業中」フラグと紐付くワークスペース名を加える。
type TaskDTO struct {
	*task.Task
	// EffectiveState は表示用の状態。ready かつライブセッションありなら "working"、
	// それ以外は LocalState と同じ。
	EffectiveState string `json:"effectiveState"`
	Working        bool   `json:"working"`
	WorkspaceName  string `json:"workspaceName,omitempty"`
	// WorkspaceMissing は WorkspaceID があるのに実体が無い(削除済み)場合 true。
	WorkspaceMissing bool `json:"workspaceMissing,omitempty"`
	// WorkDirExists は作業フォルダの実在(詳細取得時のみ検査。一覧では false のまま)。
	WorkDirExists bool `json:"workDirExists"`
	// Agents は紐付くワークスペースの pane で稼働中のエージェント数(表示用)。
	// HTTP 層が agentstatus から埋める。
	Agents []AgentSummary `json:"agents,omitempty"`
}

// AgentSummary はワークスペース内のエージェント稼働数のまとめ。
type AgentSummary struct {
	Tool    string `json:"tool"`
	Blocked int    `json:"blocked"`
	Working int    `json:"working"`
	Done    int    `json:"done"`
	Idle    int    `json:"idle"`
}

// StateWorking は導出状態「作業中」。
const StateWorking = "working"

// PreviewItem は取り込みプレビューの 1 件。Env はテンプレート指定時のみ。
type PreviewItem struct {
	Issue    port.JiraIssue `json:"issue"`
	Exists   bool           `json:"exists"` // 既に取り込み済み
	Env      *task.Env      `json:"env,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
}

// ImportResult は取り込み結果。
type ImportResult struct {
	Tasks  []TaskDTO         `json:"tasks"`
	RunIDs map[string]string `json:"runIds,omitempty"` // jiraKey → setupRunId
}

// SyncResult は全件同期の結果。
type SyncResult struct {
	Updated int      `json:"updated"`
	Added   []string `json:"added"` // JQL で新規追加した番号
	Errors  []string `json:"errors,omitempty"`
}

// JiraConfigDTO は設定画面用。トークンは有無だけ返す。
type JiraConfigDTO struct {
	task.JiraConfig
	HasToken bool `json:"hasToken"`
}

// JiraTestResult は接続テストの結果。
type JiraTestResult struct {
	OK          bool   `json:"ok"`
	DisplayName string `json:"displayName,omitempty"`
	Email       string `json:"email,omitempty"`
	Error       string `json:"error,omitempty"`
}
