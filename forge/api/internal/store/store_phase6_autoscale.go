package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Phase 6 node autoscaling policies and audit events (migrations 192/193).

type NodeAutoscalePolicy struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	ClusterGroupID  string    `json:"clusterGroupId,omitempty"`
	Provider        string    `json:"provider"`
	Region          string    `json:"region"`
	InstanceType    string    `json:"instanceType"`
	Image           string    `json:"image"`
	MinNodes        int       `json:"minNodes"`
	MaxNodes        int       `json:"maxNodes"`
	TargetCPUPercent float64  `json:"targetCpuPercent"`
	TargetMemPercent float64  `json:"targetMemPercent"`
	Evaluator       string    `json:"evaluator"` // cpu | memory | both
	AutoJoin        bool      `json:"autoJoin"`
	CooldownSeconds int       `json:"cooldownSeconds"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func (p *NodeAutoscalePolicy) normalize() {
	if p.Provider == "" {
		p.Provider = "aws"
	}
	if p.Evaluator == "" {
		p.Evaluator = "cpu"
	}
	if p.MinNodes < 1 {
		p.MinNodes = 1
	}
	if p.MaxNodes < p.MinNodes {
		p.MaxNodes = p.MinNodes
	}
	if p.CooldownSeconds <= 0 {
		p.CooldownSeconds = 900
	}
}

type NodeAutoscaleEvent struct {
	ID        string          `json:"id"`
	PolicyID  string          `json:"policyId"`
	Action    string          `json:"action"` // scale-out | scale-in | evaluate | error
	Direction string          `json:"direction"`
	State     string          `json:"state"` // suggested | applied | failed
	Deficit   int             `json:"deficit"`
	Detail    json.RawMessage `json:"detail"`
	CreatedAt time.Time       `json:"createdAt"`
}

func (s *Store) CreateNodeAutoscalePolicy(ctx context.Context, p *NodeAutoscalePolicy) error {
	p.normalize()
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	_, err := s.db.Exec(ctx, `
		INSERT INTO node_autoscale_policies (
			id, name, cluster_group_id, provider, region, instance_type, image,
			min_nodes, max_nodes, target_cpu_percent, target_mem_percent,
			evaluator, auto_join, cooldown_seconds, enabled, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`, p.ID, p.Name, nullIfEmpty(p.ClusterGroupID), p.Provider, p.Region, p.InstanceType, p.Image,
		p.MinNodes, p.MaxNodes, p.TargetCPUPercent, p.TargetMemPercent,
		p.Evaluator, p.AutoJoin, p.CooldownSeconds, p.Enabled, now, now)
	return err
}

func (s *Store) GetNodeAutoscalePolicy(ctx context.Context, policyID string) (NodeAutoscalePolicy, error) {
	var p NodeAutoscalePolicy
	var clusterGroupID sql.NullString
	err := s.db.QueryRow(ctx, `
		SELECT id::text, name, cluster_group_id, provider, region, instance_type, image,
		       min_nodes, max_nodes, target_cpu_percent, target_mem_percent,
		       evaluator, auto_join, cooldown_seconds, enabled, created_at, updated_at
		FROM node_autoscale_policies WHERE id = $1
	`, policyID).Scan(
		&p.ID, &p.Name, &clusterGroupID, &p.Provider, &p.Region, &p.InstanceType, &p.Image,
		&p.MinNodes, &p.MaxNodes, &p.TargetCPUPercent, &p.TargetMemPercent,
		&p.Evaluator, &p.AutoJoin, &p.CooldownSeconds, &p.Enabled, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return NodeAutoscalePolicy{}, err
	}
	if clusterGroupID.Valid {
		p.ClusterGroupID = clusterGroupID.String
	}
	return p, nil
}

func (s *Store) ListNodeAutoscalePolicies(ctx context.Context) ([]NodeAutoscalePolicy, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, cluster_group_id, provider, region, instance_type, image,
		       min_nodes, max_nodes, target_cpu_percent, target_mem_percent,
		       evaluator, auto_join, cooldown_seconds, enabled, created_at, updated_at
		FROM node_autoscale_policies ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	policies := []NodeAutoscalePolicy{}
	for rows.Next() {
		var p NodeAutoscalePolicy
		var clusterGroupID sql.NullString
		if err := rows.Scan(
			&p.ID, &p.Name, &clusterGroupID, &p.Provider, &p.Region, &p.InstanceType, &p.Image,
			&p.MinNodes, &p.MaxNodes, &p.TargetCPUPercent, &p.TargetMemPercent,
			&p.Evaluator, &p.AutoJoin, &p.CooldownSeconds, &p.Enabled, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if clusterGroupID.Valid {
			p.ClusterGroupID = clusterGroupID.String
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *Store) UpdateNodeAutoscalePolicy(ctx context.Context, policyID string, p *NodeAutoscalePolicy) (NodeAutoscalePolicy, error) {
	p.normalize()
	_, err := s.db.Exec(ctx, `
		UPDATE node_autoscale_policies SET
			name = $2, cluster_group_id = $3, provider = $4, region = $5,
			instance_type = $6, image = $7, min_nodes = $8, max_nodes = $9,
			target_cpu_percent = $10, target_mem_percent = $11, evaluator = $12,
			auto_join = $13, cooldown_seconds = $14, enabled = $15, updated_at = now()
		WHERE id = $1
	`, policyID, p.Name, nullIfEmpty(p.ClusterGroupID), p.Provider, p.Region,
		p.InstanceType, p.Image, p.MinNodes, p.MaxNodes,
		p.TargetCPUPercent, p.TargetMemPercent, p.Evaluator,
		p.AutoJoin, p.CooldownSeconds, p.Enabled)
	if err != nil {
		return NodeAutoscalePolicy{}, err
	}
	return s.GetNodeAutoscalePolicy(ctx, policyID)
}

func (s *Store) DeleteNodeAutoscalePolicy(ctx context.Context, policyID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM node_autoscale_policies WHERE id = $1`, policyID)
	return err
}

// RecordNodeAutoscaleEvent appends one audit row to the ledger.
func (s *Store) RecordNodeAutoscaleEvent(ctx context.Context, e *NodeAutoscaleEvent) error {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.Detail == nil {
		e.Detail = json.RawMessage(`{}`)
	}
	e.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO node_autoscale_events (id, policy_id, action, direction, state, deficit, detail, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8)
	`, e.ID, nullIfEmpty(e.PolicyID), e.Action, e.Direction, e.State, e.Deficit, string(e.Detail), e.CreatedAt)
	return err
}

func (s *Store) ListNodeAutoscaleEvents(ctx context.Context, policyID string, limit int) ([]NodeAutoscaleEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `
		SELECT id::text, policy_id::text, action, direction, state, deficit, detail::text, created_at
		FROM node_autoscale_events
	`
	args := []any{}
	if policyID != "" {
		query += ` WHERE policy_id = $1`
		args = append(args, policyID)
	}
	query += ` ORDER BY created_at DESC LIMIT $` + itoa(len(args)+1)
	args = append(args, limit)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []NodeAutoscaleEvent{}
	for rows.Next() {
		var e NodeAutoscaleEvent
		var policyIDVal sql.NullString
		var detail string
		if err := rows.Scan(&e.ID, &policyIDVal, &e.Action, &e.Direction, &e.State, &e.Deficit, &detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		if policyIDVal.Valid {
			e.PolicyID = policyIDVal.String
		}
		e.Detail = json.RawMessage(detail)
		events = append(events, e)
	}
	return events, rows.Err()
}
