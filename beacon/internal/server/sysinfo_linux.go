//go:build linux

package server

import (
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// readMemorySample returns the host memory reading for one request.
//
// /proc/meminfo is preferred because it is the only source that exposes
// MemAvailable (free memory plus the page cache the kernel can reclaim) and the
// swap totals in one place. sysinfo(2) is the fallback: its freeram is strictly
// unused pages, so a machine whose RAM is holding reclaimable cache looks full
// to a scheduler fed that figure.
func readMemorySample() (memorySample, error) {
	var memErr error
	if values, err := readProcMeminfo(); err != nil {
		memErr = err
	} else if sample, ok := sampleFromMeminfo(values); ok {
		return sample, nil
	}

	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err != nil {
		if memErr != nil {
			return memorySample{}, fmt.Errorf("read system memory: /proc/meminfo: %v; sysinfo: %w", memErr, err)
		}
		return memorySample{}, fmt.Errorf("read system memory with sysinfo: %w", err)
	}
	unit := uint64(info.Unit)
	if unit == 0 {
		unit = 1
	}
	total := bytesToMB(scaleU64(uint64(info.Totalram), unit))
	if total == 0 {
		return memorySample{}, errors.New("sysinfo reported a memory total of zero")
	}
	available := bytesToMB(scaleU64(uint64(info.Freeram), unit))
	swapTotal := bytesToMB(scaleU64(uint64(info.Totalswap), unit))
	swapFree := bytesToMB(scaleU64(uint64(info.Freeswap), unit))
	sample := memorySample{
		TotalMB:     total,
		AvailableMB: available,
		SwapTotalMB: swapTotal,
		SwapFreeMB:  swapFree,
	}
	if swapTotal > swapFree {
		sample.SwapUsedMB = swapTotal - swapFree
	}
	return sample, nil
}

// readProcMeminfo parses /proc/meminfo into kilobyte-per-field values.
func readProcMeminfo() (map[string]uint64, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return nil, err
	}
	values := make(map[string]uint64)
	for _, line := range strings.Split(string(data), "\n") {
		name, rest, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}
		amount, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			continue
		}
		values[strings.TrimSpace(name)] = amount
	}
	if len(values) == 0 {
		return nil, errors.New("/proc/meminfo contained no readable fields")
	}
	return values, nil
}

// sampleFromMeminfo converts a parsed /proc/meminfo into a memorySample. ok is
// false when the file did not carry a usable MemTotal, leaving the caller to the
// fallback rather than to a zero.
func sampleFromMeminfo(values map[string]uint64) (memorySample, bool) {
	totalKB := values["MemTotal"]
	if totalKB == 0 {
		return memorySample{}, false
	}
	availableKB, hasAvailable := values["MemAvailable"]
	if !hasAvailable {
		// Pre-3.14 kernels expose no MemAvailable. Reclaimable cache is the
		// closest honest stand-in; MemFree alone would understate what a new
		// workload can be given.
		availableKB = values["MemFree"] + values["Cached"] + values["SReclaimable"]
		if shared := values["Shmem"]; shared > availableKB {
			availableKB = 0
		} else {
			availableKB -= shared
		}
	}
	if availableKB > totalKB {
		availableKB = totalKB
	}
	swapTotalKB := values["SwapTotal"]
	swapFreeKB := values["SwapFree"]
	if swapFreeKB > swapTotalKB {
		swapFreeKB = swapTotalKB
	}
	return memorySample{
		TotalMB:     totalKB / 1024,
		AvailableMB: availableKB / 1024,
		SwapTotalMB: swapTotalKB / 1024,
		SwapFreeMB:  swapFreeKB / 1024,
		SwapUsedMB:  (swapTotalKB - swapFreeKB) / 1024,
	}, true
}

// totalSystemMemoryMB reports installed RAM in MiB, or 0 when it cannot be read.
//
// Deprecated: 0 is indistinguishable from a machine with no memory, and callers
// such as the capability report hand that 0 straight to the control plane. Use
// readMemorySample, which returns an error instead.
func totalSystemMemoryMB() uint64 {
	sample, err := readMemorySample()
	if err != nil {
		return 0
	}
	return sample.TotalMB
}

// hostUptimeSeconds returns wall-clock uptime since the machine booted.
// It is deliberately distinct from the daemon's own uptime: a long-running
// host that restarted Beacon shows a large host value and a small daemon one,
// and conflating them hides exactly the gap operators look for.
// -1 means unknown, never zero.
func hostUptimeSeconds() int64 {
	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err != nil {
		return -1
	}
	return int64(info.Uptime)
}

// totalDiskMB reports the size in MiB of the filesystem holding path, or 0 when
// it cannot be read.
//
// Deprecated: prefer partitionFromStatfs through hostDiskPartitionsPlatform,
// which surfaces the error instead of a zero.
func totalDiskMB(path string) uint64 {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0
	}
	return bytesToMB(scaleU64(uint64(stat.Blocks), uint64(stat.Bsize)))
}

// freeMemoryMB reports available (reclaimable-aware) memory in MiB, or 0 when it
// cannot be read.
//
// Deprecated: use readMemorySample, which returns an error instead of 0.
func freeMemoryMB() uint64 {
	sample, err := readMemorySample()
	if err != nil {
		return 0
	}
	return sample.AvailableMB
}

// freeDiskMB reports the bytes an unprivileged process can actually write, in
// MiB, or 0 when it cannot be read.
//
// Deprecated: availableDiskBytesPlatform returns the same figure with an error
// instead of a zero.
func freeDiskMB(path string) uint64 {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0
	}
	return bytesToMB(scaleU64(uint64(stat.Bavail), uint64(stat.Bsize)))
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

// netInterfacesPlatform lists the interfaces the host exposes. /sys/class/net
// carries the operational state and link speed; the address and hardware
// address come from getifaddrs through the standard library, which is the only
// way a Linux node reports its own IPs at all.
func netInterfacesPlatform() ([]NetworkInterface, error) {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return nil, err
	}
	addresses, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("read interface addresses: %w", err)
	}
	byName := make(map[string]net.Interface, len(addresses))
	for _, iface := range addresses {
		byName[iface.Name] = iface
	}

	ifaces := make([]NetworkInterface, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		iface := NetworkInterface{Name: name, Status: "unknown"}

		speedData, err := os.ReadFile("/sys/class/net/" + name + "/speed")
		if err == nil {
			// Drivers report -1 for "no link" and 0 for "unknown speed"; both are
			// reported as 0 here because the field has no third value.
			if speed, convErr := strconv.Atoi(strings.TrimSpace(string(speedData))); convErr == nil && speed > 0 {
				iface.Speed = speed
			}
		}

		operData, err := os.ReadFile("/sys/class/net/" + name + "/operstate")
		if err == nil {
			switch status := strings.TrimSpace(string(operData)); status {
			case "up", "lowerup":
				iface.Status = "up"
			case "down", "dormant", "notpresent":
				iface.Status = "down"
			default:
				iface.Status = "unknown"
			}
		}

		if live, ok := byName[name]; ok {
			iface.MAC = live.HardwareAddr.String()
			if addrs, addrErr := live.Addrs(); addrErr == nil {
				ipStrs := make([]string, 0, len(addrs))
				for _, addr := range addrs {
					ipStrs = append(ipStrs, addr.String())
				}
				iface.IPs = strings.Join(ipStrs, ", ")
			}
		}

		ifaces = append(ifaces, iface)
	}
	sort.Slice(ifaces, func(i, j int) bool { return ifaces[i].Name < ifaces[j].Name })
	return ifaces, nil
}

// pseudoFilesystems are RAM- or kernel-backed mounts whose capacity says nothing
// about the disk a workload would land on.
var pseudoFilesystems = map[string]struct{}{
	"autofs": {}, "binfmt_misc": {}, "bpf": {}, "cgroup": {}, "cgroup2": {},
	"configfs": {}, "debugfs": {}, "devpts": {}, "devtmpfs": {}, "efivarfs": {},
	"fuse:gvfs-fuse": {}, "hugetlbfs": {}, "mqueue": {}, "nsfs": {}, "proc": {},
	"pstore": {}, "ramfs": {}, "rpc_pipefs": {}, "securityfs": {}, "selinuxfs": {},
	"sysfs": {}, "tracefs": {},

	// tmpfs is listed separately below because its FSType string is plain "tmpfs".
}

func isPseudoFilesystem(fsType string) bool {
	if _, ok := pseudoFilesystems[fsType]; ok {
		return true
	}
	return fsType == "tmpfs" || strings.HasPrefix(fsType, "overlay") || strings.HasPrefix(fsType, "squashfs")
}

// mountEntry is one block-backed row of the mount table.
type mountEntry struct {
	mount  string
	device string
	fsType string
}

// mountEntriesFromProcMounts returns the real (block-backed) mounts in
// /proc/mounts. A missing or unreadable file is an error the caller may ignore,
// because the explicit probe paths are still reportable on their own.
func mountEntriesFromProcMounts() ([]mountEntry, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil, err
	}
	var entries []mountEntry
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		device, mount, fsType := fields[0], unescapeMount(fields[1]), fields[2]
		if isPseudoFilesystem(fsType) {
			continue
		}
		entries = append(entries, mountEntry{mount: mount, device: device, fsType: fsType})
	}
	return entries, nil
}

func unescapeMount(value string) string {
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return replacer.Replace(value)
}

// partitionFromStatfs builds one DiskPartition from a mounted filesystem.
//
// Used and Free are deliberately not complements of Total: free is Bavail, the
// space an unprivileged process can write, while used is Blocks minus Bfree.
// The difference is the filesystem's reserved-block pool, and folding it into
// "used" would make a healthy disk look fuller than any workload can find it.
func partitionFromStatfs(mount, device, fsType string, stat unix.Statfs_t) DiskPartition {
	size := uint64(stat.Bsize)
	total := bytesToMB(scaleU64(uint64(stat.Blocks), size))
	free := bytesToMB(scaleU64(uint64(stat.Bavail), size))
	freeAll := bytesToMB(scaleU64(uint64(stat.Bfree), size))
	var used uint64
	if total > freeAll {
		used = total - freeAll
	}
	var usedPct float64
	if total > 0 {
		usedPct = float64(used) / float64(total) * 100
	}
	return DiskPartition{
		MountPoint: mount,
		Device:     device,
		FSType:     fsType,
		TotalMB:    total,
		UsedMB:     used,
		FreeMB:     free,
		UsedPct:    usedPct,
	}
}

// hostDiskPartitionsPlatform reports every real filesystem the node has, with
// the caller's probe paths first so the data root is never missing.
func hostDiskPartitionsPlatform(paths []string) ([]DiskPartition, error) {
	type candidate struct {
		mount  string
		device string
		fsType string
	}
	candidates := make([]candidate, 0, 16)
	for _, path := range paths {
		if strings.TrimSpace(path) != "" {
			candidates = append(candidates, candidate{mount: path})
		}
	}
	// The mount table adds the filesystems a workload could be placed on that
	// are not the root or the data directory; failing to read it is not fatal,
	// because the probe paths above are still reportable.
	var mountTable []mountEntry
	if entries, err := mountEntriesFromProcMounts(); err == nil {
		mountTable = entries
		for _, entry := range entries {
			candidates = append(candidates, candidate{mount: entry.mount, device: entry.device, fsType: entry.fsType})
		}
	}
	if len(candidates) == 0 {
		return nil, errors.New("no mount point could be discovered")
	}

	seenFS := map[[2]int64]struct{}{}
	seenPath := map[string]struct{}{}
	partitions := make([]DiskPartition, 0, len(candidates))
	var firstErr error
	for _, entry := range candidates {
		if _, done := seenPath[entry.mount]; done {
			continue
		}
		seenPath[entry.mount] = struct{}{}
		var stat unix.Statfs_t
		if err := unix.Statfs(entry.mount, &stat); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("statfs %s: %w", entry.mount, err)
			}
			continue
		}
		key := [2]int64{stat.Fsid.Val[0], stat.Fsid.Val[1]}
		if _, dup := seenFS[key]; dup {
			continue
		}
		seenFS[key] = struct{}{}
		// Deduplicating by filesystem id can drop the mount-table row that
		// carried the device name, because the probe path was seen first.
		// Recovering it from the table keeps the device column honest instead of
		// echoing the mount point back as a device.
		device, fsType := entry.device, entry.fsType
		if device == "" || fsType == "" {
			for _, known := range mountTable {
				if known.mount != entry.mount {
					continue
				}
				device, fsType = known.device, known.fsType
				break
			}
		}
		if device == "" {
			device = entry.mount
		}
		partitions = append(partitions, partitionFromStatfs(entry.mount, device, fsType, stat))
	}
	if len(partitions) == 0 {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, errors.New("no mounted filesystem could be read")
	}
	sort.Slice(partitions, func(i, j int) bool { return partitions[i].MountPoint < partitions[j].MountPoint })
	return partitions, nil
}

func availableDiskBytesPlatform(path string) (int64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, err
	}
	free := scaleU64(uint64(stat.Bavail), uint64(stat.Bsize))
	if free > math.MaxInt64 {
		return math.MaxInt64, nil
	}
	return int64(free), nil
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

// processListPlatform lists host processes with the resource use the dashboard
// advertises. Name and State come from /proc/<pid>/status, resident memory from
// the same file, and CPU from /proc/<pid>/stat.
//
// The CPU figure is the same average ps(1) prints: jiffies of user+system time
// divided by the process's own lifetime in seconds, then by HZ. HZ is read from
// the kernel rather than assumed, because a hardcoded 100 silently mis-scales
// every percentage on a 250 Hz build.
func processListPlatform() ([]ProcessEntry, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	totalMemKB := uint64(0)
	if values, memErr := readProcMeminfo(); memErr == nil {
		totalMemKB = values["MemTotal"]
	}
	uptime := hostUptimeSeconds()
	clockTicks := float64(0)
	if hz, hzErr := unix.Sysconf(unix.SC_CLK_TCK); hzErr == nil && hz > 0 {
		clockTicks = float64(hz)
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
			// The process exited between the directory listing and the read.
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
			if strings.HasPrefix(line, "VmRSS:") {
				fields := strings.Fields(strings.TrimPrefix(line, "VmRSS:"))
				if len(fields) > 0 && totalMemKB > 0 {
					if rss, convErr := strconv.ParseUint(fields[0], 10, 64); convErr == nil {
						proc.Memory = float64(rss) / float64(totalMemKB) * 100
					}
				}
			}
		}
		if proc.Name == "" {
			proc.Name = e.Name()
		}
		if clockTicks > 0 && uptime > 0 {
			if statData, statErr := os.ReadFile("/proc/" + e.Name() + "/stat"); statErr == nil {
				proc.CPU = processCPUPercent(string(statData), uptime, clockTicks)
			}
		}
		processes = append(processes, proc)
	}
	// The dashboard lists processes in PID order; an unsorted read of /proc
	// jitters between refreshes because directory order is not numeric order.
	sort.Slice(processes, func(i, j int) bool { return processes[i].PID < processes[j].PID })
	return processes, nil
}

// processCPUPercent is ps(1)'s average CPU over a process's lifetime. Fields
// are counted from the closing parenthesis because the command name may contain
// spaces and parentheses: after it, field 3 is the state, so utime (field 14)
// is the twelfth value and stime (field 15) the thirteenth.
func processCPUPercent(statLine string, uptimeSeconds int64, clockTicks float64) float64 {
	close := strings.LastIndexByte(statLine, ')')
	if close < 0 || close+1 >= len(statLine) {
		return 0
	}
	fields := strings.Fields(statLine[close+1:])
	if len(fields) < 20 {
		return 0
	}
	utime, err1 := strconv.ParseFloat(fields[11], 64)
	stime, err2 := strconv.ParseFloat(fields[12], 64)
	startTicks, err3 := strconv.ParseFloat(fields[19], 64)
	if err1 != nil || err2 != nil || err3 != nil || clockTicks <= 0 {
		return 0
	}
	lived := float64(uptimeSeconds) - startTicks/clockTicks
	if lived <= 0 {
		// Started this very second: there is no interval to average over.
		return 0
	}
	percent := (utime + stime) / clockTicks / lived * 100
	if math.IsNaN(percent) || math.IsInf(percent, 0) || percent < 0 {
		return 0
	}
	return percent
}
