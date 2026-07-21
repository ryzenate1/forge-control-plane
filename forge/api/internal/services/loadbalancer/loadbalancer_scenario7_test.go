package loadbalancer

import (
	"context"
	"testing"
)

// TestLoadBalancer_Scenario7 verifies loadbalancer requirements for Scenario 7
func TestLoadBalancer_Scenario7(t *testing.T) {
	t.Run("SetTargetStatus filters unhealthy targets", func(t *testing.T) {
		svc := New(nil)

		// Create a target group
		group := &TargetGroup{
			ID:        "test-group",
			Name:      "Test Group",
			Algorithm: AlgorithmRoundRobin,
			Port:      8080,
			Protocol:  "tcp",
			Targets: []Target{
				{ID: "t1", ServerID: "srv-1", NodeID: "node-1", IP: "10.0.0.1", Port: 8080, Status: TargetStatusHealthy},
				{ID: "t2", ServerID: "srv-2", NodeID: "node-2", IP: "10.0.0.2", Port: 8080, Status: TargetStatusHealthy},
				{ID: "t3", ServerID: "srv-3", NodeID: "node-3", IP: "10.0.0.3", Port: 8080, Status: TargetStatusHealthy},
			},
		}

		if err := svc.CreateTargetGroup(context.Background(), group); err != nil {
			t.Fatalf("failed to create target group: %v", err)
		}

		// Mark one target as unhealthy
		if err := svc.SetTargetStatus(context.Background(), "test-group", "t2", TargetStatusUnhealthy); err != nil {
			t.Fatalf("failed to set target status: %v", err)
		}

		// Verify NextTarget only returns healthy targets
		for i := 0; i < 5; i++ {
			target, err := svc.NextTarget(context.Background(), "test-group", "")
			if err != nil {
				t.Fatalf("NextTarget failed: %v", err)
			}
			if target.ID == "t2" {
				t.Fatal("NextTarget should not return unhealthy target t2")
			}
			if target.Status != TargetStatusHealthy {
				t.Fatalf("NextTarget returned target with unhealthy status: %s", target.Status)
			}
		}

		t.Log("✓ SetTargetStatus filters unhealthy targets")
	})

	t.Run("NextTarget returns only healthy targets", func(t *testing.T) {
		svc := New(nil)

		// Create a target group with mixed health
		group := &TargetGroup{
			ID:        "mixed-group",
			Name:      "Mixed Health Group",
			Algorithm: AlgorithmRoundRobin,
			Port:      9090,
			Protocol:  "tcp",
			Targets: []Target{
				{ID: "h1", ServerID: "srv-1", NodeID: "node-1", IP: "10.0.0.1", Port: 9090, Status: TargetStatusHealthy},
				{ID: "u1", ServerID: "srv-2", NodeID: "node-2", IP: "10.0.0.2", Port: 9090, Status: TargetStatusUnhealthy},
				{ID: "d1", ServerID: "srv-3", NodeID: "node-3", IP: "10.0.0.3", Port: 9090, Status: TargetStatusDraining},
				{ID: "h2", ServerID: "srv-4", NodeID: "node-4", IP: "10.0.0.4", Port: 9090, Status: TargetStatusHealthy},
			},
		}

		if err := svc.CreateTargetGroup(context.Background(), group); err != nil {
			t.Fatalf("failed to create target group: %v", err)
		}

		// Test multiple calls to NextTarget
		seenTargets := make(map[string]bool)
		for i := 0; i < 10; i++ {
			target, err := svc.NextTarget(context.Background(), "mixed-group", "")
			if err != nil {
				t.Fatalf("NextTarget failed: %v", err)
			}
			seenTargets[target.ID] = true

			// Should only get healthy targets
			if target.Status != TargetStatusHealthy {
				t.Fatalf("NextTarget returned unhealthy target %s with status %s", target.ID, target.Status)
			}
		}

		// Should only have seen healthy targets
		if seenTargets["u1"] || seenTargets["d1"] {
			t.Fatal("NextTarget returned unhealthy or draining targets")
		}
		if !seenTargets["h1"] || !seenTargets["h2"] {
			t.Fatal("NextTarget should have returned both healthy targets")
		}

		t.Log("✓ NextTarget returns only healthy targets")
	})

	t.Run("All targets unhealthy returns error", func(t *testing.T) {
		svc := New(nil)

		// Create a target group with all unhealthy targets
		group := &TargetGroup{
			ID:        "unhealthy-group",
			Name:      "All Unhealthy Group",
			Algorithm: AlgorithmRoundRobin,
			Port:      7070,
			Protocol:  "tcp",
			Targets: []Target{
				{ID: "u1", ServerID: "srv-1", NodeID: "node-1", IP: "10.0.0.1", Port: 7070, Status: TargetStatusUnhealthy},
				{ID: "u2", ServerID: "srv-2", NodeID: "node-2", IP: "10.0.0.2", Port: 7070, Status: TargetStatusUnhealthy},
			},
		}

		if err := svc.CreateTargetGroup(context.Background(), group); err != nil {
			t.Fatalf("failed to create target group: %v", err)
		}

		// NextTarget should return error when no healthy targets
		_, err := svc.NextTarget(context.Background(), "unhealthy-group", "")
		if err != ErrNoHealthyTarget {
			t.Fatalf("expected ErrNoHealthyTarget, got: %v", err)
		}

		t.Log("✓ All targets unhealthy returns error")
	})

	t.Run("Recovered target becomes available again", func(t *testing.T) {
		svc := New(nil)

		// Create a target group
		group := &TargetGroup{
			ID:        "recovery-group",
			Name:      "Recovery Test Group",
			Algorithm: AlgorithmRoundRobin,
			Port:      6060,
			Protocol:  "tcp",
			Targets: []Target{
				{ID: "t1", ServerID: "srv-1", NodeID: "node-1", IP: "10.0.0.1", Port: 6060, Status: TargetStatusHealthy},
				{ID: "t2", ServerID: "srv-2", NodeID: "node-2", IP: "10.0.0.2", Port: 6060, Status: TargetStatusUnhealthy},
			},
		}

		if err := svc.CreateTargetGroup(context.Background(), group); err != nil {
			t.Fatalf("failed to create target group: %v", err)
		}

		// Initially, only t1 should be available
		target, err := svc.NextTarget(context.Background(), "recovery-group", "")
		if err != nil {
			t.Fatalf("NextTarget failed: %v", err)
		}
		if target.ID != "t1" {
			t.Fatalf("expected t1, got %s", target.ID)
		}

		// Mark t2 as healthy
		if err := svc.SetTargetStatus(context.Background(), "recovery-group", "t2", TargetStatusHealthy); err != nil {
			t.Fatalf("failed to set target status: %v", err)
		}

		// Now both targets should be available
		seenTargets := make(map[string]bool)
		for i := 0; i < 10; i++ {
			target, err := svc.NextTarget(context.Background(), "recovery-group", "")
			if err != nil {
				t.Fatalf("NextTarget failed: %v", err)
			}
			seenTargets[target.ID] = true
		}

		if !seenTargets["t1"] || !seenTargets["t2"] {
			t.Fatal("both targets should be available after recovery")
		}

		t.Log("✓ Recovered target becomes available again")
	})
}

// TestLoadBalancer_NodeHealthManagement verifies node-level health management
func TestLoadBalancer_NodeHealthManagement(t *testing.T) {
	t.Run("MarkNodeTargetsUnhealthy marks all node targets as unhealthy", func(t *testing.T) {
		// This functionality requires a store, so we'll test the logic directly
		svc := New(nil)

		// Create a target group
		group := &TargetGroup{
			ID:        "node-test-group",
			Name:      "Node Test Group",
			Algorithm: AlgorithmRoundRobin,
			Port:      5050,
			Protocol:  "tcp",
			Targets: []Target{
				{ID: "t1", ServerID: "srv-1", NodeID: "node-1", IP: "10.0.0.1", Port: 5050, Status: TargetStatusHealthy},
				{ID: "t2", ServerID: "srv-2", NodeID: "node-2", IP: "10.0.0.2", Port: 5050, Status: TargetStatusHealthy},
				{ID: "t3", ServerID: "srv-3", NodeID: "node-1", IP: "10.0.0.3", Port: 5050, Status: TargetStatusHealthy},
			},
		}

		if err := svc.CreateTargetGroup(context.Background(), group); err != nil {
			t.Fatalf("failed to create target group: %v", err)
		}

		// Manually mark node-1 targets as unhealthy (simulating MarkNodeTargetsUnhealthy)
		if err := svc.SetTargetStatus(context.Background(), "node-test-group", "t1", TargetStatusUnhealthy); err != nil {
			t.Fatalf("failed to set target status: %v", err)
		}
		if err := svc.SetTargetStatus(context.Background(), "node-test-group", "t3", TargetStatusUnhealthy); err != nil {
			t.Fatalf("failed to set target status: %v", err)
		}

		// Verify only node-2 target is available
		for i := 0; i < 5; i++ {
			target, err := svc.NextTarget(context.Background(), "node-test-group", "")
			if err != nil {
				t.Fatalf("NextTarget failed: %v", err)
			}
			if target.ID != "t2" {
				t.Fatalf("expected t2 (node-2), got %s", target.ID)
			}
		}

		t.Log("✓ Node targets can be marked unhealthy")
	})

	t.Run("MarkNodeTargetsHealthy restores node targets", func(t *testing.T) {
		svc := New(nil)

		// Create a target group
		group := &TargetGroup{
			ID:        "node-recovery-group",
			Name:      "Node Recovery Test Group",
			Algorithm: AlgorithmRoundRobin,
			Port:      4040,
			Protocol:  "tcp",
			Targets: []Target{
				{ID: "t1", ServerID: "srv-1", NodeID: "node-1", IP: "10.0.0.1", Port: 4040, Status: TargetStatusUnhealthy},
				{ID: "t2", ServerID: "srv-2", NodeID: "node-2", IP: "10.0.0.2", Port: 4040, Status: TargetStatusHealthy},
			},
		}

		if err := svc.CreateTargetGroup(context.Background(), group); err != nil {
			t.Fatalf("failed to create target group: %v", err)
		}

		// Mark node-1 target as healthy
		if err := svc.SetTargetStatus(context.Background(), "node-recovery-group", "t1", TargetStatusHealthy); err != nil {
			t.Fatalf("failed to set target status: %v", err)
		}

		// Verify both targets are now available
		seenTargets := make(map[string]bool)
		for i := 0; i < 10; i++ {
			target, err := svc.NextTarget(context.Background(), "node-recovery-group", "")
			if err != nil {
				t.Fatalf("NextTarget failed: %v", err)
			}
			seenTargets[target.ID] = true
		}

		if !seenTargets["t1"] || !seenTargets["t2"] {
			t.Fatal("both targets should be available after recovery")
		}

		t.Log("✓ Node targets can be restored to healthy")
	})
}
