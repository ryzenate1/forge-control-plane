package reservations

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
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

// reservationExpiryInterval is how often the manager reclaims reservations whose
// ExpiresAt has passed. It must stay well under the store's default reservation
// TTL, because an expired-but-still-active reservation keeps holding capacity.
const reservationExpiryInterval = time.Minute

type Manager struct {
	store     *store.Store
	publisher events.Publisher
	mu        sync.Mutex
	metrics   Metrics
	cancel    context.CancelFunc
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

// Start runs the expiry sweep that reclaims capacity held by reservations whose
// owner never confirmed or cancelled them. It is idempotent: a second call must
// not orphan the first loop, because an loop whose cancel handle was overwritten
// can never be stopped and keeps writing to the store after shutdown begins.
func (m *Manager) Start(ctx context.Context) {
	if m == nil || m.store == nil {
		return
	}
	m.mu.Lock()
	if m.cancel != nil {
		m.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.mu.Unlock()
	go m.expireLoop(runCtx)
}

func (m *Manager) Stop() {
	if m == nil {
		return
	}
	m.mu.Lock()
	cancel := m.cancel
	m.cancel = nil
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (m *Manager) expireLoop(ctx context.Context) {
	ticker := time.NewTicker(reservationExpiryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.runExpirySweep(ctx)
		}
	}
}

// runExpirySweep recovers per tick rather than per loop: a panic that killed the
// goroutine would silently stop reclaiming capacity for the rest of the process
// lifetime, which is exactly the failure the sweep exists to prevent.
func (m *Manager) runExpirySweep(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			slog.ErrorContext(ctx, "reservation manager panic", "panic", r, "stack", string(buf[:n]))
		}
	}()
	reservations, err := m.ExpireReservations(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "reservation expiry sweep failed", "error", err)
		return
	}
	if len(reservations) > 0 {
		slog.InfoContext(ctx, "reservation expiry sweep reclaimed capacity", "count", len(reservations))
	}
}

// ValidateReservationRequest rejects malformed reservation requests before
// they reach the store: an empty node, negative capacity (which would inflate
// available headroom), a claim on nothing at all, or a request that cannot hold
// capacity — a terminal status or an expiry that has already passed. The store
// counts a reservation as holding capacity only while it is pending or active
// and its expires_at is in the future, so accepting those would hand the caller
// a reservation that reports success while reserving nothing.
func ValidateReservationRequest(req store.CreatePlacementReservationRequest) error {
	if strings.TrimSpace(req.NodeID) == "" {
		return errors.New("nodeId is required")
	}
	if req.CPU < 0 || req.Memory < 0 || req.Disk < 0 {
		return errors.New("reservation capacity must not be negative")
	}
	if req.CPU == 0 && req.Memory == 0 && req.Disk == 0 {
		return errors.New("reservation must request capacity")
	}
	if !req.ExpiresAt.IsZero() && !req.ExpiresAt.After(time.Now()) {
		return errors.New("reservation expiry must be in the future")
	}
	if req.ReservationType != "" {
		switch req.ReservationType {
		case store.PlacementReservationTypePlacement,
			store.PlacementReservationTypeMigration,
			store.PlacementReservationTypeEvacuation,
			store.PlacementReservationTypeRecovery:
		default:
			return fmt.Errorf("unknown reservation type %q", req.ReservationType)
		}
	}
	if req.Status != "" {
		switch req.Status {
		case store.PlacementReservationStatusPending, store.PlacementReservationStatusActive:
		default:
			return fmt.Errorf("reservation status %q cannot hold capacity", req.Status)
		}
	}
	return nil
}

func (m *Manager) CreateReservation(ctx context.Context, req store.CreatePlacementReservationRequest) (store.PlacementReservation, error) {
	if err := ValidateReservationRequest(req); err != nil {
		return store.PlacementReservation{}, err
	}
	st, err := m.reservationStore()
	if err != nil {
		return store.PlacementReservation{}, err
	}
	reservation, err := st.CreatePlacementReservation(ctx, req)
	if err != nil {
		if IsConflict(err) {
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
	return m.transitionReservation(ctx, reservationID, store.PlacementReservationStatusCompleted, events.EventReservationConfirmed)
}

func (m *Manager) CancelReservation(ctx context.Context, reservationID string) (store.PlacementReservation, error) {
	return m.transitionReservation(ctx, reservationID, store.PlacementReservationStatusCancelled, events.EventReservationCancelled)
}

func (m *Manager) ExpireReservation(ctx context.Context, reservationID string) (store.PlacementReservation, error) {
	reservation, err := m.transitionReservation(ctx, reservationID, store.PlacementReservationStatusExpired, events.EventReservationExpired)
	if err != nil {
		return store.PlacementReservation{}, err
	}
	m.increment(func(metrics *Metrics) {
		metrics.ReservationExpirationsTotal++
	})
	return reservation, nil
}

func (m *Manager) transitionReservation(ctx context.Context, reservationID string, status store.PlacementReservationStatus, eventType events.EventType) (store.PlacementReservation, error) {
	st, err := m.reservationStore()
	if err != nil {
		return store.PlacementReservation{}, err
	}
	if strings.TrimSpace(reservationID) == "" {
		return store.PlacementReservation{}, errReservationIDRequired
	}
	reservation, err := st.UpdatePlacementReservationStatus(ctx, reservationID, status)
	if err != nil {
		return store.PlacementReservation{}, err
	}
	m.publish(ctx, eventType, reservation)
	return reservation, nil
}

func (m *Manager) ExpireReservations(ctx context.Context) ([]store.PlacementReservation, error) {
	st, err := m.reservationStore()
	if err != nil {
		return nil, err
	}
	reservations, err := st.ExpirePlacementReservations(ctx)
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
	st, err := m.reservationStore()
	if err != nil {
		return nil, err
	}
	return st.ListPlacementReservations(ctx)
}

func (m *Manager) GetReservation(ctx context.Context, reservationID string) (store.PlacementReservation, error) {
	st, err := m.reservationStore()
	if err != nil {
		return store.PlacementReservation{}, err
	}
	if strings.TrimSpace(reservationID) == "" {
		return store.PlacementReservation{}, errReservationIDRequired
	}
	return st.GetPlacementReservation(ctx, reservationID)
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
	st, err := m.reservationStore()
	if err != nil {
		slog.ErrorContext(ctx, "reservation manager unavailable, migration reservations left holding capacity", "migrationId", migrationID, "error", err)
		return
	}
	if strings.TrimSpace(migrationID) == "" {
		slog.ErrorContext(ctx, "migration reservations update requires a migration id")
		return
	}
	reservations, err := st.UpdatePlacementReservationsForMigration(ctx, migrationID, status)
	if err != nil {
		// A migration whose reservations never reached a terminal status keeps
		// its capacity reserved until the expiry sweep collects it; that is a
		// real leak the operator has to be able to see.
		slog.ErrorContext(ctx, "failed to update migration reservations", "migrationId", migrationID, "status", status, "error", err)
		return
	}
	for _, reservation := range reservations {
		m.publish(ctx, eventType, reservation)
	}
}

// reservationStore returns the store every Manager method writes through. A
// Manager built without one — or a nil Manager, which several services hold as
// an optional dependency — must answer with an error rather than panic in the
// middle of a placement.
func (m *Manager) reservationStore() (*store.Store, error) {
	if m == nil || m.store == nil {
		return nil, ErrUnavailable
	}
	return m.store, nil
}

func (m *Manager) increment(update func(*Metrics)) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	update(&m.metrics)
}

// ErrUnavailable is returned when a Manager was constructed without a store, or
// is nil. Reserving capacity is not optional for a correct placement, so this
// is an error and not a silent no-op.
var ErrUnavailable = errors.New("reservation manager is not configured")

var errReservationIDRequired = errors.New("reservation id is required")

// IsConflict reports whether a reservation attempt failed because the capacity,
// or the server or migration it belongs to, is already claimed — as opposed to
// the store being unreachable or the request being malformed. Only a conflict
// means "the next candidate may work"; everything else must be surfaced to the
// caller rather than folded into a capacity verdict.
//
// ErrUnavailable is deliberately not a conflict: a Manager that cannot reach the
// store has no evidence about capacity, so treating it as "try the next node"
// would let a scheduler place a workload on every node in turn.
func IsConflict(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, store.ErrReservationCapacityExceeded) ||
		errors.Is(err, store.ErrReservationServerBusy) ||
		errors.Is(err, store.ErrReservationMigrationBusy)
}
