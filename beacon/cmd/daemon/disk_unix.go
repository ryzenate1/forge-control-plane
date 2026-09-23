//go:build !windows

package main

import (
	"errors"

	"golang.org/x/sys/unix"
)

func readDiskCapacity(dataDir string) (availableGB, totalGB uint64, err error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(dataDir, &stat); err != nil {
		return 0, 0, err
	}
	availableGB = stat.Bavail * uint64(stat.Bsize) / (1024 * 1024 * 1024)
	totalGB = stat.Blocks * uint64(stat.Bsize) / (1024 * 1024 * 1024)
	return availableGB, totalGB, nil
}

// readDiskMB returns the available bytes on the filesystem holding dataDir, in
// megabytes. Failure is returned as an error so an unreadable capacity is never
// reported as a healthy zero.
func readDiskMB(dataDir string) (int64, error) {
	if dataDir == "" {
		return 0, errors.New("no data directory configured")
	}
	var stat unix.Statfs_t
	if err := unix.Statfs(dataDir, &stat); err != nil {
		return 0, err
	}
	bytes := uint64(stat.Bavail) * uint64(stat.Bsize)
	return int64(bytes / (1024 * 1024)), nil
}
