package crossnode

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/services/servicediscovery"
)

type Resolver struct {
	store        ResolutionStore
	mu           sync.RWMutex
	cache        map[string]resolutionCacheEntry
	cacheTTL     time.Duration
	discovery    *servicediscovery.Service
}

type ResolutionStore interface {
	GetServerNodeID(ctx context.Context, id string) (string, error)
	GetNodeHost(ctx context.Context, id string) (string, string, error)
}

type simpleServer struct {
	NodeID string
}

type simpleNode struct {
	PublicHostname string
	FQDN           string
}

type resolutionCacheEntry struct {
	Host      string
	ExpiresAt time.Time
}

type ResolvedBackend struct {
	ServerID string
	NodeID   string
	NodeName string
	Host     string
	Port     int
	Healthy  bool
	Reason   string
}

func NewResolver(store ResolutionStore) *Resolver {
	return &Resolver{
		store:    store,
		cache:    make(map[string]resolutionCacheEntry),
		cacheTTL: 30 * time.Second,
	}
}

func (r *Resolver) ResolveTargetHost(ctx context.Context, serverID string, nodeID string) string {
	if serverID == "" && nodeID == "" {
		return "localhost"
	}

	cacheKey := serverID + "/" + nodeID
	r.mu.RLock()
	entry, ok := r.cache[cacheKey]
	if ok && time.Now().Before(entry.ExpiresAt) {
		r.mu.RUnlock()
		return entry.Host
	}
	r.mu.RUnlock()

	host := r.resolveFromStore(ctx, serverID, nodeID)

	r.mu.Lock()
	r.cache[cacheKey] = resolutionCacheEntry{
		Host:      host,
		ExpiresAt: time.Now().Add(r.cacheTTL),
	}
	r.mu.Unlock()

	return host
}

func (r *Resolver) resolveFromStore(ctx context.Context, serverID string, nodeID string) string {
	if host := r.resolveFromDiscovery(ctx, serverID, nodeID); host != "" {
		return host
	}

	if serverID != "" && r.store != nil {
		nid, err := r.store.GetServerNodeID(ctx, serverID)
		if err == nil && nid != "" {
			nodeID = nid
		}
	}

	if nodeID != "" && r.store != nil {
		publicHostname, fqdn, err := r.store.GetNodeHost(ctx, nodeID)
		if err == nil {
			host := strings.TrimSpace(publicHostname)
			if host == "" {
				host = strings.TrimSpace(fqdn)
			}
			if host != "" {
				return host
			}
		}
	}

	return "localhost"
}

func (r *Resolver) DescribeUnreachable(host string, port int) string {
	return fmt.Sprintf("backend %s:%d unreachable — check node connectivity and container health", host, port)
}

func (r *Resolver) ClearCache() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = make(map[string]resolutionCacheEntry)
}

func (r *Resolver) SetCacheTTL(ttl time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cacheTTL = ttl
}

func (r *Resolver) SetServiceDiscovery(d *servicediscovery.Service) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.discovery = d
}

func (r *Resolver) resolveFromDiscovery(ctx context.Context, serverID string, nodeID string) string {
	if r.discovery == nil {
		return ""
	}

	if serverID != "" {
		endpoints := r.discovery.ResolveAll(ctx, serverID, "")
		for _, ep := range endpoints {
			if ep.Status == servicediscovery.EndpointStatusHealthy {
				return ep.Address.String()
			}
		}
	}

	if nodeID != "" {
		endpoints := r.discovery.ListEndpoints(ctx, servicediscovery.EndpointFilter{NodeID: nodeID, HealthyOnly: true})
		for _, ep := range endpoints {
			if ep.Address.IsValid() {
				return ep.Address.String()
			}
		}

		endpoints = r.discovery.ListEndpoints(ctx, servicediscovery.EndpointFilter{NodeID: nodeID})
		for _, ep := range endpoints {
			if ep.Address.IsValid() {
				return ep.Address.String()
			}
		}
	}

	return ""
}

func (r *Resolver) ResolveNodeAddress(ctx context.Context, nodeID string) (netip.Addr, bool) {
	if r.discovery == nil {
		return netip.Addr{}, false
	}

	endpoints := r.discovery.ListEndpoints(ctx, servicediscovery.EndpointFilter{NodeID: nodeID, HealthyOnly: true})
	for _, ep := range endpoints {
		if ep.Address.IsValid() {
			return ep.Address, true
		}
	}

	endpoints = r.discovery.ListEndpoints(ctx, servicediscovery.EndpointFilter{NodeID: nodeID})
	for _, ep := range endpoints {
		if ep.Address.IsValid() {
			return ep.Address, true
		}
	}

	return netip.Addr{}, false
}
