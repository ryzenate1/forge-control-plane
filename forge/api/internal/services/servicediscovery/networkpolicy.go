package servicediscovery

import (
	"fmt"
	"net/netip"
	"sync"
)

type NetworkAccess string

const (
	NetworkAccessPublic  NetworkAccess = "public"
	NetworkAccessPrivate NetworkAccess = "private"
	NetworkAccessIsolated NetworkAccess = "isolated"
)

type PrivateNetworkPolicy struct {
	mu           sync.RWMutex
	privateCIDRs []netip.Prefix
	allowedPorts map[string][]int
}

func NewPrivateNetworkPolicy() *PrivateNetworkPolicy {
	return &PrivateNetworkPolicy{
		privateCIDRs: []netip.Prefix{
			netip.MustParsePrefix("10.0.0.0/8"),
			netip.MustParsePrefix("172.16.0.0/12"),
			netip.MustParsePrefix("192.168.0.0/16"),
			netip.MustParsePrefix("fd00::/8"),
		},
		allowedPorts: make(map[string][]int),
	}
}

func (p *PrivateNetworkPolicy) AddPrivateCIDR(cidr string) error {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, existing := range p.privateCIDRs {
		if existing == prefix {
			return nil
		}
	}
	p.privateCIDRs = append(p.privateCIDRs, prefix)
	return nil
}

func (p *PrivateNetworkPolicy) RemovePrivateCIDR(cidr string) error {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for i, existing := range p.privateCIDRs {
		if existing == prefix {
			p.privateCIDRs = append(p.privateCIDRs[:i], p.privateCIDRs[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("CIDR %q not found", cidr)
}

func (p *PrivateNetworkPolicy) IsPrivateIP(addr netip.Addr) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, cidr := range p.privateCIDRs {
		if cidr.Contains(addr) {
			return true
		}
	}
	return false
}

func (p *PrivateNetworkPolicy) ClassifyEndpoint(ep ServiceEndpoint) NetworkAccess {
	addr := ep.Address
	if !addr.IsValid() {
		return NetworkAccessIsolated
	}

	if p.IsPrivateIP(addr) {
		return NetworkAccessPrivate
	}

	return NetworkAccessPublic
}

func (p *PrivateNetworkPolicy) AllowPort(serviceName string, port int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.allowedPorts[serviceName] = append(p.allowedPorts[serviceName], port)
}

func (p *PrivateNetworkPolicy) RevokePort(serviceName string, port int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	ports, ok := p.allowedPorts[serviceName]
	if !ok {
		return
	}

	for i, existing := range ports {
		if existing == port {
			p.allowedPorts[serviceName] = append(ports[:i], ports[i+1:]...)
			return
		}
	}
}

func (p *PrivateNetworkPolicy) IsPortAllowed(serviceName string, port int) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ports, ok := p.allowedPorts[serviceName]
	if !ok {
		return false
	}

	for _, allowed := range ports {
		if allowed == port {
			return true
		}
	}
	return false
}

func (p *PrivateNetworkPolicy) PrivateCIDRs() []netip.Prefix {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]netip.Prefix, len(p.privateCIDRs))
	copy(result, p.privateCIDRs)
	return result
}

func (p *PrivateNetworkPolicy) AllowedPorts(serviceName string) []int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ports, ok := p.allowedPorts[serviceName]
	if !ok {
		return nil
	}

	result := make([]int, len(ports))
	copy(result, ports)
	return result
}
