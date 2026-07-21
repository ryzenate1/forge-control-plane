package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Deployment struct {
	ID                      string     `json:"id"`
	ServerID                string     `json:"serverId"`
	Strategy                string     `json:"strategy"`
	Status                  string     `json:"status"`
	Image                   string     `json:"image"`
	BlueTargetID            string     `json:"blueTargetId"`
	GreenTargetID           string     `json:"greenTargetId"`
	ActiveTarget            string     `json:"activeTarget"`
	HealthCheckPath         string     `json:"healthCheckPath,omitempty"`
	HealthCheckPort         int        `json:"healthCheckPort,omitempty"`
	HealthCheckHost         string     `json:"healthCheckHost,omitempty"`
	Error                   string     `json:"error,omitempty"`
	CurrentRevisionID       *string    `json:"currentRevisionId,omitempty"`
	RolloutStrategy         string     `json:"rolloutStrategy,omitempty"`
	TimeoutSeconds          int        `json:"timeoutSeconds,omitempty"`
	HealthGateEnabled       bool       `json:"healthGateEnabled,omitempty"`
	HealthGateThreshold     int        `json:"healthGateThreshold,omitempty"`
	HealthGateIntervalMs    int        `json:"healthGateIntervalMs,omitempty"`
	AutoRollbackEnabled     bool       `json:"autoRollbackEnabled,omitempty"`
	RollbackOnHealthFailure bool       `json:"rollbackOnHealthFailure,omitempty"`
	CleanupOnFailure        bool       `json:"cleanupOnFailure,omitempty"`
	TargetReplicas          int        `json:"targetReplicas,omitempty"`
	ProgressPct             int        `json:"progressPct,omitempty"`
	NextStep                int        `json:"nextStep,omitempty"`
	TimeoutAt               *time.Time `json:"timeoutAt,omitempty"`
	ExecutorID              string     `json:"executorId,omitempty"`
	ExecutionLeaseUntil     *time.Time `json:"executionLeaseUntil,omitempty"`
	Version                 int        `json:"version"`
	CreatedAt               time.Time  `json:"createdAt"`
	UpdatedAt               time.Time  `json:"updatedAt"`
	CompletedAt             *time.Time `json:"completedAt,omitempty"`
}

func (s *Store) CreateDeployment(ctx context.Context, d *Deployment) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO deployments (id, server_id, strategy, status, image, blue_target_id, green_target_id, active_target,
			health_check_path, health_check_port, health_check_host, error, current_revision_id, rollout_strategy,
			timeout_seconds, health_gate_enabled, health_gate_threshold, health_gate_interval_ms,
			auto_rollback_enabled, rollback_on_health_failure, cleanup_on_failure, target_replicas,
			progress_pct, next_step, timeout_at, version, created_at, updated_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29)
	`, d.ID, d.ServerID, d.Strategy, d.Status, d.Image, d.BlueTargetID, d.GreenTargetID, d.ActiveTarget,
		d.HealthCheckPath, d.HealthCheckPort, d.HealthCheckHost, d.Error, d.CurrentRevisionID, d.RolloutStrategy,
		d.TimeoutSeconds, d.HealthGateEnabled, d.HealthGateThreshold, d.HealthGateIntervalMs,
		d.AutoRollbackEnabled, d.RollbackOnHealthFailure, d.CleanupOnFailure, d.TargetReplicas,
		d.ProgressPct, d.NextStep, d.TimeoutAt, 1, d.CreatedAt, d.UpdatedAt, d.CompletedAt)
	return err
}

func (s *Store) GetDeployment(ctx context.Context, id string) (Deployment, error) {
	var d Deployment
	err := s.db.QueryRow(ctx, `
		SELECT id::text, server_id::text, strategy, status, image, blue_target_id, green_target_id, active_target,
			COALESCE(health_check_path, ''), COALESCE(health_check_port, 0), COALESCE(health_check_host, ''), COALESCE(error, ''),
			current_revision_id::text, COALESCE(rollout_strategy, 'recreate'),
			COALESCE(timeout_seconds, 300), COALESCE(health_gate_enabled, false), COALESCE(health_gate_threshold, 3),
			COALESCE(health_gate_interval_ms, 5000), COALESCE(auto_rollback_enabled, false),
			COALESCE(rollback_on_health_failure, false), COALESCE(cleanup_on_failure, true),
			COALESCE(target_replicas, 1), COALESCE(progress_pct, 0), COALESCE(next_step, 0),
			timeout_at, COALESCE(executor_id, ''), execution_lease_until, COALESCE(version, 1), created_at, updated_at, completed_at
		FROM deployments
		WHERE id = $1
	`, id).Scan(&d.ID, &d.ServerID, &d.Strategy, &d.Status, &d.Image, &d.BlueTargetID, &d.GreenTargetID,
		&d.ActiveTarget, &d.HealthCheckPath, &d.HealthCheckPort, &d.HealthCheckHost, &d.Error, &d.CurrentRevisionID,
		&d.RolloutStrategy, &d.TimeoutSeconds, &d.HealthGateEnabled, &d.HealthGateThreshold,
		&d.HealthGateIntervalMs, &d.AutoRollbackEnabled, &d.RollbackOnHealthFailure, &d.CleanupOnFailure,
		&d.TargetReplicas, &d.ProgressPct, &d.NextStep, &d.TimeoutAt, &d.ExecutorID, &d.ExecutionLeaseUntil,
		&d.Version, &d.CreatedAt, &d.UpdatedAt, &d.CompletedAt)
	return d, err
}

func (s *Store) ListDeployments(ctx context.Context, serverID string) ([]Deployment, error) {
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
		WHERE server_id = $1
		ORDER BY created_at DESC
	`, serverID)
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

func (s *Store) UpdateDeployment(ctx context.Context, d *Deployment) error {
	_, err := s.db.Exec(ctx, `
		UPDATE deployments
		SET server_id = $2, strategy = $3, status = $4, image = $5, blue_target_id = $6, green_target_id = $7,
						active_target = $8, health_check_path = $9, health_check_port = $10, health_check_host = $11, error = $12,
						current_revision_id = $13, rollout_strategy = $14,
						timeout_seconds = $15, health_gate_enabled = $16, health_gate_threshold = $17,
						health_gate_interval_ms = $18, auto_rollback_enabled = $19, rollback_on_health_failure = $20,
						cleanup_on_failure = $21, target_replicas = $22, progress_pct = $23, next_step = $24,
						timeout_at = $25, executor_id = $26, execution_lease_until = $27, version = version + 1, updated_at = now(), completed_at = $28
					WHERE id = $1
				`, d.ID, d.ServerID, d.Strategy, d.Status, d.Image, d.BlueTargetID, d.GreenTargetID,
		d.ActiveTarget, d.HealthCheckPath, d.HealthCheckPort, d.HealthCheckHost, d.Error,
		d.CurrentRevisionID, d.RolloutStrategy,
		d.TimeoutSeconds, d.HealthGateEnabled, d.HealthGateThreshold,
		d.HealthGateIntervalMs, d.AutoRollbackEnabled, d.RollbackOnHealthFailure,
		d.CleanupOnFailure, d.TargetReplicas, d.ProgressPct, d.NextStep,
		d.TimeoutAt, d.ExecutorID, d.ExecutionLeaseUntil, d.CompletedAt)
	return err
}

func (s *Store) UpdateDeploymentStatus(ctx context.Context, id string, status string, errMsg string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE deployments
		SET status = $2, error = CASE WHEN $3 = '' THEN error ELSE $3 END, updated_at = now(),
		    completed_at = CASE WHEN $2 IN ('completed', 'failed', 'rolled_back', 'cancelled') THEN now() ELSE completed_at END
		WHERE id = $1
	`, id, status, errMsg)
	return err
}

func (s *Store) ListAllDeployments(ctx context.Context) ([]Deployment, error) {
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
		ORDER BY created_at DESC
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

var ErrVersionConflict = errors.New("version conflict: deployment was modified by another operation")

func (s *Store) UpdateDeploymentConfig(ctx context.Context, id string, version int, image string, blueTargetID string, greenTargetID string, activeTarget string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE deployments
		SET image = CASE WHEN $3 = '' THEN image ELSE $3 END,
			blue_target_id = CASE WHEN $4 = '' THEN blue_target_id ELSE $4 END,
			green_target_id = CASE WHEN $5 = '' THEN green_target_id ELSE $5 END,
			active_target = CASE WHEN $6 = '' THEN active_target ELSE $6 END,
			version = version + 1, updated_at = now()
		WHERE id = $1 AND version = $2
	`, id, version, image, blueTargetID, greenTargetID, activeTarget)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

func (s *Store) UpdateDeploymentFailure(ctx context.Context, id string, version int, errMsg string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE deployments
		SET status = 'failed', error = $3, completed_at = now(),
			version = version + 1, updated_at = now()
		WHERE id = $1 AND version = $2
	`, id, version, errMsg)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

func (s *Store) UpdateDeploymentRollback(ctx context.Context, id string, version int, activeTarget string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE deployments
		SET status = 'rolled_back', active_target = CASE WHEN $3 = '' THEN active_target ELSE $3 END,
			completed_at = now(), version = version + 1, updated_at = now()
		WHERE id = $1 AND version = $2
	`, id, version, activeTarget)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

func (s *Store) UpdateDeploymentCompletion(ctx context.Context, id string, version int) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE deployments
		SET status = 'completed', progress_pct = 100, completed_at = now(),
			version = version + 1, updated_at = now()
		WHERE id = $1 AND version = $2
	`, id, version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

func (s *Store) UpdateDeploymentCancelled(ctx context.Context, id string, version int, errMsg string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE deployments
		SET status = 'cancelled', error = CASE WHEN $3 = '' THEN error ELSE $3 END,
			completed_at = now(), version = version + 1, updated_at = now()
		WHERE id = $1 AND version = $2
	`, id, version, errMsg)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

func (s *Store) UpdateDeploymentStatusVersioned(ctx context.Context, id string, version int, status string, errMsg string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE deployments
		SET status = $3, error = CASE WHEN $4 = '' THEN error ELSE $4 END,
			completed_at = CASE WHEN $3 IN ('completed', 'failed', 'rolled_back', 'cancelled') THEN now() ELSE completed_at END,
			version = version + 1, updated_at = now()
		WHERE id = $1 AND version = $2
	`, id, version, status, errMsg)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrVersionConflict
	}
	return nil
}

func (s *Store) DeleteDeployment(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM deployments WHERE id = $1`, id)
	return err
}

func (s *Store) ClaimExecutionLease(ctx context.Context, deploymentID string, executorID string, leaseDuration time.Duration) (bool, error) {
	var claimed bool
	err := s.db.QueryRow(ctx, `
		UPDATE deployments
		SET executor_id = $2,
			execution_lease_until = now() + make_interval(secs => $3),
			updated_at = now()
		WHERE id = $1
		  AND (execution_lease_until IS NULL OR execution_lease_until < now())
		RETURNING true
	`, deploymentID, executorID, leaseDuration.Seconds()).Scan(&claimed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return claimed, nil
}

func (s *Store) ReleaseExecutionLease(ctx context.Context, deploymentID string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE deployments
		SET executor_id = NULL, execution_lease_until = NULL, updated_at = now()
		WHERE id = $1
	`, deploymentID)
	return err
}
