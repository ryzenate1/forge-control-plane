package crossnode

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/services/servicediscovery"
)

// ErrNoTarget is returned when no reachable host could be resolved for the
// requested server/node. Callers must surface it, never substitute a default.
var ErrNoTarget = errors.New("crossnode: no reachable target")

type Resolver struct {
	store     ResolutionStore
	mu        sync.RWMutex
	cache     map[string]resolutionCacheEntry
	cacheTTL  time.Duration
	discovery *servicediscovery.Service
}

type ResolutionStore interface {
	GetServerNodeID(ctx context.Context, id string) (string, error)
	GetNodeHost(ctx context.Context, id string) (string, string, error)
}

type resolutionCacheEntry struct {
	Host      string
	ExpiresAt time.Time
}

func NewResolver(store ResolutionStore) *Resolver {
	return &Resolver{
		store:    store,
		cache:    make(map[string]resolutionCacheEntry),
		cacheTTL: 30 * time.Second,
	}
}

// ResolveTargetHost resolves the reachable host for a server or a node.
// It returns ErrNoTarget (with an empty host) instead of falling back: a
// guessed host would proxy cross-node traffic to the wrong machine.
func (r *Resolver) ResolveTargetHost(ctx context.Context, serverID string, nodeID string) (string, error) {
	if serverID == "" && nodeID == "" {
		return "", ErrNoTarget
	}

	cacheKey := serverID + "/" + nodeID
	r.mu.RLock()
	entry, ok := r.cache[cacheKey]
	r.mu.RUnlock()
	if ok && time.Now().Before(entry.ExpiresAt) {
		return entry.Host, nil
	}

	host, err := r.resolveFromStore(ctx, serverID, nodeID)
	if err != nil {
		return "", err
	}

	r.mu.Lock()
	r.cache[cacheKey] = resolutionCacheEntry{
		Host:      host,
		ExpiresAt: time.Now().Add(r.cacheTTL),
	}
	r.mu.Unlock()

	return host, nil
}

func (r *Resolver) resolveFromStore(ctx context.Context, serverID string, nodeID string) (string, error) {
	if host := r.resolveFromDiscovery(ctx, r.serviceDiscovery(), serverID, nodeID); host != "" {
		return host, nil
	}

	if r.store == nil {
		return "", fmt.Errorf("%w: no resolution store for server %q node %q", ErrNoTarget, serverID, nodeID)
	}

	if serverID != "" {
		nid, err := r.store.GetServerNodeID(ctx, serverID)
		if err != nil {
			return "", fmt.Errorf("crossnode: lookup node for server %q: %w", serverID, err)
		}
		if nid != "" {
			nodeID = nid
		}
	}

	if nodeID == "" {
		return "", fmt.Errorf("%w: server %q is not bound to a node", ErrNoTarget, serverID)
	}

	publicHostname, fqdn, err := r.store.GetNodeHost(ctx, nodeID)
	if err != nil {
		return "", fmt.Errorf("crossnode: lookup host for node %q: %w", nodeID, err)
	}

	host := strings.TrimSpace(publicHostname)
	if host == "" {
		host = strings.TrimSpace(fqdn)
	}
	if host == "" {
		return "", fmt.Errorf("%w: node %q has no public hostname or FQDN", ErrNoTarget, nodeID)
	}

	return host, nil
}

// DescribeUnreachable reports what was observed, not what caused it: the
// resolver has no probe data, so the listed causes are suggestions only.
func (r *Resolver) DescribeUnreachable(host string, port int) string {
	return fmt.Sprintf("backend %s:%d was reported unreachable; node connectivity and container health are suggested causes, not a verified diagnosis", host, port)
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

// serviceDiscovery snapshots the field: main.go installs it at startup while
// handlers resolve concurrently, so a lock-free read would race.
func (r *Resolver) serviceDiscovery() *servicediscovery.Service {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.discovery
}

func (r *Resolver) resolveFromDiscovery(ctx context.Context, d *servicediscovery.Service, serverID string, nodeID string) string {
	if d == nil {
		return ""
	}

	if serverID != "" {
		endpoints := d.ResolveAll(ctx, serverID, "")
		for _, ep := range endpoints {
			if ep.Status == servicediscovery.EndpointStatusHealthy {
				return ep.Address.String()
			}
		}
	}

	if nodeID != "" {
		endpoints := d.ListEndpoints(ctx, servicediscovery.EndpointFilter{NodeID: nodeID, HealthyOnly: true})
		for _, ep := range endpoints {
			if ep.Address.IsValid() {
				return ep.Address.String()
			}
		}

		endpoints = d.ListEndpoints(ctx, servicediscovery.EndpointFilter{NodeID: nodeID})
		for _, ep := range endpoints {
			if ep.Address.IsValid() {
				return ep.Address.String()
			}
		}
	}

	return ""
}

func (r *Resolver) ResolveNodeAddress(ctx context.Context, nodeID string) (netip.Addr, bool) {
	d := r.serviceDiscovery()
	if d == nil || nodeID == "" {
		return netip.Addr{}, false
	}

	endpoints := d.ListEndpoints(ctx, servicediscovery.EndpointFilter{NodeID: nodeID, HealthyOnly: true})
	for _, ep := range endpoints {
		if ep.Address.IsValid() {
			return ep.Address, true
		}
	}

	endpoints = d.ListEndpoints(ctx, servicediscovery.EndpointFilter{NodeID: nodeID})
	for _, ep := range endpoints {
		if ep.Address.IsValid() {
			return ep.Address, true
		}
	}

	return netip.Addr{}, false
}
