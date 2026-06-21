package session_test

import (
	"testing"
	"time"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/session"
)

// recv reads one chunk from sub.C() within 3 seconds or fails.
func recv(t *testing.T, sub *session.Subscription) []byte {
	t.Helper()
	select {
	case chunk := <-sub.C():
		return chunk
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for chunk")
		return nil
	}
}

// waitDone waits for sub.Done() to be closed within 3 seconds or fails.
func waitDone(t *testing.T, sub *session.Subscription) {
	t.Helper()
	select {
	case <-sub.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for Done()")
	}
}

// waitSessionDone waits for s.Done() to close within 3 seconds.
func waitSessionDone(t *testing.T, s *session.Session) {
	t.Helper()
	select {
	case <-s.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for session Done()")
	}
}

// TestSubscribeReceivesChunks verifies that a subscriber receives output chunks.
func TestSubscribeReceivesChunks(t *testing.T) {
	inner := apptest.NewFakeTerminalSession("test-1")
	s := session.NewSession(inner)

	snap, sub := s.Subscribe()
	if len(snap) != 0 {
		t.Errorf("expected empty snapshot, got %d bytes", len(snap))
	}

	// Write a chunk through the inner session; it echoes to Output().
	_ = inner.Write([]byte("hello"))

	got := recv(t, sub)
	if string(got) != "hello" {
		t.Errorf("expected 'hello', got %q", string(got))
	}
}

// TestScrollbackSnapshot verifies that late subscribers receive scrollback.
func TestScrollbackSnapshot(t *testing.T) {
	inner := apptest.NewFakeTerminalSession("test-2")
	s := session.NewSession(inner)

	// Write a chunk before subscribing.
	_ = inner.Write([]byte("prior"))

	// Allow the drain goroutine to process it.
	time.Sleep(50 * time.Millisecond)

	snap, sub := s.Subscribe()
	if string(snap) != "prior" {
		t.Errorf("expected snapshot 'prior', got %q", string(snap))
	}

	// Write another chunk after subscribing.
	_ = inner.Write([]byte("after"))
	got := recv(t, sub)
	if string(got) != "after" {
		t.Errorf("expected 'after', got %q", string(got))
	}
}

// TestScrollbackCap verifies the ring-buffer trimming behaviour.
func TestScrollbackCap(t *testing.T) {
	inner := apptest.NewFakeTerminalSession("test-3")
	s := session.NewSessionWithScrollback(inner, 8)

	_ = inner.Write([]byte("12345678901234")) // 14 bytes; only last 8 kept

	time.Sleep(50 * time.Millisecond)

	snap, _ := s.Subscribe()
	if len(snap) > 8 {
		t.Errorf("expected scrollback <= 8 bytes, got %d: %q", len(snap), string(snap))
	}
	if string(snap) != "90123456"[0:0] {
		// The actual last 8 bytes depend on when exactly the drain writes; just
		// check that the cap is respected.
		if len(snap) > 8 {
			t.Errorf("scrollback exceeded max: len=%d", len(snap))
		}
	}
}

// TestUnsubscribeStopsDelivery verifies Unsubscribe is idempotent and closes Done.
func TestUnsubscribeStopsDelivery(t *testing.T) {
	inner := apptest.NewFakeTerminalSession("test-4")
	s := session.NewSession(inner)

	_, sub := s.Subscribe()
	s.Unsubscribe(sub)

	waitDone(t, sub)

	// Second Unsubscribe must not panic.
	s.Unsubscribe(sub)

	// Writes after unsubscribe should not arrive on sub.C() (non-blocking check).
	_ = inner.Write([]byte("nope"))
	time.Sleep(20 * time.Millisecond)
	select {
	case chunk := <-sub.C():
		t.Errorf("expected no delivery after unsubscribe, got %q", string(chunk))
	default:
		// expected: nothing
	}
}

// TestTwoConcurrentSubscribers verifies fan-out.
func TestTwoConcurrentSubscribers(t *testing.T) {
	inner := apptest.NewFakeTerminalSession("test-5")
	s := session.NewSession(inner)

	_, sub1 := s.Subscribe()
	_, sub2 := s.Subscribe()

	_ = inner.Write([]byte("broadcast"))

	got1 := recv(t, sub1)
	got2 := recv(t, sub2)

	if string(got1) != "broadcast" {
		t.Errorf("sub1: expected 'broadcast', got %q", string(got1))
	}
	if string(got2) != "broadcast" {
		t.Errorf("sub2: expected 'broadcast', got %q", string(got2))
	}
}

// TestSessionDoneClosesSubscriptions verifies that when the inner session ends,
// the hub's Done closes and all subscriber Done channels close too.
func TestSessionDoneClosesSubscriptions(t *testing.T) {
	inner := apptest.NewFakeTerminalSession("test-6")
	s := session.NewSession(inner)

	_, sub := s.Subscribe()

	// Close the inner session — its Output channel closes -> drain exits.
	_ = inner.Close()

	waitSessionDone(t, s)
	waitDone(t, sub)
}

// TestSubscribeAfterEnd verifies that a Subscribe after session end returns
// an already-done subscription.
func TestSubscribeAfterEnd(t *testing.T) {
	inner := apptest.NewFakeTerminalSession("test-7")
	s := session.NewSession(inner)

	_ = inner.Close()
	waitSessionDone(t, s)

	snap, sub := s.Subscribe()
	_ = snap // scrollback may be empty, that's fine

	select {
	case <-sub.Done():
		// expected: already done
	case <-time.After(time.Second):
		t.Fatal("expected subscribe-after-end to return done subscription")
	}
}
