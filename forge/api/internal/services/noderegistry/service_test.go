package noderegistry

import (
	"context"
	"testing"
	"time"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"
)

func TestHealthScore_Computation(t *testing.T) {
	svc := &Service{}

	tests := []struct {
		name     string
		node     store.Node
		capacity store.NodeCapacitySnapshot
		wantMin  int
		wantMax  int
	}{
		{
			name: "fully healthy node",
			node: store.Node{
				ActualState: string(domain.NodeActualStateOnline),
				LastSeenAt:  ptr(time.Now().Add(-30 * time.Second)),
			},
			capacity: store.NodeCapacitySnapshot{
				TotalCPU:      100,
				AvailableCPU:  100,
				TotalMemory:   100,
				AvailableMemory: 100,
				TotalDisk:     100,
				AvailableDisk: 100,
			},
			wantMin: 90,
			wantMax: 100,
		},
		{
			name: "fully exhausted node",
			node: store.Node{
				ActualState: string(domain.NodeActualStateOffline),
				LastSeenAt:  nil,
			},
			capacity: store.NodeCapacitySnapshot{
				TotalCPU:       100,
				AvailableCPU:   0,
				TotalMemory:    100,
				AvailableMemory: 0,
				TotalDisk:      100,
				AvailableDisk:  0,
			},
			wantMin: 0,
			wantMax: 20,
		},
		{
			name: "node with nil last seen",
			node: store.Node{
				ActualState: string(domain.NodeActualStateOnline),
				LastSeenAt:  nil,
			},
			capacity: store.NodeCapacitySnapshot{
				TotalCPU:       100,
				AvailableCPU:   50,
				TotalMemory:    100,
				AvailableMemory: 50,
				TotalDisk:      100,
				AvailableDisk:  50,
			},
			wantMin: 30,
			wantMax: 70,
		},
		{
			name: "degraded node",
			node: store.Node{
				ActualState: string(domain.NodeActualStateDegraded),
				LastSeenAt:  ptr(time.Now().Add(-1 * time.Minute)),
			},
			capacity: store.NodeCapacitySnapshot{
				TotalCPU:       100,
				AvailableCPU:   50,
				TotalMemory:    100,
				AvailableMemory: 50,
				TotalDisk:      100,
				AvailableDisk:  50,
			},
			wantMin: 40,
			wantMax: 70,
		},
		{
			name: "zero total capacity defaults to score 50",
			node: store.Node{
				ActualState: string(domain.NodeActualStateOnline),
				LastSeenAt:  ptr(time.Now().Add(-30 * time.Second)),
			},
			capacity: store.NodeCapacitySnapshot{
				TotalCPU:       0,
				AvailableCPU:   0,
				TotalMemory:    0,
				AvailableMemory: 0,
				TotalDisk:      0,
				AvailableDisk:  0,
			},
			wantMin: 60,
			wantMax: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := svc.HealthScore(tt.node, tt.capacity)
			if score.Total < tt.wantMin || score.Total > tt.wantMax {
				t.Fatalf("total health score = %d, want between %d and %d (cpu=%d mem=%d disk=%d hb=%d status=%d)",
					score.Total, tt.wantMin, tt.wantMax, score.CPU, score.Memory, score.Disk, score.Heartbeat, score.Status)
			}
		})
	}
}

func TestHeartbeatScoreAge(t *testing.T) {
	tests := []struct {
		name string
		age  time.Duration
		want int
	}{
		{"zero age", 0, 100},
		{"just under 2 min", 2*time.Minute - time.Nanosecond, 100},
		{"exactly 2 min", 2 * time.Minute, 100},
		{"just over 2 min", 2*time.Minute + time.Nanosecond, 75},
		{"exactly 5 min", 5 * time.Minute, 75},
		{"just over 5 min", 5*time.Minute + time.Nanosecond, 40},
		{"exactly 15 min", 15 * time.Minute, 40},
		{"way beyond 15 min", 30 * time.Minute, 0},
		{"large duration", 24 * time.Hour, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := heartbeatScoreAge(tt.age)
			if got != tt.want {
				t.Fatalf("heartbeatScoreAge(%v) = %d, want %d", tt.age, got, tt.want)
			}
		})
	}
}

func TestHeartbeatScore(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name     string
		lastSeen *time.Time
		want     int
	}{
		{"nil last seen", nil, 0},
		{"recent heartbeat", ptr(now.Add(-30 * time.Second)), 100},
		{"old heartbeat", ptr(now.Add(-20 * time.Minute)), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := heartbeatScore(tt.lastSeen)
			if got != tt.want {
				t.Fatalf("heartbeatScore() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestStatusScore(t *testing.T) {
	tests := []struct {
		status string
		want   int
	}{
		{string(domain.NodeActualStateOnline), 100},
		{string(domain.NodeActualStateDegraded), 40},
		{string(domain.NodeActualStateOffline), 0},
		{"unknown", 0},
		{"", 0},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := statusScore(tt.status)
			if got != tt.want {
				t.Fatalf("statusScore(%q) = %d, want %d", tt.status, got, tt.want)
			}
		})
	}
}

func TestResourceScore(t *testing.T) {
	tests := []struct {
		name      string
		total     int
		available int
		want      int
	}{
		{"no total capacity", 0, 0, 50},
		{"no total with non-zero available", 0, 100, 50},
		{"fully available", 100, 100, 100},
		{"half used", 100, 50, 50},
		{"fully used", 100, 0, 0},
		{"overcommitted (negative available)", 100, -50, 0},
		{"over capacity", 100, 150, 100},
		{"quarter used", 100, 75, 75},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resourceScore(tt.total, tt.available)
			if got != tt.want {
				t.Fatalf("resourceScore(%d, %d) = %d, want %d", tt.total, tt.available, got, tt.want)
			}
		})
	}
}

func TestPlacementEligibility(t *testing.T) {
	online := string(domain.NodeActualStateOnline)
	offline := string(domain.NodeActualStateOffline)
	degraded := string(domain.NodeActualStateDegraded)

	tests := []struct {
		name     string
		node     store.Node
		capacity store.NodeCapacitySnapshot
		eligible bool
		reason   string
	}{
		{
			name:     "eligible",
			node:     store.Node{ActualState: online},
			capacity: store.NodeCapacitySnapshot{TotalCPU: 100, AvailableCPU: 50, TotalMemory: 100, AvailableMemory: 50, TotalDisk: 100, AvailableDisk: 50},
			eligible: true,
			reason:   "",
		},
		{
			name:     "not online",
			node:     store.Node{ActualState: offline},
			capacity: store.NodeCapacitySnapshot{TotalCPU: 100, AvailableCPU: 50, TotalMemory: 100, AvailableMemory: 50, TotalDisk: 100, AvailableDisk: 50},
			eligible: false,
			reason:   "node is not online",
		},
		{
			name:     "degraded not eligible",
			node:     store.Node{ActualState: degraded},
			capacity: store.NodeCapacitySnapshot{TotalCPU: 100, AvailableCPU: 50, TotalMemory: 100, AvailableMemory: 50, TotalDisk: 100, AvailableDisk: 50},
			eligible: false,
			reason:   "node is not online",
		},
		{
			name:     "maintenance mode",
			node:     store.Node{ActualState: online, DesiredState: store.NodeDesiredStateMaintenance},
			capacity: store.NodeCapacitySnapshot{TotalCPU: 100, AvailableCPU: 50, TotalMemory: 100, AvailableMemory: 50, TotalDisk: 100, AvailableDisk: 50},
			eligible: false,
			reason:   "maintenance",
		},
		{
			name:     "draining",
			node:     store.Node{ActualState: online, DesiredState: store.NodeDesiredStateDraining},
			capacity: store.NodeCapacitySnapshot{TotalCPU: 100, AvailableCPU: 50, TotalMemory: 100, AvailableMemory: 50, TotalDisk: 100, AvailableDisk: 50},
			eligible: false,
			reason:   "draining",
		},
		{
			name:     "draining field",
			node:     store.Node{ActualState: online, Draining: true},
			capacity: store.NodeCapacitySnapshot{TotalCPU: 100, AvailableCPU: 50, TotalMemory: 100, AvailableMemory: 50, TotalDisk: 100, AvailableDisk: 50},
			eligible: false,
			reason:   "draining",
		},
		{
			name:     "maintenance field",
			node:     store.Node{ActualState: online, Maintenance: true},
			capacity: store.NodeCapacitySnapshot{TotalCPU: 100, AvailableCPU: 50, TotalMemory: 100, AvailableMemory: 50, TotalDisk: 100, AvailableDisk: 50},
			eligible: false,
			reason:   "maintenance",
		},
		{
			name:     "cpu exhausted",
			node:     store.Node{ActualState: online},
			capacity: store.NodeCapacitySnapshot{TotalCPU: 100, AvailableCPU: 0, TotalMemory: 100, AvailableMemory: 50, TotalDisk: 100, AvailableDisk: 50},
			eligible: false,
			reason:   "cpu exhausted",
		},
		{
			name:     "memory exhausted",
			node:     store.Node{ActualState: online},
			capacity: store.NodeCapacitySnapshot{TotalCPU: 100, AvailableCPU: 50, TotalMemory: 100, AvailableMemory: 0, TotalDisk: 100, AvailableDisk: 50},
			eligible: false,
			reason:   "memory exhausted",
		},
		{
			name:     "disk exhausted",
			node:     store.Node{ActualState: online},
			capacity: store.NodeCapacitySnapshot{TotalCPU: 100, AvailableCPU: 50, TotalMemory: 100, AvailableMemory: 50, TotalDisk: 100, AvailableDisk: 0},
			eligible: false,
			reason:   "disk exhausted",
		},
		{
			name:     "zero total capacity with zero available is fine",
			node:     store.Node{ActualState: online},
			capacity: store.NodeCapacitySnapshot{TotalCPU: 0, AvailableCPU: 0, TotalMemory: 0, AvailableMemory: 0, TotalDisk: 0, AvailableDisk: 0},
			eligible: true,
			reason:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eligible, reason := PlacementEligibility(tt.node, tt.capacity)
			if eligible != tt.eligible {
				t.Fatalf("eligible = %v, want %v", eligible, tt.eligible)
			}
			if reason != tt.reason {
				t.Fatalf("reason = %q, want %q", reason, tt.reason)
			}
		})
	}
}



func TestHealthSignal(t *testing.T) {
	tests := []struct {
		name  string
		value *int
		want  string
	}{
		{"nil", nil, "unknown"},
		{"zero", ptrInt(0), "unknown"},
		{"positive", ptrInt(100), "ok"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := healthSignal(tt.value)
			if got != tt.want {
				t.Fatalf("healthSignal(%v) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestHealthFromStatus(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{string(domain.NodeStatusOnline), "ok"},
		{string(domain.NodeStatusDegraded), "degraded"},
		{string(domain.NodeStatusMaintenance), "maintenance"},
		{string(domain.NodeStatusDraining), "maintenance"},
		{"unknown", "unknown"},
		{"", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := healthFromStatus(tt.status)
			if got != tt.want {
				t.Fatalf("healthFromStatus(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
	}
}

func TestHealth(t *testing.T) {
	svc := &Service{}

	dockerStatus := "running"
	heartbeatErr := ""
	tests := []struct {
		name string
		node store.Node
		want domain.NodeHealth
	}{
		{
			name: "healthy node",
			node: store.Node{
				DockerStatus: &dockerStatus,
				Status:       string(domain.NodeStatusOnline),
				NodeMemoryMB: ptrInt(16384),
				NodeDiskMB:   ptrInt(102400),
			},
			want: domain.NodeHealth{
				CPU: "unknown",
				Memory: "ok",
				Disk:   "ok",
				Network: "ok",
				Runtime: "running",
			},
		},
		{
			name: "node with heartbeat error",
			node: store.Node{
				DockerStatus: &dockerStatus,
				HeartbeatErr: &heartbeatErr,
				Status:       string(domain.NodeStatusOnline),
			},
			want: domain.NodeHealth{
				CPU:     "unknown",
				Memory:  "unknown",
				Disk:    "unknown",
				Network: "ok",
				Runtime: "running",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.Health(tt.node)
			if got.CPU != tt.want.CPU || got.Memory != tt.want.Memory ||
				got.Disk != tt.want.Disk || got.Network != tt.want.Network {
				t.Fatalf("Health() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCapacity(t *testing.T) {
	svc := &Service{}

	t.Run("uses node fields when no runtime reported", func(t *testing.T) {
		node := store.Node{MemoryMB: 16384, DiskMB: 102400}
		cap := svc.Capacity(node)
		if cap.MemoryMB != 16384 {
			t.Fatalf("memory = %d, want 16384", cap.MemoryMB)
		}
		if cap.DiskMB != 102400 {
			t.Fatalf("disk = %d, want 102400", cap.DiskMB)
		}
	})

	t.Run("runtime reported capacity takes priority", func(t *testing.T) {
		node := store.Node{
			MemoryMB:    16384,
			DiskMB:      102400,
			NodeMemoryMB: ptrInt(32768),
			NodeDiskMB:   ptrInt(204800),
			CPUThreads:   ptrInt(8),
		}
		cap := svc.Capacity(node)
		if cap.MemoryMB != 32768 {
			t.Fatalf("memory = %d, want 32768", cap.MemoryMB)
		}
		if cap.DiskMB != 204800 {
			t.Fatalf("disk = %d, want 204800", cap.DiskMB)
		}
		if cap.CPUThreads != 8 {
			t.Fatalf("cpu threads = %d, want 8", cap.CPUThreads)
		}
	})

	t.Run("zero runtime values do not override", func(t *testing.T) {
		node := store.Node{
			MemoryMB:    16384,
			DiskMB:      102400,
			NodeMemoryMB: ptrInt(0),
			NodeDiskMB:   ptrInt(0),
		}
		cap := svc.Capacity(node)
		if cap.MemoryMB != 16384 {
			t.Fatalf("memory = %d, want 16384", cap.MemoryMB)
		}
		if cap.DiskMB != 102400 {
			t.Fatalf("disk = %d, want 102400", cap.DiskMB)
		}
	})
}

type mockPublisher struct {
	events []events.Envelope
}

func (m *mockPublisher) Publish(_ context.Context, e events.Envelope) error {
	m.events = append(m.events, e)
	return nil
}

func TestPublishActualStateEvents(t *testing.T) {
	online := string(domain.NodeActualStateOnline)
	offline := string(domain.NodeActualStateOffline)
	degraded := string(domain.NodeActualStateDegraded)

	tests := []struct {
		name        string
		beforeState string
		node        store.Node
		wantTypes   []events.EventType
	}{
		{
			name:        "transition to online",
			beforeState: offline,
			node:        store.Node{ID: "n1", ActualState: online},
			wantTypes:   []events.EventType{events.EventActualStateChanged, events.EventNodeOnline},
		},
		{
			name:        "transition to offline",
			beforeState: online,
			node:        store.Node{ID: "n2", ActualState: offline},
			wantTypes:   []events.EventType{events.EventActualStateChanged, events.EventNodeOffline},
		},
		{
			name:        "transition to degraded",
			beforeState: online,
			node:        store.Node{ID: "n3", ActualState: degraded},
			wantTypes:   []events.EventType{events.EventActualStateChanged, events.EventNodeDegraded},
		},
		{
			name:        "transition from empty to online",
			beforeState: "",
			node:        store.Node{ID: "n4", ActualState: online},
			wantTypes:   []events.EventType{events.EventActualStateChanged, events.EventNodeOnline},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp := &mockPublisher{}
			svc := &Service{publisher: mp}

			svc.publishActualStateEvents(context.Background(), tt.beforeState, tt.node)

			if len(mp.events) != len(tt.wantTypes) {
				t.Fatalf("published %d events, want %d", len(mp.events), len(tt.wantTypes))
			}
			for i, want := range tt.wantTypes {
				if mp.events[i].Type != want {
					t.Fatalf("event[%d].Type = %q, want %q", i, mp.events[i].Type, want)
				}
				if mp.events[i].ResourceID != tt.node.ID {
					t.Fatalf("event[%d].ResourceID = %q, want %q", i, mp.events[i].ResourceID, tt.node.ID)
				}
			}
		})
	}
}

func ptr(t time.Time) *time.Time {
	return &t
}

func ptrInt(i int) *int {
	return &i
}
