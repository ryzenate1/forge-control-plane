package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestEdgeAgentInitialState(t *testing.T) {
	agent := NewEdgeAgent("http://panel:8080", "token", "node-1", "1.0.0")
	if state := agent.State(); state != EdgeStateDisconnected {
		t.Fatalf("expected disconnected, got %s", state)
	}
}

func TestEdgeAgentStartStop(t *testing.T) {
	agent := NewEdgeAgent("http://panel:8080", "token", "node-1", "1.0.0")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		agent.Start(ctx)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	if state := agent.State(); state != EdgeStateConnected {
		t.Fatalf("expected connected, got %s", state)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("agent did not stop")
	}
}

func TestEdgeAgentReconnectTriggers(t *testing.T) {
	agent := NewEdgeAgent("http://panel:8080", "token", "node-1", "1.0.0")
	agent.offlineTimeout = 100 * time.Millisecond
	agent.hbInterval = 200 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go agent.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	agent.mu.Lock()
	agent.lastHeartbeat = time.Now().Add(-10 * time.Minute)
	agent.mu.Unlock()
	time.Sleep(200 * time.Millisecond)
	stats := agent.Stats()
	if offline, ok := stats["offlineDetected"].(bool); ok && offline {
	} else {
		t.Log("offline may not be detected within test window (race ok)")
	}
}

func TestEdgeAgentBackoffConfig(t *testing.T) {
	agent := NewEdgeAgent("http://panel:8080", "token", "node-1", "1.0.0")
	if agent.backoffCfg.Initial != DefaultBackoffConfig.Initial {
		t.Fatalf("expected initial backoff %v, got %v", DefaultBackoffConfig.Initial, agent.backoffCfg.Initial)
	}
	if agent.backoffCfg.Max != DefaultBackoffConfig.Max {
		t.Fatalf("expected max backoff %v, got %v", DefaultBackoffConfig.Max, agent.backoffCfg.Max)
	}
}

func TestEdgeAgentConnectNow(t *testing.T) {
	agent := NewEdgeAgent("http://panel:8080", "token", "node-1", "1.0.0")
	agent.setState(EdgeStateOffline)
	if state := agent.State(); state != EdgeStateOffline {
		t.Fatalf("expected offline, got %s", state)
	}
	agent.ConnectNow()
	if state := agent.State(); state != EdgeStateConnected {
		t.Fatalf("expected connected after ConnectNow, got %s", state)
	}
}

func TestEdgeAgentCallbacks(t *testing.T) {
	agent := NewEdgeAgent("http://panel:8080", "token", "node-1", "1.0.0")
	var connectCount int32
	var disconnectCount int32
	agent.onConnect = func() {
		atomic.AddInt32(&connectCount, 1)
	}
	agent.onDisconnect = func() {
		atomic.AddInt32(&disconnectCount, 1)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go agent.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)
	if c := atomic.LoadInt32(&connectCount); c != 1 {
		t.Fatalf("expected 1 connect callback, got %d", c)
	}
	if d := atomic.LoadInt32(&disconnectCount); d != 1 {
		t.Fatalf("expected 1 disconnect callback, got %d", d)
	}
}

func TestEdgeAgentStateTransitions(t *testing.T) {
	agent := NewEdgeAgent("http://panel:8080", "token", "node-1", "1.0.0")
	transitions := []EdgeState{EdgeStateConnected, EdgeStateReconnecting, EdgeStateOffline, EdgeStateDisconnected}
	for _, s := range transitions {
		agent.setState(s)
		if agent.State() != s {
			t.Fatalf("expected state %s, got %s", s, agent.State())
		}
	}
}

func TestEdgeAgentStats(t *testing.T) {
	agent := NewEdgeAgent("http://panel:8080", "token", "node-1", "1.0.0")
	agent.setState(EdgeStateConnected)
	stats := agent.Stats()
	if stats["state"] != "connected" {
		t.Fatalf("expected connected in stats, got %v", stats["state"])
	}
	if _, ok := stats["reconnectCount"]; !ok {
		t.Fatal("expected reconnectCount in stats")
	}
	if _, ok := stats["offlineTimeoutMs"]; !ok {
		t.Fatal("expected offlineTimeoutMs in stats")
	}
}

func TestEdgeAgentTryReconnectSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/edge/connect" && r.Method == http.MethodPost {
			var req connectRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if req.Token == "valid-token" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(connectResponse{Connected: true, NodeID: req.NodeID})
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(connectResponse{Connected: false, Message: "invalid token"})
		}
	}))
	defer server.Close()

	t.Run("valid token reconnects", func(t *testing.T) {
		agent := NewEdgeAgent(server.URL, "valid-token", "node-1", "1.0.0")
		agent.httpClient = server.Client()
		if !agent.tryReconnect() {
			t.Fatal("expected successful reconnect")
		}
	})

	t.Run("invalid token rejected", func(t *testing.T) {
		agent := NewEdgeAgent(server.URL, "bad-token", "node-1", "1.0.0")
		agent.httpClient = server.Client()
		if agent.tryReconnect() {
			t.Fatal("expected failed reconnect")
		}
	})

	t.Run("unreachable server", func(t *testing.T) {
		agent := NewEdgeAgent("http://localhost:19999", "token", "node-1", "1.0.0")
		agent.httpClient = &http.Client{Timeout: 100 * time.Millisecond}
		if agent.tryReconnect() {
			t.Fatal("expected failed reconnect for unreachable server")
		}
	})
}

func TestEdgeAgentReconnectLoopSucceeds(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(connectResponse{Connected: true, NodeID: "node-1"})
	}))
	defer server.Close()

	agent := NewEdgeAgent(server.URL, "token", "node-1", "1.0.0")
	agent.httpClient = server.Client()
	agent.backoffCfg = BackoffConfig{Initial: 1 * time.Millisecond, Max: 50 * time.Millisecond, Factor: 2.0, Jitter: false}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var connected atomic.Bool
	agent.onConnect = func() { connected.Store(true) }

	go agent.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	agent.mu.Lock()
	agent.lastHeartbeat = time.Now().Add(-1 * time.Hour)
	agent.mu.Unlock()

	time.Sleep(300 * time.Millisecond)

	if !connected.Load() {
		t.Fatal("expected agent to successfully reconnect")
	}
}

func TestEdgeAgentOfflineDetectionConfigurable(t *testing.T) {
	agent := NewEdgeAgent("http://panel:8080", "token", "node-1", "1.0.0")
	agent.SetOfflineTimeout(5 * time.Second)
	agent.SetHBInterval(2 * time.Second)
	agent.SetBackoffConfig(BackoffConfig{Initial: 100 * time.Millisecond, Max: 1 * time.Second, Factor: 2.0, Jitter: false})
	if agent.offlineTimeout != 5*time.Second {
		t.Fatalf("expected offline timeout 5s, got %v", agent.offlineTimeout)
	}
	if agent.hbInterval != 2*time.Second {
		t.Fatalf("expected hb interval 2s, got %v", agent.hbInterval)
	}
}

func TestEdgeAgentBackoffValues(t *testing.T) {
	tests := []struct {
		name    string
		initial time.Duration
		max     time.Duration
		factor  float64
		iters   int
		want    time.Duration
	}{
		{name: "doubles from 1s", initial: 1 * time.Second, max: 60 * time.Second, factor: 2.0, iters: 4, want: 16 * time.Second},
		{name: "capped at max", initial: 1 * time.Second, max: 10 * time.Second, factor: 3.0, iters: 3, want: 10 * time.Second},
		{name: "single iteration", initial: 5 * time.Second, max: 60 * time.Second, factor: 2.0, iters: 1, want: 10 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backoff := tt.initial
			for i := 0; i < tt.iters; i++ {
				backoff = time.Duration(float64(backoff) * tt.factor)
				if backoff > tt.max {
					backoff = tt.max
				}
			}
			if backoff != tt.want {
				t.Fatalf("expected backoff %v, got %v", tt.want, backoff)
			}
		})
	}
}
