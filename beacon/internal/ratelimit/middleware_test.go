package ratelimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewLimiterDefaults(t *testing.T) {
	l := NewLimiter(Config{})
	if l.config.RequestsPerMinute != 120 {
		t.Fatalf("expected 120 rpm, got %d", l.config.RequestsPerMinute)
	}
	if l.config.BurstSize != 20 {
		t.Fatalf("expected burst 20, got %d", l.config.BurstSize)
	}
	if l.config.CleanupInterval != time.Minute {
		t.Fatalf("expected 1m cleanup, got %s", l.config.CleanupInterval)
	}
}

func TestLimiterAllow(t *testing.T) {
	l := NewLimiter(Config{RequestsPerMinute: 60, BurstSize: 5})
	ip := "1.2.3.4"
	for i := 0; i < 5; i++ {
		if !l.Allow(ip) {
			t.Fatalf("request %d should be allowed within burst", i)
		}
	}
	if l.Allow(ip) {
		t.Fatal("request should be denied after burst exhausted")
	}
}

func TestLimiterPerIP(t *testing.T) {
	l := NewLimiter(Config{RequestsPerMinute: 60, BurstSize: 2})
	if !l.Allow("10.0.0.1") {
		t.Fatal("first request from IP1 should be allowed")
	}
	if !l.Allow("10.0.0.1") {
		t.Fatal("second request from IP1 should be allowed")
	}
	if l.Allow("10.0.0.1") {
		t.Fatal("third request from IP1 should be denied")
	}
	if !l.Allow("10.0.0.2") {
		t.Fatal("first request from IP2 should be allowed")
	}
}

func TestLimiterCleanup(t *testing.T) {
	l := NewLimiter(Config{
		RequestsPerMinute: 60,
		BurstSize:         2,
		CleanupInterval:   10 * time.Millisecond,
	})
	l.Allow("1.2.3.4")
	l.mu.RLock()
	if _, ok := l.visitors["1.2.3.4"]; !ok {
		l.mu.RUnlock()
		t.Fatal("visitor should exist")
	}
	l.mu.RUnlock()

	l.visitors["1.2.3.4"].lastSeen = time.Now().Add(-1 * time.Hour)
	l.cleanup()

	l.mu.RLock()
	_, ok := l.visitors["1.2.3.4"]
	l.mu.RUnlock()
	if ok {
		t.Fatal("stale visitor should have been cleaned up")
	}
}

func TestLimiterStartCleanup(t *testing.T) {
	l := NewLimiter(Config{
		RequestsPerMinute: 60,
		BurstSize:         2,
		CleanupInterval:   20 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l.Start(ctx)

	l.Allow("5.5.5.5")
	l.mu.Lock()
	l.visitors["5.5.5.5"].lastSeen = time.Now().Add(-1 * time.Hour)
	l.mu.Unlock()

	time.Sleep(60 * time.Millisecond)
	l.mu.RLock()
	_, ok := l.visitors["5.5.5.5"]
	l.mu.RUnlock()
	if ok {
		t.Fatal("background cleanup should have removed stale visitor")
	}
}

func TestMiddlewareAllowsRequest(t *testing.T) {
	l := NewLimiter(Config{RequestsPerMinute: 120, BurstSize: 10})
	handler := Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestMiddlewareBlocksExcess(t *testing.T) {
	l := NewLimiter(Config{RequestsPerMinute: 60, BurstSize: 2})
	handler := Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "9.9.9.9:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rr.Code)
		}
	}
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "9.9.9.9:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
}

func TestMiddlewareXForwardedFor(t *testing.T) {
	l := NewLimiter(Config{RequestsPerMinute: 60, BurstSize: 1})
	handler := Middleware(l)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")
	req.RemoteAddr = "1.2.3.4:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("X-Forwarded-For", "10.0.0.1")
	req2.RemoteAddr = "5.6.7.8:5678"
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("same forwarded IP should be rate limited, got %d", rr2.Code)
	}
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(r *http.Request)
		expect string
	}{
		{
			name: "X-Forwarded-For single",
			setup: func(r *http.Request) {
				r.Header.Set("X-Forwarded-For", "10.0.0.1")
			},
			expect: "10.0.0.1",
		},
		{
			name: "X-Forwarded-For multiple",
			setup: func(r *http.Request) {
				r.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")
			},
			expect: "10.0.0.1",
		},
		{
			name: "X-Real-IP",
			setup: func(r *http.Request) {
				r.Header.Set("X-Real-IP", "192.168.1.1")
			},
			expect: "192.168.1.1",
		},
		{
			name: "RemoteAddr",
			setup: func(r *http.Request) {
				r.RemoteAddr = "172.16.0.1:9999"
			},
			expect: "172.16.0.1",
		},
		{
			name: "RemoteAddr no port",
			setup: func(r *http.Request) {
				r.RemoteAddr = "172.16.0.1"
			},
			expect: "172.16.0.1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = ""
			tt.setup(req)
			got := ExtractIP(req)
			if got != tt.expect {
				t.Fatalf("expected %q, got %q", tt.expect, got)
			}
		})
	}
}
