//go:build windows

package server

import (
	"os/exec"
	"syscall"
)

func getSysProcAttr() *syscall.SysProcAttr {
	return nil
}

func killPIDGroup(cmd *exec.Cmd, pid int, sig syscall.Signal) error {
	if cmd != nil && cmd.Process != nil {
		return cmd.Process.Kill()
	}
	return nil
}
