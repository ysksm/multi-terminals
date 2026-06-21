package web_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ysksm/multi-terminals/apps/web"
)

// TestBuildDepsSmoke verifies that BuildDeps succeeds with a temp directory
// and that the resulting mux serves GET /api/workspaces with 200 + empty array.
func TestBuildDepsSmoke(t *testing.T) {
	baseDir := t.TempDir()
	deps, err := web.BuildDeps(baseDir)
	if err != nil {
		t.Fatalf("BuildDeps: %v", err)
	}

	mux := web.NewMux(deps)

	w := doRequest(mux, http.MethodGet, "/api/workspaces", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result []interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty list, got %v", result)
	}
}

// TestBuildDepsCreateGetRoundTrip verifies that BuildDeps wires real jsonstore
// and that create → get round-trips through the filesystem.
func TestBuildDepsCreateGetRoundTrip(t *testing.T) {
	baseDir := t.TempDir()
	deps, err := web.BuildDeps(baseDir)
	if err != nil {
		t.Fatalf("BuildDeps: %v", err)
	}

	mux := web.NewMux(deps)

	// Create a workspace
	createW := doRequest(mux, http.MethodPost, "/api/workspaces", `{"name":"IntegrationWS","layout":"single"}`)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", createW.Code, createW.Body.String())
	}
	var createResp map[string]string
	if err := json.NewDecoder(createW.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create resp: %v", err)
	}
	id := createResp["id"]
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	// Get the workspace back — jsonstore must have persisted it
	getW := doRequest(mux, http.MethodGet, "/api/workspaces/"+id, "")
	if getW.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d: %s", getW.Code, getW.Body.String())
	}
	var dto map[string]interface{}
	if err := json.NewDecoder(getW.Body).Decode(&dto); err != nil {
		t.Fatalf("decode get resp: %v", err)
	}
	if dto["id"] != id {
		t.Fatalf("expected id %q, got %v", id, dto["id"])
	}
	if dto["name"] != "IntegrationWS" {
		t.Fatalf("expected name 'IntegrationWS', got %v", dto["name"])
	}

	// List — should return one workspace
	listW := doRequest(mux, http.MethodGet, "/api/workspaces", "")
	if listW.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d: %s", listW.Code, listW.Body.String())
	}
	var list []map[string]interface{}
	if err := json.NewDecoder(listW.Body).Decode(&list); err != nil {
		t.Fatalf("decode list resp: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 workspace in list, got %d: %v", len(list), list)
	}
}
