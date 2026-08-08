//go:build windows

package backup

import "golang.org/x/sys/windows"

func availableDiskBytes(path string) (int64, error) {
	var available, total, free uint64
	root, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	if err := windows.GetDiskFreeSpaceEx(root, &available, &total, &free); err != nil {
		return 0, err
	}
	const maxInt64 = uint64(^uint64(0) >> 1)
	if available > maxInt64 {
		return int64(maxInt64), nil
	}
	return int64(available), nil
}
