package server

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"gamepanel/beacon/internal/runtime"
)

// ErrStatsRuntimeUnavailable is returned when the collector is asked to measure
// a workload but has no runtime attached. There is no honest zero to report for
// that case: nothing was measured.
var ErrStatsRuntimeUnavailable = errors.New("stats collector has no runtime attached")

type ServerStats struct {
	Timestamp time.Time `json:"timestamp"`

	// Measured reports whether this frame carries a reading taken from the
	// engine. When it is false every numeric field below is zero because
	// nothing was measured, not because the workload is idle. A consumer that
	// renders "0% CPU" from an unmeasured frame cannot tell a dead collector
	// from a quiet server, so it must render unknown / not-reported instead.
	Measured bool `json:"measured"`
	// Reason explains an unmeasured frame ("inspect failed: ..."). It is empty
	// when Measured is true.
	Reason string `json:"reason,omitempty"`

	CPU         float64 `json:"cpu"`
	MemoryMB    float64 `json:"memoryMB"`
	NetworkRxMB float64 `json:"networkRxMB"`
	NetworkTxMB float64 `json:"networkTxMB"`

	// DiskMB is only a measurement when DiskKnown is true. The workload runtime
	// interface reports CPU, memory and network only, so nothing here populates
	// disk usage; a frame with DiskKnown=false carries DiskMB=0 as "unknown" and
	// must never be summed, charted or compared against a quota.
	DiskMB    float64 `json:"diskMB"`
	DiskKnown bool    `json:"diskKnown"`

	// UptimeSeconds is only a measurement when UptimeKnown is true. The engine's
	// per-workload start time is the only basis for it; a workload that has
	// never started, or whose start stamp is in the future (clock skew), is
	// reported as unknown rather than as a number borrowed from the collector's
	// own lifetime.
	UptimeSeconds float64 `json:"uptimeSeconds"`
	UptimeKnown   bool    `json:"uptimeKnown"`
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

// Collect takes one reading for a workload.
//
// Every failure path records an unmeasured frame in history and returns an
// error. Recording the failure matters as much as returning it: without it the
// newest entry in the history stays the last successful reading, and a consumer
// that reads "the latest sample" serves a stale number as if it were current.
func (sc *StatsCollector) Collect(ctx context.Context, serverID string) (*ServerStats, error) {
	if sc.runtime == nil {
		sc.recordUnmeasured(serverID, ErrStatsRuntimeUnavailable.Error())
		return nil, ErrStatsRuntimeUnavailable
	}

	// Lifecycle first: per-workload StartedAt is the uptime basis, and an
	// inspect failure is reported instead of zero telemetry.
	inspection, err := sc.runtime.Inspect(ctx, serverID)
	if err != nil {
		sc.recordUnmeasured(serverID, "inspect failed: "+err.Error())
		return nil, err
	}
	stats, err := sc.runtime.Stats(ctx, serverID)
	if err != nil {
		sc.recordUnmeasured(serverID, "stats read failed: "+err.Error())
		return nil, err
	}

	var uptimeSeconds float64
	uptimeKnown := false
	if !inspection.StartedAt.IsZero() {
		if delta := time.Since(inspection.StartedAt).Seconds(); delta >= 0 {
			uptimeSeconds = delta
			uptimeKnown = true
		}
	}
	// A zero StartedAt, or one in the future, leaves uptime unknown. It is not
	// filled in from the collector's own start time: that number describes the
	// daemon, not the workload, and would be reported as the server's uptime.

	ss := ServerStats{
		Timestamp:     time.Now(),
		Measured:      true,
		CPU:           stats.CPUPercent,
		MemoryMB:      float64(stats.MemoryBytes) / (1024 * 1024),
		NetworkRxMB:   float64(stats.NetworkRxBytes) / (1024 * 1024),
		NetworkTxMB:   float64(stats.NetworkTxBytes) / (1024 * 1024),
		UptimeSeconds: uptimeSeconds,
		UptimeKnown:   uptimeKnown,
		// DiskKnown stays false: no runtime adapter reports disk usage through
		// Stats yet. Adapters that learn to do so must set it together with the
		// value, or the field keeps meaning "not measured".
		DiskKnown: false,
	}

	sc.mu.Lock()
	sc.appendHistory(serverID, ss)
	sc.mu.Unlock()

	return &ss, nil
}

// recordUnmeasured appends an explicit "nothing was measured here" frame so a
// failed or missing reading occupies its own slot in the series instead of
// leaving the previous reading as the newest word on the workload.
func (sc *StatsCollector) recordUnmeasured(serverID, reason string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.appendHistory(serverID, ServerStats{
		Timestamp: time.Now(),
		Measured:  false,
		Reason:    reason,
	})
}

func (sc *StatsCollector) appendHistory(serverID string, ss ServerStats) {
	h := sc.history[serverID]
	h = append(h, ss)
	if len(h) > sc.maxHistory {
		h = h[len(h)-sc.maxHistory:]
	}
	sc.history[serverID] = h
}

// GetHistory returns the recorded frames, oldest first. Frames with
// Measured=false are failures, not measurements; the copy keeps callers from
// rewriting history through the slice they were handed.
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
		}
		// A failing collection is logged, not swallowed: the alternative is a
		// collector that silently stops updating a workload while the series it
		// serves still ends in a plausible-looking number.
		if _, err := sc.Collect(ctx, id); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("stats collection failed for %s: %v", id, err)
		}
	}
}
