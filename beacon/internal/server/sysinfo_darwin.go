//go:build darwin

package server

import (
	"golang.org/x/sys/unix"
)

func totalSystemMemoryMB() uint64 {
	value, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0
	}
	return value / (1024 * 1024)
}

func totalDiskMB(path string) uint64 {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0
	}
	return uint64(stat.Blocks) * uint64(stat.Bsize) / (1024 * 1024)
}

func freeDiskMB(path string) uint64 {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize) / (1024 * 1024)
}
