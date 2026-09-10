package agentstatus

import "time"

// State はペイン内エージェントの状態(herdr の AgentState と同じ 4 値)。
type State string

const (
	// StateIdle: エージェントは入力待ち(プロンプトが見えている・作業完了)。
	StateIdle State = "idle"
	// StateWorking: エージェントが処理中。
	StateWorking State = "working"
	// StateBlocked: 許可・質問などの UI を出して人の応答待ちで止まっている。
	StateBlocked State = "blocked"
	// StateUnknown: エージェントは居るが画面から状態を確信できない。
	StateUnknown State = "unknown"
)

// ActivityWindow: 直近この時間内に PTY 出力があれば「動いている」とみなす。
// herdr と同様に、画面にライブな idle/blocked の UI が見えていない限り
// PTY の活動を working の根拠にする。
const ActivityWindow = 1500 * time.Millisecond

// StartupGrace: エージェントのプロセスを検出してからこの時間は、画面に
// ライブな UI(visible_*)が見えない限り unknown のままにする。起動直後の
// 画面(前のシェル出力や起動バナー)を誤判定しないため。
const StartupGrace = 3 * time.Second

// PendingIdleConfirmations: working → (ライブ UI の無い)idle の遷移は、
// 連続してこの回数観測されるまで公開しない。ターン間の一瞬の空白で
// idle/working がちらつくのを抑える。
const PendingIdleConfirmations = 2

// Tracker は 1 ペイン分の判定履歴。画面検知の結果と PTY の活動を突き合わせ、
// herdr の agent_detection.rs に倣って安定化した状態を返す。
type Tracker struct {
	state        State
	firstSeen    time.Time
	pendingIdle  int
	lastActivity time.Time
}

// NewTracker はエージェントを初めて検出した時刻 now で Tracker を作る。
func NewTracker(now time.Time) *Tracker {
	return &Tracker{state: StateUnknown, firstSeen: now}
}

// State は現在公開している状態。
func (t *Tracker) State() State { return t.state }

// Observe は今回の画面検知結果 det と最終出力時刻を取り込み、公開する状態を返す。
func (t *Tracker) Observe(det Detection, lastOutput, now time.Time) State {
	if det.SkipStateUpdate {
		// トランスクリプト表示などライブ状態を映していない画面。前回の状態を維持。
		t.pendingIdle = 0
		return t.state
	}
	visible := det.VisibleIdle || det.VisibleBlocker || det.VisibleWorking
	active := !lastOutput.IsZero() && now.Sub(lastOutput) < ActivityWindow

	next := det.State
	switch {
	case next == StateBlocked:
		// 許可待ちは出力の有無に関わらず最優先。
	case active && !det.VisibleIdle && (next == StateIdle || next == StateUnknown):
		// 出力が流れていて、画面にライブな idle UI が無い → 動いている。
		next = StateWorking
	case !visible && now.Sub(t.firstSeen) < StartupGrace:
		next = StateUnknown
	}

	// working → 素の idle は連続確認まで保留。
	if t.state == StateWorking && next == StateIdle && !det.VisibleIdle {
		t.pendingIdle++
		if t.pendingIdle < PendingIdleConfirmations {
			return t.state
		}
	}
	t.pendingIdle = 0
	t.state = next
	return next
}
