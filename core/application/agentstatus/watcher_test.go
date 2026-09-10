package agentstatus

import (
	"errors"
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
	prompt := "done.\n────────────\n❯ \n────────────"
	sessions := []SessionInfo{{PaneID: "pane-1", Pid: 10, Screen: prompt, LastOutput: now.Add(-5 * time.Second)}}

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
	want := Snapshot{"pane-1": {{Tool: "claude", State: StateIdle, Rule: "live_prompt_box"}}}
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

	// blocked へ遷移(許可プロンプト) → push される
	sessions = []SessionInfo{{PaneID: "pane-1", Pid: 10, Screen: "Bash command\nDo you want to proceed?\n❯ 1. Yes\n  2. No", LastOutput: now.Add(-5 * time.Second)}}
	w.poll(now.Add(2 * time.Second))
	select {
	case got := <-ch:
		if got["pane-1"][0].State != StateBlocked {
			t.Fatalf("state = %q, want blocked", got["pane-1"][0].State)
		}
	case <-time.After(time.Second):
		t.Fatal("no broadcast on state change")
	}
}

func TestWatcherActivityWithoutScreenIsWorking(t *testing.T) {
	// 画面モデルの無いセッションでも、出力が流れていれば working になる。
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	procs := []Proc{{PID: 10, PPID: 1, Command: "-zsh"}, {PID: 11, PPID: 10, Command: "codex"}}
	w := NewWatcher(
		func() []SessionInfo {
			return []SessionInfo{{PaneID: "p", Pid: 10, LastOutput: now.Add(-100 * time.Millisecond)}}
		},
		func() ([]Proc, error) { return procs, nil },
		time.Hour,
	)
	w.poll(now)
	if got := w.Current()["p"][0].State; got != StateWorking {
		t.Fatalf("state = %q, want working", got)
	}
}

func TestWatcherScanErrorYieldsEmpty(t *testing.T) {
	w := NewWatcher(
		func() []SessionInfo { return []SessionInfo{{PaneID: "p", Pid: 10}} },
		func() ([]Proc, error) { return nil, errScan },
		time.Hour,
	)
	w.poll(time.Now())
	if len(w.Current()) != 0 {
		t.Fatalf("expected empty snapshot on scan error, got %v", w.Current())
	}
}

func TestWatcherSnapshotsAreDefensivelyCopied(t *testing.T) {
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	procs := []Proc{{PID: 10, PPID: 1, Command: "-zsh"}, {PID: 11, PPID: 10, Command: "claude"}}
	w := NewWatcher(
		func() []SessionInfo { return []SessionInfo{{PaneID: "p", Pid: 10, OSCTitle: "✳ Claude Code"}} },
		func() ([]Proc, error) { return procs, nil },
		time.Hour,
	)
	w.poll(now)
	got := w.Current()
	got["p"][0].Tool = "mutated"
	if w.Current()["p"][0].Tool != "claude" {
		t.Fatal("Current() must return a copy")
	}
}

var errScan = errors.New("scan failed")
