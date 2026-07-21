package cleanup

import (
	"context"
	"errors"
	"testing"

	"gamepanel/forge/internal/store"
)

type mockCleanupStore struct {
	expireReservationsResult []store.PlacementReservation
	expireReservationsErr    error
	listReservationsResult   []store.PlacementReservation
	listReservationsErr      error
	listAllocationsResult    []store.Allocation
	listAllocationsErr       error
	deleteAllocationErr      error
}

func (m *mockCleanupStore) ExpirePlacementReservations(_ context.Context) ([]store.PlacementReservation, error) {
	return m.expireReservationsResult, m.expireReservationsErr
}

func (m *mockCleanupStore) ListPlacementReservations(_ context.Context) ([]store.PlacementReservation, error) {
	return m.listReservationsResult, m.listReservationsErr
}

func (m *mockCleanupStore) ListAllocations(_ context.Context) ([]store.Allocation, error) {
	return m.listAllocationsResult, m.listAllocationsErr
}

func (m *mockCleanupStore) DeleteAllocation(_ context.Context, _ string, _ *string) error {
	return m.deleteAllocationErr
}

func TestRunCleanup(t *testing.T) {
	t.Run("expires stale reservations", func(t *testing.T) {
		mock := &mockCleanupStore{
			expireReservationsResult: []store.PlacementReservation{{ID: "r-1"}, {ID: "r-2"}},
			listAllocationsResult:    []store.Allocation{},
		}
		svc := &Service{store: mock}
		info, err := svc.RunCleanup(context.Background())
		if err != nil {
			t.Fatalf("RunCleanup error: %v", err)
		}
		if info.StaleReservations != 2 {
			t.Fatalf("expected 2 stale reservations, got %d", info.StaleReservations)
		}
	})

	t.Run("handles errors gracefully", func(t *testing.T) {
		mock := &mockCleanupStore{
			expireReservationsErr: errors.New("db error"),
		}
		svc := &Service{store: mock}
		_, err := svc.RunCleanup(context.Background())
		if err == nil {
			t.Fatal("expected error from cleanup")
		}
	})

	t.Run("nil service returns error", func(t *testing.T) {
		var svc *Service
		_, err := svc.RunCleanup(context.Background())
		if err == nil {
			t.Fatal("expected error from nil service")
		}
	})
}

func TestInspect(t *testing.T) {
	mock := &mockCleanupStore{
		listAllocationsResult: []store.Allocation{
			{ID: "a-1", IP: "10.0.0.1", Port: 8080},
			{ID: "a-2", IP: "10.0.0.1", Port: 8081, Server: strPtr("srv-1")},
		},
	}
	svc := &Service{store: mock}
	info, err := svc.Inspect(context.Background())
	if err != nil {
		t.Fatalf("Inspect error: %v", err)
	}
	_ = info
}

func strPtr(s string) *string { return &s }

func TestMetrics(t *testing.T) {
	var svc *Service
	m := svc.Metrics()
	if m.CleanupRunsTotal != 0 {
		t.Fatal("expected zero metrics")
	}
}
