package remoteterm

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ysksm/multi-terminals/core/application/port"
)

// startTimeout bounds how long the server waits for the initial start message
// after the WebSocket upgrade.
const startTimeout = 10 * time.Second

// upgrader configures WebSocket upgrade behaviour for the remote endpoint.
// Access control is enforced by the bearer token, not the Origin header —
// connections come from other multi-terminals backends, not browsers.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// bearerToken extracts the token from an "Authorization: Bearer <token>" header.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return h[len(prefix):]
	}
	return ""
}

// Handler returns the HTTP handler for the remote-terminal endpoint.
// runner starts sessions on THIS machine (the listening side); its output is
// streamed back to the connecting instance.
//
// token is the shared secret required from callers. When token is empty the
// endpoint is disabled and every request is rejected with 403, so a server
// never exposes shell execution unless explicitly configured.
func Handler(runner port.TerminalRunner, token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			http.Error(w, `{"error":"remote access disabled: no MULTI_TERMINALS_REMOTE_TOKEN configured"}`, http.StatusForbidden)
			return
		}
		got := bearerToken(r)
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			// upgrader has already written the error response.
			return
		}
		defer ws.Close()

		// wsMu serializes all writes (gorilla allows a single concurrent writer).
		var wsMu sync.Mutex
		wsWrite := func(mt int, data []byte) error {
			wsMu.Lock()
			defer wsMu.Unlock()
			return ws.WriteMessage(mt, data)
		}
		writeCtl := func(m controlMsg) error {
			b, err := json.Marshal(m)
			if err != nil {
				return err
			}
			return wsWrite(websocket.TextMessage, b)
		}

		// First message must be "start".
		_ = ws.SetReadDeadline(time.Now().Add(startTimeout))
		var start controlMsg
		if _, msgBytes, err := ws.ReadMessage(); err != nil {
			return
		} else if err := json.Unmarshal(msgBytes, &start); err != nil || start.Type != msgStart {
			_ = writeCtl(controlMsg{Type: msgError, Error: "expected start message"})
			return
		}
		_ = ws.SetReadDeadline(time.Time{})

		// Start the terminal on this (listening) machine. RemoteHost is cleared:
		// this instance is the execution target, never a further hop.
		sess, err := runner.Start(r.Context(), port.TerminalStartRequest{
			SessionID: start.SessionID,
			Dir:       start.Dir,
			Shell:     start.Shell,
			Cols:      start.Cols,
			Rows:      start.Rows,
		})
		if err != nil {
			_ = writeCtl(controlMsg{Type: msgError, Error: err.Error()})
			return
		}
		// The session must never outlive this connection: if the caller goes
		// away, the shell on this machine is terminated.
		defer sess.Close()

		if err := writeCtl(controlMsg{Type: msgStarted}); err != nil {
			return
		}

		// Output pump: session output → binary frames. On session end, notify
		// the client and close the connection so the input loop unblocks.
		go func() {
			for chunk := range sess.Output() {
				if err := wsWrite(websocket.BinaryMessage, chunk); err != nil {
					_ = sess.Close()
					return
				}
			}
			_ = writeCtl(controlMsg{Type: msgExit})
			_ = wsWrite(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session ended"))
			wsMu.Lock()
			_ = ws.Close()
			wsMu.Unlock()
		}()

		// Input loop: control frames → session. Returning closes the session
		// (deferred above), which in turn ends the output pump.
		for {
			_, msgBytes, err := ws.ReadMessage()
			if err != nil {
				return
			}
			var msg controlMsg
			if err := json.Unmarshal(msgBytes, &msg); err != nil {
				continue // ignore malformed messages
			}
			switch msg.Type {
			case msgInput:
				data, err := base64.StdEncoding.DecodeString(msg.Data)
				if err != nil {
					continue
				}
				_ = sess.Write(data)
			case msgResize:
				_ = sess.Resize(msg.Cols, msg.Rows)
			}
		}
	}
}
