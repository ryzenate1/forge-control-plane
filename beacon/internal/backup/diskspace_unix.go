//go:build !windows

package backup

import "golang.org/x/sys/unix"

func availableDiskBytes(path string) (int64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, err
	}
	free := uint64(stat.Bavail) * uint64(stat.Bsize)
	const maxInt64 = uint64(^uint64(0) >> 1)
	if free > maxInt64 {
		return int64(maxInt64), nil
	}
	return int64(free), nil
}
