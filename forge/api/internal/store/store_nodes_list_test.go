package store

import (
	"context"
	"testing"
)

// The admin Nodes list (GET /nodes) must expose the same heartbeat evidence
// as the detail view (GET /nodes/:id): runtime provider/status, version and a
// populated last-heartbeat timestamp. The list and detail queries are separate
// SELECTs, so a column added to one and forgotten in the other silently
// renders as "unknown" in the UI with no error anywhere.
func TestNodeListAndGetExposeHeartbeatRuntime(t *testing.T) {
	ctx := context.Background()
	s := setupTestStore(t, ctx)
	nodeID := createTestNode(t, ctx, s, "hb-rt")

	if _, err := s.UpdateNodeHeartbeat(ctx, nodeID, NodeHeartbeatRequest{
		Version:         "beacon-dev",
		OS:              "darwin",
		Architecture:    "arm64",
		CPUThreads:      10,
		RuntimeStatus:   "error",
		RuntimeProvider: "docker",
		Error:           "docker runtime unavailable",
	}); err != nil {
		t.Fatalf("update heartbeat: %v", err)
	}

	assertHeartbeatVisible := func(source string, node Node) {
		t.Helper()
		if node.RuntimeProvider != "docker" {
			t.Fatalf("%s: runtimeProvider = %q, want %q", source, node.RuntimeProvider, "docker")
		}
		if node.RuntimeStatus == nil || *node.RuntimeStatus != "error" {
			t.Fatalf("%s: runtimeStatus = %v, want %q", source, node.RuntimeStatus, "error")
		}
		if node.LastSeenAt == nil {
			t.Fatalf("%s: lastSeenAt is nil after heartbeat", source)
		}
		if node.LastHeartbeatAt.IsZero() {
			t.Fatalf("%s: lastHeartbeatAt is zero after heartbeat", source)
		}
		if !node.LastHeartbeatAt.Equal(*node.LastSeenAt) {
			t.Fatalf("%s: lastHeartbeatAt (%v) != lastSeenAt (%v)", source, node.LastHeartbeatAt, *node.LastSeenAt)
		}
		if node.Version == nil || *node.Version != "beacon-dev" {
			t.Fatalf("%s: version = %v, want beacon-dev", source, node.Version)
		}
	}

	nodes, _, err := s.ListNodesPaginated(ctx, 0, 50)
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	var listed *Node
	for i := range nodes {
		if nodes[i].ID == nodeID {
			listed = &nodes[i]
			break
		}
	}
	if listed == nil {
		t.Fatalf("created node %s missing from list", nodeID)
	}
	assertHeartbeatVisible("list", *listed)

	got, err := s.GetNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	assertHeartbeatVisible("get", got)
}
