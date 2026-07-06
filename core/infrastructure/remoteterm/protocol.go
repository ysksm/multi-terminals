// Package remoteterm implements remote terminal execution between two
// multi-terminals instances over WebSocket.
//
// The listening instance ("host") serves EndpointPath (see Handler). The
// connecting instance dials it with Runner, which implements
// port.TerminalRunner: the terminal process (PTY) runs on the host machine
// and its output is streamed back to the caller, so a pane on machine A can
// execute on machine B while being displayed on A.
//
// Wire protocol (all control messages are JSON text frames):
//
//	client → server  {"type":"start","sessionId":..,"dir":..,"shell":..,"cols":..,"rows":..}
//	server → client  {"type":"started"}  or  {"type":"error","error":".."}
//	client → server  {"type":"input","data":"<base64>"} | {"type":"resize","cols":..,"rows":..}
//	server → client  binary frames = raw terminal output
//	server → client  {"type":"exit"} followed by a close frame when the process ends
//
// Input bytes are base64-encoded because raw terminal input is arbitrary
// binary data and JSON strings must be valid UTF-8.
//
// Authentication uses a shared token sent as "Authorization: Bearer <token>".
// The server rejects all connections when no token is configured, so remote
// execution is opt-in.
package remoteterm

// EndpointPath is the HTTP path the remote-terminal WebSocket endpoint is
// served on and the path the dialer connects to.
const EndpointPath = "/api/remote/terminal"

// Control message type values.
const (
	msgStart   = "start"
	msgStarted = "started"
	msgError   = "error"
	msgInput   = "input"
	msgResize  = "resize"
	msgExit    = "exit"
)

// controlMsg is the JSON envelope for all text-frame control messages in both
// directions. Fields are used depending on Type; unused fields are omitted.
type controlMsg struct {
	Type string `json:"type"`

	// type=="start"
	SessionID string `json:"sessionId,omitempty"`
	Dir       string `json:"dir,omitempty"`
	Shell     string `json:"shell,omitempty"`

	// type=="start" / type=="resize"
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`

	// type=="input": base64-encoded raw input bytes
	Data string `json:"data,omitempty"`

	// type=="error"
	Error string `json:"error,omitempty"`
}
