//go:build darwin

package server

import (
	"net"
	"strings"

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

func freeMemoryMB() uint64 {
	pageSize, err := unix.SysctlUint64("hw.pagesize")
	if err != nil {
		return 0
	}
	freePages, err := unix.SysctlUint64("vm.page_free_count")
	if err != nil {
		return 0
	}
	return (freePages * pageSize) / (1024 * 1024)
}

func freeDiskMB(path string) uint64 {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize) / (1024 * 1024)
}

func cpuModelPlatform() string {
	value, err := unix.Sysctl("machdep.cpu.brand_string")
	if err != nil {
		return ""
	}
	return value
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
