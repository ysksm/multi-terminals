package port

import "context"

// TerminalStartRequest holds parameters to start a new terminal session.
// RemoteHost, when non-empty, requests that the terminal run on that remote
// multi-terminals instance instead of the local machine; runners that do not
// support remote execution must return an error in that case.
type TerminalStartRequest struct {
	SessionID  string
	Dir        string
	Shell      string
	Cols       uint16
	Rows       uint16
	RemoteHost string
}

// TerminalSession represents a live terminal process session.
// Output() returns a channel of output chunks; it is closed when the process
// exits or Close is called. Done() is closed when the session is fully
// terminated (after output channel close and process wait).
type TerminalSession interface {
	// ID returns the unique session identifier.
	ID() string

	// Write sends data to the terminal's standard input.
	Write(data []byte) error

	// Resize updates the terminal window size.
	Resize(cols, rows uint16) error

	// Output returns a read-only channel of output byte chunks.
	// The channel is closed exactly once when the process exits or Close is called.
	Output() <-chan []byte

	// Done returns a channel that is closed when the session is fully terminated.
	Done() <-chan struct{}

	// Close terminates the session. Close is idempotent and safe to call multiple times.
	Close() error
}

// TerminalRunner starts new terminal sessions.
type TerminalRunner interface {
	Start(ctx context.Context, req TerminalStartRequest) (TerminalSession, error)
}
