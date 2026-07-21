package scheduler

import (
	"testing"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/store"
)

func TestNodeRegionEnabled(t *testing.T) {
	enabledID := "enabled-region"
	disabledID := "disabled-region"
	unknownID := "unknown-region"
	regions := []store.Region{
		{ID: enabledID, Enabled: true},
		{ID: disabledID, Enabled: false},
	}

	tests := []struct {
		name string
		node store.Node
		want bool
	}{
		{name: "enabled region", node: store.Node{RegionID: &enabledID}, want: true},
		{name: "disabled region", node: store.Node{RegionID: &disabledID}, want: false},
		{name: "unassigned region", node: store.Node{}, want: true},
		{name: "unknown region", node: store.Node{RegionID: &unknownID}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nodeRegionEnabled(tt.node, regions); got != tt.want {
				t.Fatalf("nodeRegionEnabled() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestNormalizeRequest(t *testing.T) {
	tests := []struct {
		name  string
		input domain.PlacementRequest
		want  domain.PlacementRequest
	}{
		{
			name:  "defaults cpu from cpushares",
			input: domain.PlacementRequest{CPUShares: 512},
			want:  domain.PlacementRequest{CPUShares: 512, CPU: 512, MemoryMB: 2048, DiskMB: 10240},
		},
		{
			name:  "defaults cpu to 1024",
			input: domain.PlacementRequest{},
			want:  domain.PlacementRequest{CPU: 1024, MemoryMB: 2048, DiskMB: 10240},
		},
		{
			name:  "cpu takes precedence over cpushares",
			input: domain.PlacementRequest{CPU: 2048, CPUShares: 512},
			want:  domain.PlacementRequest{CPU: 2048, CPUShares: 512, MemoryMB: 2048, DiskMB: 10240},
		},
		{
			name:  "regionid falls back to region",
			input: domain.PlacementRequest{Region: "us-west"},
			want:  domain.PlacementRequest{RegionID: "us-west", Region: "us-west", CPU: 1024, MemoryMB: 2048, DiskMB: 10240},
		},
		{
			name:  "regionid takes precedence over region",
			input: domain.PlacementRequest{RegionID: "us-east", Region: "us-west"},
			want:  domain.PlacementRequest{RegionID: "us-east", Region: "us-west", CPU: 1024, MemoryMB: 2048, DiskMB: 10240},
		},
		{
			name:  "requirednode falls back to nodeid",
			input: domain.PlacementRequest{NodeID: "node-1"},
			want:  domain.PlacementRequest{RequiredNode: "node-1", NodeID: "node-1", CPU: 1024, MemoryMB: 2048, DiskMB: 10240},
		},
		{
			name:  "requirednode takes precedence over nodeid",
			input: domain.PlacementRequest{RequiredNode: "primary", NodeID: "fallback"},
			want:  domain.PlacementRequest{RequiredNode: "primary", NodeID: "fallback", CPU: 1024, MemoryMB: 2048, DiskMB: 10240},
		},
		{
			name:  "trims whitespace from string fields",
			input: domain.PlacementRequest{RegionID: "  us-east  ", RequiredNode: "  node-1  ", Region: "  us-west  "},
			want:  domain.PlacementRequest{RegionID: "us-east", RequiredNode: "node-1", Region: "  us-west  ", CPU: 1024, MemoryMB: 2048, DiskMB: 10240},
		},
		{
			name:  "preserves all explicit values",
			input: domain.PlacementRequest{RegionID: "eu", CPU: 4096, MemoryMB: 8192, DiskMB: 50000, AllocationID: "alloc-1", PreferredNode: "pref-1"},
			want:  domain.PlacementRequest{RegionID: "eu", CPU: 4096, MemoryMB: 8192, DiskMB: 50000, AllocationID: "alloc-1", PreferredNode: "pref-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeRequest(tt.input)
			if got != tt.want {
				t.Fatalf("normalizeRequest() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestHasCapacity(t *testing.T) {
	tests := []struct {
		name              string
		total, available, requested int
		want              bool
	}{
		{name: "sufficient capacity", total: 100, available: 80, requested: 50, want: true},
		{name: "exact capacity", total: 100, available: 50, requested: 50, want: true},
		{name: "insufficient capacity", total: 100, available: 30, requested: 50, want: false},
		{name: "zero requested", total: 100, available: 0, requested: 0, want: true},
		{name: "negative requested", total: 100, available: 50, requested: -1, want: true},
		{name: "zero total", total: 0, available: 50, requested: 50, want: true},
		{name: "negative total", total: -1, available: 50, requested: 50, want: true},
		{name: "zero available", total: 100, available: 0, requested: 10, want: false},
		{name: "all zero", total: 0, available: 0, requested: 0, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasCapacity(tt.total, tt.available, tt.requested); got != tt.want {
				t.Fatalf("hasCapacity(%d, %d, %d) = %t, want %t", tt.total, tt.available, tt.requested, got, tt.want)
			}
		})
	}
}

func TestWithReservations(t *testing.T) {
	s := New(nil, nil)
	if s.reservations != nil {
		t.Fatal("reservations should be nil by default")
	}
	s.WithReservations(nil)
	if s.reservations != nil {
		t.Fatal("WithReservations(nil) should set field to nil")
	}
}

func TestNormalizeRequestPReservesSkipReservation(t *testing.T) {
	req := domain.PlacementRequest{SkipReservation: true, CPU: 1024}
	got := normalizeRequest(req)
	if !got.SkipReservation {
		t.Fatal("normalizeRequest should preserve SkipReservation flag")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "first non-empty in middle", values: []string{"", "hello", "world"}, want: "hello"},
		{name: "all empty", values: []string{"", "", ""}, want: ""},
		{name: "first non-empty at start", values: []string{"first", "second"}, want: "first"},
		{name: "whitespace only treated as empty", values: []string{"  ", "actual"}, want: "actual"},
		{name: "single value", values: []string{"only"}, want: "only"},
		{name: "empty slice", values: []string{}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstNonEmpty(tt.values...); got != tt.want {
				t.Fatalf("firstNonEmpty(%v) = %q, want %q", tt.values, got, tt.want)
			}
		})
	}
}
