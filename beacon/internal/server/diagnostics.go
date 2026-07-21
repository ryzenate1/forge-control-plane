package server

import (
	"context"
	"crypto/tls"
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

func (s *Server) RunConnectivityDiagnostics(ctx context.Context, panelURL string) ConnectivityDiagnostics {
	d := ConnectivityDiagnostics{
		UptimeSeconds: int64(time.Since(beaconStart).Seconds()),
	}

	if panelURL == "" {
		return d
	}

	parsed, err := url.Parse(panelURL)
	if err != nil {
		return d
	}

	host := parsed.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	var dnsStart time.Time
	dnsStart = time.Now()
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	d.DNSResolutionMs = time.Since(dnsStart).Milliseconds()
	d.DNSResolution = err == nil && len(addrs) > 0

	if d.DNSResolution && len(addrs) > 0 {
		port := parsed.Port()
		if port == "" {
			if parsed.Scheme == "https" {
				port = "443"
			} else {
				port = "80"
			}
		}
		addr := net.JoinHostPort(addrs[0], port)
		tcpStart := time.Now()
		conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
		d.TCPConnectivityMs = time.Since(tcpStart).Milliseconds()
		if err == nil {
			d.TCPConnectivity = true
			conn.Close()

			if parsed.Scheme == "https" {
				tlsStart := time.Now()
				tlsConn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", addr, &tls.Config{
					ServerName: host,
				})
				d.TLSHandshakeMs = time.Since(tlsStart).Milliseconds()
				if err == nil {
					d.TLSHandshake = true
					tlsConn.Close()
				}
			} else {
				d.TLSHandshake = true
			}
		}
	}

	if d.DNSResolution && d.TCPConnectivity {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(panelURL, "/")+"/health", nil)
		if err == nil {
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err == nil {
				d.PanelReachable = true
				resp.Body.Close()
			}
		}
	}

	if s.panelClient != nil {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(panelURL, "/")+"/api/remote/servers?per_page=1", nil)
		if err == nil {
			req.Header.Set("Authorization", "Bearer "+s.token)
			req.Header.Set("Accept", "application/vnd.forge.v1+json")
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err == nil {
				d.Authentication = resp.StatusCode < 400
				d.AuthenticationMsg = http.StatusText(resp.StatusCode)
				if resp.StatusCode == 200 {
					d.APIVersionMatch = true
				}
				resp.Body.Close()
			} else {
				d.AuthenticationMsg = err.Error()
			}
		}
	}

	return d
}
