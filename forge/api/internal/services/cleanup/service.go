package cleanup

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"
)

type Metrics struct {
	StaleReservationsCleaned   uint64 `json:"stale_reservations_cleaned"`
	OrphanedAllocationsCleaned uint64 `json:"orphaned_allocations_cleaned"`
	CleanupRunsTotal           uint64 `json:"cleanup_runs_total"`
	CleanupErrorsTotal         uint64 `json:"cleanup_errors_total"`
}

type Info struct {
	StaleReservations int `json:"staleReservations"`
	OrphanedAllocations int `json:"orphanedAllocations"`
}

type cleanupStore interface {
	ExpirePlacementReservations(ctx context.Context) ([]store.PlacementReservation, error)
	ListPlacementReservations(ctx context.Context) ([]store.PlacementReservation, error)
	ListAllocations(ctx context.Context) ([]store.Allocation, error)
	DeleteAllocation(ctx context.Context, allocationID string, actorID *string) error
}

type Service struct {
	store     cleanupStore
	publisher events.Publisher
	mu        sync.Mutex
	metrics   Metrics
	cancel    context.CancelFunc
}

func New(store *store.Store, publishers ...events.Publisher) *Service {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Service{store: store, publisher: publisher}
}

func (s *Service) Metrics() Metrics {
	if s == nil {
		return Metrics{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metrics
}

func (s *Service) Start(ctx context.Context) {
	if s == nil || s.store == nil {
		return
	}
	ctx, s.cancel = context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = s.RunCleanup(ctx)
			}
		}
	}()
}

func (s *Service) Stop() {
	if s != nil && s.cancel != nil {
		s.cancel()
	}
}

func (s *Service) RunCleanup(ctx context.Context) (Info, error) {
	if s == nil || s.store == nil {
		return Info{}, errors.New("cleanup service unavailable")
	}
	var info Info
	s.mu.Lock()
	s.metrics.CleanupRunsTotal++
	s.mu.Unlock()

	stale, err := s.store.ExpirePlacementReservations(ctx)
	if err != nil {
		s.increment(func(m *Metrics) { m.CleanupErrorsTotal++ })
		return info, fmt.Errorf("expire reservations: %w", err)
	}
	info.StaleReservations = len(stale)
	for range stale {
		s.increment(func(m *Metrics) { m.StaleReservationsCleaned++ })
	}

	orphaned, err := s.cleanOrphanedAllocations(ctx)
	if err != nil {
		s.increment(func(m *Metrics) { m.CleanupErrorsTotal++ })
		return info, fmt.Errorf("clean allocations: %w", err)
	}
	info.OrphanedAllocations = orphaned

	s.publish(ctx, events.EventReservationExpired, "cleanup", "system", map[string]any{
		"staleReservations":  info.StaleReservations,
		"orphanedAllocations": info.OrphanedAllocations,
	})
	return info, nil
}

func (s *Service) cleanOrphanedAllocations(ctx context.Context) (int, error) {
	allocations, err := s.store.ListAllocations(ctx)
	if err != nil {
		return 0, err
	}
	cleaned := 0
	for _, alloc := range allocations {
		if alloc.Server == nil {
			nodeAllocs, err := s.store.ListPlacementReservations(ctx)
			if err != nil {
				continue
			}
			orphaned := true
			for _, res := range nodeAllocs {
				if res.NodeID == alloc.Node && res.Status == "active" {
					orphaned = false
					break
				}
			}
			if orphaned {
				if err := s.store.DeleteAllocation(ctx, alloc.ID, nil); err != nil {
					continue
				}
				s.increment(func(m *Metrics) { m.OrphanedAllocationsCleaned++ })
				cleaned++
			}
		}
	}
	return cleaned, nil
}

func (s *Service) Inspect(ctx context.Context) (Info, error) {
	if s == nil || s.store == nil {
		return Info{}, errors.New("cleanup service unavailable")
	}
	expired, err := s.store.ExpirePlacementReservations(ctx)
	if err != nil {
		return Info{}, err
	}
	allocations, err := s.store.ListAllocations(ctx)
	if err != nil {
		return Info{}, err
	}
	orphaned := 0
	for _, alloc := range allocations {
		if alloc.Server == nil {
			orphaned++
		}
	}
	return Info{
		StaleReservations:  len(expired),
		OrphanedAllocations: orphaned,
	}, nil
}

func (s *Service) publish(ctx context.Context, eventType events.EventType, resourceType, resourceID string, payload map[string]any) {
	if s == nil || s.publisher == nil {
		return
	}
	if correlationID := events.CorrelationIDFromContext(ctx); correlationID != "" {
		if _, exists := payload["correlationId"]; !exists {
			payload["correlationId"] = correlationID
		}
	}
	_ = s.publisher.Publish(ctx, events.NewEnvelope(eventType, "cleanup", resourceType, resourceID, payload))
}

func (s *Service) increment(update func(*Metrics)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	update(&s.metrics)
}
