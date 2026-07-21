package servicediscovery

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type StaleEndpointReaper struct {
	registry      *Registry
	mu            sync.Mutex
	interval      time.Duration
	heartbeatTTL  time.Duration
	lastRun       time.Time
	reaped        int
	running       bool
	stopCh        chan struct{}
	now           func() time.Time
}

func NewStaleEndpointReaper(registry *Registry) *StaleEndpointReaper {
	return &StaleEndpointReaper{
		registry:     registry,
		interval:     30 * time.Second,
		heartbeatTTL: 3 * time.Minute,
		now:          time.Now,
	}
}

func (r *StaleEndpointReaper) SetInterval(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.interval = d
}

func (r *StaleEndpointReaper) SetHeartbeatTTL(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.heartbeatTTL = d
}

func (r *StaleEndpointReaper) Start(ctx context.Context) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.stopCh = make(chan struct{})
	r.mu.Unlock()

	go func() {
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				r.mu.Lock()
				r.running = false
				r.mu.Unlock()
				return
			case <-r.stopCh:
				return
			case <-ticker.C:
				r.reap(ctx)
			}
		}
	}()
}

func (r *StaleEndpointReaper) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running && r.stopCh != nil {
		close(r.stopCh)
		r.running = false
	}
}

func (r *StaleEndpointReaper) reap(ctx context.Context) {
	r.mu.Lock()
	ttl := r.heartbeatTTL
	now := r.now
	r.mu.Unlock()

	nowTime := now()
	endpoints := r.registry.ListEndpoints(EndpointFilter{})
	var staleIDs []string

	for _, ep := range endpoints {
		if ep.Status == EndpointStatusDraining {
			continue
		}
		if nowTime.Sub(ep.LastHeartbeat) > ttl {
			staleIDs = append(staleIDs, ep.ID)
		}
	}

	if len(staleIDs) == 0 {
		return
	}

	for _, id := range staleIDs {
		if err := r.registry.UpdateEndpointStatus(ctx, id, EndpointStatusUnhealthy); err != nil {
			slog.Warn("Failed to mark endpoint unhealthy", "endpointID", id, "error", err)
		}
	}

	r.mu.Lock()
	r.lastRun = time.Now()
	r.reaped += len(staleIDs)
	r.mu.Unlock()

	if len(staleIDs) > 0 {
		slog.Info("Stale endpoint reaper marked endpoints as unhealthy",
			"count", len(staleIDs), "heartbeatTTL", ttl)
	}
}

func (r *StaleEndpointReaper) Stats() (lastRun time.Time, reaped int, interval time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastRun, r.reaped, r.interval
}
