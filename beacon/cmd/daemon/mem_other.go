//go:build !linux

package main

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func readMemoryMB() int64 {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "sysctl", "-n", "hw.memsize").Output()
	if err == nil {
		bytes, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		if err == nil && bytes > 0 {
			return bytes / (1024 * 1024)
		}
	}
	return 0
}
