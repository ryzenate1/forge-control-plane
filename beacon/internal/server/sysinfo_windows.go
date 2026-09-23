//go:build windows

package server

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modkernel32              = syscall.NewLazyDLL("kernel32.dll")
	procGlobalMemoryStatusEx = modkernel32.NewProc("GlobalMemoryStatusEx")
	procGetTickCount64       = modkernel32.NewProc("GetTickCount64")
)

// hostUptimeSeconds returns wall-clock uptime since the machine booted via
// GetTickCount64 (milliseconds since boot). -1 when it cannot be determined:
// an unknown uptime is never reported as zero.
func hostUptimeSeconds() int64 {
	if procGetTickCount64.Find() != nil {
		return -1
	}
	ticks, _, callErr := procGetTickCount64.Call()
	if callErr != syscall.Errno(0) {
		return -1
	}
	return int64(ticks) / 1000
}

type memoryStatusEx struct {
	cbSize                  uint32
	dwMemoryLoad            uint32
	ullTotalPhys            uint64
	ullAvailPhys            uint64
	ullTotalPageFile        uint64
	ullAvailPageFile        uint64
	ullTotalVirtual         uint64
	ullAvailVirtual         uint64
	ullAvailExtendedVirtual uint64
}

func getMemoryStatus() *memoryStatusEx {
	var ms memoryStatusEx
	ms.cbSize = uint32(unsafe.Sizeof(ms))
	ret, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&ms)))
	if ret == 0 {
		return nil
	}
	return &ms
}

func totalSystemMemoryMB() uint64 {
	ms := getMemoryStatus()
	if ms == nil {
		return 0
	}
	return ms.ullTotalPhys / (1024 * 1024)
}

func freeMemoryMB() uint64 {
	ms := getMemoryStatus()
	if ms == nil {
		return 0
	}
	return ms.ullAvailPhys / (1024 * 1024)
}

func totalDiskMB(path string) uint64 {
	total, _, _ := getDiskSpaceBytes(path)
	return total / (1024 * 1024)
}

func freeDiskMB(path string) uint64 {
	_, free, _ := getDiskSpaceBytes(path)
	return free / (1024 * 1024)
}

func getDiskSpaceBytes(path string) (total uint64, free uint64, avail uint64) {
	cleanPath := "C:\\"
	if path != "" && path != "/" {
		vol := filepath.VolumeName(path)
		if vol != "" {
			cleanPath = vol + "\\"
		}
	}
	pathPtr, err := windows.UTF16PtrFromString(cleanPath)
	if err != nil {
		pathPtr, _ = windows.UTF16PtrFromString("C:\\")
	}
	var freeBytes, totalBytes, totalFreeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(pathPtr, &freeBytes, &totalBytes, &totalFreeBytes); err != nil {
		return 0, 0, 0
	}
	return totalBytes, totalFreeBytes, freeBytes
}

func availableDiskBytesPlatform(path string) (int64, error) {
	_, _, avail := getDiskSpaceBytes(path)
	const maxInt64 = uint64(^uint64(0) >> 1)
	if avail > maxInt64 {
		return int64(maxInt64), nil
	}
	return int64(avail), nil
}

func hostDiskPartitionsPlatform() []DiskPartition {
	total, free, _ := getDiskSpaceBytes("C:\\")
	used := uint64(0)
	if total > free {
		used = total - free
	}
	var usedPct float64
	if total > 0 {
		usedPct = float64(used) / float64(total) * 100
	}
	return []DiskPartition{
		{
			MountPoint: "C:\\",
			Device:     "C:",
			FSType:     "NTFS",
			TotalMB:    total / (1024 * 1024),
			UsedMB:     used / (1024 * 1024),
			FreeMB:     free / (1024 * 1024),
			UsedPct:    usedPct,
		},
	}
}

func kernelVersionPlatform() string {
	v := windows.RtlGetVersion()
	if v != nil {
		return fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
	}
	return os.Getenv("OS")
}

func cpuModelPlatform() string {
	return os.Getenv("PROCESSOR_IDENTIFIER")
}

func netInterfacesPlatform() ([]NetworkInterface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	result := make([]NetworkInterface, 0, len(ifaces))
	for _, iface := range ifaces {
		entry := NetworkInterface{
			Name:   iface.Name,
			MAC:    iface.HardwareAddr.String(),
			Status: "unknown",
		}
		if iface.Flags&net.FlagUp != 0 {
			entry.Status = "up"
		}
		addrs, err := iface.Addrs()
		if err == nil {
			ipStrs := make([]string, 0, len(addrs))
			for _, addr := range addrs {
				ipStrs = append(ipStrs, addr.String())
			}
			entry.IPs = strings.Join(ipStrs, ", ")
		}
		result = append(result, entry)
	}
	return result, nil
}

func processListPlatform() ([]ProcessEntry, error) {
	return []ProcessEntry{}, nil
}
