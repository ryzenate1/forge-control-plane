package http

import (
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
)

var (
	trustedProxiesWarnOnce sync.Once
)

// parseTrustedProxies parses TRUSTED_PROXIES env var as comma-separated list of CIDRs or single IPs.
// Returns separate slices for networks and single IPs.
// Empty env returns nil slices (meaning no proxies trusted).
func parseTrustedProxies() ([]*net.IPNet, []net.IP) {
	raw := strings.TrimSpace(os.Getenv("TRUSTED_PROXIES"))
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	var nets []*net.IPNet
	var ips []net.IP
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "/") {
			_, ipNet, err := net.ParseCIDR(p)
			if err != nil {
				slog.Warn("invalid TRUSTED_PROXIES CIDR skipped", "cidr", p, "error", err)
				continue
			}
			nets = append(nets, ipNet)
		} else {
			ip := net.ParseIP(p)
			if ip == nil {
				slog.Warn("invalid TRUSTED_PROXIES IP skipped", "ip", p)
				continue
			}
			ips = append(ips, ip)
		}
	}
	return nets, ips
}

// isTrustedProxy reports whether peer IP is in the configured TRUSTED_PROXIES set.
// If TRUSTED_PROXIES is empty/not set, default deny (return false) and emit warn metric once.
func isTrustedProxy(peer net.IP) bool {
	if peer == nil {
		return false
	}
	nets, ips := parseTrustedProxies()
	if len(nets) == 0 && len(ips) == 0 {
		trustedProxiesWarnOnce.Do(func() {
			slog.Warn("TRUSTED_PROXIES not configured — proxy headers will be ignored (default deny)", "metric", "trusted_proxy_unconfigured", "peer", peer.String())
		})
		// also emit warning for observability on each call after first? Use slog.Warn with rate-limiting note.
		// We keep Once to avoid spam, but metric remains.
		return false
	}
	for _, ip := range ips {
		if ip.Equal(peer) {
			return true
		}
	}
	for _, n := range nets {
		if n.Contains(peer) {
			return true
		}
	}
	return false
}

// isTrustedProxyString is helper taking string IP.
func isTrustedProxyString(peerStr string) bool {
	peerStr = strings.TrimSpace(peerStr)
	ip := net.ParseIP(peerStr)
	if ip == nil {
		return false
	}
	return isTrustedProxy(ip)
}

// trustedProxiesConfigured reports whether any TRUSTED_PROXIES entry is configured.
func trustedProxiesConfigured() bool {
	nets, ips := parseTrustedProxies()
	return len(nets) > 0 || len(ips) > 0
}
