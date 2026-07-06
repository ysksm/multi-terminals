package remoteterm

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/port"
)

const testToken = "test-secret"

// newTestServer starts an httptest server exposing the remote-terminal
// endpoint backed by a fake runner, and returns both.
func newTestServer(t *testing.T, token string) (*httptest.Server, *apptest.FakeTerminalRunner) {
	t.Helper()
	runner := apptest.NewFakeTerminalRunner()
	srv := httptest.NewServer(Handler(runner, token))
	t.Cleanup(srv.Close)
	return srv, runner
}

// startRemote dials the test server and starts a remote session.
func startRemote(t *testing.T, srv *httptest.Server, token, sessionID string) port.TerminalSession {
	t.Helper()
	r := NewRunner(token)
	sess, err := r.Start(context.Background(), port.TerminalStartRequest{
		SessionID:  sessionID,
		Dir:        "/tmp",
		Shell:      "/bin/fake",
		Cols:       120,
		Rows:       40,
		RemoteHost: srv.URL,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	return sess
}

// eventually polls cond until it returns true or the deadline passes.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func TestRemoteSession_EndToEnd(t *testing.T) {
	srv, runner := newTestServer(t, testToken)
	sess := startRemote(t, srv, testToken, "pane-1")
	defer sess.Close()

	if got := sess.ID(); got != "pane-1" {
		t.Errorf("ID = %q, want %q", got, "pane-1")
	}

	// The server must have started a session with the request parameters
	// (RemoteHost cleared: the listening side executes locally).
	var remote *apptest.FakeTerminalSession
	eventually(t, "server session start", func() bool {
		remote = runner.Session("pane-1")
		return remote != nil
	})
	req := runner.Started[0]
	if req.Dir != "/tmp" || req.Shell != "/bin/fake" || req.Cols != 120 || req.Rows != 40 {
		t.Errorf("server start request = %+v", req)
	}
	if req.RemoteHost != "" {
		t.Errorf("server start request RemoteHost = %q, want empty", req.RemoteHost)
	}

	// Input travels to the remote session; the fake echoes it back as output,
	// which must arrive on the local Output channel. Binary-safe round trip.
	input := []byte("echo hello\x1b[A\x00\xff\n")
	if err := sess.Write(input); err != nil {
		t.Fatalf("Write: %v", err)
	}
	select {
	case chunk := <-sess.Output():
		if !bytes.Equal(chunk, input) {
			t.Errorf("output = %q, want %q", chunk, input)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for echoed output")
	}

	// Resize propagates to the remote session.
	if err := sess.Resize(200, 50); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	eventually(t, "resize propagation", func() bool {
		cols, rows := remote.LastSize()
		return cols == 200 && rows == 50
	})

	// Closing the local session terminates the remote one and ends Output/Done.
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-remote.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for remote session to close")
	}
	select {
	case <-sess.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for local session done")
	}
}

func TestRemoteSession_RemoteExitEndsLocalSession(t *testing.T) {
	srv, runner := newTestServer(t, testToken)
	sess := startRemote(t, srv, testToken, "pane-2")
	defer sess.Close()

	var remote *apptest.FakeTerminalSession
	eventually(t, "server session start", func() bool {
		remote = runner.Session("pane-2")
		return remote != nil
	})

	// Simulate the remote process exiting.
	_ = remote.Close()

	select {
	case <-sess.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for local session done after remote exit")
	}
	// Output must be closed (drained) as well.
	for range sess.Output() {
	}
}

func TestHandler_DisabledWithoutToken(t *testing.T) {
	srv, _ := newTestServer(t, "")
	r := NewRunner("anything")
	_, err := r.Start(context.Background(), port.TerminalStartRequest{
		SessionID: "p", RemoteHost: srv.URL,
	})
	if err == nil {
		t.Fatal("expected error when server has no token configured")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %v, want HTTP 403", err)
	}
}

func TestHandler_RejectsBadToken(t *testing.T) {
	srv, runner := newTestServer(t, testToken)
	r := NewRunner("wrong-token")
	_, err := r.Start(context.Background(), port.TerminalStartRequest{
		SessionID: "p", RemoteHost: srv.URL,
	})
	if err == nil {
		t.Fatal("expected error with wrong token")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %v, want HTTP 401", err)
	}
	if len(runner.Started) != 0 {
		t.Errorf("no session must be started on auth failure, got %d", len(runner.Started))
	}
}

func TestHandler_StartErrorIsReturnedToCaller(t *testing.T) {
	runner := apptest.NewFakeTerminalRunner()
	runner.StartErr = errors.New("no such directory")
	srv := httptest.NewServer(Handler(runner, testToken))
	defer srv.Close()

	r := NewRunner(testToken)
	_, err := r.Start(context.Background(), port.TerminalStartRequest{
		SessionID: "p", RemoteHost: srv.URL,
	})
	if err == nil || !strings.Contains(err.Error(), "no such directory") {
		t.Errorf("error = %v, want to contain server start error", err)
	}
}

func TestDispatchRunner_RoutesByRemoteHost(t *testing.T) {
	local := apptest.NewFakeTerminalRunner()
	remote := apptest.NewFakeTerminalRunner()
	d := NewDispatchRunner(local, remote)

	if _, err := d.Start(context.Background(), port.TerminalStartRequest{SessionID: "a"}); err != nil {
		t.Fatalf("local Start: %v", err)
	}
	if _, err := d.Start(context.Background(), port.TerminalStartRequest{SessionID: "b", RemoteHost: "other:8080"}); err != nil {
		t.Fatalf("remote Start: %v", err)
	}
	if len(local.Started) != 1 || local.Started[0].SessionID != "a" {
		t.Errorf("local runner got %+v", local.Started)
	}
	if len(remote.Started) != 1 || remote.Started[0].SessionID != "b" {
		t.Errorf("remote runner got %+v", remote.Started)
	}
}

func TestEndpointURL(t *testing.T) {
	cases := []struct {
		in, want string
		wantErr  bool
	}{
		{in: "192.168.1.10:8080", want: "ws://192.168.1.10:8080" + EndpointPath},
		{in: "http://host:8080", want: "ws://host:8080" + EndpointPath},
		{in: "https://host.example", want: "wss://host.example" + EndpointPath},
		{in: "ws://host:1", want: "ws://host:1" + EndpointPath},
		{in: "wss://host/", want: "wss://host" + EndpointPath},
		{in: "  ", wantErr: true},
	}
	for _, c := range cases {
		got, err := endpointURL(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("endpointURL(%q): expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("endpointURL(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("endpointURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
