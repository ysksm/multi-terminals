package terminal_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/ysksm/multi-terminals/core/application/port"
	"github.com/ysksm/multi-terminals/core/infrastructure/terminal"
)

// gatherOutput reads from the session's Output() channel until either the
// provided check function returns true, or the timeout is reached.
// It returns the accumulated bytes seen so far.
func gatherOutput(t *testing.T, sess port.TerminalSession, check func([]byte) bool, timeout time.Duration) []byte {
	t.Helper()
	deadline := time.After(timeout)
	var buf []byte
	for {
		select {
		case chunk, ok := <-sess.Output():
			if !ok {
				return buf
			}
			buf = append(buf, chunk...)
			if check(buf) {
				return buf
			}
		case <-deadline:
			t.Logf("gatherOutput timeout; accumulated %d bytes: %q", len(buf), buf)
			return buf
		}
	}
}

// TestRunnerEchoRoundTrip starts a real shell, sends an echo command, and
// asserts the output contains the marker. It also verifies Done() is closed
// after Close(). The shell and command are OS-specific (see testshell_*.go).
func TestRunnerEchoRoundTrip(t *testing.T) {
	r := terminal.NewRunner()
	ctx := context.Background()

	sess, err := r.Start(ctx, port.TerminalStartRequest{
		SessionID: "s1",
		Dir:       t.TempDir(),
		Shell:     testShell,
		Cols:      80,
		Rows:      24,
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if got := sess.ID(); got != "s1" {
		t.Errorf("ID() = %q, want %q", got, "s1")
	}

	// Give the shell a moment to initialise, then send the echo command.
	time.Sleep(100 * time.Millisecond)
	if err := sess.Write(echoLine("hello123")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	needle := []byte("hello123")
	out := gatherOutput(t, sess, func(b []byte) bool { return bytes.Contains(b, needle) }, 5*time.Second)

	if !bytes.Contains(out, needle) {
		t.Errorf("output does not contain %q; got %q", needle, out)
	}

	// Close the session and verify Done() is closed.
	if err := sess.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
	// Close should be idempotent.
	if err := sess.Close(); err != nil {
		t.Errorf("second Close() returned error: %v", err)
	}

	select {
	case <-sess.Done():
		// good
	case <-time.After(3 * time.Second):
		t.Error("Done() was not closed after Close()")
	}
}

// TestRunnerSetsDir verifies that the shell's working directory is set to
// the requested Dir. We write "pwd\n" and look for the basename of the tmpdir
// in the output (to tolerate macOS /private symlink aliasing).
func TestRunnerSetsDir(t *testing.T) {
	r := terminal.NewRunner()
	ctx := context.Background()
	dir := t.TempDir()

	sess, err := r.Start(ctx, port.TerminalStartRequest{
		SessionID: "s2",
		Dir:       dir,
		Shell:     testShell,
		Cols:      80,
		Rows:      24,
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer sess.Close()

	time.Sleep(100 * time.Millisecond)
	if err := sess.Write(pwdLine()); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	base := filepath.Base(dir)
	needle := []byte(base)
	out := gatherOutput(t, sess, func(b []byte) bool { return bytes.Contains(b, needle) }, 5*time.Second)

	if !bytes.Contains(out, needle) {
		t.Errorf("output does not contain dir basename %q; got %q", base, out)
	}
}
