//go:build linux

package main

import (
	"errors"
	"syscall"
)

// readMemoryMB returns total physical RAM in megabytes using syscall.Sysinfo.
// A failed syscall, or a report of zero bytes, is returned as an error: unknown
// capacity must never be flattened into 0, which a control plane would record as
// a node that genuinely has no memory.
func readMemoryMB() (int64, error) {
	var info syscall.Sysinfo_t
	if err := syscall.Sysinfo(&info); err != nil {
		return 0, err
	}
	totalBytes := info.Totalram * uint64(info.Unit)
	if totalBytes == 0 {
		return 0, errors.New("sysinfo reported zero total memory")
	}
	return int64(totalBytes / (1024 * 1024)), nil
}
