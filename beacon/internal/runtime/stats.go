package runtime

import (
	"encoding/json"
	"errors"
	"io"
)

type dockerStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage  uint64   `json:"total_usage"`
			PercpuUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	} `json:"networks"`
}

func DecodeDockerStats(reader io.Reader) (Stats, error) {
	// The first sample after a container starts has no previous CPU
	// counter to delta against, so dockerCPUPercent reports 0. That zero
	// is "no baseline yet", not "idle": callers must not treat a single
	// zero-CPU reading as a measurement.
	var payload dockerStats
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		return Stats{}, err
	}
	// A well-formed but contentless sample (`{}`) decodes to all zeros with no
	// error, which would be published as an idle container. Every field of
	// that reading is "nothing was measured", so say so instead of returning
	// zeroes that a caller cannot distinguish from a genuinely idle workload.
	if payload.CPUStats.CPUUsage.TotalUsage == 0 && payload.PreCPUStats.CPUUsage.TotalUsage == 0 &&
		payload.MemoryStats.Usage == 0 && payload.MemoryStats.Limit == 0 && len(payload.Networks) == 0 {
		return Stats{}, errors.New("container stats sample carries no measurements")
	}

	var rx uint64
	var tx uint64
	networksReported := false
	for _, network := range payload.Networks {
		rx += network.RxBytes
		tx += network.TxBytes
		networksReported = true
	}

	return Stats{
		CPUPercent:     dockerCPUPercent(payload),
		MemoryBytes:    payload.MemoryStats.Usage,
		MemoryLimit:    payload.MemoryStats.Limit,
		NetworkRxBytes: rx,
		NetworkTxBytes: tx,
		NetworkKnown:   networksReported,
	}, nil
}

func dockerCPUPercent(stats dockerStats) float64 {
	// The counters are cumulative nanoseconds sampled twice. A container that
	// restarted (or a daemon that lost its previous sample) can present a
	// "current" value smaller than the "previous" one. Subtracting unsigned
	// counters in that order wraps to ~2^64, and the wrapped value would then
	// be published as an enormous CPU percentage. Treat a regressing counter
	// as what it is: no usable baseline, which dockerCPUPercent already
	// reports as 0 with the documented "no baseline yet" meaning.
	if stats.CPUStats.CPUUsage.TotalUsage < stats.PreCPUStats.CPUUsage.TotalUsage ||
		stats.CPUStats.SystemCPUUsage < stats.PreCPUStats.SystemCPUUsage {
		return 0
	}
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemCPUUsage - stats.PreCPUStats.SystemCPUUsage)
	onlineCPUs := float64(len(stats.CPUStats.CPUUsage.PercpuUsage))
	if systemDelta <= 0 || cpuDelta <= 0 || onlineCPUs == 0 {
		return 0
	}
	return (cpuDelta / systemDelta) * onlineCPUs * 100
}
