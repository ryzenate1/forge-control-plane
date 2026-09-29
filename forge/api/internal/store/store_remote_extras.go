package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrServerNotFound is returned when a lookup names a server that has no row.
// It is distinct from a query failure: "this server does not exist" is an
// answer, "I could not ask" is not.
var ErrServerNotFound = errors.New("server not found")

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
// "none" means the server exists and has no transfer recorded — a real answer,
// not a fallback.
//
// Every failure path returns an error. This function used to return
// ("none", nil) for a nil database handle, for any query failure and for a
// server that does not exist, which made "I cannot tell you" indistinguishable
// from "it is not transferring". Callers act on that answer — one of them is
// the node-facing /api/remote transfer endpoint — so a swallowed error meant a
// node could be told a server was idle while it was mid-transfer.
//
// A caller that genuinely wants to treat an unknown server as idle must say so
// with errors.Is(err, ErrServerNotFound); it is no longer the silent default.
func (s *Store) GetServerTransferState(ctx context.Context, serverID string) (string, error) {
	if s.db == nil {
		return "", errors.New("no database connection")
	}
	var state *string
	err := s.db.QueryRow(ctx, `
		SELECT transfer_state::text
		FROM servers
		WHERE id = $1
	`, serverID).Scan(&state)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return "", fmt.Errorf("%w: %s", ErrServerNotFound, serverID)
	case err != nil:
		return "", fmt.Errorf("read transfer state for server %s: %w", serverID, err)
	}
	if state == nil || *state == "" {
		return "none", nil
	}
	return *state, nil
}
