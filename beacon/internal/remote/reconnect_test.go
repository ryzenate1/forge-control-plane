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
	// Start only reports connected after a round-trip proves the link, so a
	// fake unreachable panel URL can never satisfy this test. Inject a stub
	// client whose probe succeeds instead.
	stub := &stubPanelClient{}
	rc := NewReconnectClientWithClient(stub, nil, 30*time.Second)
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

// stubPanelClient proves the link for Start: its probe round-trip succeeds
// without a real panel. Only GetServers is overridden; every other Client
// method embeds the nil interface and would panic if called, so a test that
// starts probing new ground fails loudly instead of passing quietly.
type stubPanelClient struct {
	Client
	servers []RawServerData
	err     error
}

func (s *stubPanelClient) GetServers(ctx context.Context, perPage int) ([]RawServerData, error) {
	if s == nil {
		return nil, context.Canceled
	}
	return s.servers, s.err
}
