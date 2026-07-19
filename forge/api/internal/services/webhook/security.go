package webhook

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

type Resolver interface {
	LookupIP(context.Context, string, string) ([]net.IP, error)
}

type netResolver struct{ resolver *net.Resolver }

func (r netResolver) LookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	return r.resolver.LookupIP(ctx, network, host)
}

func ValidateURL(ctx context.Context, rawURL string) error {
	return validateURL(ctx, rawURL, netResolver{net.DefaultResolver})
}

func validateURL(ctx context.Context, rawURL string, resolver Resolver) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("webhook URL scheme must be http or https")
	}
	if u.Hostname() == "" {
		return errors.New("webhook URL host is required")
	}
	if u.User != nil {
		return errors.New("webhook URL credentials are not allowed")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		if forbiddenIP(ip) {
			return errors.New("webhook URL resolves to a non-public address")
		}
		return nil
	}
	ips, err := resolver.LookupIP(ctx, "ip", u.Hostname())
	if err != nil {
		return fmt.Errorf("resolve webhook URL: %w", err)
	}
	if len(ips) == 0 {
		return errors.New("webhook URL host has no addresses")
	}
	for _, ip := range ips {
		if forbiddenIP(ip) {
			return errors.New("webhook URL resolves to a non-public address")
		}
	}
	return nil
}

func forbiddenIP(ip net.IP) bool {
	return ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast()
}
