package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type OrphanRemediationStatus string

const (
	OrphanRemediationStatusPending  OrphanRemediationStatus = "pending"
	OrphanRemediationStatusResolved OrphanRemediationStatus = "resolved"
)

var (
	ErrOrphanRemediationNotFound = errors.New("orphan remediation not found")
	ErrOrphanRemediationResolved = errors.New("orphan remediation is already resolved")
)

type ServerOrphanRemediation struct {
	ID          string                  `json:"id"`
	ServerID    string                  `json:"serverId"`
	NodeURL     string                  `json:"nodeUrl"`
	DaemonError string                  `json:"daemonError"`
	Status      OrphanRemediationStatus `json:"status"`
	CreatedAt   time.Time               `json:"createdAt"`
	ResolvedAt  *time.Time              `json:"resolvedAt,omitempty"`
}

type DatabaseOrphanRemediation struct {
	ID               string                  `json:"id"`
	ServerDatabaseID string                  `json:"serverDatabaseId"`
	ServerID         string                  `json:"serverId"`
	DatabaseHostID   string                  `json:"databaseHostId"`
	Engine           string                  `json:"engine"`
	Host             string                  `json:"host"`
	Port             int                     `json:"port"`
	DatabaseName     string                  `json:"database"`
	Username         string                  `json:"username"`
	Remote           string                  `json:"remote"`
	Reason           string                  `json:"reason"`
	Status           OrphanRemediationStatus `json:"status"`
	CreatedAt        time.Time               `json:"createdAt"`
	ResolvedAt       *time.Time              `json:"resolvedAt,omitempty"`
}

func validOrphanRemediationStatus(status OrphanRemediationStatus) bool {
	return status == "" || status == OrphanRemediationStatusPending || status == OrphanRemediationStatusResolved
}

func (s *Store) ListServerOrphanRemediations(ctx context.Context, status OrphanRemediationStatus) ([]ServerOrphanRemediation, error) {
	if !validOrphanRemediationStatus(status) {
		return nil, errors.New("invalid orphan remediation status")
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, node_url, daemon_error, status, created_at, resolved_at
		FROM server_orphan_remediations
		WHERE ($1 = '' OR status = $1)
		ORDER BY created_at DESC
	`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	remediations := []ServerOrphanRemediation{}
	for rows.Next() {
		var remediation ServerOrphanRemediation
		if err := rows.Scan(&remediation.ID, &remediation.ServerID, &remediation.NodeURL, &remediation.DaemonError, &remediation.Status, &remediation.CreatedAt, &remediation.ResolvedAt); err != nil {
			return nil, err
		}
		remediations = append(remediations, remediation)
	}
	return remediations, rows.Err()
}

func (s *Store) ListDatabaseOrphanRemediations(ctx context.Context, status OrphanRemediationStatus) ([]DatabaseOrphanRemediation, error) {
	if !validOrphanRemediationStatus(status) {
		return nil, errors.New("invalid orphan remediation status")
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_database_id::text, server_id::text, database_host_id::text,
		       engine, host, port, database_name, username, remote, reason, status, created_at, resolved_at
		FROM database_orphan_remediations
		WHERE ($1 = '' OR status = $1)
		ORDER BY created_at DESC
	`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	remediations := []DatabaseOrphanRemediation{}
	for rows.Next() {
		var remediation DatabaseOrphanRemediation
		if err := rows.Scan(
			&remediation.ID, &remediation.ServerDatabaseID, &remediation.ServerID, &remediation.DatabaseHostID,
			&remediation.Engine, &remediation.Host, &remediation.Port, &remediation.DatabaseName,
			&remediation.Username, &remediation.Remote, &remediation.Reason, &remediation.Status,
			&remediation.CreatedAt, &remediation.ResolvedAt,
		); err != nil {
			return nil, err
		}
		remediations = append(remediations, remediation)
	}
	return remediations, rows.Err()
}

func (s *Store) ResolveServerOrphanRemediation(ctx context.Context, remediationID string, actorID *string) (ServerOrphanRemediation, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ServerOrphanRemediation{}, err
	}
	defer tx.Rollback(ctx)

	var remediation ServerOrphanRemediation
	err = tx.QueryRow(ctx, `
		UPDATE server_orphan_remediations
		SET status = $2, resolved_at = now()
		WHERE id = $1 AND status = $3
		RETURNING id::text, server_id::text, node_url, daemon_error, status, created_at, resolved_at
	`, remediationID, OrphanRemediationStatusResolved, OrphanRemediationStatusPending).Scan(
		&remediation.ID, &remediation.ServerID, &remediation.NodeURL, &remediation.DaemonError,
		&remediation.Status, &remediation.CreatedAt, &remediation.ResolvedAt,
	)
	if err != nil {
		return ServerOrphanRemediation{}, remediationResolutionError(ctx, tx, "server_orphan_remediations", remediationID, err)
	}
	if err := appendRemediationResolutionAudit(ctx, tx, actorID, remediation.ID, "server orphan remediation resolved", remediation.ServerID); err != nil {
		return ServerOrphanRemediation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ServerOrphanRemediation{}, err
	}
	return remediation, nil
}

func (s *Store) ResolveDatabaseOrphanRemediation(ctx context.Context, remediationID string, actorID *string) (DatabaseOrphanRemediation, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return DatabaseOrphanRemediation{}, err
	}
	defer tx.Rollback(ctx)

	var remediation DatabaseOrphanRemediation
	err = tx.QueryRow(ctx, `
		UPDATE database_orphan_remediations
		SET status = $2, resolved_at = now()
		WHERE id = $1 AND status = $3
		RETURNING id::text, server_database_id::text, server_id::text, database_host_id::text,
		          engine, host, port, database_name, username, remote, reason, status, created_at, resolved_at
	`, remediationID, OrphanRemediationStatusResolved, OrphanRemediationStatusPending).Scan(
		&remediation.ID, &remediation.ServerDatabaseID, &remediation.ServerID, &remediation.DatabaseHostID,
		&remediation.Engine, &remediation.Host, &remediation.Port, &remediation.DatabaseName,
		&remediation.Username, &remediation.Remote, &remediation.Reason, &remediation.Status,
		&remediation.CreatedAt, &remediation.ResolvedAt,
	)
	if err != nil {
		return DatabaseOrphanRemediation{}, remediationResolutionError(ctx, tx, "database_orphan_remediations", remediationID, err)
	}
	if err := appendRemediationResolutionAudit(ctx, tx, actorID, remediation.ID, "database orphan remediation resolved", remediation.ServerID); err != nil {
		return DatabaseOrphanRemediation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DatabaseOrphanRemediation{}, err
	}
	return remediation, nil
}

func remediationResolutionError(ctx context.Context, tx pgx.Tx, table, remediationID string, err error) error {
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var status OrphanRemediationStatus
	if err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT status FROM %s WHERE id = $1`, table), remediationID).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return ErrOrphanRemediationNotFound
	} else if err != nil {
		return err
	}
	return ErrOrphanRemediationResolved
}

func appendRemediationResolutionAudit(ctx context.Context, tx pgx.Tx, actorID *string, remediationID, action, serverID string) error {
	metadata := mustAuditJSON(map[string]string{"serverId": serverID})
	_, err := tx.Exec(ctx, `
		INSERT INTO audit_events (id, actor_id, action, target_type, target_id, metadata)
		VALUES ($1, $2, $3, 'orphan_remediation', $4, $5::jsonb)
	`, uuid.NewString(), actorID, action, remediationID, metadata)
	return err
}
