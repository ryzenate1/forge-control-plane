package runtime

import (
	"context"
	"encoding/json"
	"io"
	"time"
)

type EnhancedStats struct {
	CPUPercent    float64   `json:"cpuPercent"`
	CPUCount      int       `json:"cpuCount"`
	MemoryUsage   uint64    `json:"memoryUsage"`
	MemoryLimit   uint64    `json:"memoryLimit"`
	MemoryPercent float64   `json:"memoryPercent"`
	NetworkRx     uint64    `json:"networkRx"`
	NetworkTx     uint64    `json:"networkTx"`
	BlockRead     uint64    `json:"blockRead"`
	BlockWrite    uint64    `json:"blockWrite"`
	PIDs          int       `json:"pids"`
	UptimeSeconds float64   `json:"uptimeSeconds"`
	Timestamp     time.Time `json:"timestamp"`
}

type enhancedDockerStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage  uint64   `json:"total_usage"`
			PercpuUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     int    `json:"online_cpus"`
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
	BlkioStats struct {
		IOServiceBytesRecursive []struct {
			Op    string `json:"op"`
			Value uint64 `json:"value"`
		} `json:"io_service_bytes_recursive"`
	} `json:"blkio_stats"`
	PIDsStats struct {
		Current int `json:"current"`
	} `json:"pids_stats"`
}

func DecodeEnhancedStats(reader io.Reader) (*EnhancedStats, error) {
	var payload enhancedDockerStats
	if err := json.NewDecoder(reader).Decode(&payload); err != nil {
		return nil, err
	}

	var rx, tx uint64
	for _, network := range payload.Networks {
		rx += network.RxBytes
		tx += network.TxBytes
	}

	var blockRead, blockWrite uint64
	for _, entry := range payload.BlkioStats.IOServiceBytesRecursive {
		switch entry.Op {
		case "Read", "read":
			blockRead += entry.Value
		case "Write", "write":
			blockWrite += entry.Value
		}
	}

	cpuCount := payload.CPUStats.OnlineCPUs
	if cpuCount == 0 {
		cpuCount = len(payload.CPUStats.CPUUsage.PercpuUsage)
	}

	var memPercent float64
	if payload.MemoryStats.Limit > 0 {
		memPercent = float64(payload.MemoryStats.Usage) / float64(payload.MemoryStats.Limit) * 100
	}

	cpuDelta := float64(payload.CPUStats.CPUUsage.TotalUsage - payload.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(payload.CPUStats.SystemCPUUsage - payload.PreCPUStats.SystemCPUUsage)
	var cpuPercent float64
	if systemDelta > 0 && cpuDelta > 0 && cpuCount > 0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(cpuCount) * 100
	}

	return &EnhancedStats{
		CPUPercent:    cpuPercent,
		CPUCount:      cpuCount,
		MemoryUsage:   payload.MemoryStats.Usage,
		MemoryLimit:   payload.MemoryStats.Limit,
		MemoryPercent: memPercent,
		NetworkRx:     rx,
		NetworkTx:     tx,
		BlockRead:     blockRead,
		BlockWrite:    blockWrite,
		PIDs:          payload.PIDsStats.Current,
		Timestamp:     time.Now(),
	}, nil
}

func CollectEnhancedStats(ctx context.Context, rt Runtime, serverID string) (*EnhancedStats, error) {
	rc, err := rt.StatsStream(ctx, serverID)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return DecodeEnhancedStats(rc)
}
