package remoteterm

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/port"
)

// newIdentity generates a fresh instance identity in a temp dir.
func newIdentity(t *testing.T) *Identity {
	t.Helper()
	id, err := LoadOrCreateIdentity(t.TempDir())
	if err != nil {
		t.Fatalf("LoadOrCreateIdentity: %v", err)
	}
	return id
}

// newAuthKeys returns an AuthorizedKeys store authorizing the given keys.
func newAuthKeys(t *testing.T, authorized ...*Identity) *AuthorizedKeys {
	t.Helper()
	a := NewAuthorizedKeys(filepath.Join(t.TempDir(), AuthorizedKeysFile))
	for _, id := range authorized {
		if err := a.Add(id.PublicKeyString(), "test"); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	return a
}

// newTestServer starts an httptest server exposing the remote-terminal
// endpoint backed by a fake runner, authorizing the given client identities.
func newTestServer(t *testing.T, authorized ...*Identity) (*httptest.Server, *apptest.FakeTerminalRunner) {
	t.Helper()
	runner := apptest.NewFakeTerminalRunner()
	srv := httptest.NewServer(Handler(runner, newAuthKeys(t, authorized...)))
	t.Cleanup(srv.Close)
	return srv, runner
}

// startRemote dials the test server and starts a remote session.
func startRemote(t *testing.T, srv *httptest.Server, id *Identity, sessionID string) port.TerminalSession {
	t.Helper()
	r := NewRunner(id)
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
	id := newIdentity(t)
	srv, runner := newTestServer(t, id)
	sess := startRemote(t, srv, id, "pane-1")
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
	id := newIdentity(t)
	srv, runner := newTestServer(t, id)
	sess := startRemote(t, srv, id, "pane-2")
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

func TestHandler_DisabledWithoutAuthorizedKeys(t *testing.T) {
	srv, _ := newTestServer(t) // no keys authorized
	r := NewRunner(newIdentity(t))
	_, err := r.Start(context.Background(), port.TerminalStartRequest{
		SessionID: "p", RemoteHost: srv.URL,
	})
	if err == nil {
		t.Fatal("expected error when server has no authorized keys")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %v, want HTTP 403", err)
	}
}

func TestHandler_RejectsUnauthorizedKey(t *testing.T) {
	authorized := newIdentity(t)
	srv, runner := newTestServer(t, authorized)

	intruder := newIdentity(t) // valid keypair, but not in the authorized list
	r := NewRunner(intruder)
	_, err := r.Start(context.Background(), port.TerminalStartRequest{
		SessionID: "p", RemoteHost: srv.URL,
	})
	if err == nil {
		t.Fatal("expected error with unauthorized key")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Errorf("error = %v, want unauthorized", err)
	}
	if len(runner.Started) != 0 {
		t.Errorf("no session must be started on auth failure, got %d", len(runner.Started))
	}
}

func TestHandler_RejectsBadSignature(t *testing.T) {
	// Client presents an AUTHORIZED public key but signs with a DIFFERENT
	// private key — the signature check must reject it.
	authorized := newIdentity(t)
	srv, runner := newTestServer(t, authorized)

	forger := newIdentity(t)
	forger.pub = authorized.pub // claim the authorized identity, keep own private key
	r := NewRunner(forger)
	_, err := r.Start(context.Background(), port.TerminalStartRequest{
		SessionID: "p", RemoteHost: srv.URL,
	})
	if err == nil || !strings.Contains(err.Error(), "unauthorized") {
		t.Errorf("error = %v, want unauthorized", err)
	}
	if len(runner.Started) != 0 {
		t.Errorf("no session must be started on forged signature, got %d", len(runner.Started))
	}
}

func TestHandler_StartErrorIsReturnedToCaller(t *testing.T) {
	id := newIdentity(t)
	runner := apptest.NewFakeTerminalRunner()
	runner.StartErr = errors.New("no such directory")
	srv := httptest.NewServer(Handler(runner, newAuthKeys(t, id)))
	defer srv.Close()

	r := NewRunner(id)
	_, err := r.Start(context.Background(), port.TerminalStartRequest{
		SessionID: "p", RemoteHost: srv.URL,
	})
	if err == nil || !strings.Contains(err.Error(), "no such directory") {
		t.Errorf("error = %v, want to contain server start error", err)
	}
}

func TestLoadOrCreateIdentity_GeneratesAndReloads(t *testing.T) {
	dir := t.TempDir()

	id1, err := LoadOrCreateIdentity(dir)
	if err != nil {
		t.Fatalf("LoadOrCreateIdentity (create): %v", err)
	}
	if !strings.HasPrefix(id1.PublicKeyString(), keyPrefix) {
		t.Errorf("public key = %q, want %q prefix", id1.PublicKeyString(), keyPrefix)
	}
	if !strings.HasPrefix(id1.Fingerprint(), "SHA256:") {
		t.Errorf("fingerprint = %q, want SHA256: prefix", id1.Fingerprint())
	}

	// The private key file must exist with private permissions (Unix only).
	info, err := os.Stat(filepath.Join(dir, PrivateKeyFile))
	if err != nil {
		t.Fatalf("stat private key: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("private key mode = %v, want 0600", info.Mode().Perm())
	}

	// The .pub file must contain the same public key.
	pubData, err := os.ReadFile(filepath.Join(dir, PublicKeyFile))
	if err != nil {
		t.Fatalf("read public key file: %v", err)
	}
	if strings.TrimSpace(string(pubData)) != id1.PublicKeyString() {
		t.Errorf("pub file = %q, want %q", strings.TrimSpace(string(pubData)), id1.PublicKeyString())
	}

	// A second load must return the SAME key, not generate a new one.
	id2, err := LoadOrCreateIdentity(dir)
	if err != nil {
		t.Fatalf("LoadOrCreateIdentity (reload): %v", err)
	}
	if id1.PublicKeyString() != id2.PublicKeyString() {
		t.Errorf("reloaded key differs: %q vs %q", id1.PublicKeyString(), id2.PublicKeyString())
	}
}

func TestAuthorizedKeys_AddListRemove(t *testing.T) {
	a := NewAuthorizedKeys(filepath.Join(t.TempDir(), AuthorizedKeysFile))
	if a.Enabled() {
		t.Error("empty store must be disabled")
	}

	id := newIdentity(t)
	if err := a.Add(id.PublicKeyString(), "laptop"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !a.Enabled() || !a.IsAuthorized(id.pub) {
		t.Error("added key must be authorized and enable the store")
	}
	keys := a.List()
	if len(keys) != 1 || keys[0].Comment != "laptop" || keys[0].Key != id.PublicKeyString() {
		t.Errorf("List = %+v", keys)
	}

	// Re-adding updates the comment without duplicating.
	if err := a.Add(id.PublicKeyString(), "desktop"); err != nil {
		t.Fatalf("Add (update): %v", err)
	}
	keys = a.List()
	if len(keys) != 1 || keys[0].Comment != "desktop" {
		t.Errorf("List after update = %+v", keys)
	}

	// Invalid keys are rejected.
	if err := a.Add("not-a-key", ""); err == nil {
		t.Error("Add must reject malformed keys")
	}

	if err := a.Remove(id.PublicKeyString()); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if a.Enabled() || a.IsAuthorized(id.pub) {
		t.Error("removed key must not be authorized")
	}
}

func TestDispatchRunner_RoutesByRemoteHost(t *testing.T) {
	local := apptest.NewFakeTerminalRunner()
	ws := apptest.NewFakeTerminalRunner()
	sshR := apptest.NewFakeTerminalRunner()
	d := NewDispatchRunner(local, ws, sshR)

	if _, err := d.Start(context.Background(), port.TerminalStartRequest{SessionID: "a"}); err != nil {
		t.Fatalf("local Start: %v", err)
	}
	if _, err := d.Start(context.Background(), port.TerminalStartRequest{SessionID: "b", RemoteHost: "other:8080"}); err != nil {
		t.Fatalf("ws Start: %v", err)
	}
	if _, err := d.Start(context.Background(), port.TerminalStartRequest{SessionID: "c", RemoteHost: "ssh://user@host:22"}); err != nil {
		t.Fatalf("ssh Start: %v", err)
	}
	if len(local.Started) != 1 || local.Started[0].SessionID != "a" {
		t.Errorf("local runner got %+v", local.Started)
	}
	if len(ws.Started) != 1 || ws.Started[0].SessionID != "b" {
		t.Errorf("ws runner got %+v", ws.Started)
	}
	if len(sshR.Started) != 1 || sshR.Started[0].SessionID != "c" {
		t.Errorf("ssh runner got %+v", sshR.Started)
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
