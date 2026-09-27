package crossnode

import (
	"context"
	"log/slog"
	"runtime"
	"sort"
	"strconv"
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
	// interval is the trust window applied to recorded healthy results; it is
	// read by isTrustedLocked via FilterHealthy and IsHealthy.
	interval time.Duration

	reaperMu      sync.Mutex
	reaperRunning bool
	reaperStop    chan struct{}
	reaperDone    chan struct{}
}

// NewHealthFilter builds a filter that marks a backend down after threshold
// consecutive failures and stops trusting a healthy backend once its record is
// older than interval (see isTrustedLocked). Both arguments are clamped to sane
// defaults; neither is ignored.
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

// backendKey maps a backend to its health entry.
//
// Ports reach here straight from an HTTP path parameter, so negatives and other
// out-of-range values are reachable. The key must therefore be injective over
// every int: strconv.Itoa is used precisely because a hand-rolled formatter that
// drops anything outside 0..max collapses "host:-1" and "host:-2" onto one
// entry, letting one backend's failures mark a different backend down.
func backendKey(host string, port int) string {
	return host + ":" + strconv.Itoa(port)
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

// isTrustedLocked reports whether a recorded entry may currently receive
// traffic. Callers must hold at least hf.mu.RLock().
//
// The trust window (hf.interval, set by NewHealthFilter) is what makes
// "healthy" a claim about now rather than about the last probe: an entry whose
// LastChecked is older than the window is no longer trusted even while it still
// reads HealthHealthy, because a backend probed long ago may have disappeared
// and the reaper has not noticed yet. A stale reading is not a healthy one.
// Callers that want the raw record, not a routing decision, use GetHealth.
func (hf *HealthFilter) isTrustedLocked(entry *BackendHealth) bool {
	if entry == nil || entry.Status != HealthHealthy {
		return false
	}
	if hf.interval <= 0 {
		return true
	}
	return !entry.LastChecked.Before(time.Now().Add(-hf.interval))
}

func (hf *HealthFilter) IsHealthy(host string, port int) bool {
	hf.mu.RLock()
	defer hf.mu.RUnlock()

	key := backendKey(host, port)
	return hf.isTrustedLocked(hf.healthState[key])
}

func (hf *HealthFilter) FilterHealthy(backends []BackendAddr) []BackendAddr {
	hf.mu.RLock()
	defer hf.mu.RUnlock()

	// Only a backend with a recorded healthy result passes. Unknown backends,
	// degraded ones and down ones are all excluded: an unchecked backend is
	// not a healthy backend, and routing to it on the assumption that it
	// might be fine turns every new backend into a probe in production.
	// A recorded healthy result that has aged out of the trust window is
	// excluded by the same argument (see isTrustedLocked).
	var healthy []BackendAddr
	for _, b := range backends {
		key := backendKey(b.Host, b.Port)
		if hf.isTrustedLocked(hf.healthState[key]) {
			healthy = append(healthy, b)
		}
	}
	return healthy
}

// GetHealth returns the record for a backend. A backend that was never probed
// has no record, and is reported as HealthUnknown rather than defaulted to
// healthy: unknown is a distinct answer from "fine", and only IsHealthy /
// FilterHealthy turn health into a routing decision. Note that GetHealth reports
// the status as recorded and does not apply the trust window, so a caller
// comparing GetHealth with what FilterHealthy decided should re-check freshness
// against LastChecked itself.
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

// GetAllHealth returns every recorded entry, sorted by host then port so that
// repeated polling (the admin backends view) yields a stable, diffable ordering
// instead of Go's per-iteration map order.
func (hf *HealthFilter) GetAllHealth() []BackendHealth {
	hf.mu.RLock()
	defer hf.mu.RUnlock()

	result := make([]BackendHealth, 0, len(hf.healthState))
	for _, entry := range hf.healthState {
		result = append(result, *entry)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Host != result[j].Host {
			return result[i].Host < result[j].Host
		}
		return result[i].Port < result[j].Port
	})
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

// StartReaper starts the stale-entry reaper goroutine, which drops entries
// untouched for twice interval. It is idempotent: calling it while a reaper is
// already running is a no-op, and calling it again after StopReaper always
// creates a live goroutine, because every exit path of the previous one cleared
// the running flag.
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
	stop := make(chan struct{})
	done := make(chan struct{})
	hf.reaperStop = stop
	hf.reaperDone = done
	hf.reaperMu.Unlock()

	go hf.runReaper(ctx, interval, stop, done)
	slog.Info("health filter reaper started", "interval", interval)
}

// runReaper owns the running-flag transition for the goroutine it runs in. The
// deferred cleanup runs on every exit path — context cancellation, an explicit
// stop, and a recovered panic alike — so reaperRunning can never be left true
// with no goroutine behind it, which is what used to make the filter unable to
// restart after StopReaper.
func (hf *HealthFilter) runReaper(ctx context.Context, interval time.Duration, stop, done chan struct{}) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			slog.Error("health filter reaper panic recovered; stale entries are no longer reaped, call StartReaper to resume",
				"panic", r, "stack", string(buf[:n]))
		}
		hf.finishReaper(stop, done)
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			hf.reapStaleEntries(interval * 2)
		}
	}
}

// finishReaper closes done to release any StopReaper waiter, and clears the
// running flag only while stop is still the channel on record. If StopReaper
// already detached it (or a newer reaper replaced it), that bookkeeping belongs
// to whoever took ownership, not to this exiting goroutine.
func (hf *HealthFilter) finishReaper(stop, done chan struct{}) {
	hf.reaperMu.Lock()
	defer hf.reaperMu.Unlock()

	close(done)
	if hf.reaperStop == stop {
		hf.reaperStop = nil
		hf.reaperDone = nil
		hf.reaperRunning = false
	}
}

// StopReaper signals the reaper goroutine and returns once it has actually
// exited, so callers can rely on "stopped" rather than "probably stopping".
// Calling it twice, or after the context already ended the reaper, is a no-op:
// the first caller detaches the stop channel under the lock, so no path can
// close the same channel twice.
func (hf *HealthFilter) StopReaper() {
	hf.reaperMu.Lock()
	if !hf.reaperRunning || hf.reaperStop == nil {
		hf.reaperMu.Unlock()
		return
	}
	stop, done := hf.reaperStop, hf.reaperDone
	hf.reaperStop = nil
	hf.reaperDone = nil
	hf.reaperRunning = false
	hf.reaperMu.Unlock()

	close(stop)
	if done != nil {
		<-done
	}
	slog.Info("health filter reaper stopped")
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
