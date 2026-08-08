package runtime

import (
	"encoding/json"
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
	var payload dockerStats
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		return Stats{}, err
	}

	var rx uint64
	var tx uint64
	for _, network := range payload.Networks {
		rx += network.RxBytes
		tx += network.TxBytes
	}

	return Stats{
		CPUPercent:     dockerCPUPercent(payload),
		MemoryBytes:    payload.MemoryStats.Usage,
		MemoryLimit:    payload.MemoryStats.Limit,
		NetworkRxBytes: rx,
		NetworkTxBytes: tx,
	}, nil
}

func dockerCPUPercent(stats dockerStats) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage - stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemCPUUsage - stats.PreCPUStats.SystemCPUUsage)
	onlineCPUs := float64(len(stats.CPUStats.CPUUsage.PercpuUsage))
	if systemDelta <= 0 || cpuDelta <= 0 || onlineCPUs == 0 {
		return 0
	}
	return (cpuDelta / systemDelta) * onlineCPUs * 100
}
