package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ローカルタスク(Jira 非依存)のエンドポイント。BuildDeps で実ストアを使うが、Jira 接続は
// 不要なので外部依存なしで通る。
func TestLocalTaskEndpoints(t *testing.T) {
	deps, err := BuildDeps(t.TempDir())
	if err != nil {
		t.Fatalf("BuildDeps: %v", err)
	}
	mux := NewMux(deps)

	do := func(method, path string, body any) (int, map[string]any) {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		out := map[string]any{}
		_ = json.NewDecoder(rec.Body).Decode(&out)
		return rec.Code, out
	}

	if code, out := do(http.MethodGet, "/api/tasks/next-key", nil); code != 200 || out["key"] != "T-001" {
		t.Fatalf("next-key: %d %v", code, out)
	}

	code, out := do(http.MethodPost, "/api/tasks", map[string]any{
		"summary": "手動タスク", "priority": "High", "labels": []string{"docs"}, "links": []string{"https://x"}, "statusId": "inprogress",
	})
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, out)
	}
	created := out["task"].(map[string]any)
	if created["jiraKey"] != "T-001" || created["source"] != "local" {
		t.Errorf("created: %v", created)
	}
	if jira := created["jira"].(map[string]any); jira["status"] != "In Progress" || jira["summary"] != "手動タスク" {
		t.Errorf("status resolved in DTO: %v", jira)
	}
	id := created["id"].(string)

	// PATCH でローカル項目を更新
	code, out = do(http.MethodPatch, "/api/tasks/"+id, map[string]any{"local": map[string]any{"summary": "更新後", "statusId": "done"}})
	if code != 200 || out["jira"].(map[string]any)["summary"] != "更新後" {
		t.Errorf("patch local: %d %v", code, out)
	}

	// 概要が空は 400
	if code, _ := do(http.MethodPost, "/api/tasks", map[string]any{"summary": " "}); code != http.StatusBadRequest {
		t.Errorf("empty summary: %d", code)
	}
	// 重複番号は 400
	if code, out := do(http.MethodPost, "/api/tasks", map[string]any{"summary": "dup", "key": "T-001"}); code != http.StatusBadRequest || !strings.Contains(out["error"].(string), "T-001") {
		t.Errorf("dup key: %d %v", code, out)
	}
	// Jira 未設定での紐付けは 400(設定不備)
	if code, _ := do(http.MethodPost, "/api/tasks/"+id+"/link-jira", map[string]any{"key": "PROJ-1"}); code != http.StatusBadRequest {
		t.Errorf("link-jira without config: %d", code)
	}
	// 無いタスクは 404
	if code, _ := do(http.MethodPost, "/api/tasks/nope/link-jira", map[string]any{"key": "PROJ-1"}); code != http.StatusNotFound {
		t.Errorf("link-jira unknown task: %d", code)
	}

	// 一覧にローカルタスクが載る
	req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var list []map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 1 || list[0]["source"] != "local" {
		t.Errorf("list: %v", list)
	}
}
