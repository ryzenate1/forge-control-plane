package registrations

import (
	"context"
	"errors"
	"testing"
	"time"

	"gamepanel/forge/internal/store"
)

type mockRegistrationsStore struct {
	createOnboardingTokenResult  *store.OnboardingToken
	createOnboardingTokenErr     error
	createOnboardingTokenCalled  bool
	getOnboardingTokenResult     *store.OnboardingToken
	getOnboardingTokenErr        error
	getOnboardingTokenCalled     bool
	approveOnboardingTokenErr    error
	approveOnboardingTokenCalled bool
	rejectOnboardingTokenErr     error
	rejectOnboardingTokenCalled  bool
	revokeOnboardingTokenErr     error
	revokeOnboardingTokenCalled  bool
	listOnboardingTokensResult   []store.OnboardingToken
	listOnboardingTokensErr      error
	listOnboardingTokensCalled   bool
	upsertNodeCapabilityErr      error
	upsertNodeCapabilityCalled   bool
	getNodeCapabilityResult      *store.NodeCapability
	getNodeCapabilityErr         error
	getNodeCapabilityCalled      bool
	listCapabilitiesResult       []store.NodeCapability
	listCapabilitiesErr          error
	listCapabilitiesCalled       bool
	getCapabilityHistoryResult   []store.NodeCapabilityHistoryEntry
	getCapabilityHistoryErr      error
	getCapabilityHistoryCalled   bool
}

func (m *mockRegistrationsStore) CreateOnboardingToken(_ context.Context, _ string, _ time.Time) (*store.OnboardingToken, error) {
	m.createOnboardingTokenCalled = true
	return m.createOnboardingTokenResult, m.createOnboardingTokenErr
}

func (m *mockRegistrationsStore) GetOnboardingToken(_ context.Context, _ string) (*store.OnboardingToken, error) {
	m.getOnboardingTokenCalled = true
	return m.getOnboardingTokenResult, m.getOnboardingTokenErr
}

func (m *mockRegistrationsStore) ApproveOnboardingToken(_ context.Context, _, _ string) error {
	m.approveOnboardingTokenCalled = true
	return m.approveOnboardingTokenErr
}

func (m *mockRegistrationsStore) RejectOnboardingToken(_ context.Context, _, _ string) error {
	m.rejectOnboardingTokenCalled = true
	return m.rejectOnboardingTokenErr
}

func (m *mockRegistrationsStore) RevokeOnboardingToken(_ context.Context, _, _ string) error {
	m.revokeOnboardingTokenCalled = true
	return m.revokeOnboardingTokenErr
}

func (m *mockRegistrationsStore) ListOnboardingTokens(_ context.Context, _ string) ([]store.OnboardingToken, error) {
	m.listOnboardingTokensCalled = true
	return m.listOnboardingTokensResult, m.listOnboardingTokensErr
}

func (m *mockRegistrationsStore) UpsertNodeCapability(_ context.Context, _ *store.NodeCapability) error {
	m.upsertNodeCapabilityCalled = true
	return m.upsertNodeCapabilityErr
}

func (m *mockRegistrationsStore) GetNodeCapability(_ context.Context, _ string) (*store.NodeCapability, error) {
	m.getNodeCapabilityCalled = true
	return m.getNodeCapabilityResult, m.getNodeCapabilityErr
}

func (m *mockRegistrationsStore) ListCapabilities(_ context.Context, _ store.CapabilityInventoryFilter) ([]store.NodeCapability, error) {
	m.listCapabilitiesCalled = true
	return m.listCapabilitiesResult, m.listCapabilitiesErr
}

func (m *mockRegistrationsStore) GetCapabilityHistory(_ context.Context, _ string, _ int) ([]store.NodeCapabilityHistoryEntry, error) {
	m.getCapabilityHistoryCalled = true
	return m.getCapabilityHistoryResult, m.getCapabilityHistoryErr
}

func TestCreateOnboardingToken(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		token := &store.OnboardingToken{ID: "tok-1", NodeID: "node-1", State: "pending"}
		mock := &mockRegistrationsStore{createOnboardingTokenResult: token}
		svc := &Service{store: mock}
		got, err := svc.CreateOnboardingToken(context.Background(), "node-1", time.Now().Add(time.Hour))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "tok-1" {
			t.Fatalf("expected tok-1, got %s", got.ID)
		}
		if !mock.createOnboardingTokenCalled {
			t.Fatal("store CreateOnboardingToken not called")
		}
		m := svc.Metrics()
		if m.OnboardingTokensCreatedTotal != 1 {
			t.Fatalf("expected metrics count 1, got %d", m.OnboardingTokensCreatedTotal)
		}
	})

	t.Run("store error", func(t *testing.T) {
		mock := &mockRegistrationsStore{createOnboardingTokenErr: errors.New("db error")}
		svc := &Service{store: mock}
		_, err := svc.CreateOnboardingToken(context.Background(), "node-1", time.Now().Add(time.Hour))
		if err == nil {
			t.Fatal("expected error")
		}
		m := svc.Metrics()
		if m.OnboardingTokensCreatedTotal != 0 {
			t.Fatalf("expected metrics count 0, got %d", m.OnboardingTokensCreatedTotal)
		}
	})

	t.Run("nil service", func(t *testing.T) {
		var svc *Service
		_, err := svc.CreateOnboardingToken(context.Background(), "node-1", time.Now().Add(time.Hour))
		if err == nil {
			t.Fatal("expected error from nil service")
		}
	})
}

func TestGetOnboardingToken(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		token := &store.OnboardingToken{ID: "tok-1", NodeID: "node-1", State: "pending"}
		mock := &mockRegistrationsStore{getOnboardingTokenResult: token}
		svc := &Service{store: mock}
		got, err := svc.GetOnboardingToken(context.Background(), "tok-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "tok-1" {
			t.Fatalf("expected tok-1, got %s", got.ID)
		}
		if !mock.getOnboardingTokenCalled {
			t.Fatal("store GetOnboardingToken not called")
		}
	})

	t.Run("not found", func(t *testing.T) {
		mock := &mockRegistrationsStore{getOnboardingTokenErr: errors.New("not found")}
		svc := &Service{store: mock}
		_, err := svc.GetOnboardingToken(context.Background(), "tok-404")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("nil service", func(t *testing.T) {
		var svc *Service
		_, err := svc.GetOnboardingToken(context.Background(), "tok-1")
		if err == nil {
			t.Fatal("expected error from nil service")
		}
	})
}

func TestApproveOnboardingToken(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &mockRegistrationsStore{}
		svc := &Service{store: mock}
		err := svc.ApproveOnboardingToken(context.Background(), "tok-1", "admin")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !mock.approveOnboardingTokenCalled {
			t.Fatal("store ApproveOnboardingToken not called")
		}
		m := svc.Metrics()
		if m.OnboardingTokensApprovedTotal != 1 {
			t.Fatalf("expected metrics count 1, got %d", m.OnboardingTokensApprovedTotal)
		}
	})

	t.Run("store error", func(t *testing.T) {
		mock := &mockRegistrationsStore{approveOnboardingTokenErr: errors.New("token not found or already processed")}
		svc := &Service{store: mock}
		err := svc.ApproveOnboardingToken(context.Background(), "tok-1", "admin")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("nil service", func(t *testing.T) {
		var svc *Service
		err := svc.ApproveOnboardingToken(context.Background(), "tok-1", "admin")
		if err == nil {
			t.Fatal("expected error from nil service")
		}
	})
}

func TestRejectOnboardingToken(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &mockRegistrationsStore{}
		svc := &Service{store: mock}
		err := svc.RejectOnboardingToken(context.Background(), "tok-1", "expired")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !mock.rejectOnboardingTokenCalled {
			t.Fatal("store RejectOnboardingToken not called")
		}
		m := svc.Metrics()
		if m.OnboardingTokensRejectedTotal != 1 {
			t.Fatalf("expected metrics count 1, got %d", m.OnboardingTokensRejectedTotal)
		}
	})

	t.Run("not found error", func(t *testing.T) {
		mock := &mockRegistrationsStore{rejectOnboardingTokenErr: errors.New("token not found or already processed")}
		svc := &Service{store: mock}
		err := svc.RejectOnboardingToken(context.Background(), "tok-404", "expired")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("nil service", func(t *testing.T) {
		var svc *Service
		err := svc.RejectOnboardingToken(context.Background(), "tok-1", "expired")
		if err == nil {
			t.Fatal("expected error from nil service")
		}
	})
}

func TestRevokeOnboardingToken(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mock := &mockRegistrationsStore{}
		svc := &Service{store: mock}
		err := svc.RevokeOnboardingToken(context.Background(), "tok-1", "compromised")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !mock.revokeOnboardingTokenCalled {
			t.Fatal("store RevokeOnboardingToken not called")
		}
		m := svc.Metrics()
		if m.OnboardingTokensRevokedTotal != 1 {
			t.Fatalf("expected metrics count 1, got %d", m.OnboardingTokensRevokedTotal)
		}
	})

	t.Run("store error", func(t *testing.T) {
		mock := &mockRegistrationsStore{revokeOnboardingTokenErr: errors.New("token not found or not active")}
		svc := &Service{store: mock}
		err := svc.RevokeOnboardingToken(context.Background(), "tok-404", "compromised")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("nil service", func(t *testing.T) {
		var svc *Service
		err := svc.RevokeOnboardingToken(context.Background(), "tok-1", "compromised")
		if err == nil {
			t.Fatal("expected error from nil service")
		}
	})
}

func TestListOnboardingTokens(t *testing.T) {
	t.Run("returns tokens", func(t *testing.T) {
		tokens := []store.OnboardingToken{
			{ID: "tok-1", NodeID: "node-1", State: "pending"},
			{ID: "tok-2", NodeID: "node-1", State: "approved"},
		}
		mock := &mockRegistrationsStore{listOnboardingTokensResult: tokens}
		svc := &Service{store: mock}
		got, err := svc.ListOnboardingTokens(context.Background(), "node-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 tokens, got %d", len(got))
		}
		if !mock.listOnboardingTokensCalled {
			t.Fatal("store ListOnboardingTokens not called")
		}
		m := svc.Metrics()
		if m.OnboardingTokensListedTotal != 1 {
			t.Fatalf("expected metrics count 1, got %d", m.OnboardingTokensListedTotal)
		}
	})

	t.Run("empty list", func(t *testing.T) {
		mock := &mockRegistrationsStore{listOnboardingTokensResult: []store.OnboardingToken{}}
		svc := &Service{store: mock}
		got, err := svc.ListOnboardingTokens(context.Background(), "node-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected 0 tokens, got %d", len(got))
		}
	})

	t.Run("store error", func(t *testing.T) {
		mock := &mockRegistrationsStore{listOnboardingTokensErr: errors.New("db error")}
		svc := &Service{store: mock}
		_, err := svc.ListOnboardingTokens(context.Background(), "node-1")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("nil service", func(t *testing.T) {
		var svc *Service
		_, err := svc.ListOnboardingTokens(context.Background(), "node-1")
		if err == nil {
			t.Fatal("expected error from nil service")
		}
	})
}

func TestUpsertNodeCapability(t *testing.T) {
	t.Run("insert", func(t *testing.T) {
		mock := &mockRegistrationsStore{}
		svc := &Service{store: mock}
		nc := &store.NodeCapability{NodeID: "node-1", BeaconVersion: "1.0"}
		err := svc.UpsertNodeCapability(context.Background(), nc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !mock.upsertNodeCapabilityCalled {
			t.Fatal("store UpsertNodeCapability not called")
		}
		m := svc.Metrics()
		if m.NodeCapabilitiesUpsertedTotal != 1 {
			t.Fatalf("expected metrics count 1, got %d", m.NodeCapabilitiesUpsertedTotal)
		}
	})

	t.Run("update", func(t *testing.T) {
		mock := &mockRegistrationsStore{}
		svc := &Service{store: mock}
		nc := &store.NodeCapability{NodeID: "node-1", BeaconVersion: "2.0"}
		err := svc.UpsertNodeCapability(context.Background(), nc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !mock.upsertNodeCapabilityCalled {
			t.Fatal("store UpsertNodeCapability not called")
		}
		m := svc.Metrics()
		if m.NodeCapabilitiesUpsertedTotal != 1 {
			t.Fatalf("expected metrics count 1, got %d", m.NodeCapabilitiesUpsertedTotal)
		}
	})

	t.Run("store error", func(t *testing.T) {
		mock := &mockRegistrationsStore{upsertNodeCapabilityErr: errors.New("db error")}
		svc := &Service{store: mock}
		nc := &store.NodeCapability{NodeID: "node-1"}
		err := svc.UpsertNodeCapability(context.Background(), nc)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("nil service", func(t *testing.T) {
		var svc *Service
		nc := &store.NodeCapability{NodeID: "node-1"}
		err := svc.UpsertNodeCapability(context.Background(), nc)
		if err == nil {
			t.Fatal("expected error from nil service")
		}
	})
}

func TestGetNodeCapability(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		nc := &store.NodeCapability{ID: "cap-1", NodeID: "node-1", BeaconVersion: "1.0"}
		mock := &mockRegistrationsStore{getNodeCapabilityResult: nc}
		svc := &Service{store: mock}
		got, err := svc.GetNodeCapability(context.Background(), "node-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != "cap-1" {
			t.Fatalf("expected cap-1, got %s", got.ID)
		}
		if !mock.getNodeCapabilityCalled {
			t.Fatal("store GetNodeCapability not called")
		}
		m := svc.Metrics()
		if m.NodeCapabilitiesGetTotal != 1 {
			t.Fatalf("expected metrics count 1, got %d", m.NodeCapabilitiesGetTotal)
		}
	})

	t.Run("not found", func(t *testing.T) {
		mock := &mockRegistrationsStore{getNodeCapabilityErr: errors.New("not found")}
		svc := &Service{store: mock}
		_, err := svc.GetNodeCapability(context.Background(), "node-404")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("nil service", func(t *testing.T) {
		var svc *Service
		_, err := svc.GetNodeCapability(context.Background(), "node-1")
		if err == nil {
			t.Fatal("expected error from nil service")
		}
	})
}

func TestListCapabilities(t *testing.T) {
	t.Run("returns capabilities", func(t *testing.T) {
		caps := []store.NodeCapability{
			{ID: "cap-1", NodeID: "node-1"},
			{ID: "cap-2", NodeID: "node-2"},
		}
		mock := &mockRegistrationsStore{listCapabilitiesResult: caps}
		svc := &Service{store: mock}
		got, err := svc.ListCapabilities(context.Background(), store.CapabilityInventoryFilter{Offset: 0, Limit: 50})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 capabilities, got %d", len(got))
		}
		if !mock.listCapabilitiesCalled {
			t.Fatal("store ListCapabilities not called")
		}
		m := svc.Metrics()
		if m.NodeCapabilitiesListedTotal != 1 {
			t.Fatalf("expected metrics count 1, got %d", m.NodeCapabilitiesListedTotal)
		}
	})

	t.Run("empty", func(t *testing.T) {
		mock := &mockRegistrationsStore{listCapabilitiesResult: []store.NodeCapability{}}
		svc := &Service{store: mock}
		got, err := svc.ListCapabilities(context.Background(), store.CapabilityInventoryFilter{Offset: 0, Limit: 50})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected 0 capabilities, got %d", len(got))
		}
	})

	t.Run("store error", func(t *testing.T) {
		mock := &mockRegistrationsStore{listCapabilitiesErr: errors.New("db error")}
		svc := &Service{store: mock}
		_, err := svc.ListCapabilities(context.Background(), store.CapabilityInventoryFilter{Offset: 0, Limit: 50})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("nil service", func(t *testing.T) {
		var svc *Service
		_, err := svc.ListCapabilities(context.Background(), store.CapabilityInventoryFilter{Offset: 0, Limit: 50})
		if err == nil {
			t.Fatal("expected error from nil service")
		}
	})
}

func TestGetCapabilityHistory(t *testing.T) {
	t.Run("returns entries", func(t *testing.T) {
		entries := []store.NodeCapabilityHistoryEntry{
			{ID: "hist-1", NodeID: "node-1", BeaconVersion: "1.0"},
			{ID: "hist-2", NodeID: "node-1", BeaconVersion: "1.1"},
		}
		mock := &mockRegistrationsStore{getCapabilityHistoryResult: entries}
		svc := &Service{store: mock}
		got, err := svc.GetCapabilityHistory(context.Background(), "node-1", 20)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(got))
		}
		if !mock.getCapabilityHistoryCalled {
			t.Fatal("store GetCapabilityHistory not called")
		}
		m := svc.Metrics()
		if m.NodeCapabilityHistoryGetTotal != 1 {
			t.Fatalf("expected metrics count 1, got %d", m.NodeCapabilityHistoryGetTotal)
		}
	})

	t.Run("empty", func(t *testing.T) {
		mock := &mockRegistrationsStore{getCapabilityHistoryResult: []store.NodeCapabilityHistoryEntry{}}
		svc := &Service{store: mock}
		got, err := svc.GetCapabilityHistory(context.Background(), "node-1", 20)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("expected 0 entries, got %d", len(got))
		}
	})

	t.Run("store error", func(t *testing.T) {
		mock := &mockRegistrationsStore{getCapabilityHistoryErr: errors.New("db error")}
		svc := &Service{store: mock}
		_, err := svc.GetCapabilityHistory(context.Background(), "node-1", 20)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("nil service", func(t *testing.T) {
		var svc *Service
		_, err := svc.GetCapabilityHistory(context.Background(), "node-1", 20)
		if err == nil {
			t.Fatal("expected error from nil service")
		}
	})
}

func TestMetrics(t *testing.T) {
	t.Run("zero on nil service", func(t *testing.T) {
		var svc *Service
		m := svc.Metrics()
		if m.OnboardingTokensCreatedTotal != 0 {
			t.Fatal("expected zero OnboardingTokensCreatedTotal")
		}
		if m.OnboardingTokensGetTotal != 0 {
			t.Fatal("expected zero OnboardingTokensGetTotal")
		}
		if m.OnboardingTokensApprovedTotal != 0 {
			t.Fatal("expected zero OnboardingTokensApprovedTotal")
		}
		if m.OnboardingTokensRejectedTotal != 0 {
			t.Fatal("expected zero OnboardingTokensRejectedTotal")
		}
		if m.OnboardingTokensRevokedTotal != 0 {
			t.Fatal("expected zero OnboardingTokensRevokedTotal")
		}
		if m.OnboardingTokensListedTotal != 0 {
			t.Fatal("expected zero OnboardingTokensListedTotal")
		}
		if m.NodeCapabilitiesUpsertedTotal != 0 {
			t.Fatal("expected zero NodeCapabilitiesUpsertedTotal")
		}
		if m.NodeCapabilitiesGetTotal != 0 {
			t.Fatal("expected zero NodeCapabilitiesGetTotal")
		}
		if m.NodeCapabilitiesListedTotal != 0 {
			t.Fatal("expected zero NodeCapabilitiesListedTotal")
		}
		if m.NodeCapabilityHistoryGetTotal != 0 {
			t.Fatal("expected zero NodeCapabilityHistoryGetTotal")
		}
	})

	t.Run("increments after method calls", func(t *testing.T) {
		mock := &mockRegistrationsStore{
			createOnboardingTokenResult: &store.OnboardingToken{ID: "tok-1"},
			getOnboardingTokenResult:    &store.OnboardingToken{ID: "tok-1"},
			listOnboardingTokensResult:  []store.OnboardingToken{},
			getNodeCapabilityResult:     &store.NodeCapability{ID: "cap-1"},
			listCapabilitiesResult:      []store.NodeCapability{},
			getCapabilityHistoryResult:  []store.NodeCapabilityHistoryEntry{},
		}
		svc := &Service{store: mock}
		ctx := context.Background()

		svc.CreateOnboardingToken(ctx, "node-1", time.Now())
		svc.GetOnboardingToken(ctx, "tok-1")
		svc.ApproveOnboardingToken(ctx, "tok-1", "admin")
		svc.RejectOnboardingToken(ctx, "tok-2", "expired")
		svc.RevokeOnboardingToken(ctx, "tok-3", "compromised")
		svc.ListOnboardingTokens(ctx, "node-1")
		svc.UpsertNodeCapability(ctx, &store.NodeCapability{NodeID: "node-1"})
		svc.GetNodeCapability(ctx, "node-1")
		svc.ListCapabilities(ctx, store.CapabilityInventoryFilter{})
		svc.GetCapabilityHistory(ctx, "node-1", 20)

		m := svc.Metrics()
		if m.OnboardingTokensCreatedTotal != 1 {
			t.Fatalf("expected 1, got %d", m.OnboardingTokensCreatedTotal)
		}
		if m.OnboardingTokensGetTotal != 1 {
			t.Fatalf("expected 1, got %d", m.OnboardingTokensGetTotal)
		}
		if m.OnboardingTokensApprovedTotal != 1 {
			t.Fatalf("expected 1, got %d", m.OnboardingTokensApprovedTotal)
		}
		if m.OnboardingTokensRejectedTotal != 1 {
			t.Fatalf("expected 1, got %d", m.OnboardingTokensRejectedTotal)
		}
		if m.OnboardingTokensRevokedTotal != 1 {
			t.Fatalf("expected 1, got %d", m.OnboardingTokensRevokedTotal)
		}
		if m.OnboardingTokensListedTotal != 1 {
			t.Fatalf("expected 1, got %d", m.OnboardingTokensListedTotal)
		}
		if m.NodeCapabilitiesUpsertedTotal != 1 {
			t.Fatalf("expected 1, got %d", m.NodeCapabilitiesUpsertedTotal)
		}
		if m.NodeCapabilitiesGetTotal != 1 {
			t.Fatalf("expected 1, got %d", m.NodeCapabilitiesGetTotal)
		}
		if m.NodeCapabilitiesListedTotal != 1 {
			t.Fatalf("expected 1, got %d", m.NodeCapabilitiesListedTotal)
		}
		if m.NodeCapabilityHistoryGetTotal != 1 {
			t.Fatalf("expected 1, got %d", m.NodeCapabilityHistoryGetTotal)
		}
	})
}
