package servicediscovery

import (
	"context"
	"log/slog"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"
)

type Service struct {
	discovery     *ServiceDiscovery
	registry      *Registry
	reaper        *StaleEndpointReaper
	verifier      *ReachabilityVerifier
	policy        *PrivateNetworkPolicy
	store         *store.Store
	endpointStore EndpointStore
	publisher     events.Publisher
	adapter       NetworkAdapter
}

func New(store *store.Store, endpointStore EndpointStore, publishers ...events.Publisher) *Service {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}

	registry := NewRegistry(adapterStore{store: store}, endpointStore, publisher)
	reaper := NewStaleEndpointReaper(registry)
	verifier := NewReachabilityVerifier(registry)
	policy := NewPrivateNetworkPolicy()
	discovery := NewServiceDiscovery(registry, reaper, verifier, policy)

	return &Service{
		discovery:     discovery,
		registry:      registry,
		reaper:        reaper,
		verifier:      verifier,
		policy:        policy,
		store:         store,
		endpointStore: endpointStore,
		publisher:     publisher,
	}
}

func (s *Service) Start(ctx context.Context) {
	if s.endpointStore != nil {
		if err := s.registry.LoadFromStore(ctx); err != nil {
			slog.Error("Failed to load persisted endpoints", "error", err)
		}
	}
	s.discovery.Start(ctx)
	slog.Info("Service discovery started")
}

func (s *Service) Stop() {
	s.discovery.Stop()
	slog.Info("Service discovery stopped")
}

func (s *Service) SetNetworkAdapter(adapter NetworkAdapter) {
	s.adapter = adapter
}

func (s *Service) Adapter() NetworkAdapter {
	return s.adapter
}

func (s *Service) RegisterEndpoint(ctx context.Context, ep ServiceEndpoint) (*ServiceEndpoint, error) {
	return s.discovery.RegisterEndpoint(ctx, ep)
}

func (s *Service) RemoveEndpoint(ctx context.Context, endpointID string) error {
	return s.discovery.RemoveEndpoint(ctx, endpointID)
}

func (s *Service) Resolve(ctx context.Context, serviceName string, tenantID string) ([]ServiceEndpoint, error) {
	return s.discovery.Resolve(serviceName, tenantID)
}

func (s *Service) ResolveAll(ctx context.Context, serviceName string, tenantID string) []ServiceEndpoint {
	return s.discovery.ResolveAll(serviceName, tenantID)
}

func (s *Service) ListServices(ctx context.Context) []EndpointSet {
	return s.discovery.ListServices()
}

func (s *Service) ListEndpoints(ctx context.Context, filter EndpointFilter) []ServiceEndpoint {
	return s.discovery.ListEndpoints(filter)
}

func (s *Service) GetEndpoint(ctx context.Context, endpointID string) (*ServiceEndpoint, error) {
	ep, ok := s.registry.GetEndpoint(endpointID)
	if !ok {
		return nil, ErrEndpointNotFound
	}
	return ep, nil
}

func (s *Service) UpdateEndpointStatus(ctx context.Context, endpointID string, status EndpointStatus) error {
	return s.registry.UpdateEndpointStatus(ctx, endpointID, status)
}

func (s *Service) VerifyCrossNodeReachability(ctx context.Context, sourceNodeID, targetNodeID, serviceName string) (*ReachabilityResult, error) {
	return s.discovery.VerifyCrossNodeReachability(ctx, sourceNodeID, targetNodeID, serviceName)
}

func (s *Service) SweepReachability(ctx context.Context) []ReachabilityResult {
	return s.discovery.SweepReachability(ctx)
}

func (s *Service) NetworkVisibility(ctx context.Context) NetworkVisibilityView {
	return s.discovery.NetworkVisibility()
}

func (s *Service) NodeNetworkView(ctx context.Context, nodeID string) NodeNetworkView {
	return s.discovery.NodeNetworkView(nodeID)
}

func (s *Service) ReaperStats() (time.Time, int, time.Duration) {
	return s.reaper.Stats()
}

var ErrEndpointNotFound = buildErr("endpoint not found")

func buildErr(msg string) error {
	return &serviceError{msg: msg}
}

type serviceError struct {
	msg string
}

func (e *serviceError) Error() string { return e.msg }

type adapterStore struct {
	store *store.Store
}

func (a adapterStore) ListNodes(ctx context.Context) ([]storeNode, error) {
	nodes, err := a.store.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]storeNode, len(nodes))
	for i := range nodes {
		result[i] = nodeAdapter{node: nodes[i]}
	}
	return result, nil
}

func (a adapterStore) GetNode(ctx context.Context, nodeID string) (storeNode, error) {
	node, err := a.store.GetNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	return nodeAdapter{node: node}, nil
}

type nodeAdapter struct {
	node store.Node
}

func (n nodeAdapter) GetID() string              { return n.node.ID }
func (n nodeAdapter) GetName() string             { return n.node.Name }
func (n nodeAdapter) GetFQDN() string             { return n.node.FQDN }
func (n nodeAdapter) GetPublicHostname() string   { return n.node.PublicHostname }
func (n nodeAdapter) GetAllowedIPs() []string     { return n.node.AllowedIPs }
func (n nodeAdapter) GetStatus() string           { return n.node.Status }
func (n nodeAdapter) GetActualState() string      { return n.node.ActualState }
func (n nodeAdapter) GetRegionID() string         { if n.node.RegionID != nil { return *n.node.RegionID }; return "" }
