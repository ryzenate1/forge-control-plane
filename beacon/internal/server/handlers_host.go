package server

import (
	"errors"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"
)

type HostInfo struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Kernel   string `json:"kernel"`
	// Uptime is wall time since the machine booted; DaemonUptime is since this
	// process started. They answer different questions and are reported apart
	// on purpose.
	Uptime       int64  `json:"uptimeSeconds"`
	DaemonUptime int64  `json:"daemonUptimeSeconds"`
	CPUModel     string `json:"cpuModel"`
	CPUCores     int    `json:"cpuCores"`
	Arch         string `json:"arch"`
	Time         string `json:"time"`
}

type DiskPartition struct {
	MountPoint string  `json:"mountPoint"`
	Device     string  `json:"device"`
	FSType     string  `json:"fsType"`
	TotalMB    uint64  `json:"totalMb"`
	UsedMB     uint64  `json:"usedMb"`
	FreeMB     uint64  `json:"freeMb"`
	UsedPct    float64 `json:"usedPercent"`
}

type MemoryInfo struct {
	TotalMB     uint64  `json:"totalMb"`
	UsedMB      uint64  `json:"usedMb"`
	FreeMB      uint64  `json:"freeMb"`
	UsedPct     float64 `json:"usedPercent"`
	SwapTotalMB uint64  `json:"swapTotalMb"`
	SwapUsedMB  uint64  `json:"swapUsedMb"`
	SwapFreeMB  uint64  `json:"swapFreeMb"`
}

type NetworkInterface struct {
	Name   string `json:"name"`
	IPs    string `json:"ips"`
	MAC    string `json:"mac"`
	Speed  int    `json:"speedMbps"`
	Status string `json:"status"`
}

type ProcessEntry struct {
	PID    int     `json:"pid"`
	Name   string  `json:"name"`
	CPU    float64 `json:"cpuPercent"`
	Memory float64 `json:"memoryPercent"`
	State  string  `json:"state"`
}

// memorySample is one synchronous reading of host memory taken while serving a
// request. Every size is in MiB (bytes / 1024 / 1024).
//
// AvailableMB is the memory a new workload could actually be given: Linux
// reports MemAvailable, Darwin counts the free, inactive and speculative page
// queues. It is deliberately not the raw free-page figure, because the kernel's
// reclaimable page cache is not spare memory — a node that looks empty by the
// free-page count is usually full, and a scheduler fed that number places
// workloads onto it.
//
// A sample is only ever returned together with a nil error. A platform that
// cannot read any of these numbers returns an error instead of a zero-filled
// sample, so an unknown capacity can never reach the caller as "0 MB".
type memorySample struct {
	TotalMB     uint64
	AvailableMB uint64
	SwapTotalMB uint64
	SwapUsedMB  uint64
	SwapFreeMB  uint64
}

func (s *Server) handleHostInfo(w http.ResponseWriter, r *http.Request) {
	hostname, err := os.Hostname()
	if err != nil {
		// The hostname identifies the node. Serving it empty would let a panel
		// label an unknown machine, so the read failure is reported instead of
		// being flattened into a blank string.
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read hostname: " + err.Error()})
		return
	}
	// A -1 uptime sentinel is "unknown", not a measurement. Serving it as a
	// number would tell the panel this host booted one second before the
	// epoch (or a daemon that started in the future); fail instead.
	hostUptime, daemonUptime, err := hostUptimesForInfo(s.started)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	info := HostInfo{
		Hostname:     hostname,
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		Uptime:       hostUptime,
		DaemonUptime: daemonUptime,
		CPUCores:     runtime.NumCPU(),
		Time:         time.Now().UTC().Format(time.RFC3339),
		Kernel:       kernelVersion(),
		CPUModel:     cpuModel(),
	}
	writeJSON(w, http.StatusOK, info)
}

func hostUptimesForInfo(started time.Time) (hostUptime, daemonUptime int64, err error) {
	hostUptime = hostUptimeSeconds()
	if hostUptime < 0 {
		return 0, 0, errors.New("host uptime is unknown")
	}
	daemonUptime = daemonUptimeSeconds(started)
	if daemonUptime < 0 {
		return 0, 0, errors.New("daemon uptime is unknown")
	}
	return hostUptime, daemonUptime, nil
}

// diskProbePaths returns the mount points whose filesystems the node actually
// writes workload data to. Reporting only the root filesystem hid the disk a
// server really lands on when the data root is a separate mount, so placement
// was decided against the wrong numbers.
func (s *Server) diskProbePaths() []string {
	paths := make([]string, 0, 2)
	paths = append(paths, "/")
	if cleaned := strings.TrimSpace(s.dataDir); cleaned != "" {
		paths = append(paths, cleaned)
	}
	return paths
}

func (s *Server) handleHostDisk(w http.ResponseWriter, r *http.Request) {
	partitions, err := hostDiskPartitionsPlatform(s.diskProbePaths())
	if err != nil {
		// An unreadable filesystem is not an empty one: a 200 with no partitions
		// would tell the control plane this host has no disk to run out of.
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read disk partitions: " + err.Error()})
		return
	}
	if len(partitions) == 0 {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read disk partitions: no mounted filesystem could be read"})
		return
	}
	writeJSON(w, http.StatusOK, partitions)
}

func (s *Server) handleHostMemory(w http.ResponseWriter, r *http.Request) {
	sample, err := readMemorySample()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read system memory: " + err.Error()})
		return
	}
	if sample.TotalMB == 0 {
		// A total of zero is not a measurement any caller should plan against.
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "read system memory: reported total is zero"})
		return
	}

	available := sample.AvailableMB
	if available > sample.TotalMB {
		// Should be impossible; clamped so a kernel rounding difference cannot
		// make used, and therefore usedPercent, go negative.
		available = sample.TotalMB
	}
	used := sample.TotalMB - available
	usedPct := float64(used) / float64(sample.TotalMB) * 100

	swapUsed := sample.SwapUsedMB
	if sample.SwapTotalMB > 0 && swapUsed > sample.SwapTotalMB {
		swapUsed = sample.SwapTotalMB
	}

	info := MemoryInfo{
		TotalMB:     sample.TotalMB,
		UsedMB:      used,
		FreeMB:      available,
		UsedPct:     usedPct,
		SwapTotalMB: sample.SwapTotalMB,
		SwapUsedMB:  swapUsed,
		SwapFreeMB:  sample.SwapFreeMB,
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleHostNetwork(w http.ResponseWriter, r *http.Request) {
	ifaces, err := netInterfaces()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ifaces)
}

func (s *Server) handleHostProcesses(w http.ResponseWriter, r *http.Request) {
	processes, err := processList()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, processes)
}

func kernelVersion() string {
	return kernelVersionPlatform()
}

func cpuModel() string {
	return cpuModelPlatform()
}

func netInterfaces() ([]NetworkInterface, error) {
	return netInterfacesPlatform()
}

func processList() ([]ProcessEntry, error) {
	return processListPlatform()
}
