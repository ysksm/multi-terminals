package remoteterm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/ysksm/multi-terminals/core/application/port"
)

// Compile-time interface assertions.
var _ port.TerminalRunner = (*Runner)(nil)
var _ port.TerminalSession = (*wsSession)(nil)

// CurrentIdentityFunc returns the instance's current signing identity and
// whether one exists yet. IdentityStore.Current satisfies it directly.
type CurrentIdentityFunc func() (*Identity, bool)

// Runner is a port.TerminalRunner that starts terminal sessions on a remote
// multi-terminals instance (identified by TerminalStartRequest.RemoteHost) by
// dialing its remote-terminal WebSocket endpoint. The process runs on the
// remote machine; output is streamed back over the connection.
type Runner struct {
	current CurrentIdentityFunc
	dialer  *websocket.Dialer
}

// NewRunner returns a Runner that authenticates with the instance identity
// returned by current at dial time: the remote host must have that identity's
// public key in its authorized-keys list. Reading the identity lazily lets a
// key created (or regenerated) after startup take effect without a restart.
func NewRunner(current CurrentIdentityFunc) *Runner {
	return &Runner{current: current, dialer: websocket.DefaultDialer}
}

// endpointURL converts a user-supplied host value into the WebSocket URL of
// the remote-terminal endpoint. Accepted forms: "host:port", "http://…",
// "https://…", "ws://…", "wss://…" (an existing path prefix is preserved).
func endpointURL(host string) (string, error) {
	h := strings.TrimSuffix(strings.TrimSpace(host), "/")
	if h == "" {
		return "", fmt.Errorf("remote host must not be empty")
	}
	switch {
	case strings.HasPrefix(h, "ws://"), strings.HasPrefix(h, "wss://"):
		// already a WebSocket URL
	case strings.HasPrefix(h, "http://"):
		h = "ws://" + strings.TrimPrefix(h, "http://")
	case strings.HasPrefix(h, "https://"):
		h = "wss://" + strings.TrimPrefix(h, "https://")
	default:
		h = "ws://" + h
	}
	return h + EndpointPath, nil
}

// Start dials the remote instance, answers its authentication challenge with
// this instance's key, requests a terminal session and returns a
// port.TerminalSession bridged over the WebSocket connection.
func (r *Runner) Start(ctx context.Context, req port.TerminalStartRequest) (port.TerminalSession, error) {
	if r.current == nil {
		return nil, fmt.Errorf("remote terminal: no instance identity configured")
	}
	identity, ok := r.current()
	if !ok || identity == nil {
		return nil, fmt.Errorf("remote terminal: this instance has no key yet — create one in 🔑 リモート設定 (この端末の鍵を作成) before connecting to another multi-terminals instance")
	}
	url, err := endpointURL(req.RemoteHost)
	if err != nil {
		return nil, fmt.Errorf("remote terminal: %w", err)
	}

	ws, resp, err := r.dialer.DialContext(ctx, url, http.Header{})
	if err != nil {
		if resp != nil {
			if resp.StatusCode == http.StatusForbidden {
				return nil, fmt.Errorf("remote terminal: connect %s: remote access is disabled on the listening side (HTTP 403) — add THIS instance's public key to the far machine's authorized keys (🔑 リモート設定 → 許可された鍵)", req.RemoteHost)
			}
			return nil, fmt.Errorf("remote terminal: connect %s: %w (HTTP %d)", req.RemoteHost, err, resp.StatusCode)
		}
		return nil, fmt.Errorf("remote terminal: connect %s: %w — check the host is running and reachable, the port is included (e.g. host:8080), and no firewall is blocking it (macOS: システム設定 → ネットワーク → ファイアウォール)", req.RemoteHost, err)
	}

	s := &wsSession{
		id:   req.SessionID,
		ws:   ws,
		out:  make(chan []byte, 256),
		done: make(chan struct{}),
	}

	// Answer the server's challenge by signing its nonce with our key.
	var challenge controlMsg
	if _, msgBytes, err := ws.ReadMessage(); err != nil {
		ws.Close()
		return nil, fmt.Errorf("remote terminal: read challenge: %w", err)
	} else if err := json.Unmarshal(msgBytes, &challenge); err != nil || challenge.Type != msgChallenge {
		ws.Close()
		return nil, fmt.Errorf("remote terminal: expected challenge from %s", req.RemoteHost)
	}
	nonce, err := base64.StdEncoding.DecodeString(challenge.Nonce)
	if err != nil {
		ws.Close()
		return nil, fmt.Errorf("remote terminal: invalid challenge nonce: %w", err)
	}
	if err := s.writeCtl(controlMsg{
		Type:      msgAuth,
		PublicKey: identity.PublicKeyString(),
		Signature: base64.StdEncoding.EncodeToString(identity.sign(nonce)),
	}); err != nil {
		ws.Close()
		return nil, fmt.Errorf("remote terminal: send auth: %w", err)
	}

	// Send the start request and wait for the server's ack so auth and start
	// errors (unauthorized key, bad directory, …) surface here, not
	// asynchronously.
	if err := s.writeCtl(controlMsg{
		Type:      msgStart,
		SessionID: req.SessionID,
		Dir:       req.Dir,
		Shell:     req.Shell,
		Cols:      req.Cols,
		Rows:      req.Rows,
	}); err != nil {
		ws.Close()
		return nil, fmt.Errorf("remote terminal: send start: %w", err)
	}

	var ack controlMsg
	if _, msgBytes, err := ws.ReadMessage(); err != nil {
		ws.Close()
		return nil, fmt.Errorf("remote terminal: read start ack: %w", err)
	} else if err := json.Unmarshal(msgBytes, &ack); err != nil {
		ws.Close()
		return nil, fmt.Errorf("remote terminal: invalid start ack: %w", err)
	}
	switch ack.Type {
	case msgStarted:
		// ok
	case msgError:
		ws.Close()
		return nil, fmt.Errorf("remote terminal: %s: %s", req.RemoteHost, ack.Error)
	default:
		ws.Close()
		return nil, fmt.Errorf("remote terminal: unexpected ack %q", ack.Type)
	}

	go s.pump()
	return s, nil
}

// wsSession implements port.TerminalSession over a WebSocket connection to a
// remote multi-terminals instance.
type wsSession struct {
	id string
	ws *websocket.Conn

	out  chan []byte
	done chan struct{}

	writeMu   sync.Mutex // serializes WebSocket writes
	closeOnce sync.Once  // ensures the connection is closed exactly once
}

// pump reads frames from the WebSocket and forwards terminal output (binary
// frames) to the out channel. It exits on any read error — server close,
// network failure, or local Close() — and is the sole closer of out and done.
func (s *wsSession) pump() {
	defer func() {
		close(s.out)
		close(s.done)
	}()
	for {
		mt, data, err := s.ws.ReadMessage()
		if err != nil {
			s.closeConn()
			return
		}
		if mt != websocket.BinaryMessage {
			// Text frames are control messages; "exit" announces process end
			// (the server closes the connection right after, ending this loop).
			continue
		}
		s.out <- data
	}
}

func (s *wsSession) closeConn() {
	s.closeOnce.Do(func() { _ = s.ws.Close() })
}

func (s *wsSession) writeCtl(m controlMsg) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.ws.WriteMessage(websocket.TextMessage, b)
}

// ID returns the session identifier.
func (s *wsSession) ID() string { return s.id }

// Write sends input bytes to the remote terminal (base64-encoded on the wire).
func (s *wsSession) Write(data []byte) error {
	return s.writeCtl(controlMsg{Type: msgInput, Data: base64.StdEncoding.EncodeToString(data)})
}

// Resize updates the remote terminal window size.
func (s *wsSession) Resize(cols, rows uint16) error {
	return s.writeCtl(controlMsg{Type: msgResize, Cols: cols, Rows: rows})
}

// Output returns the read-only output channel. It is closed when the remote
// session ends or the connection is lost.
func (s *wsSession) Output() <-chan []byte { return s.out }

// Done returns a channel closed when the session is fully terminated.
func (s *wsSession) Done() <-chan struct{} { return s.done }

// Close terminates the session by closing the connection; the remote side
// kills the terminal process when the connection drops. Idempotent.
func (s *wsSession) Close() error {
	s.closeConn()
	return nil
}
