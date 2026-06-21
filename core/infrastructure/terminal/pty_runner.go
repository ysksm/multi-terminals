// Package terminal provides a creack/pty-backed implementation of
// port.TerminalRunner and port.TerminalSession.
package terminal

import (
	"context"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
	"github.com/ysksm/multi-terminals/core/application/port"
)

// Compile-time interface assertions.
var _ port.TerminalSession = (*ptySession)(nil)
var _ port.TerminalRunner = (*Runner)(nil)

// Runner is a port.TerminalRunner that starts PTY-backed shell sessions.
type Runner struct {
	defaultShell string
}

// NewRunner returns a new Runner. The default shell is taken from the
// SHELL environment variable; if empty, /bin/sh is used.
func NewRunner() *Runner {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	return &Runner{defaultShell: shell}
}

// Start launches a new PTY-backed terminal session for the given request.
// A goroutine is started to pump PTY output into the out channel; that
// goroutine closes out and done exactly once when the process exits.
func (r *Runner) Start(ctx context.Context, req port.TerminalStartRequest) (port.TerminalSession, error) {
	shell := req.Shell
	if shell == "" {
		shell = r.defaultShell
	}

	// Honor caller cancellation for the start operation only.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// IMPORTANT: use exec.Command, NOT exec.CommandContext. A terminal session
	// outlives the request that starts it; its lifetime is controlled solely by
	// Close() (and Registry.CloseAll() on server shutdown). Binding the process
	// to the caller's context would kill the shell the moment the originating
	// HTTP request completes.
	cmd := exec.Command(shell)
	if req.Dir != "" {
		cmd.Dir = req.Dir
	}

	f, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}

	// Set window size if provided.
	if req.Cols > 0 || req.Rows > 0 {
		_ = pty.Setsize(f, &pty.Winsize{Cols: req.Cols, Rows: req.Rows})
	}

	s := &ptySession{
		id:        req.SessionID,
		cmd:       cmd,
		f:         f,
		out:       make(chan []byte, 256),
		done:      make(chan struct{}),
		closeKill: make(chan struct{}),
	}

	// Output pump: reads PTY output and forwards to s.out. Responsible for
	// closing s.out and s.done exactly once when the process ends.
	go s.pump()

	return s, nil
}

// ptySession implements port.TerminalSession backed by a creack/pty file.
type ptySession struct {
	id  string
	cmd *exec.Cmd
	f   *os.File

	out       chan []byte
	done      chan struct{}
	closeKill chan struct{} // closed by Close() to signal pump to stop

	writeMu   sync.Mutex // guards f.Write so Close can't race with Write
	killOnce  sync.Once  // ensures we kill the process and close the PTY exactly once
	chanOnce  sync.Once  // ensures out and done are closed exactly once (owned by pump)
}

// pump reads from the PTY file and forwards data to the out channel.
// It terminates when the PTY file returns EOF or an error (which happens
// after the process exits or after Close() closes the PTY file).
// It is the sole owner of chanOnce and is responsible for closing out/done.
func (s *ptySession) pump() {
	buf := make([]byte, 4096)
	for {
		n, err := s.f.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			select {
			case s.out <- chunk:
			case <-s.closeKill:
				// Close() was called while we were blocked sending; drop chunk
				// and exit. The chanOnce below will close the channels.
				goto done
			}
		}
		if err != nil {
			break
		}
	}
done:
	s.chanOnce.Do(func() {
		close(s.out)
		_ = s.cmd.Wait()
		close(s.done)
	})
}

// ID returns the session identifier.
func (s *ptySession) ID() string {
	return s.id
}

// Write sends data to the PTY's standard input.
func (s *ptySession) Write(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.f.Write(data)
	return err
}

// Resize updates the PTY window size.
// It acquires writeMu (the same lock used by Write and Close) so that it
// cannot race with a concurrent Close that calls f.Close().
func (s *ptySession) Resize(cols, rows uint16) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	// If the session has already been closed, f is invalid — return early.
	select {
	case <-s.closeKill:
		return nil
	default:
	}
	return pty.Setsize(s.f, &pty.Winsize{Cols: cols, Rows: rows})
}

// Output returns the read-only output channel.
func (s *ptySession) Output() <-chan []byte {
	return s.out
}

// Done returns the channel that is closed when the session is fully terminated.
func (s *ptySession) Done() <-chan struct{} {
	return s.done
}

// Close terminates the session idempotently. It signals the pump goroutine
// (via closeKill), kills the process, and closes the PTY file descriptor.
// The pump goroutine closes the out and done channels after it observes the
// EOF, so callers should wait on Done() if they need to ensure cleanup.
func (s *ptySession) Close() error {
	var killErr error
	s.killOnce.Do(func() {
		// Signal the pump goroutine first so it can unblock from a channel send.
		close(s.closeKill)

		// Kill and close the PTY under the write mutex so we don't race with Write.
		s.writeMu.Lock()
		if s.cmd.Process != nil {
			killErr = s.cmd.Process.Kill()
		}
		_ = s.f.Close()
		s.writeMu.Unlock()
	})
	return killErr
}
