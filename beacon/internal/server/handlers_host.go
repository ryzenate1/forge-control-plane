package server

import (
	"net/http"
	"os"
	"runtime"
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

func (s *Server) handleHostInfo(w http.ResponseWriter, r *http.Request) {
	hostname, _ := os.Hostname()
	info := HostInfo{
		Hostname:     hostname,
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		Uptime:       hostUptimeSeconds(),
		DaemonUptime: daemonUptimeSeconds(s.started),
		CPUCores:     runtime.NumCPU(),
		Time:         time.Now().UTC().Format(time.RFC3339),
		Kernel:       kernelVersion(),
		CPUModel:     cpuModel(),
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleHostDisk(w http.ResponseWriter, r *http.Request) {
	partitions := hostDiskPartitionsPlatform()
	writeJSON(w, http.StatusOK, partitions)
}

func (s *Server) handleHostMemory(w http.ResponseWriter, r *http.Request) {
	total := totalSystemMemoryMB()
	free := freeMemoryMB()
	var used uint64
	if total > free {
		used = total - free
	}
	var usedPct float64
	if total > 0 {
		usedPct = float64(used) / float64(total) * 100
	}

	info := MemoryInfo{
		TotalMB: total,
		UsedMB:  used,
		FreeMB:  free,
		UsedPct: usedPct,
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
