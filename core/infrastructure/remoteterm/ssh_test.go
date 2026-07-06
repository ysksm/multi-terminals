package remoteterm

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/ysksm/multi-terminals/core/application/port"
)

func TestIsSSHHost(t *testing.T) {
	cases := map[string]bool{
		"ssh://host":            true,
		"  ssh://user@host:22 ": true,
		"host:8080":             false,
		"http://host":           false,
		"":                      false,
	}
	for in, want := range cases {
		if got := IsSSHHost(in); got != want {
			t.Errorf("IsSSHHost(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseSSHTarget(t *testing.T) {
	// Expected default login must come from the same helper the implementation
	// uses (currentUsername strips Windows domain prefixes and falls back to
	// env), so the default-user cases stay correct across platforms.
	me, _ := currentUsername()

	cases := []struct {
		in       string
		wantUser string // "" means "current user"
		wantAddr string
		wantErr  bool
	}{
		{in: "ssh://user@host:2222", wantUser: "user", wantAddr: "host:2222"},
		{in: "ssh://user@host", wantUser: "user", wantAddr: "host:22"},
		{in: "ssh://host:2222", wantUser: me, wantAddr: "host:2222"},
		{in: "ssh://host", wantUser: me, wantAddr: "host:22"},
		{in: "  ssh://bob@10.0.0.5:22  ", wantUser: "bob", wantAddr: "10.0.0.5:22"},
		{in: "host:8080", wantErr: true},            // not an ssh:// url
		{in: "ssh://", wantErr: true},               // no host
		{in: "ssh://host/some/path", wantErr: true}, // path not allowed
		{in: "ssh://host:22?x=1", wantErr: true},    // query not allowed
	}
	for _, c := range cases {
		gotUser, gotAddr, err := parseSSHTarget(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseSSHTarget(%q): expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseSSHTarget(%q): %v", c.in, err)
			continue
		}
		if gotAddr != c.wantAddr {
			t.Errorf("parseSSHTarget(%q) addr = %q, want %q", c.in, gotAddr, c.wantAddr)
		}
		if c.wantUser != "" && gotUser != c.wantUser {
			t.Errorf("parseSSHTarget(%q) user = %q, want %q", c.in, gotUser, c.wantUser)
		}
	}
}

func TestSSHRunner_NoCredentials(t *testing.T) {
	r := &SSHRunner{
		authMethods:     func(string) ([]ssh.AuthMethod, func()) { return nil, func() {} },
		hostKeyCallback: ssh.InsecureIgnoreHostKey(),
		timeout:         time.Second,
	}
	_, err := r.Start(context.Background(), port.TerminalStartRequest{
		SessionID: "s", RemoteHost: "ssh://user@127.0.0.1:22",
	})
	if err == nil {
		t.Fatal("expected error when no SSH credentials are available")
	}
}

func TestSSHRunner_EndToEnd(t *testing.T) {
	srv := newTestSSHServer(t)

	r := &SSHRunner{
		authMethods: func(string) ([]ssh.AuthMethod, func()) {
			return []ssh.AuthMethod{ssh.PublicKeys(srv.clientKey)}, func() {}
		},
		hostKeyCallback: ssh.InsecureIgnoreHostKey(),
		timeout:         5 * time.Second,
	}

	host, p, err := net.SplitHostPort(srv.addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	remoteHost := fmt.Sprintf("ssh://tester@%s:%s", host, p)

	sess, err := r.Start(context.Background(), port.TerminalStartRequest{
		SessionID:  "s1",
		Cols:       100,
		Rows:       30,
		RemoteHost: remoteHost,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer sess.Close()

	if got := sess.ID(); got != "s1" {
		t.Errorf("ID = %q, want s1", got)
	}

	// Initial PTY size from the start request must have reached the server.
	eventually(t, "initial pty size", func() bool {
		c, rrows := srv.size()
		return c == 100 && rrows == 30
	})

	// Input is echoed by the fake shell and must arrive on Output. Binary-safe.
	input := []byte("ls -la\x1b[A\x00\xff\n")
	if err := sess.Write(input); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := readN(t, sess.Output(), len(input))
	if !bytes.Equal(got, input) {
		t.Errorf("echoed output = %q, want %q", got, input)
	}

	// Resize sends an SSH window-change; the server must observe the new size.
	if err := sess.Resize(200, 50); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	eventually(t, "resized pty size", func() bool {
		c, rrows := srv.size()
		return c == 200 && rrows == 50
	})

	// Closing the session tears down the SSH connection and ends Done.
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case <-sess.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for session done after Close")
	}
}

// readN reads from out until it has collected n bytes or times out.
func readN(t *testing.T, out <-chan []byte, n int) []byte {
	t.Helper()
	var buf []byte
	deadline := time.After(3 * time.Second)
	for len(buf) < n {
		select {
		case chunk, ok := <-out:
			if !ok {
				t.Fatalf("output channel closed after %d/%d bytes", len(buf), n)
			}
			buf = append(buf, chunk...)
		case <-deadline:
			t.Fatalf("timeout: read %d/%d bytes", len(buf), n)
		}
	}
	return buf
}

// ---- in-process SSH server for tests ----

// testSSHServer is a minimal sshd that accepts any public key, allocates a
// (fake) PTY, and echoes the shell's stdin back to its stdout. It records the
// most recent PTY window size so window-change propagation can be asserted.
type testSSHServer struct {
	addr      string
	clientKey ssh.Signer

	mu       sync.Mutex
	lastCols int
	lastRows int
}

func newTestSSHServer(t *testing.T) *testSSHServer {
	t.Helper()

	_, hostPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromKey(hostPriv)
	if err != nil {
		t.Fatalf("host signer: %v", err)
	}
	_, clientPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	clientSigner, err := ssh.NewSignerFromKey(clientPriv)
	if err != nil {
		t.Fatalf("client signer: %v", err)
	}

	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
			return &ssh.Permissions{}, nil
		},
	}
	cfg.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	s := &testSSHServer{addr: ln.Addr().String(), clientKey: clientSigner}
	go s.serve(ln, cfg)
	return s
}

func (s *testSSHServer) serve(ln net.Listener, cfg *ssh.ServerConfig) {
	for {
		nConn, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handleConn(nConn, cfg)
	}
}

func (s *testSSHServer) handleConn(nConn net.Conn, cfg *ssh.ServerConfig) {
	conn, chans, reqs, err := ssh.NewServerConn(nConn, cfg)
	if err != nil {
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(reqs)
	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(ssh.UnknownChannelType, "only session channels")
			continue
		}
		ch, requests, err := newChan.Accept()
		if err != nil {
			return
		}
		go s.handleSession(ch, requests)
	}
}

func (s *testSSHServer) handleSession(ch ssh.Channel, requests <-chan *ssh.Request) {
	for req := range requests {
		switch req.Type {
		case "pty-req":
			var m struct {
				Term          string
				Columns, Rows uint32
				Width, Height uint32
				Modes         string
			}
			_ = ssh.Unmarshal(req.Payload, &m)
			s.setSize(int(m.Columns), int(m.Rows))
			_ = req.Reply(true, nil)
		case "shell", "exec":
			_ = req.Reply(true, nil)
			go func() {
				_, _ = io.Copy(ch, ch) // echo stdin back to stdout
				_ = ch.Close()
			}()
		case "window-change":
			var m struct {
				Columns, Rows uint32
				Width, Height uint32
			}
			_ = ssh.Unmarshal(req.Payload, &m)
			s.setSize(int(m.Columns), int(m.Rows))
		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}

func (s *testSSHServer) setSize(cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastCols, s.lastRows = cols, rows
}

func (s *testSSHServer) size() (cols, rows int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastCols, s.lastRows
}
