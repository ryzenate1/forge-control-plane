//go:build linux

package main

import "syscall"

// readMemoryMB returns total physical RAM in megabytes using syscall.Sysinfo.
func readMemoryMB() int64 {
	var info syscall.Sysinfo_t
	if err := syscall.Sysinfo(&info); err != nil {
		return 0
	}
	totalBytes := info.Totalram * uint64(info.Unit)
	return int64(totalBytes / (1024 * 1024))
}
