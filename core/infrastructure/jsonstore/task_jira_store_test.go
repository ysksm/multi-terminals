package jsonstore

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ysksm/multi-terminals/core/domain/task"
)

func TestJiraConfigStore_MissingFiles(t *testing.T) {
	s := NewJiraConfigStore(t.TempDir())
	cfg, hasToken, err := s.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if hasToken || cfg.Kind != "" || cfg.BaseURL != "" || cfg.Email != "" || cfg.JQL != "" || len(cfg.FieldIDs) != 0 {
		t.Errorf("expected zero config and no token, got %+v hasToken=%v", cfg, hasToken)
	}
	tok, err := s.Token(context.Background())
	if err != nil || tok != "" {
		t.Errorf("expected empty token, got %q err=%v", tok, err)
	}
}

func TestJiraConfigStore_ConfigRoundTrip(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	s := NewJiraConfigStore(base)
	want := task.JiraConfig{
		Kind:     task.JiraCloud,
		BaseURL:  "https://acme.atlassian.net",
		Email:    "me@acme.example",
		JQL:      "assignee = currentUser()",
		FieldIDs: map[string]string{"Sprint": "customfield_10020"},
	}
	if err := s.SaveConfig(ctx, want); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	cfg, hasToken, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if hasToken {
		t.Error("expected no token yet")
	}
	if cfg.Kind != want.Kind || cfg.BaseURL != want.BaseURL || cfg.Email != want.Email || cfg.JQL != want.JQL || cfg.FieldIDs["Sprint"] != "customfield_10020" {
		t.Errorf("config not preserved: %+v", cfg)
	}

	// トークンが設定 JSON に混ざらないこと
	data, err := os.ReadFile(filepath.Join(base, "jira.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveToken(ctx, "secret-token"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(base, "jira.json"))
	if string(after) != string(data) {
		t.Error("SaveToken must not touch jira.json")
	}
	if strings.Contains(string(after), "secret-token") {
		t.Error("token leaked into jira.json")
	}
}

func TestJiraConfigStore_Token(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	s := NewJiraConfigStore(base)

	if err := s.SaveToken(ctx, "abc123"); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}
	tokenPath := filepath.Join(base, "jira_token")
	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("token file missing: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("expected token file mode 0600, got %o", perm)
		}
	}
	_, hasToken, err := s.Load(ctx)
	if err != nil || !hasToken {
		t.Errorf("expected hasToken=true, got %v err=%v", hasToken, err)
	}
	tok, err := s.Token(ctx)
	if err != nil || tok != "abc123" {
		t.Errorf("expected token abc123, got %q err=%v", tok, err)
	}

	// 末尾改行は落とす
	if err := os.WriteFile(tokenPath, []byte("with-newline\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tok, _ = s.Token(ctx)
	if tok != "with-newline" {
		t.Errorf("expected trailing newline trimmed, got %q", tok)
	}

	// 空で削除
	if err := s.SaveToken(ctx, ""); err != nil {
		t.Fatalf("SaveToken(\"\"): %v", err)
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Errorf("expected token file removed, stat err=%v", err)
	}
	_, hasToken, _ = s.Load(ctx)
	if hasToken {
		t.Error("expected hasToken=false after removal")
	}
	// 無いときの再削除はエラーにしない
	if err := s.SaveToken(ctx, ""); err != nil {
		t.Errorf("SaveToken(\"\") on missing file: %v", err)
	}
}

func TestJiraConfigStore_RejectsFutureVersion(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "jira.json"), []byte(`{"version": 7, "record": {}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewJiraConfigStore(base).Load(context.Background()); err == nil {
		t.Error("expected error for version 7")
	}
}
