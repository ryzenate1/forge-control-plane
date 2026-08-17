package loadbalancer

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLoadBalancerLoad_ConcurrentRoundRobin verifies round-robin remains correct and
// race-free under concurrent load. Non-flaky: deterministic algorithm, no sleeps, bounded.
func TestLoadBalancerLoad_ConcurrentRoundRobin(t *testing.T) {
	svc := New(nil)
	ctx := context.Background()

	group := &TargetGroup{
		ID:        "group-rr",
		Name:      "round-robin-group",
		Algorithm: AlgorithmRoundRobin,
		Port:      30000,
		Protocol:  "tcp",
		Targets: []Target{
			{ID: "t1", ServerID: "s1", NodeID: "n1", IP: "10.0.0.1", Port: 8080, Weight: 1, Status: TargetStatusHealthy},
			{ID: "t2", ServerID: "s2", NodeID: "n2", IP: "10.0.0.2", Port: 8080, Weight: 1, Status: TargetStatusHealthy},
			{ID: "t3", ServerID: "s3", NodeID: "n3", IP: "10.0.0.3", Port: 8080, Weight: 1, Status: TargetStatusHealthy},
		},
	}
	require.NoError(t, svc.CreateTargetGroup(ctx, group))

	const workers = 30
	const reqPerWorker = 100
	// Count distribution across workers; round-robin should be roughly even and never fail
	var mu sync.Mutex
	counts := map[string]int{"t1": 0, "t2": 0, "t3": 0}
	var wg sync.WaitGroup
	errCh := make(chan error, workers*reqPerWorker)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < reqPerWorker; i++ {
				target, err := svc.NextTarget(ctx, "group-rr", "1.2.3.4")
				if err != nil {
					errCh <- err
					return
				}
				mu.Lock()
				counts[target.ID]++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("NextTarget failed: %v", err)
	}

	total := counts["t1"] + counts["t2"] + counts["t3"]
	expected := workers * reqPerWorker
	if total != expected {
		t.Fatalf("total requests mismatch: got %d want %d", total, expected)
	}
	// Each target should have been hit at least 20% of total (allow some skew due to concurrency but deterministic round-robin under lock)
	for id, c := range counts {
		if c < expected/5 {
			t.Fatalf("target %s starved: count %d < %d", id, c, expected/5)
		}
	}
}

// TestLoadBalancerLoad_LeastConnections verifies least-connections under concurrent acquire/release.
func TestLoadBalancerLoad_LeastConnections(t *testing.T) {
	svc := New(nil)
	ctx := context.Background()

	group := &TargetGroup{
		ID:        "group-lc",
		Name:      "least-conn-group",
		Algorithm: AlgorithmLeastConnections,
		Port:      30001,
		Protocol:  "tcp",
		Targets: []Target{
			{ID: "lc1", ServerID: "s1", NodeID: "n1", IP: "10.0.0.1", Port: 8080, Weight: 1, Status: TargetStatusHealthy},
			{ID: "lc2", ServerID: "s2", NodeID: "n2", IP: "10.0.0.2", Port: 8080, Weight: 1, Status: TargetStatusHealthy},
		},
	}
	require.NoError(t, svc.CreateTargetGroup(ctx, group))

	const workers = 20
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				target, err := svc.NextTarget(ctx, "group-lc", "")
				require.NoError(t, err)
				require.NotNil(t, target)
				// Simulate immediate release to keep counts balanced; deterministic
				svc.ReleaseConnection(target.ID)
			}
		}()
	}
	wg.Wait()

	// After all releases, conn counts should be 0 (or very low)
	svc.mu.RLock()
	for id, c := range svc.connCount {
		if c != 0 {
			t.Fatalf("conn count for %s not zero after releases: %d", id, c)
		}
	}
	svc.mu.RUnlock()
}

// TestLoadBalancerLoad_IPHash verifies ip_hash is deterministic and race-free under concurrency.
func TestLoadBalancerLoad_IPHash(t *testing.T) {
	svc := New(nil)
	ctx := context.Background()

	group := &TargetGroup{
		ID:        "group-hash",
		Name:      "ip-hash-group",
		Algorithm: AlgorithmIPHash,
		Port:      30002,
		Protocol:  "tcp",
		Targets: []Target{
			{ID: "h1", ServerID: "s1", NodeID: "n1", IP: "10.0.0.1", Port: 8080, Weight: 1, Status: TargetStatusHealthy},
			{ID: "h2", ServerID: "s2", NodeID: "n2", IP: "10.0.0.2", Port: 8080, Weight: 1, Status: TargetStatusHealthy},
			{ID: "h3", ServerID: "s3", NodeID: "n3", IP: "10.0.0.3", Port: 8080, Weight: 1, Status: TargetStatusHealthy},
		},
	}
	require.NoError(t, svc.CreateTargetGroup(ctx, group))

	ips := []string{"192.168.1.1", "10.0.0.5", "172.16.0.10", "8.8.8.8", "1.1.1.1"}

	const workers = 25
	var wg sync.WaitGroup
	errCh := make(chan string, workers*len(ips))

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, ip := range ips {
				// Same IP must always map to same target (deterministic)
				t1, _ := svc.NextTarget(ctx, "group-hash", ip)
				t2, _ := svc.NextTarget(ctx, "group-hash", ip)
				if t1.ID != t2.ID {
					errCh <- "ip hash not deterministic for " + ip + ": " + t1.ID + " vs " + t2.ID
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for e := range errCh {
		t.Fatalf("ip hash load failed: %s", e)
	}
}

// TestLoadBalancerLoad_MixedAlgorithms runs all algorithms concurrently to ensure no cross-group interference.
func TestLoadBalancerLoad_MixedAlgorithms(t *testing.T) {
	svc := New(nil)
	ctx := context.Background()

	groups := []*TargetGroup{
		{ID: "g-rr", Name: "g-rr", Algorithm: AlgorithmRoundRobin, Port: 30010, Protocol: "tcp", Targets: []Target{{ID: "r1", ServerID: "s1", NodeID: "n1", IP: "10.0.0.1", Port: 8080, Status: TargetStatusHealthy}, {ID: "r2", ServerID: "s2", NodeID: "n2", IP: "10.0.0.2", Port: 8080, Status: TargetStatusHealthy}}},
		{ID: "g-lc", Name: "g-lc", Algorithm: AlgorithmLeastConnections, Port: 30011, Protocol: "tcp", Targets: []Target{{ID: "l1", ServerID: "s1", NodeID: "n1", IP: "10.0.0.1", Port: 8080, Status: TargetStatusHealthy}}},
		{ID: "g-hash", Name: "g-hash", Algorithm: AlgorithmIPHash, Port: 30012, Protocol: "tcp", Targets: []Target{{ID: "h1", ServerID: "s1", NodeID: "n1", IP: "10.0.0.1", Port: 8080, Status: TargetStatusHealthy}, {ID: "h2", ServerID: "s2", NodeID: "n2", IP: "10.0.0.2", Port: 8080, Status: TargetStatusHealthy}}},
	}
	for _, g := range groups {
		require.NoError(t, svc.CreateTargetGroup(ctx, g))
	}

	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				_, err := svc.NextTarget(ctx, "g-rr", "1.1.1.1")
				require.NoError(t, err)
				_, err = svc.NextTarget(ctx, "g-lc", "")
				require.NoError(t, err)
				// Release for LC
				svc.ReleaseConnection("l1")
				_, err = svc.NextTarget(ctx, "g-hash", "2.2.2.2")
				require.NoError(t, err)
			}
		}(i)
	}
	wg.Wait()
}
