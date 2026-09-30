package eventstore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gamepanel/forge/internal/events"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// PublishTx/PublishTxBatch were removed by the outbox refactor: services now
// persist business state and call EventStore.Publish (fire-and-forget outbox
// insert). Relay also lost SubscriberCount/SubscribeSubscriber/SubscribeTyped
// and the "zero subscribers must not dispatch" guard — delivery is now plain
// fan-out to every Subscribe() handler. The tests below pin the current
// contracts.
// ---------------------------------------------------------------------------

type recordingExecutor struct {
	execs     []string
	args      [][]any
	failOn    int // 1-indexed call to fail, 0 = never fail
	shouldErr error
}

func (m *recordingExecutor) Exec(_ context.Context, sql string, arguments ...any) (int64, error) {
	m.execs = append(m.execs, sql)
	m.args = append(m.args, arguments)
	if m.failOn > 0 && len(m.execs) == m.failOn {
		if m.shouldErr != nil {
			return 0, m.shouldErr
		}
		return 0, errors.New("injected failure")
	}
	return 1, nil
}

func (m *recordingExecutor) Query(context.Context, string, ...any) (rows, error) {
	return nil, errors.New("unexpected query in recording executor")
}

func TestPublish_InsertShape_NoTenantColumn(t *testing.T) {
	rec := &recordingExecutor{}
	store := &EventStore{db: rec}
	env := events.NewEnvelope(events.EventServerCreated, "unit-test", "server", uuid.NewString(), map[string]any{"ok": true})

	require.NoError(t, store.Publish(context.Background(), env))
	require.Len(t, rec.execs, 1)
	assert.Contains(t, rec.execs[0], "INSERT INTO events")
	assert.NotContains(t, rec.execs[0], "tenant_id", "outbox insert must not reference the removed tenant_id column")
	require.Len(t, rec.args[0], 11) // id,type,source,resource_type,resource_id,correlation_id,payload,created_at,dispatched,failure_count,last_error
	assert.Equal(t, env.ID, rec.args[0][0], "Publish must not replace a provided ID")
	assert.Equal(t, string(env.Type), rec.args[0][1])
	assert.Equal(t, env.Source, rec.args[0][2])
	assert.Equal(t, false, rec.args[0][8])
	assert.Equal(t, 0, rec.args[0][9])
	payloadStr := rec.args[0][6].(string)
	var payloadMap map[string]any
	require.NoError(t, json.Unmarshal([]byte(payloadStr), &payloadMap))
	assert.Equal(t, true, payloadMap["ok"], "payload must be JSON, not Go map stringification")
}

func TestPublish_GeneratesIDAndTimestampWhenZero(t *testing.T) {
	rec := &recordingExecutor{}
	store := &EventStore{db: rec}
	env := events.Envelope{
		Type:         events.EventServerCreated,
		Source:       "unit-test",
		ResourceType: "server",
		ResourceID:   uuid.NewString(),
		Payload:      map[string]any{"x": 1},
		// ID and Timestamp intentionally zero
	}
	require.NoError(t, store.Publish(context.Background(), env))
	require.Len(t, rec.args, 1)
	gotID := rec.args[0][0].(string)
	if gotID == "" {
		t.Fatal("Publish with empty ID should generate one")
	}
	if _, err := uuid.Parse(gotID); err != nil {
		t.Fatalf("generated ID not a UUID: %q err=%v", gotID, err)
	}
	ts := rec.args[0][7].(time.Time)
	if ts.IsZero() {
		t.Fatal("Publish with zero Timestamp should generate one")
	}
}

func TestPublish_InsertErrorPropagates(t *testing.T) {
	rec := &recordingExecutor{failOn: 1, shouldErr: errors.New("unique violation")}
	store := &EventStore{db: rec}
	env := events.NewEnvelope(events.EventServerCreated, "unit-test", "server", uuid.NewString(), nil)
	err := store.Publish(context.Background(), env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "publish insert")
}

// ---------------------------------------------------------------------------
// Relay delivery contract
// ---------------------------------------------------------------------------

func TestRelay_SubscribeDeliversAndDispatches(t *testing.T) {
	db := setupTestDB(t)
	store := newTestStore(db)
	relay := NewRelay(store, 20*time.Millisecond)

	var calls atomic.Int32
	relay.Subscribe(func(ctx context.Context, env events.Envelope) error {
		calls.Add(1)
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	relay.Start(ctx)

	require.NoError(t, store.Publish(ctx, events.NewEnvelope(events.EventServerCreated, "test", "server", uuid.NewString(), nil)))
	require.NoError(t, store.Publish(ctx, events.NewEnvelope(events.EventServerDeleted, "test", "server", uuid.NewString(), nil)))

	deadline := time.After(500 * time.Millisecond)
	for calls.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("expected both events delivered, got %d", calls.Load())
		case <-time.After(10 * time.Millisecond):
		}
	}
	relay.Stop()

	pending, err := store.Pending(context.Background(), 10)
	require.NoError(t, err)
	assert.Empty(t, pending, "both events should be dispatched")
}

func TestRelay_HandlerFailure_IncrementsFailureCount(t *testing.T) {
	db := setupTestDB(t)
	store := newTestStore(db)
	relay := NewRelay(store, 20*time.Millisecond)
	relay.Subscribe(func(ctx context.Context, env events.Envelope) error {
		return errors.New("handler exploded")
	})

	env := events.NewEnvelope(events.EventServerCreated, "test", "server", uuid.NewString(), nil)
	require.NoError(t, store.Publish(context.Background(), env))

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	relay.Start(ctx)
	<-ctx.Done()
	relay.Stop()

	var dispatched bool
	var failureCount int
	var lastError string
	err := db.QueryRow("SELECT dispatched, failure_count, last_error FROM events WHERE id = ?", env.ID).Scan(&dispatched, &failureCount, &lastError)
	require.NoError(t, err)
	assert.False(t, dispatched, "failed delivery must not mark dispatched")
	assert.GreaterOrEqual(t, failureCount, 1)
	assert.True(t, strings.Contains(lastError, "handler error") || strings.Contains(lastError, "handler failed"),
		"last_error should describe the delivery failure, got %q", lastError)
}

func TestRelay_ZeroSubscribers_MarksDispatched(t *testing.T) {
	// Historical AF-1 guard (zero subscribers = configuration error, keep the
	// event pending) was removed with the relay refactor: with no handlers an
	// event trivially "succeeds" and is dispatched. This test pins the new
	// behavior so a future change to it is caught.
	db := setupTestDB(t)
	store := newTestStore(db)
	relay := NewRelay(store, 20*time.Millisecond)
	// Intentionally no Subscribe.

	env := events.NewEnvelope(events.EventServerCreated, "test", "server", uuid.NewString(), nil)
	require.NoError(t, store.Publish(context.Background(), env))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	relay.Start(ctx)
	<-ctx.Done()
	relay.Stop()

	var dispatched bool
	var failureCount int
	err := db.QueryRow("SELECT dispatched, failure_count FROM events WHERE id = ?", env.ID).Scan(&dispatched, &failureCount)
	require.NoError(t, err)
	assert.True(t, dispatched, "current relay dispatches even with zero subscribers")
	assert.Equal(t, 0, failureCount)
}

func TestPublish_PersistsDispatchedFalse(t *testing.T) {
	db := setupTestDB(t)
	store := newTestStore(db)
	env := events.NewEnvelope(events.EventServerCreated, "test", "server", uuid.NewString(), nil)
	require.NoError(t, store.Publish(context.Background(), env))
	var dispatched bool
	var failureCount int
	var lastError string
	err := db.QueryRow("SELECT dispatched, failure_count, last_error FROM events WHERE id = ?", env.ID).Scan(&dispatched, &failureCount, &lastError)
	require.NoError(t, err)
	assert.False(t, dispatched)
	assert.Equal(t, 0, failureCount)
	assert.Equal(t, "", lastError)
}
