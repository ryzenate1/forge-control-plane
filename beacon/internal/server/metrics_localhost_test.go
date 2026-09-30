package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsAllowsLocalhostWithoutToken(t *testing.T) {
	server, h := NewServer(nil, t.TempDir(), "secret")
	server.SetMetricsToken("super-secret-metrics-token-32chars-long-xyz")
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("localhost metrics should be allowed without token, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "game_panel_daemon_uptime_seconds") {
		t.Fatalf("localhost metrics should contain daemon metrics")
	}
}

func TestMetricsRequiresTokenForRemote(t *testing.T) {
	server, h := NewServer(nil, t.TempDir(), "secret")
	server.SetMetricsToken("super-secret-metrics-token-32chars-long-xyz")
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "203.0.113.5:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("remote metrics without token should be 401, got %d", rec.Code)
	}
	// With correct token, remote should succeed
	req2 := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req2.RemoteAddr = "203.0.113.5:1234"
	req2.Header.Set("Authorization", "Bearer super-secret-metrics-token-32chars-long-xyz")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("remote metrics with correct token should be 200, got %d", rec2.Code)
	}
}

func TestMetricsIPv6LocalhostAllowed(t *testing.T) {
	server, h := NewServer(nil, t.TempDir(), "secret")
	server.SetMetricsToken("super-secret-metrics-token-32chars-long-xyz")
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.RemoteAddr = "[::1]:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ipv6 localhost metrics should be allowed, got %d", rec.Code)
	}
}
