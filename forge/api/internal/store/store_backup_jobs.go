package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type BackupJobFilter struct {
	ConfigurationID, JobType, ServerID, AppID, DatabaseID, VolumeID, Status, TriggeredBy, Search *string
	StartDate, EndDate                                                                           *time.Time
	Limit, Offset                                                                                int
}

type BackupJob struct {
	ID                string          `json:"id"`
	ConfigurationID   *string         `json:"configurationId,omitempty"`
	JobType           string          `json:"jobType"`
	ServerID          *string         `json:"serverId,omitempty"`
	AppID             *string         `json:"appId,omitempty"`
	DatabaseID        *string         `json:"databaseId,omitempty"`
	VolumeID          *string         `json:"volumeId,omitempty"`
	Name              string          `json:"name"`
	Description       string          `json:"description,omitempty"`
	Status            string          `json:"status"`
	StartedAt         *time.Time      `json:"startedAt,omitempty"`
	CompletedAt       *time.Time      `json:"completedAt,omitempty"`
	DurationSeconds   *int            `json:"durationSeconds,omitempty"`
	BytesProcessed    int64           `json:"bytesProcessed"`
	TotalBytes        *int64          `json:"totalBytes,omitempty"`
	CurrentPhase      string          `json:"currentPhase,omitempty"`
	ErrorMessage      *string         `json:"errorMessage,omitempty"`
	RetryCount        int             `json:"retryCount"`
	MaxRetries        int             `json:"maxRetries"`
	LastRetryAt       *time.Time      `json:"lastRetryAt,omitempty"`
	TriggeredBy       string          `json:"triggeredBy"`
	TriggeredByUserID *string         `json:"triggeredByUserId,omitempty"`
	NodeID            *string         `json:"nodeId,omitempty"`
	BeaconTaskID      *string         `json:"beaconTaskId,omitempty"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
	Data              json.RawMessage `json:"-"`
}

func (s *Store) CreateBackupJob(ctx context.Context, j *BackupJob) error {
	if j.ID == "" {
		j.ID = uuid.NewString()
	}
	now := time.Now()
	j.CreatedAt = now
	j.UpdatedAt = now
	if len(j.Data) == 0 {
		j.Data = json.RawMessage(`{}`)
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO backup_jobs (id, configuration_id, job_type, server_id, app_id, database_id, volume_id,
			name, description, status, started_at, completed_at, duration_seconds,
			bytes_processed, total_bytes, current_phase, error_message, retry_count, max_retries,
			last_retry_at, triggered_by, triggered_by_user_id, node_id, beacon_task_id,
			created_at, updated_at, data)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27)
	`, j.ID, j.ConfigurationID, j.JobType, j.ServerID, j.AppID, j.DatabaseID, j.VolumeID,
		j.Name, j.Description, j.Status, j.StartedAt, j.CompletedAt, j.DurationSeconds,
		j.BytesProcessed, j.TotalBytes, j.CurrentPhase, j.ErrorMessage, j.RetryCount, j.MaxRetries,
		j.LastRetryAt, j.TriggeredBy, j.TriggeredByUserID, j.NodeID, j.BeaconTaskID,
		j.CreatedAt, j.UpdatedAt, j.Data)
	return err
}

func (s *Store) ListAllBackupJobs(ctx context.Context) ([]BackupJob, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, configuration_id, job_type, server_id, app_id, database_id, volume_id,
			name, COALESCE(description, ''), status, started_at, completed_at, duration_seconds,
			bytes_processed, total_bytes, COALESCE(current_phase, ''), error_message, retry_count, max_retries,
			last_retry_at, triggered_by, triggered_by_user_id, node_id, beacon_task_id,
			created_at, updated_at, data
		FROM backup_jobs
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []BackupJob
	for rows.Next() {
		var j BackupJob
		if err := rows.Scan(&j.ID, &j.ConfigurationID, &j.JobType, &j.ServerID, &j.AppID, &j.DatabaseID, &j.VolumeID,
			&j.Name, &j.Description, &j.Status, &j.StartedAt, &j.CompletedAt, &j.DurationSeconds,
			&j.BytesProcessed, &j.TotalBytes, &j.CurrentPhase, &j.ErrorMessage, &j.RetryCount, &j.MaxRetries,
			&j.LastRetryAt, &j.TriggeredBy, &j.TriggeredByUserID, &j.NodeID, &j.BeaconTaskID,
			&j.CreatedAt, &j.UpdatedAt, &j.Data); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (s *Store) ListBackupJobs(ctx context.Context, filter BackupJobFilter) ([]BackupJob, int, error) {
	conditions := []string{"TRUE"}
	args := make([]any, 0, 12)
	add := func(column string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	for column, value := range map[string]*string{
		"configuration_id::text": filter.ConfigurationID,
		"job_type":               filter.JobType,
		"server_id::text":        filter.ServerID,
		"app_id::text":           filter.AppID,
		"database_id::text":      filter.DatabaseID,
		"volume_id::text":        filter.VolumeID,
		"status":                 filter.Status,
		"triggered_by":           filter.TriggeredBy,
	} {
		if value != nil {
			add(column, *value)
		}
	}
	if filter.Search != nil && strings.TrimSpace(*filter.Search) != "" {
		args = append(args, "%"+strings.TrimSpace(*filter.Search)+"%")
		conditions = append(conditions, fmt.Sprintf("(name ILIKE $%d OR description ILIKE $%d)", len(args), len(args)))
	}
	if filter.StartDate != nil {
		args = append(args, *filter.StartDate)
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", len(args)))
	}
	if filter.EndDate != nil {
		args = append(args, *filter.EndDate)
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", len(args)))
	}
	limit := filter.Limit
	if limit < 1 || limit > 200 {
		limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	args = append(args, limit, filter.Offset)
	query := fmt.Sprintf(`
		SELECT id, configuration_id, job_type, server_id, app_id, database_id, volume_id,
			name, COALESCE(description, ''), status, started_at, completed_at, duration_seconds,
			bytes_processed, total_bytes, COALESCE(current_phase, ''), error_message, retry_count, max_retries,
			last_retry_at, triggered_by, triggered_by_user_id, node_id, beacon_task_id,
			created_at, updated_at, data, COUNT(*) OVER()
		FROM backup_jobs
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, strings.Join(conditions, " AND "), len(args)-1, len(args))
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	jobs := make([]BackupJob, 0, limit)
	total := 0
	for rows.Next() {
		var j BackupJob
		if err := rows.Scan(&j.ID, &j.ConfigurationID, &j.JobType, &j.ServerID, &j.AppID, &j.DatabaseID, &j.VolumeID,
			&j.Name, &j.Description, &j.Status, &j.StartedAt, &j.CompletedAt, &j.DurationSeconds,
			&j.BytesProcessed, &j.TotalBytes, &j.CurrentPhase, &j.ErrorMessage, &j.RetryCount, &j.MaxRetries,
			&j.LastRetryAt, &j.TriggeredBy, &j.TriggeredByUserID, &j.NodeID, &j.BeaconTaskID,
			&j.CreatedAt, &j.UpdatedAt, &j.Data, &total); err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, j)
	}
	return jobs, total, rows.Err()
}

func (s *Store) UpdateBackupJobStatus(ctx context.Context, id, status string) error {
	_, err := s.db.Exec(ctx, `UPDATE backup_jobs SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	return err
}

// ClaimBackupJobForExecution atomically transitions a pending or failed backup
// job to running. Only one caller can win the transition, which prevents
// concurrent workers from executing the same job twice.
func (s *Store) ClaimBackupJobForExecution(ctx context.Context, id string) (bool, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE backup_jobs
		SET status = 'running', started_at = COALESCE(started_at, now()), updated_at = now()
		WHERE id = $1 AND status IN ('pending', 'failed')
	`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ListRetryableBackupJobs returns pending or failed backup jobs that still
// have retries remaining and whose backoff window has elapsed. Backoff grows
// exponentially with the retry count (2^retry_count minutes, capped at 30).
func (s *Store) ListRetryableBackupJobs(ctx context.Context, limit int) ([]BackupJob, error) {
	if limit <= 0 {
		limit = 5
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, configuration_id, job_type, server_id, app_id, database_id, volume_id,
			name, COALESCE(description, ''), status, started_at, completed_at, duration_seconds,
			bytes_processed, total_bytes, COALESCE(current_phase, ''), error_message, retry_count, max_retries,
			last_retry_at, triggered_by, triggered_by_user_id, node_id, beacon_task_id,
			created_at, updated_at, data
		FROM backup_jobs
		WHERE status IN ('pending', 'failed')
		  AND retry_count < max_retries
		  AND (last_retry_at IS NULL OR last_retry_at < now() - make_interval(mins => LEAST(1 << retry_count, 30)))
		ORDER BY created_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]BackupJob, 0, limit)
	for rows.Next() {
		var j BackupJob
		if err := rows.Scan(&j.ID, &j.ConfigurationID, &j.JobType, &j.ServerID, &j.AppID, &j.DatabaseID, &j.VolumeID,
			&j.Name, &j.Description, &j.Status, &j.StartedAt, &j.CompletedAt, &j.DurationSeconds,
			&j.BytesProcessed, &j.TotalBytes, &j.CurrentPhase, &j.ErrorMessage, &j.RetryCount, &j.MaxRetries,
			&j.LastRetryAt, &j.TriggeredBy, &j.TriggeredByUserID, &j.NodeID, &j.BeaconTaskID,
			&j.CreatedAt, &j.UpdatedAt, &j.Data); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// FailBackupJob retries apply a terminal or retryable failure using targeted
// column updates instead of a full-row overwrite.
func (s *Store) FailBackupJob(ctx context.Context, id, status string, retryCount int, lastRetryAt, completedAt *time.Time, errMsg string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE backup_jobs
		SET status = $2, error_message = CASE WHEN $7 = '' THEN error_message ELSE $7 END,
		    retry_count = $3, last_retry_at = $4,
		    completed_at = CASE WHEN $5 IS NULL THEN completed_at ELSE $5 END,
		    updated_at = now()
		WHERE id = $1
	`, id, status, retryCount, lastRetryAt, completedAt, errMsg)
	return err
}

// CompleteBackupJobSuccess applies a successful terminal state with targeted
// column updates.
func (s *Store) CompleteBackupJobSuccess(ctx context.Context, id string, durationSeconds *int, bytesProcessed int64) error {
	_, err := s.db.Exec(ctx, `
		UPDATE backup_jobs
		SET status = 'completed', completed_at = now(), duration_seconds = $2,
		    bytes_processed = $3, progress_percentage = 100, error_message = NULL,
		    updated_at = now()
		WHERE id = $1
	`, id, durationSeconds, bytesProcessed)
	return err
}

// UpdateBackupJobProgress updates non-terminal progress fields.
func (s *Store) UpdateBackupJobProgress(ctx context.Context, id, phase string, progressPct float64, bytesProcessed int64) error {
	_, err := s.db.Exec(ctx, `
		UPDATE backup_jobs
		SET current_phase = CASE WHEN $2 = '' THEN current_phase ELSE $2 END,
		    progress_percentage = $3, bytes_processed = $4, updated_at = now()
		WHERE id = $1
	`, id, phase, progressPct, bytesProcessed)
	return err
}

func (s *Store) UpdateBackupJob(ctx context.Context, j *BackupJob) error {
	j.UpdatedAt = time.Now().UTC()
	command, err := s.db.Exec(ctx, `
		UPDATE backup_jobs SET status=$2, started_at=$3, completed_at=$4,
			duration_seconds=$5, bytes_processed=$6, total_bytes=$7,
			current_phase=$8, error_message=$9, retry_count=$10,
			max_retries=$11, last_retry_at=$12, node_id=$13,
			beacon_task_id=$14, data=$15, updated_at=$16
		WHERE id=$1
	`, j.ID, j.Status, j.StartedAt, j.CompletedAt, j.DurationSeconds,
		j.BytesProcessed, j.TotalBytes, j.CurrentPhase, j.ErrorMessage,
		j.RetryCount, j.MaxRetries, j.LastRetryAt, j.NodeID, j.BeaconTaskID,
		j.Data, j.UpdatedAt)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteBackupJob(ctx context.Context, id string) error {
	command, err := s.db.Exec(ctx, `DELETE FROM backup_jobs WHERE id=$1 AND status NOT IN ('running')`, id)
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) GetBackupJob(ctx context.Context, id string) (BackupJob, error) {
	var j BackupJob
	err := s.db.QueryRow(ctx, `
		SELECT id, configuration_id, job_type, server_id, app_id, database_id, volume_id,
			name, COALESCE(description, ''), status, started_at, completed_at, duration_seconds,
			bytes_processed, total_bytes, COALESCE(current_phase, ''), error_message, retry_count, max_retries,
			last_retry_at, triggered_by, triggered_by_user_id, node_id, beacon_task_id,
			created_at, updated_at, data
		FROM backup_jobs
		WHERE id = $1
	`, id).Scan(&j.ID, &j.ConfigurationID, &j.JobType, &j.ServerID, &j.AppID, &j.DatabaseID, &j.VolumeID,
		&j.Name, &j.Description, &j.Status, &j.StartedAt, &j.CompletedAt, &j.DurationSeconds,
		&j.BytesProcessed, &j.TotalBytes, &j.CurrentPhase, &j.ErrorMessage, &j.RetryCount, &j.MaxRetries,
		&j.LastRetryAt, &j.TriggeredBy, &j.TriggeredByUserID, &j.NodeID, &j.BeaconTaskID,
		&j.CreatedAt, &j.UpdatedAt, &j.Data)
	return j, err
}
