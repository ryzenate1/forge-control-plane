package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func setupTransferMigration(t *testing.T) (*Store, Migration, string, string) {
	t.Helper()
	s := migrationTestStore(t, false)
	ctx := context.Background()
	if err := s.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	sourceNodeID := "22222222-2222-2222-2222-222222222222"
	serverID := "44444444-4444-4444-4444-444444444444"
	targetNodeID := uuid.NewString()
	targetAllocationID := uuid.NewString()
	if _, err := s.db.Exec(ctx, `INSERT INTO nodes (id,name,region,base_url,token_hash,daemon_token_id,daemon_token,memory_mb,disk_mb)
		VALUES ($1,'transfer target','local','http://target.test','secret','target-token','secret',8192,8192)`, targetNodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO allocations (id,node_id,ip,port) VALUES ($1,$2,'127.0.0.2',25566)`, targetAllocationID, targetNodeID); err != nil {
		t.Fatal(err)
	}
	migration, err := s.CreateMigration(ctx, CreateMigrationRequest{ServerID: serverID, SourceNodeID: sourceNodeID, TargetNodeID: targetNodeID})
	if err != nil {
		t.Fatal(err)
	}
	return s, migration, targetNodeID, targetAllocationID
}

func TestMigrationRunRestartReclaimAndCancellationRelease(t *testing.T) {
	s, migration, targetNodeID, targetAllocationID := setupTransferMigration(t)
	ctx := context.Background()
	run, err := s.EnsureMigrationRun(ctx, migration.ID, "forge-beacon-transfer/v1")
	if err != nil {
		t.Fatal(err)
	}
	if run.TargetAllocationID != targetAllocationID || run.IdempotencyKey == "" {
		t.Fatalf("unexpected run: %+v", run)
	}
	if _, err := s.ClaimMigrationRun(ctx, migration.ID, "dead-worker", time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	ids, err := s.ReclaimableMigrationIDs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != migration.ID {
		t.Fatalf("reclaimable migrations=%v", ids)
	}
	if err := s.CancelMigrationRun(ctx, migration.ID); err != nil {
		t.Fatal(err)
	}
	allocation, err := s.FindAvailableAllocation(ctx, targetNodeID)
	if err != nil {
		t.Fatal(err)
	}
	if allocation.ID != targetAllocationID {
		t.Fatalf("cancel did not release target allocation: %+v", allocation)
	}
}

func TestFinalizeMigrationRequiresDestinationCreatedAndMovesOwnershipAtomically(t *testing.T) {
	s, migration, targetNodeID, targetAllocationID := setupTransferMigration(t)
	ctx := context.Background()
	if _, err := s.EnsureMigrationRun(ctx, migration.ID, "forge-beacon-transfer/v1"); err != nil {
		t.Fatal(err)
	}
	if err := s.FinalizeMigration(ctx, migration.ID); err == nil {
		t.Fatal("finalized before destination creation")
	}
	if nodeID, err := s.ServerNodeID(ctx, migration.ServerID); err != nil || nodeID != migration.SourceNodeID {
		t.Fatalf("failed finalization changed source ownership: node=%s err=%v", nodeID, err)
	}
	if _, err := s.UpdateMigrationStatus(ctx, migration.ID, MigrationStatusTransferring, "test transfer"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateMigrationStatus(ctx, migration.ID, MigrationStatusRestoring, "test restore"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateMigrationRun(ctx, migration.ID, "destination_created", "", 100, "checksum"); err != nil {
		t.Fatal(err)
	}
	if err := s.FinalizeMigration(ctx, migration.ID); err != nil {
		t.Fatal(err)
	}
	if nodeID, err := s.ServerNodeID(ctx, migration.ServerID); err != nil || nodeID != targetNodeID {
		t.Fatalf("ownership not committed: node=%s err=%v", nodeID, err)
	}
	var assignedServer string
	if err := s.db.QueryRow(ctx, `SELECT server_id::text FROM allocations WHERE id=$1`, targetAllocationID).Scan(&assignedServer); err != nil {
		t.Fatal(err)
	}
	if assignedServer != migration.ServerID {
		t.Fatalf("target allocation assigned to %s", assignedServer)
	}
	var sourceAssignments int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM allocations WHERE node_id=$1 AND server_id=$2`, migration.SourceNodeID, migration.ServerID).Scan(&sourceAssignments); err != nil {
		t.Fatal(err)
	}
	if sourceAssignments != 0 {
		t.Fatalf("source allocations retained after commit: %d", sourceAssignments)
	}
	final, err := s.GetMigration(ctx, migration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != string(MigrationStatusCompleted) || !final.CleanupPending {
		t.Fatalf("unexpected finalized migration: %+v", final)
	}
	cleanupIDs, err := s.CleanupPendingMigrationIDs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleanupIDs) != 1 || cleanupIDs[0] != migration.ID {
		t.Fatalf("cleanup pending migrations=%v", cleanupIDs)
	}
	if _, _, err := s.MigrationProvisionTargets(ctx, migration.ID); err != nil {
		t.Fatalf("resolve finalized migration cleanup targets: %v", err)
	}
	if err := s.MarkMigrationCleanupComplete(ctx, migration.ID); err != nil {
		t.Fatal(err)
	}
	final, err = s.GetMigration(ctx, migration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.CleanupPending {
		t.Fatal("cleanup remained pending")
	}
	cleanupIDs, err = s.CleanupPendingMigrationIDs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleanupIDs) != 0 {
		t.Fatalf("cleanup pending migrations=%v", cleanupIDs)
	}
}
