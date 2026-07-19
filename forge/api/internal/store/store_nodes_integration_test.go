package store

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestNodeListAndLocationRegionCounts(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	locationID := uuid.NewString()
	regionID := uuid.NewString()
	nodeID := uuid.NewString()
	if _, err := s.db.Exec(ctx, `INSERT INTO locations (id, short, long) VALUES ($1, 'test', 'Test Location')`, locationID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO regions (id, uuid, name, slug) VALUES ($1, $1, 'Test Region', 'test-region')`, regionID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO nodes (id, name, region, base_url, token_hash, location_id, region_id)
		VALUES ($1, 'Test Node', 'test', 'https://node.example.test', 'hash', $2, $3)
	`, nodeID, locationID, regionID); err != nil {
		t.Fatal(err)
	}

	nodes, err := s.ListNodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 1 || nodes[0].ID != nodeID || nodes[0].HeartbeatState != string(NodeHeartbeatStateOffline) {
		t.Fatalf("unexpected nodes: %+v", nodes)
	}
	if _, err := s.GetNode(ctx, nodeID); err != nil {
		t.Fatal(err)
	}

	location, err := s.GetLocation(ctx, locationID)
	if err != nil {
		t.Fatal(err)
	}
	if location.NodeCount != 1 || location.ServerCount != 0 {
		t.Fatalf("unexpected location counts: %+v", location)
	}

	region, err := s.GetRegion(ctx, regionID)
	if err != nil {
		t.Fatal(err)
	}
	if region.NodeCount != 1 {
		t.Fatalf("unexpected region node count: %+v", region)
	}
}
