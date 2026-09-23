//go:build darwin

package server

import (
	"net"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// hostUptimeSeconds returns wall-clock uptime since the machine booted, read
// from kern.boottime via sysctl. -1 when it cannot be determined: an unknown
// uptime is never reported as zero.
func hostUptimeSeconds() int64 {
	tv, err := unix.SysctlTimeval("kern.boottime")
	if err != nil || tv == nil {
		return -1
	}
	uptime := time.Now().Unix() - int64(tv.Sec)
	if uptime < 0 {
		return -1
	}
	return uptime
}

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

func availableDiskBytesPlatform(path string) (int64, error) {
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

func hostDiskPartitionsPlatform() []DiskPartition {
	partitions := []DiskPartition{}
	rootStat := unix.Statfs_t{}
	if err := unix.Statfs("/", &rootStat); err == nil {
		total := uint64(rootStat.Blocks) * uint64(rootStat.Bsize) / (1024 * 1024)
		free := uint64(rootStat.Bavail) * uint64(rootStat.Bsize) / (1024 * 1024)
		used := total - free
		var usedPct float64
		if total > 0 {
			usedPct = float64(used) / float64(total) * 100
		}
		partitions = append(partitions, DiskPartition{
			MountPoint: "/",
			Device:     "/",
			FSType:     "rootfs",
			TotalMB:    total,
			UsedMB:     used,
			FreeMB:     free,
			UsedPct:    usedPct,
		})
	}
	return partitions
}

func kernelVersionPlatform() string {
	uts := unix.Utsname{}
	if err := unix.Uname(&uts); err != nil {
		return ""
	}
	b := make([]byte, 0, len(uts.Release))
	for _, v := range uts.Release {
		if v == 0 {
			break
		}
		b = append(b, byte(v))
	}
	return string(b)
}
