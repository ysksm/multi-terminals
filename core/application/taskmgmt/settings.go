package taskmgmt

import (
	"context"
	"fmt"
	"strings"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/domain/task"
)

// GetSettings はタスク環境の全体設定を返す。
func (s *Service) GetSettings(ctx context.Context) (task.Settings, error) {
	st, err := s.d.Settings.Load(ctx)
	if err != nil {
		return task.Settings{}, err
	}
	return st.Normalized(), nil
}

// PutSettings は全体設定を保存する。ローカルタスクの採番位置(NextSeq)はクライアントに
// 上書きさせず、保存済みの値を保つ(手入力で先の番号を使ったときだけ進む)。
func (s *Service) PutSettings(ctx context.Context, st task.Settings) (task.Settings, error) {
	prev, err := s.d.Settings.Load(ctx)
	if err != nil {
		return task.Settings{}, fmt.Errorf("load settings: %w", err)
	}
	prev = prev.Normalized()
	st.LocalTask.NextSeq = prev.LocalTask.NextSeq
	if err := s.validateLocalTaskSettings(&st.LocalTask); err != nil {
		return task.Settings{}, err
	}
	if err := s.d.Settings.Save(ctx, st); err != nil {
		return task.Settings{}, fmt.Errorf("save settings: %w", err)
	}
	return st, nil
}

// GetJiraConfig は接続設定を返す(トークンは有無だけ)。
func (s *Service) GetJiraConfig(ctx context.Context) (JiraConfigDTO, error) {
	cfg, has, err := s.d.JiraStore.Load(ctx)
	if err != nil {
		return JiraConfigDTO{}, fmt.Errorf("load jira config: %w", err)
	}
	cfg.FieldIDs = nil // 内部キャッシュは返さない
	return JiraConfigDTO{JiraConfig: cfg, HasToken: has}, nil
}

// JiraConfigInput は接続設定の保存入力。Token が空なら既存トークンを維持する。
// ClearToken=true で削除。
type JiraConfigInput struct {
	Kind       string `json:"kind"`
	BaseURL    string `json:"baseUrl"`
	Email      string `json:"email"`
	JQL        string `json:"jql"`
	Token      string `json:"token"`
	ClearToken bool   `json:"clearToken"`
}

// PutJiraConfig は接続設定(と任意でトークン)を保存する。
func (s *Service) PutJiraConfig(ctx context.Context, in JiraConfigInput) (JiraConfigDTO, error) {
	cfg := task.JiraConfig{
		Kind:    task.JiraKind(strings.TrimSpace(in.Kind)),
		BaseURL: strings.TrimRight(strings.TrimSpace(in.BaseURL), "/"),
		Email:   strings.TrimSpace(in.Email),
		JQL:     strings.TrimSpace(in.JQL),
	}
	if err := cfg.Validate(); err != nil {
		return JiraConfigDTO{}, apperr.Validation(err)
	}
	// サイト(baseUrl)が変わればフィールド ID のキャッシュは無効。同じなら引き継ぐ。
	if prev, _, err := s.d.JiraStore.Load(ctx); err == nil && prev.BaseURL == cfg.BaseURL {
		cfg.FieldIDs = prev.FieldIDs
	}
	if err := s.d.JiraStore.SaveConfig(ctx, cfg); err != nil {
		return JiraConfigDTO{}, fmt.Errorf("save jira config: %w", err)
	}
	if in.ClearToken {
		if err := s.d.JiraStore.SaveToken(ctx, ""); err != nil {
			return JiraConfigDTO{}, fmt.Errorf("clear jira token: %w", err)
		}
	} else if tok := strings.TrimSpace(in.Token); tok != "" {
		if err := s.d.JiraStore.SaveToken(ctx, tok); err != nil {
			return JiraConfigDTO{}, fmt.Errorf("save jira token: %w", err)
		}
	}
	s.invalidateJira()
	return s.GetJiraConfig(ctx)
}

// TestJira は現在の設定で接続テストを行う。失敗は結果に載せて 200 で返す(UI 表示用)。
func (s *Service) TestJira(ctx context.Context) JiraTestResult {
	cli, err := s.jira(ctx)
	if err != nil {
		return JiraTestResult{Error: err.Error()}
	}
	u, err := cli.TestConnection(ctx)
	if err != nil {
		return JiraTestResult{Error: err.Error()}
	}
	return JiraTestResult{OK: true, DisplayName: u.DisplayName, Email: u.Email}
}
