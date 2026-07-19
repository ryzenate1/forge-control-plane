//go:build linux

package server

import (
	"golang.org/x/sys/unix"
)

func totalSystemMemoryMB() uint64 {
	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err != nil {
		return 0
	}
	return uint64(info.Totalram) / (1024 * 1024)
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
