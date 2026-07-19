package queue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type memoryStore struct {
	mu      sync.Mutex
	jobs    map[string]*Job
	retries int
}

func newMemoryStore() *memoryStore { return &memoryStore{jobs: map[string]*Job{}} }
func (m *memoryStore) Enqueue(_ context.Context, j *Job) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *j
	m.jobs[j.ID] = &cp
	return nil
}
func (m *memoryStore) Dequeue(_ context.Context, _ string, worker string, lease time.Duration) (*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.Status == JobStatusPending && !j.AvailableAt.After(time.Now()) {
			cp := *j
			j.Status = JobStatusRunning
			j.LockedBy = worker
			until := time.Now().Add(lease)
			j.LockedUntil = &until
			return &cp, nil
		}
	}
	return nil, nil
}
func (m *memoryStore) Acknowledge(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs[id].Status = JobStatusCompleted
	return nil
}
func (m *memoryStore) Fail(_ context.Context, id string, err error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs[id].Status = JobStatusFailed
	if err != nil {
		m.jobs[id].Error = err.Error()
	}
	return nil
}
func (m *memoryStore) Retry(_ context.Context, id string, err error, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.jobs[id]
	j.Status = JobStatusPending
	j.RetryCount++
	j.AvailableAt = at
	m.retries++
	return nil
}
func (m *memoryStore) Heartbeat(context.Context, string, string, time.Duration) error { return nil }
func (m *memoryStore) ListPending(context.Context, string) ([]Job, error)             { return nil, nil }
func (m *memoryStore) GetJob(_ context.Context, id string) (*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j := m.jobs[id]; j != nil {
		cp := *j
		return &cp, nil
	}
	return nil, nil
}

func TestFailedJobUsesRetryUpdateInsteadOfDuplicateInsert(t *testing.T) {
	store := newMemoryStore()
	service := New(store, 1)
	service.RegisterHandler(JobServerStart, func(context.Context, *Job) error { return errors.New("temporary") })
	job, err := service.Dispatch(context.Background(), JobServerStart, "", "", map[string]string{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.Dequeue(context.Background(), "", service.workerID, service.lease)
	if err != nil {
		t.Fatal(err)
	}
	service.process(context.Background(), claimed)
	stored, _ := store.GetJob(context.Background(), job.ID)
	if store.retries != 1 || stored.Status != JobStatusPending || stored.RetryCount != 1 {
		t.Fatalf("job was not rescheduled in place: %+v retries=%d", stored, store.retries)
	}
}

func TestDispatchIdempotentUsesStableOperationID(t *testing.T) {
	store := newMemoryStore()
	service := New(store, 1)
	first, err := service.DispatchIdempotent(context.Background(), "request-1", JobServerRestart, "srv", "", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.DispatchIdempotent(context.Background(), "request-1", JobServerRestart, "srv", "", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("idempotent dispatch IDs differ: %s %s", first.ID, second.ID)
	}
}
