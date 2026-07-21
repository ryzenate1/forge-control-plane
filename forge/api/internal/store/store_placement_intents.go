package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

func (s *Store) CreatePlacementIntent(ctx context.Context, intent PlacementIntent) (PlacementIntent, error) {
	if intent.NodeID == "" {
		return PlacementIntent{}, errors.New("nodeId is required")
	}
	intent.ID = uuid.NewString()
	if intent.Status == "" {
		intent.Status = PlacementIntentStatusPending
	}
	intent.CreatedAt = time.Now().UTC()
	intent.UpdatedAt = time.Now().UTC()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return PlacementIntent{}, err
	}
	defer tx.Rollback(ctx)

	var nodeID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM nodes WHERE id = $1 FOR UPDATE`, intent.NodeID).Scan(&nodeID); err != nil {
		return PlacementIntent{}, errors.New("node not found")
	}

	var allocatedCPU, allocatedMemory, allocatedDisk int
	if err := tx.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(s.cpu_shares) FILTER (WHERE s.status <> 'deleted'), 0)::int,
			COALESCE(SUM(s.memory_mb) FILTER (WHERE s.status <> 'deleted'), 0)::int,
			COALESCE(SUM(s.disk_mb) FILTER (WHERE s.status <> 'deleted'), 0)::int
		FROM servers s
		WHERE s.node_id = $1
	`, intent.NodeID).Scan(&allocatedCPU, &allocatedMemory, &allocatedDisk); err != nil {
		return PlacementIntent{}, err
	}

	var pendingCPU, pendingMemory, pendingDisk int
	if err := tx.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(pi.cpu) FILTER (WHERE pi.status = 'pending'), 0)::int,
			COALESCE(SUM(pi.memory_mb) FILTER (WHERE pi.status = 'pending'), 0)::int,
			COALESCE(SUM(pi.disk_mb) FILTER (WHERE pi.status = 'pending'), 0)::int
		FROM placement_intents pi
		WHERE pi.node_id = $1
	`, intent.NodeID).Scan(&pendingCPU, &pendingMemory, &pendingDisk); err != nil {
		return PlacementIntent{}, err
	}

	var cpuThreads, memoryMB, diskMB int
	if err := tx.QueryRow(ctx, `
		SELECT
			COALESCE(n.cpu_threads, 0),
			COALESCE(NULLIF(n.node_memory_mb, 0), n.memory_mb, 0),
			COALESCE(NULLIF(n.node_disk_mb, 0), n.disk_mb, 0)
		FROM nodes n
		WHERE n.id = $1
	`, intent.NodeID).Scan(&cpuThreads, &memoryMB, &diskMB); err != nil {
		return PlacementIntent{}, err
	}

	totalCPU := cpuThreads * 1024
	usedCPU := allocatedCPU + pendingCPU
	if intent.CPU > 0 && totalCPU > 0 && usedCPU+intent.CPU > totalCPU {
		return PlacementIntent{}, errors.New("insufficient cpu capacity")
	}
	if intent.MemoryMB > 0 && memoryMB > 0 && allocatedMemory+intent.MemoryMB+pendingMemory > memoryMB {
		return PlacementIntent{}, errors.New("insufficient memory capacity")
	}
	if intent.DiskMB > 0 && diskMB > 0 && allocatedDisk+intent.DiskMB+pendingDisk > diskMB {
		return PlacementIntent{}, errors.New("insufficient disk capacity")
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO placement_intents (id, server_id, node_id, allocation_id, reservation_id, cpu, memory_mb, disk_mb, status, error, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, intent.ID, intent.ServerID, intent.NodeID, intent.AllocationID, intent.ReservationID, intent.CPU, intent.MemoryMB, intent.DiskMB, string(intent.Status), intent.Error, intent.CreatedAt, intent.UpdatedAt); err != nil {
		return PlacementIntent{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PlacementIntent{}, err
	}
	return s.GetPlacementIntent(ctx, intent.ID)
}

func (s *Store) GetPlacementIntent(ctx context.Context, id string) (PlacementIntent, error) {
	var intent PlacementIntent
	var serverID, allocationID, reservationID, errStr sql.NullString
	var statusStr string
	var confirmedAt, expiredAt sql.NullTime
	err := s.db.QueryRow(ctx, `
		SELECT id::text, server_id::text, node_id::text, allocation_id::text, reservation_id::text,
		       cpu, memory_mb, disk_mb, status::text, error, created_at, updated_at, confirmed_at, expired_at
		FROM placement_intents
		WHERE id = $1
	`, id).Scan(
		&intent.ID, &serverID, &intent.NodeID, &allocationID, &reservationID,
		&intent.CPU, &intent.MemoryMB, &intent.DiskMB, &statusStr,
		&errStr, &intent.CreatedAt, &intent.UpdatedAt, &confirmedAt, &expiredAt,
	)
	if err != nil {
		return PlacementIntent{}, err
	}
	intent.Status = PlacementIntentStatus(statusStr)
	if serverID.Valid {
		intent.ServerID = serverID.String
	}
	if allocationID.Valid {
		intent.AllocationID = allocationID.String
	}
	if reservationID.Valid {
		intent.ReservationID = reservationID.String
	}
	if errStr.Valid {
		intent.Error = errStr.String
	}
	if confirmedAt.Valid {
		intent.ConfirmedAt = &confirmedAt.Time
	}
	if expiredAt.Valid {
		intent.ExpiredAt = &expiredAt.Time
	}
	return intent, nil
}

func (s *Store) ListPendingPlacementIntents(ctx context.Context) ([]PlacementIntent, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text
		FROM placement_intents
		WHERE status IN ('pending', 'completing')
		ORDER BY created_at, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	intents := []PlacementIntent{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		intent, err := s.GetPlacementIntent(ctx, id)
		if err != nil {
			return nil, err
		}
		intents = append(intents, intent)
	}
	return intents, rows.Err()
}

func (s *Store) UpdatePlacementIntentStatus(ctx context.Context, id string, status PlacementIntentStatus, errStr string) (PlacementIntent, error) {
	column := "updated_at"
	switch status {
	case PlacementIntentStatusCompleted:
		column = "confirmed_at"
	case PlacementIntentStatusExpired:
		column = "expired_at"
	}
	commandTag, err := s.db.Exec(ctx, `
		UPDATE placement_intents
		SET status = $2::text,
		    error = $3,
		    updated_at = now(),
		    confirmed_at = CASE WHEN $4 = 'confirmed_at' THEN now() ELSE confirmed_at END,
		    expired_at = CASE WHEN $4 = 'expired_at' THEN now() ELSE expired_at END
		WHERE id = $1
	`, id, string(status), errStr, column)
	if err != nil {
		return PlacementIntent{}, err
	}
	if commandTag.RowsAffected() == 0 {
		return PlacementIntent{}, errors.New("placement intent not found")
	}
	return s.GetPlacementIntent(ctx, id)
}

func (s *Store) RecoverPendingPlacementIntents(ctx context.Context) ([]PlacementIntent, error) {
	_, err := s.db.Exec(ctx, `
		UPDATE placement_intents
		SET status = 'expired',
		    error = 'recovered: rolled back on restart',
		    expired_at = now(),
		    updated_at = now()
		WHERE status = 'pending'
	`)
	if err != nil {
		return nil, err
	}
	return s.ListPendingPlacementIntents(ctx)
}
