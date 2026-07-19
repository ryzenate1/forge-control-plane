package reconciler

import (
	"context"
	"sync"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

const DefaultInterval = 30 * time.Second

type MetricsSnapshot struct {
	ReconciliationCount       uint64 `json:"reconciliation_count"`
	ReconciliationFailures    uint64 `json:"reconciliation_failures"`
	NodeRefreshFailures       uint64 `json:"node_refresh_failures"`
	ServerSyncFailures        uint64 `json:"server_sync_failures"`
	ServerReconciliationTotal uint64 `json:"server_reconciliation_total"`
	NodeReconciliationTotal   uint64 `json:"node_reconciliation_total"`
}

type Service struct {
	store          *store.Store
	clusterManager *clustermanager.Service
	publisher      events.Publisher
	interval       time.Duration
	mu             sync.Mutex
	metrics        MetricsSnapshot
}

func New(store *store.Store, clusterManager *clustermanager.Service, interval time.Duration, publishers ...events.Publisher) *Service {
	if interval <= 0 {
		interval = DefaultInterval
	}
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Service{store: store, clusterManager: clusterManager, publisher: publisher, interval: interval}
}

func (s *Service) Start(ctx context.Context) {
	if s == nil || s.store == nil || s.clusterManager == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.RunOnce(ctx)
			}
		}
	}()
}

func (s *Service) RunOnce(ctx context.Context) error {
	if s == nil || s.store == nil || s.clusterManager == nil {
		return nil
	}
	correlationID := uuid.NewString()
	ctx = events.ContextWithCorrelationID(ctx, correlationID)
	s.increment(func(metrics *MetricsSnapshot) {
		metrics.ReconciliationCount++
	})
	if err := s.refreshNodes(ctx, correlationID); err != nil {
		s.increment(func(metrics *MetricsSnapshot) {
			metrics.NodeRefreshFailures++
			metrics.ReconciliationFailures++
		})
		return err
	}
	if err := s.refreshServers(ctx, correlationID); err != nil {
		s.increment(func(metrics *MetricsSnapshot) {
			metrics.ServerSyncFailures++
			metrics.ReconciliationFailures++
		})
		return err
	}
	return nil
}

func (s *Service) Metrics() MetricsSnapshot {
	if s == nil {
		return MetricsSnapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metrics
}

func (s *Service) increment(update func(*MetricsSnapshot)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	update(&s.metrics)
}

func (s *Service) refreshNodes(ctx context.Context, correlationID string) error {
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if _, err := s.store.NodeCapacitySnapshot(ctx, node.ID); err != nil {
			s.increment(func(metrics *MetricsSnapshot) {
				metrics.NodeRefreshFailures++
			})
			continue
		}
		s.increment(func(metrics *MetricsSnapshot) {
			metrics.NodeReconciliationTotal++
		})
		s.reconcileNode(ctx, node, correlationID)
	}
	return nil
}

func (s *Service) refreshServers(ctx context.Context, correlationID string) error {
	servers, err := s.store.ListServers(ctx)
	if err != nil {
		return err
	}
	for _, server := range servers {
		s.increment(func(metrics *MetricsSnapshot) {
			metrics.ServerReconciliationTotal++
		})
		if err := s.reconcileServer(ctx, server, correlationID); err != nil {
			s.increment(func(metrics *MetricsSnapshot) {
				metrics.ServerSyncFailures++
				metrics.ReconciliationFailures++
			})
		}
	}
	return nil
}

func (s *Service) reconcileNode(ctx context.Context, node store.Node, correlationID string) {
	if node.DesiredState == store.NodeDesiredStateMaintenance || node.DesiredState == store.NodeDesiredStateDraining {
		s.publish(ctx, events.EventDesiredStateChanged, "node", node.ID, map[string]any{
			"desiredState":  node.DesiredState,
			"reason":        "reconciler placement guard",
			"correlationId": correlationID,
		})
		return
	}
}

func (s *Service) reconcileServer(ctx context.Context, server store.Server, correlationID string) error {
	s.publish(ctx, events.EventDesiredStateChanged, "server", server.ID, map[string]any{
		"desiredState":  server.DesiredState,
		"reason":        "reconciler comparison",
		"correlationId": correlationID,
	})
	actualState, err := s.clusterManager.RefreshServerActualState(ctx, server.ID)
	if err != nil {
		return err
	}
	storeActual := store.ServerActualState(actualState)
	// Refresh desired state after the remote observation. A user request may
	// have advanced the generation while the node call was in flight; acting
	// on the stale list snapshot could otherwise undo the newer request.
	current, err := s.store.GetServer(ctx, server.ID)
	if err != nil {
		return err
	}
	s.publish(ctx, events.EventActualStateChanged, "server", server.ID, map[string]any{
		"actualState":   storeActual,
		"reason":        "reconciler comparison",
		"correlationId": correlationID,
	})
	switch {
	case current.DesiredState == store.ServerDesiredStateRunning && storeActual == store.ServerActualStateStopped:
		_, err := s.clusterManager.StartServer(ctx, server.ID)
		return err
	case current.DesiredState == store.ServerDesiredStateStopped && storeActual == store.ServerActualStateRunning:
		_, err := s.clusterManager.StopServer(ctx, server.ID)
		return err
	case current.DesiredState == store.ServerDesiredStateRunning && storeActual == store.ServerActualStateCrashed:
		_, err := s.clusterManager.RestartServer(ctx, server.ID)
		return err
	default:
		return nil
	}
}

func (s *Service) publish(ctx context.Context, eventType events.EventType, resourceType, resourceID string, payload map[string]any) {
	if s == nil || s.publisher == nil {
		return
	}
	_ = s.publisher.Publish(ctx, events.NewEnvelope(eventType, "reconciler", resourceType, resourceID, payload))
}
