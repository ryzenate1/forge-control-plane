package websocketlimiter

import (
	"testing"
)

func TestConnectionManagerDefault(t *testing.T) {
	cm := NewConnectionManager(0)
	if cm.maxPerServer != 30 {
		t.Fatalf("expected default 30, got %d", cm.maxPerServer)
	}
}

func TestConnectionManagerCanConnect(t *testing.T) {
	cm := NewConnectionManager(3)
	sid := "server-1"
	if !cm.CanConnect(sid) {
		t.Fatal("should allow first connection")
	}
	cm.Connected(sid)
	cm.Connected(sid)
	cm.Connected(sid)
	if cm.CanConnect(sid) {
		t.Fatal("should deny connection at max")
	}
}

func TestConnectionManagerDisconnect(t *testing.T) {
	cm := NewConnectionManager(2)
	sid := "server-2"
	cm.Connected(sid)
	cm.Connected(sid)
	if cm.CanConnect(sid) {
		t.Fatal("should be at max")
	}
	cm.Disconnected(sid)
	if !cm.CanConnect(sid) {
		t.Fatal("should allow after disconnect")
	}
}

func TestConnectionManagerCount(t *testing.T) {
	cm := NewConnectionManager(10)
	sid := "server-3"
	if cm.Count(sid) != 0 {
		t.Fatalf("expected 0, got %d", cm.Count(sid))
	}
	cm.Connected(sid)
	cm.Connected(sid)
	if cm.Count(sid) != 2 {
		t.Fatalf("expected 2, got %d", cm.Count(sid))
	}
}

func TestConnectionManagerDisconnectBelowZero(t *testing.T) {
	cm := NewConnectionManager(10)
	sid := "server-4"
	cm.Disconnected(sid)
	if cm.Count(sid) != 0 {
		t.Fatalf("expected 0 after disconnect on empty, got %d", cm.Count(sid))
	}
}

func TestConnectionManagerCleanup(t *testing.T) {
	cm := NewConnectionManager(10)
	sid := "server-5"
	cm.Connected(sid)
	cm.Disconnected(sid)
	cm.mu.Lock()
	_, exists := cm.conns[sid]
	cm.mu.Unlock()
	if exists {
		t.Fatal("entry should be cleaned up when count reaches 0")
	}
}

func TestConnectionManagerMultipleServers(t *testing.T) {
	cm := NewConnectionManager(2)
	cm.Connected("s1")
	cm.Connected("s1")
	cm.Connected("s2")
	if cm.CanConnect("s1") {
		t.Fatal("s1 should be at max")
	}
	if !cm.CanConnect("s2") {
		t.Fatal("s2 should still allow connections")
	}
}

func TestGlobalRateLimiter(t *testing.T) {
	grl := NewGlobalRateLimiter()
	allowed := 0
	for i := 0; i < 20; i++ {
		if grl.Allow() {
			allowed++
		}
	}
	if allowed > 10 {
		t.Fatalf("expected at most 10 allowed (burst), got %d", allowed)
	}
	if allowed == 0 {
		t.Fatal("expected at least some requests to be allowed")
	}
}

func TestGlobalRateLimiterBurst(t *testing.T) {
	grl := NewGlobalRateLimiter()
	for i := 0; i < 10; i++ {
		if !grl.Allow() {
			t.Fatalf("burst request %d should be allowed", i)
		}
	}
	if grl.Allow() {
		t.Fatal("request beyond burst should be denied")
	}
}
