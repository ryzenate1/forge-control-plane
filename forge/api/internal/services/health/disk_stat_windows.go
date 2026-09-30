//go:build windows

package health

import (
	"golang.org/x/sys/windows"
)

func getDiskSpace(path string) (total uint64, free uint64, used uint64, err error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, 0, err
	}
	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes uint64
	err = windows.GetDiskFreeSpaceEx(pathPtr, &freeBytesAvailable, &totalNumberOfBytes, &totalNumberOfFreeBytes)
	if err != nil {
		return 0, 0, 0, err
	}
	total = totalNumberOfBytes
	free = totalNumberOfFreeBytes
	if total > free {
		used = total - free
	}
	return total, free, used, nil
}
