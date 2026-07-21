package registrations

import (
	"context"
	"errors"
	"sync"
	"time"

	"gamepanel/forge/internal/store"
)

type Metrics struct {
	OnboardingTokensCreatedTotal  uint64
	OnboardingTokensGetTotal      uint64
	OnboardingTokensApprovedTotal uint64
	OnboardingTokensRejectedTotal uint64
	OnboardingTokensRevokedTotal  uint64
	OnboardingTokensListedTotal   uint64
	NodeCapabilitiesUpsertedTotal uint64
	NodeCapabilitiesGetTotal      uint64
	NodeCapabilitiesListedTotal   uint64
	NodeCapabilityHistoryGetTotal uint64
}

type registrationsStore interface {
	CreateOnboardingToken(ctx context.Context, nodeID string, expiresAt time.Time) (*store.OnboardingToken, error)
	GetOnboardingToken(ctx context.Context, tokenID string) (*store.OnboardingToken, error)
	ApproveOnboardingToken(ctx context.Context, tokenID, approvedBy string) error
	RejectOnboardingToken(ctx context.Context, tokenID, reason string) error
	RevokeOnboardingToken(ctx context.Context, tokenID, reason string) error
	ListOnboardingTokens(ctx context.Context, nodeID string) ([]store.OnboardingToken, error)
	UpsertNodeCapability(ctx context.Context, nc *store.NodeCapability) error
	GetNodeCapability(ctx context.Context, nodeID string) (*store.NodeCapability, error)
	ListCapabilities(ctx context.Context, filter store.CapabilityInventoryFilter) ([]store.NodeCapability, error)
	GetCapabilityHistory(ctx context.Context, nodeID string, limit int) ([]store.NodeCapabilityHistoryEntry, error)
}

type Service struct {
	store   registrationsStore
	mu      sync.Mutex
	metrics Metrics
}

func New(store *store.Store) *Service {
	return &Service{
		store: store,
	}
}

func (s *Service) Metrics() Metrics {
	if s == nil {
		return Metrics{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.metrics
}

func (s *Service) CreateOnboardingToken(ctx context.Context, nodeID string, expiresAt time.Time) (*store.OnboardingToken, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("registrations service unavailable")
	}
	token, err := s.store.CreateOnboardingToken(ctx, nodeID, expiresAt)
	if err != nil {
		return nil, err
	}
	s.increment(func(m *Metrics) { m.OnboardingTokensCreatedTotal++ })
	return token, nil
}

func (s *Service) GetOnboardingToken(ctx context.Context, tokenID string) (*store.OnboardingToken, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("registrations service unavailable")
	}
	token, err := s.store.GetOnboardingToken(ctx, tokenID)
	if err != nil {
		return nil, err
	}
	s.increment(func(m *Metrics) { m.OnboardingTokensGetTotal++ })
	return token, nil
}

func (s *Service) ApproveOnboardingToken(ctx context.Context, tokenID, approvedBy string) error {
	if s == nil || s.store == nil {
		return errors.New("registrations service unavailable")
	}
	if err := s.store.ApproveOnboardingToken(ctx, tokenID, approvedBy); err != nil {
		return err
	}
	s.increment(func(m *Metrics) { m.OnboardingTokensApprovedTotal++ })
	return nil
}

func (s *Service) RejectOnboardingToken(ctx context.Context, tokenID, reason string) error {
	if s == nil || s.store == nil {
		return errors.New("registrations service unavailable")
	}
	if err := s.store.RejectOnboardingToken(ctx, tokenID, reason); err != nil {
		return err
	}
	s.increment(func(m *Metrics) { m.OnboardingTokensRejectedTotal++ })
	return nil
}

func (s *Service) RevokeOnboardingToken(ctx context.Context, tokenID, reason string) error {
	if s == nil || s.store == nil {
		return errors.New("registrations service unavailable")
	}
	if err := s.store.RevokeOnboardingToken(ctx, tokenID, reason); err != nil {
		return err
	}
	s.increment(func(m *Metrics) { m.OnboardingTokensRevokedTotal++ })
	return nil
}

func (s *Service) ListOnboardingTokens(ctx context.Context, nodeID string) ([]store.OnboardingToken, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("registrations service unavailable")
	}
	tokens, err := s.store.ListOnboardingTokens(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	s.increment(func(m *Metrics) { m.OnboardingTokensListedTotal++ })
	return tokens, nil
}

func (s *Service) UpsertNodeCapability(ctx context.Context, nc *store.NodeCapability) error {
	if s == nil || s.store == nil {
		return errors.New("registrations service unavailable")
	}
	if err := s.store.UpsertNodeCapability(ctx, nc); err != nil {
		return err
	}
	s.increment(func(m *Metrics) { m.NodeCapabilitiesUpsertedTotal++ })
	return nil
}

func (s *Service) GetNodeCapability(ctx context.Context, nodeID string) (*store.NodeCapability, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("registrations service unavailable")
	}
	nc, err := s.store.GetNodeCapability(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	s.increment(func(m *Metrics) { m.NodeCapabilitiesGetTotal++ })
	return nc, nil
}

func (s *Service) ListCapabilities(ctx context.Context, filter store.CapabilityInventoryFilter) ([]store.NodeCapability, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("registrations service unavailable")
	}
	caps, err := s.store.ListCapabilities(ctx, filter)
	if err != nil {
		return nil, err
	}
	s.increment(func(m *Metrics) { m.NodeCapabilitiesListedTotal++ })
	return caps, nil
}

func (s *Service) GetCapabilityHistory(ctx context.Context, nodeID string, limit int) ([]store.NodeCapabilityHistoryEntry, error) {
	if s == nil || s.store == nil {
		return nil, errors.New("registrations service unavailable")
	}
	entries, err := s.store.GetCapabilityHistory(ctx, nodeID, limit)
	if err != nil {
		return nil, err
	}
	s.increment(func(m *Metrics) { m.NodeCapabilityHistoryGetTotal++ })
	return entries, nil
}

func (s *Service) increment(update func(*Metrics)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	update(&s.metrics)
}
