package scheduler

import (
	"testing"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/store"
)

func TestCanonicalStorageLocality(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"local", "local"},
		{"local_only", "local"},
		{"LOCAL_ONLY", "local"},
		{" Local ", "local"},
		{"shared", "shared"},
		{"replicated", "replicated"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := canonicalStorageLocality(tt.input); got != tt.want {
			t.Errorf("canonicalStorageLocality(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestStorageLocalityEqual(t *testing.T) {
	if !storageLocalityEqual("local", "local_only") {
		t.Error("local and local_only should be equal")
	}
	if !storageLocalityEqual("LOCAL", "local_only") {
		t.Error("case insensitive equality failed")
	}
	if storageLocalityEqual("local", "shared") {
		t.Error("local and shared should not be equal")
	}
	if !storageLocalityEqual("shared", "shared") {
		t.Error("shared should equal shared")
	}
}

func TestIsLocalStorageLocality(t *testing.T) {
	if !isLocalStorageLocality("local") {
		t.Error("local should be local")
	}
	if !isLocalStorageLocality("local_only") {
		t.Error("local_only alias should be considered local")
	}
	if isLocalStorageLocality("shared") {
		t.Error("shared should not be local")
	}
	if isLocalStorageLocality("replicated") {
		t.Error("replicated should not be local")
	}
}

func TestNormalizeRequestStorageLocality(t *testing.T) {
	req := domain.PlacementRequest{StorageLocality: "local_only", CPU: 1024, MemoryMB: 2048, DiskMB: 10240}
	got := normalizeRequest(req)
	if got.StorageLocality != "local" {
		t.Errorf("normalizeRequest should canonicalize local_only to local, got %q", got.StorageLocality)
	}
	req2 := domain.PlacementRequest{StorageLocality: " LOCAL ", CPU: 1024}
	got2 := normalizeRequest(req2)
	if got2.StorageLocality != "local" {
		t.Errorf("normalizeRequest should trim and lower, got %q", got2.StorageLocality)
	}
}

func TestNodeToCandidateStorageLocalityVocab(t *testing.T) {
	tests := []struct {
		runtime string
		want    string
	}{
		{"", "local"},
		{"docker", "local"},
		{"local", "local"},
		{"nfs", "shared"},
		{"shared", "shared"},
	}
	for _, tt := range tests {
		snap := store.NodeCapacitySnapshot{
			TotalCPU: 100, TotalMemory: 1000, TotalDisk: 10000,
			AvailableCPU: 50, AvailableMemory: 500, AvailableDisk: 5000,
			AllocatedCPU: 50, AllocatedMemory: 500, AllocatedDisk: 5000,
			ServerCount: 1,
		}
		node := store.Node{RuntimeProvider: tt.runtime}
		cand := nodeToCandidate(snap, node)
		if cand.StorageLocality != tt.want {
			t.Errorf("nodeToCandidate runtime %q => %q, want %q", tt.runtime, cand.StorageLocality, tt.want)
		}
		// Also ensure candidate locality canonical equals want
		if canonicalStorageLocality(cand.StorageLocality) != tt.want {
			t.Errorf("candidate locality canonical mismatch for %q", tt.runtime)
		}
	}
}
