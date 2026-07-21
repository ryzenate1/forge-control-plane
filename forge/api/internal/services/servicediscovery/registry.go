package servicediscovery

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"sync"
	"time"

	"gamepanel/forge/internal/events"

	"github.com/google/uuid"
)

type nodeStore interface {
	ListNodes(ctx context.Context) ([]storeNode, error)
	GetNode(ctx context.Context, nodeID string) (storeNode, error)
}

type storeNode interface {
	GetID() string
	GetName() string
	GetFQDN() string
	GetPublicHostname() string
	GetAllowedIPs() []string
	GetStatus() string
	GetActualState() string
	GetRegionID() string
}

type Registry struct {
	mu            sync.RWMutex
	endpoints     map[string]*ServiceEndpoint
	services      map[string]*EndpointSet
	publisher     events.Publisher
	store         nodeStore
	endpointStore EndpointStore
	now           func() time.Time
}

func NewRegistry(store nodeStore, endpointStore EndpointStore, publishers ...events.Publisher) *Registry {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Registry{
		endpoints:     make(map[string]*ServiceEndpoint),
		services:      make(map[string]*EndpointSet),
		publisher:     publisher,
		store:         store,
		endpointStore: endpointStore,
		now:           time.Now,
	}
}

func serviceKey(serviceName, tenantID string) string {
	if tenantID == "" {
		return serviceName
	}
	return tenantID + "/" + serviceName
}

func (r *Registry) RegisterEndpoint(ctx context.Context, ep ServiceEndpoint) (*ServiceEndpoint, error) {
	if ep.ServiceName == "" {
		return nil, fmt.Errorf("serviceName is required")
	}
	if ep.NodeID == "" {
		return nil, fmt.Errorf("nodeID is required")
	}
	if !ep.Address.IsValid() {
		return nil, fmt.Errorf("valid address is required")
	}
	if ep.Port < 1 || ep.Port > 65535 {
		return nil, fmt.Errorf("port must be between 1 and 65535")
	}
	if ep.ID == "" {
		ep.ID = uuid.NewString()
	}
	if ep.Status == "" {
		ep.Status = EndpointStatusUnknown
	}
	if ep.Protocol == "" {
		ep.Protocol = ProtocolTCP
	}
	now := r.now()
	if ep.CreatedAt.IsZero() {
		ep.CreatedAt = now
	}
	ep.UpdatedAt = now
	ep.LastHeartbeat = now

	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.endpoints[ep.ID]
	if exists {
		ep.CreatedAt = existing.CreatedAt
	}

	r.endpoints[ep.ID] = &ep
	r.rebuildServiceSet(ep.ServiceName, ep.TenantID)

	if r.endpointStore != nil {
		if err := r.endpointStore.SaveEndpoint(ctx, ep); err != nil {
			return nil, fmt.Errorf("persist endpoint: %w", err)
		}
	}

	if r.publisher != nil {
		_ = r.publisher.Publish(ctx, events.NewEnvelope("endpoint_registered", "service-discovery", "endpoint", ep.ID, map[string]any{
			"serviceName": ep.ServiceName,
			"nodeId":      ep.NodeID,
			"address":     ep.Address.String(),
			"port":        ep.Port,
		}))
	}

	return &ep, nil
}

func (r *Registry) RemoveEndpoint(ctx context.Context, endpointID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ep, exists := r.endpoints[endpointID]
	if !exists {
		return fmt.Errorf("endpoint not found: %s", endpointID)
	}

	serviceName := ep.ServiceName
	tenantID := ep.TenantID
	delete(r.endpoints, endpointID)
	r.rebuildServiceSet(serviceName, tenantID)

	if r.endpointStore != nil {
		if err := r.endpointStore.DeleteEndpoint(ctx, endpointID); err != nil {
			return fmt.Errorf("delete persisted endpoint: %w", err)
		}
	}

	if r.publisher != nil {
		_ = r.publisher.Publish(ctx, events.NewEnvelope("endpoint_removed", "service-discovery", "endpoint", endpointID, map[string]any{
			"serviceName": serviceName,
			"nodeId":      ep.NodeID,
		}))
	}

	return nil
}

func (r *Registry) GetEndpoint(endpointID string) (*ServiceEndpoint, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ep, ok := r.endpoints[endpointID]
	if !ok {
		return nil, false
	}
	cp := *ep
	return &cp, true
}

func (r *Registry) ListEndpoints(filter EndpointFilter) []ServiceEndpoint {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []ServiceEndpoint
	for _, ep := range r.endpoints {
		if !matchFilter(ep, filter) {
			continue
		}
		result = append(result, *ep)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].ServiceName != result[j].ServiceName {
			return result[i].ServiceName < result[j].ServiceName
		}
		return result[i].NodeID < result[j].NodeID
	})

	return result
}

func (r *Registry) GetServiceEndpoints(serviceName string, tenantID string) []ServiceEndpoint {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := serviceKey(serviceName, tenantID)
	set, ok := r.services[key]
	if !ok {
		return nil
	}

	result := make([]ServiceEndpoint, len(set.Endpoints))
	copy(result, set.Endpoints)
	return result
}

func (r *Registry) ListServices() []EndpointSet {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]EndpointSet, 0, len(r.services))
	for _, set := range r.services {
		result = append(result, *set)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ServiceName < result[j].ServiceName
	})

	return result
}

func (r *Registry) UpdateEndpointStatus(ctx context.Context, endpointID string, status EndpointStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ep, exists := r.endpoints[endpointID]
	if !exists {
		return fmt.Errorf("endpoint not found: %s", endpointID)
	}

	ep.Status = status
	ep.UpdatedAt = r.now()

	if r.endpointStore != nil {
		if err := r.endpointStore.SaveEndpoint(ctx, *ep); err != nil {
			return fmt.Errorf("persist endpoint status: %w", err)
		}
	}

	if r.publisher != nil {
		_ = r.publisher.Publish(ctx, events.NewEnvelope("endpoint_status_changed", "service-discovery", "endpoint", endpointID, map[string]any{
			"serviceName": ep.ServiceName,
			"status":      string(status),
		}))
	}

	return nil
}

func (r *Registry) TouchHeartbeat(ctx context.Context, endpointID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ep, exists := r.endpoints[endpointID]
	if !exists {
		return fmt.Errorf("endpoint not found: %s", endpointID)
	}

	ep.LastHeartbeat = r.now()
	ep.UpdatedAt = r.now()

	if r.endpointStore != nil {
		if err := r.endpointStore.SaveEndpoint(ctx, *ep); err != nil {
			return fmt.Errorf("persist heartbeat: %w", err)
		}
	}

	return nil
}

func (r *Registry) LoadFromStore(ctx context.Context) error {
	if r.endpointStore == nil {
		return nil
	}
	endpoints, err := r.endpointStore.ListEndpoints(ctx)
	if err != nil {
		return fmt.Errorf("load endpoints from store: %w", err)
	}
	if len(endpoints) > 0 {
		r.RebuildFromEndpoints(endpoints)
	}
	return nil
}

func (r *Registry) RebuildFromEndpoints(endpoints []ServiceEndpoint) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.endpoints = make(map[string]*ServiceEndpoint, len(endpoints))
	r.services = make(map[string]*EndpointSet)

	for i := range endpoints {
		ep := endpoints[i]
		r.endpoints[ep.ID] = &ep
	}

	for _, ep := range r.endpoints {
		r.rebuildServiceSet(ep.ServiceName, ep.TenantID)
	}
}

func (r *Registry) rebuildServiceSet(serviceName, tenantID string) {
	key := serviceKey(serviceName, tenantID)
	var endpoints []ServiceEndpoint
	for _, ep := range r.endpoints {
		if ep.ServiceName == serviceName && ep.TenantID == tenantID {
			endpoints = append(endpoints, *ep)
		}
	}

	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].ReplicaIndex < endpoints[j].ReplicaIndex
	})

	if len(endpoints) == 0 {
		delete(r.services, key)
		return
	}

	r.services[key] = &EndpointSet{
		ServiceName: serviceName,
		ServiceID:   endpoints[0].ServiceID,
		Endpoints:   endpoints,
		TenantID:    tenantID,
	}
}

func (r *Registry) EndpointCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.endpoints)
}

func (r *Registry) ServiceCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.services)
}

func matchFilter(ep *ServiceEndpoint, filter EndpointFilter) bool {
	if filter.ServiceName != "" && ep.ServiceName != filter.ServiceName {
		return false
	}
	if filter.ServiceID != "" && ep.ServiceID != filter.ServiceID {
		return false
	}
	if filter.NodeID != "" && ep.NodeID != filter.NodeID {
		return false
	}
	if filter.TenantID != "" && ep.TenantID != filter.TenantID {
		return false
	}
	if filter.Status != "" && ep.Status != filter.Status {
		return false
	}
	if filter.HealthyOnly && ep.Status != EndpointStatusHealthy {
		return false
	}
	return true
}

func SelectNodeAddress(node interface {
	GetFQDN() string
	GetPublicHostname() string
	GetAllowedIPs() []string
}) netip.Addr {
	if ips := node.GetAllowedIPs(); len(ips) > 0 {
		for _, ipStr := range ips {
			if addr, err := netip.ParseAddr(ipStr); err == nil && addr.IsValid() {
				return addr
			}
		}
	}

	hostname := node.GetFQDN()
	if hostname == "" {
		hostname = node.GetPublicHostname()
	}

	if hostname != "" {
		if addr, err := netip.ParseAddr(hostname); err == nil {
			return addr
		}
	}

	return netip.Addr{}
}
