# Wails デスクトップ版 + 全アプリ一括ビルド Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 既存のコア/UI を再利用した Wails デスクトップ版を追加し、Windows/macOS 両対応でビルドできるようにし、全アプリを一括ビルドするスクリプトと CI を整備する。

**Architecture:** Wails アプリは `apps/web` の `NewMux` と `apps/web/webui` の埋め込み UI を `AssetServer.Handler` で在プロセス配信（REST/SPA はポート不要）。端末 I/O のみ Go↔JS バインディング + `runtime.EventsEmit` でネイティブ化。フロントは Wails 環境を自動判定し、デスクトップではバインディング、ブラウザでは従来の WebSocket を使う。

**Tech Stack:** Go 1.26, Wails v2, Svelte 5 + Vite + xterm.js, Bash, GitHub Actions。

## Global Constraints

- Go 1.26（`go.mod` の `go 1.26` を維持）。
- 追加 Go 依存は Wails v2 のみ（`github.com/wailsapp/wails/v2`）。
- フロントの同一オリジン契約を壊さない：web 版（ブラウザ）の挙動・UI は不変。
- Wails はクロスコンパイル不可：ローカルスクリプトは実行 OS 向けのみビルドし、他 OS は警告してスキップ。両 OS は GitHub Actions のネイティブランナーでビルド。
- 端末出力は任意バイト列を含むため、Go→JS のチャンクは base64 文字列で受け渡す。端末入力は web 版同様 UTF-8 文字列で受け渡す（base64 不要）。
- イベント名は `pane:<paneID>`（データ）と `pane:<paneID>:done`（終了）で統一。
- 署名/公証・自動アップデータ・Linux 対応はスコープ外（YAGNI）。

確定済みの型シグネチャ（コア、変更しない）:
- `web.BuildDeps(baseDir string) (web.Deps, error)` / `web.NewMux(d web.Deps) *http.ServeMux` / `webui.Handler() http.Handler`
- `web.Deps` フィールド: `Write *command.WriteToPaneHandler`, `Resize *command.ResizePaneHandler`, `Registry *session.Registry` ほか
- `session.Registry`: `Get(id string) (*session.Session, bool)`, `Add(id string, s *session.Session)`, `CloseAll()`
- `session.Session`: `Subscribe() (snapshot []byte, sub *session.Subscription)`, `Unsubscribe(sub *session.Subscription)`
- `session.Subscription`: `C() <-chan []byte`, `Done() <-chan struct{}`
- `session.NewSession(inner port.TerminalSession) *session.Session`
- `port.TerminalSession`: `ID() string`, `Write([]byte) error`, `Resize(uint16, uint16) error`, `Output() <-chan []byte`, `Done() <-chan struct{}`, `Close() error`
- `command.WriteToPaneCommand{PaneID string; Data []byte}` / `command.ResizePaneCommand{PaneID string; Cols, Rows uint16}`
- ハンドラ: `(*command.WriteToPaneHandler).Handle(ctx, cmd) error` / `(*command.ResizePaneHandler).Handle(ctx, cmd) error`

## File Structure

- `apps/wails/app.go`（新規）— `App` 構造体と端末バインディング。GUI 非依存（`emit` シームで `runtime.EventsEmit` を注入）。
- `apps/wails/app_test.go`（新規）— `App` のユニットテスト（フェイク端末セッション使用）。
- `apps/wails/main.go`（新規）— `wails.Run`。`AssetServer.Handler` に mux、`Bind` に App。
- `apps/wails/wails.json`（新規）— Wails プロジェクト設定（`frontend:dir = ../../frontend`）。
- `frontend/src/lib/termTransport.js`（新規）— 端末転送の抽象（desktop=bindings / browser=WS）。
- `frontend/src/lib/termTransport.node.test.mjs`（新規）— 純粋ヘルパ（URL/判定）の node テスト。
- `frontend/src/lib/Terminal.svelte`（変更）— 直接 WS をやめ `connectPane()` 経由へ。
- `scripts/build-all.sh`（新規）— 全アプリ一括ビルド。
- `.github/workflows/build.yml`（新規）— macOS/Windows マトリクスビルド。
- `README.md` / `release/NOTES.md`（変更）— デスクトップ版の起動・注意を追記。

---

### Task 1: Wails App 構造体と端末バインディング（GUI 非依存・TDD）

**Files:**
- Create: `apps/wails/app.go`
- Test: `apps/wails/app_test.go`
- Modify: `go.mod` / `go.sum`（`go get` で Wails 追加）

**Interfaces:**
- Consumes: `web.Deps`（`.Registry`, `.Write`, `.Resize`）、`session.*`、`command.*`、`port.TerminalSession`。
- Produces:
  - `func NewApp(deps web.Deps) *App`
  - `func (a *App) PaneSubscribe(paneID string) error`
  - `func (a *App) PaneUnsubscribe(paneID string)`
  - `func (a *App) PaneWrite(paneID string, data string) error`
  - `func (a *App) PaneResize(paneID string, cols uint16, rows uint16) error`
  - `func (a *App) startup(ctx context.Context)` / `func (a *App) shutdown(ctx context.Context)`
  - `App.emit func(event string, optionalData ...interface{})`（テスト差し替え用シーム）

- [ ] **Step 1: Wails 依存を追加**

Run:
```bash
go get github.com/wailsapp/wails/v2@latest
```
Expected: `go.mod` に `github.com/wailsapp/wails/v2` が追加される（ダウンロード成功）。

- [ ] **Step 2: 失敗するテストを書く**

Create `apps/wails/app_test.go`:
```go
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
```

- [ ] **Step 3: テストを実行して失敗を確認**

Run: `go test ./apps/wails/ -run TestPane -v`
Expected: FAIL（`NewApp`/`App` 未定義によるコンパイルエラー）。

- [ ] **Step 4: 最小実装を書く**

Create `apps/wails/app.go`:
```go
// Package main is the Wails desktop adapter for multi-terminals. It reuses the
// web app's mux (served in-process via the Wails AssetServer) for REST/SPA and
// exposes terminal I/O over native Go<->JS bindings + runtime events.
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"

	"github.com/ysksm/multi-terminals/apps/web"
	"github.com/ysksm/multi-terminals/core/application/command"
	"github.com/ysksm/multi-terminals/core/application/session"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
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
func NewApp(deps web.Deps) *App {
	return &App{deps: deps, subs: make(map[string]*session.Subscription)}
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
			a.emit(paneEvent(paneID) + ":done")
			a.mu.Lock()
			if a.subs[paneID] == sub {
				delete(a.subs, paneID)
			}
			a.mu.Unlock()
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
```

Note: the two write/resize tests are not included above because they require
wired handlers; PaneWrite/Resize are covered by web's existing handler tests.
The subscribe/stream/done paths are the Wails-specific logic and are tested here.

- [ ] **Step 5: テストを実行して成功を確認**

Run: `go test ./apps/wails/ -run TestPane -v`
Expected: PASS（3 テスト）。

- [ ] **Step 6: go vet と tidy**

Run:
```bash
go mod tidy
go vet ./apps/wails/
```
Expected: エラーなし。

- [ ] **Step 7: コミット**

```bash
git add apps/wails/app.go apps/wails/app_test.go go.mod go.sum
git commit -m "feat(wails): add App with terminal bindings (subscribe/stream/write/resize)"
```

---

### Task 2: Wails エントリポイント（main.go + wails.json）

**Files:**
- Create: `apps/wails/main.go`
- Create: `apps/wails/wails.json`

**Interfaces:**
- Consumes: `NewApp`, `App.startup`, `App.shutdown`（Task 1）、`web.BuildDeps`, `web.NewMux`, `webui.Handler`。
- Produces: ビルド可能な `apps/wails` バイナリ（`go build ./apps/wails`）。

- [ ] **Step 1: main.go を書く**

Create `apps/wails/main.go`:
```go
package main

import (
	"log"
	"os"

	"github.com/ysksm/multi-terminals/apps/web"
	"github.com/ysksm/multi-terminals/apps/web/webui"
	"github.com/ysksm/multi-terminals/core/infrastructure/jsonstore"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

func main() {
	baseDir := os.Getenv("MULTI_TERMINALS_DIR")
	if baseDir == "" {
		var err error
		baseDir, err = jsonstore.DefaultBaseDir()
		if err != nil {
			log.Fatalf("multi-terminals (wails): default base dir: %v", err)
		}
	}

	deps, err := web.BuildDeps(baseDir)
	if err != nil {
		log.Fatalf("multi-terminals (wails): build deps: %v", err)
	}

	// Reuse the web mux for REST + embedded SPA, served in-process by Wails.
	mux := web.NewMux(deps)
	mux.Handle("/", webui.Handler())

	app := NewApp(deps)

	if err := wails.Run(&options.App{
		Title:  "multi-terminals",
		Width:  1200,
		Height: 800,
		AssetServer: &assetserver.Options{
			Handler: mux,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
	}); err != nil {
		log.Fatalf("multi-terminals (wails): run: %v", err)
	}
}
```

- [ ] **Step 2: wails.json を書く**

Create `apps/wails/wails.json`:
```json
{
  "$schema": "https://wails.io/schemas/config.v2.json",
  "name": "multi-terminals",
  "outputfilename": "multi-terminals",
  "frontend:dir": "../../frontend",
  "frontend:install": "npm install",
  "frontend:build": "npm run build",
  "author": {
    "name": "ysksm"
  }
}
```

- [ ] **Step 3: フロントを埋め込み用に用意してからコンパイル確認**

Run:
```bash
(cd frontend && npm install && npm run build)
rm -rf apps/web/webui/dist && mkdir -p apps/web/webui/dist && touch apps/web/webui/dist/.gitkeep
cp -R frontend/dist/. apps/web/webui/dist/
go build ./apps/wails
```
Expected: `apps/wails` バイナリ生成（カレントに `wails` という実行ファイル）。コンパイル/リンク成功。
（注: macOS は CGO + WebKit、Windows は WebView2 が必要。ネイティブ環境前提。）

- [ ] **Step 4: 後片付け（生成バイナリを削除）**

Run: `rm -f wails`
Expected: ルートの一時バイナリ削除。

- [ ] **Step 5: コミット**

```bash
git add apps/wails/main.go apps/wails/wails.json
git commit -m "feat(wails): wire wails.Run with AssetServer mux and App bindings"
```

---

### Task 3: フロントエンド転送層（両対応・自動判定）

**Files:**
- Create: `frontend/src/lib/termTransport.js`
- Create: `frontend/src/lib/termTransport.node.test.mjs`
- Modify: `frontend/src/lib/Terminal.svelte`

**Interfaces:**
- Consumes: デスクトップ時 `window.go.main.App.{PaneSubscribe,PaneUnsubscribe,PaneWrite,PaneResize}` と `window.runtime.{EventsOn,EventsOff}`（Task 1 の App メソッド／Wails ランタイム）。
- Produces:
  - `isDesktop(): boolean`
  - `paneWsURL(location, paneId): string`
  - `b64ToBytes(b64: string): Uint8Array`
  - `connectPane(paneId, { onData, onClose }): { send(data), resize(cols, rows), close() }`

- [ ] **Step 1: 失敗する純粋ヘルパのテストを書く**

Create `frontend/src/lib/termTransport.node.test.mjs`:
```js
import assert from 'node:assert'
import { paneWsURL, b64ToBytes, isDesktop } from './termTransport.js'

// paneWsURL: ws for http
assert.equal(
  paneWsURL({ protocol: 'http:', host: 'localhost:8080' }, 'abc'),
  'ws://localhost:8080/api/panes/abc/io',
)
// paneWsURL: wss for https
assert.equal(
  paneWsURL({ protocol: 'https:', host: 'example.com' }, 'p1'),
  'wss://example.com/api/panes/p1/io',
)
// b64ToBytes round-trip
const bytes = b64ToBytes(Buffer.from('hello').toString('base64'))
assert.deepEqual(Array.from(bytes), Array.from(new TextEncoder().encode('hello')))
// isDesktop is false without window globals
assert.equal(isDesktop(), false)

console.log('termTransport helpers: OK')
```

- [ ] **Step 2: テストを実行して失敗を確認**

Run: `node frontend/src/lib/termTransport.node.test.mjs`
Expected: FAIL（`Cannot find module './termTransport.js'` または export 未定義）。

- [ ] **Step 3: termTransport.js を実装**

Create `frontend/src/lib/termTransport.js`:
```js
// 端末転送の抽象。Wails デスクトップ環境ではネイティブバインディング
// (window.go + runtime events)、ブラウザでは従来の WebSocket を使う。

export function isDesktop() {
  return (
    typeof window !== 'undefined' &&
    !!window.runtime &&
    !!window.go &&
    !!window.go.main &&
    !!window.go.main.App
  )
}

export function paneWsURL(location, paneId) {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${location.host}/api/panes/${paneId}/io`
}

export function b64ToBytes(b64) {
  const bin = atob(b64)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out
}

// connectPane は端末1ペイン分の双方向接続を確立し、
// { send, resize, close } を返す。
//   onData(Uint8Array|string): 出力受信
//   onClose(): セッション終了/切断
export function connectPane(paneId, { onData, onClose }) {
  if (isDesktop()) {
    return connectDesktop(paneId, { onData, onClose })
  }
  return connectBrowser(paneId, { onData, onClose })
}

function connectDesktop(paneId, { onData, onClose }) {
  const App = window.go.main.App
  const dataEvent = `pane:${paneId}`
  const doneEvent = `pane:${paneId}:done`

  window.runtime.EventsOn(dataEvent, (b64) => onData(b64ToBytes(b64)))
  window.runtime.EventsOn(doneEvent, () => onClose && onClose())

  // 購読開始。失敗時は onClose で通知。
  Promise.resolve(App.PaneSubscribe(paneId)).catch(() => onClose && onClose())

  return {
    send(data) {
      App.PaneWrite(paneId, data)
    },
    resize(cols, rows) {
      App.PaneResize(paneId, cols, rows)
    },
    close() {
      window.runtime.EventsOff(dataEvent)
      window.runtime.EventsOff(doneEvent)
      App.PaneUnsubscribe(paneId)
    },
  }
}

function connectBrowser(paneId, { onData, onClose }) {
  const ws = new WebSocket(paneWsURL(window.location, paneId))
  ws.binaryType = 'arraybuffer'

  ws.onmessage = (ev) => {
    if (ev.data instanceof ArrayBuffer) onData(new Uint8Array(ev.data))
    else if (typeof ev.data === 'string') onData(ev.data)
  }
  ws.onclose = () => onClose && onClose()

  return {
    send(data) {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'input', data }))
      }
    },
    resize(cols, rows) {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols, rows }))
      }
    },
    close() {
      ws.close()
    },
    _ws: ws, // Terminal.svelte が onopen で初回リサイズするために参照
  }
}
```

- [ ] **Step 4: テストを実行して成功を確認**

Run: `node frontend/src/lib/termTransport.node.test.mjs`
Expected: PASS（`termTransport helpers: OK`）。

- [ ] **Step 5: Terminal.svelte を connectPane 経由に変更**

Replace the entire `<script>` block of `frontend/src/lib/Terminal.svelte` with:
```svelte
<script>
  import { onMount } from 'svelte'
  import { Terminal } from '@xterm/xterm'
  import { FitAddon } from '@xterm/addon-fit'
  import '@xterm/xterm/css/xterm.css'
  import { connectPane } from './termTransport.js'

  // paneId が設定されると接続してライブ端末を表示する。
  let { paneId } = $props()

  let host
  let term
  let fit
  let conn
  let status = $state('connecting')

  function connect(id) {
    term = new Terminal({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: 'Menlo, Monaco, "Courier New", monospace',
      theme: { background: '#1e1e1e' },
    })
    fit = new FitAddon()
    term.loadAddon(fit)
    term.open(host)
    fit.fit()

    conn = connectPane(id, {
      onData: (data) => term.write(data),
      onClose: () => {
        status = 'closed'
        term?.write('\r\n\x1b[33m[session closed]\x1b[0m\r\n')
      },
    })

    // ブラウザ(WS)では onopen を待って初回リサイズ。デスクトップは即送れる。
    if (conn._ws) {
      conn._ws.addEventListener('open', () => {
        status = 'connected'
        sendResize()
      })
    } else {
      status = 'connected'
      sendResize()
    }

    term.onData((data) => conn.send(data))

    const ro = new ResizeObserver(() => {
      try {
        fit.fit()
        sendResize()
      } catch {
        // 端末未初期化時などは無視
      }
    })
    ro.observe(host)
    return ro
  }

  function sendResize() {
    if (conn && term) conn.resize(term.cols, term.rows)
  }

  onMount(() => {
    let ro
    if (paneId) ro = connect(paneId)
    return () => {
      ro?.disconnect()
      conn?.close()
      term?.dispose()
    }
  })
</script>
```
（`<div class="term-wrap">…</div>` 以降のマークアップと `<style>` は変更しない。）

- [ ] **Step 6: フロントビルドで回帰がないことを確認**

Run: `(cd frontend && npm run build)`
Expected: ビルド成功（エラーなし）。

- [ ] **Step 7: web 版スモーク（ブラウザ経路が壊れていないこと）**

Run:
```bash
rm -rf apps/web/webui/dist && mkdir -p apps/web/webui/dist && touch apps/web/webui/dist/.gitkeep
cp -R frontend/dist/. apps/web/webui/dist/
go build -o bin/multi-terminals ./apps/web/cmd
PORT=18120 ./bin/multi-terminals >/tmp/mt-smoke.log 2>&1 &
sleep 2
curl -s -o /dev/null -w "GET / -> %{http_code}\n" http://localhost:18120/
curl -s -o /dev/null -w "GET /api/sessions -> %{http_code}\n" http://localhost:18120/api/sessions
kill %1 2>/dev/null || true
```
Expected: `GET / -> 200`、`GET /api/sessions -> 200`。

- [ ] **Step 8: コミット**

```bash
git add frontend/src/lib/termTransport.js frontend/src/lib/termTransport.node.test.mjs frontend/src/lib/Terminal.svelte
git commit -m "feat(frontend): dual-transport terminal (Wails bindings / browser WS) with auto-detect"
```

---

### Task 4: 全アプリ一括ビルドスクリプト

**Files:**
- Create: `scripts/build-all.sh`

**Interfaces:**
- Consumes: `frontend`（npm build）、`apps/web/cmd`（web バイナリ）、`apps/wails`（`wails build`）。
- Produces: `release/` 配下の web バイナリ群と（実行 OS 向け）Wails 成果物、`release/SHA256SUMS.txt`。

- [ ] **Step 1: build-all.sh を書く**

Create `scripts/build-all.sh`:
```bash
#!/usr/bin/env bash
#
# multi-terminals 全アプリ一括ビルドスクリプト
#
# 使い方:
#   scripts/build-all.sh [target]
#
# target:
#   all    web + wails（既定）
#   web    web サーバ版バイナリのみ（darwin/windows をクロスビルド）
#   wails  Wails デスクトップ版のみ（実行 OS 向け）
#
# 注意: Wails はクロスコンパイル不可。Windows 版は Windows 上、macOS 版は
#       macOS 上でのみビルドできる。非対象 OS は警告してスキップする。
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$ROOT"

TARGET="${1:-all}"
OUTDIR="release"
mkdir -p "$OUTDIR"

if ! command -v go >/dev/null 2>&1; then
  echo "error: go コマンドが見つかりません。" >&2
  exit 1
fi

HOST_OS="$(uname -s)"

# --- フロントエンドをビルドして埋め込みディレクトリへ配置 ---
build_frontend() {
  if [ ! -d "frontend" ]; then
    echo ">> frontend なし: UI 非組み込みでビルドします"
    return 0
  fi
  if [ ! -d "frontend/node_modules" ]; then
    echo ">> (cd frontend && npm install)"
    (cd frontend && npm install)
  fi
  echo ">> (cd frontend && npm run build)"
  (cd frontend && npm run build)
  echo ">> embed frontend/dist -> apps/web/webui/dist"
  rm -rf apps/web/webui/dist
  mkdir -p apps/web/webui/dist
  touch apps/web/webui/dist/.gitkeep
  cp -R frontend/dist/. apps/web/webui/dist/
}

# --- web サーバ版バイナリ（クロスビルド可能） ---
build_web() {
  echo ">> web: クロスビルド開始"
  local targets=(
    "darwin arm64 multi-terminals-darwin-arm64"
    "darwin amd64 multi-terminals-darwin-amd64"
    "windows amd64 multi-terminals-windows-amd64.exe"
    "windows arm64 multi-terminals-windows-arm64.exe"
  )
  local t goos goarch out
  for t in "${targets[@]}"; do
    read -r goos goarch out <<<"$t"
    echo "   - $goos/$goarch -> $out"
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
      go build -trimpath -ldflags "-s -w" -o "$OUTDIR/$out" ./apps/web/cmd
  done
  # macOS 上なら universal も作る
  if command -v lipo >/dev/null 2>&1; then
    echo "   - darwin/universal (lipo)"
    lipo -create \
      "$OUTDIR/multi-terminals-darwin-arm64" \
      "$OUTDIR/multi-terminals-darwin-amd64" \
      -output "$OUTDIR/multi-terminals-darwin-universal"
  fi
}

# --- Wails デスクトップ版（実行 OS 向けのみ） ---
build_wails() {
  if ! command -v wails >/dev/null 2>&1; then
    echo ">> [skip] wails CLI 未導入のため Wails ビルドをスキップします。" >&2
    echo "   導入: go install github.com/wailsapp/wails/v2/cmd/wails@latest" >&2
    return 0
  fi
  case "$HOST_OS" in
    Darwin)
      echo ">> wails: darwin/universal をビルド"
      (cd apps/wails && wails build -platform darwin/universal -clean)
      # 生成物 (apps/wails/build/bin/*.app) を release/ へコピー
      if [ -d "apps/wails/build/bin" ]; then
        cp -R apps/wails/build/bin/. "$OUTDIR/"
      fi
      ;;
    MINGW*|MSYS*|CYGWIN*)
      echo ">> wails: windows/amd64 をビルド"
      (cd apps/wails && wails build -platform windows/amd64 -clean)
      if [ -d "apps/wails/build/bin" ]; then
        cp -R apps/wails/build/bin/. "$OUTDIR/"
      fi
      ;;
    *)
      echo ">> [skip] この OS ($HOST_OS) では Wails ビルド対象外です。" >&2
      echo "   Windows 版は Windows 上、macOS 版は macOS 上で実行するか CI を使ってください。" >&2
      ;;
  esac
}

# --- チェックサム ---
write_checksums() {
  echo ">> チェックサム生成: $OUTDIR/SHA256SUMS.txt"
  (
    cd "$OUTDIR"
    files=$(find . -maxdepth 1 -type f ! -name 'SHA256SUMS*' ! -name 'NOTES.md' -print | sed 's|^\./||' | sort)
    : >SHA256SUMS.txt
    if [ -n "$files" ]; then
      if command -v shasum >/dev/null 2>&1; then
        echo "$files" | xargs shasum -a 256 >SHA256SUMS.txt
      else
        echo "$files" | xargs sha256sum >SHA256SUMS.txt
      fi
    fi
  )
  cat "$OUTDIR/SHA256SUMS.txt" || true
}

case "$TARGET" in
  web)
    build_frontend
    build_web
    ;;
  wails)
    build_frontend
    build_wails
    ;;
  all)
    build_frontend
    build_web
    build_wails
    ;;
  *)
    echo "unknown target: $TARGET (all|web|wails)" >&2
    exit 1
    ;;
esac

write_checksums

echo ""
echo "✅ ビルド完了: $ROOT/$OUTDIR"
ls -lh "$OUTDIR" || true
```

- [ ] **Step 2: 実行権限を付与し構文チェック**

Run:
```bash
chmod +x scripts/build-all.sh
bash -n scripts/build-all.sh && echo "syntax OK"
```
Expected: `syntax OK`。

- [ ] **Step 3: web ターゲットを実行して検証**

Run: `scripts/build-all.sh web`
Expected: `release/` に `multi-terminals-darwin-arm64`/`-amd64`/`-universal`、`multi-terminals-windows-amd64.exe`/`-arm64.exe`、`SHA256SUMS.txt` が生成。最後に一覧表示。

- [ ] **Step 4: 生成 web バイナリのスモーク（実行 OS 向け）**

Run:
```bash
BIN=$([ "$(uname -s)" = "Darwin" ] && echo release/multi-terminals-darwin-arm64 || echo release/multi-terminals-darwin-amd64)
PORT=18130 "$BIN" >/tmp/mt-all.log 2>&1 &
sleep 2
curl -s -o /dev/null -w "GET / -> %{http_code}\n" http://localhost:18130/
kill %1 2>/dev/null || true
```
Expected: `GET / -> 200`（Apple Silicon の場合。Intel なら amd64 バイナリで確認）。

- [ ] **Step 5: 全体ターゲットを実行（Wails 含む。wails CLI 導入時）**

Run: `scripts/build-all.sh all`
Expected: web バイナリ群に加え、wails CLI があれば実行 OS 向け `.app`/`.exe` が `release/` に生成。未導入時は `[skip]` 警告のみで web は成功。

- [ ] **Step 6: コミット**

```bash
git add scripts/build-all.sh
git commit -m "feat(build): add build-all script (web cross-build + native Wails)"
```

---

### Task 5: GitHub Actions による両 OS ビルド

**Files:**
- Create: `.github/workflows/build.yml`

**Interfaces:**
- Consumes: `scripts`/`apps`（リポジトリ全体）、Wails CLI（CI で `go install`）。
- Produces: macOS/Windows の成果物アーティファクト。

- [ ] **Step 1: ワークフローを書く**

Create `.github/workflows/build.yml`:
```yaml
name: build

on:
  workflow_dispatch:
  push:
    tags:
      - "v*"

jobs:
  desktop:
    name: build ${{ matrix.os }}
    runs-on: ${{ matrix.os }}
    strategy:
      fail-fast: false
      matrix:
        os: [macos-latest, windows-latest]
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"

      - uses: actions/setup-node@v4
        with:
          node-version: "20"

      - name: Install Wails CLI
        run: go install github.com/wailsapp/wails/v2/cmd/wails@latest

      - name: Build frontend and embed
        shell: bash
        run: |
          (cd frontend && npm install && npm run build)
          rm -rf apps/web/webui/dist
          mkdir -p apps/web/webui/dist
          touch apps/web/webui/dist/.gitkeep
          cp -R frontend/dist/. apps/web/webui/dist/

      - name: Build web binaries (this OS family)
        shell: bash
        run: |
          mkdir -p release
          if [ "$RUNNER_OS" = "macOS" ]; then
            CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o release/multi-terminals-darwin-arm64 ./apps/web/cmd
            CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o release/multi-terminals-darwin-amd64 ./apps/web/cmd
            lipo -create release/multi-terminals-darwin-arm64 release/multi-terminals-darwin-amd64 -output release/multi-terminals-darwin-universal
          else
            CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o release/multi-terminals-windows-amd64.exe ./apps/web/cmd
          fi

      - name: Build Wails desktop app
        shell: bash
        run: |
          if [ "$RUNNER_OS" = "macOS" ]; then
            (cd apps/wails && wails build -platform darwin/universal -clean)
          else
            (cd apps/wails && wails build -platform windows/amd64 -clean)
          fi
          cp -R apps/wails/build/bin/. release/ || true

      - name: Upload artifacts
        uses: actions/upload-artifact@v4
        with:
          name: multi-terminals-${{ matrix.os }}
          path: release/*
          if-no-files-found: error
```

- [ ] **Step 2: ローカルで lint（actionlint があれば）**

Run:
```bash
command -v actionlint >/dev/null 2>&1 && actionlint .github/workflows/build.yml || echo "actionlint 未導入: 目視確認のみ"
```
Expected: actionlint 導入時はエラーなし。未導入なら目視確認メッセージ。

- [ ] **Step 3: コミット**

```bash
git add .github/workflows/build.yml
git commit -m "ci: build Wails + web for macOS and Windows on native runners"
```

---

### Task 6: ドキュメント追記

**Files:**
- Modify: `README.md`
- Modify: `release/NOTES.md`

**Interfaces:**
- Consumes: なし（説明のみ）。
- Produces: デスクトップ版の起動方法・注意の記述。

- [ ] **Step 1: README にデスクトップ版セクションを追記**

`README.md` の「本番ビルド（単一バイナリ）」セクションの直後に、以下を挿入:
```markdown
## デスクトップ版（Wails）

`apps/wails` は Wails によるネイティブデスクトップ版です。REST/SPA は既存の
mux を在プロセス配信し、端末 I/O は Go↔JS バインディングで動きます（ネット
ワークポートを開きません）。

```sh
# 前提: wails CLI（go install github.com/wailsapp/wails/v2/cmd/wails@latest）
cd apps/wails && wails dev      # 開発実行
```

ビルド（**クロスコンパイル不可**。Windows 版は Windows 上、macOS 版は macOS 上で）:

```sh
./scripts/build-all.sh all      # web バイナリ + 実行 OS 向け Wails 成果物
./scripts/build-all.sh wails    # Wails のみ
```

Windows / macOS 両方の成果物は GitHub Actions（`.github/workflows/build.yml`）で
ネイティブランナー上から取得できます。
```

- [ ] **Step 2: release/NOTES.md にデスクトップ版の注記を追記**

`release/NOTES.md` の末尾に、以下を追記:
```markdown

## デスクトップ版（Wails）について

`multi-terminals.app`（macOS）/ `multi-terminals.exe`（Windows）は Wails 製の
デスクトップ版です。サーバ版と異なりブラウザ不要・ネットワークポート不使用で
起動します。**未署名**のため、初回起動時に Gatekeeper（macOS）/ SmartScreen
（Windows）の警告が出る場合があります。
```

- [ ] **Step 3: コミット**

```bash
git add README.md release/NOTES.md
git commit -m "docs: document the Wails desktop build and all-apps build script"
```

---

## Self-Review

**1. Spec coverage:**
- Wails ネイティブバインディング（在プロセス mux + 端末イベント）→ Task 1, 2 ✓
- 両対応・自動判定の転送層 → Task 3 ✓
- 全アプリ一括ビルドスクリプト → Task 4 ✓
- スクリプト + CI で両 OS 対応 → Task 4（ローカル）+ Task 5（CI）✓
- ドキュメント / NOTES → Task 6 ✓
- スコープ外（署名・更新・Linux・実験的クロスコンパイル）→ どのタスクにも含めない ✓

**2. Placeholder scan:** すべてのコード/コマンドは実体を記載。TBD/TODO なし。

**3. Type consistency:**
- `App` メソッド名（`PaneSubscribe`/`PaneUnsubscribe`/`PaneWrite`/`PaneResize`）は Task 1 定義と Task 3（`window.go.main.App.*`）で一致。
- イベント名 `pane:<id>` / `pane:<id>:done` は Task 1（emit）と Task 3（EventsOn）で一致。
- `web.Deps` フィールド（`Registry`/`Write`/`Resize`）と各ハンドラ `Handle(ctx, cmd)` はコア定義と一致。
- `connectPane({onData,onClose}) -> {send,resize,close}` は Task 3 内で一貫。`_ws` シームは Terminal.svelte と connectBrowser で一致。

## 既知の注意点
- `go build ./apps/wails` / `wails build` は CGO とネイティブ Webview（macOS=WebKit, Windows=WebView2）を要求。CI と各 OS のローカルでのみ通る。Linux ランナーは対象外。
- `apps/wails` のビルド生成バイナリ（`.app`/`.exe`）と `apps/wails/build/` は `.gitignore` 追加を検討（実装時、`release/` と同様に追跡対象外にする）。
