package queue

import (
	"context"
	"encoding/json"
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

func (s *PostgresStore) Enqueue(ctx context.Context, job *Job) error {
	data, _ := json.Marshal(job.Payload)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO job_queue (id, type, status, server_id, node_id, payload, priority, max_retries, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, job.ID, string(job.Type), string(job.Status), job.ServerID, job.NodeID, data, job.Priority, job.MaxRetries, job.CreatedAt)
	return err
}

func (s *PostgresStore) Dequeue(ctx context.Context, nodeID string) (*Job, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var job Job
	var payload []byte
	var status, jobType string
	err = tx.QueryRow(ctx, `
		SELECT id, type, status, server_id, node_id, payload, priority, max_retries, retry_count, created_at
		FROM job_queue
		WHERE status = 'pending' AND ($1 = '' OR node_id = $1)
		ORDER BY priority DESC, created_at ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`, nodeID).Scan(&job.ID, &jobType, &status, &job.ServerID, &job.NodeID, &payload,
		&job.Priority, &job.MaxRetries, &job.RetryCount, &job.CreatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	job.Type = JobType(jobType)
	job.Status = JobStatus(status)
	json.Unmarshal(payload, &job.Payload)

	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `UPDATE job_queue SET status = 'running', started_at = $2 WHERE id = $1`,
		job.ID, now)
	if err != nil {
		return nil, err
	}

	job.Status = JobStatusRunning
	job.StartedAt = &now

	return &job, tx.Commit(ctx)
}

func (s *PostgresStore) Acknowledge(ctx context.Context, jobID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE job_queue SET status = 'completed', completed_at = $2 WHERE id = $1
	`, jobID, time.Now().UTC())
	return err
}

func (s *PostgresStore) Fail(ctx context.Context, jobID string, errMsg error) error {
	msg := ""
	if errMsg != nil {
		msg = errMsg.Error()
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE job_queue SET status = 'failed', error = $2, completed_at = $3 WHERE id = $1
	`, jobID, msg, time.Now().UTC())
	return err
}

func (s *PostgresStore) ListPending(ctx context.Context, nodeID string) ([]Job, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, type, status, server_id, node_id, payload, priority, max_retries, retry_count, created_at
		FROM job_queue
		WHERE status = 'pending' AND ($1 = '' OR node_id = $1)
		ORDER BY priority DESC, created_at ASC
	`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var job Job
		var payload []byte
		var status, jobType string
		if err := rows.Scan(&job.ID, &jobType, &status, &job.ServerID, &job.NodeID, &payload,
			&job.Priority, &job.MaxRetries, &job.RetryCount, &job.CreatedAt); err != nil {
			return nil, err
		}
		job.Type = JobType(jobType)
		job.Status = JobStatus(status)
		json.Unmarshal(payload, &job.Payload)
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *PostgresStore) GetJob(ctx context.Context, jobID string) (*Job, error) {
	var job Job
	var payload []byte
	var status, jobType string
	err := s.pool.QueryRow(ctx, `
		SELECT id, type, status, server_id, node_id, payload, priority, max_retries, retry_count, created_at, started_at, completed_at, COALESCE(error, '')
		FROM job_queue
		WHERE id = $1
	`, jobID).Scan(&job.ID, &jobType, &status, &job.ServerID, &job.NodeID, &payload,
		&job.Priority, &job.MaxRetries, &job.RetryCount, &job.CreatedAt, &job.StartedAt, &job.CompletedAt, &job.Error)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	job.Type = JobType(jobType)
	job.Status = JobStatus(status)
	json.Unmarshal(payload, &job.Payload)
	return &job, nil
}
