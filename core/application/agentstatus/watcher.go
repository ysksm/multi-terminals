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

// PollNow は即時に 1 回ポーリングする(初期化・テスト用)。
func (w *Watcher) PollNow() { w.poll(time.Now()) }

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
		case ch <- cloneSnapshot(snap):
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
	return cloneSnapshot(w.current)
}

// Subscribe returns the current snapshot, a channel receiving subsequent
// changed snapshots, and a cancel function releasing the subscription.
func (w *Watcher) Subscribe() (Snapshot, <-chan Snapshot, func()) {
	ch := make(chan Snapshot, 8)
	w.mu.Lock()
	w.subs[ch] = struct{}{}
	snap := cloneSnapshot(w.current)
	w.mu.Unlock()
	cancel := func() {
		w.mu.Lock()
		delete(w.subs, ch)
		w.mu.Unlock()
	}
	return snap, ch, cancel
}

// cloneSnapshot returns a deep copy so callers cannot mutate Watcher's
// internal state or snapshots received by other subscribers.
func cloneSnapshot(snap Snapshot) Snapshot {
	cloned := make(Snapshot, len(snap))
	for paneID, agents := range snap {
		cloned[paneID] = append([]PaneAgent(nil), agents...)
	}
	return cloned
}
