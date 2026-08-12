//go:build windows

package build

import (
	"os"
	"os/exec"
)

func setSysProcAttr(cmd *exec.Cmd) {
}

func killPID(pid int) {
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}
