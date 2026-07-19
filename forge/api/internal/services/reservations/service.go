package reservations

import (
	"context"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"
)

type Metrics struct {
	PlacementReservationsTotal  uint64 `json:"placement_reservations_total"`
	ReservationConflictsTotal   uint64 `json:"reservation_conflicts_total"`
	ReservationExpirationsTotal uint64 `json:"reservation_expirations_total"`
}

type Manager struct {
	store     *store.Store
	publisher events.Publisher
	mu        sync.Mutex
	metrics   Metrics
}

func New(store *store.Store, publishers ...events.Publisher) *Manager {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Manager{store: store, publisher: publisher}
}

func (m *Manager) Metrics() Metrics {
	if m == nil {
		return Metrics{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.metrics
}

func (m *Manager) Start(ctx context.Context) {
	if m == nil || m.store == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = m.ExpireReservations(ctx)
			}
		}
	}()
}

func (m *Manager) CreateReservation(ctx context.Context, req store.CreatePlacementReservationRequest) (store.PlacementReservation, error) {
	reservation, err := m.store.CreatePlacementReservation(ctx, req)
	if err != nil {
		if isConflict(err) {
			m.increment(func(metrics *Metrics) {
				metrics.ReservationConflictsTotal++
			})
		}
		return store.PlacementReservation{}, err
	}
	m.increment(func(metrics *Metrics) {
		metrics.PlacementReservationsTotal++
	})
	m.publish(ctx, events.EventReservationCreated, reservation)
	return reservation, nil
}

func (m *Manager) ConfirmReservation(ctx context.Context, reservationID string) (store.PlacementReservation, error) {
	reservation, err := m.store.UpdatePlacementReservationStatus(ctx, reservationID, store.PlacementReservationStatusCompleted)
	if err != nil {
		return store.PlacementReservation{}, err
	}
	m.publish(ctx, events.EventReservationConfirmed, reservation)
	return reservation, nil
}

func (m *Manager) CancelReservation(ctx context.Context, reservationID string) (store.PlacementReservation, error) {
	reservation, err := m.store.UpdatePlacementReservationStatus(ctx, reservationID, store.PlacementReservationStatusCancelled)
	if err != nil {
		return store.PlacementReservation{}, err
	}
	m.publish(ctx, events.EventReservationCancelled, reservation)
	return reservation, nil
}

func (m *Manager) ExpireReservation(ctx context.Context, reservationID string) (store.PlacementReservation, error) {
	reservation, err := m.store.UpdatePlacementReservationStatus(ctx, reservationID, store.PlacementReservationStatusExpired)
	if err != nil {
		return store.PlacementReservation{}, err
	}
	m.increment(func(metrics *Metrics) {
		metrics.ReservationExpirationsTotal++
	})
	m.publish(ctx, events.EventReservationExpired, reservation)
	return reservation, nil
}

func (m *Manager) ExpireReservations(ctx context.Context) ([]store.PlacementReservation, error) {
	reservations, err := m.store.ExpirePlacementReservations(ctx)
	if err != nil {
		return nil, err
	}
	for _, reservation := range reservations {
		m.increment(func(metrics *Metrics) {
			metrics.ReservationExpirationsTotal++
		})
		m.publish(ctx, events.EventReservationExpired, reservation)
	}
	return reservations, nil
}

func (m *Manager) CompleteMigrationReservations(ctx context.Context, migrationID string) {
	m.updateMigrationReservations(ctx, migrationID, store.PlacementReservationStatusCompleted, events.EventReservationConfirmed)
}

func (m *Manager) CancelMigrationReservations(ctx context.Context, migrationID string) {
	m.updateMigrationReservations(ctx, migrationID, store.PlacementReservationStatusCancelled, events.EventReservationCancelled)
}

func (m *Manager) ListReservations(ctx context.Context) ([]store.PlacementReservation, error) {
	return m.store.ListPlacementReservations(ctx)
}

func (m *Manager) GetReservation(ctx context.Context, reservationID string) (store.PlacementReservation, error) {
	return m.store.GetPlacementReservation(ctx, reservationID)
}

func (m *Manager) publish(ctx context.Context, eventType events.EventType, reservation store.PlacementReservation) {
	if m == nil || m.publisher == nil {
		return
	}
	payload := map[string]any{
		"nodeId":          reservation.NodeID,
		"reservationType": reservation.ReservationType,
		"status":          reservation.Status,
		"cpu":             reservation.CPU,
		"memory":          reservation.Memory,
		"disk":            reservation.Disk,
		"expiresAt":       reservation.ExpiresAt,
	}
	if reservation.ServerID != nil {
		payload["serverId"] = *reservation.ServerID
	}
	if reservation.MigrationID != nil {
		payload["migrationId"] = *reservation.MigrationID
	}
	if correlationID := events.CorrelationIDFromContext(ctx); correlationID != "" {
		payload["correlationId"] = correlationID
	}
	_ = m.publisher.Publish(ctx, events.NewEnvelope(eventType, "reservation-manager", "reservation", reservation.ID, payload))
}

func (m *Manager) updateMigrationReservations(ctx context.Context, migrationID string, status store.PlacementReservationStatus, eventType events.EventType) {
	if m == nil || m.store == nil || strings.TrimSpace(migrationID) == "" {
		return
	}
	reservations, err := m.store.UpdatePlacementReservationsForMigration(ctx, migrationID, status)
	if err != nil {
		return
	}
	for _, reservation := range reservations {
		m.publish(ctx, eventType, reservation)
	}
}

func (m *Manager) increment(update func(*Metrics)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	update(&m.metrics)
}

func isConflict(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "reservation") || strings.Contains(text, "exceeds available capacity")
}
