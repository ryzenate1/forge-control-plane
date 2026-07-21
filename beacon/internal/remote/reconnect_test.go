package remote

import (
	"context"
	"testing"
	"time"
)

func TestReconnectClientInitialState(t *testing.T) {
	rc := NewReconnectClient("http://panel:8080", "test-token", 30*time.Second)
	if state := rc.State(); state != StateDisconnected {
		t.Fatalf("expected disconnected, got %s", state)
	}
}

func TestReconnectClientStartStop(t *testing.T) {
	rc := NewReconnectClient("http://panel:8080", "test-token", 30*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		rc.Start(ctx)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	if state := rc.State(); state != StateConnected {
		t.Fatalf("expected connected, got %s", state)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reconnect client did not stop")
	}
}

func TestReconnectClientStateTransition(t *testing.T) {
	rc := NewReconnectClient("http://panel:8080", "test-token", 30*time.Second)
	if rc.State().String() != "disconnected" {
		t.Fatalf("expected disconnected string, got %s", rc.State().String())
	}
}

func TestReconnectClientInner(t *testing.T) {
	rc := NewReconnectClient("http://panel:8080", "test-token", 30*time.Second)
	inner := rc.Inner()
	if inner == nil {
		t.Fatal("expected non-nil inner client")
	}
}

func TestReconnectClientStats(t *testing.T) {
	rc := NewReconnectClient("http://panel:8080", "test-token", 30*time.Second)
	stats := rc.Stats()
	if stats["state"] != "disconnected" {
		t.Fatalf("expected disconnected in stats, got %v", stats["state"])
	}
	if _, ok := stats["attempts"]; !ok {
		t.Fatal("expected attempts in stats")
	}
}
