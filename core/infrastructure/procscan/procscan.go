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
