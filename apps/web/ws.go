package web

import (
	"encoding/json"
	"net/http"
	"sync"

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
// On connect: fetches the scrollback snapshot from the hub and sends it as a
// single binary frame, then streams live output via a hub subscription.
//
// Output pump: reads from sub.C() and forwards each chunk as a binary
// WebSocket message. When sub.Done() closes (session ended or slow-subscriber
// drop), a Close frame is sent and the pump stops.
//
// Input loop: reads WebSocket messages from the client.
//   - type=="input"  → WriteToPaneCommand (data field → bytes)
//   - type=="resize" → ResizePaneCommand  (cols/rows)
//
// The handler returns when either the client disconnects or the subscription ends.
// Multiple concurrent WebSocket connections to the same pane are supported; the
// hub fan-out handles them safely.
func (d Deps) handlePaneIO(w http.ResponseWriter, r *http.Request) {
	paneID := r.PathValue("paneId")

	// Look up the live session hub. Return 404 before upgrading if not found.
	hub, ok := d.Registry.Get(paneID)
	if !ok {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}

	// Subscribe before upgrading so we don't miss output that arrives between
	// the lookup and the upgrade.
	snapshot, sub := hub.Subscribe()
	// Unsubscribe when this handler exits (idempotent).
	defer hub.Unsubscribe(sub)

	// Upgrade the HTTP connection to WebSocket.
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// upgrader has already written the error response.
		return
	}

	// A1: idempotent done-channel closer shared by both goroutines.
	// Closed when either the client disconnects (input loop) or the subscription
	// ends (output pump).
	done := make(chan struct{})
	var closeOnce sync.Once
	closeDone := func() {
		closeOnce.Do(func() { close(done) })
	}

	// wsMu serializes all WebSocket writes so gorilla's single-writer
	// requirement is met even when two code paths could write concurrently.
	var wsMu sync.Mutex
	wsWrite := func(mt int, data []byte) error {
		wsMu.Lock()
		defer wsMu.Unlock()
		return ws.WriteMessage(mt, data)
	}

	// Send the scrollback snapshot first (if non-empty) so the client
	// immediately sees prior output on reconnect.
	if len(snapshot) > 0 {
		if err := wsWrite(websocket.BinaryMessage, snapshot); err != nil {
			wsMu.Lock()
			ws.Close()
			wsMu.Unlock()
			return
		}
	}

	// Output pump: session hub → client.
	// Drives off sub.C() (live output) and sub.Done() (subscription ended).
	// sub.C() is never closed, so no ok-check needed.
	var pumpDone sync.WaitGroup
	pumpDone.Add(1)
	go func() {
		defer func() {
			pumpDone.Done()
			closeDone()
			wsMu.Lock()
			ws.Close()
			wsMu.Unlock()
		}()

		for {
			select {
			case chunk := <-sub.C():
				if err := wsWrite(websocket.BinaryMessage, chunk); err != nil {
					// Client write failed.
					return
				}
			case <-sub.Done():
				// Subscription ended (session exited or slow-subscriber drop).
				_ = wsWrite(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session ended"))
				return
			case <-done:
				// Client disconnected; stop pump without draining.
				return
			}
		}
	}()

	// Input loop: client → session.
	for {
		_, msgBytes, err := ws.ReadMessage()
		if err != nil {
			// Client disconnected or read error — signal the output pump to stop.
			closeDone()
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

	// Wait for the output pump to finish before returning. This ensures ws is
	// not closed from two goroutines simultaneously.
	<-done
}
