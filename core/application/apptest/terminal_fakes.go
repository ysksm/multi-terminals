package apptest

import (
	"context"
	"sync"

	"github.com/ysksm/multi-terminals/core/application/port"
)

// Compile-time interface assertions.
var _ port.TerminalSession = (*FakeTerminalSession)(nil)
var _ port.TerminalRunner = (*FakeTerminalRunner)(nil)

// FakeTerminalSession is a test implementation of port.TerminalSession.
// Write records received data and echoes it to the output channel so tests can
// observe output. Close is idempotent.
type FakeTerminalSession struct {
	id string

	mu       sync.Mutex
	Writes   [][]byte
	LastCols uint16
	LastRows uint16

	out    chan []byte
	done   chan struct{}
	once   sync.Once
	closed bool // guarded by mu; set before out is closed
}

// NewFakeTerminalSession returns a new FakeTerminalSession with the given id.
func NewFakeTerminalSession(id string) *FakeTerminalSession {
	return &FakeTerminalSession{
		id:   id,
		out:  make(chan []byte, 64),
		done: make(chan struct{}),
	}
}

// ID returns the session ID.
func (s *FakeTerminalSession) ID() string {
	return s.id
}

// Write records the data and echoes it to the output channel.
func (s *FakeTerminalSession) Write(data []byte) error {
	cp := make([]byte, len(data))
	copy(cp, data)

	// A6.3: hold mu across the closed-check AND the channel send so that a
	// concurrent Close() cannot close s.out between the check and the send.
	// Use a non-blocking select so we never block while holding the mutex
	// (avoids a potential deadlock when the buffer is full).
	s.mu.Lock()
	s.Writes = append(s.Writes, cp)
	if !s.closed {
		select {
		case s.out <- cp:
		default:
			// buffer full; drop the echo (rare in tests)
		}
	}
	s.mu.Unlock()

	return nil
}

// Resize records the last resize request.
func (s *FakeTerminalSession) Resize(cols, rows uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastCols = cols
	s.LastRows = rows
	return nil
}

// LastSize returns the last recorded resize values in a concurrency-safe manner.
func (s *FakeTerminalSession) LastSize() (cols, rows uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.LastCols, s.LastRows
}

// Output returns the output channel. It is closed when Close is called.
func (s *FakeTerminalSession) Output() <-chan []byte {
	return s.out
}

// Done returns a channel that is closed when the session is fully terminated.
func (s *FakeTerminalSession) Done() <-chan struct{} {
	return s.done
}

// Close is idempotent: it closes the output and done channels exactly once.
// A6.3: mu is held while setting the closed flag and closing s.out so that
// a concurrent Write cannot send on a closed channel.
func (s *FakeTerminalSession) Close() error {
	s.once.Do(func() {
		s.mu.Lock()
		s.closed = true
		close(s.out)
		s.mu.Unlock()
		close(s.done)
	})
	return nil
}

// FakeTerminalRunner is a test implementation of port.TerminalRunner.
// Each call to Start creates a FakeTerminalSession and records the request.
type FakeTerminalRunner struct {
	mu       sync.Mutex
	Started  []port.TerminalStartRequest
	StartErr error
}

// NewFakeTerminalRunner returns a new FakeTerminalRunner.
func NewFakeTerminalRunner() *FakeTerminalRunner {
	return &FakeTerminalRunner{}
}

// Start records req and returns a new FakeTerminalSession (or StartErr if set).
func (r *FakeTerminalRunner) Start(_ context.Context, req port.TerminalStartRequest) (port.TerminalSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.StartErr != nil {
		return nil, r.StartErr
	}
	r.Started = append(r.Started, req)
	return NewFakeTerminalSession(req.SessionID), nil
}
