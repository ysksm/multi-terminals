package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ysksm/multi-terminals/apps/web"
	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/application/query"
	"github.com/ysksm/multi-terminals/core/application/session"
)

// buildTestDeps constructs Deps using in-memory fakes for fast unit tests.
func buildTestDeps(t *testing.T) web.Deps {
	t.Helper()
	repo := apptest.NewFakeRepo()
	idgen := apptest.NewFakeIDGen("ws-1", "ws-2", "ws-3", "pane-1", "pane-2", "pane-3")
	state := apptest.NewFakeAppStateStore()
	runner := apptest.NewFakeTerminalRunner()
	reg := session.NewRegistry()

	return web.Deps{
		Create:        command.NewCreateWorkspaceHandler(repo, idgen),
		Rename:        command.NewRenameWorkspaceHandler(repo),
		ChangeLayout:  command.NewChangeLayoutHandler(repo),
		Maximize:      command.NewMaximizePaneHandler(repo),
		Restore:       command.NewRestoreLayoutHandler(repo),
		SetLastActive: command.NewSetLastActivePaneHandler(repo),
		Get:           query.NewGetWorkspaceHandler(repo),
		List:          query.NewListWorkspacesHandler(repo),
		GetLastOpened: query.NewGetLastOpenedWorkspaceHandler(state, repo),
		AddPane:       command.NewAddPaneHandler(repo, idgen),
		RemovePane:    command.NewRemovePaneHandler(repo),
		SetDir:        command.NewSetPaneDirectoryHandler(repo),
		SetCmds:       command.NewSetPaneStartupCommandsHandler(repo),
		Open:          command.NewOpenWorkspaceHandler(repo, runner, reg, state, "/bin/sh"),
		Write:         command.NewWriteToPaneHandler(reg),
		Resize:        command.NewResizePaneHandler(reg),
		ClosePane:     command.NewClosePaneHandler(reg),
		Registry:      reg,
	}
}

// doRequest performs an HTTP request against the mux and returns the recorder.
func doRequest(mux http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var reqBody *strings.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	} else {
		reqBody = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reqBody)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestListWorkspacesEmpty(t *testing.T) {
	mux := web.NewMux(buildTestDeps(t))
	w := doRequest(mux, http.MethodGet, "/api/workspaces", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result []interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty list, got %v", result)
	}
}

func TestCreateWorkspace(t *testing.T) {
	mux := web.NewMux(buildTestDeps(t))

	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"MyWS","layout":"single"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: body=%s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	id, ok := resp["id"]
	if !ok || id == "" {
		t.Fatalf("expected non-empty id in response, got %v", resp)
	}
}

func TestGetWorkspaceNotFound(t *testing.T) {
	mux := web.NewMux(buildTestDeps(t))
	w := doRequest(mux, http.MethodGet, "/api/workspaces/nonexistent", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCreateThenGetWorkspace(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	// Create
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"Test WS","layout":"single"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var createResp map[string]string
	json.NewDecoder(w.Body).Decode(&createResp)
	id := createResp["id"]

	// Get — WorkspaceDTO fields are Title-cased (no json tags in the DTO struct)
	w2 := doRequest(mux, http.MethodGet, "/api/workspaces/"+id, "")
	if w2.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var dto map[string]interface{}
	if err := json.NewDecoder(w2.Body).Decode(&dto); err != nil {
		t.Fatalf("decode get resp: %v", err)
	}
	if dto["ID"] != id {
		t.Fatalf("expected ID %q, got %v", id, dto["ID"])
	}
	if dto["Name"] != "Test WS" {
		t.Fatalf("expected Name 'Test WS', got %v", dto["Name"])
	}
}

func TestListWorkspacesAfterCreate(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	// Create two workspaces
	for _, name := range []string{"Alpha", "Beta"} {
		w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"`+name+`","layout":"single"}`)
		if w.Code != http.StatusCreated {
			t.Fatalf("create %s: expected 201, got %d", name, w.Code)
		}
	}

	// List
	w := doRequest(mux, http.MethodGet, "/api/workspaces", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}
	var dtos []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&dtos); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(dtos) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(dtos))
	}
}

func TestPatchWorkspaceRename(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	// Create
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"OldName","layout":"single"}`)
	var createResp map[string]string
	json.NewDecoder(w.Body).Decode(&createResp)
	id := createResp["id"]

	// Patch (rename)
	w2 := doRequest(mux, http.MethodPatch, "/api/workspaces/"+id, `{"name":"NewName"}`)
	if w2.Code != http.StatusNoContent {
		t.Fatalf("patch: expected 204, got %d: %s", w2.Code, w2.Body.String())
	}

	// Get and verify — field is "Name" (Title-cased)
	w3 := doRequest(mux, http.MethodGet, "/api/workspaces/"+id, "")
	var dto map[string]interface{}
	json.NewDecoder(w3.Body).Decode(&dto)
	if dto["Name"] != "NewName" {
		t.Fatalf("expected renamed to 'NewName', got %v", dto["Name"])
	}
}

func TestMaximizeAndRestore(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	// Create workspace
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"WS","layout":"split_horizontal"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create workspace: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var createResp map[string]string
	json.NewDecoder(w.Body).Decode(&createResp)
	id := createResp["id"]

	// Add a pane first so we have a valid paneId to maximize
	addW := doRequest(mux, http.MethodPost, "/api/workspaces/"+id+"/panes",
		`{"directory":"/tmp","slot":0,"commands":[]}`)
	if addW.Code != http.StatusCreated {
		t.Fatalf("add pane: expected 201, got %d: %s", addW.Code, addW.Body.String())
	}
	var addResp map[string]string
	json.NewDecoder(addW.Body).Decode(&addResp)
	paneID := addResp["paneId"]

	// Maximize
	maxBody, _ := json.Marshal(map[string]string{"paneId": paneID})
	maxReq := httptest.NewRequest(http.MethodPost, "/api/workspaces/"+id+"/maximize", bytes.NewReader(maxBody))
	maxReq.Header.Set("Content-Type", "application/json")
	maxW := httptest.NewRecorder()
	mux.ServeHTTP(maxW, maxReq)
	if maxW.Code != http.StatusNoContent {
		t.Fatalf("maximize: expected 204, got %d: %s", maxW.Code, maxW.Body.String())
	}

	// Restore
	restoreReq := httptest.NewRequest(http.MethodPost, "/api/workspaces/"+id+"/restore", nil)
	restoreW := httptest.NewRecorder()
	mux.ServeHTTP(restoreW, restoreReq)
	if restoreW.Code != http.StatusNoContent {
		t.Fatalf("restore: expected 204, got %d: %s", restoreW.Code, restoreW.Body.String())
	}
}

func TestSetActivePane(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	// Create workspace + add pane
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"WS","layout":"single"}`)
	var cr map[string]string
	json.NewDecoder(w.Body).Decode(&cr)
	id := cr["id"]

	addW := doRequest(mux, http.MethodPost, "/api/workspaces/"+id+"/panes",
		`{"directory":"/tmp","slot":0,"commands":[]}`)
	var ar map[string]string
	json.NewDecoder(addW.Body).Decode(&ar)
	paneID := ar["paneId"]

	// SetLastActive
	body, _ := json.Marshal(map[string]string{"paneId": paneID})
	req2 := httptest.NewRequest(http.MethodPost, "/api/workspaces/"+id+"/active-pane", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if w2.Code != http.StatusNoContent {
		t.Fatalf("set active pane: expected 204, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestGetLastOpenedNotFound(t *testing.T) {
	mux := web.NewMux(buildTestDeps(t))
	w := doRequest(mux, http.MethodGet, "/api/last-opened", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found, _ := resp["found"].(bool)
	if found {
		t.Fatalf("expected found=false, got %v", resp)
	}
}

func TestGetLastOpenedAfterOpen(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	// Create a workspace
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"WS","layout":"single"}`)
	var cr map[string]string
	json.NewDecoder(w.Body).Decode(&cr)
	id := cr["id"]

	// Open the workspace (sets last opened)
	openW := doRequest(mux, http.MethodPost, "/api/workspaces/"+id+"/open", "")
	if openW.Code != http.StatusOK {
		t.Fatalf("open: expected 200, got %d: %s", openW.Code, openW.Body.String())
	}

	// GetLastOpened
	w2 := doRequest(mux, http.MethodGet, "/api/last-opened", "")
	if w2.Code != http.StatusOK {
		t.Fatalf("last-opened: expected 200, got %d", w2.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w2.Body).Decode(&resp)
	if found, _ := resp["found"].(bool); !found {
		t.Fatalf("expected found=true, got %v", resp)
	}
	// workspace field contains WorkspaceDTO with Title-cased fields
	ws, _ := resp["workspace"].(map[string]interface{})
	if ws == nil {
		t.Fatalf("expected workspace in response, got %v", resp)
	}
	if ws["ID"] != id {
		t.Fatalf("expected workspace.ID=%q, got %v", id, ws["ID"])
	}
}

func TestCreateWorkspaceInvalidBody(t *testing.T) {
	mux := web.NewMux(buildTestDeps(t))
	w := doRequest(mux, http.MethodPost, "/api/workspaces", "not-json")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// TestOpenWorkspace verifies the /open endpoint returns pane list.
func TestOpenWorkspace(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	// Create workspace
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"WS","layout":"single"}`)
	var cr map[string]string
	json.NewDecoder(w.Body).Decode(&cr)
	id := cr["id"]

	// Add pane
	addW := doRequest(mux, http.MethodPost, "/api/workspaces/"+id+"/panes",
		`{"directory":"/tmp","slot":0,"commands":[]}`)
	if addW.Code != http.StatusCreated {
		t.Fatalf("add pane: expected 201, got %d", addW.Code)
	}

	// Open
	openW := doRequest(mux, http.MethodPost, "/api/workspaces/"+id+"/open", "")
	if openW.Code != http.StatusOK {
		t.Fatalf("open: expected 200, got %d: %s", openW.Code, openW.Body.String())
	}
	var openResp map[string]interface{}
	json.NewDecoder(openW.Body).Decode(&openResp)
	panes, ok := openResp["panes"].([]interface{})
	if !ok || len(panes) == 0 {
		t.Fatalf("expected non-empty panes, got %v", openResp)
	}
}

func TestOpenWorkspaceNotFound(t *testing.T) {
	mux := web.NewMux(buildTestDeps(t))
	w := doRequest(mux, http.MethodPost, "/api/workspaces/nonexistent/open", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// Verify context is plumbed — cancelled context should not panic.
func TestContextPassThrough(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled immediately

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/workspaces", nil)
	w := httptest.NewRecorder()
	// Should not panic - handler gracefully uses request context
	mux.ServeHTTP(w, req)
}
