package crossnode

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type HealthStatus int

const (
	HealthUnknown  HealthStatus = 0
	HealthHealthy  HealthStatus = 1
	HealthDegraded HealthStatus = 2
	HealthDown     HealthStatus = 3
)

type BackendHealth struct {
	ServerID    string       `json:"serverId"`
	NodeID      string       `json:"nodeId"`
	Host        string       `json:"host"`
	Port        int          `json:"port"`
	Status      HealthStatus `json:"status"`
	LastChecked time.Time    `json:"lastChecked"`
	FailCount   int          `json:"failCount"`
	Reason      string       `json:"reason,omitempty"`
}

type HealthFilter struct {
	mu          sync.RWMutex
	healthState map[string]*BackendHealth
	threshold   int
	interval    time.Duration

	reaperMu    sync.Mutex
	reaperRunning bool
	reaperStop    chan struct{}
}

func NewHealthFilter(threshold int, interval time.Duration) *HealthFilter {
	if threshold <= 0 {
		threshold = 2
	}
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &HealthFilter{
		healthState: make(map[string]*BackendHealth),
		threshold:   threshold,
		interval:    interval,
	}
}

func backendKey(host string, port int) string {
	return host + ":" + itoa(port)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func (hf *HealthFilter) RecordSuccess(host string, port int) {
	hf.mu.Lock()
	defer hf.mu.Unlock()

	key := backendKey(host, port)
	entry, ok := hf.healthState[key]
	if !ok {
		hf.healthState[key] = &BackendHealth{
			Host:        host,
			Port:        port,
			Status:      HealthHealthy,
			LastChecked: time.Now(),
			FailCount:   0,
		}
		return
	}
	entry.Status = HealthHealthy
	entry.LastChecked = time.Now()
	entry.FailCount = 0
	entry.Reason = ""
}

func (hf *HealthFilter) RecordFailure(host string, port int, reason string) {
	hf.mu.Lock()
	defer hf.mu.Unlock()

	key := backendKey(host, port)
	entry, ok := hf.healthState[key]
	if !ok {
		hf.healthState[key] = &BackendHealth{
			Host:        host,
			Port:        port,
			Status:      HealthDegraded,
			LastChecked: time.Now(),
			FailCount:   1,
			Reason:      reason,
		}
		return
	}
	entry.FailCount++
	entry.LastChecked = time.Now()
	entry.Reason = reason

	if entry.FailCount >= hf.threshold {
		entry.Status = HealthDown
	} else {
		entry.Status = HealthDegraded
	}
}

func (hf *HealthFilter) IsHealthy(host string, port int) bool {
	hf.mu.RLock()
	defer hf.mu.RUnlock()

	key := backendKey(host, port)
	entry, ok := hf.healthState[key]
	if !ok {
		return true
	}
	return entry.Status == HealthHealthy || entry.Status == HealthUnknown
}

func (hf *HealthFilter) FilterHealthy(backends []BackendAddr) []BackendAddr {
	hf.mu.RLock()
	defer hf.mu.RUnlock()

	var healthy []BackendAddr
	for _, b := range backends {
		key := backendKey(b.Host, b.Port)
		entry, ok := hf.healthState[key]
		if !ok || entry.Status == HealthHealthy || entry.Status == HealthUnknown {
			healthy = append(healthy, b)
		}
	}
	return healthy
}

func (hf *HealthFilter) GetHealth(host string, port int) *BackendHealth {
	hf.mu.RLock()
	defer hf.mu.RUnlock()

	key := backendKey(host, port)
	entry, ok := hf.healthState[key]
	if !ok {
		return &BackendHealth{
			Host:   host,
			Port:   port,
			Status: HealthUnknown,
		}
	}
	cp := *entry
	return &cp
}

func (hf *HealthFilter) GetAllHealth() []BackendHealth {
	hf.mu.RLock()
	defer hf.mu.RUnlock()

	result := make([]BackendHealth, 0, len(hf.healthState))
	for _, entry := range hf.healthState {
		result = append(result, *entry)
	}
	return result
}

func (hf *HealthFilter) Clear(host string, port int) {
	hf.mu.Lock()
	defer hf.mu.Unlock()

	key := backendKey(host, port)
	delete(hf.healthState, key)
}

func (hf *HealthFilter) ClearAll() {
	hf.mu.Lock()
	defer hf.mu.Unlock()

	hf.healthState = make(map[string]*BackendHealth)
}

func (hf *HealthFilter) StaleEntries(olderThan time.Duration) []BackendHealth {
	hf.mu.RLock()
	defer hf.mu.RUnlock()

	cutoff := time.Now().Add(-olderThan)
	var stale []BackendHealth
	for _, entry := range hf.healthState {
		if entry.LastChecked.Before(cutoff) {
			stale = append(stale, *entry)
		}
	}
	return stale
}

func (hf *HealthFilter) StartReaper(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}

	hf.reaperMu.Lock()
	if hf.reaperRunning {
		hf.reaperMu.Unlock()
		return
	}
	hf.reaperRunning = true
	hf.reaperStop = make(chan struct{})
	hf.reaperMu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				hf.reaperMu.Lock()
				hf.reaperRunning = false
				hf.reaperMu.Unlock()
				return
			case <-hf.reaperStop:
				return
			case <-ticker.C:
				hf.reapStaleEntries(interval * 2)
			}
		}
	}()
	slog.Info("health filter reaper started", "interval", interval)
}

func (hf *HealthFilter) StopReaper() {
	hf.reaperMu.Lock()
	defer hf.reaperMu.Unlock()
	if hf.reaperRunning && hf.reaperStop != nil {
		close(hf.reaperStop)
		hf.reaperRunning = false
	}
}

func (hf *HealthFilter) reapStaleEntries(olderThan time.Duration) {
	stale := hf.StaleEntries(olderThan)
	if len(stale) == 0 {
		return
	}

	hf.mu.Lock()
	for _, entry := range stale {
		key := backendKey(entry.Host, entry.Port)
		delete(hf.healthState, key)
	}
	hf.mu.Unlock()

	slog.Info("health filter reaped stale entries", "count", len(stale))
}
