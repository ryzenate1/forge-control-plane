//go:build !windows

package build

import (
	"os/exec"
	"syscall"
)

func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killPID(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGTERM)
}
