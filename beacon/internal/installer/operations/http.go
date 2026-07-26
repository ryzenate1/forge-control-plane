package operations

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxInstallerRedirects = 5

// SecureHTTPClient returns an HTTP client that rejects non-HTTP schemes,
// credentials in URLs, redirects to restricted hosts, and connections to
// loopback/private/link-local/multicast addresses. Dialing is pinned to an IP
// that was validated immediately before the connection to prevent DNS rebinding.
func SecureHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ForceAttemptHTTP2:     true,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("invalid download address: %w", err)
			}
			ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("resolve download host: %w", err)
			}
			for _, ip := range ips {
				if restrictedDownloadIP(ip) {
					continue
				}
				conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if dialErr == nil {
					return conn, nil
				}
				err = dialErr
			}
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("download host resolves only to restricted addresses")
		},
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxInstallerRedirects {
				return fmt.Errorf("too many redirects")
			}
			return ValidateDownloadURL(req.URL)
		},
	}
}

func ValidateDownloadURL(parsed *url.URL) error {
	if parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return fmt.Errorf("download URL must use http or https")
	}
	if parsed.User != nil {
		return fmt.Errorf("download URL must not contain credentials")
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" || host == "metadata.google.internal" {
		return fmt.Errorf("download URL targets a restricted host")
	}
	if ip := net.ParseIP(host); ip != nil && restrictedDownloadIP(ip) {
		return fmt.Errorf("download URL targets a restricted address")
	}
	return nil
}

func restrictedDownloadIP(ip net.IP) bool {
	return ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}
