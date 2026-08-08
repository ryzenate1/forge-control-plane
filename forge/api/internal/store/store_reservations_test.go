//go:build integration

package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCreatePlacementReservation(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	nodeID, _, _ := setupTestNode(t, s)
	reservation, err := s.CreatePlacementReservation(ctx, CreatePlacementReservationRequest{
		NodeID: nodeID,
		CPU:    512,
		Memory: 1024,
		Disk:   5120,
	})
	if err != nil {
		t.Fatal(err)
	}
	if reservation.ID == "" {
		t.Fatal("expected non-empty reservation ID")
	}
	if reservation.NodeID != nodeID {
		t.Fatalf("expected node %s, got %s", nodeID, reservation.NodeID)
	}
	if reservation.Status != PlacementReservationStatusActive {
		t.Fatalf("expected status active, got %s", reservation.Status)
	}
	if reservation.ReservationType != string(PlacementReservationTypePlacement) {
		t.Fatalf("expected type placement, got %s", reservation.ReservationType)
	}
	if reservation.CPU != 512 {
		t.Fatalf("expected CPU 512, got %d", reservation.CPU)
	}
	if reservation.Memory != 1024 {
		t.Fatalf("expected memory 1024, got %d", reservation.Memory)
	}
	if reservation.Disk != 5120 {
		t.Fatalf("expected disk 5120, got %d", reservation.Disk)
	}
}

func TestCreatePlacementReservationRequiresNodeID(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	_, err := s.CreatePlacementReservation(ctx, CreatePlacementReservationRequest{
		CPU: 512,
	})
	if err == nil {
		t.Fatal("expected error for missing nodeID")
	}
}

func TestCreatePlacementReservationNodeNotFound(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	_, err := s.CreatePlacementReservation(ctx, CreatePlacementReservationRequest{
		NodeID: uuid.NewString(),
		CPU:    512,
		Memory: 1024,
		Disk:   5120,
	})
	if err == nil {
		t.Fatal("expected error for non-existent node")
	}
}

func TestGetPlacementReservation(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	nodeID, _, _ := setupTestNode(t, s)
	created, err := s.CreatePlacementReservation(ctx, CreatePlacementReservationRequest{
		NodeID: nodeID,
		CPU:    256,
		Memory: 512,
		Disk:   2048,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.GetPlacementReservation(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != created.ID {
		t.Fatalf("expected ID %s, got %s", created.ID, got.ID)
	}
	if got.NodeID != nodeID {
		t.Fatalf("expected node %s, got %s", nodeID, got.NodeID)
	}
}

func TestGetPlacementReservationNotFound(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	_, err := s.GetPlacementReservation(ctx, uuid.NewString())
	if err == nil {
		t.Fatal("expected error for non-existent reservation")
	}
}

func TestListPlacementReservations(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	nodeID, _, _ := setupTestNode(t, s)
	ids := make([]string, 3)
	for i := 0; i < 3; i++ {
		r, err := s.CreatePlacementReservation(ctx, CreatePlacementReservationRequest{
			NodeID: nodeID,
			CPU:    100,
			Memory: 256,
			Disk:   512,
		})
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = r.ID
	}

	reservations, err := s.ListPlacementReservations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(reservations) < 3 {
		t.Fatalf("expected at least 3 reservations, got %d", len(reservations))
	}
	found := map[string]bool{}
	for _, r := range reservations {
		found[r.ID] = true
	}
	for _, id := range ids {
		if !found[id] {
			t.Fatalf("expected reservation %s in list", id)
		}
	}
}

func TestUpdatePlacementReservationStatus(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	nodeID, _, _ := setupTestNode(t, s)

	t.Run("complete", func(t *testing.T) {
		r, err := s.CreatePlacementReservation(ctx, CreatePlacementReservationRequest{
			NodeID: nodeID,
			CPU:    100,
			Memory: 256,
			Disk:   512,
		})
		if err != nil {
			t.Fatal(err)
		}
		updated, err := s.UpdatePlacementReservationStatus(ctx, r.ID, PlacementReservationStatusCompleted)
		if err != nil {
			t.Fatal(err)
		}
		if updated.Status != PlacementReservationStatusCompleted {
			t.Fatalf("expected completed, got %s", updated.Status)
		}
		if updated.ConfirmedAt == nil {
			t.Fatal("expected ConfirmedAt to be set")
		}
	})

	t.Run("cancel", func(t *testing.T) {
		r, err := s.CreatePlacementReservation(ctx, CreatePlacementReservationRequest{
			NodeID: nodeID,
			CPU:    100,
			Memory: 256,
			Disk:   512,
		})
		if err != nil {
			t.Fatal(err)
		}
		updated, err := s.UpdatePlacementReservationStatus(ctx, r.ID, PlacementReservationStatusCancelled)
		if err != nil {
			t.Fatal(err)
		}
		if updated.Status != PlacementReservationStatusCancelled {
			t.Fatalf("expected cancelled, got %s", updated.Status)
		}
		if updated.CancelledAt == nil {
			t.Fatal("expected CancelledAt to be set")
		}
	})

	t.Run("expire", func(t *testing.T) {
		r, err := s.CreatePlacementReservation(ctx, CreatePlacementReservationRequest{
			NodeID: nodeID,
			CPU:    100,
			Memory: 256,
			Disk:   512,
		})
		if err != nil {
			t.Fatal(err)
		}
		updated, err := s.UpdatePlacementReservationStatus(ctx, r.ID, PlacementReservationStatusExpired)
		if err != nil {
			t.Fatal(err)
		}
		if updated.Status != PlacementReservationStatusExpired {
			t.Fatalf("expected expired, got %s", updated.Status)
		}
		if updated.ExpiredAt == nil {
			t.Fatal("expected ExpiredAt to be set")
		}
	})

	t.Run("already terminal returns error", func(t *testing.T) {
		r, err := s.CreatePlacementReservation(ctx, CreatePlacementReservationRequest{
			NodeID: nodeID,
			CPU:    100,
			Memory: 256,
			Disk:   512,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.UpdatePlacementReservationStatus(ctx, r.ID, PlacementReservationStatusCompleted); err != nil {
			t.Fatal(err)
		}
		_, err = s.UpdatePlacementReservationStatus(ctx, r.ID, PlacementReservationStatusCancelled)
		if err == nil {
			t.Fatal("expected error updating already-terminal reservation")
		}
	})
}

func TestExpirePlacementReservations(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	nodeID, _, _ := setupTestNode(t, s)
	past := time.Now().UTC().Add(-1 * time.Minute)

	id := uuid.NewString()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO placement_reservations (id, node_id, cpu, memory, disk, status, expires_at)
		VALUES ($1, $2, 100, 256, 512, 'active', $3)
	`, id, nodeID, past); err != nil {
		t.Fatal(err)
	}

	expired, err := s.ExpirePlacementReservations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 1 || expired[0].ID != id {
		t.Fatalf("expected 1 expired reservation with ID %s, got %+v", id, expired)
	}
	if expired[0].Status != PlacementReservationStatusExpired {
		t.Fatalf("expected expired status, got %s", expired[0].Status)
	}
	if expired[0].ExpiredAt == nil {
		t.Fatal("expected ExpiredAt timestamp")
	}
}

func TestExpirePlacementReservationsOnlyExpired(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	nodeID, _, _ := setupTestNode(t, s)

	future := time.Now().UTC().Add(10 * time.Minute)
	id := uuid.NewString()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO placement_reservations (id, node_id, cpu, memory, disk, status, expires_at)
		VALUES ($1, $2, 100, 256, 512, 'active', $3)
	`, id, nodeID, future); err != nil {
		t.Fatal(err)
	}

	expired, err := s.ExpirePlacementReservations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 0 {
		t.Fatalf("expected 0 expired reservations, got %d", len(expired))
	}
}

func TestUpdatePlacementReservationsForMigration(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	nodeID, _, _ := setupTestNode(t, s)
	migrationID := uuid.NewString()

	ids := make([]string, 2)
	for i := 0; i < 2; i++ {
		r, err := s.CreatePlacementReservation(ctx, CreatePlacementReservationRequest{
			NodeID:      nodeID,
			MigrationID: migrationID,
			CPU:         100,
			Memory:      256,
			Disk:        512,
		})
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = r.ID
	}

	t.Run("complete", func(t *testing.T) {
		// create fresh reservations for this subtest
		localMigrationID := uuid.NewString()
		localIDs := make([]string, 2)
		for i := 0; i < 2; i++ {
			r, err := s.CreatePlacementReservation(ctx, CreatePlacementReservationRequest{
				NodeID:      nodeID,
				MigrationID: localMigrationID,
				CPU:         100,
				Memory:      256,
				Disk:        512,
			})
			if err != nil {
				t.Fatal(err)
			}
			localIDs[i] = r.ID
		}
		updated, err := s.UpdatePlacementReservationsForMigration(ctx, localMigrationID, PlacementReservationStatusCompleted)
		if err != nil {
			t.Fatal(err)
		}
		if len(updated) != 2 {
			t.Fatalf("expected 2 updated reservations, got %d", len(updated))
		}
		for _, r := range updated {
			if r.Status != PlacementReservationStatusCompleted {
				t.Fatalf("expected completed, got %s", r.Status)
			}
			if r.ConfirmedAt == nil {
				t.Fatal("expected ConfirmedAt to be set")
			}
		}
	})

	t.Run("cancel", func(t *testing.T) {
		localMigrationID := uuid.NewString()
		for range 2 {
			_, err := s.CreatePlacementReservation(ctx, CreatePlacementReservationRequest{
				NodeID:      nodeID,
				MigrationID: localMigrationID,
				CPU:         100,
				Memory:      256,
				Disk:        512,
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		updated, err := s.UpdatePlacementReservationsForMigration(ctx, localMigrationID, PlacementReservationStatusCancelled)
		if err != nil {
			t.Fatal(err)
		}
		if len(updated) != 2 {
			t.Fatalf("expected 2 updated, got %d", len(updated))
		}
		for _, r := range updated {
			if r.Status != PlacementReservationStatusCancelled {
				t.Fatalf("expected cancelled, got %s", r.Status)
			}
		}
	})

	t.Run("no matching reservations", func(t *testing.T) {
		updated, err := s.UpdatePlacementReservationsForMigration(ctx, uuid.NewString(), PlacementReservationStatusCompleted)
		if err != nil {
			t.Fatal(err)
		}
		if len(updated) != 0 {
			t.Fatalf("expected 0 updated, got %d", len(updated))
		}
	})
}

func TestActiveReservedCapacity(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	nodeID, _, _ := setupTestNode(t, s)

	if _, err := s.CreatePlacementReservation(ctx, CreatePlacementReservationRequest{
		NodeID: nodeID,
		CPU:    200,
		Memory: 512,
		Disk:   1024,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreatePlacementReservation(ctx, CreatePlacementReservationRequest{
		NodeID: nodeID,
		CPU:    300,
		Memory: 1024,
		Disk:   2048,
	}); err != nil {
		t.Fatal(err)
	}

	capacity, err := s.ActiveReservedCapacity(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if capacity.CPU != 500 {
		t.Fatalf("expected CPU 500, got %d", capacity.CPU)
	}
	if capacity.Memory != 1536 {
		t.Fatalf("expected Memory 1536, got %d", capacity.Memory)
	}
	if capacity.Disk != 3072 {
		t.Fatalf("expected Disk 3072, got %d", capacity.Disk)
	}
}

func TestActiveReservedCapacityExcludesExpired(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	nodeID, _, _ := setupTestNode(t, s)

	if _, err := s.CreatePlacementReservation(ctx, CreatePlacementReservationRequest{
		NodeID: nodeID,
		CPU:    200,
		Memory: 512,
		Disk:   1024,
	}); err != nil {
		t.Fatal(err)
	}

	past := time.Now().UTC().Add(-1 * time.Hour)
	if _, err := s.db.Exec(ctx, `
		INSERT INTO placement_reservations (id, node_id, cpu, memory, disk, status, expires_at)
		VALUES ($1, $2, 999, 9999, 99999, 'active', $3)
	`, uuid.NewString(), nodeID, past); err != nil {
		t.Fatal(err)
	}

	capacity, err := s.ActiveReservedCapacity(ctx, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	if capacity.CPU != 200 {
		t.Fatalf("expected CPU 200 (excluding expired), got %d", capacity.CPU)
	}
}

func TestPlacementReservationsTotal(t *testing.T) {
	s := migrationTestStore(t, false)
	ctx := context.Background()

	nodeID, _, _ := setupTestNode(t, s)

	count, err := s.PlacementReservationsTotal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before := count

	if _, err := s.CreatePlacementReservation(ctx, CreatePlacementReservationRequest{
		NodeID: nodeID,
		CPU:    100,
		Memory: 256,
		Disk:   512,
	}); err != nil {
		t.Fatal(err)
	}

	count, err = s.PlacementReservationsTotal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != before+1 {
		t.Fatalf("expected %d total, got %d", before+1, count)
	}
}

func setupTestNode(t *testing.T, s *Store) (nodeID, locationID, regionID string) {
	t.Helper()
	ctx := context.Background()
	locationID = uuid.NewString()
	regionID = uuid.NewString()
	nodeID = uuid.NewString()
	if _, err := s.db.Exec(ctx, `INSERT INTO locations (id, short, long) VALUES ($1, 'test', 'Test Location')`, locationID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO regions (id, uuid, name, slug) VALUES ($1, $1, 'Test Region', 'test-region')`, regionID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO nodes (id, name, region, base_url, token_hash, location_id, region_id, cpu_threads, memory_mb, disk_mb)
		VALUES ($1, 'Test Node', 'test', 'http://node.test', 'hash', $2, $3, 8, 16384, 102400)
	`, nodeID, locationID, regionID); err != nil {
		t.Fatal(err)
	}
	return
}
