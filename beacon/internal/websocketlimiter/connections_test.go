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

func TestConnectionManagerAcquire(t *testing.T) {
	cm := NewConnectionManager(3)
	sid := "server-1"
	if !cm.Acquire(sid) {
		t.Fatal("should allow first connection")
	}
	if !cm.Acquire(sid) || !cm.Acquire(sid) {
		t.Fatal("should reserve slots up to max")
	}
	if cm.Acquire(sid) {
		t.Fatal("should deny connection at max")
	}
}

func TestConnectionManagerDisconnect(t *testing.T) {
	cm := NewConnectionManager(2)
	sid := "server-2"
	cm.Acquire(sid)
	cm.Acquire(sid)
	if cm.Acquire(sid) {
		t.Fatal("should be at max")
	}
	cm.Disconnected(sid)
	if !cm.Acquire(sid) {
		t.Fatal("should allow after disconnect")
	}
}

func TestConnectionManagerCount(t *testing.T) {
	cm := NewConnectionManager(10)
	sid := "server-3"
	if cm.Count(sid) != 0 {
		t.Fatalf("expected 0, got %d", cm.Count(sid))
	}
	cm.Acquire(sid)
	cm.Acquire(sid)
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
	cm.Acquire(sid)
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
	cm.Acquire("s1")
	cm.Acquire("s1")
	cm.Acquire("s2")
	if cm.Acquire("s1") {
		t.Fatal("s1 should be at max")
	}
	if !cm.Acquire("s2") {
		t.Fatal("s2 should still allow connections")
	}
}
