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
//	server → client  {"type":"challenge","nonce":"<base64>"}
//	client → server  {"type":"auth","publicKey":"ed25519:<base64>","signature":"<base64>"}
//	client → server  {"type":"start","sessionId":..,"dir":..,"shell":..,"cols":..,"rows":..}
//	server → client  {"type":"started"}  or  {"type":"error","error":".."}
//	client → server  {"type":"input","data":"<base64>"} | {"type":"resize","cols":..,"rows":..}
//	server → client  binary frames = raw terminal output
//	server → client  {"type":"exit"} followed by a close frame when the process ends
//
// Input bytes are base64-encoded because raw terminal input is arbitrary
// binary data and JSON strings must be valid UTF-8.
//
// Authentication is Ed25519 public-key challenge-response: the server sends a
// fresh random nonce, the client signs it with its auto-generated instance
// key (see Identity), and the server verifies the signature against its
// authorized-keys list (see AuthorizedKeys). No secret ever crosses the wire,
// and the server rejects all connections while its authorized list is empty,
// so remote execution is opt-in per client key.
package remoteterm

// EndpointPath is the HTTP path the remote-terminal WebSocket endpoint is
// served on and the path the dialer connects to.
const EndpointPath = "/api/remote/terminal"

// Control message type values.
const (
	msgChallenge = "challenge"
	msgAuth      = "auth"
	msgStart     = "start"
	msgStarted   = "started"
	msgError     = "error"
	msgInput     = "input"
	msgResize    = "resize"
	msgExit      = "exit"
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

	// type=="challenge": base64-encoded random nonce to sign
	Nonce string `json:"nonce,omitempty"`

	// type=="auth": client public key and base64 signature over the nonce
	PublicKey string `json:"publicKey,omitempty"`
	Signature string `json:"signature,omitempty"`
}
