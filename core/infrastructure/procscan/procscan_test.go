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
