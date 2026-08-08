package operation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type memoryStore struct {
	mu       sync.Mutex
	ops      map[string]*Operation
	attempts map[string]int
}

func newMemoryStore() *memoryStore {
	return &memoryStore{ops: make(map[string]*Operation), attempts: make(map[string]int)}
}

func (m *memoryStore) Create(_ context.Context, op *Operation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.ops[op.ID]; !exists {
		copy := *op
		m.ops[op.ID] = &copy
	}
	return nil
}

func (m *memoryStore) Dequeue(context.Context) (*Operation, error) { return nil, nil }

func (m *memoryStore) Get(_ context.Context, id string) (*Operation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	op := m.ops[id]
	if op == nil {
		return nil, nil
	}
	copy := *op
	return &copy, nil
}

func (m *memoryStore) ListByResource(context.Context, string, string) ([]Operation, error) {
	return nil, nil
}

func (m *memoryStore) ListPending(context.Context, int) ([]Operation, error) { return nil, nil }

func (m *memoryStore) UpdateStatus(_ context.Context, id string, status Status, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	op := m.ops[id]
	if op == nil {
		return errors.New("operation not found")
	}
	op.Status = status
	op.Error = errMsg
	if status == StatusRunning {
		m.attempts[id]++
	}
	return nil
}

func (m *memoryStore) AttemptCount(_ context.Context, id string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.attempts[id], nil
}

func (m *memoryStore) Cancel(ctx context.Context, id string) error {
	return m.UpdateStatus(ctx, id, StatusCancelled, "")
}

func TestProcessStopsAfterConfiguredAttempts(t *testing.T) {
	st := newMemoryStore()
	svc := NewWithConfig(st, Config{
		MaxWorkers: 1, PollInterval: time.Millisecond, MaxRetries: 2,
		BaseBackoff: time.Millisecond, MaxBackoff: time.Millisecond,
	})
	op := &Operation{ID: "op-1", Kind: string(OpServerStart), Status: StatusQueued}
	if err := st.Create(context.Background(), op); err != nil {
		t.Fatal(err)
	}
	svc.RegisterHandler(OpServerStart, func(context.Context, *Operation) error {
		return errors.New("runtime unavailable")
	})

	svc.process(context.Background(), op)
	current, _ := st.Get(context.Background(), op.ID)
	if current.Status != StatusRetrying {
		t.Fatalf("first failure status=%s, want retrying", current.Status)
	}
	svc.process(context.Background(), op)
	current, _ = st.Get(context.Background(), op.ID)
	if current.Status != StatusFailed {
		t.Fatalf("second failure status=%s, want failed", current.Status)
	}
	if attempts, _ := st.AttemptCount(context.Background(), op.ID); attempts != 2 {
		t.Fatalf("attempts=%d, want 2", attempts)
	}
}

func TestDispatchPowerIdempotencyReturnsPersistedOperation(t *testing.T) {
	st := newMemoryStore()
	svc := New(st)
	first, err := svc.DispatchPower(context.Background(), "server-1", "restart", "request-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.DispatchPower(context.Background(), "server-1", "restart", "request-1")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || second.IdempotencyKey != "request-1" {
		t.Fatalf("idempotent replay mismatch: first=%+v second=%+v", first, second)
	}
}
