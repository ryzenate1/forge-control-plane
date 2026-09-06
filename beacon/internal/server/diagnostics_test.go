package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestConnectivityDiagnosticsEmptyURL(t *testing.T) {
	s := &Server{}
	d := s.RunConnectivityDiagnostics(context.Background(), "")
	if d.UptimeSeconds < 0 {
		t.Fatal("expected uptime >= 0")
	}
	if d.DNSResolution {
		t.Fatal("expected no DNS resolution for empty URL")
	}
}

func TestConnectivityDiagnosticsInvalidURL(t *testing.T) {
	s := &Server{}
	d := s.RunConnectivityDiagnostics(context.Background(), "://invalid")
	if d.DNSResolution {
		t.Fatal("expected no DNS resolution for invalid URL")
	}
}

func TestConnectivityDiagnosticsBadHost(t *testing.T) {
	s := &Server{}
	d := s.RunConnectivityDiagnostics(context.Background(), "http://nonexistent.example.invalid:9999")
	if d.DNSResolution {
		t.Log("unexpected DNS resolution for invalid host")
	}
	if d.TCPConnectivity {
		t.Log("unexpected TCP connectivity for invalid host")
	}
}

func TestDiagnosticPanelURLRejectsUntrustedTargets(t *testing.T) {
	for _, raw := range []string{
		"http://attacker.example",
		"file:///etc/passwd",
		"https://user:secret@example.com",
	} {
		if _, err := diagnosticPanelURL(raw); err == nil {
			t.Errorf("diagnosticPanelURL(%q) unexpectedly succeeded", raw)
		}
	}
	if _, err := diagnosticPanelURL("http://127.0.0.1:8080/api/v1"); err != nil {
		t.Fatalf("loopback development endpoint rejected: %v", err)
	}
	if !restrictedDiagnosticIP(net.ParseIP("169.254.169.254")) || !restrictedDiagnosticIP(net.ParseIP("100.64.0.1")) {
		t.Fatal("metadata and CGNAT addresses must be restricted")
	}
}

func TestRunConnectivityDiagnosticsDoesNotForwardCredentials(t *testing.T) {
	var mu sync.Mutex
	var received []http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		received = append(received, r.Header.Clone())
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := &Server{token: "super-secret-node-token-0123456789"}
	d := s.RunConnectivityDiagnostics(context.Background(), srv.URL)
	if !d.PanelReachable {
		t.Fatalf("expected loopback probe to succeed, got %+v", d)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(received) == 0 {
		t.Fatal("expected the probe target to receive a request")
	}
	for _, h := range received {
		for _, name := range []string{"Authorization", "X-Forge-Token", "Cookie"} {
			if v := h.Get(name); v != "" {
				t.Errorf("credential header %s forwarded to probe target: %q", name, v)
			}
		}
		for name := range h {
			if strings.HasPrefix(strings.ToLower(name), "x-forge") {
				t.Errorf("forged header %s forwarded to probe target", name)
			}
		}
	}
}

func TestProbePanelHealthLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/health" {
			t.Errorf("unexpected probe path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	status, err := ProbePanelHealth(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}
}

func TestProbePanelHealthRejectsRestrictedTargets(t *testing.T) {
	for _, raw := range []string{
		"https://169.254.169.254/latest/meta-data",
		"http://169.254.169.254/latest/meta-data",
		"http://10.0.0.5/health",
		"http://[fe80::1]/health",
		"http://0.0.0.0/health",
		"ftp://example.com/health",
		"https://user:pass@example.com/health",
	} {
		if _, err := ProbePanelHealth(context.Background(), raw); err == nil {
			t.Errorf("ProbePanelHealth(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestVersionInventory(t *testing.T) {
	inv := VersionInventory{
		BeaconVersion: "1.0.0-test",
		GoVersion:     "go1.26",
		OS:            "linux",
		Architecture:  "amd64",
		Capabilities:  []string{"docker", "edge-agent"},
		UptimeSeconds: 42,
		EdgeState:     "connected",
	}
	if inv.BeaconVersion != "1.0.0-test" {
		t.Fatalf("expected version 1.0.0-test, got %s", inv.BeaconVersion)
	}
	if len(inv.Capabilities) != 2 {
		t.Fatalf("expected 2 capabilities, got %d", len(inv.Capabilities))
	}
}

func TestEdgeStatusResponse(t *testing.T) {
	data := map[string]any{"state": "connected", "service": "edge-agent"}
	if data["state"] != "connected" {
		t.Fatal("expected connected state")
	}
}

func TestVersionCompatibility(t *testing.T) {
	tests := []struct {
		beacon   string
		api      string
		expected bool
	}{
		{"", "", false},
		{"1.0.0", "", true},
		{"1.0.0", "1.0.0", true},
		{"2.0.0", "", true},
	}
	for _, tt := range tests {
		result := CheckVersionCompatibility(tt.beacon, tt.api)
		if result.Compatible != tt.expected {
			t.Errorf("CheckVersionCompatibility(%q, %q) = %v, want %v", tt.beacon, tt.api, result.Compatible, tt.expected)
		}
	}
}
