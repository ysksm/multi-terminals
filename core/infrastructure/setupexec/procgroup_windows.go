//go:build windows

package setupexec

import "os/exec"

// setProcessGroup は windows では既定の Kill に任せる。
func setProcessGroup(cmd *exec.Cmd) {}
