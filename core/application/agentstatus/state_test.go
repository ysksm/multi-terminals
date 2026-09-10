package agentstatus

import (
	"testing"
	"time"
)

func TestTrackerBlockedWinsOverActivity(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	tr := NewTracker(now.Add(-time.Minute))
	got := tr.Observe(Detection{State: StateBlocked, VisibleBlocker: true}, now, now)
	if got != StateBlocked {
		t.Errorf("state = %q, want blocked", got)
	}
}

func TestTrackerActivityMakesPlainIdleWorking(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	tr := NewTracker(now.Add(-time.Minute))
	// フォールバック idle(ライブ UI 無し)+ 直近出力あり → working
	if got := tr.Observe(Detection{State: StateIdle}, now.Add(-200*time.Millisecond), now); got != StateWorking {
		t.Errorf("state = %q, want working", got)
	}
	// ライブな idle UI(プロンプトボックス)が見えていれば、出力があっても idle
	tr2 := NewTracker(now.Add(-time.Minute))
	if got := tr2.Observe(Detection{State: StateIdle, VisibleIdle: true}, now.Add(-200*time.Millisecond), now); got != StateIdle {
		t.Errorf("visible idle: state = %q, want idle", got)
	}
}

func TestTrackerStartupGraceHoldsUnknown(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	tr := NewTracker(now)
	if got := tr.Observe(Detection{State: StateIdle}, now.Add(-time.Minute), now.Add(time.Second)); got != StateUnknown {
		t.Errorf("within grace: state = %q, want unknown", got)
	}
	// ライブ UI が見えていれば猶予中でも確定する
	if got := tr.Observe(Detection{State: StateWorking, VisibleWorking: true}, now, now.Add(time.Second)); got != StateWorking {
		t.Errorf("visible working within grace: state = %q, want working", got)
	}
	// 猶予を過ぎればフォールバック idle が通る(working からなので連続確認後)
	tr.Observe(Detection{State: StateIdle}, now.Add(-time.Minute), now.Add(10*time.Second))
	if got := tr.Observe(Detection{State: StateIdle}, now.Add(-time.Minute), now.Add(12*time.Second)); got != StateIdle {
		t.Errorf("after grace: state = %q, want idle", got)
	}
}

func TestTrackerPendingIdleConfirmation(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	tr := NewTracker(now.Add(-time.Minute))
	tr.Observe(Detection{State: StateWorking, VisibleWorking: true}, now, now)
	old := now.Add(-time.Minute)
	// 1 回目の素の idle は保留(working のまま)
	if got := tr.Observe(Detection{State: StateIdle}, old, now.Add(time.Second)); got != StateWorking {
		t.Errorf("first plain idle: state = %q, want working (held)", got)
	}
	// 2 回目で確定
	if got := tr.Observe(Detection{State: StateIdle}, old, now.Add(2*time.Second)); got != StateIdle {
		t.Errorf("second plain idle: state = %q, want idle", got)
	}
	// ライブな idle UI なら即確定
	tr.Observe(Detection{State: StateWorking, VisibleWorking: true}, now, now.Add(3*time.Second))
	if got := tr.Observe(Detection{State: StateIdle, VisibleIdle: true}, old, now.Add(4*time.Second)); got != StateIdle {
		t.Errorf("visible idle: state = %q, want idle", got)
	}
}

func TestTrackerSkipStateUpdateKeepsPrevious(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	tr := NewTracker(now.Add(-time.Minute))
	tr.Observe(Detection{State: StateBlocked, VisibleBlocker: true}, now, now)
	if got := tr.Observe(Detection{State: StateUnknown, SkipStateUpdate: true}, now, now.Add(time.Second)); got != StateBlocked {
		t.Errorf("skip: state = %q, want blocked (kept)", got)
	}
}
