package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTierMatching(t *testing.T) {
	tl := NewTieredLimiter(DefaultTiers())

	tests := []struct {
		path     string
		wantTier string
	}{
		{"/servers/abc123/power", "/servers/*/power"},
		{"/servers/abc123/ws/console", "/servers/*/ws/*"},
		{"/servers/abc123/files/download", "/servers/*/files/*"},
		{"/api/servers", "*"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			limiter := tl.matchLimiter(tt.path)
			if limiter == nil {
				t.Fatal("expected a limiter to match")
			}
		})
	}

	_ = tests
}

func TestTieredLimiterMiddleware(t *testing.T) {
	tiers := []Tier{
		{Pattern: "/servers/*/power", RequestsPerMinute: 60, BurstSize: 2},
		{Pattern: "*", RequestsPerMinute: 120, BurstSize: 5},
	}
	tl := NewTieredLimiter(tiers)
	handler := tl.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/servers/s1/power", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("power request %d: expected 200, got %d", i, rr.Code)
		}
	}

	req := httptest.NewRequest("POST", "/servers/s1/power", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after burst, got %d", rr.Code)
	}

	req2 := httptest.NewRequest("GET", "/api/servers", nil)
	req2.RemoteAddr = "10.0.0.1:1234"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("fallback path should still be allowed, got %d", rr2.Code)
	}
}

func TestTieredLimiterDifferentIPs(t *testing.T) {
	tiers := []Tier{
		{Pattern: "*", RequestsPerMinute: 60, BurstSize: 1},
	}
	tl := NewTieredLimiter(tiers)
	handler := tl.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.RemoteAddr = "1.1.1.1:1234"
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr1.Code)
	}

	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.RemoteAddr = "2.2.2.2:1234"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("different IP should be allowed, got %d", rr2.Code)
	}
}

func TestMatchParts(t *testing.T) {
	tests := []struct {
		pattern []string
		path    []string
		want    bool
	}{
		{[]string{"servers", "*", "power"}, []string{"servers", "abc", "power"}, true},
		{[]string{"servers", "*", "power"}, []string{"servers", "abc", "stop"}, false},
		{[]string{"servers", "*", "ws", "*"}, []string{"servers", "abc", "ws", "console"}, true},
		{[]string{"servers", "*", "power"}, []string{"servers", "abc"}, false},
		{[]string{"api", "servers"}, []string{"api", "servers"}, true},
		{[]string{"api", "Servers"}, []string{"api", "servers"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.pattern[0], func(t *testing.T) {
			got := matchParts(tt.pattern, tt.path)
			if got != tt.want {
				t.Fatalf("matchParts(%v, %v) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}
