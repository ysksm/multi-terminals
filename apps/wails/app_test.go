package main

import (
	"encoding/base64"
	"sync"
	"testing"
	"time"

	"github.com/ysksm/multi-terminals/apps/web"
	"github.com/ysksm/multi-terminals/core/application/session"
)

// fakeTerm is a port.TerminalSession used to drive App tests without a real PTY.
type fakeTerm struct {
	out    chan []byte
	mu     sync.Mutex
	writes [][]byte
	cols   uint16
	rows   uint16
	done   chan struct{}
}

func newFakeTerm() *fakeTerm {
	return &fakeTerm{out: make(chan []byte, 8), done: make(chan struct{})}
}

func (f *fakeTerm) ID() string { return "fake" }
func (f *fakeTerm) Write(d []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, append([]byte(nil), d...))
	return nil
}
func (f *fakeTerm) Resize(c, r uint16) error { f.cols, f.rows = c, r; return nil }
func (f *fakeTerm) Output() <-chan []byte    { return f.out }
func (f *fakeTerm) Done() <-chan struct{}    { return f.done }
func (f *fakeTerm) Close() error {
	close(f.out)
	close(f.done)
	return nil
}

// newAppWithSession builds an App whose registry holds a single live session
// backed by ft, with a capturing emit. Returns the app and a thread-safe
// reader for emitted events.
func newAppWithSession(t *testing.T, paneID string, ft *fakeTerm) (*App, func() []emitted) {
	t.Helper()
	reg := session.NewRegistry()
	reg.Add(paneID, session.NewSession(ft))
	app := NewApp(web.Deps{Registry: reg})

	var mu sync.Mutex
	var events []emitted
	app.emit = func(event string, data ...interface{}) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, emitted{event: event, data: data})
	}
	read := func() []emitted {
		mu.Lock()
		defer mu.Unlock()
		return append([]emitted(nil), events...)
	}
	return app, read
}

type emitted struct {
	event string
	data  []interface{}
}

func TestPaneSubscribe_UnknownSession_ReturnsError(t *testing.T) {
	app := NewApp(web.Deps{Registry: session.NewRegistry()})
	app.emit = func(string, ...interface{}) {}
	if err := app.PaneSubscribe("nope"); err == nil {
		t.Fatal("expected error for unknown session, got nil")
	}
}

func TestPaneSubscribe_StreamsOutputAsBase64(t *testing.T) {
	ft := newFakeTerm()
	app, read := newAppWithSession(t, "p1", ft)

	if err := app.PaneSubscribe("p1"); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	ft.out <- []byte("hello")

	want := base64.StdEncoding.EncodeToString([]byte("hello"))
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("did not observe emitted chunk; events=%v", read())
		default:
		}
		for _, e := range read() {
			if e.event == "pane:p1" && len(e.data) == 1 {
				if s, _ := e.data[0].(string); s == want {
					return
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestPaneSubscribe_EmitsDoneOnClose(t *testing.T) {
	ft := newFakeTerm()
	app, read := newAppWithSession(t, "p2", ft)
	if err := app.PaneSubscribe("p2"); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	_ = ft.Close()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("did not observe done event; events=%v", read())
		default:
		}
		for _, e := range read() {
			if e.event == "pane:p2:done" {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
}
