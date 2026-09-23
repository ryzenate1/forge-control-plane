//go:build linux

package server

import (
	"os"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

func totalSystemMemoryMB() uint64 {
	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err != nil {
		return 0
	}
	return uint64(info.Totalram) / (1024 * 1024)
}

// hostUptimeSeconds returns wall-clock uptime since the machine booted.
// It is deliberately distinct from the daemon's own uptime: a long-running
// host that restarted Beacon shows a large host value and a small daemon one,
// and conflating them hides exactly the gap operators look for.
func hostUptimeSeconds() int64 {
	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err != nil {
		return -1
	}
	return int64(info.Uptime)
}

func totalDiskMB(path string) uint64 {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0
	}
	return uint64(stat.Blocks) * uint64(stat.Bsize) / (1024 * 1024)
}

func freeMemoryMB() uint64 {
	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err != nil {
		return 0
	}
	return uint64(info.Freeram) / (1024 * 1024)
}

func freeDiskMB(path string) uint64 {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize) / (1024 * 1024)
}

func cpuModelPlatform() string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func netInterfacesPlatform() ([]NetworkInterface, error) {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil, err
	}
	ifaces := []NetworkInterface{}
	for _, e := range entries {
		name := e.Name()
		iface := NetworkInterface{Name: name}

		speedData, err := os.ReadFile("/sys/class/net/" + name + "/speed")
		if err == nil {
			speed, _ := strconv.Atoi(strings.TrimSpace(string(speedData)))
			iface.Speed = speed
		}

		operData, err := os.ReadFile("/sys/class/net/" + name + "/operstate")
		if err == nil {
			status := strings.TrimSpace(string(operData))
			if status == "up" {
				iface.Status = "up"
			} else if status == "down" {
				iface.Status = "down"
			} else {
				iface.Status = "unknown"
			}
		}

		ifaces = append(ifaces, iface)
	}
	return ifaces, nil
}

func processListPlatform() ([]ProcessEntry, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	processes := []ProcessEntry{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		statusData, err := os.ReadFile("/proc/" + e.Name() + "/status")
		if err != nil {
			continue
		}
		proc := ProcessEntry{PID: pid}
		for _, line := range strings.Split(string(statusData), "\n") {
			if strings.HasPrefix(line, "Name:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					proc.Name = strings.TrimSpace(parts[1])
				}
			}
			if strings.HasPrefix(line, "State:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					stateParts := strings.SplitN(strings.TrimSpace(parts[1]), " ", 2)
					proc.State = stateParts[0]
				}
			}
		}
		if proc.Name == "" {
			proc.Name = e.Name()
		}
		processes = append(processes, proc)
	}
	// The dashboard lists processes in PID order; an unsorted read of /proc
	// jitters between refreshes because directory order is not numeric order.
	sort.Slice(processes, func(i, j int) bool { return processes[i].PID < processes[j].PID })
	return processes, nil
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
