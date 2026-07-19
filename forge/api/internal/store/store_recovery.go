package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

func (s *Store) CreateRecoveryPlan(ctx context.Context, nodeID, reason string) (RecoveryPlan, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO recovery_plans (id, node_id, status, reason)
		VALUES ($1, $2, 'pending', $3)
	`, id, nodeID, reason)
	if err != nil {
		return RecoveryPlan{}, err
	}
	return s.GetRecoveryPlan(ctx, id)
}

func (s *Store) UpdateRecoveryPlanStatus(ctx context.Context, planID string, status RecoveryPlanStatus, reason string) (RecoveryPlan, error) {
	_, err := s.db.Exec(ctx, `
		UPDATE recovery_plans
		SET status = $2::recovery_plan_status,
		    reason = CASE WHEN $3 = '' THEN reason ELSE $3 END,
		    updated_at = now()
		WHERE id = $1
	`, planID, string(status), reason)
	if err != nil {
		return RecoveryPlan{}, err
	}
	return s.GetRecoveryPlan(ctx, planID)
}

func (s *Store) GetRecoveryPlan(ctx context.Context, planID string) (RecoveryPlan, error) {
	var plan RecoveryPlan
	if err := s.db.QueryRow(ctx, `
		SELECT id::text, node_id::text, status::text, reason, created_at, updated_at
		FROM recovery_plans
		WHERE id = $1
	`, planID).Scan(&plan.ID, &plan.NodeID, &plan.Status, &plan.Reason, &plan.CreatedAt, &plan.UpdatedAt); err != nil {
		return RecoveryPlan{}, err
	}
	items, err := s.ListRecoveryItems(ctx, plan.ID)
	if err != nil {
		return RecoveryPlan{}, err
	}
	plan.Items = items
	return plan, nil
}

func (s *Store) ListRecoveryPlans(ctx context.Context) ([]RecoveryPlan, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text
		FROM recovery_plans
		ORDER BY created_at DESC, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	plans := []RecoveryPlan{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		plan, err := s.GetRecoveryPlan(ctx, id)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, rows.Err()
}

func (s *Store) CreateRecoveryItem(ctx context.Context, planID string, item RecoveryItem) (RecoveryItem, error) {
	id := uuid.NewString()
	var targetNodeID any
	if item.TargetNodeID != "" {
		targetNodeID = item.TargetNodeID
	}
	var reservationID any
	if item.ReservationID != "" {
		reservationID = item.ReservationID
	}
	var migrationID any
	if item.MigrationID != "" {
		migrationID = item.MigrationID
	}
	status := item.Status
	if status == "" {
		status = string(RecoveryItemStatusPending)
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO recovery_items (
			id, plan_id, server_id, source_node_id, target_node_id, reservation_id, migration_id,
			source_backup_name, source_backup_checksum, source_backup_size, status, reason
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::recovery_item_status, $12)
	`, id, planID, item.ServerID, item.SourceNodeID, targetNodeID, reservationID, migrationID,
		item.SourceBackupName, item.SourceBackupChecksum, item.SourceBackupSize, status, item.Reason)
	if err != nil {
		return RecoveryItem{}, err
	}
	items, err := s.ListRecoveryItems(ctx, planID)
	if err != nil {
		return RecoveryItem{}, err
	}
	for _, created := range items {
		if created.ID == id {
			return created, nil
		}
	}
	return RecoveryItem{}, sql.ErrNoRows
}

func (s *Store) ListRecoveryItems(ctx context.Context, planID string) ([]RecoveryItem, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, plan_id::text, server_id::text, source_node_id::text,
		       target_node_id::text, reservation_id::text, migration_id::text,
		       source_backup_name, source_backup_checksum, source_backup_size, status::text, reason
		FROM recovery_items
		WHERE plan_id = $1
		ORDER BY created_at, id
	`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []RecoveryItem{}
	for rows.Next() {
		var item RecoveryItem
		var targetNodeID, reservationID, migrationID sql.NullString
		if err := rows.Scan(&item.ID, &item.PlanID, &item.ServerID, &item.SourceNodeID, &targetNodeID, &reservationID, &migrationID, &item.SourceBackupName, &item.SourceBackupChecksum, &item.SourceBackupSize, &item.Status, &item.Reason); err != nil {
			return nil, err
		}
		if targetNodeID.Valid {
			item.TargetNodeID = targetNodeID.String
		}
		if reservationID.Valid {
			item.ReservationID = reservationID.String
		}
		if migrationID.Valid {
			item.MigrationID = migrationID.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// StartRecoveryPlan atomically claims a planned recovery plan and its planned
// items for execution. A concurrent caller observes no updated rows and gets a
// state-specific error instead of starting the same migrations twice.
func (s *Store) StartRecoveryPlan(ctx context.Context, planID string) (RecoveryPlan, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return RecoveryPlan{}, err
	}
	defer tx.Rollback(ctx)

	var status string
	if err := tx.QueryRow(ctx, `SELECT status::text FROM recovery_plans WHERE id = $1 FOR UPDATE`, planID).Scan(&status); err != nil {
		return RecoveryPlan{}, err
	}
	if status != string(RecoveryPlanStatusPlanned) && status != string(RecoveryPlanStatusExecuting) {
		return RecoveryPlan{}, fmt.Errorf("recovery plan is not executable from status %q", status)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE recovery_plans
		SET status = $2::recovery_plan_status, reason = $3, updated_at = now()
		WHERE id = $1
	`, planID, string(RecoveryPlanStatusExecuting), "executing recovery migrations"); err != nil {
		return RecoveryPlan{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE recovery_items
		SET status = $2::recovery_item_status, reason = $3, updated_at = now()
		WHERE plan_id = $1 AND status = 'planned'
	`, planID, string(RecoveryItemStatusExecuting), "migration execution started"); err != nil {
		return RecoveryPlan{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return RecoveryPlan{}, err
	}
	return s.GetRecoveryPlan(ctx, planID)
}

func (s *Store) UpdateRecoveryItemStatus(ctx context.Context, itemID string, status RecoveryItemStatus, reason string) (RecoveryItem, error) {
	if _, err := s.db.Exec(ctx, `
		UPDATE recovery_items
		SET status = $2::recovery_item_status,
		    reason = CASE WHEN $3 = '' THEN reason ELSE $3 END,
		    updated_at = now()
		WHERE id = $1
	`, itemID, string(status), reason); err != nil {
		return RecoveryItem{}, err
	}
	var item RecoveryItem
	var targetNodeID, reservationID, migrationID sql.NullString
	if err := s.db.QueryRow(ctx, `
		SELECT id::text, plan_id::text, server_id::text, source_node_id::text,
		       target_node_id::text, reservation_id::text, migration_id::text,
		       source_backup_name, source_backup_checksum, source_backup_size, status::text, reason
		FROM recovery_items WHERE id = $1
	`, itemID).Scan(&item.ID, &item.PlanID, &item.ServerID, &item.SourceNodeID, &targetNodeID, &reservationID, &migrationID, &item.SourceBackupName, &item.SourceBackupChecksum, &item.SourceBackupSize, &item.Status, &item.Reason); err != nil {
		return RecoveryItem{}, err
	}
	if targetNodeID.Valid {
		item.TargetNodeID = targetNodeID.String
	}
	if reservationID.Valid {
		item.ReservationID = reservationID.String
	}
	if migrationID.Valid {
		item.MigrationID = migrationID.String
	}
	return item, nil
}

func (s *Store) UpdateRecoveryItemsForPlanStatus(ctx context.Context, planID string, status RecoveryItemStatus, reason string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE recovery_items
		SET status = $2::recovery_item_status,
		    reason = CASE WHEN $3 = '' THEN reason ELSE $3 END,
		    updated_at = now()
		WHERE plan_id = $1
		  AND status IN ('pending', 'planned', 'executing')
	`, planID, string(status), reason)
	return err
}

// LatestVerifiedRecoveryBackup returns only a completed backup with immutable
// integrity metadata. A database row alone is not enough to execute recovery;
// the recovery executor verifies this same archive on the target daemon.
func (s *Store) LatestVerifiedRecoveryBackup(ctx context.Context, serverID string) (Backup, error) {
	var backup Backup
	err := s.db.QueryRow(ctx, `
		SELECT uuid::text, server_id::text, name, checksum, size, status, upload_id, completed_at, created_at, updated_at, is_locked, status_message, status_callback, retry_count, last_retry_at
		FROM backups
		WHERE server_id = $1 AND status = 'completed' AND checksum <> '' AND size > 0
		ORDER BY completed_at DESC NULLS LAST, created_at DESC
		LIMIT 1
	`, serverID).Scan(&backup.UUID, &backup.ServerID, &backup.Name, &backup.Checksum, &backup.Size, &backup.Status, &backup.UploadID, &backup.CompletedAt, &backup.CreatedAt, &backup.UpdatedAt, &backup.IsLocked, &backup.StatusMessage, &backup.StatusCallback, &backup.RetryCount, &backup.LastRetryAt)
	return backup, err
}

// RecoveryRestoreTarget obtains the target daemon credential without changing
// server ownership. Ownership is intentionally left untouched by backup-only
// recovery.
func (s *Store) RecoveryRestoreTarget(ctx context.Context, nodeID, serverID string) (ServerControlTarget, error) {
	var target ServerControlTarget
	var credentialID, credential, encrypted string
	err := s.db.QueryRow(ctx, `
		SELECT $2::text, n.base_url, n.id::text, COALESCE(n.daemon_token_id, ''),
		       COALESCE(n.daemon_token, ''), COALESCE(n.daemon_token_encrypted, '')
		FROM nodes n WHERE n.id = $1
	`, nodeID, serverID).Scan(&target.ServerID, &target.NodeURL, &nodeID, &credentialID, &credential, &encrypted)
	if err != nil {
		return ServerControlTarget{}, err
	}
	plain, err := s.decryptSecret(encrypted, credential, secretAAD("nodes", nodeID, "daemon_token"))
	if err != nil {
		return ServerControlTarget{}, err
	}
	if credentialID != "" && plain != "" {
		target.NodeToken = credentialID + "." + plain
	}
	return target, nil
}

// BeginRecoveryOwnership atomically reserves a destination allocation and
// points the server at the recovery node while retaining the source
// allocations for rollback until the destination workload is created.
func (s *Store) BeginRecoveryOwnership(ctx context.Context, itemID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var serverID, sourceNodeID, targetNodeID string
	if err := tx.QueryRow(ctx, `SELECT server_id::text, source_node_id::text, target_node_id::text FROM recovery_items WHERE id=$1 FOR UPDATE`, itemID).Scan(&serverID, &sourceNodeID, &targetNodeID); err != nil {
		return err
	}
	if targetNodeID == "" {
		return errors.New("recovery item has no target node")
	}
	var currentNode string
	if err := tx.QueryRow(ctx, `SELECT node_id::text FROM servers WHERE id=$1 FOR UPDATE`, serverID).Scan(&currentNode); err != nil {
		return err
	}
	if currentNode == targetNodeID {
		return tx.Commit(ctx)
	}
	if currentNode != sourceNodeID {
		return errors.New("server ownership changed during recovery")
	}
	var allocationID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM allocations WHERE node_id=$1 AND server_id IS NULL
		AND NOT EXISTS (SELECT 1 FROM migration_allocation_reservations mar WHERE mar.allocation_id=allocations.id)
		ORDER BY port, protocol FOR UPDATE SKIP LOCKED LIMIT 1`, targetNodeID).Scan(&allocationID); err != nil {
		return fmt.Errorf("select recovery allocation: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE allocations SET server_id=$1 WHERE id=$2`, serverID, allocationID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE servers SET node_id=$2, primary_allocation_id=$3, desired_state='stopped', actual_state='stopped', status='recovering', config_sync_pending=true, updated_at=now() WHERE id=$1`, serverID, targetNodeID, allocationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CompleteRecoveryOwnership removes stale source bindings and consumes the
// placement reservation only after the destination workload exists.
func (s *Store) CompleteRecoveryOwnership(ctx context.Context, itemID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var serverID, sourceNodeID, reservationID string
	if err := tx.QueryRow(ctx, `SELECT server_id::text, source_node_id::text, COALESCE(reservation_id::text,'') FROM recovery_items WHERE id=$1 FOR UPDATE`, itemID).Scan(&serverID, &sourceNodeID, &reservationID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE allocations SET server_id=NULL WHERE server_id=$1 AND node_id=$2`, serverID, sourceNodeID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE servers SET status='created', installed=true, config_sync_pending=false, updated_at=now() WHERE id=$1`, serverID); err != nil {
		return err
	}
	if reservationID != "" {
		if _, err := tx.Exec(ctx, `UPDATE placement_reservations SET status='completed', confirmed_at=now(), updated_at=now() WHERE id=$1 AND status IN ('pending','active')`, reservationID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// RollbackRecoveryOwnership restores the original node and primary allocation
// when destination provisioning fails.
func (s *Store) RollbackRecoveryOwnership(ctx context.Context, itemID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var serverID, sourceNodeID, targetNodeID string
	if err := tx.QueryRow(ctx, `SELECT server_id::text, source_node_id::text, target_node_id::text FROM recovery_items WHERE id=$1 FOR UPDATE`, itemID).Scan(&serverID, &sourceNodeID, &targetNodeID); err != nil {
		return err
	}
	var sourceAllocationID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM allocations WHERE server_id=$1 AND node_id=$2 ORDER BY port LIMIT 1`, serverID, sourceNodeID).Scan(&sourceAllocationID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE allocations SET server_id=NULL WHERE server_id=$1 AND node_id=$2`, serverID, targetNodeID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE servers SET node_id=$2, primary_allocation_id=$3, status='stopped', config_sync_pending=false, updated_at=now() WHERE id=$1`, serverID, sourceNodeID, sourceAllocationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RecoveryPlansTotal(ctx context.Context) (int64, error) {
	var total int64
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM recovery_plans`).Scan(&total)
	return total, err
}

func (s *Store) RecoveryItemsTotal(ctx context.Context) (int64, error) {
	var total int64
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM recovery_items`).Scan(&total)
	return total, err
}
