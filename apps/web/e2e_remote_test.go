package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestEndToEndRemoteExecution exercises remote execution across TWO full
// backend instances with the real key-exchange flow: instance A's
// auto-generated public key is fetched from its identity endpoint and
// authorized on instance B via B's REST API. A pane on A whose remoteHost
// points at B then opens: A answers B's challenge with its key, B spawns a
// REAL shell PTY locally, and the shell's output streams back through A to a
// browser-facing WebSocket.
func TestEndToEndRemoteExecution(t *testing.T) {
	newline := "\n"
	if runtime.GOOS == "windows" {
		t.Setenv("MULTI_TERMINALS_SHELL", "cmd.exe")
		newline = "\r\n"
	}

	// Instance B: the listening host that executes terminals locally.
	depsB, err := BuildDeps(t.TempDir())
	if err != nil {
		t.Fatalf("BuildDeps(B): %v", err)
	}
	srvB := httptest.NewServer(NewMux(depsB))
	defer srvB.Close()

	// Instance A: the connecting side the "browser" talks to.
	depsA, err := BuildDeps(t.TempDir())
	if err != nil {
		t.Fatalf("BuildDeps(A): %v", err)
	}
	srvA := httptest.NewServer(NewMux(depsA))
	defer srvA.Close()

	getJSON := func(base, path string) map[string]any {
		t.Helper()
		resp, err := http.Get(base + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			t.Fatalf("GET %s: status %d", path, resp.StatusCode)
		}
		out := map[string]any{}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out
	}
	postJSON := func(base, path string, body any) map[string]any {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			if err := json.NewEncoder(&buf).Encode(body); err != nil {
				t.Fatalf("encode: %v", err)
			}
		}
		resp, err := http.Post(base+path, "application/json", &buf)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			var b bytes.Buffer
			_, _ = b.ReadFrom(resp.Body)
			t.Fatalf("POST %s: status %d body=%s", path, resp.StatusCode, b.String())
		}
		out := map[string]any{}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return out
	}

	// 1. key exchange: A has no key until the user creates one; create it, then
	// authorize A's public key on B.
	identityA := postJSON(srvA.URL, "/api/remote/identity", nil)
	pubA, _ := identityA["publicKey"].(string)
	if pubA == "" {
		t.Fatalf("no publicKey in identity response: %v", identityA)
	}
	postJSON(srvB.URL, "/api/remote/authorized-keys", map[string]any{"key": pubA, "comment": "machine A"})
	authList := getJSON(srvB.URL, "/api/remote/authorized-keys")
	if enabled, _ := authList["enabled"].(bool); !enabled {
		t.Fatalf("B must report remote access enabled after authorizing a key: %v", authList)
	}

	// 2. on A: create a workspace with a pane that executes on B.
	ws := postJSON(srvA.URL, "/api/workspaces", map[string]any{"name": "e2e-remote", "layout": "single"})
	wsID, _ := ws["id"].(string)
	if wsID == "" {
		t.Fatalf("no workspace id in response: %v", ws)
	}
	pane := postJSON(srvA.URL, "/api/workspaces/"+wsID+"/panes", map[string]any{
		"directory":  t.TempDir(),
		"slot":       0,
		"remoteHost": srvB.URL,
		"commands":   []any{},
	})
	paneID, _ := pane["paneId"].(string)
	if paneID == "" {
		t.Fatalf("no pane id in response: %v", pane)
	}

	// 3. open the workspace on A -> A authenticates to B, B spawns the shell.
	opened := postJSON(srvA.URL, "/api/workspaces/"+wsID+"/open", nil)
	panes, _ := opened["panes"].([]any)
	if len(panes) != 1 {
		t.Fatalf("expected 1 opened pane, got %v", opened)
	}

	// Terminate the bridged session (and thus B's shell) before TempDir cleanup.
	t.Cleanup(func() {
		if s, ok := depsA.Registry.Get(paneID); ok {
			_ = s.Close()
			select {
			case <-s.Done():
			case <-time.After(5 * time.Second):
			}
		}
	})

	// 4. browser-facing WebSocket on A for the remote pane.
	wsURL := "ws" + strings.TrimPrefix(srvA.URL, "http") + "/api/panes/" + paneID + "/io"
	conn, dialResp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		status := "no-response"
		if dialResp != nil {
			var b bytes.Buffer
			_, _ = b.ReadFrom(dialResp.Body)
			status = dialResp.Status + " body=" + b.String()
		}
		t.Fatalf("ws dial: %v (resp: %s)", err, status)
	}
	defer conn.Close()

	// 5. type a command; it must execute on B's shell and stream back via A.
	const marker = "e2e_remote_marker_24680"
	input, _ := json.Marshal(map[string]any{"type": "input", "data": "echo " + marker + newline})
	if err := conn.WriteMessage(websocket.TextMessage, input); err != nil {
		t.Fatalf("ws write: %v", err)
	}

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
			if strings.Count(acc.String(), marker) >= 2 {
				return // success: B executed the command, A relayed the output
			}
		}
	}
	t.Fatalf("did not observe marker %q in remote PTY output within deadline; got:\n%s", marker, acc.String())
}

// TestEndToEndRemoteExecutionRejectedWithoutAuthorization verifies that
// opening a workspace whose pane targets a host that has NOT authorized this
// instance's key fails instead of silently starting a local terminal.
func TestEndToEndRemoteExecutionRejectedWithoutAuthorization(t *testing.T) {
	// B authorizes nothing -> its remote endpoint is disabled.
	depsB, err := BuildDeps(t.TempDir())
	if err != nil {
		t.Fatalf("BuildDeps(B): %v", err)
	}
	srvB := httptest.NewServer(NewMux(depsB))
	defer srvB.Close()

	depsA, err := BuildDeps(t.TempDir())
	if err != nil {
		t.Fatalf("BuildDeps(A): %v", err)
	}
	srvA := httptest.NewServer(NewMux(depsA))
	defer srvA.Close()

	postJSON := func(base, path string, body any) (*http.Response, map[string]any) {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		resp, err := http.Post(base+path, "application/json", &buf)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer resp.Body.Close()
		out := map[string]any{}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp, out
	}

	// A creates its key so the failure below is genuinely B's authorization
	// rejection, not A simply having no key.
	if resp, body := postJSON(srvA.URL, "/api/remote/identity", nil); resp.StatusCode >= 300 {
		t.Fatalf("create A identity: status %d (%v)", resp.StatusCode, body)
	}

	_, ws := postJSON(srvA.URL, "/api/workspaces", map[string]any{"name": "e2e-denied", "layout": "single"})
	wsID, _ := ws["id"].(string)
	_, _ = postJSON(srvA.URL, "/api/workspaces/"+wsID+"/panes", map[string]any{
		"directory":  t.TempDir(),
		"slot":       0,
		"remoteHost": srvB.URL,
		"commands":   []any{},
	})

	resp, body := postJSON(srvA.URL, "/api/workspaces/"+wsID+"/open", nil)
	if resp.StatusCode < 400 {
		t.Fatalf("open must fail when the remote host rejects the connection, got %d (%v)", resp.StatusCode, body)
	}
}
