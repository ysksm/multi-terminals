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
	// 別ルート配下の claude(20→21) は root=10 では検出されないこと
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
