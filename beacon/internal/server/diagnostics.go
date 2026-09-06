package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ConnectivityDiagnostics struct {
	DNSResolution     bool   `json:"dnsResolution"`
	DNSResolutionMs   int64  `json:"dnsResolutionMs"`
	TCPConnectivity   bool   `json:"tcpConnectivity"`
	TCPConnectivityMs int64  `json:"tcpConnectivityMs"`
	TLSHandshake      bool   `json:"tlsHandshake"`
	TLSHandshakeMs    int64  `json:"tlsHandshakeMs"`
	Authentication    bool   `json:"authentication"`
	AuthenticationMsg string `json:"authenticationMsg,omitempty"`
	APIVersionMatch   bool   `json:"apiVersionMatch"`
	HeartbeatLatency  int64  `json:"heartbeatLatencyMs"`
	LastContact       string `json:"lastContact,omitempty"`
	UptimeSeconds     int64  `json:"uptimeSeconds"`
	PanelReachable    bool   `json:"panelReachable"`
	AgentConnected    bool   `json:"agentConnected"`
	EdgeState         string `json:"edgeState,omitempty"`
}

// RunConnectivityDiagnostics probes only the configured panel endpoint. The
// supplied URL must pass the same HTTPS-or-loopback policy as the panel client,
// and dialing is pinned to a validated DNS result to prevent rebinding.
func (s *Server) RunConnectivityDiagnostics(ctx context.Context, panelURL string) ConnectivityDiagnostics {
	d := ConnectivityDiagnostics{UptimeSeconds: int64(time.Since(s.started).Seconds())}
	base, err := diagnosticPanelURL(panelURL)
	if err != nil {
		d.AuthenticationMsg = "invalid panel endpoint"
		return d
	}

	dnsStart := time.Now()
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", base.Hostname())
	d.DNSResolutionMs = time.Since(dnsStart).Milliseconds()
	if err != nil || len(ips) == 0 {
		return d
	}
	allowLoopback := diagnosticLoopbackHost(base.Hostname())
	validated := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		if allowLoopback && ip.IsLoopback() || !restrictedDiagnosticIP(ip) {
			validated = append(validated, ip)
		}
	}
	if len(validated) == 0 {
		d.AuthenticationMsg = "panel endpoint resolves to a restricted address"
		return d
	}
	d.DNSResolution = true

	port := base.Port()
	if port == "" {
		port = map[bool]string{true: "443", false: "80"}[base.Scheme == "https"]
	}
	pinnedAddress := net.JoinHostPort(validated[0].String(), port)
	dialer, client := pinnedDiagnosticClient(pinnedAddress)
	defer client.CloseIdleConnections()
	tcpStart := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", pinnedAddress)
	d.TCPConnectivityMs = time.Since(tcpStart).Milliseconds()
	if err != nil {
		return d
	}
	d.TCPConnectivity = true
	_ = conn.Close()

	if base.Scheme == "https" {
		tlsStart := time.Now()
		tlsConn, tlsErr := tls.DialWithDialer(dialer, "tcp", pinnedAddress, &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: base.Hostname(),
		})
		d.TLSHandshakeMs = time.Since(tlsStart).Milliseconds()
		if tlsErr != nil {
			return d
		}
		d.TLSHandshake = true
		_ = tlsConn.Close()
	} else {
		d.TLSHandshake = true
	}

	healthURL := *base
	healthURL.Path = "/api/v1/health"
	healthURL.RawQuery = ""
	if resp, requestErr := diagnosticGET(ctx, client, healthURL.String()); requestErr == nil {
		d.PanelReachable = resp.StatusCode >= 200 && resp.StatusCode < 500
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}

	if s.panelClient != nil && s.token != "" {
		_, requestErr := s.panelClient.GetServers(ctx, 1)
		if requestErr != nil {
			d.AuthenticationMsg = "panel authentication probe failed"
			return d
		}
		d.Authentication = true
		d.AuthenticationMsg = http.StatusText(http.StatusOK)
		d.APIVersionMatch = true
	}
	return d
}

func diagnosticPanelURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("panel URL is empty")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil {
		return nil, errors.New("panel URL must include a host and no credentials")
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && diagnosticLoopbackHost(parsed.Hostname())) {
		return nil, errors.New("panel URL must use HTTPS except on loopback")
	}
	parsed.Path = strings.TrimSuffix(strings.TrimSuffix(strings.TrimRight(parsed.Path, "/"), "/api/v1"), "/api/remote")
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed, nil
}

func diagnosticLoopbackHost(host string) bool {
	if strings.EqualFold(strings.TrimSuffix(host, "."), "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func restrictedDiagnosticIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
	return cgnat.Contains(ip)
}

func diagnosticGET(ctx context.Context, client *http.Client, endpoint string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create diagnostic request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.forge.v1+json")
	return client.Do(req)
}

// pinnedDiagnosticClient returns a dialer and an HTTP client whose dialing is
// pinned to address, which must already have passed restricted-address
// validation. Every phase is capped at five seconds, redirects are never
// followed, and no proxy configuration is honored.
func pinnedDiagnosticClient(address string) (*net.Dialer, *http.Client) {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		Proxy:               nil,
		TLSHandshakeTimeout: 5 * time.Second,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, address)
		},
	}
	return dialer, &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// ProbePanelHealth performs a single credential-free probe of the configured
// panel health endpoint, applying the same HTTPS-or-loopback policy, restricted
// address rejection, dial pinning, timeouts, and response-size limits as the
// diagnostics API. It returns the HTTP status code of the probe, or 0 and an
// error when the endpoint is invalid, restricted, or unreachable.
func ProbePanelHealth(ctx context.Context, panelURL string) (int, error) {
	base, err := diagnosticPanelURL(panelURL)
	if err != nil {
		return 0, err
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", base.Hostname())
	if err != nil || len(ips) == 0 {
		return 0, fmt.Errorf("resolve panel host: %w", err)
	}
	allowLoopback := diagnosticLoopbackHost(base.Hostname())
	var pinned net.IP
	for _, ip := range ips {
		if allowLoopback && ip.IsLoopback() || !restrictedDiagnosticIP(ip) {
			pinned = ip
			break
		}
	}
	if pinned == nil {
		return 0, errors.New("panel endpoint resolves only to restricted addresses")
	}
	port := base.Port()
	if port == "" {
		port = map[bool]string{true: "443", false: "80"}[base.Scheme == "https"]
	}
	_, client := pinnedDiagnosticClient(net.JoinHostPort(pinned.String(), port))
	defer client.CloseIdleConnections()

	healthURL := *base
	healthURL.Path = "/api/v1/health"
	healthURL.RawQuery = ""
	resp, err := diagnosticGET(ctx, client, healthURL.String())
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, nil
}
