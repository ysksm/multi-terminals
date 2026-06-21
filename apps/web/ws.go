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

// ConnGuard enforces at most one active WebSocket attachment per paneId.
// It is safe for concurrent use.
type ConnGuard struct {
	mu       sync.Mutex
	attached map[string]bool
}

// NewConnGuard returns a new, empty ConnGuard.
func NewConnGuard() *ConnGuard {
	return &ConnGuard{attached: make(map[string]bool)}
}

// claim tries to claim paneID. Returns true if successful (caller holds the
// attachment), false if already claimed.
func (g *ConnGuard) claim(paneID string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.attached[paneID] {
		return false
	}
	g.attached[paneID] = true
	return true
}

// release releases a previously claimed paneID.
func (g *ConnGuard) release(paneID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.attached, paneID)
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

	// A3: enforce single consumer per pane. Reject (409) if already attached.
	if !d.ConnGuard.claim(paneID) {
		http.Error(w, `{"error":"pane already attached"}`, http.StatusConflict)
		return
	}
	// Release the claim when the handler (and the pump goroutine) are both done.
	// We use a separate WaitGroup so release happens only after the pump exits.
	var pumpDone sync.WaitGroup
	pumpDone.Add(1)
	go func() {
		pumpDone.Wait()
		d.ConnGuard.release(paneID)
	}()

	// Upgrade the HTTP connection to WebSocket.
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// upgrader has already written the error response; the pump was never
		// started, so signal that it is "done" immediately.
		pumpDone.Done()
		return
	}

	// A1: idempotent done-channel closer shared by both goroutines.
	// closed when either the client disconnects (input loop) or the session ends
	// (output pump).
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

	// Output pump: session → client.
	// A4: drive purely off sess.Output() (range loop) so no buffered chunks are
	// dropped. The pump exits when:
	//   a) the output channel is closed (session ended), or
	//   b) done fires (client disconnected) — we select on done alongside
	//      the channel to avoid being blocked forever on a long-lived session.
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
			case chunk, more := <-sess.Output():
				if !more {
					// Session output channel closed — process exited or Close called.
					// Send a clean close frame before exiting.
					_ = wsWrite(websocket.CloseMessage,
						websocket.FormatCloseMessage(websocket.CloseNormalClosure, "session ended"))
					return
				}
				if err := wsWrite(websocket.BinaryMessage, chunk); err != nil {
					// Client write failed.
					return
				}
			case <-done:
				// Client disconnected; exit pump without draining to avoid a
				// goroutine leak.  Any remaining output is intentionally dropped
				// because there is no client to receive it.
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
	// not closed from two goroutines and that the ConnGuard claim is released
	// only once the pump has fully exited.
	<-done
}
