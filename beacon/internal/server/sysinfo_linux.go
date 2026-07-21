//go:build linux

package server

import (
	"os"
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
	return processes, nil
}
