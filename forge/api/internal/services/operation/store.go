package operation

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) Create(ctx context.Context, op *Operation) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO operations
		(id, kind, resource_type, resource_id, status, idempotency_key, input, desired_generation, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),$7,1,$8,$9) ON CONFLICT DO NOTHING`,
		op.ID, op.Kind, op.ResourceType, op.ResourceID, string(op.Status), op.IdempotencyKey,
		[]byte(op.Input), op.CreatedAt, op.UpdatedAt)
	if err != nil {
		return err
	}
	stepID := "step-" + op.ID
	_, err = s.pool.Exec(ctx, `INSERT INTO operation_steps
		(id, operation_id, name, position, status, max_attempts)
		VALUES ($1,$2,'execute',0,'queued',3) ON CONFLICT DO NOTHING`,
		stepID, op.ID)
	return err
}

func (s *PostgresStore) Dequeue(ctx context.Context) (*Operation, error) {
	var op Operation
	var status string
	var payload []byte
	err := s.pool.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id FROM operations
			WHERE status IN ('queued','retrying')
			ORDER BY CASE WHEN status='retrying' THEN 0 ELSE 1 END, created_at ASC
			LIMIT 1 FOR UPDATE SKIP LOCKED
		)
		UPDATE operations SET status='running', started_at=COALESCE(started_at,NOW()), updated_at=NOW()
		FROM candidate WHERE operations.id=candidate.id
		RETURNING id, kind, resource_type, resource_id, status,
			COALESCE(error,''), COALESCE(input,'{}'::jsonb), COALESCE(idempotency_key,''),
			desired_generation, observed_generation, created_at, updated_at, started_at, completed_at
	`).Scan(
		&op.ID, &op.Kind, &op.ResourceType, &op.ResourceID, &status,
		&op.Error, &payload, &op.IdempotencyKey,
		&op.DesiredGen, &op.ObservedGen, &op.CreatedAt, &op.UpdatedAt, &op.StartedAt, &op.CompletedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	op.Status = Status(status)
	op.Input = payload
	return &op, nil
}

func (s *PostgresStore) Get(ctx context.Context, id string) (*Operation, error) {
	var op Operation
	var status string
	var payload []byte
	err := s.pool.QueryRow(ctx, `SELECT id, kind, resource_type, resource_id, status,
		COALESCE(error,''), COALESCE(input,'{}'::jsonb), COALESCE(idempotency_key,''),
		desired_generation, observed_generation, created_at, updated_at, started_at, completed_at
		FROM operations WHERE id=$1`, id).Scan(
		&op.ID, &op.Kind, &op.ResourceType, &op.ResourceID, &status,
		&op.Error, &payload, &op.IdempotencyKey,
		&op.DesiredGen, &op.ObservedGen, &op.CreatedAt, &op.UpdatedAt, &op.StartedAt, &op.CompletedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	op.Status = Status(status)
	op.Input = payload
	return &op, nil
}

func (s *PostgresStore) ListByResource(ctx context.Context, resourceType, resourceID string) ([]Operation, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, kind, resource_type, resource_id, status,
		COALESCE(error,''), COALESCE(input,'{}'::jsonb), COALESCE(idempotency_key,''),
		desired_generation, observed_generation, created_at, updated_at, started_at, completed_at
		FROM operations WHERE resource_type=$1 AND resource_id=$2 ORDER BY created_at DESC`, resourceType, resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ops []Operation
	for rows.Next() {
		var op Operation
		var status string
		var payload []byte
		if err := rows.Scan(&op.ID, &op.Kind, &op.ResourceType, &op.ResourceID, &status,
			&op.Error, &payload, &op.IdempotencyKey,
			&op.DesiredGen, &op.ObservedGen, &op.CreatedAt, &op.UpdatedAt, &op.StartedAt, &op.CompletedAt); err != nil {
			return nil, err
		}
		op.Status = Status(status)
		op.Input = payload
		ops = append(ops, op)
	}
	return ops, rows.Err()
}

func (s *PostgresStore) ListPending(ctx context.Context, limit int) ([]Operation, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, kind, resource_type, resource_id, status,
		COALESCE(error,''), COALESCE(input,'{}'::jsonb), COALESCE(idempotency_key,''),
		desired_generation, observed_generation, created_at, updated_at, started_at, completed_at
		FROM operations WHERE status IN ('queued','retrying')
		ORDER BY CASE WHEN status='retrying' THEN 0 ELSE 1 END, created_at ASC LIMIT $1 FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ops []Operation
	for rows.Next() {
		var op Operation
		var status string
		var payload []byte
		if err := rows.Scan(&op.ID, &op.Kind, &op.ResourceType, &op.ResourceID, &status,
			&op.Error, &payload, &op.IdempotencyKey,
			&op.DesiredGen, &op.ObservedGen, &op.CreatedAt, &op.UpdatedAt, &op.StartedAt, &op.CompletedAt); err != nil {
			return nil, err
		}
		op.Status = Status(status)
		op.Input = payload
		ops = append(ops, op)
	}
	return ops, rows.Err()
}

func (s *PostgresStore) UpdateStatus(ctx context.Context, id string, status Status, errMsg string) error {
	now := time.Now().UTC()
	switch status {
	case StatusRunning:
		_, err := s.pool.Exec(ctx, `UPDATE operations SET status='running', started_at=COALESCE(started_at,$2), updated_at=$2 WHERE id=$1`, id, now)
		return err
	case StatusSucceeded:
		_, err := s.pool.Exec(ctx, `UPDATE operations SET status='succeeded', completed_at=$2, updated_at=$2 WHERE id=$1`, id, now)
		return err
	case StatusFailed:
		_, err := s.pool.Exec(ctx, `UPDATE operations SET status='failed', error=$3, completed_at=$2, updated_at=$2 WHERE id=$1`, id, now, errMsg)
		return err
	case StatusRetrying:
		_, err := s.pool.Exec(ctx, `UPDATE operations SET status='retrying', error=$3, updated_at=$2 WHERE id=$1`, id, now, errMsg)
		return err
	default:
		_, err := s.pool.Exec(ctx, `UPDATE operations SET status=$2, updated_at=NOW() WHERE id=$1`, id, string(status))
		return err
	}
}

func (s *PostgresStore) Cancel(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `UPDATE operations SET status='cancelled', completed_at=$2, updated_at=$2 WHERE id=$1 AND status NOT IN ('succeeded','failed','cancelled')`, id, now)
	return err
}
