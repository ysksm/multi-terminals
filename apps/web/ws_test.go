package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ysksm/multi-terminals/apps/web"
	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/session"
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

	// Register a fake session hub in the registry so the handler can find it.
	paneID := "test-pane-echo"
	fakeSess := apptest.NewFakeTerminalSession(paneID)
	hub := session.NewSession(fakeSess)
	deps.Registry.Add(paneID, hub)

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
	// The hub drain goroutine buffers it; the output pump forwards it as BinaryMessage.
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
	hub := session.NewSession(fakeSess)
	deps.Registry.Add(paneID, hub)

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

// TestWebSocketTwoSubscribersBothReceiveOutput verifies that two concurrent
// WebSocket connections to the same pane both receive output (hub fan-out).
// This replaces the old duplicate-connection-rejection test; the hub supports
// multiple subscribers safely, so ConnGuard is no longer needed.
func TestWebSocketTwoSubscribersBothReceiveOutput(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	paneID := "test-pane-fanout"
	fakeSess := apptest.NewFakeTerminalSession(paneID)
	hub := session.NewSession(fakeSess)
	deps.Registry.Add(paneID, hub)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/panes/" + paneID + "/io"

	// Two concurrent connections — both must succeed.
	conn1 := dialWS(t, wsURL)
	t.Cleanup(func() { conn1.Close() })

	conn2 := dialWS(t, wsURL)
	t.Cleanup(func() { conn2.Close() })

	// Send an input on conn1 which causes echo output; both subscribers should receive it.
	msg := `{"type":"input","data":"fanout"}`
	if err := conn1.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
		t.Fatalf("conn1 WriteMessage: %v", err)
	}

	// Both connections should receive the echoed output.
	var wg sync.WaitGroup
	wg.Add(2)
	results := make([]string, 2)

	for i, conn := range []*websocket.Conn{conn1, conn2} {
		i, conn := i, conn
		go func() {
			defer wg.Done()
			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			mt, recv, err := conn.ReadMessage()
			if err != nil {
				t.Errorf("conn%d ReadMessage: %v", i+1, err)
				return
			}
			if mt != websocket.BinaryMessage {
				t.Errorf("conn%d: expected BinaryMessage, got %d", i+1, mt)
				return
			}
			results[i] = string(recv)
		}()
	}
	wg.Wait()

	for i, got := range results {
		if got != "fanout" {
			t.Errorf("conn%d: expected 'fanout', got %q", i+1, got)
		}
	}
}

// TestWebSocketScrollbackSentOnConnect verifies that the scrollback snapshot is
// sent as the first binary frame when a client connects.
func TestWebSocketScrollbackSentOnConnect(t *testing.T) {
	deps := buildTestDeps(t)
	mux := web.NewMux(deps)

	paneID := "test-pane-scrollback"
	fakeSess := apptest.NewFakeTerminalSession(paneID)
	hub := session.NewSession(fakeSess)
	deps.Registry.Add(paneID, hub)

	// Write some data before the WebSocket client connects.
	_ = fakeSess.Write([]byte("prior-output"))
	// Wait for the drain goroutine to buffer it.
	time.Sleep(50 * time.Millisecond)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/panes/" + paneID + "/io"
	conn := dialWS(t, wsURL)
	t.Cleanup(func() { conn.Close() })

	// The first message should be the scrollback snapshot.
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	mt, recv, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if mt != websocket.BinaryMessage {
		t.Fatalf("expected BinaryMessage, got %d", mt)
	}
	if string(recv) != "prior-output" {
		t.Fatalf("expected scrollback 'prior-output', got %q", string(recv))
	}
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
