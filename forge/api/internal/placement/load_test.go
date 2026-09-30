package placement

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPlacementLoad_ConcurrentDecisions verifies placement engine remains correct
// and race-free under concurrent load. Deterministic: fixed candidates, no sleeps,
// bounded workers, and strict correctness checks.
func TestPlacementLoad_ConcurrentDecisions(t *testing.T) {
	candidates := []Candidate{
		{NodeID: "node-1", TotalMemory: 16384, TotalCPU: 8000, TotalDisk: 200000, AvailableMemory: 4096, AvailableCPU: 2000, AvailableDisk: 50000, RegionID: "us-east"},
		{NodeID: "node-2", TotalMemory: 16384, TotalCPU: 8000, TotalDisk: 200000, AvailableMemory: 8192, AvailableCPU: 4000, AvailableDisk: 100000, RegionID: "us-east"},
		{NodeID: "node-3", TotalMemory: 16384, TotalCPU: 8000, TotalDisk: 200000, AvailableMemory: 16384, AvailableCPU: 8000, AvailableDisk: 200000, RegionID: "us-west"},
		{NodeID: "node-4", TotalMemory: 16384, TotalCPU: 8000, TotalDisk: 200000, AvailableMemory: 2048, AvailableCPU: 1000, AvailableDisk: 25000, RegionID: "us-east"},
		{NodeID: "node-5", TotalMemory: 16384, TotalCPU: 8000, TotalDisk: 200000, AvailableMemory: 12288, AvailableCPU: 6000, AvailableDisk: 150000, RegionID: "eu-west"},
	}

	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	req := WorkloadRequest{
		CPU:      1000,
		MemoryMB: 2048,
		DiskMB:   10000,
		Constraints: []Constraint{
			{Type: ConstraintRegion, Required: true, Values: []string{"us-east"}},
		},
	}

	const workers = 50
	const iterationsPerWorker = 100

	var wg sync.WaitGroup
	errCh := make(chan string, workers*iterationsPerWorker)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterationsPerWorker; i++ {
				result, err := engine.Place(context.Background(), candidates, req)
				if err != nil {
					errCh <- "place failed: " + err.Error()
					return
				}
				// With us-east constraint and least-loaded scorer, node-2 should win (highest resources among us-east)
				if result.NodeID != "node-2" {
					errCh <- "unexpected placement: got " + result.NodeID + " want node-2"
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for e := range errCh {
		t.Fatalf("placement load failed: %s", e)
	}
}

// TestPlacementLoad_ConcurrentPlaceAll verifies PlaceAll ordering remains deterministic
// under concurrent access.
func TestPlacementLoad_ConcurrentPlaceAll(t *testing.T) {
	candidates := []Candidate{
		{NodeID: "node-a", TotalMemory: 1000, TotalCPU: 100, TotalDisk: 10000, AvailableMemory: 100, AvailableCPU: 10, AvailableDisk: 1000},
		{NodeID: "node-b", TotalMemory: 1000, TotalCPU: 100, TotalDisk: 10000, AvailableMemory: 300, AvailableCPU: 30, AvailableDisk: 3000},
		{NodeID: "node-c", TotalMemory: 1000, TotalCPU: 100, TotalDisk: 10000, AvailableMemory: 200, AvailableCPU: 20, AvailableDisk: 2000},
	}
	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())

	const workers = 20
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				results, err := engine.PlaceAll(context.Background(), candidates, WorkloadRequest{})
				require.NoError(t, err)
				require.Len(t, results, 3)
				// Must be sorted descending by score
				if results[0].NodeID != "node-b" || results[1].NodeID != "node-c" || results[2].NodeID != "node-a" {
					t.Errorf("unexpected order: %v %v %v", results[0].NodeID, results[1].NodeID, results[2].NodeID)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// TestPlacementLoad_WithConstraints stresses constraint filtering under load.
func TestPlacementLoad_WithConstraints(t *testing.T) {
	candidates := []Candidate{
		{NodeID: "node-1", RegionID: "us-east", TotalMemory: 16384, TotalCPU: 8000, TotalDisk: 200000, AvailableMemory: 8192, AvailableCPU: 4000, AvailableDisk: 100000},
		{NodeID: "node-2", RegionID: "us-west", TotalMemory: 16384, TotalCPU: 8000, TotalDisk: 200000, AvailableMemory: 16384, AvailableCPU: 8000, AvailableDisk: 200000},
		{NodeID: "node-3", RegionID: "us-east", TotalMemory: 16384, TotalCPU: 8000, TotalDisk: 200000, AvailableMemory: 4096, AvailableCPU: 2000, AvailableDisk: 50000},
	}
	engine := NewEngine(&LeastLoadedScorer{}, NewConstraintChecker())
	constraints := []Constraint{
		{Type: ConstraintRegion, Required: true, Values: []string{"us-east"}},
	}

	const workers = 30
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				res, err := engine.Place(context.Background(), candidates, WorkloadRequest{Constraints: constraints})
				require.NoError(t, err)
				if res.NodeID != "node-1" && res.NodeID != "node-3" {
					t.Errorf("filtered incorrectly, got %s", res.NodeID)
					return
				}
				// node-1 should win (more memory than node-3)
				if res.NodeID != "node-1" {
					t.Errorf("expected node-1 to win among us-east, got %s", res.NodeID)
					return
				}
			}
		}()
	}
	wg.Wait()
}
