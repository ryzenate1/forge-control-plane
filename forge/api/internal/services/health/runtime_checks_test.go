package health

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestAPIRuntimeCheck(t *testing.T) {
	result := NewAPIRuntimeCheck(time.Now().Add(-time.Second)).Run(context.Background())

	if result.Name != "api" {
		t.Fatalf("name = %q, want api", result.Name)
	}
	if result.Label != "API Runtime" {
		t.Fatalf("label = %q, want API Runtime", result.Label)
	}
	if result.Status != StatusOK {
		t.Fatalf("status = %q, want ok", result.Status)
	}
	if _, ok := result.Details["heapAllocBytes"]; !ok {
		t.Fatalf("details missing heapAllocBytes: %#v", result.Details)
	}
}

func TestMemoryCheckWithoutThresholdIsInformational(t *testing.T) {
	result := NewMemoryCheck(0).Run(context.Background())

	if result.Status != StatusOK {
		t.Fatalf("status = %q, want ok without a configured limit", result.Status)
	}
	if !strings.Contains(result.Message, "informational") {
		t.Fatalf("message = %q, want informational semantics", result.Message)
	}
	if _, ok := result.Details["thresholdBytes"]; ok {
		t.Fatalf("details unexpectedly contain a threshold: %#v", result.Details)
	}
}

func TestMemoryCheckExceedingThresholdWarns(t *testing.T) {
	result := NewMemoryCheck(1).Run(context.Background())

	if result.Status != StatusWarning {
		t.Fatalf("status = %q, want warning", result.Status)
	}
	if result.Details["thresholdBytes"] != uint64(1) {
		t.Fatalf("thresholdBytes = %#v, want 1", result.Details["thresholdBytes"])
	}
}

func TestDaemonCheckDescribesPersistedHeartbeatState(t *testing.T) {
	check := NewDaemonCheck(func(context.Context) (int, int, int, map[string]any, error) {
		return 2, 1, 1, map[string]any{"oldestHeartbeatAgeSeconds": int64(42)}, nil
	})
	result := check.Run(context.Background())

	if result.Label != "Daemon Heartbeat Status" {
		t.Fatalf("label = %q", result.Label)
	}
	if !strings.Contains(result.Message, "persisted") {
		t.Fatalf("message = %q, want persisted-heartbeat semantics", result.Message)
	}
	if result.Details["healthyHeartbeatNodes"] != 1 || result.Details["nonHealthyHeartbeatNodes"] != 1 {
		t.Fatalf("unexpected heartbeat counts: %#v", result.Details)
	}
	if result.Details["stateSource"] != "persistedHeartbeatState" {
		t.Fatalf("unexpected state source: %#v", result.Details)
	}
	// Keep aliases for established health-report consumers, but they are
	// documented by DaemonCheck as persisted heartbeat state.
	if result.Details["onlineNodes"] != 1 || result.Details["offlineNodes"] != 1 {
		t.Fatalf("unexpected compatibility counts: %#v", result.Details)
	}
}
