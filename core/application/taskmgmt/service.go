// Package taskmgmt はタスク管理(Jira 取り込み・環境テンプレート・ベースクローン・
// セットアップ実行)のアプリケーションサービス。
//
// Workspace 側の command/query パッケージが 1 ユースケース 1 ハンドラなのに対し、
// こちらはユースケースが多く依存も共通なので 1 つの Service にまとめ、
// ファイルをユースケース群ごとに分ける(tasks.go / templates.go / base_clones.go /
// settings.go / runner.go)。ステップ列の生成(plan.go)は pure でテストしやすくしてある。
package taskmgmt

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain"
	"github.com/ysksm/multi-terminals/core/domain/task"
)

// Deps は Service の依存。全てポート/リポジトリで、テストでは apptest の偽物を刺す。
type Deps struct {
	Tasks      task.TaskRepository
	Templates  task.TemplateRepository
	BaseClones task.BaseCloneRepository
	Runs       task.SetupRunRepository
	Settings   task.SettingsStore
	JiraStore  task.JiraConfigStore
	// JiraFactory は設定とトークンからクライアントを作る(設定変更のたびに作り直す)。
	JiraFactory port.JiraClientFactory
	Git         port.GitService
	Exec        port.SetupExecutor
	Workspaces  domain.WorkspaceRepository
	IDGen       port.IDGenerator
	// LivePaneIDs はライブセッションを持つ pane の ID 一覧(作業中の導出に使う)。
	LivePaneIDs func() []string
	// DeleteWorkspace はワークスペース削除(セッションのクローズ込み)。
	// command.DeleteWorkspaceHandler をそのまま渡す想定。nil なら削除しない。
	DeleteWorkspace func(ctx context.Context, workspaceID string) error
	// BaseDir はベースクローンの既定の置き場(MULTI_TERMINALS_DIR)。
	BaseDir string
	// Now は現在時刻(テストで差し替える)。nil なら time.Now。
	Now func() time.Time
}

// Service はタスク管理のアプリケーションサービス。
type Service struct {
	d      Deps
	runner *runner

	mu       sync.Mutex
	jiraCli  port.JiraClient // 設定から作ったクライアントのキャッシュ
	jiraOnce bool
}

// New は依存を検証して Service を返す。
func New(d Deps) (*Service, error) {
	switch {
	case d.Tasks == nil, d.Templates == nil, d.BaseClones == nil, d.Runs == nil,
		d.Settings == nil, d.JiraStore == nil, d.JiraFactory == nil, d.Git == nil,
		d.Exec == nil, d.Workspaces == nil, d.IDGen == nil:
		return nil, errors.New("taskmgmt: all repositories and ports are required")
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.LivePaneIDs == nil {
		d.LivePaneIDs = func() []string { return nil }
	}
	s := &Service{d: d}
	s.runner = newRunner(s)
	return s, nil
}

func (s *Service) now() time.Time { return s.d.Now() }

// invalidateJira は設定変更後にクライアントを作り直させる。
func (s *Service) invalidateJira() {
	s.mu.Lock()
	s.jiraCli = nil
	s.jiraOnce = false
	s.mu.Unlock()
}

// jira は現在の設定からクライアントを返す。未設定なら port.ErrJiraNotConfigured。
func (s *Service) jira(ctx context.Context) (port.JiraClient, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.jiraOnce && s.jiraCli != nil {
		return s.jiraCli, nil
	}
	cfg, hasToken, err := s.d.JiraStore.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("load jira config: %w", err)
	}
	if !cfg.IsConfigured() || !hasToken {
		return nil, apperr.Validation(port.ErrJiraNotConfigured)
	}
	token, err := s.d.JiraStore.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("load jira token: %w", err)
	}
	s.jiraCli = s.d.JiraFactory(cfg, token)
	s.jiraOnce = true
	return s.jiraCli, nil
}

// absPath は ~ を展開した絶対パスを返す。
func (s *Service) absPath(p string) (string, error) {
	return s.d.Exec.ExpandPath(p)
}

// repoDir はタスクの作業フォルダ配下のリポジトリパス。
func repoDir(workDir, repoName string) string {
	return filepath.Join(workDir, repoName)
}

// wrapJiraErr は Jira クライアントのエラーを HTTP 400 相当にする(設定・入力起因のため)。
// 通信断もユーザーが対処するものなので同じ扱い。
func wrapJiraErr(op string, err error) error {
	if err == nil {
		return nil
	}
	return apperr.Validation(fmt.Errorf("%s: %w", op, err))
}
