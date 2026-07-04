package web_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ysksm/multi-terminals/apps/web"
	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/application/query"
)

// buildOpenInDeps は open-in / git / clone エンドポイントのテストに必要な
// 最小構成の Deps と各 fake を返す。
func buildOpenInDeps(t *testing.T) (web.Deps, *apptest.FakeDirectoryOpener, *apptest.FakeGitService) {
	t.Helper()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")
	opener := apptest.NewFakeDirectoryOpener()
	git := apptest.NewFakeGitService()

	return web.Deps{
		Create:     command.NewCreateWorkspaceHandler(repo, idgen),
		AddPane:    command.NewAddPaneHandler(repo, idgen),
		OpenIn:     command.NewOpenPaneInHandler(repo, opener, git),
		CloneRepo:  command.NewCloneRepositoryHandler(git),
		GetPaneGit: query.NewGetPaneGitInfoHandler(repo, git),
	}, opener, git
}

// createWorkspaceWithPane は HTTP 経由でワークスペースと pane を作成して ID を返す。
func createWorkspaceWithPane(t *testing.T, mux http.Handler, dir string) (wsID, paneID string) {
	t.Helper()
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"WS","layout":"single"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create workspace: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	w = doRequest(mux, http.MethodPost, "/api/workspaces/"+created.ID+"/panes",
		`{"directory":"`+dir+`","slot":0}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("add pane: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var pane struct {
		PaneID string `json:"paneId"`
	}
	if err := json.NewDecoder(w.Body).Decode(&pane); err != nil {
		t.Fatalf("decode pane response: %v", err)
	}
	return created.ID, pane.PaneID
}

func TestOpenPaneInFinder(t *testing.T) {
	deps, opener, _ := buildOpenInDeps(t)
	mux := web.NewMux(deps)
	wsID, paneID := createWorkspaceWithPane(t, mux, "/tmp/project")

	w := doRequest(mux, http.MethodPost,
		"/api/workspaces/"+wsID+"/panes/"+paneID+"/open-in", `{"target":"finder"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if got := opener.RevealedDirs; len(got) != 1 || got[0] != "/tmp/project" {
		t.Errorf("RevealedDirs = %v, want [/tmp/project]", got)
	}
}

func TestOpenPaneInVSCode(t *testing.T) {
	deps, opener, _ := buildOpenInDeps(t)
	mux := web.NewMux(deps)
	wsID, paneID := createWorkspaceWithPane(t, mux, "/tmp/project")

	w := doRequest(mux, http.MethodPost,
		"/api/workspaces/"+wsID+"/panes/"+paneID+"/open-in", `{"target":"vscode"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if got := opener.EditorDirs; len(got) != 1 || got[0] != "/tmp/project" {
		t.Errorf("EditorDirs = %v, want [/tmp/project]", got)
	}
}

func TestOpenPaneInUnknownTarget(t *testing.T) {
	deps, _, _ := buildOpenInDeps(t)
	mux := web.NewMux(deps)
	wsID, paneID := createWorkspaceWithPane(t, mux, "/tmp/project")

	w := doRequest(mux, http.MethodPost,
		"/api/workspaces/"+wsID+"/panes/"+paneID+"/open-in", `{"target":"sublime"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOpenPaneInWorkspaceNotFound(t *testing.T) {
	deps, _, _ := buildOpenInDeps(t)
	mux := web.NewMux(deps)

	w := doRequest(mux, http.MethodPost,
		"/api/workspaces/nonexistent/panes/pane-1/open-in", `{"target":"finder"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestOpenPaneInGitHub(t *testing.T) {
	deps, opener, git := buildOpenInDeps(t)
	mux := web.NewMux(deps)
	wsID, paneID := createWorkspaceWithPane(t, mux, "/tmp/project")
	git.Remotes["/tmp/project"] = "git@github.com:user/repo.git"

	w := doRequest(mux, http.MethodPost,
		"/api/workspaces/"+wsID+"/panes/"+paneID+"/open-in", `{"target":"github"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if got := opener.OpenedURLs; len(got) != 1 || got[0] != "https://github.com/user/repo" {
		t.Errorf("OpenedURLs = %v, want [https://github.com/user/repo]", got)
	}
}

func TestGetPaneGitInfo(t *testing.T) {
	deps, _, git := buildOpenInDeps(t)
	mux := web.NewMux(deps)
	wsID, paneID := createWorkspaceWithPane(t, mux, "/tmp/project")
	git.Infos["/tmp/project"] = port.GitInfo{IsRepo: true, Branch: "main", Dirty: true}

	w := doRequest(mux, http.MethodGet,
		"/api/workspaces/"+wsID+"/panes/"+paneID+"/git", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var dto struct {
		IsRepo bool   `json:"isRepo"`
		Branch string `json:"branch"`
		Dirty  bool   `json:"dirty"`
	}
	if err := json.NewDecoder(w.Body).Decode(&dto); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !dto.IsRepo || dto.Branch != "main" || !dto.Dirty {
		t.Errorf("dto = %+v", dto)
	}
}

func TestCloneRepo(t *testing.T) {
	deps, _, git := buildOpenInDeps(t)
	mux := web.NewMux(deps)

	w := doRequest(mux, http.MethodPost, "/api/repos/clone",
		`{"url":"https://github.com/user/repo.git","dest":"~/src/github/repo"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var res struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.Path != "~/src/github/repo" {
		t.Errorf("path = %q", res.Path)
	}
	if len(git.Clones) != 1 || git.Clones[0].URL != "https://github.com/user/repo.git" {
		t.Errorf("Clones = %+v", git.Clones)
	}
}

func TestCloneRepoMissingURL(t *testing.T) {
	deps, _, _ := buildOpenInDeps(t)
	mux := web.NewMux(deps)

	w := doRequest(mux, http.MethodPost, "/api/repos/clone", `{"url":"","dest":"/tmp/x"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
