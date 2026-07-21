//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/services/trafficmanager"
	"gamepanel/forge/internal/store"
)

// TestNodeFailureScenario verifies node failure detection via heartbeat monitor.
func TestNodeFailureScenario(t *testing.T) {
	suite := SetupTestSuite(t)
	if suite == nil {
		return
	}
	defer suite.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	regionID := suite.CreateTestRegion(ctx, t)
	node1 := suite.CreateTestNode(ctx, t, regionID, 4096, 8192, 102400)
	node2 := suite.CreateTestNode(ctx, t, regionID, 4096, 8192, 102400)
	node3 := suite.CreateTestNode(ctx, t, regionID, 4096, 8192, 102400)

	suite.CreateTestServer(ctx, t, "server-on-node1", node1, regionID)
	suite.CreateTestServer(ctx, t, "server-on-node2", node2, regionID)
	suite.CreateTestServer(ctx, t, "server-on-node3", node3, regionID)

	t.Run("Heartbeat monitor detects offline node", func(t *testing.T) {
		suite.Publisher.Reset()

		oldLastSeen := time.Now().UTC().Add(-10 * time.Minute)
		_, err := suite.Store.DB().Exec(ctx,
			`UPDATE nodes SET last_seen_at = $1 WHERE id = $2`,
			oldLastSeen, node2)
		require.NoError(t, err)

		err = suite.HeartbeatMonitor.EvaluateAll(ctx)
		require.NoError(t, err)

		updatedNode, err := suite.Store.GetNode(ctx, node2)
		require.NoError(t, err)
		assert.Equal(t, string(store.NodeHeartbeatStateOffline), updatedNode.HeartbeatState)
		assert.Equal(t, "offline", updatedNode.Status)
		assert.Equal(t, "offline", updatedNode.ActualState)

		events := suite.Publisher.Events()
		hasOfflineEvent := false
		for _, ev := range events {
			if ev.Type == events.EventNodeOffline {
				hasOfflineEvent = true
				break
			}
		}
		assert.True(t, hasOfflineEvent, "expected EventNodeOffline to be published")
	})

	t.Run("Replica instances are replaced after node failure", func(t *testing.T) {
		app := suite.CreateTestApplication(ctx, t, "failure-test-app", 3)
		err := suite.ReplicaMgr.DeployApp(ctx, app.ID)
		require.NoError(t, err)

		instances, err := suite.Store.ListInstancesByApp(ctx, app.ID)
		require.NoError(t, err)
		assert.Len(t, instances, 3)

		var instanceOnNode2 *store.Instance
		for i := range instances {
			if instances[i].NodeID == node2 {
				instanceOnNode2 = &instances[i]
				break
			}
		}

		if instanceOnNode2 != nil {
			_, err = suite.Store.DB().Exec(ctx,
				`UPDATE instances SET status = $1 WHERE id = $2`,
				"failed",
				instanceOnNode2.ID)
			require.NoError(t, err)

			_, err = suite.ReplicaMgr.ReplaceInstance(ctx, instanceOnNode2.ID)
			assert.NoError(t, err)

			newInstances, err := suite.Store.ListInstancesByApp(ctx, app.ID)
			require.NoError(t, err)

			nodeInstanceCount := make(map[string]int)
			for _, inst := range newInstances {
				if inst.Status != "removing" && inst.Status != "failed" {
					nodeInstanceCount[inst.NodeID]++
				}
			}
			assert.Equal(t, 0, nodeInstanceCount[node2])
		}

		activeReservations := 0
		for _, inst := range instances {
			if inst.ReservationID != nil {
				reservation, err := suite.Reservations.GetReservation(ctx, *inst.ReservationID)
				if err == nil && reservation.Status == store.PlacementReservationStatusActive {
					activeReservations++
				}
			}
		}
		assert.LessOrEqual(t, activeReservations, 3)
	})

	t.Run("Node recovery via heartbeat monitor", func(t *testing.T) {
		now := time.Now().UTC()
		_, err := suite.Store.DB().Exec(ctx,
			`UPDATE nodes SET last_seen_at = $1 WHERE id = $2`,
			now, node2)
		require.NoError(t, err)

		suite.AddNodeHeartbeatHistoryForRecovery(ctx, node2, 5)

		suite.Publisher.Reset()
		err = suite.HeartbeatMonitor.EvaluateAll(ctx)
		require.NoError(t, err)

		recoveredNode, err := suite.Store.GetNode(ctx, node2)
		require.NoError(t, err)
		assert.Equal(t, string(store.NodeHeartbeatStateHealthy), recoveredNode.HeartbeatState)
		assert.Equal(t, "online", recoveredNode.Status)
		assert.Equal(t, "online", recoveredNode.ActualState)

		events := suite.Publisher.Events()
		hasRecoveredEvent := false
		for _, ev := range events {
			if ev.Type == events.EventNodeRecovered {
				hasRecoveredEvent = true
				break
			}
		}
		assert.True(t, hasRecoveredEvent, "expected EventNodeRecovered to be published")

		app := suite.CreateTestApplication(ctx, t, "recovery-test-app", 2)
		err = suite.ReplicaMgr.DeployApp(ctx, app.ID)
		require.NoError(t, err)

		err = suite.ReplicaMgr.VerifyNoDuplicates(ctx, app.ID)
		assert.NoError(t, err)
	})

	t.Run("Frontend shows recovery reason", func(t *testing.T) {
		app := suite.CreateTestApplication(ctx, t, "recovery-reason-app", 2)
		err := suite.ReplicaMgr.DeployApp(ctx, app.ID)
		require.NoError(t, err)

		decisions, err := suite.Store.ListPlacementDecisionsByApp(ctx, app.ID)
		require.NoError(t, err)
		assert.Greater(t, len(decisions), 0)
		for _, decision := range decisions {
			assert.NotEmpty(t, decision.Reasons)
		}
	})
}

// TestNodeHeartbeatExpiry tests the heartbeat monitor's state machine transitions.
func TestNodeHeartbeatExpiry(t *testing.T) {
	suite := SetupTestSuite(t)
	if suite == nil {
		return
	}
	defer suite.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	regionID := suite.CreateTestRegion(ctx, t)
	node1 := suite.CreateTestNode(ctx, t, regionID, 4096, 8192, 102400)

	t.Run("Heartbeat timeout detection via EvaluateAll", func(t *testing.T) {
		node, err := suite.Store.GetNode(ctx, node1)
		require.NoError(t, err)
		assert.Equal(t, "online", node.Status)
		assert.Equal(t, "online", node.ActualState)
		assert.Equal(t, string(store.NodeHeartbeatStateHealthy), node.HeartbeatState)

		oldLastSeen := time.Now().UTC().Add(-10 * time.Minute)
		_, err = suite.Store.DB().Exec(ctx,
			`UPDATE nodes SET last_seen_at = $1 WHERE id = $2`,
			oldLastSeen, node1)
		require.NoError(t, err)

		suite.Publisher.Reset()
		err = suite.HeartbeatMonitor.EvaluateAll(ctx)
		require.NoError(t, err)

		updatedNode, err := suite.Store.GetNode(ctx, node1)
		require.NoError(t, err)
		assert.Equal(t, string(store.NodeHeartbeatStateOffline), updatedNode.HeartbeatState)
		assert.Equal(t, "offline", updatedNode.Status)
		assert.Equal(t, "offline", updatedNode.ActualState)

		events := suite.Publisher.Events()
		hasOfflineEvent := false
		for _, ev := range events {
			if ev.Type == events.EventNodeOffline {
				hasOfflineEvent = true
				break
			}
		}
		assert.True(t, hasOfflineEvent, "expected EventNodeOffline to be published")

		servers, err := suite.Store.ListServersForNode(ctx, node1)
		require.NoError(t, err)
		for _, server := range servers {
			assert.NotEmpty(t, server.ID)
		}
	})

	t.Run("Node recovery after reconnect via heartbeat monitor", func(t *testing.T) {
		now := time.Now().UTC()
		_, err := suite.Store.DB().Exec(ctx,
			`UPDATE nodes SET last_seen_at = $1 WHERE id = $2`,
			now, node1)
		require.NoError(t, err)

		suite.AddNodeHeartbeatHistoryForRecovery(ctx, node1, 5)

		suite.Publisher.Reset()
		err = suite.HeartbeatMonitor.EvaluateAll(ctx)
		require.NoError(t, err)

		recoveredNode, err := suite.Store.GetNode(ctx, node1)
		require.NoError(t, err)
		assert.Equal(t, string(store.NodeHeartbeatStateHealthy), recoveredNode.HeartbeatState)
		assert.Equal(t, "online", recoveredNode.Status)
		assert.Equal(t, "online", recoveredNode.ActualState)

		events := suite.Publisher.Events()
		hasRecoveredEvent := false
		for _, ev := range events {
			if ev.Type == events.EventNodeRecovered {
				hasRecoveredEvent = true
				break
			}
		}
		assert.True(t, hasRecoveredEvent, "expected EventNodeRecovered to be published")

		snapshot, err := suite.Store.NodeCapacitySnapshot(ctx, node1)
		require.NoError(t, err)
		assert.Equal(t, node1, snapshot.NodeID)
		assert.Greater(t, snapshot.AvailableCPU, 0)
		assert.Greater(t, snapshot.AvailableMemory, 0)
		assert.Greater(t, snapshot.AvailableDisk, 0)
	})
}

// TestNodeEvacuation tests node maintenance/draining scenarios.
func TestNodeEvacuation(t *testing.T) {
	suite := SetupTestSuite(t)
	if suite == nil {
		return
	}
	defer suite.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	regionID := suite.CreateTestRegion(ctx, t)
	node1 := suite.CreateTestNode(ctx, t, regionID, 4096, 8192, 102400)
	_ = suite.CreateTestNode(ctx, t, regionID, 4096, 8192, 102400)

	for i := 0; i < 3; i++ {
		suite.CreateTestServer(ctx, t, fmt.Sprintf("evac-server-%d", i), node1, regionID)
	}

	t.Run("Node maintenance mode", func(t *testing.T) {
		_, err := suite.Store.DB().Exec(ctx,
			`UPDATE nodes SET desired_state = $1 WHERE id = $2`,
			"maintenance", node1)
		require.NoError(t, err)

		updatedNode, err := suite.Store.GetNode(ctx, node1)
		require.NoError(t, err)
		assert.Equal(t, "maintenance", updatedNode.DesiredState)
	})

	t.Run("Node draining", func(t *testing.T) {
		_, err := suite.Store.DB().Exec(ctx,
			`UPDATE nodes SET desired_state = $1 WHERE id = $2`,
			"draining", node1)
		require.NoError(t, err)

		updatedNode, err := suite.Store.GetNode(ctx, node1)
		require.NoError(t, err)
		assert.Equal(t, "draining", updatedNode.DesiredState)
	})
}

// TestNodeFailure_EventPublication verifies events are published through
// the heartbeat monitor's EvaluateAll path when nodes go offline and recover.
func TestNodeFailure_EventPublication(t *testing.T) {
	suite := SetupTestSuite(t)
	if suite == nil {
		return
	}
	defer suite.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	regionID := suite.CreateTestRegion(ctx, t)
	nodeID := suite.CreateTestNode(ctx, t, regionID, 4096, 8192, 102400)

	t.Run("EventNodeOffline published when heartbeat expires", func(t *testing.T) {
		suite.Publisher.Reset()

		oldLastSeen := time.Now().UTC().Add(-10 * time.Minute)
		_, err := suite.Store.DB().Exec(ctx,
			`UPDATE nodes SET last_seen_at = $1 WHERE id = $2`,
			oldLastSeen, nodeID)
		require.NoError(t, err)

		err = suite.HeartbeatMonitor.EvaluateAll(ctx)
		require.NoError(t, err)

		allEvents := suite.Publisher.Events()
		offlineEvents := filterEventsByType(allEvents, events.EventNodeOffline)
		assert.NotEmpty(t, offlineEvents, "expected at least one EventNodeOffline to be published via EvaluateAll")

		for _, ev := range offlineEvents {
			assert.Equal(t, string(store.NodeHeartbeatStateOffline), ev.Payload["heartbeatState"])
			assert.Equal(t, "offline", ev.Payload["actualState"])
			assert.NotEmpty(t, ev.Payload["reason"])
			assert.NotNil(t, ev.Payload["lastHeartbeatAgeSeconds"])
		}

		actualStateEvents := filterEventsByType(allEvents, events.EventActualStateChanged)
		assert.NotEmpty(t, actualStateEvents, "expected EventActualStateChanged when actual_state transitions to offline")
	})

	t.Run("EventNodeRecovered published when heartbeat resumes", func(t *testing.T) {
		suite.Publisher.Reset()

		now := time.Now().UTC()
		_, err := suite.Store.DB().Exec(ctx,
			`UPDATE nodes SET last_seen_at = $1 WHERE id = $2`,
			now, nodeID)
		require.NoError(t, err)

		suite.AddNodeHeartbeatHistoryForRecovery(ctx, nodeID, 5)

		err = suite.HeartbeatMonitor.EvaluateAll(ctx)
		require.NoError(t, err)

		allEvents := suite.Publisher.Events()
		recoveredEvents := filterEventsByType(allEvents, events.EventNodeRecovered)
		assert.NotEmpty(t, recoveredEvents, "expected at least one EventNodeRecovered to be published via EvaluateAll")

		for _, ev := range recoveredEvents {
			assert.Equal(t, string(store.NodeHeartbeatStateHealthy), ev.Payload["heartbeatState"])
			assert.Equal(t, "online", ev.Payload["actualState"])
			assert.NotEmpty(t, ev.Payload["reason"])
		}
	})

	t.Run("EventActualStateChanged published on state transitions", func(t *testing.T) {
		suite.Publisher.Reset()

		oldLastSeen := time.Now().UTC().Add(-10 * time.Minute)
		_, err := suite.Store.DB().Exec(ctx,
			`UPDATE nodes SET last_seen_at = $1 WHERE id = $2`,
			oldLastSeen, nodeID)
		require.NoError(t, err)

		err = suite.HeartbeatMonitor.EvaluateAll(ctx)
		require.NoError(t, err)

		allEvents := suite.Publisher.Events()
		actualStateEvents := filterEventsByType(allEvents, events.EventActualStateChanged)
		assert.NotEmpty(t, actualStateEvents, "expected EventActualStateChanged when actual_state transitions")
	})

	t.Run("State transitions recorded in database", func(t *testing.T) {
		rows, err := suite.DB.Query(ctx,
			`SELECT to_state FROM state_transitions WHERE resource_type = 'node' AND resource_id = $1 AND state_kind = 'heartbeat' ORDER BY created_at DESC LIMIT 10`,
			nodeID)
		require.NoError(t, err)
		defer rows.Close()

		foundOffline := false
		foundHealthy := false
		for rows.Next() {
			var toState string
			require.NoError(t, rows.Scan(&toState))
			if toState == string(store.NodeHeartbeatStateOffline) {
				foundOffline = true
			}
			if toState == string(store.NodeHeartbeatStateHealthy) {
				foundHealthy = true
			}
		}
		require.NoError(t, rows.Err())
		assert.True(t, foundOffline, "expected a transition to offline state")
		assert.True(t, foundHealthy, "expected a transition to healthy state")
	})
}

// TestNodeStateTransitions verifies the recovering → reconciling transition path.
func TestNodeStateTransitions(t *testing.T) {
	suite := SetupTestSuite(t)
	if suite == nil {
		return
	}
	defer suite.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	regionID := suite.CreateTestRegion(ctx, t)
	nodeID := suite.CreateTestNode(ctx, t, regionID, 4096, 8192, 102400)

	t.Run("Transitions from offline to recovering with partial heartbeats", func(t *testing.T) {
		oldLastSeen := time.Now().UTC().Add(-10 * time.Minute)
		_, err := suite.Store.DB().Exec(ctx,
			`UPDATE nodes SET last_seen_at = $1 WHERE id = $2`,
			oldLastSeen, nodeID)
		require.NoError(t, err)

		err = suite.HeartbeatMonitor.EvaluateAll(ctx)
		require.NoError(t, err)

		offlineNode, err := suite.Store.GetNode(ctx, nodeID)
		require.NoError(t, err)
		require.Equal(t, string(store.NodeHeartbeatStateOffline), offlineNode.HeartbeatState)
	})

	t.Run("Transitions from offline to recovering with one heartbeat", func(t *testing.T) {
		now := time.Now().UTC()
		_, err := suite.Store.DB().Exec(ctx,
			`UPDATE nodes SET last_seen_at = $1 WHERE id = $2`,
			now, nodeID)
		require.NoError(t, err)

		suite.AddNodeHeartbeatHistoryForRecovery(ctx, nodeID, 1)

		suite.Publisher.Reset()
		err = suite.HeartbeatMonitor.EvaluateAll(ctx)
		require.NoError(t, err)

		node, err := suite.Store.GetNode(ctx, nodeID)
		require.NoError(t, err)
		assert.Equal(t, string(store.NodeHeartbeatStateRecovering), node.HeartbeatState)
		assert.Equal(t, "degraded", node.ActualState)
	})

	t.Run("Transitions from recovering to reconciling with enough heartbeats", func(t *testing.T) {
		now := time.Now().UTC()
		_, err := suite.Store.DB().Exec(ctx,
			`UPDATE nodes SET last_seen_at = $1 WHERE id = $2`,
			now, nodeID)
		require.NoError(t, err)

		suite.AddNodeHeartbeatHistoryForRecovery(ctx, nodeID, 1)

		suite.Publisher.Reset()
		err = suite.HeartbeatMonitor.EvaluateAll(ctx)
		require.NoError(t, err)

		node, err := suite.Store.GetNode(ctx, nodeID)
		require.NoError(t, err)
		assert.Equal(t, string(store.NodeHeartbeatStateReconciling), node.HeartbeatState)
		assert.Equal(t, "reconciling", node.ActualState)
	})

	t.Run("Stays in reconciling on subsequent evaluations", func(t *testing.T) {
		now := time.Now().UTC()
		_, err := suite.Store.DB().Exec(ctx,
			`UPDATE nodes SET last_seen_at = $1 WHERE id = $2`,
			now, nodeID)
		require.NoError(t, err)

		suite.Publisher.Reset()
		err = suite.HeartbeatMonitor.EvaluateAll(ctx)
		require.NoError(t, err)

		node, err := suite.Store.GetNode(ctx, nodeID)
		require.NoError(t, err)
		assert.Equal(t, string(store.NodeHeartbeatStateReconciling), node.HeartbeatState)
	})
}

// TestNodeOfflineTrafficWithdrawal verifies traffic manager withdraws routing
// rules for servers on a node that goes offline.
func TestNodeOfflineTrafficWithdrawal(t *testing.T) {
	suite := SetupTestSuite(t)
	if suite == nil {
		return
	}
	defer suite.Cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	regionID := suite.CreateTestRegion(ctx, t)
	nodeID := suite.CreateTestNode(ctx, t, regionID, 4096, 8192, 102400)
	server := suite.CreateTestServer(ctx, t, "traffic-test-server", nodeID, regionID)

	t.Run("WithdrawNodeTargets removes rules for given node", func(t *testing.T) {
		tmSvc := trafficmanager.New(suite.Store, suite.Store, nil, suite.Publisher)
		require.NotNil(t, tmSvc)

		rule := &trafficmanager.RoutingRule{
			ID:         "test-rule-1",
			Name:       "test-rule",
			ServerID:   server.ID,
			Domain:     "example.com",
			TargetPort: 8080,
			Enabled:    true,
		}
		err := tmSvc.CreateRoutingRule(ctx, rule)
		require.NoError(t, err)

		rules, err := tmSvc.ListRoutingRulesByServer(ctx, server.ID)
		require.NoError(t, err)
		assert.Len(t, rules, 1, "should have one rule before withdrawal")

		err = tmSvc.WithdrawNodeTargets(ctx, nodeID)
		require.NoError(t, err)

		rules, err = tmSvc.ListRoutingRulesByServer(ctx, server.ID)
		require.NoError(t, err)
		assert.Empty(t, rules, "rules should be withdrawn after node goes offline")
	})
}

func filterEventsByType(events []events.Envelope, eventType events.EventType) []events.Envelope {
	var filtered []events.Envelope
	for _, ev := range events {
		if ev.Type == eventType {
			filtered = append(filtered, ev)
		}
	}
	return filtered
}
