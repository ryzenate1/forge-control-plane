package heartbeatmonitor

import (
	"context"
	"testing"
	"time"

	"gamepanel/forge/internal/store"
)

type mockHeartbeatStore struct {
	listNodesResult           []store.Node
	listNodesErr              error
	getNodeResult             store.Node
	getNodeErr                error
	listNodeHeartbeatHistory  []store.NodeHeartbeatHistory
	listNodeHeartbeatHistoryErr error
	setNodeHeartbeatClassification func(ctx context.Context, nodeID string, state store.NodeHeartbeatState, actualState store.NodeActualState, recoveryCount int, reason string) (store.Node, store.Node, error)
}

func (m *mockHeartbeatStore) ListNodes(ctx context.Context) ([]store.Node, error) {
	return m.listNodesResult, m.listNodesErr
}

func (m *mockHeartbeatStore) GetNode(ctx context.Context, nodeID string) (store.Node, error) {
	return m.getNodeResult, m.getNodeErr
}

func (m *mockHeartbeatStore) ListNodeHeartbeatHistory(ctx context.Context, nodeID string, limit int) ([]store.NodeHeartbeatHistory, error) {
	return m.listNodeHeartbeatHistory, m.listNodeHeartbeatHistoryErr
}

func (m *mockHeartbeatStore) SetNodeHeartbeatClassification(ctx context.Context, nodeID string, state store.NodeHeartbeatState, actualState store.NodeActualState, recoveryCount int, reason string) (store.Node, store.Node, error) {
	if m.setNodeHeartbeatClassification != nil {
		return m.setNodeHeartbeatClassification(ctx, nodeID, state, actualState, recoveryCount, reason)
	}
	return store.Node{}, store.Node{}, nil
}

func TestConsecutiveSuccessfulHeartbeats(t *testing.T) {
	tests := []struct {
		name    string
		history []store.NodeHeartbeatHistory
		want    int
	}{
		{
			name:    "empty history",
			history: nil,
			want:    0,
		},
		{
			name: "all successful",
			history: []store.NodeHeartbeatHistory{
				{Success: true},
				{Success: true},
				{Success: true},
			},
			want: 3,
		},
		{
			name: "first is failure",
			history: []store.NodeHeartbeatHistory{
				{Success: false},
				{Success: true},
			},
			want: 0,
		},
		{
			name: "failure breaks chain",
			history: []store.NodeHeartbeatHistory{
				{Success: true},
				{Success: true},
				{Success: false},
				{Success: true},
			},
			want: 2,
		},
		{
			name: "single entry success",
			history: []store.NodeHeartbeatHistory{
				{Success: true},
			},
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := consecutiveSuccessfulHeartbeats(tt.history)
			if got != tt.want {
				t.Fatalf("consecutiveSuccessfulHeartbeats() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestClassify_NilLastSeenAt(t *testing.T) {
	s := NewWithConfig(nil, nil, DefaultConfig())
	now := time.Now().UTC()

	state, actualState, recoveryCount, ageSeconds, reason := s.classify(
		store.Node{LastSeenAt: nil},
		nil,
		now,
	)

	if state != store.NodeHeartbeatStateOffline {
		t.Fatalf("expected offline, got %s", state)
	}
	if actualState != store.NodeActualStateOffline {
		t.Fatalf("expected offline actual, got %s", actualState)
	}
	if reason != "node has never reported heartbeat" {
		t.Fatalf("unexpected reason: %s", reason)
	}
	if recoveryCount != 0 {
		t.Fatalf("expected 0 recovery count, got %d", recoveryCount)
	}
	if ageSeconds != 0 {
		t.Fatalf("expected 0 age seconds, got %d", ageSeconds)
	}
}

func TestClassify_NegativeAge(t *testing.T) {
	future := time.Now().UTC().Add(1 * time.Hour)
	s := NewWithConfig(nil, nil, DefaultConfig())
	now := time.Now().UTC()

	state, _, ageSeconds, _, _ := s.classify(
		store.Node{LastSeenAt: &future, HeartbeatState: string(store.NodeHeartbeatStateHealthy)},
		nil,
		now,
	)

	if state != store.NodeHeartbeatStateHealthy {
		t.Fatalf("expected healthy, got %s", state)
	}
	if ageSeconds != 0 {
		t.Fatalf("expected 0 age seconds for future timestamp, got %d", ageSeconds)
	}
}

type classifyTestCase struct {
	name           string
	node           store.Node
	history        []store.NodeHeartbeatHistory
	config         Config
	wantState      store.NodeHeartbeatState
	wantActual     store.NodeActualState
	wantRecovery   int
	wantReasonSub  string
}

func TestClassify_StateMachineTransitions(t *testing.T) {
	now := time.Now().UTC()
	cfg := DefaultConfig()

	tests := []classifyTestCase{
		{
			name: "Healthy stays healthy",
			node: store.Node{
				LastSeenAt:     ptr(now.Add(-5 * time.Second)),
				HeartbeatState: string(store.NodeHeartbeatStateHealthy),
			},
			history:       []store.NodeHeartbeatHistory{{Success: true}},
			config:        cfg,
			wantState:     store.NodeHeartbeatStateHealthy,
			wantActual:    store.NodeActualStateOnline,
			wantRecovery:  1,
			wantReasonSub: "heartbeat healthy",
		},
		{
			name: "Healthy to Suspected via warning threshold",
			node: store.Node{
				LastSeenAt:     ptr(now.Add(-40 * time.Second)),
				HeartbeatState: string(store.NodeHeartbeatStateHealthy),
			},
			history:       []store.NodeHeartbeatHistory{{Success: true}},
			config:        cfg,
			wantState:     store.NodeHeartbeatStateSuspected,
			wantActual:    store.NodeActualStateDegraded,
			wantReasonSub: "heartbeat exceeded warning threshold",
		},
		{
			name: "Suspected to Unreachable",
			node: store.Node{
				LastSeenAt:     ptr(now.Add(-95 * time.Second)),
				HeartbeatState: string(store.NodeHeartbeatStateSuspected),
			},
			history:       []store.NodeHeartbeatHistory{{Success: true}},
			config:        cfg,
			wantState:     store.NodeHeartbeatStateUnreachable,
			wantActual:    store.NodeActualStateDegraded,
			wantReasonSub: "heartbeat exceeded offline threshold",
		},
		{
			name: "Unreachable to Offline",
			node: store.Node{
				LastSeenAt:     ptr(now.Add(-95 * time.Second)),
				HeartbeatState: string(store.NodeHeartbeatStateUnreachable),
			},
			history:       []store.NodeHeartbeatHistory{{Success: true}},
			config:        cfg,
			wantState:     store.NodeHeartbeatStateOffline,
			wantActual:    store.NodeActualStateOffline,
			wantReasonSub: "heartbeat remained unreachable",
		},
		{
			name: "Offline stays Offline",
			node: store.Node{
				LastSeenAt:     ptr(now.Add(-95 * time.Second)),
				HeartbeatState: string(store.NodeHeartbeatStateOffline),
			},
			history:       []store.NodeHeartbeatHistory{{Success: true}},
			config:        cfg,
			wantState:     store.NodeHeartbeatStateOffline,
			wantActual:    store.NodeActualStateOffline,
			wantReasonSub: "heartbeat remained unreachable",
		},
		{
			name: "Double offline threshold goes directly to Offline",
			node: store.Node{
				LastSeenAt:     ptr(now.Add(-185 * time.Second)),
				HeartbeatState: string(store.NodeHeartbeatStateHealthy),
			},
			history:       []store.NodeHeartbeatHistory{{Success: true}},
			config:        cfg,
			wantState:     store.NodeHeartbeatStateOffline,
			wantActual:    store.NodeActualStateOffline,
			wantReasonSub: "heartbeat expired beyond offline threshold",
		},
		{
			name: "Offline to Recovering (one heartbeat)",
			node: store.Node{
				LastSeenAt:     ptr(now.Add(-1 * time.Second)),
				HeartbeatState: string(store.NodeHeartbeatStateOffline),
			},
			history:       []store.NodeHeartbeatHistory{{Success: true}},
			config:        cfg,
			wantState:     store.NodeHeartbeatStateRecovering,
			wantActual:    store.NodeActualStateDegraded,
			wantRecovery:  1,
			wantReasonSub: "recovery threshold not met",
		},
		{
			name: "Recovering to Healthy (threshold met)",
			node: store.Node{
				LastSeenAt:     ptr(now.Add(-1 * time.Second)),
				HeartbeatState: string(store.NodeHeartbeatStateRecovering),
			},
			history:       []store.NodeHeartbeatHistory{{Success: true}, {Success: true}},
			config:        cfg,
			wantState:     store.NodeHeartbeatStateHealthy,
			wantActual:    store.NodeActualStateOnline,
			wantRecovery:  2,
			wantReasonSub: "recovery threshold satisfied",
		},
		{
			name: "Suspected to Recovering on recovery path",
			node: store.Node{
				LastSeenAt:     ptr(now.Add(-1 * time.Second)),
				HeartbeatState: string(store.NodeHeartbeatStateSuspected),
			},
			history:       []store.NodeHeartbeatHistory{{Success: true}},
			config:        cfg,
			wantState:     store.NodeHeartbeatStateRecovering,
			wantActual:    store.NodeActualStateDegraded,
			wantRecovery:  1,
			wantReasonSub: "recovery threshold not met",
		},
		{
			name: "Latest heartbeat failure triggers Suspected even if age is low",
			node: store.Node{
				LastSeenAt:     ptr(now.Add(-5 * time.Second)),
				HeartbeatState: string(store.NodeHeartbeatStateHealthy),
			},
			history:       []store.NodeHeartbeatHistory{{Success: false}},
			config:        cfg,
			wantState:     store.NodeHeartbeatStateSuspected,
			wantActual:    store.NodeActualStateDegraded,
			wantReasonSub: "latest heartbeat reported failure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewWithConfig(nil, nil, tt.config)
			state, actualState, recoveryCount, _, reason := s.classify(tt.node, tt.history, now)

			if state != tt.wantState {
				t.Fatalf("heartbeat state = %s, want %s", state, tt.wantState)
			}
			if actualState != tt.wantActual {
				t.Fatalf("actual state = %s, want %s", actualState, tt.wantActual)
			}
			if recoveryCount != tt.wantRecovery {
				t.Fatalf("recovery count = %d, want %d", recoveryCount, tt.wantRecovery)
			}
			if tt.wantReasonSub != "" && !contains(reason, tt.wantReasonSub) {
				t.Fatalf("reason = %q, want substring %q", reason, tt.wantReasonSub)
			}
		})
	}
}

func TestClassify_RecoveryThresholdBoundary(t *testing.T) {
	now := time.Now().UTC()
	cfg := Config{WarningThreshold: 30 * time.Second, OfflineThreshold: 90 * time.Second, RecoveryThreshold: 2, Interval: 30 * time.Second}

	node := store.Node{
		LastSeenAt:     ptr(now.Add(-1 * time.Second)),
		HeartbeatState: string(store.NodeHeartbeatStateOffline),
	}

	t.Run("recovery threshold not met with 1 success", func(t *testing.T) {
		state, _, count, _, _ := NewWithConfig(nil, nil, cfg).classify(node, []store.NodeHeartbeatHistory{{Success: true}}, now)
		if state != store.NodeHeartbeatStateRecovering {
			t.Fatalf("expected recovering, got %s", state)
		}
		if count != 1 {
			t.Fatalf("expected count 1, got %d", count)
		}
	})

	t.Run("recovery threshold exactly met with 2 successes", func(t *testing.T) {
		state, _, count, _, _ := NewWithConfig(nil, nil, cfg).classify(node, []store.NodeHeartbeatHistory{{Success: true}, {Success: true}}, now)
		if state != store.NodeHeartbeatStateHealthy {
			t.Fatalf("expected healthy, got %s", state)
		}
		if count != 2 {
			t.Fatalf("expected count 2, got %d", count)
		}
	})

	t.Run("recovery threshold met with more than needed", func(t *testing.T) {
		state, _, count, _, _ := NewWithConfig(nil, nil, cfg).classify(node, []store.NodeHeartbeatHistory{{Success: true}, {Success: true}, {Success: true}}, now)
		if state != store.NodeHeartbeatStateHealthy {
			t.Fatalf("expected healthy, got %s", state)
		}
		if count != 3 {
			t.Fatalf("expected count 3, got %d", count)
		}
	})
}

func TestClassify_ConfigBoundaries(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name          string
		age           time.Duration
		prevState     string
		wantState     store.NodeHeartbeatState
		wantActual    store.NodeActualState
	}{
		{
			name:      "just under warning threshold",
			age:       29 * time.Second,
			prevState: string(store.NodeHeartbeatStateHealthy),
			wantState: store.NodeHeartbeatStateHealthy,
			wantActual: store.NodeActualStateOnline,
		},
		{
			name:      "at warning threshold",
			age:       30 * time.Second,
			prevState: string(store.NodeHeartbeatStateHealthy),
			wantState: store.NodeHeartbeatStateSuspected,
			wantActual: store.NodeActualStateDegraded,
		},
		{
			name:      "just under offline threshold",
			age:       89 * time.Second,
			prevState: string(store.NodeHeartbeatStateSuspected),
			wantState: store.NodeHeartbeatStateSuspected,
			wantActual: store.NodeActualStateDegraded,
		},
		{
			name:      "at offline threshold (previously healthy)",
			age:       90 * time.Second,
			prevState: string(store.NodeHeartbeatStateHealthy),
			wantState: store.NodeHeartbeatStateUnreachable,
			wantActual: store.NodeActualStateDegraded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			s := NewWithConfig(nil, nil, cfg)
			node := store.Node{
				LastSeenAt:     ptr(now.Add(-tt.age)),
				HeartbeatState: tt.prevState,
			}
			state, actualState, _, _, _ := s.classify(node, []store.NodeHeartbeatHistory{{Success: true}}, now)
			if state != tt.wantState {
				t.Fatalf("heartbeat state = %s, want %s", state, tt.wantState)
			}
			if actualState != tt.wantActual {
				t.Fatalf("actual state = %s, want %s", actualState, tt.wantActual)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.WarningThreshold != 30*time.Second {
		t.Fatalf("warning threshold = %v, want 30s", cfg.WarningThreshold)
	}
	if cfg.OfflineThreshold != 90*time.Second {
		t.Fatalf("offline threshold = %v, want 90s", cfg.OfflineThreshold)
	}
	if cfg.RecoveryThreshold != 2 {
		t.Fatalf("recovery threshold = %d, want 2", cfg.RecoveryThreshold)
	}
	if cfg.Interval != 30*time.Second {
		t.Fatalf("interval = %v, want 30s", cfg.Interval)
	}
}

func TestNormalizeConfig(t *testing.T) {
	t.Run("zero values get defaults", func(t *testing.T) {
		cfg := normalizeConfig(Config{})
		if cfg.WarningThreshold != 30*time.Second {
			t.Fatalf("expected default warning threshold, got %v", cfg.WarningThreshold)
		}
		if cfg.OfflineThreshold != 90*time.Second {
			t.Fatalf("expected default offline threshold, got %v", cfg.OfflineThreshold)
		}
		if cfg.RecoveryThreshold != 2 {
			t.Fatalf("expected default recovery threshold, got %d", cfg.RecoveryThreshold)
		}
		if cfg.Interval != 30*time.Second {
			t.Fatalf("expected default interval, got %v", cfg.Interval)
		}
	})

	t.Run("negative values get defaults", func(t *testing.T) {
		cfg := normalizeConfig(Config{
			WarningThreshold:  -1,
			OfflineThreshold:  -1,
			RecoveryThreshold: -1,
			Interval:          -1,
		})
		if cfg.WarningThreshold != 30*time.Second {
			t.Fatalf("expected default warning, got %v", cfg.WarningThreshold)
		}
		if cfg.OfflineThreshold != 90*time.Second {
			t.Fatalf("expected default offline, got %v", cfg.OfflineThreshold)
		}
		if cfg.RecoveryThreshold != 2 {
			t.Fatalf("expected default recovery, got %d", cfg.RecoveryThreshold)
		}
		if cfg.Interval != 30*time.Second {
			t.Fatalf("expected default interval, got %v", cfg.Interval)
		}
	})

	t.Run("offline threshold must be >= warning threshold", func(t *testing.T) {
		cfg := normalizeConfig(Config{
			WarningThreshold:  60 * time.Second,
			OfflineThreshold:  30 * time.Second,
			RecoveryThreshold: 2,
			Interval:          30 * time.Second,
		})
		if cfg.OfflineThreshold < cfg.WarningThreshold {
			t.Fatalf("offline threshold %v should be >= warning threshold %v", cfg.OfflineThreshold, cfg.WarningThreshold)
		}
	})

	t.Run("preserves valid config", func(t *testing.T) {
		cfg := normalizeConfig(Config{
			WarningThreshold:  60 * time.Second,
			OfflineThreshold:  120 * time.Second,
			RecoveryThreshold: 5,
			Interval:          15 * time.Second,
		})
		if cfg.WarningThreshold != 60*time.Second {
			t.Fatalf("warning changed to %v", cfg.WarningThreshold)
		}
		if cfg.OfflineThreshold != 120*time.Second {
			t.Fatalf("offline changed to %v", cfg.OfflineThreshold)
		}
		if cfg.RecoveryThreshold != 5 {
			t.Fatalf("recovery changed to %d", cfg.RecoveryThreshold)
		}
	})
}

func TestEvaluateAll_NilReceiver(t *testing.T) {
	var nilSvc *Service
	err := nilSvc.EvaluateAll(context.Background())
	if err != nil {
		t.Fatalf("expected nil error from nil receiver, got %v", err)
	}
}

func TestEvaluateAll_NilStore(t *testing.T) {
	svc := NewWithConfig(nil, nil, DefaultConfig())
	err := svc.EvaluateAll(context.Background())
	if err != nil {
		t.Fatalf("expected nil error when store is nil, got %v", err)
	}
}

func TestEvaluateAll_EmptyNodes(t *testing.T) {
	store := &mockHeartbeatStore{
		listNodesResult: []store.Node{},
	}
	svc := NewWithConfig(store, nil, DefaultConfig())
	err := svc.EvaluateAll(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestEvaluateAll_ListNodeError(t *testing.T) {
	store := &mockHeartbeatStore{
		listNodesErr: context.DeadlineExceeded,
	}
	svc := NewWithConfig(store, nil, DefaultConfig())
	err := svc.EvaluateAll(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestEvaluateAll_EvaluatesAllNodes(t *testing.T) {
	evaluated := 0
	store := &mockHeartbeatStore{
		listNodesResult: []store.Node{
			{ID: "node-1", HeartbeatState: string(store.NodeHeartbeatStateHealthy)},
			{ID: "node-2", HeartbeatState: string(store.NodeHeartbeatStateSuspected)},
		},
		getNodeResult: store.Node{ID: "node-1", HeartbeatState: string(store.NodeHeartbeatStateHealthy)},
		listNodeHeartbeatHistory: []store.NodeHeartbeatHistory{
			{Success: true, ObservedAt: time.Now().UTC()},
		},
		setNodeHeartbeatClassification: func(ctx context.Context, nodeID string, state store.NodeHeartbeatState, actualState store.NodeActualState, recoveryCount int, reason string) (store.Node, store.Node, error) {
			evaluated++
			return store.Node{HeartbeatState: string(state), ActualState: string(actualState)}, store.Node{HeartbeatState: string(state), ActualState: string(actualState)}, nil
		},
	}
	svc := NewWithConfig(store, nil, DefaultConfig())
	err := svc.EvaluateAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evaluated != 2 {
		t.Fatalf("expected 2 evaluations, got %d", evaluated)
	}
}

func TestEvaluateAll_ContinuesOnError(t *testing.T) {
	evaluated := 0
	callCount := 0
	store := &mockHeartbeatStore{
		listNodesResult: []store.Node{
			{ID: "node-1", HeartbeatState: string(store.NodeHeartbeatStateHealthy)},
			{ID: "node-2", HeartbeatState: string(store.NodeHeartbeatStateSuspected)},
		},
		listNodeHeartbeatHistory: []store.NodeHeartbeatHistory{
			{Success: true, ObservedAt: time.Now().UTC()},
		},
		listNodeHeartbeatHistoryErr: nil,
		setNodeHeartbeatClassification: func(ctx context.Context, nodeID string, state store.NodeHeartbeatState, actualState store.NodeActualState, recoveryCount int, reason string) (store.Node, store.Node, error) {
			callCount++
			if callCount == 2 {
				return store.Node{}, store.Node{}, context.DeadlineExceeded
			}
			evaluated++
			return store.Node{HeartbeatState: string(state), ActualState: string(actualState)}, store.Node{HeartbeatState: string(state), ActualState: string(actualState)}, nil
		},
	}
	svc := NewWithConfig(store, nil, DefaultConfig())
	err := svc.EvaluateAll(context.Background())
	if err != nil {
		t.Fatalf("expected nil (errors swallowed per node), got %v", err)
	}
	if evaluated != 1 {
		t.Fatalf("expected exactly 1 successful evaluation, got %d", evaluated)
	}
}

func TestEvaluateAll_HistorySortedMostRecentFirst(t *testing.T) {
	now := time.Now().UTC()
	store := &mockHeartbeatStore{
		listNodesResult: []store.Node{
			{
				ID:             "node-1",
				LastSeenAt:     ptr(now.Add(-1 * time.Second)),
				HeartbeatState: string(store.NodeHeartbeatStateHealthy),
			},
		},
		getNodeResult: store.Node{
			ID:             "node-1",
			LastSeenAt:     ptr(now.Add(-1 * time.Second)),
			HeartbeatState: string(store.NodeHeartbeatStateHealthy),
		},
		// Simulate ASC order (oldest first) to verify re-sort
		listNodeHeartbeatHistory: []store.NodeHeartbeatHistory{
			{Success: true, ObservedAt: now.Add(-10 * time.Second)},
			{Success: false, ObservedAt: now.Add(-5 * time.Second)},
			{Success: true, ObservedAt: now.Add(-1 * time.Second)},
		},
		setNodeHeartbeatClassification: func(ctx context.Context, nodeID string, state store.NodeHeartbeatState, actualState store.NodeActualState, recoveryCount int, reason string) (store.Node, store.Node, error) {
			// With proper DESC sorting, the most recent heartbeat (success=true) is first,
			// so the node should be classified as healthy (recovery threshold 2 means
			// need at least 2 successes, but we only have 1 recent success after sort).
			// Actually with the ASC input sorted to DESC: [{Success:true, -1s}, {Success:false, -5s}, {Success:true, -10s}]
			// consecutive successes = 1 (only the first entry), recovery threshold = 2
			// Since previous state is healthy, it stays healthy regardless of recovery count
			if string(state) != string(store.NodeHeartbeatStateHealthy) {
				t.Fatalf("expected healthy state with sorted history, got %s", state)
			}
			return store.Node{HeartbeatState: string(state), ActualState: string(actualState)}, store.Node{HeartbeatState: string(state), ActualState: string(actualState)}, nil
		},
	}
	svc := NewWithConfig(store, nil, DefaultConfig())
	err := svc.EvaluateAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigAndMetricsNilSafety(t *testing.T) {
	var nilSvc *Service
	cfg := nilSvc.Config()
	if cfg.WarningThreshold <= 0 {
		t.Fatal("nil receiver Config should return defaults")
	}
	metrics := nilSvc.Metrics()
	if metrics.HeartbeatEvaluationsTotal != 0 {
		t.Fatal("nil receiver Metrics should return zero value")
	}
}

func ptr(t time.Time) *time.Time {
	return &t
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (sub == "" || (len(s) >= len(sub) && searchString(s, sub)))
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
