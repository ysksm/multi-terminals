package session

import (
	"sync"

	"github.com/ysksm/multi-terminals/core/application/port"
)

// DefaultScrollbackBytes is the default size of the per-session scrollback ring.
const DefaultScrollbackBytes = 256 * 1024

// Subscription is a live output subscription to a Session.
type Subscription struct {
	ch   chan []byte
	done chan struct{}
}

// C returns the channel delivering live output chunks. It is never closed;
// use Done to detect that the subscription has ended.
func (s *Subscription) C() <-chan []byte { return s.ch }

// Done is closed when the subscription ends (session ended, or this subscriber
// was dropped / unsubscribed).
func (s *Subscription) Done() <-chan struct{} { return s.done }

// Session wraps a port.TerminalSession with a scrollback ring buffer and
// detachable subscribers, so a client can disconnect and later reconnect
// (resume) without losing the running shell or its recent output.
type Session struct {
	inner         port.TerminalSession
	maxScrollback int

	mu         sync.Mutex
	scrollback []byte
	subs       map[*Subscription]struct{}
	ended      bool

	done chan struct{} // closed when the underlying session has ended
}

// NewSession wraps inner with the default scrollback size and starts draining.
func NewSession(inner port.TerminalSession) *Session {
	return NewSessionWithScrollback(inner, DefaultScrollbackBytes)
}

// NewSessionWithScrollback wraps inner with a custom scrollback size.
func NewSessionWithScrollback(inner port.TerminalSession, maxScrollback int) *Session {
	s := &Session{
		inner:         inner,
		maxScrollback: maxScrollback,
		subs:          make(map[*Subscription]struct{}),
		done:          make(chan struct{}),
	}
	go s.drain()
	return s
}

func (s *Session) drain() {
	for chunk := range s.inner.Output() {
		s.mu.Lock()
		s.appendScrollback(chunk)
		for sub := range s.subs {
			select {
			case sub.ch <- chunk:
			default:
				// Slow subscriber: drop it so the PTY never stalls. The client
				// reconnects and replays scrollback (which already contains this
				// chunk), so nothing is lost visually.
				delete(s.subs, sub)
				close(sub.done)
			}
		}
		s.mu.Unlock()
	}
	// inner output closed -> the shell exited.
	s.mu.Lock()
	s.ended = true
	for sub := range s.subs {
		delete(s.subs, sub)
		close(sub.done)
	}
	s.mu.Unlock()
	close(s.done)
}

func (s *Session) appendScrollback(chunk []byte) {
	s.scrollback = append(s.scrollback, chunk...)
	if len(s.scrollback) > s.maxScrollback {
		excess := len(s.scrollback) - s.maxScrollback
		s.scrollback = append(s.scrollback[:0], s.scrollback[excess:]...)
	}
}

// Subscribe returns a snapshot of the current scrollback and a live
// Subscription. If the session has already ended, the Subscription is already
// done (its Done channel is closed).
func (s *Session) Subscribe() (snapshot []byte, sub *Subscription) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := make([]byte, len(s.scrollback))
	copy(snap, s.scrollback)
	sub = &Subscription{ch: make(chan []byte, 1024), done: make(chan struct{})}
	if s.ended {
		close(sub.done)
		return snap, sub
	}
	s.subs[sub] = struct{}{}
	return snap, sub
}

// Unsubscribe removes sub. Idempotent and safe against a concurrent drop.
func (s *Session) Unsubscribe(sub *Subscription) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subs[sub]; ok {
		delete(s.subs, sub)
		close(sub.done)
	}
}

// ID, Write, Resize delegate to the wrapped session.
func (s *Session) ID() string                    { return s.inner.ID() }
func (s *Session) Write(data []byte) error        { return s.inner.Write(data) }
func (s *Session) Resize(cols, rows uint16) error { return s.inner.Resize(cols, rows) }

// Done is closed when the underlying session has fully ended.
func (s *Session) Done() <-chan struct{} { return s.done }

// Close terminates the underlying session. The drain goroutine closes Done and
// all subscriptions once the PTY output channel closes.
func (s *Session) Close() error { return s.inner.Close() }
