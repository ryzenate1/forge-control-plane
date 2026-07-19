package eventstore

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	"gamepanel/forge/internal/events"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testDB struct {
	db *sql.DB
}

func (t *testDB) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	res, err := t.db.ExecContext(ctx, sql, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

type sqlRowsWrap struct {
	*sql.Rows
}

func (w *sqlRowsWrap) Close() {
	w.Rows.Close()
}

func (t *testDB) Query(ctx context.Context, sql string, args ...any) (rows, error) {
	r, err := t.db.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return &sqlRowsWrap{Rows: r}, nil
}

func newTestStore(db *sql.DB) *EventStore {
	return &EventStore{
		db: &testDB{db: db},
		pendingQuery: `SELECT id, type, source, resource_type, resource_id, correlation_id, payload, created_at, dispatched, failure_count, last_error
		 FROM events WHERE dispatched = false AND failure_count < ?
		 ORDER BY created_at ASC LIMIT ?`,
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	_, err = db.ExecContext(context.Background(),
		`CREATE TABLE events (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			source TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			correlation_id TEXT NOT NULL DEFAULT '',
			payload TEXT NOT NULL DEFAULT '{}',
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			dispatched BOOLEAN NOT NULL DEFAULT false,
			failure_count INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			claimed_by TEXT,
			claimed_until TIMESTAMP
		)`)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), `CREATE INDEX IF NOT EXISTS idx_events_dispatched ON events (dispatched)`)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), `CREATE INDEX IF NOT EXISTS idx_events_created_at ON events (created_at)`)
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), `CREATE INDEX IF NOT EXISTS idx_events_type ON events (type)`)
	require.NoError(t, err)
	return db
}

func TestPublish(t *testing.T) {
	db := setupTestDB(t)
	store := newTestStore(db)

	envelope := events.NewEnvelope(
		events.EventServerCreated,
		"test-source",
		"server",
		uuid.NewString(),
		map[string]any{"name": "test-server"},
	)

	err := store.Publish(context.Background(), envelope)
	require.NoError(t, err)

	var stored StoredEvent
	row := db.QueryRow("SELECT id, type, source, resource_type, resource_id, correlation_id, payload, created_at, dispatched FROM events WHERE id = $1", envelope.ID)
	err = row.Scan(&stored.ID, &stored.Type, &stored.Source, &stored.ResourceType, &stored.ResourceID,
		&stored.CorrelationID, &stored.Payload, &stored.CreatedAt, &stored.Dispatched)
	require.NoError(t, err)

	assert.Equal(t, envelope.ID, stored.ID)
	assert.Equal(t, string(envelope.Type), stored.Type)
	assert.Equal(t, envelope.Source, stored.Source)
	assert.Equal(t, envelope.ResourceType, stored.ResourceType)
	assert.Equal(t, envelope.ResourceID, stored.ResourceID)
	assert.Equal(t, envelope.CorrelationID, stored.CorrelationID)
	assert.False(t, stored.Dispatched)
	assert.Contains(t, stored.Payload, "test-server")
}

func TestPendingReturnsOnlyUndispatched(t *testing.T) {
	db := setupTestDB(t)
	store := newTestStore(db)

	for range 5 {
		env := events.NewEnvelope(
			events.EventType("test-type"),
			"source",
			"resource",
			uuid.NewString(),
			map[string]any{},
		)
		require.NoError(t, store.Publish(context.Background(), env))
	}

	pending, err := store.Pending(context.Background(), 10)
	require.NoError(t, err)
	assert.Len(t, pending, 5)

	require.NoError(t, store.MarkDispatched(context.Background(), pending[0].ID))

	pending2, err := store.Pending(context.Background(), 10)
	require.NoError(t, err)
	assert.Len(t, pending2, 4)

	for _, p := range pending2 {
		assert.False(t, p.Dispatched)
	}
}

func TestPendingOrderedByCreationTime(t *testing.T) {
	db := setupTestDB(t)
	store := newTestStore(db)

	var ids []string
	for range 3 {
		env := events.NewEnvelope(
			events.EventType("test-type"),
			"source",
			"resource",
			uuid.NewString(),
			map[string]any{},
		)
		require.NoError(t, store.Publish(context.Background(), env))
		ids = append(ids, env.ID)
		time.Sleep(10 * time.Millisecond)
	}

	pending, err := store.Pending(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, pending, 3)

	assert.Equal(t, ids[0], pending[0].ID)
	assert.Equal(t, ids[1], pending[1].ID)
	assert.Equal(t, ids[2], pending[2].ID)
}

func TestPendingRespectsLimit(t *testing.T) {
	db := setupTestDB(t)
	store := newTestStore(db)

	for range 10 {
		env := events.NewEnvelope(
			events.EventType("test-type"),
			"source",
			"resource",
			uuid.NewString(),
			nil,
		)
		require.NoError(t, store.Publish(context.Background(), env))
	}

	pending, err := store.Pending(context.Background(), 3)
	require.NoError(t, err)
	assert.Len(t, pending, 3)
}

func TestMarkDispatched(t *testing.T) {
	db := setupTestDB(t)
	store := newTestStore(db)

	envelope := events.NewEnvelope(
		events.EventServerDeleted,
		"test",
		"server",
		uuid.NewString(),
		nil,
	)
	require.NoError(t, store.Publish(context.Background(), envelope))

	err := store.MarkDispatched(context.Background(), envelope.ID)
	require.NoError(t, err)

	var dispatched bool
	err = db.QueryRow("SELECT dispatched FROM events WHERE id = $1", envelope.ID).Scan(&dispatched)
	require.NoError(t, err)
	assert.True(t, dispatched)
}

func TestOutboxPublisherPersistsAndDispatches(t *testing.T) {
	db := setupTestDB(t)
	store := newTestStore(db)
	registry := events.NewRegistry("test")
	op := NewOutboxPublisher(store, registry)

	var delivered atomic.Int32
	registry.Subscribe(events.WildcardEventType, events.HandlerFunc(func(ctx context.Context, env events.Envelope) error {
		delivered.Add(1)
		return nil
	}))

	envelope := events.NewEnvelope(
		events.EventServerCreated,
		"test-source",
		"server",
		uuid.NewString(),
		map[string]any{"name": "test-server"},
	)

	err := op.Publish(context.Background(), envelope)
	require.NoError(t, err)

	var stored StoredEvent
	row := db.QueryRow("SELECT id, type, source, resource_type, resource_id, correlation_id, payload, created_at, dispatched FROM events WHERE id = $1", envelope.ID)
	err = row.Scan(&stored.ID, &stored.Type, &stored.Source, &stored.ResourceType, &stored.ResourceID,
		&stored.CorrelationID, &stored.Payload, &stored.CreatedAt, &stored.Dispatched)
	require.NoError(t, err)
	assert.Equal(t, envelope.ID, stored.ID)
	assert.False(t, stored.Dispatched)

	assert.Equal(t, int32(1), delivered.Load())
}

func TestOutboxPublisherReturnsStoreError(t *testing.T) {
	db := setupTestDB(t)
	store := newTestStore(db)
	registry := events.NewRegistry("test")
	op := NewOutboxPublisher(store, registry)

	envelope := events.NewEnvelope(
		events.EventServerCreated,
		"test",
		"server",
		uuid.NewString(),
		nil,
	)

	_, err := db.ExecContext(context.Background(), `DROP TABLE events`)
	require.NoError(t, err)

	err = op.Publish(context.Background(), envelope)
	require.Error(t, err)
}

func TestRelayPollsAndDispatches(t *testing.T) {
	db := setupTestDB(t)
	store := newTestStore(db)
	relay := NewRelay(store, 50*time.Millisecond)

	var delivered atomic.Int32
	relay.Subscribe(func(ctx context.Context, env events.Envelope) error {
		delivered.Add(1)
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	relay.Start(ctx)

	require.NoError(t, store.Publish(context.Background(),
		events.NewEnvelope(events.EventServerCreated, "test", "server", uuid.NewString(), nil)))
	require.NoError(t, store.Publish(context.Background(),
		events.NewEnvelope(events.EventServerDeleted, "test", "server", uuid.NewString(), nil)))
	require.NoError(t, store.Publish(context.Background(),
		events.NewEnvelope(events.EventNodeOnline, "test", "node", uuid.NewString(), nil)))

	<-ctx.Done()
	relay.Stop()

	assert.Equal(t, int32(3), delivered.Load())

	pending, err := store.Pending(context.Background(), 10)
	require.NoError(t, err)
	assert.Empty(t, pending)
}

func TestRelayStopsOnCancellation(t *testing.T) {
	db := setupTestDB(t)
	store := newTestStore(db)
	relay := NewRelay(store, 10*time.Hour)

	var delivered atomic.Int32
	relay.Subscribe(func(ctx context.Context, env events.Envelope) error {
		delivered.Add(1)
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	relay.Start(ctx)

	require.NoError(t, store.Publish(context.Background(),
		events.NewEnvelope(events.EventServerCreated, "test", "server", uuid.NewString(), nil)))

	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, int32(0), delivered.Load())

	cancel()
	relay.Stop()

	pending, err := store.Pending(context.Background(), 10)
	require.NoError(t, err)
	assert.Len(t, pending, 1)
}
