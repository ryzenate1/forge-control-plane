package eventstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gamepanel/forge/internal/events"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxFailureCount = 5

type rows interface {
	Close()
	Err() error
	Next() bool
	Scan(dest ...any) error
}

type executor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (int64, error)
	Query(ctx context.Context, sql string, args ...any) (rows, error)
}

type poolExecutor struct {
	pool *pgxpool.Pool
}

func (e *poolExecutor) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	tag, err := e.pool.Exec(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (e *poolExecutor) Query(ctx context.Context, sql string, args ...any) (rows, error) {
	r, err := e.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return r, nil
}

type StoredEvent struct {
	ID            string
	Type          string
	Source        string
	ResourceType  string
	ResourceID    string
	CorrelationID string
	Payload       string
	CreatedAt     time.Time
	Dispatched    bool
	FailureCount  int
	LastError     string
	ClaimToken    string
}

// ClaimPending atomically leases a batch to one relay. The lease allows a
// crashed relay's events to become available again without duplicate claims
// by healthy replicas.
func (s *EventStore) ClaimPending(ctx context.Context, limit int, claimToken string, lease time.Duration) ([]StoredEvent, error) {
	if s.pendingQuery != "" {
		return s.Pending(ctx, limit)
	}
	r, err := s.db.Query(ctx, `WITH candidates AS (
		SELECT id FROM events WHERE dispatched=false AND failure_count < $1
		AND (claimed_until IS NULL OR claimed_until < NOW())
		ORDER BY created_at ASC LIMIT $2 FOR UPDATE SKIP LOCKED)
		UPDATE events e SET claimed_by=$3, claimed_until=NOW()+$4::interval
		FROM candidates WHERE e.id=candidates.id
		RETURNING e.id,e.type,e.source,e.resource_type,e.resource_id,e.correlation_id,e.payload,e.created_at,
		e.dispatched,e.failure_count,e.last_error,e.claimed_by`, maxFailureCount, limit, claimToken, lease.String())
	if err != nil {
		return nil, fmt.Errorf("eventstore claim query: %w", err)
	}
	defer r.Close()
	var out []StoredEvent
	for r.Next() {
		var e StoredEvent
		if err := r.Scan(&e.ID, &e.Type, &e.Source, &e.ResourceType, &e.ResourceID, &e.CorrelationID, &e.Payload,
			&e.CreatedAt, &e.Dispatched, &e.FailureCount, &e.LastError, &e.ClaimToken); err != nil {
			return nil, fmt.Errorf("eventstore claim scan: %w", err)
		}
		out = append(out, e)
	}
	return out, r.Err()
}

type EventStore struct {
	db           executor
	pendingQuery string
}

func (s *EventStore) pendingSQL() string {
	if s.pendingQuery != "" {
		return s.pendingQuery
	}
	return `SELECT id, type, source, resource_type, resource_id, correlation_id, payload, created_at, dispatched, failure_count, last_error
		 FROM events WHERE dispatched = false AND failure_count < $1
		 ORDER BY created_at ASC LIMIT $2 FOR UPDATE SKIP LOCKED`
}

func New(pool *pgxpool.Pool) *EventStore {
	return &EventStore{db: &poolExecutor{pool: pool}}
}

func (s *EventStore) Publish(ctx context.Context, envelope events.Envelope) error {
	if envelope.ID == "" {
		envelope.ID = uuid.NewString()
	}
	if envelope.Timestamp.IsZero() {
		envelope.Timestamp = time.Now().UTC()
	}
	payloadBytes, err := json.Marshal(envelope.Payload)
	if err != nil {
		return fmt.Errorf("eventstore publish marshal: %w", err)
	}
	_, err = s.db.Exec(ctx,
		`INSERT INTO events (id, type, source, resource_type, resource_id, correlation_id, payload, created_at, dispatched, failure_count, last_error)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		envelope.ID, string(envelope.Type), envelope.Source, envelope.ResourceType,
		envelope.ResourceID, envelope.CorrelationID, string(payloadBytes), envelope.Timestamp, false, 0, "")
	if err != nil {
		return fmt.Errorf("eventstore publish insert: %w", err)
	}
	return nil
}

func (s *EventStore) Pending(ctx context.Context, limit int) ([]StoredEvent, error) {
	r, err := s.db.Query(ctx, s.pendingSQL(), maxFailureCount, limit)
	if err != nil {
		return nil, fmt.Errorf("eventstore pending query: %w", err)
	}
	defer r.Close()

	var eventsList []StoredEvent
	for r.Next() {
		var e StoredEvent
		if err := r.Scan(&e.ID, &e.Type, &e.Source, &e.ResourceType, &e.ResourceID,
			&e.CorrelationID, &e.Payload, &e.CreatedAt, &e.Dispatched, &e.FailureCount, &e.LastError); err != nil {
			return nil, fmt.Errorf("eventstore pending scan: %w", err)
		}
		eventsList = append(eventsList, e)
	}
	if err := r.Err(); err != nil {
		return nil, fmt.Errorf("eventstore pending rows: %w", err)
	}
	return eventsList, nil
}

func (s *EventStore) MarkDispatched(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `UPDATE events SET dispatched = true, claimed_by=NULL, claimed_until=NULL WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("eventstore mark dispatched: %w", err)
	}
	return nil
}

func (s *EventStore) MarkFailed(ctx context.Context, id string, lastErr string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE events SET failure_count = failure_count + 1, last_error = $1, claimed_by=NULL, claimed_until=NULL WHERE id = $2`,
		lastErr, id)
	if err != nil {
		return fmt.Errorf("eventstore mark failed: %w", err)
	}
	return nil
}

func (s *EventStore) MoveToDeadLetter(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO events_dead_letter (id, type, source, resource_type, resource_id, correlation_id, payload, created_at, failed_at, failure_count, last_error)
		 SELECT id, type, source, resource_type, resource_id, correlation_id, payload, created_at, NOW(), failure_count, last_error
		 FROM events WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("eventstore move to dead letter insert: %w", err)
	}
	_, err = s.db.Exec(ctx, `DELETE FROM events WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("eventstore move to dead letter delete: %w", err)
	}
	return nil
}

type OutboxPublisher struct {
	store    *EventStore
	registry *events.Registry
}

func NewOutboxPublisher(store *EventStore, registry *events.Registry) *OutboxPublisher {
	return &OutboxPublisher{store: store, registry: registry}
}

func (p *OutboxPublisher) Publish(ctx context.Context, envelope events.Envelope) error {
	if err := p.store.Publish(ctx, envelope); err != nil {
		return err
	}
	return p.registry.Publish(ctx, envelope)
}

func (s *EventStore) Count(ctx context.Context, dispatched bool) (int, error) {
	r, err := s.db.Query(ctx, `SELECT COUNT(*) FROM events WHERE dispatched = $1`, dispatched)
	if err != nil {
		return 0, fmt.Errorf("eventstore count: %w", err)
	}
	defer r.Close()
	var count int
	if r.Next() {
		if err := r.Scan(&count); err != nil {
			return 0, fmt.Errorf("eventstore count scan: %w", err)
		}
	}
	return count, r.Err()
}
