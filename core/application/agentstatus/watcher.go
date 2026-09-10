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
	// Rule は状態の根拠になった画面ルール ID(診断用。フォールバック時は空)。
	Rule string `json:"rule,omitempty"`
}

// Snapshot は paneID → 検出エージェント一覧。検出ゼロのペインは含めない。
type Snapshot map[string][]PaneAgent

// SessionInfo は 1 ペインの判定に必要なライブセッション情報。
type SessionInfo struct {
	PaneID      string
	Pid         int
	Screen      string    // 描画済み画面テキスト(画面モデルが無ければ "")
	OSCTitle    string    // 直近の OSC 0/2 タイトル
	OSCProgress string    // 直近の OSC 9 ペイロード
	LastOutput  time.Time // 最終出力時刻
}

// Source は監視対象セッションを列挙する(web アダプタが Registry から供給)。
type Source func() []SessionInfo

// Scanner は全プロセスのスナップショットを返す(procscan.Snapshot が実装)。
type Scanner func() ([]Proc, error)

// DefaultInterval はポーリング周期の既定値。
const DefaultInterval = 1500 * time.Millisecond

// Watcher は周期的にエージェント稼働状況を算出し、変化時のみ購読者へ
// push する。全メソッド並行安全。
type Watcher struct {
	source   Source
	scan     Scanner
	interval time.Duration

	mu       sync.Mutex
	current  Snapshot
	subs     map[chan Snapshot]struct{}
	trackers map[string]*Tracker // paneID+"\x00"+tool → 判定履歴

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
		trackers: make(map[string]*Tracker),
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

func trackerKey(paneID, tool string) string { return paneID + "\x00" + tool }

func (w *Watcher) compute(now time.Time) Snapshot {
	snap := Snapshot{}
	procs, err := w.scan()
	if err != nil || len(procs) == 0 {
		w.mu.Lock()
		w.trackers = make(map[string]*Tracker)
		w.mu.Unlock()
		return snap
	}
	alive := make(map[string]struct{})
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, si := range w.source() {
		if si.Pid <= 0 {
			continue
		}
		tools := DetectTools(procs, si.Pid)
		if len(tools) == 0 {
			continue
		}
		in := Input{Screen: si.Screen, OSCTitle: si.OSCTitle, OSCProgress: si.OSCProgress}
		agents := make([]PaneAgent, 0, len(tools))
		for _, tool := range tools {
			key := trackerKey(si.PaneID, tool)
			alive[key] = struct{}{}
			tr, ok := w.trackers[key]
			if !ok {
				tr = NewTracker(now)
				w.trackers[key] = tr
			}
			det := Detection{State: StateIdle}
			if m := ManifestFor(tool); m != nil {
				det = m.Detect(in)
			}
			state := tr.Observe(det, si.LastOutput, now)
			agents = append(agents, PaneAgent{Tool: tool, State: state, Rule: det.MatchedRule})
		}
		snap[si.PaneID] = agents
	}
	// 消えたエージェントの履歴は破棄(再起動時は起動猶予からやり直す)。
	for key := range w.trackers {
		if _, ok := alive[key]; !ok {
			delete(w.trackers, key)
		}
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
