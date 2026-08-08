package store

import (
	"context"
	"time"
)

type DeploymentRelease struct {
	ID          string     `json:"id"`
	ServerID    string     `json:"serverId"`
	Version     int        `json:"version"`
	ImageTag    string     `json:"imageTag"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"createdAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

type ZeroDowntimeHealthCheckConfig struct {
	ID                 string    `json:"id"`
	ServerID           string    `json:"serverId"`
	Path               string    `json:"path"`
	Port               int       `json:"port"`
	Protocol           string    `json:"protocol"`
	IntervalSeconds    int       `json:"intervalSeconds"`
	TimeoutSeconds     int       `json:"timeoutSeconds"`
	HealthyThreshold   int       `json:"healthyThreshold"`
	UnhealthyThreshold int       `json:"unhealthyThreshold"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type ZeroDowntimeHealthCheckResult struct {
	ID              string    `json:"id"`
	DeploymentID    string    `json:"deploymentId"`
	CheckTimestamp  time.Time `json:"checkTimestamp"`
	Status          string    `json:"status"`
	ResponseCode    int       `json:"responseCode"`
	ResponseTimeMs  int       `json:"responseTimeMs"`
	ErrorMessage    string    `json:"errorMessage"`
}

type DeploymentEvent struct {
	ID           string    `json:"id"`
	DeploymentID string    `json:"deploymentId"`
	EventType    string    `json:"eventType"`
	Message      string    `json:"message"`
	CreatedAt    time.Time `json:"createdAt"`
}

func (s *Store) CreateDeploymentRelease(ctx context.Context, r *DeploymentRelease) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO deployment_releases (id, server_id, version, image_tag, status, created_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, r.ID, r.ServerID, r.Version, r.ImageTag, r.Status, r.CreatedAt, r.CompletedAt)
	return err
}

func (s *Store) GetDeploymentRelease(ctx context.Context, id string) (DeploymentRelease, error) {
	var r DeploymentRelease
	err := s.db.QueryRow(ctx, `
		SELECT id::text, server_id::text, version, image_tag, status, created_at, completed_at
		FROM deployment_releases WHERE id = $1
	`, id).Scan(&r.ID, &r.ServerID, &r.Version, &r.ImageTag, &r.Status, &r.CreatedAt, &r.CompletedAt)
	return r, err
}

func (s *Store) ListDeploymentReleases(ctx context.Context, serverID string) ([]DeploymentRelease, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, version, image_tag, status, created_at, completed_at
		FROM deployment_releases WHERE server_id = $1
		ORDER BY version DESC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DeploymentRelease
	for rows.Next() {
		var r DeploymentRelease
		if err := rows.Scan(&r.ID, &r.ServerID, &r.Version, &r.ImageTag, &r.Status, &r.CreatedAt, &r.CompletedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Store) GetLatestDeploymentRelease(ctx context.Context, serverID string) (DeploymentRelease, error) {
	var r DeploymentRelease
	err := s.db.QueryRow(ctx, `
		SELECT id::text, server_id::text, version, image_tag, status, created_at, completed_at
		FROM deployment_releases WHERE server_id = $1
		ORDER BY version DESC LIMIT 1
	`, serverID).Scan(&r.ID, &r.ServerID, &r.Version, &r.ImageTag, &r.Status, &r.CreatedAt, &r.CompletedAt)
	if err != nil {
		return r, err
	}
	return r, nil
}

func (s *Store) UpdateDeploymentReleaseStatus(ctx context.Context, id string, status string, completedAt *time.Time) error {
	_, err := s.db.Exec(ctx, `
		UPDATE deployment_releases SET status = $2, completed_at = COALESCE($3, completed_at)
		WHERE id = $1
	`, id, status, completedAt)
	return err
}

func (s *Store) GetActiveDeploymentRelease(ctx context.Context, serverID string) (DeploymentRelease, error) {
	var r DeploymentRelease
	err := s.db.QueryRow(ctx, `
		SELECT id::text, server_id::text, version, image_tag, status, created_at, completed_at
		FROM deployment_releases
		WHERE server_id = $1 AND status = 'live'
		ORDER BY version DESC LIMIT 1
	`, serverID).Scan(&r.ID, &r.ServerID, &r.Version, &r.ImageTag, &r.Status, &r.CreatedAt, &r.CompletedAt)
	if err != nil {
		return r, err
	}
	return r, nil
}

func (s *Store) GetZeroDowntimeHealthCheckConfig(ctx context.Context, serverID string) (ZeroDowntimeHealthCheckConfig, error) {
	var c ZeroDowntimeHealthCheckConfig
	err := s.db.QueryRow(ctx, `
		SELECT id::text, server_id::text, path, port, protocol, interval_seconds, timeout_seconds,
		       healthy_threshold, unhealthy_threshold, created_at, updated_at
		FROM health_check_configs WHERE server_id = $1
	`, serverID).Scan(&c.ID, &c.ServerID, &c.Path, &c.Port, &c.Protocol,
		&c.IntervalSeconds, &c.TimeoutSeconds, &c.HealthyThreshold, &c.UnhealthyThreshold,
		&c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (s *Store) UpsertZeroDowntimeHealthCheckConfig(ctx context.Context, c *ZeroDowntimeHealthCheckConfig) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO health_check_configs (id, server_id, path, port, protocol, interval_seconds, timeout_seconds,
			healthy_threshold, unhealthy_threshold, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (server_id) DO UPDATE SET
			path = EXCLUDED.path, port = EXCLUDED.port, protocol = EXCLUDED.protocol,
			interval_seconds = EXCLUDED.interval_seconds, timeout_seconds = EXCLUDED.timeout_seconds,
			healthy_threshold = EXCLUDED.healthy_threshold, unhealthy_threshold = EXCLUDED.unhealthy_threshold,
			updated_at = now()
	`, c.ID, c.ServerID, c.Path, c.Port, c.Protocol, c.IntervalSeconds, c.TimeoutSeconds,
		c.HealthyThreshold, c.UnhealthyThreshold, c.CreatedAt, c.UpdatedAt)
	return err
}

func (s *Store) CreateZeroDowntimeHealthCheckResult(ctx context.Context, r *ZeroDowntimeHealthCheckResult) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO health_check_results (id, deployment_id, check_timestamp, status, response_code, response_time_ms, error_message)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, r.ID, r.DeploymentID, r.CheckTimestamp, r.Status, r.ResponseCode, r.ResponseTimeMs, r.ErrorMessage)
	return err
}

func (s *Store) ListZeroDowntimeHealthCheckResults(ctx context.Context, deploymentID string) ([]ZeroDowntimeHealthCheckResult, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, deployment_id::text, check_timestamp, status, response_code, response_time_ms, error_message
		FROM health_check_results WHERE deployment_id = $1
		ORDER BY check_timestamp DESC
	`, deploymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ZeroDowntimeHealthCheckResult
	for rows.Next() {
		var r ZeroDowntimeHealthCheckResult
		if err := rows.Scan(&r.ID, &r.DeploymentID, &r.CheckTimestamp, &r.Status, &r.ResponseCode, &r.ResponseTimeMs, &r.ErrorMessage); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Store) CreateDeploymentEvent(ctx context.Context, e *DeploymentEvent) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO deployment_events (id, deployment_id, event_type, message, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, e.ID, e.DeploymentID, e.EventType, e.Message, e.CreatedAt)
	return err
}

func (s *Store) ListDeploymentEvents(ctx context.Context, deploymentID string) ([]DeploymentEvent, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, deployment_id::text, event_type, message, created_at
		FROM deployment_events WHERE deployment_id = $1
		ORDER BY created_at ASC
	`, deploymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DeploymentEvent
	for rows.Next() {
		var e DeploymentEvent
		if err := rows.Scan(&e.ID, &e.DeploymentID, &e.EventType, &e.Message, &e.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}
