package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ysksm/multi-terminals/apps/web"
	"github.com/ysksm/multi-terminals/core/application/apptest"
)

// dialWS connects a gorilla WebSocket client to the given URL.
func dialWS(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("WebSocket dial failed: %v", err)
	}
	return conn
}

// TestWebSocketInputEcho verifies that sending an input message over WebSocket
// causes the fake session to echo it back as binary output.
func TestWebSocketInputEcho(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	// Register a fake session in the registry so the handler can find it.
	paneID := "test-pane-echo"
	fakeSess := apptest.NewFakeTerminalSession(paneID)
	deps.Registry.Add(paneID, fakeSess)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Connect WebSocket.
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/panes/" + paneID + "/io"
	conn := dialWS(t, wsURL)
	t.Cleanup(func() { conn.Close() })

	// Send an input message.
	msg := `{"type":"input","data":"hi"}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	// The fake session echoes Write data on its Output() channel.
	// The output pump goroutine in the handler forwards it as a BinaryMessage.
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	mt, recv, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if mt != websocket.BinaryMessage {
		t.Fatalf("expected BinaryMessage, got %d", mt)
	}
	if string(recv) != "hi" {
		t.Fatalf("expected echoed 'hi', got %q", string(recv))
	}
}

// TestWebSocketResizeUpdatesSession verifies that sending a resize message
// updates the fake session's LastCols and LastRows fields.
func TestWebSocketResizeUpdatesSession(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	paneID := "test-pane-resize"
	fakeSess := apptest.NewFakeTerminalSession(paneID)
	deps.Registry.Add(paneID, fakeSess)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/panes/" + paneID + "/io"
	conn := dialWS(t, wsURL)
	t.Cleanup(func() { conn.Close() })

	// Send resize message.
	resizeMsg := `{"type":"resize","cols":120,"rows":40}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(resizeMsg)); err != nil {
		t.Fatalf("WriteMessage resize: %v", err)
	}

	// Poll until the resize is applied or deadline passes.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		cols, rows := fakeSess.LastSize()
		if cols == 120 && rows == 40 {
			return // success
		}
		time.Sleep(10 * time.Millisecond)
	}
	cols, rows := fakeSess.LastSize()
	t.Fatalf("resize not applied: LastCols=%d LastRows=%d", cols, rows)
}

// TestWebSocketPaneNotFound verifies that connecting to a non-existent pane
// returns HTTP 404 (upgrade rejected).
func TestWebSocketPaneNotFound(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/panes/no-such-pane/io"
	dialer := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	_, resp, err := dialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatalf("expected dial error for missing pane, got none")
	}
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 response, got %v", resp)
	}
}
