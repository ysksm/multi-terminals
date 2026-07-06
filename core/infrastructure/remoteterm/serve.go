package remoteterm

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ysksm/multi-terminals/core/application/port"
)

// startTimeout bounds how long the server waits for the auth and start
// messages after the WebSocket upgrade.
const startTimeout = 10 * time.Second

// nonceSize is the length of the random challenge nonce in bytes.
const nonceSize = 32

// upgrader configures WebSocket upgrade behaviour for the remote endpoint.
// Access control is enforced by key authentication, not the Origin header —
// connections come from other multi-terminals backends, not browsers.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Handler returns the HTTP handler for the remote-terminal endpoint.
// runner starts sessions on THIS machine (the listening side); its output is
// streamed back to the connecting instance.
//
// auth is the list of client public keys allowed to connect. While the list
// is empty the endpoint is disabled and every request is rejected with 403,
// so a server never exposes shell execution unless keys were explicitly
// authorized. Each connection must pass an Ed25519 challenge-response before
// a terminal is started.
func Handler(runner port.TerminalRunner, auth *AuthorizedKeys) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !auth.Enabled() {
			http.Error(w, `{"error":"remote access disabled: no authorized keys configured"}`, http.StatusForbidden)
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

		// Challenge-response authentication: send a fresh nonce, require a
		// valid signature from an authorized key before anything else.
		nonce := make([]byte, nonceSize)
		if _, err := rand.Read(nonce); err != nil {
			_ = writeCtl(controlMsg{Type: msgError, Error: "internal error"})
			return
		}
		if err := writeCtl(controlMsg{Type: msgChallenge, Nonce: base64.StdEncoding.EncodeToString(nonce)}); err != nil {
			return
		}

		_ = ws.SetReadDeadline(time.Now().Add(startTimeout))
		var authMsg controlMsg
		if _, msgBytes, err := ws.ReadMessage(); err != nil {
			return
		} else if err := json.Unmarshal(msgBytes, &authMsg); err != nil || authMsg.Type != msgAuth {
			_ = writeCtl(controlMsg{Type: msgError, Error: "expected auth message"})
			return
		}
		pub, err := ParsePublicKey(authMsg.PublicKey)
		if err != nil {
			_ = writeCtl(controlMsg{Type: msgError, Error: "unauthorized"})
			return
		}
		sig, err := base64.StdEncoding.DecodeString(authMsg.Signature)
		if err != nil || !auth.IsAuthorized(pub) || !verifyAuth(pub, nonce, sig) {
			_ = writeCtl(controlMsg{Type: msgError, Error: "unauthorized"})
			return
		}

		// Next message must be "start".
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
