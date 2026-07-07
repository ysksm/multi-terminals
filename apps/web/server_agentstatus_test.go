package web_test

import (
	"bufio"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ysksm/multi-terminals/apps/web"
	"github.com/ysksm/multi-terminals/core/application/agentstatus"
)

// newAgentStatusWatcher は claude が 1 ペインで active な固定スナップショットを
// 返す Watcher を作る(ティッカーは実質無効化し PollNow で駆動)。
func newAgentStatusWatcher() *agentstatus.Watcher {
	procs := []agentstatus.Proc{
		{PID: 10, PPID: 1, Command: "-zsh"},
		{PID: 11, PPID: 10, Command: "claude"},
	}
	sessions := []agentstatus.SessionInfo{{PaneID: "pane-1", Pid: 10, LastOutput: time.Now().Add(-5 * time.Second)}}
	return agentstatus.NewWatcher(
		func() []agentstatus.SessionInfo { return sessions },
		func() ([]agentstatus.Proc, error) { return procs, nil },
		time.Hour,
	)
}

func TestAgentStatusSnapshotEndpoint(t *testing.T) {
	deps := buildTestDeps(t)
	deps.AgentStatus = newAgentStatusWatcher()
	deps.AgentStatus.PollNow()
	mux := web.NewMux(deps)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/api/agent-status", nil))
	if rr.Code != 200 {
		t.Fatalf("GET /api/agent-status: status %d", rr.Code)
	}
	var body struct {
		Panes map[string][]agentstatus.PaneAgent `json:"panes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Panes["pane-1"]) != 1 || body.Panes["pane-1"][0].Tool != "claude" {
		t.Fatalf("panes = %v, want pane-1 → claude", body.Panes)
	}
}

func TestAgentStatusDisabledWithoutWatcher(t *testing.T) {
	mux := web.NewMux(buildTestDeps(t))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/api/agent-status", nil))
	if rr.Code != 404 {
		t.Fatalf("GET /api/agent-status without watcher: status %d, want 404", rr.Code)
	}
}

func TestAgentStatusStreamSendsInitialSnapshot(t *testing.T) {
	deps := buildTestDeps(t)
	deps.AgentStatus = newAgentStatusWatcher()
	deps.AgentStatus.PollNow()
	srv := httptest.NewServer(web.NewMux(deps))
	defer srv.Close()

	res, err := srv.Client().Get(srv.URL + "/api/agent-status/stream")
	if err != nil {
		t.Fatalf("GET stream: %v", err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q", ct)
	}
	sc := bufio.NewScanner(res.Body)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "data: ") {
			if !strings.Contains(line, `"claude"`) {
				t.Fatalf("first event = %q, want claude snapshot", line)
			}
			return
		}
	}
	t.Fatal("no data event received")
}
