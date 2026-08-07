package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ScheduledTask is a cron-style command that runs inside a server's
// container context. Schedule is a standard 5-field cron expression; the
// scheduler keeps next_run_at current and dispatches when it comes due.
type ScheduledTask struct {
	ID        string     `json:"id"`
	ServerID  string     `json:"serverId"`
	Name      string     `json:"name"`
	Command   string     `json:"command"`
	Schedule  string     `json:"schedule"`
	Enabled   bool       `json:"enabled"`
	LastRunAt *time.Time `json:"lastRunAt,omitempty"`
	NextRunAt *time.Time `json:"nextRunAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	CreatedBy string     `json:"createdBy,omitempty"`
	// Denormalized result of the most recent task_runs row (empty when the
	// task has never run) so list responses can render status without N+1.
	LastRunStatus     string     `json:"lastRunStatus,omitempty"`
	LastRunExitCode   *int       `json:"lastRunExitCode,omitempty"`
	LastRunFinishedAt *time.Time `json:"lastRunFinishedAt,omitempty"`
}

// TaskRun records one execution of a ScheduledTask.
type TaskRun struct {
	ID         string     `json:"id"`
	TaskID     string     `json:"taskId"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	ExitCode   *int       `json:"exitCode,omitempty"`
	Output     string     `json:"output"`
	Status     string     `json:"status"`
}

// Run status values stored in task_runs.status.
const (
	TaskRunStatusRunning = "running"
	TaskRunStatusSuccess = "success"
	TaskRunStatusFailed  = "failed"
)

// CreateScheduledTaskRequest carries an already-validated task row. NextRunAt
// is computed by the caller because cron parsing lives in the service layer.
type CreateScheduledTaskRequest struct {
	ServerID  string
	Name      string
	Command   string
	Schedule  string
	Enabled   bool
	NextRunAt *time.Time
	CreatedBy string
}

// scheduledTasksLeaderName is the singleton lease key for the scheduler.
const scheduledTasksLeaderName = "scheduled-tasks"

const scheduledTaskColumns = `
	t.id::text, t.server_id::text, t.name, t.command, t.schedule, t.enabled,
	t.last_run_at, t.next_run_at, t.created_at, t.updated_at, COALESCE(t.created_by::text, ''),
	COALESCE(r.status, ''), r.exit_code, r.finished_at`

// scheduledTaskFromSQL joins the latest run so every read carries the
// last-run result the UI needs. The lateral keeps it to one indexed lookup.
const scheduledTaskFromSQL = `
	FROM scheduled_tasks t
	LEFT JOIN LATERAL (
		SELECT tr.status, tr.exit_code, tr.finished_at
		FROM task_runs tr
		WHERE tr.task_id = t.id
		ORDER BY tr.started_at DESC
		LIMIT 1
	) r ON true`

func scanScheduledTask(row pgx.Row) (ScheduledTask, error) {
	var task ScheduledTask
	var lastFinished *time.Time
	err := row.Scan(&task.ID, &task.ServerID, &task.Name, &task.Command, &task.Schedule, &task.Enabled,
		&task.LastRunAt, &task.NextRunAt, &task.CreatedAt, &task.UpdatedAt, &task.CreatedBy,
		&task.LastRunStatus, &task.LastRunExitCode, &lastFinished)
	task.LastRunFinishedAt = lastFinished
	return task, err
}

func (s *Store) CreateScheduledTask(ctx context.Context, req CreateScheduledTaskRequest) (ScheduledTask, error) {
	id := uuid.NewString()
	var createdBy any
	if req.CreatedBy != "" {
		createdBy = req.CreatedBy
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO scheduled_tasks (id, server_id, name, command, schedule, enabled, next_run_at, created_by)
		VALUES ($1, $2::uuid, $3, $4, $5, $6, $7, $8::uuid)
	`, id, req.ServerID, req.Name, req.Command, req.Schedule, req.Enabled, req.NextRunAt, createdBy)
	if err != nil {
		return ScheduledTask{}, err
	}
	return s.GetScheduledTask(ctx, id)
}

func (s *Store) GetScheduledTask(ctx context.Context, id string) (ScheduledTask, error) {
	return scanScheduledTask(s.db.QueryRow(ctx, `SELECT `+scheduledTaskColumns+scheduledTaskFromSQL+` WHERE t.id = $1`, id))
}

func (s *Store) ListScheduledTasksByServer(ctx context.Context, serverID string) ([]ScheduledTask, error) {
	rows, err := s.db.Query(ctx, `SELECT `+scheduledTaskColumns+scheduledTaskFromSQL+`
		WHERE t.server_id = $1::uuid
		ORDER BY t.created_at DESC`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []ScheduledTask{}
	for rows.Next() {
		task, err := scanScheduledTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// UpdateScheduledTaskRequest carries patch semantics: nil fields are left as-is.
type UpdateScheduledTaskRequest struct {
	Name      *string
	Command   *string
	Schedule  *string
	Enabled   *bool
	NextRunAt **time.Time
}

func (s *Store) UpdateScheduledTask(ctx context.Context, id string, req UpdateScheduledTaskRequest) (ScheduledTask, error) {
	existing, err := s.GetScheduledTask(ctx, id)
	if err != nil {
		return ScheduledTask{}, err
	}
	name := existing.Name
	if req.Name != nil {
		name = *req.Name
	}
	command := existing.Command
	if req.Command != nil {
		command = *req.Command
	}
	schedule := existing.Schedule
	if req.Schedule != nil {
		schedule = *req.Schedule
	}
	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	nextRunAt := existing.NextRunAt
	if req.NextRunAt != nil {
		nextRunAt = *req.NextRunAt
	}
	_, err = s.db.Exec(ctx, `
		UPDATE scheduled_tasks
		SET name = $1, command = $2, schedule = $3, enabled = $4, next_run_at = $5, updated_at = NOW()
		WHERE id = $6`, name, command, schedule, enabled, nextRunAt, id)
	if err != nil {
		return ScheduledTask{}, err
	}
	return s.GetScheduledTask(ctx, id)
}

func (s *Store) DeleteScheduledTask(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM scheduled_tasks WHERE id = $1`, id)
	return err
}

// UpdateScheduledTaskRunMeta records dispatch times (last_run_at / next_run_at)
// without touching the user-facing fields; either pointer may be nil.
func (s *Store) UpdateScheduledTaskRunMeta(ctx context.Context, id string, lastRunAt, nextRunAt *time.Time) error {
	_, err := s.db.Exec(ctx, `
		UPDATE scheduled_tasks
		SET last_run_at = COALESCE($2, last_run_at), next_run_at = COALESCE($3, next_run_at), updated_at = NOW()
		WHERE id = $1`, id, lastRunAt, nextRunAt)
	return err
}

// ListDueScheduledTasks returns enabled tasks whose next occurrence has
// arrived, earliest first — the scheduler's core query (index
// idx_scheduled_tasks_due covers it).
func (s *Store) ListDueScheduledTasks(ctx context.Context, now time.Time, limit int) ([]ScheduledTask, error) {
	if limit <= 0 || limit > 256 {
		limit = 64
	}
	return queryScheduledTasks(s.db.Query(ctx, `SELECT `+scheduledTaskColumns+scheduledTaskFromSQL+`
		WHERE t.enabled AND t.next_run_at IS NOT NULL AND t.next_run_at <= $1
		ORDER BY t.next_run_at ASC
		LIMIT $2`, now, limit))
}

// ListEnabledScheduledTasksWithoutNextRun backs the startup pass that
// initializes next_run_at for enabled tasks that have none (e.g. rows
// created while the API was down, or a schedule edit made by another node).
func (s *Store) ListEnabledScheduledTasksWithoutNextRun(ctx context.Context) ([]ScheduledTask, error) {
	return queryScheduledTasks(s.db.Query(ctx, `SELECT `+scheduledTaskColumns+scheduledTaskFromSQL+`
		WHERE t.enabled AND t.next_run_at IS NULL
		ORDER BY t.created_at ASC`))
}

func queryScheduledTasks(rows pgx.Rows, err error) ([]ScheduledTask, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []ScheduledTask{}
	for rows.Next() {
		task, err := scanScheduledTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// ---- task runs ----

func (s *Store) CreateTaskRun(ctx context.Context, taskID string) (TaskRun, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO task_runs (id, task_id, status)
		VALUES ($1, $2::uuid, $3)`, id, taskID, TaskRunStatusRunning)
	if err != nil {
		return TaskRun{}, err
	}
	return s.GetTaskRun(ctx, id)
}

func (s *Store) GetTaskRun(ctx context.Context, id string) (TaskRun, error) {
	var run TaskRun
	var finishedAt *time.Time
	var exitCode *int
	err := s.db.QueryRow(ctx, `
		SELECT id::text, task_id::text, started_at, finished_at, exit_code, output, status
		FROM task_runs WHERE id = $1`, id).
		Scan(&run.ID, &run.TaskID, &run.StartedAt, &finishedAt, &exitCode, &run.Output, &run.Status)
	if err != nil {
		return TaskRun{}, err
	}
	run.FinishedAt = finishedAt
	run.ExitCode = exitCode
	return run, nil
}

func (s *Store) CompleteTaskRun(ctx context.Context, id string, status string, exitCode int, output string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE task_runs
		SET status = $1, exit_code = $2, output = $3, finished_at = NOW()
		WHERE id = $4`, status, exitCode, output, id)
	return err
}

func (s *Store) ListTaskRuns(ctx context.Context, taskID string, limit int) ([]TaskRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, task_id::text, started_at, finished_at, exit_code, output, status
		FROM task_runs
		WHERE task_id = $1::uuid
		ORDER BY started_at DESC
		LIMIT $2`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := []TaskRun{}
	for rows.Next() {
		var run TaskRun
		var finishedAt *time.Time
		var exitCode *int
		if err := rows.Scan(&run.ID, &run.TaskID, &run.StartedAt, &finishedAt, &exitCode, &run.Output, &run.Status); err != nil {
			return nil, err
		}
		run.FinishedAt = finishedAt
		run.ExitCode = exitCode
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

// ---- scheduler leader lease ----

// TryAcquireScheduledTasksLease attempts to take (or renew) the singleton
// scheduler lease. It returns true only when this instance holds the lease,
// so exactly one API replica runs the scheduler loop; a crashed leader stops
// renewing and its lease expires after ttl.
func (s *Store) TryAcquireScheduledTasksLease(ctx context.Context, instanceID string, ttl time.Duration) (bool, error) {
	var holder string
	err := s.db.QueryRow(ctx, `
		INSERT INTO scheduled_tasks_leader (name, instance_id, acquired_at, expires_at)
		VALUES ($1, $2, NOW(), NOW() + make_interval(secs => $3))
		ON CONFLICT (name) DO UPDATE
		SET instance_id = EXCLUDED.instance_id, acquired_at = NOW(), expires_at = EXCLUDED.expires_at
		WHERE scheduled_tasks_leader.expires_at < NOW() OR scheduled_tasks_leader.instance_id = EXCLUDED.instance_id
		RETURNING instance_id`, scheduledTasksLeaderName, instanceID, ttl.Seconds()).Scan(&holder)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return holder == instanceID, nil
}

// ReleaseScheduledTasksLease drops the lease when this instance held it so a
// successor does not wait for the TTL after a graceful shutdown.
func (s *Store) ReleaseScheduledTasksLease(ctx context.Context, instanceID string) error {
	_, err := s.db.Exec(ctx, `
		DELETE FROM scheduled_tasks_leader WHERE name = $1 AND instance_id = $2`, scheduledTasksLeaderName, instanceID)
	return err
}
