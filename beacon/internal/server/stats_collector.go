package server

import (
	"context"
	"sync"
	"time"

	"gamepanel/beacon/internal/runtime"
)

type ServerStats struct {
	Timestamp     time.Time `json:"timestamp"`
	CPU           float64   `json:"cpu"`
	MemoryMB      float64   `json:"memoryMB"`
	DiskMB        float64   `json:"diskMB"`
	NetworkRxMB   float64   `json:"networkRxMB"`
	NetworkTxMB   float64   `json:"networkTxMB"`
	UptimeSeconds float64   `json:"uptimeSeconds"`
}

type StatsCollector struct {
	mu         sync.Mutex
	runtime    runtime.Runtime
	history    map[string][]ServerStats
	maxHistory int
	serverIDs  map[string]bool
	startTime  time.Time
}

func NewStatsCollector(rt runtime.Runtime, maxHistory int) *StatsCollector {
	if maxHistory < 1 {
		maxHistory = 60
	}
	return &StatsCollector{
		runtime:    rt,
		history:    make(map[string][]ServerStats),
		maxHistory: maxHistory,
		serverIDs:  make(map[string]bool),
		startTime:  time.Now(),
	}
}

func (sc *StatsCollector) RegisterServer(serverID string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.serverIDs[serverID] = true
}

func (sc *StatsCollector) UnregisterServer(serverID string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	delete(sc.serverIDs, serverID)
	delete(sc.history, serverID)
}

func (sc *StatsCollector) Collect(ctx context.Context, serverID string) (*ServerStats, error) {
	var ss ServerStats
	if sc.runtime == nil {
		ss = ServerStats{
			Timestamp:     time.Now(),
			UptimeSeconds: time.Since(sc.startTime).Seconds(),
		}
	} else {
		// Lifecycle first: per-workload StartedAt is the uptime basis, and an
		// inspect failure is reported instead of zero telemetry.
		inspection, err := sc.runtime.Inspect(ctx, serverID)
		if err != nil {
			return nil, err
		}
		stats, err := sc.runtime.Stats(ctx, serverID)
		if err != nil {
			return nil, err
		}
		uptime := time.Since(sc.startTime).Seconds()
		if !inspection.StartedAt.IsZero() {
			if delta := time.Since(inspection.StartedAt).Seconds(); delta >= 0 {
				uptime = delta
			} else {
				uptime = 0
			}
		}
		ss = ServerStats{
			Timestamp:     time.Now(),
			CPU:           stats.CPUPercent,
			MemoryMB:      float64(stats.MemoryBytes) / (1024 * 1024),
			NetworkRxMB:   float64(stats.NetworkRxBytes) / (1024 * 1024),
			NetworkTxMB:   float64(stats.NetworkTxBytes) / (1024 * 1024),
			UptimeSeconds: uptime,
		}
	}

	sc.mu.Lock()
	sc.appendHistory(serverID, ss)
	sc.mu.Unlock()

	return &ss, nil
}

func (sc *StatsCollector) appendHistory(serverID string, ss ServerStats) {
	h := sc.history[serverID]
	h = append(h, ss)
	if len(h) > sc.maxHistory {
		h = h[len(h)-sc.maxHistory:]
	}
	sc.history[serverID] = h
}

func (sc *StatsCollector) GetHistory(serverID string) []ServerStats {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	h := sc.history[serverID]
	result := make([]ServerStats, len(h))
	copy(result, h)
	return result
}

func (sc *StatsCollector) Start(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sc.collectAll(ctx)
			}
		}
	}()
}

func (sc *StatsCollector) collectAll(ctx context.Context) {
	sc.mu.Lock()
	ids := make([]string, 0, len(sc.serverIDs))
	for id := range sc.serverIDs {
		ids = append(ids, id)
	}
	sc.mu.Unlock()

	for _, id := range ids {
		select {
		case <-ctx.Done():
			return
		default:
			sc.Collect(ctx, id)
		}
	}
}
