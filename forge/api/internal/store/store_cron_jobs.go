package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type CronJob struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Schedule        string    `json:"schedule"`
	Command         string    `json:"command"`
	Type            string    `json:"type"`
	TargetType      string    `json:"targetType"`
	TargetID        string    `json:"targetId"`
	Enabled         bool      `json:"enabled"`
	RetryCount      int       `json:"retryCount"`
	TimeoutSeconds  int       `json:"timeoutSeconds"`
	NotifyOnFailure bool      `json:"notifyOnFailure"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type CronJobExecution struct {
	ID         string     `json:"id"`
	CronJobID  string     `json:"cronJobId"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Status     string     `json:"status"`
	ExitCode   *int       `json:"exitCode,omitempty"`
	Output     string     `json:"output"`
	Error      string     `json:"error"`
	DurationMs *int       `json:"durationMs,omitempty"`
}

type CreateCronJobRequest struct {
	Name            string
	Description     string
	Schedule        string
	Command         string
	Type            string
	TargetType      string
	TargetID        string
	Enabled         bool
	RetryCount      int
	TimeoutSeconds  int
	NotifyOnFailure bool
}

type UpdateCronJobRequest struct {
	Name            *string
	Description     *string
	Schedule        *string
	Command         *string
	Type            *string
	TargetType      *string
	TargetID        *string
	Enabled         *bool
	RetryCount      *int
	TimeoutSeconds  *int
	NotifyOnFailure *bool
}

func (s *Store) CreateCronJob(ctx context.Context, req CreateCronJobRequest) (CronJob, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO cron_jobs (id, name, description, schedule, command, type, target_type, target_id, enabled, retry_count, timeout_seconds, notify_on_failure)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8::text, '')::uuid, $9, $10, $11, $12)
	`, id, req.Name, req.Description, req.Schedule, req.Command, req.Type, req.TargetType, req.TargetID, req.Enabled, req.RetryCount, req.TimeoutSeconds, req.NotifyOnFailure)
	if err != nil {
		return CronJob{}, err
	}
	return s.GetCronJob(ctx, id)
}

func (s *Store) GetCronJob(ctx context.Context, id string) (CronJob, error) {
	var job CronJob
	var desc, targetType, targetID string
	err := s.db.QueryRow(ctx, `
		SELECT id::text, name, COALESCE(description, ''), schedule, command, type, COALESCE(target_type, ''), COALESCE(target_id::text, ''), enabled, retry_count, timeout_seconds, notify_on_failure, created_at, updated_at
		FROM cron_jobs WHERE id = $1
	`, id).Scan(&job.ID, &job.Name, &desc, &job.Schedule, &job.Command, &job.Type, &targetType, &targetID, &job.Enabled, &job.RetryCount, &job.TimeoutSeconds, &job.NotifyOnFailure, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return CronJob{}, err
	}
	job.Description = desc
	job.TargetType = targetType
	job.TargetID = targetID
	return job, nil
}

func (s *Store) ListCronJobs(ctx context.Context) ([]CronJob, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, COALESCE(description, ''), schedule, command, type, COALESCE(target_type, ''), COALESCE(target_id::text, ''), enabled, retry_count, timeout_seconds, notify_on_failure, created_at, updated_at
		FROM cron_jobs ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := []CronJob{}
	for rows.Next() {
		var job CronJob
		var desc, targetType, targetID string
		if err := rows.Scan(&job.ID, &job.Name, &desc, &job.Schedule, &job.Command, &job.Type, &targetType, &targetID, &job.Enabled, &job.RetryCount, &job.TimeoutSeconds, &job.NotifyOnFailure, &job.CreatedAt, &job.UpdatedAt); err != nil {
			return nil, err
		}
		job.Description = desc
		job.TargetType = targetType
		job.TargetID = targetID
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Store) ListEnabledCronJobs(ctx context.Context) ([]CronJob, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, COALESCE(description, ''), schedule, command, type, COALESCE(target_type, ''), COALESCE(target_id::text, ''), enabled, retry_count, timeout_seconds, notify_on_failure, created_at, updated_at
		FROM cron_jobs WHERE enabled = true ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := []CronJob{}
	for rows.Next() {
		var job CronJob
		var desc, targetType, targetID string
		if err := rows.Scan(&job.ID, &job.Name, &desc, &job.Schedule, &job.Command, &job.Type, &targetType, &targetID, &job.Enabled, &job.RetryCount, &job.TimeoutSeconds, &job.NotifyOnFailure, &job.CreatedAt, &job.UpdatedAt); err != nil {
			return nil, err
		}
		job.Description = desc
		job.TargetType = targetType
		job.TargetID = targetID
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *Store) UpdateCronJob(ctx context.Context, id string, req UpdateCronJobRequest) (CronJob, error) {
	existing, err := s.GetCronJob(ctx, id)
	if err != nil {
		return CronJob{}, err
	}
	name := existing.Name
	if req.Name != nil {
		name = *req.Name
	}
	description := existing.Description
	if req.Description != nil {
		description = *req.Description
	}
	schedule := existing.Schedule
	if req.Schedule != nil {
		schedule = *req.Schedule
	}
	command := existing.Command
	if req.Command != nil {
		command = *req.Command
	}
	jobType := existing.Type
	if req.Type != nil {
		jobType = *req.Type
	}
	targetType := existing.TargetType
	if req.TargetType != nil {
		targetType = *req.TargetType
	}
	targetID := existing.TargetID
	if req.TargetID != nil {
		targetID = *req.TargetID
	}
	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	retryCount := existing.RetryCount
	if req.RetryCount != nil {
		retryCount = *req.RetryCount
	}
	timeoutSeconds := existing.TimeoutSeconds
	if req.TimeoutSeconds != nil {
		timeoutSeconds = *req.TimeoutSeconds
	}
	notifyOnFailure := existing.NotifyOnFailure
	if req.NotifyOnFailure != nil {
		notifyOnFailure = *req.NotifyOnFailure
	}

	_, err = s.db.Exec(ctx, `
		UPDATE cron_jobs SET name = $1, description = $2, schedule = $3, command = $4, type = $5, target_type = NULLIF($6, ''), target_id = NULLIF($7::text, '')::uuid, enabled = $8, retry_count = $9, timeout_seconds = $10, notify_on_failure = $11, updated_at = NOW()
		WHERE id = $12
	`, name, description, schedule, command, jobType, targetType, targetID, enabled, retryCount, timeoutSeconds, notifyOnFailure, id)
	if err != nil {
		return CronJob{}, err
	}
	return s.GetCronJob(ctx, id)
}

func (s *Store) DeleteCronJob(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM cron_jobs WHERE id = $1`, id)
	return err
}

func (s *Store) ToggleCronJob(ctx context.Context, id string) (CronJob, error) {
	_, err := s.db.Exec(ctx, `UPDATE cron_jobs SET enabled = NOT enabled, updated_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return CronJob{}, err
	}
	return s.GetCronJob(ctx, id)
}

func (s *Store) CreateCronJobExecution(ctx context.Context, cronJobID string) (CronJobExecution, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO cron_job_executions (id, cron_job_id, status)
		VALUES ($1, $2, 'running')
	`, id, cronJobID)
	if err != nil {
		return CronJobExecution{}, err
	}
	return s.GetCronJobExecution(ctx, id)
}

func (s *Store) GetCronJobExecution(ctx context.Context, id string) (CronJobExecution, error) {
	var exec CronJobExecution
	var finishedAt *time.Time
	var exitCode *int
	var output, errStr string
	var durationMs *int
	err := s.db.QueryRow(ctx, `
		SELECT id::text, cron_job_id::text, started_at, finished_at, status, exit_code, COALESCE(output, ''), COALESCE(error, ''), duration_ms
		FROM cron_job_executions WHERE id = $1
	`, id).Scan(&exec.ID, &exec.CronJobID, &exec.StartedAt, &finishedAt, &exec.Status, &exitCode, &output, &errStr, &durationMs)
	if err != nil {
		return CronJobExecution{}, err
	}
	exec.FinishedAt = finishedAt
	exec.ExitCode = exitCode
	exec.Output = output
	exec.Error = errStr
	exec.DurationMs = durationMs
	return exec, nil
}

func (s *Store) ListCronJobExecutions(ctx context.Context, cronJobID string, limit int) ([]CronJobExecution, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, cron_job_id::text, started_at, finished_at, status, exit_code, COALESCE(output, ''), COALESCE(error, ''), duration_ms
		FROM cron_job_executions WHERE cron_job_id = $1
		ORDER BY started_at DESC LIMIT $2
	`, cronJobID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	execs := []CronJobExecution{}
	for rows.Next() {
		var exec CronJobExecution
		var finishedAt *time.Time
		var exitCode *int
		var output, errStr string
		var durationMs *int
		if err := rows.Scan(&exec.ID, &exec.CronJobID, &exec.StartedAt, &finishedAt, &exec.Status, &exitCode, &output, &errStr, &durationMs); err != nil {
			return nil, err
		}
		exec.FinishedAt = finishedAt
		exec.ExitCode = exitCode
		exec.Output = output
		exec.Error = errStr
		exec.DurationMs = durationMs
		execs = append(execs, exec)
	}
	return execs, rows.Err()
}

func (s *Store) CompleteCronJobExecution(ctx context.Context, id string, status string, exitCode int, output, errStr string, durationMs int) error {
	_, err := s.db.Exec(ctx, `
		UPDATE cron_job_executions SET status = $1, exit_code = $2, output = $3, error = $4, duration_ms = $5, finished_at = NOW()
		WHERE id = $6
	`, status, exitCode, output, errStr, durationMs, id)
	return err
}

func (s *Store) ListAllCronJobExecutions(ctx context.Context, limit int) ([]CronJobExecution, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT e.id::text, e.cron_job_id::text, e.started_at, e.finished_at, e.status, e.exit_code, COALESCE(e.output, ''), COALESCE(e.error, ''), e.duration_ms
		FROM cron_job_executions e
		ORDER BY e.started_at DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	execs := []CronJobExecution{}
	for rows.Next() {
		var exec CronJobExecution
		var finishedAt *time.Time
		var exitCode *int
		var output, errStr string
		var durationMs *int
		if err := rows.Scan(&exec.ID, &exec.CronJobID, &exec.StartedAt, &finishedAt, &exec.Status, &exitCode, &output, &errStr, &durationMs); err != nil {
			return nil, err
		}
		exec.FinishedAt = finishedAt
		exec.ExitCode = exitCode
		exec.Output = output
		exec.Error = errStr
		exec.DurationMs = durationMs
		execs = append(execs, exec)
	}
	return execs, rows.Err()
}
