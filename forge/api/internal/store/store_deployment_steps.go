package store

import (
	"context"
	"time"
)

type DeploymentStep struct {
	ID           string     `json:"id"`
	DeploymentID string     `json:"deploymentId"`
	StepNumber   int        `json:"stepNumber"`
	StepName     string     `json:"stepName"`
	Status       string     `json:"status"`
	StartedAt    *time.Time `json:"startedAt,omitempty"`
	CompletedAt  *time.Time `json:"completedAt,omitempty"`
	Error        string     `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

func (s *Store) CreateDeploymentStep(ctx context.Context, step *DeploymentStep) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO deployment_steps (id, deployment_id, step_number, step_name, status, started_at, completed_at, error, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, step.ID, step.DeploymentID, step.StepNumber, step.StepName, step.Status, step.StartedAt, step.CompletedAt, step.Error, step.CreatedAt, step.UpdatedAt)
	return err
}

func (s *Store) ListDeploymentSteps(ctx context.Context, deploymentID string) ([]DeploymentStep, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, deployment_id::text, step_number, step_name, status, started_at, completed_at, COALESCE(error, ''), created_at, updated_at
		FROM deployment_steps
		WHERE deployment_id = $1
		ORDER BY step_number ASC
	`, deploymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DeploymentStep
	for rows.Next() {
		var ds DeploymentStep
		if err := rows.Scan(&ds.ID, &ds.DeploymentID, &ds.StepNumber, &ds.StepName, &ds.Status, &ds.StartedAt, &ds.CompletedAt, &ds.Error, &ds.CreatedAt, &ds.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, ds)
	}
	return result, rows.Err()
}

func (s *Store) GetDeploymentStep(ctx context.Context, stepID string) (DeploymentStep, error) {
	var ds DeploymentStep
	err := s.db.QueryRow(ctx, `
		SELECT id::text, deployment_id::text, step_number, step_name, status, started_at, completed_at, COALESCE(error, ''), created_at, updated_at
		FROM deployment_steps
		WHERE id = $1
	`, stepID).Scan(&ds.ID, &ds.DeploymentID, &ds.StepNumber, &ds.StepName, &ds.Status, &ds.StartedAt, &ds.CompletedAt, &ds.Error, &ds.CreatedAt, &ds.UpdatedAt)
	return ds, err
}

func (s *Store) UpdateDeploymentStep(ctx context.Context, step *DeploymentStep) error {
	_, err := s.db.Exec(ctx, `
		UPDATE deployment_steps
		SET status = $2, started_at = $3, completed_at = $4, error = $5, updated_at = now()
		WHERE id = $1
	`, step.ID, step.Status, step.StartedAt, step.CompletedAt, step.Error)
	return err
}

func (s *Store) UpdateDeploymentStepStatus(ctx context.Context, stepID string, status string, errMsg string) error {
	var startedAt, completedAt *time.Time
	now := time.Now().UTC()
	if status == "in_progress" {
		startedAt = &now
	}
	if status == "completed" || status == "failed" || status == "cancelled" {
		completedAt = &now
	}
	_, err := s.db.Exec(ctx, `
		UPDATE deployment_steps
		SET status = $2, started_at = CASE WHEN $2 = 'in_progress' AND started_at IS NULL THEN $3 ELSE started_at END,
		    completed_at = CASE WHEN $2 IN ('completed', 'failed', 'cancelled') THEN $4 ELSE completed_at END,
		    error = CASE WHEN $5 = '' THEN error ELSE $5 END,
		    updated_at = now()
		WHERE id = $1
	`, stepID, status, startedAt, completedAt, errMsg)
	return err
}

func (s *Store) UpdateDeploymentProgress(ctx context.Context, deploymentID string, progressPct int, nextStep int, timeoutAt *time.Time) error {
	_, err := s.db.Exec(ctx, `
		UPDATE deployments
		SET progress_pct = $2, next_step = $3, timeout_at = $4, updated_at = now()
		WHERE id = $1
	`, deploymentID, progressPct, nextStep, timeoutAt)
	return err
}

func (s *Store) ListInProgressDeployments(ctx context.Context) ([]Deployment, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, strategy, status, image, blue_target_id, green_target_id, active_target,
			COALESCE(health_check_path, ''), COALESCE(health_check_port, 0), COALESCE(health_check_host, ''), COALESCE(error, ''),
			current_revision_id::text, COALESCE(rollout_strategy, 'recreate'),
			COALESCE(timeout_seconds, 300), COALESCE(health_gate_enabled, false), COALESCE(health_gate_threshold, 3),
			COALESCE(health_gate_interval_ms, 5000), COALESCE(auto_rollback_enabled, false),
			COALESCE(rollback_on_health_failure, false), COALESCE(cleanup_on_failure, true),
			COALESCE(target_replicas, 1), COALESCE(progress_pct, 0), COALESCE(next_step, 0),
			timeout_at, COALESCE(executor_id, ''), execution_lease_until, COALESCE(version, 1), created_at, updated_at, completed_at
		FROM deployments
		WHERE status IN ('pending', 'in_progress', 'provisioning', 'awaiting_health', 'promoting', 'rollback_pending', 'rolling_back')
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Deployment
	for rows.Next() {
		var d Deployment
		if err := rows.Scan(&d.ID, &d.ServerID, &d.Strategy, &d.Status, &d.Image, &d.BlueTargetID, &d.GreenTargetID,
			&d.ActiveTarget, &d.HealthCheckPath, &d.HealthCheckPort, &d.HealthCheckHost, &d.Error, &d.CurrentRevisionID,
			&d.RolloutStrategy, &d.TimeoutSeconds, &d.HealthGateEnabled, &d.HealthGateThreshold,
			&d.HealthGateIntervalMs, &d.AutoRollbackEnabled, &d.RollbackOnHealthFailure, &d.CleanupOnFailure,
			&d.TargetReplicas, &d.ProgressPct, &d.NextStep, &d.TimeoutAt, &d.ExecutorID, &d.ExecutionLeaseUntil,
			&d.Version, &d.CreatedAt, &d.UpdatedAt, &d.CompletedAt); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}
