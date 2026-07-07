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

// buildGitOpsDeps は git 操作エンドポイントのテストに必要な最小 Deps と fake を返す。
func buildGitOpsDeps(t *testing.T) (web.Deps, *apptest.FakeGitService) {
	t.Helper()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "pane-1")
	git := apptest.NewFakeGitService()

	return web.Deps{
		Create:      command.NewCreateWorkspaceHandler(repo, idgen),
		AddPane:     command.NewAddPaneHandler(repo, idgen),
		GitBranches: query.NewListPaneBranchesHandler(repo, git),
		GitCheckout: command.NewCheckoutPaneBranchHandler(repo, git),
		GitOp:       command.NewRunPaneGitOpHandler(repo, git),
	}, git
}

func TestListPaneBranchesEndpoint(t *testing.T) {
	deps, git := buildGitOpsDeps(t)
	mux := web.NewMux(deps)
	wsID, paneID := createWorkspaceWithPane(t, mux, "/tmp/project")
	git.BranchLists["/tmp/project"] = []port.BranchInfo{
		{Name: "main", IsCurrent: true},
		{Name: "feature", IsRemote: true},
	}

	w := doRequest(mux, http.MethodGet,
		"/api/workspaces/"+wsID+"/panes/"+paneID+"/git/branches", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var res struct {
		Branches []struct {
			Name      string `json:"name"`
			IsCurrent bool   `json:"isCurrent"`
			IsRemote  bool   `json:"isRemote"`
		} `json:"branches"`
	}
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(res.Branches) != 2 || res.Branches[0].Name != "main" || !res.Branches[0].IsCurrent {
		t.Errorf("branches = %+v", res.Branches)
	}
}

func TestListPaneBranchesEndpoint_EmptyIsArray(t *testing.T) {
	deps, _ := buildGitOpsDeps(t)
	mux := web.NewMux(deps)
	wsID, paneID := createWorkspaceWithPane(t, mux, "/tmp/plain")

	w := doRequest(mux, http.MethodGet,
		"/api/workspaces/"+wsID+"/panes/"+paneID+"/git/branches", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); !json.Valid([]byte(body)) || body == "" {
		t.Fatalf("invalid body: %q", body)
	}
	var res map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(res["branches"]) == "null" {
		t.Error(`branches が null(空配列 [] を返すべき)`)
	}
}

func TestGitCheckoutEndpoint(t *testing.T) {
	deps, git := buildGitOpsDeps(t)
	mux := web.NewMux(deps)
	wsID, paneID := createWorkspaceWithPane(t, mux, "/tmp/project")

	w := doRequest(mux, http.MethodPost,
		"/api/workspaces/"+wsID+"/panes/"+paneID+"/git/checkout", `{"branch":"feature"}`)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
	if len(git.Checkouts) != 1 || git.Checkouts[0].Branch != "feature" {
		t.Errorf("Checkouts = %+v", git.Checkouts)
	}
}

func TestGitOpEndpoints(t *testing.T) {
	for _, op := range []string{"pull", "push", "fetch"} {
		deps, git := buildGitOpsDeps(t)
		mux := web.NewMux(deps)
		wsID, paneID := createWorkspaceWithPane(t, mux, "/tmp/project")

		w := doRequest(mux, http.MethodPost,
			"/api/workspaces/"+wsID+"/panes/"+paneID+"/git/"+op, "")
		if w.Code != http.StatusNoContent {
			t.Fatalf("%s: expected 204, got %d: %s", op, w.Code, w.Body.String())
		}
		if len(git.GitOps) != 1 || git.GitOps[0].Op != op {
			t.Errorf("%s: GitOps = %+v", op, git.GitOps)
		}
	}
}

func TestGitOpEndpoint_UnknownOp(t *testing.T) {
	deps, _ := buildGitOpsDeps(t)
	mux := web.NewMux(deps)
	wsID, paneID := createWorkspaceWithPane(t, mux, "/tmp/project")

	w := doRequest(mux, http.MethodPost,
		"/api/workspaces/"+wsID+"/panes/"+paneID+"/git/rebase", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGitCheckoutEndpoint_GitError(t *testing.T) {
	deps, git := buildGitOpsDeps(t)
	mux := web.NewMux(deps)
	wsID, paneID := createWorkspaceWithPane(t, mux, "/tmp/project")
	git.CheckoutErr = errTest("gitcli: switch: local changes would be overwritten")

	w := doRequest(mux, http.MethodPost,
		"/api/workspaces/"+wsID+"/panes/"+paneID+"/git/checkout", `{"branch":"feature"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var res struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.Error == "" {
		t.Error("エラーメッセージが空(git の stderr を UI に届ける)")
	}
}

// errTest は文字列だけのシンプルな error。
type errTest string

func (e errTest) Error() string { return string(e) }
