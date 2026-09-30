//go:build !windows

package health

import "golang.org/x/sys/unix"

func getDiskSpace(path string) (total uint64, free uint64, used uint64, err error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, 0, 0, err
	}
	total = stat.Blocks * uint64(stat.Bsize)
	free = stat.Bfree * uint64(stat.Bsize)
	used = total - free
	return total, free, used, nil
}
