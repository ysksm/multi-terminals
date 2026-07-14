package session_test

import (
	"testing"
	"time"

	"github.com/ysksm/multi-terminals/core/application/apptest"
	"github.com/ysksm/multi-terminals/core/application/session"
)

// fakePidSession は Pid() を持つフェイク端末セッション。
type fakePidSession struct {
	*apptest.FakeTerminalSession
	pid int
}

func (f *fakePidSession) Pid() int { return f.pid }

func TestSessionPidTailLastOutput(t *testing.T) {
	inner := apptest.NewFakeTerminalSession("p1")
	s := session.NewSession(&fakePidSession{FakeTerminalSession: inner, pid: 4321})
	defer func() { _ = s.Close() }()

	if got := s.Pid(); got != 4321 {
		t.Errorf("Pid() = %d, want 4321", got)
	}
	if !s.LastOutputAt().IsZero() {
		t.Error("LastOutputAt() should be zero before any output")
	}

	// Subscribe してから書き込み、受信を待つことで drain がスクロールバックへ
	// 反映済みであることを保証する。
	_, sub := s.Subscribe()
	before := time.Now()
	_ = inner.Write([]byte("hello "))
	_ = inner.Write([]byte("world"))
	recv(t, sub)
	recv(t, sub)

	if got := string(s.Tail(5)); got != "world" {
		t.Errorf("Tail(5) = %q, want %q", got, "world")
	}
	if got := string(s.Tail(1024)); got != "hello world" {
		t.Errorf("Tail(1024) = %q, want full scrollback", got)
	}
	if s.LastOutputAt().Before(before) {
		t.Errorf("LastOutputAt() = %v, want >= %v", s.LastOutputAt(), before)
	}
}

func TestSessionPidZeroWithoutProvider(t *testing.T) {
	inner := apptest.NewFakeTerminalSession("p2")
	s := session.NewSession(inner)
	defer func() { _ = s.Close() }()
	if got := s.Pid(); got != 0 {
		t.Errorf("Pid() = %d, want 0 for inner session without Pid()", got)
	}
}
