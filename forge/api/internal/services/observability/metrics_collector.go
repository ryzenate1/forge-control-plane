package observability

import (
	"context"
	"runtime"
	"sync"
	"time"
)

type SystemMetrics struct {
	GoroutineCount int       `json:"goroutine_count"`
	HeapAlloc      uint64    `json:"heap_alloc"`
	HeapSys        uint64    `json:"heap_sys"`
	GCPauseNs      uint64    `json:"gc_pause_ns"`
	NumGC          uint32    `json:"num_gc"`
	MemoryUsage    float64   `json:"memory_usage"`
	Alloc          uint64    `json:"alloc"`
	TotalAlloc     uint64    `json:"total_alloc"`
	Sys            uint64    `json:"sys"`
	NumCPU         int       `json:"num_cpu"`
	Timestamp      time.Time `json:"timestamp"`
}

type MetricsHistory struct {
	mu        sync.RWMutex
	snapshots []SystemMetrics
	maxSize   int
}

func NewMetricsHistory(maxSize int) *MetricsHistory {
	if maxSize <= 0 {
		maxSize = 60
	}
	return &MetricsHistory{
		snapshots: make([]SystemMetrics, 0, maxSize),
		maxSize:   maxSize,
	}
}

func CollectSystemMetrics() (*SystemMetrics, error) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	memoryUsage := 0.0
	if mem.HeapSys > 0 {
		memoryUsage = float64(mem.HeapAlloc) / float64(mem.HeapSys) * 100
	}

	return &SystemMetrics{
		GoroutineCount: runtime.NumGoroutine(),
		HeapAlloc:      mem.HeapAlloc,
		HeapSys:        mem.HeapSys,
		GCPauseNs:      mem.PauseNs[(mem.NumGC+255)%256],
		NumGC:          mem.NumGC,
		MemoryUsage:    memoryUsage,
		Alloc:          mem.Alloc,
		TotalAlloc:     mem.TotalAlloc,
		Sys:            mem.Sys,
		NumCPU:         runtime.NumCPU(),
		Timestamp:      time.Now(),
	}, nil
}

func (h *MetricsHistory) Add(m SystemMetrics) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.snapshots = append(h.snapshots, m)
	if len(h.snapshots) > h.maxSize {
		h.snapshots = h.snapshots[len(h.snapshots)-h.maxSize:]
	}
}

func (h *MetricsHistory) GetSnapshots() []SystemMetrics {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make([]SystemMetrics, len(h.snapshots))
	copy(result, h.snapshots)
	return result
}

func (h *MetricsHistory) Latest() *SystemMetrics {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.snapshots) == 0 {
		return nil
	}
	m := h.snapshots[len(h.snapshots)-1]
	return &m
}

func (h *MetricsHistory) StartCollection(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m, err := CollectSystemMetrics()
				if err == nil {
					h.Add(*m)
				}
			}
		}
	}()
}
