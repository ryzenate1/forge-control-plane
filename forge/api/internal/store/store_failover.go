package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type FailoverPolicy struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	NodeID           string    `json:"nodeId"`
	Enabled          bool      `json:"enabled"`
	MaxFailures      int       `json:"maxFailures"`
	FailureWindowSec int       `json:"failureWindowSec"`
	CooldownSec      int       `json:"cooldownSec"`
	Action           string    `json:"action"`
	HealthCheckPath  string    `json:"healthCheckPath,omitempty"`
	HealthCheckPort  int       `json:"healthCheckPort,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type FailoverEvent struct {
	ID        string    `json:"id"`
	PolicyID  string    `json:"policyId"`
	NodeID    string    `json:"nodeId"`
	ServerID  string    `json:"serverId,omitempty"`
	EventType string    `json:"eventType"`
	Action    string    `json:"action"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"timestamp"`
}

func (s *Store) CreateFailoverPolicy(ctx context.Context, p *FailoverPolicy) error {
	p.ID = uuid.NewString()
	p.CreatedAt = time.Now().UTC()
	p.UpdatedAt = time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO failover_policies (id, name, node_id, enabled, max_failures, failure_window_sec, cooldown_sec, action, health_check_path, health_check_port, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, p.ID, p.Name, p.NodeID, p.Enabled, p.MaxFailures, p.FailureWindowSec, p.CooldownSec, p.Action, p.HealthCheckPath, p.HealthCheckPort, p.CreatedAt, p.UpdatedAt)
	return err
}

func (s *Store) GetFailoverPolicy(ctx context.Context, id string) (FailoverPolicy, error) {
	var p FailoverPolicy
	err := s.db.QueryRow(ctx, `
		SELECT id::text, name, node_id::text, enabled, max_failures, failure_window_sec, cooldown_sec, action, health_check_path, health_check_port, created_at, updated_at
		FROM failover_policies
		WHERE id = $1
	`, id).Scan(&p.ID, &p.Name, &p.NodeID, &p.Enabled, &p.MaxFailures, &p.FailureWindowSec, &p.CooldownSec, &p.Action, &p.HealthCheckPath, &p.HealthCheckPort, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (s *Store) ListFailoverPolicies(ctx context.Context) ([]FailoverPolicy, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, node_id::text, enabled, max_failures, failure_window_sec, cooldown_sec, action, health_check_path, health_check_port, created_at, updated_at
		FROM failover_policies
		ORDER BY created_at, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []FailoverPolicy
	for rows.Next() {
		var p FailoverPolicy
		if err := rows.Scan(&p.ID, &p.Name, &p.NodeID, &p.Enabled, &p.MaxFailures, &p.FailureWindowSec, &p.CooldownSec, &p.Action, &p.HealthCheckPath, &p.HealthCheckPort, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *Store) ListFailoverPoliciesByNode(ctx context.Context, nodeID string) ([]FailoverPolicy, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, node_id::text, enabled, max_failures, failure_window_sec, cooldown_sec, action, health_check_path, health_check_port, created_at, updated_at
		FROM failover_policies
		WHERE node_id = $1
		ORDER BY created_at, id
	`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []FailoverPolicy
	for rows.Next() {
		var p FailoverPolicy
		if err := rows.Scan(&p.ID, &p.Name, &p.NodeID, &p.Enabled, &p.MaxFailures, &p.FailureWindowSec, &p.CooldownSec, &p.Action, &p.HealthCheckPath, &p.HealthCheckPort, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *Store) UpdateFailoverPolicy(ctx context.Context, p *FailoverPolicy) error {
	p.UpdatedAt = time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		UPDATE failover_policies
		SET name = $2, node_id = $3, enabled = $4, max_failures = $5, failure_window_sec = $6, cooldown_sec = $7, action = $8, health_check_path = $9, health_check_port = $10, updated_at = $11
		WHERE id = $1
	`, p.ID, p.Name, p.NodeID, p.Enabled, p.MaxFailures, p.FailureWindowSec, p.CooldownSec, p.Action, p.HealthCheckPath, p.HealthCheckPort, p.UpdatedAt)
	return err
}

func (s *Store) DeleteFailoverPolicy(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM failover_policies WHERE id = $1`, id)
	return err
}

func (s *Store) CreateFailoverEvent(ctx context.Context, e *FailoverEvent) error {
	e.ID = uuid.NewString()
	e.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO failover_events (id, policy_id, node_id, server_id, event_type, action, status, message, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, e.ID, nullableUUID(e.PolicyID), e.NodeID, e.ServerID, e.EventType, e.Action, e.Status, e.Message, e.CreatedAt)
	return err
}

func (s *Store) ListFailoverEvents(ctx context.Context, policyID string, limit int) ([]FailoverEvent, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, COALESCE(policy_id::text, ''), node_id::text, server_id, event_type, action, status, message, created_at
		FROM failover_events
		WHERE policy_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, policyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []FailoverEvent
	for rows.Next() {
		var e FailoverEvent
		if err := rows.Scan(&e.ID, &e.PolicyID, &e.NodeID, &e.ServerID, &e.EventType, &e.Action, &e.Status, &e.Message, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *Store) ListRecentFailoverEvents(ctx context.Context, nodeID string, limit int) ([]FailoverEvent, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, COALESCE(policy_id::text, ''), node_id::text, server_id, event_type, action, status, message, created_at
		FROM failover_events
		WHERE node_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, nodeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []FailoverEvent
	for rows.Next() {
		var e FailoverEvent
		if err := rows.Scan(&e.ID, &e.PolicyID, &e.NodeID, &e.ServerID, &e.EventType, &e.Action, &e.Status, &e.Message, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
