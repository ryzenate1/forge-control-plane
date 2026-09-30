//go:build !linux

package main

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// readMemoryMB returns total physical RAM in megabytes. Every failure mode is
// returned as an error so callers can distinguish "unknown" from a real reading
// of zero.
func readMemoryMB() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, fmt.Errorf("read hw.memsize: %w", err)
	}
	raw := strings.TrimSpace(string(out))
	bytes, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse hw.memsize %q: %w", raw, err)
	}
	if bytes <= 0 {
		return 0, fmt.Errorf("hw.memsize reported %d bytes", bytes)
	}
	return bytes / (1024 * 1024), nil
}
