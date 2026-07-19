package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateMigration(ctx context.Context, req CreateMigrationRequest) (Migration, error) {
	migrationID := uuid.NewString()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Migration{}, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO migrations (id, server_id, source_node_id, target_node_id, status)
		VALUES ($1, $2, $3, $4, 'planned')
	`, migrationID, req.ServerID, req.SourceNodeID, req.TargetNodeID); err != nil {
		return Migration{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO migration_history (id, migration_id, from_status, to_status, reason)
		VALUES ($1, $2, NULL, 'planned', 'migration created')
	`, uuid.NewString(), migrationID); err != nil {
		return Migration{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Migration{}, err
	}
	return s.GetMigration(ctx, migrationID)
}

func (s *Store) GetMigration(ctx context.Context, migrationID string) (Migration, error) {
	var migration Migration
	if err := s.db.QueryRow(ctx, `
		SELECT m.id::text, m.server_id::text, m.source_node_id::text, m.target_node_id::text, m.status::text, m.failure_reason,
		       COALESCE(r.phase, ''), COALESCE(r.idempotency_key::text, ''), COALESCE(r.archive_size, 0),
		       COALESCE(r.archive_checksum, ''), COALESCE(r.cleanup_pending, false), m.created_at, m.updated_at
			FROM migrations m
			LEFT JOIN migration_runs r ON r.migration_id = m.id
			WHERE m.id = $1
		`, migrationID).Scan(&migration.ID, &migration.ServerID, &migration.SourceNodeID, &migration.TargetNodeID, &migration.Status, &migration.FailureReason,
		&migration.TransferPhase, &migration.IdempotencyKey, &migration.ArchiveSize, &migration.ArchiveChecksum, &migration.CleanupPending,
		&migration.CreatedAt, &migration.UpdatedAt); err != nil {
		return Migration{}, err
	}
	history, err := s.ListMigrationHistory(ctx, migrationID)
	if err != nil {
		return Migration{}, err
	}
	migration.History = history
	return migration, nil
}

func (s *Store) ListMigrations(ctx context.Context) ([]Migration, error) {
	rows, err := s.db.Query(ctx, `
		SELECT m.id::text, m.server_id::text, m.source_node_id::text, m.target_node_id::text, m.status::text, m.failure_reason,
		       COALESCE(r.phase, ''), COALESCE(r.idempotency_key::text, ''), COALESCE(r.archive_size, 0),
		       COALESCE(r.archive_checksum, ''), COALESCE(r.cleanup_pending, false), m.created_at, m.updated_at
			FROM migrations m
			LEFT JOIN migration_runs r ON r.migration_id = m.id
			ORDER BY m.created_at DESC, m.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	migrations := []Migration{}
	for rows.Next() {
		var migration Migration
		if err := rows.Scan(&migration.ID, &migration.ServerID, &migration.SourceNodeID, &migration.TargetNodeID, &migration.Status, &migration.FailureReason,
			&migration.TransferPhase, &migration.IdempotencyKey, &migration.ArchiveSize, &migration.ArchiveChecksum, &migration.CleanupPending,
			&migration.CreatedAt, &migration.UpdatedAt); err != nil {
			return nil, err
		}
		migrations = append(migrations, migration)
	}
	return migrations, rows.Err()
}

func (s *Store) UpdateMigrationStatus(ctx context.Context, migrationID string, status MigrationStatus, reason string) (Migration, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Migration{}, err
	}
	defer tx.Rollback(ctx)

	var previous string
	if err := tx.QueryRow(ctx, `SELECT status::text FROM migrations WHERE id = $1 FOR UPDATE`, migrationID).Scan(&previous); err != nil {
		return Migration{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE migrations
		SET status = $2::migration_status,
		    failure_reason = CASE WHEN $2::text = 'failed' THEN $3 ELSE failure_reason END,
		    updated_at = now()
		WHERE id = $1
	`, migrationID, string(status), reason); err != nil {
		return Migration{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO migration_history (id, migration_id, from_status, to_status, reason)
		VALUES ($1, $2, $3::migration_status, $4::migration_status, $5)
	`, uuid.NewString(), migrationID, previous, string(status), reason); err != nil {
		return Migration{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Migration{}, err
	}
	return s.GetMigration(ctx, migrationID)
}

func (s *Store) ListMigrationHistory(ctx context.Context, migrationID string) ([]MigrationHistory, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, migration_id::text, from_status::text, to_status::text, reason, created_at
		FROM migration_history
		WHERE migration_id = $1
		ORDER BY created_at, id
	`, migrationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	history := []MigrationHistory{}
	for rows.Next() {
		var item MigrationHistory
		var from sql.NullString
		if err := rows.Scan(&item.ID, &item.MigrationID, &from, &item.ToStatus, &item.Reason, &item.CreatedAt); err != nil {
			return nil, err
		}
		if from.Valid {
			item.FromStatus = from.String
		}
		history = append(history, item)
	}
	return history, rows.Err()
}

func (s *Store) ServerNodeID(ctx context.Context, serverID string) (string, error) {
	var nodeID string
	err := s.db.QueryRow(ctx, `SELECT node_id::text FROM servers WHERE id = $1`, serverID).Scan(&nodeID)
	return nodeID, err
}

func (s *Store) EnsureMigrationRun(ctx context.Context, migrationID, protocolVersion string) (MigrationRun, error) {
	if run, err := s.GetMigrationRun(ctx, migrationID); err == nil {
		return run, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return MigrationRun{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return MigrationRun{}, err
	}
	defer tx.Rollback(ctx)
	var allocationID string
	if err := tx.QueryRow(ctx, `SELECT a.id::text FROM allocations a JOIN migrations m ON m.target_node_id=a.node_id
		WHERE m.id=$1 AND a.server_id IS NULL
		  AND NOT EXISTS (SELECT 1 FROM migration_allocation_reservations mar WHERE mar.allocation_id=a.id)
		ORDER BY a.id LIMIT 1 FOR UPDATE OF a SKIP LOCKED`, migrationID).Scan(&allocationID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return MigrationRun{}, errors.New("target node has no available allocation")
		}
		return MigrationRun{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO migration_runs (migration_id,protocol_version,phase,idempotency_key,target_allocation_id)
		VALUES ($1,$2,'planned',$3,$4)`, migrationID, protocolVersion, uuid.NewString(), allocationID); err != nil {
		return MigrationRun{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO migration_allocation_reservations (allocation_id,migration_id) VALUES ($1,$2)`, allocationID, migrationID); err != nil {
		return MigrationRun{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return MigrationRun{}, err
	}
	return s.GetMigrationRun(ctx, migrationID)
}

func (s *Store) GetMigrationRun(ctx context.Context, migrationID string) (MigrationRun, error) {
	var run MigrationRun
	var leaseOwner, allocation sql.NullString
	err := s.db.QueryRow(ctx, `
		SELECT migration_id::text, protocol_version, phase, idempotency_key::text, attempt,
		       lease_owner, target_allocation_id::text, archive_size, archive_checksum,
		       source_credential_hash, destination_credential_hash, credential_expires_at,
		       cleanup_pending, last_error
		FROM migration_runs WHERE migration_id = $1
	`, migrationID).Scan(&run.MigrationID, &run.ProtocolVersion, &run.Phase, &run.IdempotencyKey, &run.Attempt,
		&leaseOwner, &allocation, &run.ArchiveSize, &run.ArchiveChecksum, &run.SourceCredentialHash,
		&run.DestinationCredentialHash, &run.CredentialExpiresAt, &run.CleanupPending, &run.LastError)
	if leaseOwner.Valid {
		run.LeaseOwner = leaseOwner.String
	}
	if allocation.Valid {
		run.TargetAllocationID = allocation.String
	}
	return run, err
}

func (s *Store) ClaimMigrationRun(ctx context.Context, migrationID, worker string, lease time.Duration) (MigrationRun, error) {
	command, err := s.db.Exec(ctx, `
		UPDATE migration_runs SET lease_owner = $2, lease_expires_at = now() + $3::interval,
		       attempt = attempt + 1, updated_at = now()
		WHERE migration_id = $1 AND phase NOT IN ('completed','failed','cancelled')
		  AND (lease_expires_at IS NULL OR lease_expires_at < now() OR lease_owner = $2)
	`, migrationID, worker, lease.String())
	if err != nil {
		return MigrationRun{}, err
	}
	if command.RowsAffected() == 0 {
		return MigrationRun{}, errors.New("migration run is already claimed")
	}
	return s.GetMigrationRun(ctx, migrationID)
}

func (s *Store) UpdateMigrationRun(ctx context.Context, migrationID, phase, lastError string, archiveSize int64, checksum string) (MigrationRun, error) {
	_, err := s.db.Exec(ctx, `UPDATE migration_runs SET phase=$2, last_error=$3,
		archive_size=CASE WHEN $4 > 0 THEN $4 ELSE archive_size END,
		archive_checksum=CASE WHEN $5 <> '' THEN $5 ELSE archive_checksum END,
		lease_expires_at=now() + interval '2 minutes', updated_at=now() WHERE migration_id=$1`,
		migrationID, phase, lastError, archiveSize, checksum)
	if err != nil {
		return MigrationRun{}, err
	}
	return s.GetMigrationRun(ctx, migrationID)
}

func (s *Store) SetMigrationCredentialHashes(ctx context.Context, migrationID, sourceHash, destinationHash string, expiresAt time.Time) error {
	_, err := s.db.Exec(ctx, `UPDATE migration_runs SET source_credential_hash=$2, destination_credential_hash=$3,
		credential_expires_at=$4, updated_at=now() WHERE migration_id=$1`, migrationID, sourceHash, destinationHash, expiresAt)
	return err
}

func (s *Store) ReclaimableMigrationIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT migration_id::text FROM migration_runs
		WHERE phase NOT IN ('completed','failed','cancelled') AND (lease_expires_at IS NULL OR lease_expires_at < now()) ORDER BY updated_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// CleanupPendingMigrationIDs returns completed migrations whose daemon-side
// transfer artifacts still need to be finalized and removed.
func (s *Store) CleanupPendingMigrationIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.Query(ctx, `SELECT migration_id::text FROM migration_runs
		WHERE phase = 'completed' AND cleanup_pending = true ORDER BY updated_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) MigrationProvisionTargets(ctx context.Context, migrationID string) (ServerProvisionTarget, ServerProvisionTarget, error) {
	migration, err := s.GetMigration(ctx, migrationID)
	if err != nil {
		return ServerProvisionTarget{}, ServerProvisionTarget{}, err
	}
	run, err := s.GetMigrationRun(ctx, migrationID)
	if err != nil {
		return ServerProvisionTarget{}, ServerProvisionTarget{}, err
	}
	source, err := s.ServerProvisionTarget(ctx, migration.ServerID)
	if err != nil {
		return ServerProvisionTarget{}, ServerProvisionTarget{}, err
	}
	target := source
	if err := s.db.QueryRow(ctx, `SELECT base_url FROM nodes WHERE id=$1`, migration.TargetNodeID).Scan(&target.NodeURL); err != nil {
		return ServerProvisionTarget{}, ServerProvisionTarget{}, err
	}
	target.NodeToken, err = s.GetNodeDaemonCredential(ctx, migration.TargetNodeID)
	if err != nil {
		return ServerProvisionTarget{}, ServerProvisionTarget{}, err
	}
	var containerPort int
	var protocol string
	if err := s.db.QueryRow(ctx, `SELECT host(ip), port, container_port, protocol FROM allocations WHERE id=$1 AND node_id=$2
		AND (server_id IS NULL OR server_id=$3)`, run.TargetAllocationID, migration.TargetNodeID, migration.ServerID).Scan(&target.AllocationIP, &target.AllocationPort, &containerPort, &protocol); err != nil {
		return ServerProvisionTarget{}, ServerProvisionTarget{}, err
	}
	target.Allocations = []ServerRuntimeAllocation{{ID: run.TargetAllocationID, IP: target.AllocationIP, Port: target.AllocationPort, ContainerPort: containerPort, Protocol: protocol}}
	return source, target, nil
}

func (s *Store) FinalizeMigration(ctx context.Context, migrationID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var serverID, sourceNodeID, targetNodeID, allocationID, phase string
	if err := tx.QueryRow(ctx, `SELECT m.server_id::text, m.source_node_id::text, m.target_node_id::text,
		r.target_allocation_id::text, r.phase FROM migrations m JOIN migration_runs r ON r.migration_id=m.id
		WHERE m.id=$1 FOR UPDATE OF m,r`, migrationID).Scan(&serverID, &sourceNodeID, &targetNodeID, &allocationID, &phase); err != nil {
		return err
	}
	if phase == "completed" {
		return tx.Commit(ctx)
	}
	if phase != "destination_created" {
		return errors.New("destination has not reported restored and created")
	}
	var currentNode string
	if err := tx.QueryRow(ctx, `SELECT node_id::text FROM servers WHERE id=$1 FOR UPDATE`, serverID).Scan(&currentNode); err != nil {
		return err
	}
	if currentNode != sourceNodeID {
		return errors.New("server source ownership changed during migration")
	}
	command, err := tx.Exec(ctx, `UPDATE allocations SET server_id=$1 WHERE id=$2 AND node_id=$3 AND server_id IS NULL`, serverID, allocationID, targetNodeID)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return errors.New("target allocation is no longer available")
	}
	if _, err := tx.Exec(ctx, `UPDATE allocations SET server_id=NULL WHERE server_id=$1 AND node_id=$2`, serverID, sourceNodeID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE servers SET node_id=$2, primary_allocation_id=$3, transferring=false,
		transfer_target_node_id=NULL, transfer_state='completed', transfer_error=NULL, config_sync_pending=false, updated_at=now() WHERE id=$1`,
		serverID, targetNodeID, allocationID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE migrations SET status='completed', failure_reason='', updated_at=now() WHERE id=$1`, migrationID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM migration_allocation_reservations WHERE migration_id=$1`, migrationID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE migration_runs SET phase='completed', cleanup_pending=true, lease_owner=NULL,
		lease_expires_at=NULL, updated_at=now() WHERE migration_id=$1`, migrationID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO migration_history (id,migration_id,from_status,to_status,reason)
		VALUES ($1,$2,'restoring','completed','destination verified, restored, and container created; ownership committed')`, uuid.NewString(), migrationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) MarkMigrationCleanupComplete(ctx context.Context, migrationID string) error {
	_, err := s.db.Exec(ctx, `UPDATE migration_runs SET cleanup_pending=false, updated_at=now() WHERE migration_id=$1 AND phase='completed'`, migrationID)
	return err
}

func (s *Store) CancelMigrationRun(ctx context.Context, migrationID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM migration_allocation_reservations WHERE migration_id=$1`, migrationID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE migration_runs SET phase='cancelled', cleanup_pending=false, lease_owner=NULL,
		lease_expires_at=NULL, updated_at=now() WHERE migration_id=$1 AND phase <> 'completed'`, migrationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) FailMigrationRun(ctx context.Context, migrationID, reason string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM migration_allocation_reservations WHERE migration_id=$1`, migrationID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE migration_runs SET phase='failed', last_error=$2, cleanup_pending=false,
		lease_owner=NULL, lease_expires_at=NULL, updated_at=now() WHERE migration_id=$1 AND phase <> 'completed'`, migrationID, reason); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// GetActiveMigrationForServer returns the most recent non-terminal migration for a server
func (s *Store) GetActiveMigrationForServer(ctx context.Context, serverID string) (Migration, error) {
	var migration Migration
	if err := s.db.QueryRow(ctx, `
		SELECT m.id::text, m.server_id::text, m.source_node_id::text, m.target_node_id::text, m.status::text, m.failure_reason,
		       COALESCE(r.phase, ''), COALESCE(r.idempotency_key::text, ''), COALESCE(r.archive_size, 0),
		       COALESCE(r.archive_checksum, ''), COALESCE(r.cleanup_pending, false), m.created_at, m.updated_at
		FROM migrations m
		LEFT JOIN migration_runs r ON r.migration_id = m.id
		WHERE m.server_id = $1 AND m.status NOT IN ('completed', 'failed', 'cancelled')
		ORDER BY m.created_at DESC LIMIT 1
	`, serverID).Scan(&migration.ID, &migration.ServerID, &migration.SourceNodeID, &migration.TargetNodeID, &migration.Status, &migration.FailureReason,
		&migration.TransferPhase, &migration.IdempotencyKey, &migration.ArchiveSize, &migration.ArchiveChecksum, &migration.CleanupPending,
		&migration.CreatedAt, &migration.UpdatedAt); err != nil {
		return Migration{}, err
	}
	return migration, nil
}

func (s *Store) MigrationsTotal(ctx context.Context) (int64, error) {
	var total int64
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM migrations`).Scan(&total)
	return total, err
}

func (s *Store) MigrationsByStatusTotal(ctx context.Context, status MigrationStatus) (int64, error) {
	var total int64
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM migrations WHERE status = $1::migration_status`, string(status)).Scan(&total)
	return total, err
}
