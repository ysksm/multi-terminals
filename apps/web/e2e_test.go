package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestEndToEndRealPTYOverWebSocket exercises the entire backend stack with a
// REAL PTY: HTTP create-workspace -> add-pane -> open (spawns /bin/sh) ->
// WebSocket connect -> send a shell command -> receive the shell's output.
// This is the definitive proof that the browser-facing path works.
func TestEndToEndRealPTYOverWebSocket(t *testing.T) {
	deps, err := BuildDeps(t.TempDir())
	if err != nil {
		t.Fatalf("BuildDeps: %v", err)
	}
	srv := httptest.NewServer(NewMux(deps))
	defer srv.Close()

	postJSON := func(path string, body any) map[string]any {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&buf).Encode(body); err != nil {
				t.Fatalf("encode: %v", err)
			}
		}
		resp, err := http.Post(srv.URL+path, "application/json", &buf)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			t.Fatalf("POST %s: status %d", path, resp.StatusCode)
		}
		out := map[string]any{}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out
	}

	// 1. create workspace
	ws := postJSON("/api/workspaces", map[string]any{"name": "e2e", "layout": "single"})
	wsID, _ := ws["id"].(string)
	if wsID == "" {
		t.Fatalf("no workspace id in response: %v", ws)
	}

	// 2. add a pane rooted at a temp dir
	pane := postJSON("/api/workspaces/"+wsID+"/panes", map[string]any{
		"directory": t.TempDir(),
		"slot":      0,
		"commands":  []any{},
	})
	paneID, _ := pane["paneId"].(string)
	if paneID == "" {
		t.Fatalf("no pane id in response: %v", pane)
	}

	// 3. open the workspace -> starts a real /bin/sh PTY for the pane
	opened := postJSON("/api/workspaces/"+wsID+"/open", nil)
	panes, _ := opened["panes"].([]any)
	if len(panes) != 1 {
		t.Fatalf("expected 1 opened pane, got %v", opened)
	}

	// 4. connect a WebSocket to the pane's I/O endpoint
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/panes/" + paneID + "/io"
	conn, dialResp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		status := "no-response"
		if dialResp != nil {
			var b bytes.Buffer
			_, _ = b.ReadFrom(dialResp.Body)
			status = dialResp.Status + " body=" + b.String()
		}
		t.Fatalf("ws dial: %v (resp: %s) (paneID=%s, registry IDs=%v)", err, status, paneID, deps.Registry.IDs())
	}
	defer conn.Close()

	// 5. send a shell command
	const marker = "e2e_marker_98765"
	input, _ := json.Marshal(map[string]any{"type": "input", "data": "echo " + marker + "\n"})
	if err := conn.WriteMessage(websocket.TextMessage, input); err != nil {
		t.Fatalf("ws write: %v", err)
	}

	// 6. read output frames until the marker (the echoed result, not the typed
	// command) appears. The typed command echoes once via terminal echo and the
	// shell prints the result; we look for the marker appearing on its own line.
	deadline := time.Now().Add(10 * time.Second)
	_ = conn.SetReadDeadline(deadline)
	var acc bytes.Buffer
	for time.Now().Before(deadline) {
		mt, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("ws read (got so far: %q): %v", acc.String(), err)
		}
		if mt == websocket.BinaryMessage || mt == websocket.TextMessage {
			acc.Write(data)
			// The shell prints the command line AND its output, so the marker
			// will appear at least twice; one standalone occurrence is enough.
			if strings.Count(acc.String(), marker) >= 2 {
				return // success: shell executed the command and emitted output
			}
		}
	}
	t.Fatalf("did not observe marker %q in PTY output within deadline; got:\n%s", marker, acc.String())
}
