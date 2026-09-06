//go:build !windows

package server

import (
	"os/exec"
	"syscall"
)

func getSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func killPIDGroup(cmd *exec.Cmd, pid int, sig syscall.Signal) error {
	return syscall.Kill(-pid, sig)
}
