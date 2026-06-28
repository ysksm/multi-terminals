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
		Create:          command.NewCreateWorkspaceHandler(repo, idgen),
		Rename:          command.NewRenameWorkspaceHandler(repo),
		ChangeLayout:    command.NewChangeLayoutHandler(repo),
		Maximize:        command.NewMaximizePaneHandler(repo),
		Restore:         command.NewRestoreLayoutHandler(repo),
		SetLastActive:   command.NewSetLastActivePaneHandler(repo),
		Get:             query.NewGetWorkspaceHandler(repo),
		List:            query.NewListWorkspacesHandler(repo),
		GetLastOpened:   query.NewGetLastOpenedWorkspaceHandler(state, repo),
		AddPane:         command.NewAddPaneHandler(repo, idgen),
		RemovePane:      command.NewRemovePaneHandler(repo),
		SetDir:          command.NewSetPaneDirectoryHandler(repo),
		SetTitle:        command.NewSetPaneTitleHandler(repo),
		SetCmds:         command.NewSetPaneStartupCommandsHandler(repo),
		Open:            command.NewOpenWorkspaceHandler(repo, runner, reg, state, "/bin/sh"),
		Write:           command.NewWriteToPaneHandler(reg),
		Resize:          command.NewResizePaneHandler(reg),
		ClosePane:       command.NewClosePaneHandler(reg),
		DeleteWorkspace: command.NewDeleteWorkspaceHandler(repo, reg),
		Registry:        reg,
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

	// Get — WorkspaceDTO fields are lower-camelCase (explicit json tags on DTO struct)
	w2 := doRequest(mux, http.MethodGet, "/api/workspaces/"+id, "")
	if w2.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var dto map[string]interface{}
	if err := json.NewDecoder(w2.Body).Decode(&dto); err != nil {
		t.Fatalf("decode get resp: %v", err)
	}
	if dto["id"] != id {
		t.Fatalf("expected id %q, got %v", id, dto["id"])
	}
	if dto["name"] != "Test WS" {
		t.Fatalf("expected name 'Test WS', got %v", dto["name"])
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

	// Get and verify — field is "name" (lower-camelCase json tag)
	w3 := doRequest(mux, http.MethodGet, "/api/workspaces/"+id, "")
	var dto map[string]interface{}
	json.NewDecoder(w3.Body).Decode(&dto)
	if dto["name"] != "NewName" {
		t.Fatalf("expected renamed to 'NewName', got %v", dto["name"])
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
	// workspace field contains WorkspaceDTO with lower-camelCase fields
	ws, _ := resp["workspace"].(map[string]interface{})
	if ws == nil {
		t.Fatalf("expected workspace in response, got %v", resp)
	}
	if ws["id"] != id {
		t.Fatalf("expected workspace.id=%q, got %v", id, ws["id"])
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

// ---- Task 5: pane CRUD and runtime endpoint tests ----

func TestAddPaneReflectedInGet(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	// Create workspace with split layout (capacity 2) so we can add a pane at slot 0
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"WS","layout":"split_horizontal"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create workspace: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var cr map[string]string
	json.NewDecoder(w.Body).Decode(&cr)
	id := cr["id"]

	// Add a pane at slot 0
	addW := doRequest(mux, http.MethodPost, "/api/workspaces/"+id+"/panes",
		`{"directory":"/tmp","slot":0,"commands":[]}`)
	if addW.Code != http.StatusCreated {
		t.Fatalf("add pane: expected 201, got %d: %s", addW.Code, addW.Body.String())
	}
	var addResp map[string]string
	json.NewDecoder(addW.Body).Decode(&addResp)
	paneID := addResp["paneId"]
	if paneID == "" {
		t.Fatalf("expected non-empty paneId, got %v", addResp)
	}

	// GET workspace — pane should appear in panes list
	getW := doRequest(mux, http.MethodGet, "/api/workspaces/"+id, "")
	if getW.Code != http.StatusOK {
		t.Fatalf("get workspace: expected 200, got %d: %s", getW.Code, getW.Body.String())
	}
	var dto map[string]interface{}
	json.NewDecoder(getW.Body).Decode(&dto)
	panes, ok := dto["panes"].([]interface{})
	if !ok || len(panes) == 0 {
		t.Fatalf("expected panes in workspace DTO, got %v", dto)
	}
	found := false
	for _, p := range panes {
		pm, _ := p.(map[string]interface{})
		if pm["id"] == paneID {
			found = true
		}
	}
	if !found {
		t.Fatalf("pane %q not found in workspace panes: %v", paneID, panes)
	}
}

func TestRemovePaneReflectedInGet(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	// Create workspace
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"WS","layout":"single"}`)
	var cr map[string]string
	json.NewDecoder(w.Body).Decode(&cr)
	id := cr["id"]

	// Add a pane
	addW := doRequest(mux, http.MethodPost, "/api/workspaces/"+id+"/panes",
		`{"directory":"/tmp","slot":0,"commands":[]}`)
	var addResp map[string]string
	json.NewDecoder(addW.Body).Decode(&addResp)
	paneID := addResp["paneId"]

	// Remove the pane
	removeW := doRequest(mux, http.MethodDelete, "/api/workspaces/"+id+"/panes/"+paneID, "")
	if removeW.Code != http.StatusNoContent {
		t.Fatalf("remove pane: expected 204, got %d: %s", removeW.Code, removeW.Body.String())
	}

	// GET workspace — pane should be gone from panes list
	getW := doRequest(mux, http.MethodGet, "/api/workspaces/"+id, "")
	if getW.Code != http.StatusOK {
		t.Fatalf("get workspace: expected 200, got %d: %s", getW.Code, getW.Body.String())
	}
	var dto map[string]interface{}
	json.NewDecoder(getW.Body).Decode(&dto)
	panes, _ := dto["panes"].([]interface{})
	for _, p := range panes {
		pm, _ := p.(map[string]interface{})
		if pm["id"] == paneID {
			t.Fatalf("pane %q still present after removal", paneID)
		}
	}
}

func TestSetPaneDirectoryReflectedInGet(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	// Create workspace + add pane
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"WS","layout":"single"}`)
	var cr map[string]string
	json.NewDecoder(w.Body).Decode(&cr)
	id := cr["id"]

	addW := doRequest(mux, http.MethodPost, "/api/workspaces/"+id+"/panes",
		`{"directory":"/tmp","slot":0,"commands":[]}`)
	var addResp map[string]string
	json.NewDecoder(addW.Body).Decode(&addResp)
	paneID := addResp["paneId"]

	// Set directory
	setDirW := doRequest(mux, http.MethodPut,
		"/api/workspaces/"+id+"/panes/"+paneID+"/directory",
		`{"directory":"/var/log"}`)
	if setDirW.Code != http.StatusNoContent {
		t.Fatalf("set directory: expected 204, got %d: %s", setDirW.Code, setDirW.Body.String())
	}

	// GET workspace — directory should be updated
	getW := doRequest(mux, http.MethodGet, "/api/workspaces/"+id, "")
	var dto map[string]interface{}
	json.NewDecoder(getW.Body).Decode(&dto)
	panes, _ := dto["panes"].([]interface{})
	found := false
	for _, p := range panes {
		pm, _ := p.(map[string]interface{})
		if pm["id"] == paneID {
			found = true
			if pm["directory"] != "/var/log" {
				t.Fatalf("expected directory '/var/log', got %v", pm["directory"])
			}
		}
	}
	if !found {
		t.Fatalf("pane %q not found in workspace DTO", paneID)
	}
}

func TestSetPaneCommandsReflectedInGet(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	// Create workspace + add pane
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"WS","layout":"single"}`)
	var cr map[string]string
	json.NewDecoder(w.Body).Decode(&cr)
	id := cr["id"]

	addW := doRequest(mux, http.MethodPost, "/api/workspaces/"+id+"/panes",
		`{"directory":"/tmp","slot":0,"commands":[]}`)
	var addResp map[string]string
	json.NewDecoder(addW.Body).Decode(&addResp)
	paneID := addResp["paneId"]

	// Set commands
	setCmdsW := doRequest(mux, http.MethodPut,
		"/api/workspaces/"+id+"/panes/"+paneID+"/commands",
		`{"commands":[{"command":"echo hello","autoRun":true}]}`)
	if setCmdsW.Code != http.StatusNoContent {
		t.Fatalf("set commands: expected 204, got %d: %s", setCmdsW.Code, setCmdsW.Body.String())
	}

	// GET workspace — commands should be updated
	getW := doRequest(mux, http.MethodGet, "/api/workspaces/"+id, "")
	var dto map[string]interface{}
	json.NewDecoder(getW.Body).Decode(&dto)
	panes, _ := dto["panes"].([]interface{})
	found := false
	for _, p := range panes {
		pm, _ := p.(map[string]interface{})
		if pm["id"] == paneID {
			found = true
			cmds, _ := pm["commands"].([]interface{})
			if len(cmds) != 1 {
				t.Fatalf("expected 1 command, got %d", len(cmds))
			}
			cm, _ := cmds[0].(map[string]interface{})
			if cm["command"] != "echo hello" {
				t.Fatalf("expected command 'echo hello', got %v", cm["command"])
			}
			if ar, _ := cm["autoRun"].(bool); !ar {
				t.Fatalf("expected autoRun=true, got %v", cm["autoRun"])
			}
		}
	}
	if !found {
		t.Fatalf("pane %q not found in workspace DTO", paneID)
	}
}

func TestAddPaneInvalidBody400(t *testing.T) {
	mux := web.NewMux(buildTestDeps(t))

	// Create workspace first
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"WS","layout":"single"}`)
	var cr map[string]string
	json.NewDecoder(w.Body).Decode(&cr)
	id := cr["id"]

	// Invalid JSON body
	errW := doRequest(mux, http.MethodPost, "/api/workspaces/"+id+"/panes", "not-json")
	if errW.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", errW.Code)
	}
}

func TestRemovePaneNonexistent400(t *testing.T) {
	mux := web.NewMux(buildTestDeps(t))

	// Create workspace
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"WS","layout":"single"}`)
	var cr map[string]string
	json.NewDecoder(w.Body).Decode(&cr)
	id := cr["id"]

	// Remove non-existent pane from existing workspace — pane-not-found is a client error (400)
	removeW := doRequest(mux, http.MethodDelete, "/api/workspaces/"+id+"/panes/nonexistent-pane", "")
	if removeW.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for nonexistent pane, got %d: %s", removeW.Code, removeW.Body.String())
	}
}

func TestRemovePaneWorkspaceNonexistent404(t *testing.T) {
	mux := web.NewMux(buildTestDeps(t))

	// Remove pane from non-existent workspace
	removeW := doRequest(mux, http.MethodDelete, "/api/workspaces/nonexistent/panes/somepane", "")
	if removeW.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent workspace, got %d: %s", removeW.Code, removeW.Body.String())
	}
}

func TestOpenWorkspaceResponse(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	// Create workspace with split_horizontal layout (capacity 2) so we can add two panes
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"WS","layout":"split_horizontal"}`)
	var cr map[string]string
	json.NewDecoder(w.Body).Decode(&cr)
	id := cr["id"]

	// Add two panes with autoRun commands
	for i, slot := range []int{0, 1} {
		body, _ := json.Marshal(map[string]interface{}{
			"directory": "/tmp",
			"slot":      slot,
			"commands": []map[string]interface{}{
				{"command": "echo pane" + string(rune('A'+i)), "autoRun": true},
			},
		})
		addW := doRequest(mux, http.MethodPost, "/api/workspaces/"+id+"/panes", string(body))
		if addW.Code != http.StatusCreated {
			t.Fatalf("add pane slot %d: expected 201, got %d: %s", slot, addW.Code, addW.Body.String())
		}
	}

	// Open the workspace
	openW := doRequest(mux, http.MethodPost, "/api/workspaces/"+id+"/open", "")
	if openW.Code != http.StatusOK {
		t.Fatalf("open: expected 200, got %d: %s", openW.Code, openW.Body.String())
	}
	var openResp map[string]interface{}
	json.NewDecoder(openW.Body).Decode(&openResp)

	panes, ok := openResp["panes"].([]interface{})
	if !ok {
		t.Fatalf("expected panes array in response, got %v", openResp)
	}
	if len(panes) != 2 {
		t.Fatalf("expected 2 opened panes, got %d: %v", len(panes), panes)
	}
	for _, p := range panes {
		pm, _ := p.(map[string]interface{})
		if pm["paneId"] == "" || pm["paneId"] == nil {
			t.Fatalf("expected non-empty paneId in opened pane, got %v", pm)
		}
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

// TestCreateWorkspaceEmptyName400 verifies that an empty workspace name returns 400.
func TestCreateWorkspaceEmptyName400(t *testing.T) {
	mux := web.NewMux(buildTestDeps(t))
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"","layout":"single"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty name, got %d: %s", w.Code, w.Body.String())
	}
}

// TestSetPaneDirectoryEmptyDir400 verifies that setting an empty directory returns 400.
func TestSetPaneDirectoryEmptyDir400(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	// Create workspace + add pane
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"WS","layout":"single"}`)
	var cr map[string]string
	json.NewDecoder(w.Body).Decode(&cr)
	id := cr["id"]

	addW := doRequest(mux, http.MethodPost, "/api/workspaces/"+id+"/panes",
		`{"directory":"/tmp","slot":0,"commands":[]}`)
	var addResp map[string]string
	json.NewDecoder(addW.Body).Decode(&addResp)
	paneID := addResp["paneId"]

	// Set empty directory — should be 400
	setW := doRequest(mux, http.MethodPut,
		"/api/workspaces/"+id+"/panes/"+paneID+"/directory",
		`{"directory":""}`)
	if setW.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty directory, got %d: %s", setW.Code, setW.Body.String())
	}
}

// TestGetWorkspaceNotFound404 verifies that fetching a nonexistent workspace returns 404.
func TestGetWorkspaceNotFound404(t *testing.T) {
	mux := web.NewMux(buildTestDeps(t))
	w := doRequest(mux, http.MethodGet, "/api/workspaces/does-not-exist", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent workspace, got %d: %s", w.Code, w.Body.String())
	}
}

// TestCreateWorkspaceMalformedJSON400 verifies that a malformed JSON body returns 400.
func TestCreateWorkspaceMalformedJSON400(t *testing.T) {
	mux := web.NewMux(buildTestDeps(t))
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{not valid json`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed JSON, got %d: %s", w.Code, w.Body.String())
	}
}

// ---- Task 5: GET /api/sessions tests ----

// TestListSessionsEmpty verifies that /api/sessions returns {"paneIds":[]} when no sessions are live.
func TestListSessionsEmpty(t *testing.T) {
	mux := web.NewMux(buildTestDeps(t))
	w := doRequest(mux, http.MethodGet, "/api/sessions", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	paneIds, ok := resp["paneIds"].([]interface{})
	if !ok {
		t.Fatalf("expected paneIds array, got %v", resp)
	}
	if len(paneIds) != 0 {
		t.Fatalf("expected empty paneIds, got %v", paneIds)
	}
}

// TestListSessionsAfterOpen verifies that /api/sessions contains the pane ID after opening.
func TestListSessionsAfterOpen(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	// Create workspace and add a pane.
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"WS","layout":"single"}`)
	var cr map[string]string
	json.NewDecoder(w.Body).Decode(&cr)
	id := cr["id"]

	addW := doRequest(mux, http.MethodPost, "/api/workspaces/"+id+"/panes",
		`{"directory":"/tmp","slot":0,"commands":[]}`)
	if addW.Code != http.StatusCreated {
		t.Fatalf("add pane: expected 201, got %d: %s", addW.Code, addW.Body.String())
	}
	var addResp map[string]string
	json.NewDecoder(addW.Body).Decode(&addResp)
	paneID := addResp["paneId"]

	// Open the workspace — this starts the session.
	openW := doRequest(mux, http.MethodPost, "/api/workspaces/"+id+"/open", "")
	if openW.Code != http.StatusOK {
		t.Fatalf("open: expected 200, got %d: %s", openW.Code, openW.Body.String())
	}

	// GET /api/sessions — pane ID must appear.
	sessW := doRequest(mux, http.MethodGet, "/api/sessions", "")
	if sessW.Code != http.StatusOK {
		t.Fatalf("sessions: expected 200, got %d: %s", sessW.Code, sessW.Body.String())
	}
	var sessResp map[string]interface{}
	json.NewDecoder(sessW.Body).Decode(&sessResp)
	paneIds, ok := sessResp["paneIds"].([]interface{})
	if !ok {
		t.Fatalf("expected paneIds array, got %v", sessResp)
	}
	found := false
	for _, p := range paneIds {
		if p == paneID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected paneId %q in paneIds, got %v", paneID, paneIds)
	}
}

// ---- DELETE /api/workspaces/{id} tests ----

// TestDeleteWorkspace verifies that DELETE /api/workspaces/{id} returns 204 and
// that the workspace is gone from GET /api/workspaces.
func TestDeleteWorkspace(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	// Create a workspace
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"ToDelete","layout":"single"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create workspace: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var cr map[string]string
	json.NewDecoder(w.Body).Decode(&cr)
	id := cr["id"]

	// Delete the workspace
	delW := doRequest(mux, http.MethodDelete, "/api/workspaces/"+id, "")
	if delW.Code != http.StatusNoContent {
		t.Fatalf("delete workspace: expected 204, got %d: %s", delW.Code, delW.Body.String())
	}

	// Confirm it is gone from GET /api/workspaces
	listW := doRequest(mux, http.MethodGet, "/api/workspaces", "")
	if listW.Code != http.StatusOK {
		t.Fatalf("list workspaces: expected 200, got %d", listW.Code)
	}
	var dtos []map[string]interface{}
	json.NewDecoder(listW.Body).Decode(&dtos)
	for _, dto := range dtos {
		if dto["id"] == id {
			t.Fatalf("workspace %q still present in list after delete", id)
		}
	}
}

// TestDeleteWorkspaceNotFound verifies that deleting a nonexistent workspace returns 404.
func TestDeleteWorkspaceNotFound(t *testing.T) {
	mux := web.NewMux(buildTestDeps(t))
	w := doRequest(mux, http.MethodDelete, "/api/workspaces/nonexistent", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent workspace, got %d: %s", w.Code, w.Body.String())
	}
}

// TestDeleteWorkspaceGetAfterDelete verifies that GET on the deleted workspace returns 404.
func TestDeleteWorkspaceGetAfterDelete(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	// Create
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"Gone","layout":"single"}`)
	var cr map[string]string
	json.NewDecoder(w.Body).Decode(&cr)
	id := cr["id"]

	// Delete
	delW := doRequest(mux, http.MethodDelete, "/api/workspaces/"+id, "")
	if delW.Code != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d: %s", delW.Code, delW.Body.String())
	}

	// GET should now return 404
	getW := doRequest(mux, http.MethodGet, "/api/workspaces/"+id, "")
	if getW.Code != http.StatusNotFound {
		t.Fatalf("get after delete: expected 404, got %d: %s", getW.Code, getW.Body.String())
	}
}

// TestSetPaneTitleEndpoint verifies that PUT /api/workspaces/{id}/panes/{paneId}/title
// returns 204 and the title is reflected in subsequent GET.
func TestSetPaneTitleEndpoint(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	// Create workspace
	w := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"WS","layout":"single"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create workspace: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var cr map[string]string
	json.NewDecoder(w.Body).Decode(&cr)
	wsID := cr["id"]

	// Add a pane
	addW := doRequest(mux, http.MethodPost, "/api/workspaces/"+wsID+"/panes",
		`{"directory":"/tmp","slot":0,"commands":[]}`)
	if addW.Code != http.StatusCreated {
		t.Fatalf("add pane: expected 201, got %d: %s", addW.Code, addW.Body.String())
	}
	var addResp map[string]string
	json.NewDecoder(addW.Body).Decode(&addResp)
	paneID := addResp["paneId"]

	// PUT title
	putW := doRequest(mux, http.MethodPut,
		"/api/workspaces/"+wsID+"/panes/"+paneID+"/title",
		`{"title":"My Server"}`)
	if putW.Code != http.StatusNoContent {
		t.Fatalf("PUT title: got %d, want 204; body=%s", putW.Code, putW.Body.String())
	}

	// GET workspace — title should be reflected in the pane DTO
	getW := doRequest(mux, http.MethodGet, "/api/workspaces/"+wsID, "")
	if getW.Code != http.StatusOK {
		t.Fatalf("get workspace: expected 200, got %d: %s", getW.Code, getW.Body.String())
	}
	var dto map[string]interface{}
	if err := json.NewDecoder(getW.Body).Decode(&dto); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	panes, _ := dto["panes"].([]interface{})
	found := false
	for _, p := range panes {
		pm, _ := p.(map[string]interface{})
		if pm["id"] == paneID {
			found = true
			if pm["title"] != "My Server" {
				t.Fatalf("title not reflected: got %v", pm["title"])
			}
		}
	}
	if !found {
		t.Fatalf("pane %q not found in workspace DTO", paneID)
	}

	_ = deps
}
