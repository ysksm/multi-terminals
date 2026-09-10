//go:build !windows

package setupexec

import (
	"os/exec"
	"syscall"
)

// setProcessGroup は子プロセスを独立したプロセスグループにし、ctx キャンセル時に
// グループごと SIGKILL する(シェル経由で起動した孫プロセスも道連れにする)。
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
