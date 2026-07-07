# エージェント稼働状況インジケータ Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** ワークスペース一覧に claude code / codex の稼働状況（active / wait とツール別件数）をリアルタイム表示する。

**Architecture:** サーバ側は `ps` によるプロセス木走査でペインのシェル子孫から claude/codex を検出し、スクロールバック末尾＋idle 時間で active/wait を判定。`agentstatus.Watcher` が 1.5 秒周期で差分を検出し SSE（`/api/agent-status/stream`）で push、スナップショット REST（`/api/agent-status`）をフォールバックに用意。フロントは pane→workspace 集計してサイドバーにバッジ表示。

**Tech Stack:** Go（stdlib のみ、DDD レイヤード: application=`agentstatus` / infrastructure=`procscan`）、SSE、Svelte 5 runes、node:test。

## Global Constraints

- core/application・core/domain は stdlib のみ（外部依存禁止）。`agentstatus` が `Proc` 型とスキャナ関数型を定義し、`procscan`（infrastructure）が実装する
- `port.TerminalSession` インターフェースは**変更しない**。PID はオプショナルインターフェース `interface{ Pid() int }` の型アサーションで取得
- Windows / リモートペインは検出対象外（エラーにせず空・スキップ）
- コメントは既存コードの流儀に合わせる（パッケージコメント英語 or 日本語混在可、既存に準拠）
- テスト: Go は `go test ./...`、フロント純関数は `node --test frontend/src/lib/<name>.node.test.mjs`
- コミットメッセージ末尾: `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`

---

### Task 1: agentstatus 検出ロジック（Proc / MatchTool / DetectTools）

**Files:**
- Create: `core/application/agentstatus/detect.go`
- Test: `core/application/agentstatus/detect_test.go`

**Interfaces:**
- Produces: `type Proc struct{ PID, PPID int; Command string }`, `func MatchTool(command string) string`, `func DetectTools(procs []Proc, rootPID int) []string`

- [ ] **Step 1: Write the failing test**

```go
package agentstatus

import (
	"reflect"
	"testing"
)

func TestMatchTool(t *testing.T) {
	cases := []struct {
		command string
		want    string
	}{
		{"claude", "claude"},
		{"/usr/local/bin/claude --continue", "claude"},
		{"node /usr/local/lib/node_modules/@anthropic-ai/claude-code/cli.js", "claude"},
		{"codex exec", "codex"},
		{"/opt/homebrew/bin/codex", "codex"},
		{"vim main.go", ""},
		{"-zsh", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := MatchTool(c.command); got != c.want {
			t.Errorf("MatchTool(%q) = %q, want %q", c.command, got, c.want)
		}
	}
}

func TestDetectTools(t *testing.T) {
	// shell(10) ─ claude(11) ─ node helper(12)
	//          └ codex(13)
	// unrelated claude under another root(20→21)
	procs := []Proc{
		{PID: 10, PPID: 1, Command: "-zsh"},
		{PID: 11, PPID: 10, Command: "claude"},
		{PID: 12, PPID: 11, Command: "node helper.js"},
		{PID: 13, PPID: 10, Command: "codex exec"},
		{PID: 20, PPID: 1, Command: "-bash"},
		{PID: 21, PPID: 20, Command: "claude"},
	}
	if got := DetectTools(procs, 10); !reflect.DeepEqual(got, []string{"claude", "codex"}) {
		t.Errorf("DetectTools(root=10) = %v, want [claude codex]", got)
	}
	if got := DetectTools(procs, 20); !reflect.DeepEqual(got, []string{"claude"}) {
		t.Errorf("DetectTools(root=20) = %v, want [claude]", got)
	}
	if got := DetectTools(procs, 99); len(got) != 0 {
		t.Errorf("DetectTools(root=99) = %v, want empty", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./core/application/agentstatus/`
Expected: FAIL（`MatchTool` 未定義のコンパイルエラー）

- [ ] **Step 3: Write minimal implementation**

```go
// Package agentstatus detects coding-agent CLIs (claude code / codex) running
// inside pane shells and classifies their state (active / waiting for
// permission). It defines the process-snapshot port implemented by
// core/infrastructure/procscan and is consumed by the web adapter's
// agent-status endpoints.
package agentstatus

import (
	"sort"
	"strings"
)

// Proc is one OS process in a snapshot (see the Scanner port in watcher.go).
type Proc struct {
	PID     int
	PPID    int
	Command string
}

// MatchTool は command 行がエージェント CLI に見える場合にツール名
// ("claude" / "codex") を返す。それ以外は空文字。
// claude はネイティブバイナリでも node 実行(@anthropic-ai/claude-code)でも
// コマンド行に "claude" を含むため、実行形態によらず拾える。
func MatchTool(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	base := fields[0]
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[i+1:]
	}
	switch base {
	case "claude":
		return "claude"
	case "codex":
		return "codex"
	}
	if strings.Contains(command, "claude") {
		return "claude"
	}
	if strings.Contains(command, "codex") {
		return "codex"
	}
	return ""
}

// DetectTools は procs のプロセス木を rootPID の子孫方向に走査し、検出した
// エージェントのツール名を重複なし・昇順で返す。root 自身は含めない。
func DetectTools(procs []Proc, rootPID int) []string {
	children := make(map[int][]Proc, len(procs))
	for _, p := range procs {
		children[p.PPID] = append(children[p.PPID], p)
	}
	found := map[string]bool{}
	stack := []int{rootPID}
	for len(stack) > 0 {
		pid := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, c := range children[pid] {
			if t := MatchTool(c.Command); t != "" {
				found[t] = true
			}
			stack = append(stack, c.PID)
		}
	}
	out := make([]string, 0, len(found))
	for t := range found {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./core/application/agentstatus/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add core/application/agentstatus/detect.go core/application/agentstatus/detect_test.go
git commit -m "feat(agentstatus): detect claude/codex in a process tree"
```

---

### Task 2: agentstatus 状態判定（active / wait）

**Files:**
- Create: `core/application/agentstatus/state.go`
- Test: `core/application/agentstatus/state_test.go`

**Interfaces:**
- Produces: `type State string`（`StateActive` / `StateWait`）, `func ClassifyState(tail []byte, lastOutput, now time.Time) State`, `const WaitIdleThreshold = 2 * time.Second`

- [ ] **Step 1: Write the failing test**

```go
package agentstatus

import (
	"testing"
	"time"
)

func TestClassifyState(t *testing.T) {
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	idle := now.Add(-3 * time.Second)   // WaitIdleThreshold(2s) 超え
	recent := now.Add(-500 * time.Millisecond)

	cases := []struct {
		name       string
		tail       string
		lastOutput time.Time
		want       State
	}{
		{"許可プロンプト + idle → wait", "Do you want to proceed?\n❯ 1. Yes", idle, StateWait},
		{"claude 選択プロンプト + idle → wait", "some output\n❯ 1. Yes\n  2. No", idle, StateWait},
		{"codex 承認プロンプト + idle → wait", "Allow command? [y/n]", idle, StateWait},
		{"プロンプトがあっても直近に出力あり → active", "Do you want to proceed?", recent, StateActive},
		{"プロンプトなし + idle → active", "$ compiling...", idle, StateActive},
		{"空の tail → active", "", idle, StateActive},
	}
	for _, c := range cases {
		if got := ClassifyState([]byte(c.tail), c.lastOutput, now); got != c.want {
			t.Errorf("%s: ClassifyState = %q, want %q", c.name, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./core/application/agentstatus/`
Expected: FAIL（`ClassifyState` 未定義のコンパイルエラー）

- [ ] **Step 3: Write minimal implementation**

```go
package agentstatus

import (
	"strings"
	"time"
)

// State はペイン内エージェントの状態。
type State string

const (
	// StateActive: エージェントが実行中(プロンプト待ちでない)。
	StateActive State = "active"
	// StateWait: 許可プロンプトを表示してユーザー入力を待って停止中。
	StateWait State = "wait"
)

// WaitIdleThreshold: 末尾に許可プロンプトが見えていても、この時間以上
// 新規出力が無い場合にのみ wait と判定する。スクロールバックに過去の
// プロンプトが残っているだけのケース(直後に出力が続いた)を弾くための条件。
const WaitIdleThreshold = 2 * time.Second

// waitPatterns は「許可待ちで停止中」を示す既知のプロンプト文字列。
// 端末のエスケープシーケンス混じりの生バイト列に対する部分一致で判定する。
var waitPatterns = []string{
	"Do you want",   // claude code の許可プロンプト
	"❯ 1. Yes",      // claude code の選択プロンプト
	"Allow command", // codex の承認プロンプト
	"Approve",       // codex
}

// ClassifyState はスクロールバック末尾 tail と最終出力時刻から状態を判定する。
// wait = 末尾に許可プロンプト文字列があり、かつ WaitIdleThreshold 以上 idle。
func ClassifyState(tail []byte, lastOutput, now time.Time) State {
	if now.Sub(lastOutput) < WaitIdleThreshold {
		return StateActive
	}
	s := string(tail)
	for _, p := range waitPatterns {
		if strings.Contains(s, p) {
			return StateWait
		}
	}
	return StateActive
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./core/application/agentstatus/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add core/application/agentstatus/state.go core/application/agentstatus/state_test.go
git commit -m "feat(agentstatus): classify agent state (active / permission-wait)"
```

---

### Task 3: procscan（ps 実行とパース）

**Files:**
- Create: `core/infrastructure/procscan/procscan.go`
- Test: `core/infrastructure/procscan/procscan_test.go`

**Interfaces:**
- Consumes: `agentstatus.Proc`（Task 1）
- Produces: `func Parse(out string) []agentstatus.Proc`, `func Snapshot() ([]agentstatus.Proc, error)`

- [ ] **Step 1: Write the failing test**

```go
package procscan

import (
	"runtime"
	"testing"

	"github.com/ysksm/multi-terminals/core/application/agentstatus"
)

func TestParse(t *testing.T) {
	out := "    1     0 /sbin/launchd\n" +
		"  501     1 -zsh\n" +
		" 1234   501 claude --continue\n" +
		"garbage line\n" +
		"\n"
	procs := Parse(out)
	want := []agentstatus.Proc{
		{PID: 1, PPID: 0, Command: "/sbin/launchd"},
		{PID: 501, PPID: 1, Command: "-zsh"},
		{PID: 1234, PPID: 501, Command: "claude --continue"},
	}
	if len(procs) != len(want) {
		t.Fatalf("Parse: got %d procs, want %d (%v)", len(procs), len(want), procs)
	}
	for i := range want {
		if procs[i] != want[i] {
			t.Errorf("Parse[%d] = %+v, want %+v", i, procs[i], want[i])
		}
	}
}

func TestSnapshotSmoke(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ps is unavailable on windows")
	}
	procs, err := Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(procs) == 0 {
		t.Fatal("Snapshot: got 0 procs, expected at least this test process")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./core/infrastructure/procscan/`
Expected: FAIL（パッケージ未作成のコンパイルエラー）

- [ ] **Step 3: Write minimal implementation**

```go
// Package procscan takes OS process snapshots via ps(1) for agent-status
// detection. It implements the agentstatus.Scanner port. Windows has no ps,
// so Snapshot returns an empty list there (agent badges simply don't show).
package procscan

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/ysksm/multi-terminals/core/application/agentstatus"
)

// Snapshot は全プロセスの PID/PPID/コマンド行を 1 回の ps 実行で取得する。
func Snapshot() ([]agentstatus.Proc, error) {
	if runtime.GOOS == "windows" {
		return nil, nil
	}
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,command=").Output()
	if err != nil {
		return nil, err
	}
	return Parse(string(out)), nil
}

// Parse は `ps -axo pid=,ppid=,command=` の出力をパースする。
// 解釈できない行は読み飛ばす。
func Parse(out string) []agentstatus.Proc {
	var procs []agentstatus.Proc
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		procs = append(procs, agentstatus.Proc{PID: pid, PPID: ppid, Command: strings.Join(fields[2:], " ")})
	}
	return procs
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./core/infrastructure/procscan/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add core/infrastructure/procscan/
git commit -m "feat(procscan): take process snapshots via ps for agent detection"
```

---

### Task 4: Session に Pid / Tail / LastOutputAt を追加

**Files:**
- Modify: `core/application/session/session.go`（drain で最終出力時刻を記録、Tail/LastOutputAt/Pid を追加）
- Modify: `core/infrastructure/terminal/pty_runner.go`（`ptySession.Pid()` を追加）
- Test: `core/application/session/session_test.go` に追記

**Interfaces:**
- Produces: `(*session.Session).Pid() int`（内側が `interface{ Pid() int }` を実装しない場合は 0）、`(*session.Session).Tail(n int) []byte`、`(*session.Session).LastOutputAt() time.Time`

- [ ] **Step 1: Write the failing test**

`core/application/session/session_test.go` に追記（既存のフェイク `port.TerminalSession` の構造に合わせること。既存テストのフェイク型名が異なる場合はそれを再利用する）:

```go
// fakePidSession は Pid() を持つフェイク。既存の fake セッション型を埋め込み
// (または既存フェイクに準拠して) Output チャネルを制御できるようにする。
type fakePidSession struct {
	port.TerminalSession
	pid int
}

func (f *fakePidSession) Pid() int { return f.pid }

func TestSessionPidTailLastOutput(t *testing.T) {
	out := make(chan []byte, 4)
	inner := newFakeTerminalSession("p1", out) // 既存テストのフェイク生成に合わせる
	s := NewSession(&fakePidSession{TerminalSession: inner, pid: 4321})
	defer s.Close()

	if got := s.Pid(); got != 4321 {
		t.Errorf("Pid() = %d, want 4321", got)
	}

	if !s.LastOutputAt().IsZero() {
		t.Error("LastOutputAt() should be zero before any output")
	}

	before := time.Now()
	out <- []byte("hello ")
	out <- []byte("world")
	waitForScrollback(t, s, "hello world") // 既存ヘルパが無ければ Eventually ループで待つ

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
	out := make(chan []byte)
	inner := newFakeTerminalSession("p2", out)
	s := NewSession(inner)
	defer s.Close()
	if got := s.Pid(); got != 0 {
		t.Errorf("Pid() = %d, want 0 for inner session without Pid()", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./core/application/session/`
Expected: FAIL（`Pid` / `Tail` / `LastOutputAt` 未定義のコンパイルエラー）

- [ ] **Step 3: Write minimal implementation**

`session.go` の変更点:

```go
// struct に追加 (mu 保護下)
	lastOutput time.Time

// drain() のチャンク処理先頭 (s.appendScrollback(chunk) の直後) に追加
		s.lastOutput = time.Now()

// メソッド追加
// Pid returns the OS process ID of the underlying local terminal process, or
// 0 when unknown (e.g. remote sessions or fakes without a local PID).
func (s *Session) Pid() int {
	if p, ok := s.inner.(interface{ Pid() int }); ok {
		return p.Pid()
	}
	return 0
}

// Tail returns a copy of up to the last n bytes of scrollback.
func (s *Session) Tail(n int) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n > len(s.scrollback) {
		n = len(s.scrollback)
	}
	out := make([]byte, n)
	copy(out, s.scrollback[len(s.scrollback)-n:])
	return out
}

// LastOutputAt returns the arrival time of the most recent output chunk
// (zero value if no output has been received yet).
func (s *Session) LastOutputAt() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastOutput
}
```

`pty_runner.go` に追加:

```go
// Pid returns the shell's OS process ID (0 if unavailable). Exposed for
// agent-status detection, which walks the process tree below the shell.
func (s *ptySession) Pid() int {
	if s.cmd != nil && s.cmd.Process != nil {
		return s.cmd.Process.Pid
	}
	return 0
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./core/application/session/ ./core/infrastructure/terminal/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add core/application/session/session.go core/application/session/session_test.go core/infrastructure/terminal/pty_runner.go
git commit -m "feat(session): expose pid, scrollback tail and last-output time"
```

---

### Task 5: agentstatus.Watcher（周期監視・差分 push）

**Files:**
- Create: `core/application/agentstatus/watcher.go`
- Test: `core/application/agentstatus/watcher_test.go`

**Interfaces:**
- Consumes: `Proc` / `DetectTools`（Task 1）、`ClassifyState`（Task 2）
- Produces:
  - `type PaneAgent struct{ Tool string; State State }`（JSON タグ `tool` / `state`）
  - `type Snapshot map[string][]PaneAgent`
  - `type SessionInfo struct{ PaneID string; Pid int; Tail []byte; LastOutput time.Time }`
  - `type Source func() []SessionInfo` / `type Scanner func() ([]Proc, error)`
  - `func NewWatcher(source Source, scan Scanner, interval time.Duration) *Watcher`
  - `(*Watcher).Start()` / `(*Watcher).Stop()` / `(*Watcher).Current() Snapshot`
  - `(*Watcher).Subscribe() (Snapshot, <-chan Snapshot, func())`

- [ ] **Step 1: Write the failing test**

```go
package agentstatus

import (
	"reflect"
	"testing"
	"time"
)

func TestWatcherPollDetectsAndBroadcastsOnChange(t *testing.T) {
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	procs := []Proc{
		{PID: 10, PPID: 1, Command: "-zsh"},
		{PID: 11, PPID: 10, Command: "claude"},
	}
	sessions := []SessionInfo{{PaneID: "pane-1", Pid: 10, Tail: []byte("building..."), LastOutput: now.Add(-5 * time.Second)}}

	w := NewWatcher(
		func() []SessionInfo { return sessions },
		func() ([]Proc, error) { return procs, nil },
		time.Hour, // ティッカーは実質無効化し poll を直接叩く
	)

	snap0, ch, cancel := w.Subscribe()
	defer cancel()
	if len(snap0) != 0 {
		t.Fatalf("initial snapshot should be empty, got %v", snap0)
	}

	w.poll(now)
	want := Snapshot{"pane-1": {{Tool: "claude", State: StateActive}}}
	select {
	case got := <-ch:
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("broadcast = %v, want %v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast after first poll")
	}
	if !reflect.DeepEqual(w.Current(), want) {
		t.Fatalf("Current() = %v, want %v", w.Current(), want)
	}

	// 変化なし → 再 push されない
	w.poll(now.Add(time.Second))
	select {
	case got := <-ch:
		t.Fatalf("unexpected broadcast on unchanged snapshot: %v", got)
	case <-time.After(50 * time.Millisecond):
	}

	// wait へ遷移(末尾にプロンプト + idle) → push される
	sessions = []SessionInfo{{PaneID: "pane-1", Pid: 10, Tail: []byte("Do you want to proceed?"), LastOutput: now.Add(-5 * time.Second)}}
	w.poll(now.Add(2 * time.Second))
	select {
	case got := <-ch:
		if got["pane-1"][0].State != StateWait {
			t.Fatalf("state = %q, want wait", got["pane-1"][0].State)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast on state change")
	}
}

func TestWatcherScanErrorYieldsEmpty(t *testing.T) {
	w := NewWatcher(
		func() []SessionInfo { return []SessionInfo{{PaneID: "p", Pid: 1}} },
		func() ([]Proc, error) { return nil, errScan },
		time.Hour,
	)
	w.poll(time.Now())
	if len(w.Current()) != 0 {
		t.Fatalf("Current() = %v, want empty on scan error", w.Current())
	}
}

var errScan = errFake("scan failed")

type errFake string

func (e errFake) Error() string { return string(e) }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./core/application/agentstatus/`
Expected: FAIL（`NewWatcher` 未定義のコンパイルエラー）

- [ ] **Step 3: Write minimal implementation**

```go
package agentstatus

import (
	"reflect"
	"sync"
	"time"
)

// PaneAgent は 1 ペインで検出された 1 エージェントの状態。
type PaneAgent struct {
	Tool  string `json:"tool"`
	State State  `json:"state"`
}

// Snapshot は paneID → 検出エージェント一覧。検出ゼロのペインは含めない。
type Snapshot map[string][]PaneAgent

// SessionInfo は 1 ペインの判定に必要なライブセッション情報。
type SessionInfo struct {
	PaneID     string
	Pid        int
	Tail       []byte
	LastOutput time.Time
}

// Source は監視対象セッションを列挙する(web アダプタが Registry から供給)。
type Source func() []SessionInfo

// Scanner は全プロセスのスナップショットを返す(procscan.Snapshot が実装)。
type Scanner func() ([]Proc, error)

// DefaultInterval はポーリング周期の既定値。
const DefaultInterval = 1500 * time.Millisecond

// TailBytes は状態判定に使うスクロールバック末尾のバイト数。
const TailBytes = 2048

// Watcher は周期的にエージェント稼働状況を算出し、変化時のみ購読者へ
// push する。全メソッド並行安全。
type Watcher struct {
	source   Source
	scan     Scanner
	interval time.Duration

	mu      sync.Mutex
	current Snapshot
	subs    map[chan Snapshot]struct{}

	stop     chan struct{}
	stopOnce sync.Once
}

// NewWatcher returns a Watcher. interval <= 0 selects DefaultInterval.
func NewWatcher(source Source, scan Scanner, interval time.Duration) *Watcher {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return &Watcher{
		source:   source,
		scan:     scan,
		interval: interval,
		current:  Snapshot{},
		subs:     make(map[chan Snapshot]struct{}),
		stop:     make(chan struct{}),
	}
}

// Start begins the polling loop. Stop ends it.
func (w *Watcher) Start() { go w.loop() }

// Stop terminates the polling loop. Idempotent.
func (w *Watcher) Stop() { w.stopOnce.Do(func() { close(w.stop) }) }

func (w *Watcher) loop() {
	t := time.NewTicker(w.interval)
	defer t.Stop()
	w.poll(time.Now())
	for {
		select {
		case <-w.stop:
			return
		case now := <-t.C:
			w.poll(now)
		}
	}
}

// poll は 1 回分のスナップショットを算出し、前回から変化していれば
// current を更新して全購読者へ push する。
func (w *Watcher) poll(now time.Time) {
	snap := w.compute(now)
	w.mu.Lock()
	if reflect.DeepEqual(snap, w.current) {
		w.mu.Unlock()
		return
	}
	w.current = snap
	subs := make([]chan Snapshot, 0, len(w.subs))
	for ch := range w.subs {
		subs = append(subs, ch)
	}
	w.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- snap:
		default:
			// 受信が滞っている購読者はスキップ(次の変化でまた試す)。
		}
	}
}

func (w *Watcher) compute(now time.Time) Snapshot {
	snap := Snapshot{}
	procs, err := w.scan()
	if err != nil || len(procs) == 0 {
		return snap
	}
	for _, si := range w.source() {
		if si.Pid <= 0 {
			continue
		}
		tools := DetectTools(procs, si.Pid)
		if len(tools) == 0 {
			continue
		}
		state := ClassifyState(si.Tail, si.LastOutput, now)
		agents := make([]PaneAgent, len(tools))
		for i, tool := range tools {
			agents[i] = PaneAgent{Tool: tool, State: state}
		}
		snap[si.PaneID] = agents
	}
	return snap
}

// Current returns the most recent snapshot.
func (w *Watcher) Current() Snapshot {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.current
}

// Subscribe returns the current snapshot, a channel receiving subsequent
// changed snapshots, and a cancel function releasing the subscription.
func (w *Watcher) Subscribe() (Snapshot, <-chan Snapshot, func()) {
	ch := make(chan Snapshot, 8)
	w.mu.Lock()
	w.subs[ch] = struct{}{}
	snap := w.current
	w.mu.Unlock()
	cancel := func() {
		w.mu.Lock()
		delete(w.subs, ch)
		w.mu.Unlock()
	}
	return snap, ch, cancel
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./core/application/agentstatus/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add core/application/agentstatus/watcher.go core/application/agentstatus/watcher_test.go
git commit -m "feat(agentstatus): add polling watcher with change-only broadcast"
```

---

### Task 6: web エンドポイント（snapshot + SSE）と配線

**Files:**
- Create: `apps/web/server_agentstatus.go`
- Modify: `apps/web/server.go`（`Deps` に `AgentStatus *agentstatus.Watcher` 追加、ルート登録）
- Modify: `apps/web/app.go`（`BuildDeps` で Watcher を構築・Start、Registry→Source アダプタ）
- Test: `apps/web/server_agentstatus_test.go`

**Interfaces:**
- Consumes: `agentstatus.Watcher`（Task 5）、`procscan.Snapshot`（Task 3）、`(*session.Session).Pid/Tail/LastOutputAt`（Task 4）
- Produces: `GET /api/agent-status` → `{"panes": {...}}`、`GET /api/agent-status/stream` → SSE（同ペイロードを `data:` 行で配信）

- [ ] **Step 1: Write the failing test**

```go
package web

import (
	"bufio"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ysksm/multi-terminals/core/application/agentstatus"
)

// newAgentStatusDeps は AgentStatus 付きの最小 Deps を作る。
// 既存テストのヘルパ(newTestDeps 等)がある場合はそれを使い AgentStatus を差し込む。
func newAgentStatusWatcher() *agentstatus.Watcher {
	procs := []agentstatus.Proc{
		{PID: 10, PPID: 1, Command: "-zsh"},
		{PID: 11, PPID: 10, Command: "claude"},
	}
	sessions := []agentstatus.SessionInfo{{PaneID: "pane-1", Pid: 10, LastOutput: time.Now().Add(-5 * time.Second)}}
	return agentstatus.NewWatcher(
		func() []agentstatus.SessionInfo { return sessions },
		func() ([]agentstatus.Proc, error) { return procs, nil },
		time.Hour,
	)
}

func TestAgentStatusSnapshotEndpoint(t *testing.T) {
	deps := newTestDeps(t) // 既存の Deps 構築ヘルパに合わせる
	deps.AgentStatus = newAgentStatusWatcher()
	deps.AgentStatus.PollNow()
	mux := NewMux(deps)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/api/agent-status", nil))
	if rr.Code != 200 {
		t.Fatalf("GET /api/agent-status: status %d", rr.Code)
	}
	var body struct {
		Panes map[string][]agentstatus.PaneAgent `json:"panes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Panes["pane-1"]) != 1 || body.Panes["pane-1"][0].Tool != "claude" {
		t.Fatalf("panes = %v, want pane-1 → claude", body.Panes)
	}
}

func TestAgentStatusStreamSendsInitialSnapshot(t *testing.T) {
	deps := newTestDeps(t)
	deps.AgentStatus = newAgentStatusWatcher()
	deps.AgentStatus.PollNow()
	srv := httptest.NewServer(NewMux(deps))
	defer srv.Close()

	res, err := srv.Client().Get(srv.URL + "/api/agent-status/stream")
	if err != nil {
		t.Fatalf("GET stream: %v", err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q", ct)
	}
	sc := bufio.NewScanner(res.Body)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "data: ") {
			if !strings.Contains(line, `"claude"`) {
				t.Fatalf("first event = %q, want claude snapshot", line)
			}
			return
		}
	}
	t.Fatal("no data event received")
}
```

（`PollNow()` は Watcher にテスト/初期化用として追加する: `func (w *Watcher) PollNow() { w.poll(time.Now()) }`）

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./apps/web/ -run TestAgentStatus`
Expected: FAIL（`Deps.AgentStatus` 未定義のコンパイルエラー）

- [ ] **Step 3: Write minimal implementation**

`core/application/agentstatus/watcher.go` に追加:

```go
// PollNow は即時に 1 回ポーリングする(初期化・テスト用)。
func (w *Watcher) PollNow() { w.poll(time.Now()) }
```

`apps/web/server.go`: `Deps` に追加（import に `agentstatus` を追加）:

```go
	// AgentStatus watches panes for running agent CLIs (claude/codex).
	// Nil disables the /api/agent-status endpoints.
	AgentStatus *agentstatus.Watcher
```

`NewMux` のルート登録（`/api/sessions` 付近）:

```go
	// Agent status (claude/codex activity per pane)
	if d.AgentStatus != nil {
		mux.HandleFunc("GET /api/agent-status", d.handleAgentStatus)
		mux.HandleFunc("GET /api/agent-status/stream", d.handleAgentStatusStream)
	}
```

`apps/web/server_agentstatus.go`（新規）:

```go
package web

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/ysksm/multi-terminals/core/application/agentstatus"
	"github.com/ysksm/multi-terminals/core/application/session"
)

// handleAgentStatus は現在のエージェント稼働状況スナップショットを返す。
// SSE が使えない環境(ポーリングフォールバック)向け。
func (d Deps) handleAgentStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"panes": d.AgentStatus.Current()})
}

// handleAgentStatusStream は稼働状況を SSE で配信する。接続直後に現在の
// スナップショットを 1 回送り、以後は変化時のみ送る。
func (d Deps) handleAgentStatusStream(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusNotImplemented)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	snap, ch, cancel := d.AgentStatus.Subscribe()
	defer cancel()

	send := func(s agentstatus.Snapshot) bool {
		b, err := json.Marshal(map[string]any{"panes": s})
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		f.Flush()
		return true
	}
	if !send(snap) {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case s := <-ch:
			if !send(s) {
				return
			}
		}
	}
}

// registrySource は Registry のライブセッションを agentstatus.Source に
// 適合させる。PID を持たないセッション(リモート等)は対象外。
func registrySource(reg *session.Registry) agentstatus.Source {
	return func() []agentstatus.SessionInfo {
		var out []agentstatus.SessionInfo
		for _, id := range reg.IDs() {
			s, ok := reg.Get(id)
			if !ok {
				continue
			}
			pid := s.Pid()
			if pid <= 0 {
				continue
			}
			out = append(out, agentstatus.SessionInfo{
				PaneID:     id,
				Pid:        pid,
				Tail:       s.Tail(agentstatus.TailBytes),
				LastOutput: s.LastOutputAt(),
			})
		}
		return out
	}
}
```

`apps/web/app.go` の `BuildDeps`（Registry 構築後、Deps を組み立てる箇所）に追加（import に `agentstatus` / `procscan` を追加）:

```go
	// エージェント稼働状況の監視を開始(サーバ寿命と同じでよいので Stop は不要)。
	watcher := agentstatus.NewWatcher(registrySource(reg), procscan.Snapshot, 0)
	watcher.Start()
	// Deps リテラルに: AgentStatus: watcher,
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./apps/web/ ./core/application/agentstatus/`
Expected: PASS（既存テスト含む）

- [ ] **Step 5: Commit**

```bash
git add apps/web/server_agentstatus.go apps/web/server_agentstatus_test.go apps/web/server.go apps/web/app.go core/application/agentstatus/watcher.go
git commit -m "feat(web): serve agent status via snapshot + SSE endpoints"
```

---

### Task 7: フロント購読・集計ロジック（agentStatus.js）

**Files:**
- Create: `frontend/src/lib/agentStatus.js`
- Test: `frontend/src/lib/agentStatus.node.test.mjs`

**Interfaces:**
- Consumes: `GET /api/agent-status`（JSON）/ `GET /api/agent-status/stream`（SSE）
- Produces: `aggregateByWorkspace(panes, workspaces)` → `Map(wsId → [{tool, active, wait}])`、`connectAgentStatus(onUpdate, {pollMs}) → stop()`

- [ ] **Step 1: Write the failing test**

```js
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { aggregateByWorkspace } from './agentStatus.js'

test('pane 単位の状態を workspace 単位のツール別件数へ集計する', () => {
  const panes = {
    'p1': [{ tool: 'claude', state: 'active' }],
    'p2': [{ tool: 'claude', state: 'wait' }, { tool: 'codex', state: 'wait' }],
    'p9': [{ tool: 'claude', state: 'active' }], // どの workspace にも属さない
  }
  const workspaces = [
    { id: 'w1', panes: [{ id: 'p1' }, { id: 'p2' }] },
    { id: 'w2', panes: [{ id: 'p3' }] },
  ]
  const m = aggregateByWorkspace(panes, workspaces)
  assert.deepEqual(m.get('w1'), [
    { tool: 'claude', active: 1, wait: 1 },
    { tool: 'codex', active: 0, wait: 1 },
  ])
  assert.equal(m.has('w2'), false, '稼働ゼロの workspace はエントリなし')
})

test('空入力は空 Map', () => {
  assert.equal(aggregateByWorkspace({}, []).size, 0)
  assert.equal(aggregateByWorkspace(undefined, undefined).size, 0)
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `node --test frontend/src/lib/agentStatus.node.test.mjs`
Expected: FAIL（`agentStatus.js` が存在しない）

- [ ] **Step 3: Write minimal implementation**

```js
// エージェント(claude/codex)稼働状況の購読と、ワークスペース単位への集計。
// サーバは pane 単位で配信する(疎結合)ため、workspace への割り付けはここで行う。

// panes: { [paneId]: [{tool, state}] } / workspaces: [{id, panes: [{id}]}]
// 戻り値: Map(workspaceId → [{tool, active, wait}] ツール名昇順)。稼働ゼロの
// workspace はエントリを持たない。
export function aggregateByWorkspace(panes, workspaces) {
  const byWs = new Map()
  for (const ws of workspaces || []) {
    const counts = new Map()
    for (const pane of ws.panes || []) {
      for (const a of (panes || {})[pane.id] || []) {
        const c = counts.get(a.tool) || { tool: a.tool, active: 0, wait: 0 }
        if (a.state === 'wait') c.wait++
        else c.active++
        counts.set(a.tool, c)
      }
    }
    if (counts.size > 0) {
      byWs.set(ws.id, [...counts.values()].sort((x, y) => x.tool.localeCompare(y.tool)))
    }
  }
  return byWs
}

// SSE(/api/agent-status/stream)で購読し、使えない環境(SSE 非対応・接続断)では
// /api/agent-status の定期ポーリングにフォールバックする。
// onUpdate(panes) を受信のたびに呼ぶ。戻り値は購読停止関数。
export function connectAgentStatus(onUpdate, { pollMs = 3000 } = {}) {
  let stopped = false
  let es = null
  let timer = null

  const poll = async () => {
    if (stopped) return
    try {
      const res = await fetch('/api/agent-status')
      if (res.ok) onUpdate((await res.json()).panes || {})
    } catch {
      // サーバ再起動中など。次回のポーリングに任せる。
    }
    timer = setTimeout(poll, pollMs)
  }

  try {
    es = new EventSource('/api/agent-status/stream')
    es.onmessage = (ev) => {
      try {
        onUpdate(JSON.parse(ev.data).panes || {})
      } catch {
        // 壊れたイベントは読み飛ばす
      }
    }
    es.onerror = () => {
      // SSE が張れない環境(Wails の AssetServer 等)はポーリングへ切替。
      es?.close()
      es = null
      if (!timer) poll()
    }
  } catch {
    poll()
  }

  return () => {
    stopped = true
    es?.close()
    if (timer) clearTimeout(timer)
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `node --test frontend/src/lib/agentStatus.node.test.mjs`
Expected: PASS（2 tests）

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/agentStatus.js frontend/src/lib/agentStatus.node.test.mjs
git commit -m "feat(frontend): agent-status subscription and per-workspace aggregation"
```

---

### Task 8: サイドバー表示（App.svelte）と仕上げ

**Files:**
- Modify: `frontend/src/App.svelte`（購読開始、ワークスペース行にバッジ、CSS）
- Modify: `README.md`（機能一覧に 1 行）

**Interfaces:**
- Consumes: `aggregateByWorkspace` / `connectAgentStatus`（Task 7）

- [ ] **Step 1: 購読と状態を追加**

`App.svelte` の script 部:

```js
import { aggregateByWorkspace, connectAgentStatus } from './lib/agentStatus.js'

let agentPanes = $state({})
const agentByWs = $derived(aggregateByWorkspace(agentPanes, workspaces))
```

初期化箇所（既存の `onMount` / `$effect` に合わせて）:

```js
$effect(() => {
  const stop = connectAgentStatus((p) => (agentPanes = p))
  return stop
})
```

- [ ] **Step 2: ワークスペース一覧の行にバッジを追加**

`.ws-select` 内、レイアウトバッジ `<span class="badge">` の後:

```svelte
{#if agentByWs.get(w.id)}
  {#each agentByWs.get(w.id) as a (a.tool)}
    <span class="agent-badge" title="{a.tool}: 実行中 {a.active} / 許可待ち {a.wait}">
      {a.tool}
      {#if a.active > 0}<span class="agent-st active">●{a.active}</span>{/if}
      {#if a.wait > 0}<span class="agent-st wait">⏸{a.wait}</span>{/if}
    </span>
  {/each}
{/if}
```

- [ ] **Step 3: CSS（App.svelte の style 部、既存バッジ系のトーンに合わせる）**

```css
.agent-badge {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-size: 10px;
  padding: 1px 5px;
  border-radius: 8px;
  background: #2c313a;
  color: #9aa4b2;
}
.agent-st.active { color: #4caf50; }
.agent-st.wait { color: #ffb300; }
```

- [ ] **Step 4: ビルドと動作確認**

```bash
(cd frontend && npm run build)
./scripts/dev.sh check
```

Expected: フロントビルド成功、`go build`/`vet`/`test` すべて成功。

手動確認: `./scripts/dev.sh web` + `./scripts/dev.sh frontend` を起動し、ペインのターミナルで `claude` を起動 → 数秒以内にサイドバーの該当ワークスペース行に `claude ●1` が出る。許可プロンプトで止めると `⏸1` に変わる。終了するとバッジが消える。

- [ ] **Step 5: README 追記とコミット**

README の機能一覧（先頭の箇条書き）に追加:

```markdown
- ワークスペース一覧に claude code / codex の稼働状況を表示（● 実行中 / ⏸ 許可待ち、件数つき・リアルタイム更新）
```

```bash
git add frontend/src/App.svelte README.md
git commit -m "feat(ui): show agent activity badges in the workspace list"
```

---

## Self-Review

- **Spec coverage**: 検出（Task 1,3,4）、状態判定（Task 2）、リアルタイム push（Task 5,6）、フォールバック snapshot（Task 6,7）、pane→workspace 集計とバッジ表示（Task 7,8）— spec の全要件にタスクあり。Windows/リモート除外は Task 3（GOOS ガード）と Task 6（`pid <= 0` スキップ）。
- **Placeholder scan**: 全ステップに実コード・実コマンドあり。
- **Type consistency**: `Proc`/`SessionInfo`/`Snapshot`/`PaneAgent`/`State` は Task 1,2,5 で定義したものを Task 3,6,7 が同名で消費。`PollNow` は Task 6 で watcher.go に追加。
