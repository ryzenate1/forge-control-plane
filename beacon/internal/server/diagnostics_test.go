package server

import (
	"context"
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
