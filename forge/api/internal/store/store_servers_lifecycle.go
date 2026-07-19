package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// HardDeleteServer releases every allocation and removes the panel server and
// all ON DELETE CASCADE relationships in one transaction.
func (s *Store) HardDeleteServer(ctx context.Context, serverID string) error {
	return s.hardDeleteServer(ctx, serverID, "", "")
}

// RecordOrphanAndHardDeleteServer records daemon cleanup work before removing
// panel state. The remediation row intentionally has no foreign key to servers.
func (s *Store) RecordOrphanAndHardDeleteServer(ctx context.Context, serverID, nodeURL, daemonError string) error {
	if daemonError == "" {
		return errors.New("daemon error is required for orphan remediation")
	}
	return s.hardDeleteServer(ctx, serverID, nodeURL, daemonError)
}

func (s *Store) hardDeleteServer(ctx context.Context, serverID, nodeURL, daemonError string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var exists int
	if err := tx.QueryRow(ctx, `SELECT 1 FROM servers WHERE id = $1 FOR UPDATE`, serverID).Scan(&exists); err != nil {
		return errors.New("server not found")
	}
	if daemonError != "" {
		if _, err := tx.Exec(ctx, `
			INSERT INTO server_orphan_remediations (id, server_id, node_url, daemon_error)
			VALUES ($1, $2, $3, $4)
		`, uuid.NewString(), serverID, nodeURL, daemonError); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE servers SET primary_allocation_id = NULL WHERE id = $1`, serverID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE allocations SET server_id = NULL, assigned_at = NULL WHERE server_id = $1`, serverID); err != nil {
		return err
	}
	commandTag, err := tx.Exec(ctx, `DELETE FROM servers WHERE id = $1`, serverID)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("server not found")
	}
	return tx.Commit(ctx)
}
