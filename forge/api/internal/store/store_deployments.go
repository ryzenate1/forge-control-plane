package store

import (
	"context"
	"time"
)

type Deployment struct {
	ID                 string     `json:"id"`
	ServerID           string     `json:"serverId"`
	Strategy           string     `json:"strategy"`
	Status             string     `json:"status"`
	Image              string     `json:"image"`
	BlueTargetID       string     `json:"blueTargetId"`
	GreenTargetID      string     `json:"greenTargetId"`
	ActiveTarget       string     `json:"activeTarget"`
	HealthCheckPath    string     `json:"healthCheckPath,omitempty"`
	HealthCheckPort    int        `json:"healthCheckPort,omitempty"`
	Error              string     `json:"error,omitempty"`
	CurrentRevisionID  *string    `json:"currentRevisionId,omitempty"`
	RolloutStrategy    string     `json:"rolloutStrategy,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	CompletedAt        *time.Time `json:"completedAt,omitempty"`
}

func (s *Store) CreateDeployment(ctx context.Context, d *Deployment) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO deployments (id, server_id, strategy, status, image, blue_target_id, green_target_id, active_target, health_check_path, health_check_port, error, current_revision_id, rollout_strategy, created_at, updated_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`, d.ID, d.ServerID, d.Strategy, d.Status, d.Image, d.BlueTargetID, d.GreenTargetID, d.ActiveTarget, d.HealthCheckPath, d.HealthCheckPort, d.Error, d.CurrentRevisionID, d.RolloutStrategy, d.CreatedAt, d.UpdatedAt, d.CompletedAt)
	return err
}

func (s *Store) GetDeployment(ctx context.Context, id string) (Deployment, error) {
	var d Deployment
	err := s.db.QueryRow(ctx, `
		SELECT id::text, server_id::text, strategy, status, image, blue_target_id, green_target_id, active_target, COALESCE(health_check_path, ''), COALESCE(health_check_port, 0), COALESCE(error, ''), current_revision_id::text, COALESCE(rollout_strategy, 'recreate'), created_at, updated_at, completed_at
		FROM deployments
		WHERE id = $1
	`, id).Scan(&d.ID, &d.ServerID, &d.Strategy, &d.Status, &d.Image, &d.BlueTargetID, &d.GreenTargetID, &d.ActiveTarget, &d.HealthCheckPath, &d.HealthCheckPort, &d.Error, &d.CurrentRevisionID, &d.RolloutStrategy, &d.CreatedAt, &d.UpdatedAt, &d.CompletedAt)
	return d, err
}

func (s *Store) ListDeployments(ctx context.Context, serverID string) ([]Deployment, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, strategy, status, image, blue_target_id, green_target_id, active_target, COALESCE(health_check_path, ''), COALESCE(health_check_port, 0), COALESCE(error, ''), current_revision_id::text, COALESCE(rollout_strategy, 'recreate'), created_at, updated_at, completed_at
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
		if err := rows.Scan(&d.ID, &d.ServerID, &d.Strategy, &d.Status, &d.Image, &d.BlueTargetID, &d.GreenTargetID, &d.ActiveTarget, &d.HealthCheckPath, &d.HealthCheckPort, &d.Error, &d.CurrentRevisionID, &d.RolloutStrategy, &d.CreatedAt, &d.UpdatedAt, &d.CompletedAt); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (s *Store) UpdateDeployment(ctx context.Context, d *Deployment) error {
	_, err := s.db.Exec(ctx, `
		UPDATE deployments
		SET server_id = $2, strategy = $3, status = $4, image = $5, blue_target_id = $6, green_target_id = $7, active_target = $8, health_check_path = $9, health_check_port = $10, error = $11, updated_at = $12, completed_at = $13
		WHERE id = $1
	`, d.ID, d.ServerID, d.Strategy, d.Status, d.Image, d.BlueTargetID, d.GreenTargetID, d.ActiveTarget, d.HealthCheckPath, d.HealthCheckPort, d.Error, d.UpdatedAt, d.CompletedAt)
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

func (s *Store) DeleteDeployment(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM deployments WHERE id = $1`, id)
	return err
}
