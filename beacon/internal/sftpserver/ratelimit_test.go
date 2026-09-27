package sftpserver

import (
	"fmt"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestAllowAuthenticationBurstAndSeparateIPs(t *testing.T) {
	s := &Server{}
	ip := "203.0.113.1"
	otherIP := "203.0.113.2"
	// First 3 should succeed (strict burst 3)
	for i := 0; i < 3; i++ {
		if !s.allowAuthentication(ip) {
			t.Fatalf("allow %d for %s should succeed", i+1, ip)
		}
	}
	// 4th should be denied
	if s.allowAuthentication(ip) {
		t.Fatal("4th auth within burst window should be denied")
	}
	// Different IP should still have its own burst
	for i := 0; i < 3; i++ {
		if !s.allowAuthentication(otherIP) {
			t.Fatalf("allow %d for otherIP %s should succeed", i+1, otherIP)
		}
	}
	if s.allowAuthentication(otherIP) {
		t.Fatal("otherIP 4th auth should be denied")
	}
}

func TestAllowAuthenticationRefillsOverTime(t *testing.T) {
	s := &Server{
		authLimiters: map[string]*authVisitor{
			"10.0.0.1": {limiter: rate.NewLimiter(rate.Every(100*time.Millisecond), 1), lastSeen: time.Now()},
		},
	}
	// First should succeed
	if !s.allowAuthentication("10.0.0.1") {
		t.Fatal("first should succeed")
	}
	// Immediate second should be denied (burst 1)
	if s.allowAuthentication("10.0.0.1") {
		t.Fatal("immediate second should be denied")
	}
	// After refill interval, should succeed again
	time.Sleep(150 * time.Millisecond)
	if !s.allowAuthentication("10.0.0.1") {
		t.Fatal("after refill should succeed")
	}
}

func TestAllowAuthenticationStaleEviction(t *testing.T) {
	s := &Server{authLimiters: make(map[string]*authVisitor)}
	// Insert 9998 recent entries
	for i := 0; i < 9998; i++ {
		k := fmt.Sprintf("recent-%d", i)
		s.authLimiters[k] = &authVisitor{limiter: rate.NewLimiter(rate.Every(5*time.Second), 5), lastSeen: time.Now()}
	}
	// Add 2 stale entries older than 15min
	s.authLimiters["stale-ip-1"] = &authVisitor{limiter: rate.NewLimiter(rate.Every(5*time.Second), 5), lastSeen: time.Now().Add(-16 * time.Minute)}
	s.authLimiters["stale-ip-2"] = &authVisitor{limiter: rate.NewLimiter(rate.Every(5*time.Second), 5), lastSeen: time.Now().Add(-20 * time.Minute)}
	if len(s.authLimiters) != 10000 {
		t.Fatalf("expected 10000 entries, got %d", len(s.authLimiters))
	}
	// Next allow should trigger stale cleanup and succeed
	if !s.allowAuthentication("new-ip-after-stale") {
		t.Fatal("new IP should be allowed after stale eviction")
	}
	if _, ok := s.authLimiters["stale-ip-1"]; ok {
		t.Error("stale-ip-1 should have been evicted")
	}
	if _, ok := s.authLimiters["stale-ip-2"]; ok {
		t.Error("stale-ip-2 should have been evicted")
	}
	if _, ok := s.authLimiters["new-ip-after-stale"]; !ok {
		t.Error("new IP should have been inserted")
	}
}

func TestAllowAuthenticationEvictsOldestWhenAtCapacityWithAllRecent(t *testing.T) {
	s := &Server{authLimiters: make(map[string]*authVisitor)}
	for i := 0; i < 10000; i++ {
		k := fmt.Sprintf("recent-%d", i)
		s.authLimiters[k] = &authVisitor{limiter: rate.NewLimiter(rate.Every(5*time.Second), 5), lastSeen: time.Now()}
	}
	// When at capacity with all recent entries, new IP should be denied
	if s.allowAuthentication("new-ip-at-capacity") {
		t.Fatal("new IP at capacity with all recent should be denied")
	}
	if len(s.authLimiters) != 10000 {
		t.Fatalf("expected map to stay at 10000 when denied, got %d", len(s.authLimiters))
	}
	if _, ok := s.authLimiters["new-ip-at-capacity"]; ok {
		t.Error("new IP should not have been inserted when denied")
	}
}

func TestAllowAuthenticationUpdatesLastSeen(t *testing.T) {
	s := &Server{}
	ip := "198.51.100.5"
	if !s.allowAuthentication(ip) {
		t.Fatal("first should succeed")
	}
	firstSeen := s.authLimiters[ip].lastSeen
	time.Sleep(10 * time.Millisecond)
	if !s.allowAuthentication(ip) {
		t.Fatal("second should succeed (still within burst)")
	}
	secondSeen := s.authLimiters[ip].lastSeen
	if !secondSeen.After(firstSeen) {
		t.Error("lastSeen should be updated on each allow")
	}
}
