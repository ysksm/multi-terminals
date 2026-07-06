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

// Runner is a port.TerminalRunner that starts terminal sessions on a remote
// multi-terminals instance (identified by TerminalStartRequest.RemoteHost) by
// dialing its remote-terminal WebSocket endpoint. The process runs on the
// remote machine; output is streamed back over the connection.
type Runner struct {
	token  string
	dialer *websocket.Dialer
}

// NewRunner returns a Runner that authenticates with the given shared token.
func NewRunner(token string) *Runner {
	return &Runner{token: token, dialer: websocket.DefaultDialer}
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

// Start dials the remote instance, requests a terminal session and returns a
// port.TerminalSession bridged over the WebSocket connection.
func (r *Runner) Start(ctx context.Context, req port.TerminalStartRequest) (port.TerminalSession, error) {
	url, err := endpointURL(req.RemoteHost)
	if err != nil {
		return nil, fmt.Errorf("remote terminal: %w", err)
	}

	header := http.Header{}
	if r.token != "" {
		header.Set("Authorization", "Bearer "+r.token)
	}

	ws, resp, err := r.dialer.DialContext(ctx, url, header)
	if err != nil {
		if resp != nil {
			return nil, fmt.Errorf("remote terminal: connect %s: %w (HTTP %d)", req.RemoteHost, err, resp.StatusCode)
		}
		return nil, fmt.Errorf("remote terminal: connect %s: %w", req.RemoteHost, err)
	}

	s := &wsSession{
		id:   req.SessionID,
		ws:   ws,
		out:  make(chan []byte, 256),
		done: make(chan struct{}),
	}

	// Send the start request and wait for the server's ack so start errors
	// (bad directory, unknown shell, …) surface here, not asynchronously.
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
