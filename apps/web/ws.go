package web

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/ysksm/multi-terminals/core/application/command"
)

// upgrader configures WebSocket upgrade behaviour.
// CheckOrigin always returns true — the server is local, so origin checks are
// left to the caller environment.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// wsInputMsg is the client→server message format.
type wsInputMsg struct {
	Type string `json:"type"`
	// For type=="input"
	Data string `json:"data"`
	// For type=="resize"
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// handlePaneIO upgrades the connection to WebSocket and bridges I/O for the
// terminal session identified by {paneId}.
//
// Output pump: reads from sess.Output() and forwards each chunk as a binary
// WebSocket message. When the output channel is closed (process exit / Close),
// the pump sends a Close frame and stops.
//
// Input loop: reads WebSocket messages from the client.
//   - type=="input"  → WriteToPaneCommand (data field → bytes)
//   - type=="resize" → ResizePaneCommand  (cols/rows)
//
// The handler returns when either the client disconnects or the session ends.
func (d Deps) handlePaneIO(w http.ResponseWriter, r *http.Request) {
	paneID := r.PathValue("paneId")

	// Look up the live session. Return 404 before upgrading if not found.
	sess, ok := d.Registry.Get(paneID)
	if !ok {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}

	// Upgrade the HTTP connection to WebSocket.
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// upgrader has already written the error response.
		return
	}

	// done is closed when either side (client or output pump) terminates, so
	// the other side can stop cleanly.
	done := make(chan struct{})

	// Output pump: session → client.
	go func() {
		defer func() {
			// Signal the input loop to stop and close the WebSocket.
			select {
			case <-done:
			default:
				close(done)
			}
			ws.Close()
		}()

		for {
			select {
			case chunk, more := <-sess.Output():
				if !more {
					// Session output channel closed — process exited or Close called.
					return
				}
				if err := ws.WriteMessage(websocket.BinaryMessage, chunk); err != nil {
					// Client disconnected or write failed.
					return
				}
			case <-done:
				return
			case <-sess.Done():
				// Drain remaining output.
				for {
					select {
					case chunk, more := <-sess.Output():
						if !more {
							return
						}
						_ = ws.WriteMessage(websocket.BinaryMessage, chunk)
					default:
						return
					}
				}
			}
		}
	}()

	// Input loop: client → session.
	for {
		_, msgBytes, err := ws.ReadMessage()
		if err != nil {
			// Client disconnected or read error — stop the output pump too.
			select {
			case <-done:
			default:
				close(done)
			}
			break
		}

		var msg wsInputMsg
		if jsonErr := json.Unmarshal(msgBytes, &msg); jsonErr != nil {
			// Ignore malformed messages.
			continue
		}

		switch msg.Type {
		case "input":
			_ = d.Write.Handle(r.Context(), command.WriteToPaneCommand{
				PaneID: paneID,
				Data:   []byte(msg.Data),
			})
		case "resize":
			_ = d.Resize.Handle(r.Context(), command.ResizePaneCommand{
				PaneID: paneID,
				Cols:   msg.Cols,
				Rows:   msg.Rows,
			})
		}
	}
}
