package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool: pool} }

func (s *PostgresStore) Enqueue(ctx context.Context, job *Job) error {
	if job.AvailableAt.IsZero() {
		job.AvailableAt = time.Now().UTC()
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO job_queue
		(id,type,status,server_id,node_id,payload,priority,max_retries,idempotency_key,available_at,created_at)
		VALUES ($1,$2,$3,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,$6,$7,$8,NULLIF($9,''),$10,$11)
		ON CONFLICT DO NOTHING`, job.ID, string(job.Type), string(job.Status), job.ServerID, job.NodeID,
		[]byte(job.Payload), job.Priority, job.MaxRetries, job.IdempotencyKey, job.AvailableAt, job.CreatedAt)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO operations(id,kind,resource_type,resource_id,status,idempotency_key,input,created_at,updated_at)
		VALUES($1,$2,'server',$3,'queued',NULLIF($4,''),$5,$6,$6) ON CONFLICT DO NOTHING`, job.ID, string(job.Type), job.ServerID,
		job.IdempotencyKey, []byte(job.Payload), job.CreatedAt)
	if err != nil {
		return err
	}
	stepID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("forge-operation-step:"+job.ID)).String()
	_, err = tx.Exec(ctx, `INSERT INTO operation_steps(id,operation_id,name,position,status,max_attempts)
		VALUES($1,$2,'execute',0,'queued',$3) ON CONFLICT DO NOTHING`, stepID, job.ID, job.MaxRetries)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *PostgresStore) Dequeue(ctx context.Context, nodeID, workerID string, lease time.Duration) (*Job, error) {
	var job Job
	var payload []byte
	var status, jobType string
	err := s.pool.QueryRow(ctx, `WITH candidate AS (
		SELECT id,status FROM job_queue
		WHERE ((status='pending' AND available_at<=NOW()) OR (status='running' AND locked_until<NOW()))
		AND ($1='' OR node_id=NULLIF($1,'')::uuid)
		ORDER BY priority DESC,available_at ASC,created_at ASC LIMIT 1 FOR UPDATE SKIP LOCKED)
		UPDATE job_queue j SET status='running',started_at=COALESCE(started_at,NOW()),locked_by=$2,
		locked_until=NOW()+$3::interval,last_heartbeat_at=NOW(),
		retry_count=j.retry_count+CASE WHEN candidate.status='running' THEN 1 ELSE 0 END
		FROM candidate WHERE j.id=candidate.id
		RETURNING j.id,j.type,j.status,COALESCE(j.server_id::text,''),COALESCE(j.node_id::text,''),j.payload,
		j.priority,j.max_retries,j.retry_count,COALESCE(j.idempotency_key,''),j.available_at,j.locked_by,
		j.locked_until,j.created_at,j.started_at`, nodeID, workerID, lease.String()).Scan(
		&job.ID, &jobType, &status, &job.ServerID, &job.NodeID, &payload, &job.Priority, &job.MaxRetries,
		&job.RetryCount, &job.IdempotencyKey, &job.AvailableAt, &job.LockedBy, &job.LockedUntil, &job.CreatedAt, &job.StartedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	job.Type, job.Status, job.Payload = JobType(jobType), JobStatus(status), append(job.Payload[:0], payload...)
	_, _ = s.pool.Exec(ctx, `UPDATE operations SET status='running',started_at=COALESCE(started_at,NOW()),updated_at=NOW() WHERE id=$1`, job.ID)
	_, _ = s.pool.Exec(ctx, `UPDATE operation_steps SET status='running',started_at=COALESCE(started_at,NOW()) WHERE operation_id=$1 AND position=0`, job.ID)
	stepID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("forge-operation-step:"+job.ID)).String()
	attemptID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("forge-operation-attempt:"+job.ID+":"+fmt.Sprint(job.RetryCount+1))).String()
	_, _ = s.pool.Exec(ctx, `INSERT INTO operation_attempts(id,operation_step_id,attempt,status,worker_id)
		VALUES($1,$2,$3,'running',$4) ON CONFLICT DO NOTHING`, attemptID, stepID, job.RetryCount+1, workerID)
	return &job, nil
}

func (s *PostgresStore) Acknowledge(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE job_queue SET status='completed',completed_at=$2,locked_by=NULL,locked_until=NULL WHERE id=$1`, id, time.Now().UTC())
	if err != nil {
		return err
	}
	if _, err = s.pool.Exec(ctx, `UPDATE operations SET status='succeeded',observed_generation=desired_generation,completed_at=NOW(),updated_at=NOW() WHERE id=$1`, id); err != nil {
		return err
	}
	if _, err = s.pool.Exec(ctx, `UPDATE operation_steps SET status='succeeded',completed_at=NOW() WHERE operation_id=$1 AND position=0`, id); err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE operation_attempts SET status='succeeded',completed_at=NOW() WHERE operation_step_id=(SELECT id FROM operation_steps WHERE operation_id=$1 AND position=0) AND completed_at IS NULL`, id)
	return err
}

func (s *PostgresStore) Fail(ctx context.Context, id string, jobErr error) error {
	msg := ""
	if jobErr != nil {
		msg = jobErr.Error()
	}
	_, err := s.pool.Exec(ctx, `UPDATE job_queue SET status='failed',error=$2,completed_at=$3,locked_by=NULL,locked_until=NULL WHERE id=$1`, id, msg, time.Now().UTC())
	if err != nil {
		return err
	}
	if _, err = s.pool.Exec(ctx, `UPDATE operations SET status='failed',error=$2,completed_at=NOW(),updated_at=NOW() WHERE id=$1`, id, msg); err != nil {
		return err
	}
	if _, err = s.pool.Exec(ctx, `UPDATE operation_steps SET status='failed',completed_at=NOW() WHERE operation_id=$1 AND position=0`, id); err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE operation_attempts SET status='failed',error=$2,completed_at=NOW() WHERE operation_step_id=(SELECT id FROM operation_steps WHERE operation_id=$1 AND position=0) AND completed_at IS NULL`, id, msg)
	return err
}

func (s *PostgresStore) Retry(ctx context.Context, id string, jobErr error, availableAt time.Time) error {
	msg := ""
	if jobErr != nil {
		msg = jobErr.Error()
	}
	_, err := s.pool.Exec(ctx, `UPDATE job_queue SET status='pending',retry_count=retry_count+1,error=$2,
		available_at=$3,locked_by=NULL,locked_until=NULL,last_heartbeat_at=NULL WHERE id=$1`, id, msg, availableAt)
	if err != nil {
		return err
	}
	if _, err = s.pool.Exec(ctx, `UPDATE operations SET status='retrying',error=$2,updated_at=NOW() WHERE id=$1`, id, msg); err != nil {
		return err
	}
	if _, err = s.pool.Exec(ctx, `UPDATE operation_steps SET status='retrying' WHERE operation_id=$1 AND position=0`, id); err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `UPDATE operation_attempts SET status='failed',error=$2,completed_at=NOW() WHERE operation_step_id=(SELECT id FROM operation_steps WHERE operation_id=$1 AND position=0) AND completed_at IS NULL`, id, msg)
	return err
}

func (s *PostgresStore) Heartbeat(ctx context.Context, id, workerID string, lease time.Duration) error {
	_, err := s.pool.Exec(ctx, `UPDATE job_queue SET locked_until=NOW()+$3::interval,last_heartbeat_at=NOW()
		WHERE id=$1 AND status='running' AND locked_by=$2`, id, workerID, lease.String())
	return err
}

func (s *PostgresStore) ListPending(ctx context.Context, nodeID string) ([]Job, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,type,status,COALESCE(server_id::text,''),COALESCE(node_id::text,''),payload,
		priority,max_retries,retry_count,COALESCE(idempotency_key,''),available_at,created_at FROM job_queue
		WHERE status='pending' AND ($1='' OR node_id=NULLIF($1,'')::uuid) ORDER BY priority DESC,available_at,created_at`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []Job
	for rows.Next() {
		var j Job
		var typ, status string
		var payload []byte
		if err := rows.Scan(&j.ID, &typ, &status, &j.ServerID, &j.NodeID, &payload, &j.Priority, &j.MaxRetries, &j.RetryCount,
			&j.IdempotencyKey, &j.AvailableAt, &j.CreatedAt); err != nil {
			return nil, err
		}
		j.Type, j.Status, j.Payload = JobType(typ), JobStatus(status), append(j.Payload[:0], payload...)
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (s *PostgresStore) GetJob(ctx context.Context, id string) (*Job, error) {
	var j Job
	var typ, status string
	var payload []byte
	err := s.pool.QueryRow(ctx, `SELECT id,type,status,COALESCE(server_id::text,''),COALESCE(node_id::text,''),payload,
		priority,max_retries,retry_count,COALESCE(idempotency_key,''),available_at,COALESCE(locked_by,''),locked_until,created_at,
		started_at,completed_at,COALESCE(error,'') FROM job_queue WHERE id=$1`, id).Scan(&j.ID, &typ, &status, &j.ServerID,
		&j.NodeID, &payload, &j.Priority, &j.MaxRetries, &j.RetryCount, &j.IdempotencyKey, &j.AvailableAt, &j.LockedBy,
		&j.LockedUntil, &j.CreatedAt, &j.StartedAt, &j.CompletedAt, &j.Error)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	j.Type, j.Status, j.Payload = JobType(typ), JobStatus(status), append(j.Payload[:0], payload...)
	return &j, nil
}
