package server

import (
	"net/http"
	"os"
	"runtime"
	"time"

	"golang.org/x/sys/unix"
)

type HostInfo struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Kernel   string `json:"kernel"`
	Uptime   int64  `json:"uptimeSeconds"`
	CPUModel string `json:"cpuModel"`
	CPUCores int    `json:"cpuCores"`
	Arch     string `json:"arch"`
	Time     string `json:"time"`
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
		Hostname: hostname,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Uptime:   int64(time.Since(s.started).Seconds()),
		CPUCores: runtime.NumCPU(),
		Time:     time.Now().UTC().Format(time.RFC3339),
		Kernel:   kernelVersion(),
		CPUModel: cpuModel(),
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleHostDisk(w http.ResponseWriter, r *http.Request) {
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

func cpuModel() string {
	return cpuModelPlatform()
}

func netInterfaces() ([]NetworkInterface, error) {
	return netInterfacesPlatform()
}

func processList() ([]ProcessEntry, error) {
	return processListPlatform()
}
