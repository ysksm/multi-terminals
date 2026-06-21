# Server-side Session Persistence & Resume Plan

**Goal:** ターミナルセッションをサーバー側で保持し、ブラウザの遷移/クローズ後に再オープンしたとき**再起動せず継続(resume)**できるようにする。スクロールバックも復元する。

**現状の課題:**
1. 切断中は PTY 出力が単一消費チャネル(256)に溜まり、満杯でシェルが停止する。
2. `OpenWorkspace` は既存セッションを Close して作り直す（＝再起動）。
3. フロントはリロード時に接続状態をリセットし、生存セッションへ再接続しない。

**方針:** PTY を常時ドレインしてスクロールバック(リングバッファ)に保持し、購読(subscribe)で「スナップショット＋ライブ」を返す**セッションハブ**を導入。`Registry` はハブを保持。`OpenWorkspace` を resume 化。`GET /api/sessions` で生存ペインを返し、フロントが自動再接続する。

## Global Constraints

- `core/application/session` は標準ライブラリのみ（`port` のみ依存）。
- 並行安全を厳守: ハブの drain・subscribe/unsubscribe は mutex で保護。`Subscription.ch` は**決して close しない**（GC 任せ）、`Subscription.done` のみ membership ガード下で**ちょうど一度** close。drain は非ブロッキング送信(select default)で、遅い購読者は drop（スクロールバックに残るので再接続で復元）。`go test -race` で検証。
- コミット末尾に `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`。

## Task 1: セッションハブ `core/application/session/session.go`（TDD）

以下を**そのまま**実装する:

```go
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
func (s *Session) ID() string                     { return s.inner.ID() }
func (s *Session) Write(data []byte) error         { return s.inner.Write(data) }
func (s *Session) Resize(cols, rows uint16) error  { return s.inner.Resize(cols, rows) }

// Done is closed when the underlying session has fully ended.
func (s *Session) Done() <-chan struct{} { return s.done }

// Close terminates the underlying session. The drain goroutine closes Done and
// all subscriptions once the PTY output channel closes.
func (s *Session) Close() error { return s.inner.Close() }
```

**Tests** (`session_test.go`, use a local fake `port.TerminalSession` whose `Output()` channel you control, or `apptest.FakeTerminalSession`):
- Subscribe before any output → empty snapshot; after writing chunks via the fake, a subscriber receives them on `C()`.
- Scrollback snapshot: feed chunks, then Subscribe → snapshot contains the prior bytes; a late subscriber still sees history.
- Scrollback cap: with a tiny max (e.g. 8), feeding more bytes keeps only the last `max` bytes.
- Unsubscribe stops delivery and closes `Done()`; idempotent (second Unsubscribe is a no-op, no panic).
- Two concurrent subscribers both receive a chunk (fan-out).
- When the fake's Output() closes, the Session's `Done()` closes and any subscriber's `Done()` closes; a subsequent Subscribe returns an already-done subscription.
- Run `go test -race ./core/application/session/`.

## Task 2: Registry はハブを保持

`core/application/session/registry.go`: 値型を `port.TerminalSession` → `*Session` に変更。`Add(id string, s *Session)`, `Get(id) (*Session, bool)`, `Remove`, `IDs() []string`, `CloseAll()`（各 `*Session.Close()`）。registry_test を追随。

## Task 3: ランタイムハンドラの追随 + OpenWorkspace を resume 化

- `open_workspace.go`:
  - 既存の「Close して作り直す」を削除。**生存セッションがあれば再起動しない**: `if _, ok := h.registry.Get(paneID); ok { result に paneID を含めて continue }`（autoRun も再送しない）。
  - 新規時のみ `inner, err := h.runner.Start(...)` → `hub := session.NewSession(inner)` → `registry.Add(paneID, hub)` → reaper `go func(){ <-hub.Done(); registry.Remove(paneID) }()` → autoRun 送信（`hub.Write`）。
  - 失敗時クリーンアップは「この呼び出しで新規に開いたもの」のみ Close/Remove（resume したものは触らない）。
  - 結果 `OpenWorkspaceResult.Panes` には resume・新規の両方の paneID を含める（フロントが全て再接続できるように）。
- `write_to_pane.go`/`resize_pane.go`/`close_pane.go`: `registry.Get` の戻りが `*session.Session` になるだけ。`Write`/`Resize`/`Close`+`Remove` はそのまま。
- 既存テスト追随: `apptest.FakeTerminalRunner` が生成した `FakeTerminalSession` を取得できるよう、必要なら `FakeTerminalRunner` に `Session(id string) *FakeTerminalSession`（生成済みを記録）を追加し、OpenWorkspace テストの「autoRun が Write された」検証を fake 経由で行う。resume の新規テスト: 既存セッションがある状態で再度 Open → `runner.Start` が再呼び出しされない（`Started` が増えない）こと、autoRun が再送されないこと。

## Task 4: WebSocket をハブ購読に変更（`apps/web/ws.go`）

- `hub, ok := d.Registry.Get(paneID)`（無ければ 404）。
- `snapshot, sub := hub.Subscribe(); defer hub.Unsubscribe(sub)`。
- アップグレード後、まず `snapshot` を Binary で送信（あれば）。
- 出力ポンプ: `select { case chunk := <-sub.C(): wsWrite(Binary, chunk); case <-sub.Done(): close+return; case <-done(client切断): return }`。`sub.C()` は close されないので ok 判定不要。
- 入力ループは現状どおり（input→WriteToPane、resize→ResizePane）。
- **ConnGuard は撤去**（ハブが複数購読を安全に扱うため不要。2タブ閲覧も可。重複接続 409 テストは「2接続とも出力を受け取る」テストに置換）。
- gorilla の単一ライター要件は維持（`wsWrite` の mutex）。

## Task 5: `GET /api/sessions` エンドポイント

- `server.go`: `mux.HandleFunc("GET /api/sessions", d.handleListSessions)`。`handleListSessions` は `{"paneIds": d.Registry.IDs()}` を返す（空でも `[]`）。
- テスト: open 後に paneId が含まれる、未 open では空。

## 検証

- `go test -race ./...` 全 PASS、`go vet ./...`、`GOOS=windows GOARCH=amd64 go build ./...`。

## フロント（別途、controller が実装）

- `api.js`: `listSessions: () => req('GET','/api/sessions')`。
- `App.svelte`: ワークスペース選択/ロード時に `listSessions()` を取得し、`current.panes` と突き合わせて生存ペインの `openedPaneIds` をセット → 端末が自動再接続（スナップショットで画面復元）。「開く」は resume 化された API を呼び、結果の paneIds で `openedPaneIds` を更新。
