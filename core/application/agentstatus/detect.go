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
