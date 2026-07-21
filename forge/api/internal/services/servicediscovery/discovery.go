package servicediscovery

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"sync"
)

type ServiceDiscovery struct {
	registry  *Registry
	reaper    *StaleEndpointReaper
	verifier  *ReachabilityVerifier
	policy    *PrivateNetworkPolicy
	mu        sync.RWMutex
	resolveFn func(ctx context.Context, endpoint string) ([]netip.Addr, error)
}

func NewServiceDiscovery(registry *Registry, reaper *StaleEndpointReaper, verifier *ReachabilityVerifier, policy *PrivateNetworkPolicy) *ServiceDiscovery {
	return &ServiceDiscovery{
		registry: registry,
		reaper:   reaper,
		verifier: verifier,
		policy:   policy,
		resolveFn: defaultResolve,
	}
}

func (sd *ServiceDiscovery) Start(ctx context.Context) {
	if sd.reaper != nil {
		sd.reaper.Start(ctx)
	}
}

func (sd *ServiceDiscovery) Stop() {
	if sd.reaper != nil {
		sd.reaper.Stop()
	}
}

func (sd *ServiceDiscovery) RegisterEndpoint(ctx context.Context, ep ServiceEndpoint) (*ServiceEndpoint, error) {
	if sd.policy != nil && !ep.Address.IsValid() {
		return nil, fmt.Errorf("endpoint address must be valid")
	}

	return sd.registry.RegisterEndpoint(ctx, ep)
}

func (sd *ServiceDiscovery) RemoveEndpoint(ctx context.Context, endpointID string) error {
	return sd.registry.RemoveEndpoint(ctx, endpointID)
}

func (sd *ServiceDiscovery) Resolve(serviceName string, tenantID string) ([]ServiceEndpoint, error) {
	endpoints := sd.registry.GetServiceEndpoints(serviceName, tenantID)
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("service %q not found", serviceName)
	}

	var healthy []ServiceEndpoint
	for _, ep := range endpoints {
		if ep.Status == EndpointStatusHealthy {
			healthy = append(healthy, ep)
		}
	}

	if len(healthy) == 0 {
		return endpoints, nil
	}

	sort.Slice(healthy, func(i, j int) bool {
		return healthy[i].ReplicaIndex < healthy[j].ReplicaIndex
	})

	return healthy, nil
}

func (sd *ServiceDiscovery) ResolveAll(serviceName string, tenantID string) []ServiceEndpoint {
	return sd.registry.GetServiceEndpoints(serviceName, tenantID)
}

func (sd *ServiceDiscovery) ListServices() []EndpointSet {
	return sd.registry.ListServices()
}

func (sd *ServiceDiscovery) ListEndpoints(filter EndpointFilter) []ServiceEndpoint {
	return sd.registry.ListEndpoints(filter)
}

func (sd *ServiceDiscovery) VerifyCrossNodeReachability(ctx context.Context, sourceNodeID string, targetNodeID string, serviceName string) (*ReachabilityResult, error) {
	endpoints := sd.registry.GetServiceEndpoints(serviceName, "")
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("service %q has no endpoints on target node", serviceName)
	}

	var targetEp *ServiceEndpoint
	for _, ep := range endpoints {
		if ep.NodeID == targetNodeID {
			targetEp = &ep
			break
		}
	}
	if targetEp == nil {
		return nil, fmt.Errorf("service %q has no endpoints on node %s", serviceName, targetNodeID)
	}

	address := fmt.Sprintf("%s:%d", targetEp.Address.String(), targetEp.Port)
	result := sd.verifier.VerifyCrossNode(ctx, sourceNodeID, targetNodeID, serviceName, address)
	return result, nil
}

func (sd *ServiceDiscovery) SweepReachability(ctx context.Context) []ReachabilityResult {
	return sd.verifier.Sweep(ctx)
}

func (sd *ServiceDiscovery) NetworkVisibility() NetworkVisibilityView {
	return BuildNetworkVisibility(sd.registry, sd.policy)
}

func (sd *ServiceDiscovery) NodeNetworkView(nodeID string) NodeNetworkView {
	return BuildNodeView(sd.registry, sd.verifier, nodeID)
}

func (sd *ServiceDiscovery) Registry() *Registry {
	return sd.registry
}

func (sd *ServiceDiscovery) Verifier() *ReachabilityVerifier {
	return sd.verifier
}

func (sd *ServiceDiscovery) Policy() *PrivateNetworkPolicy {
	return sd.policy
}

func (sd *ServiceDiscovery) SetResolveFn(fn func(ctx context.Context, endpoint string) ([]netip.Addr, error)) {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.resolveFn = fn
}

func (sd *ServiceDiscovery) ResolveAddress(ctx context.Context, endpoint string) ([]netip.Addr, error) {
	sd.mu.RLock()
	fn := sd.resolveFn
	sd.mu.RUnlock()
	return fn(ctx, endpoint)
}

func defaultResolve(ctx context.Context, endpoint string) ([]netip.Addr, error) {
	addr, err := netip.ParseAddr(endpoint)
	if err == nil {
		return []netip.Addr{addr}, nil
	}
	return nil, fmt.Errorf("cannot resolve endpoint %q: %w", endpoint, err)
}
