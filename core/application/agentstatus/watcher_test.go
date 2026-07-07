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
