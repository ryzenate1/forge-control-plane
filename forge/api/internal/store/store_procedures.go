package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Procedure struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	TenantID    *string          `json:"tenantId,omitempty"`
	Enabled     bool             `json:"enabled"`
	Steps       []ProcedureStep  `json:"steps,omitempty"`
	Schedule    *ProcedureSchedule `json:"schedule,omitempty"`
	CreatedAt   time.Time        `json:"createdAt"`
	UpdatedAt   time.Time        `json:"updatedAt"`
}

type ProcedureStep struct {
	ID                string         `json:"id"`
	ProcedureID       string         `json:"procedureId"`
	Position          int            `json:"position"`
	Name              string         `json:"name"`
	Action            string         `json:"action"`
	Config            map[string]any `json:"config"`
	MaxRetries        int            `json:"maxRetries"`
	TimeoutSeconds    int            `json:"timeoutSeconds"`
	RequiresApproval  bool           `json:"requiresApproval"`
	ContinueOnFailure bool           `json:"continueOnFailure"`
	RollbackEnabled   bool           `json:"rollbackEnabled"`
	CreatedAt         time.Time      `json:"createdAt"`
}

type ProcedureExecution struct {
	ID          string                    `json:"id"`
	ProcedureID string                    `json:"procedureId"`
	Status      string                    `json:"status"`
	Trigger     string                    `json:"trigger"`
	TenantID    *string                   `json:"tenantId,omitempty"`
	ActorID     *string                   `json:"actorId,omitempty"`
	StartedAt   *time.Time                `json:"startedAt,omitempty"`
	CompletedAt *time.Time                `json:"completedAt,omitempty"`
	CreatedAt   time.Time                 `json:"createdAt"`
	UpdatedAt   time.Time                 `json:"updatedAt"`
	Steps       []ProcedureStepExecution  `json:"steps,omitempty"`
}

type ProcedureStepExecution struct {
	ID          string     `json:"id"`
	ExecutionID string     `json:"executionId"`
	StepID      string     `json:"stepId"`
	Position    int        `json:"position"`
	Status      string     `json:"status"`
	Attempt     int        `json:"attempt"`
	MaxAttempts int        `json:"maxAttempts"`
	Output      string     `json:"output"`
	Error       string     `json:"error,omitempty"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	OperationID *string    `json:"operationId,omitempty"`
}

type ProcedureSchedule struct {
	ID            string     `json:"id"`
	ProcedureID   string     `json:"procedureId"`
	CronExpression string    `json:"cronExpression"`
	Timezone      string     `json:"timezone"`
	Enabled       bool       `json:"enabled"`
	LastRunAt     *time.Time `json:"lastRunAt,omitempty"`
	NextRunAt     *time.Time `json:"nextRunAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

type ProcedureStepLog struct {
	ID             string    `json:"id"`
	StepExecutionID string   `json:"stepExecutionId"`
	Level          string    `json:"level"`
	Message        string    `json:"message"`
	CreatedAt      time.Time `json:"createdAt"`
}

type CreateProcedureRequest struct {
	Name        string
	Description string
	TenantID    *string
	Enabled     bool
	Steps       []CreateProcedureStepRequest
	Schedule    *CreateProcedureScheduleRequest
}

type CreateProcedureStepRequest struct {
	Position          int
	Name              string
	Action            string
	Config            map[string]any
	MaxRetries        int
	TimeoutSeconds    int
	RequiresApproval  bool
	ContinueOnFailure bool
	RollbackEnabled   bool
}

type CreateProcedureScheduleRequest struct {
	CronExpression string
	Timezone       string
	Enabled        bool
}

func (s *Store) CreateProcedure(ctx context.Context, req CreateProcedureRequest) (Procedure, error) {
	if strings.TrimSpace(req.Name) == "" {
		return Procedure{}, errors.New("name is required")
	}
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO procedures (id, name, description, tenant_id, enabled)
		VALUES ($1, $2, $3, $4, $5)
	`, id, strings.TrimSpace(req.Name), req.Description, req.TenantID, req.Enabled)
	if err != nil {
		return Procedure{}, err
	}
	for _, step := range req.Steps {
		configRaw, _ := json.Marshal(step.Config)
		if configRaw == nil {
			configRaw = []byte("{}")
		}
		stepID := uuid.NewString()
		_, err = s.db.Exec(ctx, `
			INSERT INTO procedure_steps (id, procedure_id, position, name, action, config, max_retries, timeout_seconds, requires_approval, continue_on_failure, rollback_enabled)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		`, stepID, id, step.Position, step.Name, step.Action, string(configRaw), step.MaxRetries, step.TimeoutSeconds, step.RequiresApproval, step.ContinueOnFailure, step.RollbackEnabled)
		if err != nil {
			return Procedure{}, err
		}
	}
	if req.Schedule != nil && strings.TrimSpace(req.Schedule.CronExpression) != "" {
		timezone := req.Schedule.Timezone
		if timezone == "" {
			timezone = "UTC"
		}
		_, err = s.db.Exec(ctx, `
			INSERT INTO procedure_schedules (id, procedure_id, cron_expression, timezone, enabled)
			VALUES ($1, $2, $3, $4, $5)
		`, uuid.NewString(), id, strings.TrimSpace(req.Schedule.CronExpression), timezone, req.Schedule.Enabled)
		if err != nil {
			return Procedure{}, err
		}
	}
	return s.GetProcedure(ctx, id)
}

func (s *Store) GetProcedure(ctx context.Context, id string) (Procedure, error) {
	var p Procedure
	err := s.db.QueryRow(ctx, `
		SELECT id::text, name, description, tenant_id::text, enabled, created_at, updated_at
		FROM procedures WHERE id = $1
	`, id).Scan(&p.ID, &p.Name, &p.Description, &p.TenantID, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return Procedure{}, errors.New("procedure not found")
		}
		return Procedure{}, err
	}
	steps, err := s.ListProcedureSteps(ctx, id)
	if err != nil {
		return Procedure{}, err
	}
	p.Steps = steps
	sched, err := s.GetProcedureSchedule(ctx, id)
	if err == nil {
		p.Schedule = &sched
	}
	return p, nil
}

func (s *Store) ListProcedures(ctx context.Context, tenantID *string) ([]Procedure, error) {
	query := `SELECT id::text, name, description, tenant_id::text, enabled, created_at, updated_at FROM procedures`
	var args []any
	if tenantID != nil && *tenantID != "" {
		query += ` WHERE tenant_id = $1 ORDER BY created_at DESC`
		args = append(args, *tenantID)
	} else {
		query += ` ORDER BY created_at DESC`
	}
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var procedures []Procedure
	for rows.Next() {
		var p Procedure
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.TenantID, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		procedures = append(procedures, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range procedures {
		steps, err := s.ListProcedureSteps(ctx, procedures[i].ID)
		if err == nil {
			procedures[i].Steps = steps
		}
	}
	return procedures, nil
}

func (s *Store) ListProcedureSteps(ctx context.Context, procedureID string) ([]ProcedureStep, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, procedure_id::text, position, name, action, config, max_retries, timeout_seconds, requires_approval, continue_on_failure, rollback_enabled, created_at
		FROM procedure_steps WHERE procedure_id = $1 ORDER BY position
	`, procedureID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var steps []ProcedureStep
	for rows.Next() {
		var step ProcedureStep
		var configRaw []byte
		if err := rows.Scan(&step.ID, &step.ProcedureID, &step.Position, &step.Name, &step.Action, &configRaw, &step.MaxRetries, &step.TimeoutSeconds, &step.RequiresApproval, &step.ContinueOnFailure, &step.RollbackEnabled, &step.CreatedAt); err != nil {
			return nil, err
		}
		step.Config = map[string]any{}
		if len(configRaw) > 0 {
			_ = json.Unmarshal(configRaw, &step.Config)
		}
		steps = append(steps, step)
	}
	return steps, rows.Err()
}

func (s *Store) UpdateProcedure(ctx context.Context, id string, req CreateProcedureRequest) (Procedure, error) {
	if strings.TrimSpace(req.Name) == "" {
		return Procedure{}, errors.New("name is required")
	}
	_, err := s.db.Exec(ctx, `
		UPDATE procedures SET name=$1, description=$2, tenant_id=$3, enabled=$4, updated_at=now()
		WHERE id=$5
	`, strings.TrimSpace(req.Name), req.Description, req.TenantID, req.Enabled, id)
	if err != nil {
		return Procedure{}, err
	}
	_, _ = s.db.Exec(ctx, `DELETE FROM procedure_steps WHERE procedure_id = $1`, id)
	for _, step := range req.Steps {
		configRaw, _ := json.Marshal(step.Config)
		if configRaw == nil {
			configRaw = []byte("{}")
		}
		stepID := uuid.NewString()
		_, err = s.db.Exec(ctx, `
			INSERT INTO procedure_steps (id, procedure_id, position, name, action, config, max_retries, timeout_seconds, requires_approval, continue_on_failure, rollback_enabled)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		`, stepID, id, step.Position, step.Name, step.Action, string(configRaw), step.MaxRetries, step.TimeoutSeconds, step.RequiresApproval, step.ContinueOnFailure, step.RollbackEnabled)
		if err != nil {
			return Procedure{}, err
		}
	}
	if req.Schedule != nil && strings.TrimSpace(req.Schedule.CronExpression) != "" {
		timezone := req.Schedule.Timezone
		if timezone == "" {
			timezone = "UTC"
		}
		_, _ = s.db.Exec(ctx, `DELETE FROM procedure_schedules WHERE procedure_id = $1`, id)
		_, err = s.db.Exec(ctx, `
			INSERT INTO procedure_schedules (id, procedure_id, cron_expression, timezone, enabled)
			VALUES ($1, $2, $3, $4, $5)
		`, uuid.NewString(), id, strings.TrimSpace(req.Schedule.CronExpression), timezone, req.Schedule.Enabled)
		if err != nil {
			return Procedure{}, err
		}
	} else {
		_, _ = s.db.Exec(ctx, `DELETE FROM procedure_schedules WHERE procedure_id = $1`, id)
	}
	return s.GetProcedure(ctx, id)
}

func (s *Store) DeleteProcedure(ctx context.Context, id string) error {
	commandTag, err := s.db.Exec(ctx, `DELETE FROM procedures WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if commandTag.RowsAffected() == 0 {
		return errors.New("procedure not found")
	}
	return nil
}

func (s *Store) GetProcedureSchedule(ctx context.Context, procedureID string) (ProcedureSchedule, error) {
	var ps ProcedureSchedule
	err := s.db.QueryRow(ctx, `
		SELECT id::text, procedure_id::text, cron_expression, timezone, enabled, last_run_at, next_run_at, created_at, updated_at
		FROM procedure_schedules WHERE procedure_id = $1
	`, procedureID).Scan(&ps.ID, &ps.ProcedureID, &ps.CronExpression, &ps.Timezone, &ps.Enabled, &ps.LastRunAt, &ps.NextRunAt, &ps.CreatedAt, &ps.UpdatedAt)
	if err != nil {
		return ProcedureSchedule{}, err
	}
	return ps, nil
}

func (s *Store) UpdateProcedureScheduleMeta(ctx context.Context, scheduleID string, lastRunAt, nextRunAt *time.Time) error {
	_, err := s.db.Exec(ctx, `
		UPDATE procedure_schedules
		SET last_run_at = COALESCE($1, last_run_at),
		    next_run_at = COALESCE($2, next_run_at),
		    updated_at = now()
		WHERE id = $3
	`, lastRunAt, nextRunAt, scheduleID)
	return err
}

func (s *Store) ListDueProcedureSchedules(ctx context.Context, now time.Time, limit int) ([]ProcedureSchedule, error) {
	if limit <= 0 {
		limit = 64
	}
	rows, err := s.db.Query(ctx, `
		SELECT ps.id::text, ps.procedure_id::text, ps.cron_expression, ps.timezone, ps.enabled, ps.last_run_at, ps.next_run_at, ps.created_at, ps.updated_at
		FROM procedure_schedules ps
		JOIN procedures p ON p.id = ps.procedure_id
		WHERE ps.enabled = TRUE AND p.enabled = TRUE
		  AND (ps.next_run_at IS NULL OR ps.next_run_at <= $1)
		ORDER BY ps.next_run_at NULLS FIRST, ps.created_at ASC
		LIMIT $2
	`, now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var schedules []ProcedureSchedule
	for rows.Next() {
		var ps ProcedureSchedule
		if err := rows.Scan(&ps.ID, &ps.ProcedureID, &ps.CronExpression, &ps.Timezone, &ps.Enabled, &ps.LastRunAt, &ps.NextRunAt, &ps.CreatedAt, &ps.UpdatedAt); err != nil {
			return nil, err
		}
		schedules = append(schedules, ps)
	}
	return schedules, rows.Err()
}

func (s *Store) NextProcedureScheduleRunAt(ctx context.Context, now time.Time) (*time.Time, error) {
	var next *time.Time
	if err := s.db.QueryRow(ctx, `
		SELECT MIN(ps.next_run_at)
		FROM procedure_schedules ps
		JOIN procedures p ON p.id = ps.procedure_id
		WHERE ps.enabled = TRUE AND p.enabled = TRUE
		  AND ps.next_run_at IS NOT NULL
		  AND ps.next_run_at >= $1
	`, now.UTC()).Scan(&next); err != nil {
		return nil, err
	}
	return next, nil
}

func (s *Store) CreateProcedureExecution(ctx context.Context, procedureID, trigger string, tenantID, actorID *string) (ProcedureExecution, error) {
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO procedure_executions (id, procedure_id, status, trigger, tenant_id, actor_id)
		VALUES ($1, $2, 'queued', $3, $4, $5)
	`, id, procedureID, trigger, tenantID, actorID)
	if err != nil {
		return ProcedureExecution{}, err
	}
	steps, err := s.ListProcedureSteps(ctx, procedureID)
	if err != nil {
		return ProcedureExecution{}, err
	}
	for _, step := range steps {
		stepExecID := uuid.NewString()
		_, err = s.db.Exec(ctx, `
			INSERT INTO procedure_step_executions (id, execution_id, step_id, position, status, max_attempts)
			VALUES ($1, $2, $3, $4, 'queued', $5)
		`, stepExecID, id, step.ID, step.Position, step.MaxRetries)
		if err != nil {
			return ProcedureExecution{}, err
		}
	}
	return s.GetProcedureExecution(ctx, id)
}

func (s *Store) GetProcedureExecution(ctx context.Context, id string) (ProcedureExecution, error) {
	var pe ProcedureExecution
	err := s.db.QueryRow(ctx, `
		SELECT id::text, procedure_id::text, status, trigger, tenant_id::text, actor_id::text, started_at, completed_at, created_at, updated_at
		FROM procedure_executions WHERE id = $1
	`, id).Scan(&pe.ID, &pe.ProcedureID, &pe.Status, &pe.Trigger, &pe.TenantID, &pe.ActorID, &pe.StartedAt, &pe.CompletedAt, &pe.CreatedAt, &pe.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ProcedureExecution{}, errors.New("execution not found")
		}
		return ProcedureExecution{}, err
	}
	steps, err := s.ListProcedureStepExecutions(ctx, id)
	if err == nil {
		pe.Steps = steps
	}
	return pe, nil
}

func (s *Store) ListProcedureExecutions(ctx context.Context, procedureID string, limit int) ([]ProcedureExecution, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, procedure_id::text, status, trigger, tenant_id::text, actor_id::text, started_at, completed_at, created_at, updated_at
		FROM procedure_executions WHERE procedure_id = $1
		ORDER BY created_at DESC LIMIT $2
	`, procedureID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var execs []ProcedureExecution
	for rows.Next() {
		var pe ProcedureExecution
		if err := rows.Scan(&pe.ID, &pe.ProcedureID, &pe.Status, &pe.Trigger, &pe.TenantID, &pe.ActorID, &pe.StartedAt, &pe.CompletedAt, &pe.CreatedAt, &pe.UpdatedAt); err != nil {
			return nil, err
		}
		execs = append(execs, pe)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range execs {
		steps, err := s.ListProcedureStepExecutions(ctx, execs[i].ID)
		if err == nil {
			execs[i].Steps = steps
		}
	}
	return execs, nil
}

func (s *Store) ListProcedureStepExecutions(ctx context.Context, executionID string) ([]ProcedureStepExecution, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, execution_id::text, step_id::text, position, status, attempt, max_attempts, output, error, started_at, completed_at, operation_id::text
		FROM procedure_step_executions WHERE execution_id = $1 ORDER BY position
	`, executionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var steps []ProcedureStepExecution
	for rows.Next() {
		var step ProcedureStepExecution
		if err := rows.Scan(&step.ID, &step.ExecutionID, &step.StepID, &step.Position, &step.Status, &step.Attempt, &step.MaxAttempts, &step.Output, &step.Error, &step.StartedAt, &step.CompletedAt, &step.OperationID); err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}
	return steps, rows.Err()
}

func (s *Store) UpdateProcedureExecutionStatus(ctx context.Context, id, status string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE procedure_executions SET status=$1, updated_at=now()
		WHERE id=$2
	`, status, id)
	return err
}

func (s *Store) StartProcedureExecution(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		UPDATE procedure_executions SET status='running', started_at=$1, updated_at=$1
		WHERE id=$2 AND status='queued'
	`, now, id)
	return err
}

func (s *Store) CompleteProcedureExecution(ctx context.Context, id, status string) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		UPDATE procedure_executions SET status=$1, completed_at=$2, updated_at=$2
		WHERE id=$3
	`, status, now, id)
	return err
}

func (s *Store) UpdateProcedureStepExecution(ctx context.Context, id, status string, attempt int) error {
	_, err := s.db.Exec(ctx, `
		UPDATE procedure_step_executions SET status=$1, attempt=$2
		WHERE id=$3
	`, status, attempt, id)
	return err
}

func (s *Store) StartProcedureStepExecution(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		UPDATE procedure_step_executions SET status='running', started_at=$1
		WHERE id=$2 AND status='queued'
	`, now, id)
	return err
}

func (s *Store) CompleteProcedureStepExecution(ctx context.Context, id, status, output, errMsg string) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		UPDATE procedure_step_executions SET status=$1, output=$2, error=$3, completed_at=$4
		WHERE id=$5
	`, status, output, errMsg, now, id)
	return err
}

func (s *Store) LinkProcedureStepOperation(ctx context.Context, stepExecID, operationID string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE procedure_step_executions SET operation_id=$2 WHERE id=$1
	`, stepExecID, operationID)
	return err
}

func (s *Store) AppendProcedureStepLog(ctx context.Context, stepExecutionID, level, message string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO procedure_step_logs (id, step_execution_id, level, message)
		VALUES ($1, $2, $3, $4)
	`, uuid.NewString(), stepExecutionID, level, message)
	return err
}

func (s *Store) ListProcedureStepLogs(ctx context.Context, stepExecutionID string) ([]ProcedureStepLog, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, step_execution_id::text, level, message, created_at
		FROM procedure_step_logs WHERE step_execution_id = $1 ORDER BY created_at
	`, stepExecutionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []ProcedureStepLog
	for rows.Next() {
		var l ProcedureStepLog
		if err := rows.Scan(&l.ID, &l.StepExecutionID, &l.Level, &l.Message, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (s *Store) FindQueuedStepExecution(ctx context.Context, executionID string) (*ProcedureStepExecution, error) {
	var step ProcedureStepExecution
	err := s.db.QueryRow(ctx, `
		SELECT id::text, execution_id::text, step_id::text, position, status, attempt, max_attempts, output, error, started_at, completed_at, operation_id::text
		FROM procedure_step_executions
		WHERE execution_id = $1 AND status = 'queued'
		ORDER BY position LIMIT 1
	`, executionID).Scan(&step.ID, &step.ExecutionID, &step.StepID, &step.Position, &step.Status, &step.Attempt, &step.MaxAttempts, &step.Output, &step.Error, &step.StartedAt, &step.CompletedAt, &step.OperationID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &step, nil
}

func (s *Store) FindWaitingApprovalStep(ctx context.Context, executionID string) (*ProcedureStepExecution, error) {
	var step ProcedureStepExecution
	err := s.db.QueryRow(ctx, `
		SELECT id::text, execution_id::text, step_id::text, position, status, attempt, max_attempts, output, error, started_at, completed_at, operation_id::text
		FROM procedure_step_executions
		WHERE execution_id = $1 AND status = 'waiting_approval'
		ORDER BY position LIMIT 1
	`, executionID).Scan(&step.ID, &step.ExecutionID, &step.StepID, &step.Position, &step.Status, &step.Attempt, &step.MaxAttempts, &step.Output, &step.Error, &step.StartedAt, &step.CompletedAt, &step.OperationID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &step, nil
}

func (s *Store) ApproveProcedureStep(ctx context.Context, stepExecID string) error {
	_, err := s.db.Exec(ctx, `UPDATE procedure_step_executions SET status='queued' WHERE id=$1 AND status='waiting_approval'`, stepExecID)
	return err
}

func (s *Store) RejectProcedureStep(ctx context.Context, stepExecID string) error {
	_, err := s.db.Exec(ctx, `UPDATE procedure_step_executions SET status='cancelled' WHERE id=$1 AND status='waiting_approval'`, stepExecID)
	return err
}

func (s *Store) CancelProcedureExecution(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE procedure_executions SET status='cancelled', updated_at=now()
		WHERE id=$1 AND status IN ('queued','running','waiting_approval')
	`, id)
	return err
}

func (s *Store) CancelQueuedStepExecutions(ctx context.Context, executionID string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE procedure_step_executions SET status='cancelled'
		WHERE execution_id=$1 AND status='queued'
	`, executionID)
	return err
}

func (s *Store) CreateRollbackExecution(ctx context.Context, originalExecutionID string) (string, error) {
	original, err := s.GetProcedureExecution(ctx, originalExecutionID)
	if err != nil {
		return "", err
	}
	rollbackID := uuid.NewString()
	_, err = s.db.Exec(ctx, `
		INSERT INTO procedure_executions (id, procedure_id, status, trigger, tenant_id, actor_id)
		VALUES ($1, $2, 'rolling_back', 'rollback', $3, $4)
	`, rollbackID, original.ProcedureID, original.TenantID, original.ActorID)
	if err != nil {
		return "", err
	}
	steps, err := s.ListProcedureSteps(ctx, original.ProcedureID)
	if err != nil {
		return "", err
	}
	for _, step := range steps {
		if step.RollbackEnabled {
			stepExecID := uuid.NewString()
			_, err = s.db.Exec(ctx, `
				INSERT INTO procedure_step_executions (id, execution_id, step_id, position, status, max_attempts)
				VALUES ($1, $2, $3, $4, 'queued', $5)
			`, stepExecID, rollbackID, step.ID, step.Position, step.MaxRetries)
			if err != nil {
				return "", err
			}
		}
	}
	return rollbackID, nil
}

var _ = fmt.Sprintf
