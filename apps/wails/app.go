// Package main is the Wails desktop adapter for multi-terminals. It reuses the
// web app's mux (served in-process via the Wails AssetServer) for REST/SPA and
// exposes terminal I/O over native Go<->JS bindings + runtime events.
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/ysksm/multi-terminals/apps/web"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/application/session"
)

// App holds the wired dependencies and bridges terminal I/O to the frontend.
type App struct {
	deps web.Deps

	// emit publishes an event to the frontend. It is set in startup to wrap
	// wails runtime.EventsEmit; tests replace it to capture events.
	emit func(event string, optionalData ...interface{})

	mu   sync.Mutex
	subs map[string]*session.Subscription
}

// NewApp builds an App from wired web dependencies.
// emit defaults to a no-op so goroutines started before startup() never
// dereference a nil function pointer.
func NewApp(deps web.Deps) *App {
	return &App{
		deps:  deps,
		subs:  make(map[string]*session.Subscription),
		emit:  func(string, ...interface{}) {},
	}
}

// startup captures the Wails context and installs the real emit function.
func (a *App) startup(ctx context.Context) {
	a.emit = func(event string, data ...interface{}) {
		wruntime.EventsEmit(ctx, event, data...)
	}
}

// shutdown closes all live PTY sessions so child processes are not orphaned.
func (a *App) shutdown(_ context.Context) {
	a.deps.Registry.CloseAll()
}

// paneEvent returns the data event name for a pane.
func paneEvent(paneID string) string { return "pane:" + paneID }

// PaneSubscribe attaches to the live session for paneID, emits the scrollback
// snapshot, then streams live output as base64 strings via runtime events.
func (a *App) PaneSubscribe(paneID string) error {
	sess, ok := a.deps.Registry.Get(paneID)
	if !ok {
		return fmt.Errorf("pane %s: session not found", paneID)
	}
	snapshot, sub := sess.Subscribe()

	a.mu.Lock()
	if old, ok := a.subs[paneID]; ok {
		sess.Unsubscribe(old)
	}
	a.subs[paneID] = sub
	a.mu.Unlock()

	if len(snapshot) > 0 {
		a.emit(paneEvent(paneID), base64.StdEncoding.EncodeToString(snapshot))
	}
	go a.pump(paneID, sub)
	return nil
}

func (a *App) pump(paneID string, sub *session.Subscription) {
	for {
		select {
		case chunk := <-sub.C():
			a.emit(paneEvent(paneID), base64.StdEncoding.EncodeToString(chunk))
		case <-sub.Done():
			a.mu.Lock()
			current := a.subs[paneID] == sub
			if current {
				delete(a.subs, paneID)
			}
			a.mu.Unlock()
			if current {
				a.emit(paneEvent(paneID) + ":done")
			}
			return
		}
	}
}

// PaneUnsubscribe detaches the active subscription for paneID (idempotent).
func (a *App) PaneUnsubscribe(paneID string) {
	a.mu.Lock()
	sub, ok := a.subs[paneID]
	delete(a.subs, paneID)
	a.mu.Unlock()
	if !ok {
		return
	}
	if sess, ok := a.deps.Registry.Get(paneID); ok {
		sess.Unsubscribe(sub)
	}
}

// PaneWrite sends input bytes (UTF-8 from JS) to the pane's terminal.
func (a *App) PaneWrite(paneID string, data string) error {
	return a.deps.Write.Handle(context.Background(), command.WriteToPaneCommand{
		PaneID: paneID,
		Data:   []byte(data),
	})
}

// PaneResize updates the pane terminal window size.
func (a *App) PaneResize(paneID string, cols uint16, rows uint16) error {
	return a.deps.Resize.Handle(context.Background(), command.ResizePaneCommand{
		PaneID: paneID,
		Cols:   cols,
		Rows:   rows,
	})
}
