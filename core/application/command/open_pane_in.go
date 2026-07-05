package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/ysksm/multi-terminals/core/application/apperr"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/domain"
)

// OpenPaneIn の対象アプリケーション。
const (
	OpenTargetFinder = "finder"
	OpenTargetVSCode = "vscode"
	OpenTargetGitHub = "github"
)

// OpenPaneInCommand は pane の作業ディレクトリを外部アプリで開くコマンドの入力 DTO。
type OpenPaneInCommand struct {
	WorkspaceID string
	PaneID      string
	Target      string // OpenTargetFinder | OpenTargetVSCode | OpenTargetGitHub
}

// OpenPaneInHandler は pane のディレクトリを Finder / VS Code / リモート(GitHub) で開くハンドラ。
type OpenPaneInHandler struct {
	repo   domain.WorkspaceRepository
	opener port.DirectoryOpener
	git    port.GitService
}

// NewOpenPaneInHandler は依存を注入して OpenPaneInHandler を返す。
func NewOpenPaneInHandler(repo domain.WorkspaceRepository, opener port.DirectoryOpener, git port.GitService) *OpenPaneInHandler {
	return &OpenPaneInHandler{repo: repo, opener: opener, git: git}
}

// RemoteWebURL は git リモート URL をブラウザで開ける https URL に正規化する。
// 対応形式: https/http、ssh://git@host/path、scp 形式 git@host:path。
func RemoteWebURL(remote string) (string, error) {
	r := strings.TrimSpace(remote)
	switch {
	case r == "":
		return "", fmt.Errorf("empty remote url")
	case strings.HasPrefix(r, "http://"), strings.HasPrefix(r, "https://"):
		return strings.TrimSuffix(r, ".git"), nil
	case strings.HasPrefix(r, "ssh://"):
		u := strings.TrimPrefix(r, "ssh://")
		if i := strings.Index(u, "@"); i >= 0 {
			u = u[i+1:]
		}
		return "https://" + strings.TrimSuffix(u, ".git"), nil
	}
	// scp 形式: git@github.com:user/repo.git
	if i := strings.Index(r, "@"); i >= 0 {
		hostPath := r[i+1:]
		if j := strings.Index(hostPath, ":"); j >= 0 {
			return "https://" + hostPath[:j] + "/" + strings.TrimSuffix(hostPath[j+1:], ".git"), nil
		}
	}
	return "", fmt.Errorf("unsupported remote url: %q", r)
}

// Handle は指定 pane の作業ディレクトリを target のアプリケーションで開く。
func (h *OpenPaneInHandler) Handle(ctx context.Context, cmd OpenPaneInCommand) error {
	wsID, err := domain.NewWorkspaceId(cmd.WorkspaceID)
	if err != nil {
		return apperr.Validation(fmt.Errorf("open pane in: invalid workspace id: %w", err))
	}

	w, err := h.repo.FindByID(ctx, wsID)
	if err != nil {
		return err
	}

	paneID, err := domain.NewPaneId(cmd.PaneID)
	if err != nil {
		return apperr.Validation(fmt.Errorf("open pane in: invalid pane id: %w", err))
	}

	var dir string
	for _, p := range w.Panes() {
		if p.ID().Equals(paneID) {
			dir = p.Directory().String()
			break
		}
	}
	if dir == "" {
		return apperr.Validation(fmt.Errorf("open pane in: pane not found: %s", cmd.PaneID))
	}

	switch cmd.Target {
	case OpenTargetFinder:
		if err := h.opener.RevealInFileManager(dir); err != nil {
			return fmt.Errorf("open pane in: reveal in file manager: %w", err)
		}
	case OpenTargetVSCode:
		if err := h.opener.OpenInEditor(dir); err != nil {
			return fmt.Errorf("open pane in: open in editor: %w", err)
		}
	case OpenTargetGitHub:
		remote, err := h.git.RemoteURL(dir)
		if err != nil {
			return apperr.Validation(fmt.Errorf("open pane in: remote url: %w", err))
		}
		webURL, err := RemoteWebURL(remote)
		if err != nil {
			return apperr.Validation(fmt.Errorf("open pane in: %w", err))
		}
		if err := h.opener.OpenURL(webURL); err != nil {
			return fmt.Errorf("open pane in: open url: %w", err)
		}
	default:
		return apperr.Validation(fmt.Errorf("open pane in: unknown target: %q", cmd.Target))
	}

	return nil
}
