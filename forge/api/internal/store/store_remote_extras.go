package store

import (
	"context"
	"errors"
)

// GetBackupByUUID returns a backup looked up by its UUID column
// (called by the daemon via /api/remote/backups/{backup}).
func (s *Store) GetBackupByUUID(ctx context.Context, uuid string) (Backup, error) {
	if s.db == nil {
		return Backup{}, errors.New("no database connection")
	}
	row := s.db.QueryRow(ctx, `
		SELECT uuid, server_id, name, checksum, size, status, upload_id, completed_at, created_at, updated_at
		FROM backups
		WHERE uuid = $1
		LIMIT 1
	`, uuid)
	var b Backup
	var uploadID *string
	var completedAt *interface{}
	if err := row.Scan(&b.UUID, &b.ServerID, &b.Name, &b.Checksum, &b.Size, &b.Status, &uploadID, &completedAt, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return Backup{}, err
	}
	b.UploadID = uploadID
	return b, nil
}

// GetServerTransferState returns the current transfer state for a server.
// Returns "none" if no transfer is in progress or has been recorded.
func (s *Store) GetServerTransferState(ctx context.Context, serverID string) (string, error) {
	if s.db == nil {
		return "none", nil
	}
	var state *string
	err := s.db.QueryRow(ctx, `
		SELECT transfer_state::text
		FROM servers
		WHERE id = $1
	`, serverID).Scan(&state)
	if err != nil {
		return "none", nil
	}
	if state == nil || *state == "" {
		return "none", nil
	}
	return *state, nil
}
