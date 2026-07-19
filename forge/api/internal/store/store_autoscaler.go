package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type ScalingPolicy struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	ServerID            string    `json:"serverId"`
	Enabled             bool      `json:"enabled"`
	MinMemoryMB         int64     `json:"minMemoryMb"`
	MaxMemoryMB         int64     `json:"maxMemoryMb"`
	MinCPU              int64     `json:"minCpu"`
	MaxCPU              int64     `json:"maxCpu"`
	TargetCPUPercent    float64   `json:"targetCpuPercent"`
	TargetMemoryPercent float64   `json:"targetMemoryPercent"`
	ScaleUpThreshold    float64   `json:"scaleUpThreshold"`
	ScaleDownThreshold  float64   `json:"scaleDownThreshold"`
	CooldownSeconds     int       `json:"cooldownSeconds"`
	PollIntervalSeconds int       `json:"pollIntervalSeconds"`
	ScaleUpFactor       float64   `json:"scaleUpFactor"`
	ScaleDownFactor     float64   `json:"scaleDownFactor"`
	MaxScaleUpStepMB    int64     `json:"maxScaleUpStepMb"`
	MaxScaleDownStepMB  int64     `json:"maxScaleDownStepMb"`
	CreatedAt           time.Time `json:"createdAt"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

type ScalingEvent struct {
	ID        string    `json:"id"`
	PolicyID  string    `json:"policyId"`
	ServerID  string    `json:"serverId"`
	Direction string    `json:"direction"`
	OldMemory int64     `json:"oldMemory"`
	NewMemory int64     `json:"newMemory"`
	OldCPU    int64     `json:"oldCpu"`
	NewCPU    int64     `json:"newCpu"`
	CPUUsage  float64   `json:"cpuUsage"`
	MemUsage  float64   `json:"memUsage"`
	Reason    string    `json:"reason"`
	Success   bool      `json:"success"`
	CreatedAt time.Time `json:"timestamp"`
}

func (s *Store) CreateScalingPolicy(ctx context.Context, p *ScalingPolicy) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	_, err := s.db.Exec(ctx, `
		INSERT INTO scaling_policies (
			id, name, server_id, enabled,
			min_memory_mb, max_memory_mb, min_cpu, max_cpu,
			target_cpu_percent, target_memory_percent,
			scale_up_threshold, scale_down_threshold,
			cooldown_seconds, poll_interval_seconds,
			scale_up_factor, scale_down_factor,
			max_scale_up_step_mb, max_scale_down_step_mb,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8,
			$9, $10,
			$11, $12,
			$13, $14,
			$15, $16,
			$17, $18,
			$19, $20
		)
	`, p.ID, p.Name, p.ServerID, p.Enabled,
		p.MinMemoryMB, p.MaxMemoryMB, p.MinCPU, p.MaxCPU,
		p.TargetCPUPercent, p.TargetMemoryPercent,
		p.ScaleUpThreshold, p.ScaleDownThreshold,
		p.CooldownSeconds, p.PollIntervalSeconds,
		p.ScaleUpFactor, p.ScaleDownFactor,
		p.MaxScaleUpStepMB, p.MaxScaleDownStepMB,
		p.CreatedAt, p.UpdatedAt)
	return err
}

func (s *Store) GetScalingPolicy(ctx context.Context, id string) (ScalingPolicy, error) {
	var p ScalingPolicy
	err := s.db.QueryRow(ctx, `
		SELECT
			id::text, name, server_id::text, enabled,
			min_memory_mb, max_memory_mb, min_cpu, max_cpu,
			target_cpu_percent, target_memory_percent,
			scale_up_threshold, scale_down_threshold,
			cooldown_seconds, poll_interval_seconds,
			scale_up_factor, scale_down_factor,
			max_scale_up_step_mb, max_scale_down_step_mb,
			created_at, updated_at
		FROM scaling_policies WHERE id = $1
	`, id).Scan(
		&p.ID, &p.Name, &p.ServerID, &p.Enabled,
		&p.MinMemoryMB, &p.MaxMemoryMB, &p.MinCPU, &p.MaxCPU,
		&p.TargetCPUPercent, &p.TargetMemoryPercent,
		&p.ScaleUpThreshold, &p.ScaleDownThreshold,
		&p.CooldownSeconds, &p.PollIntervalSeconds,
		&p.ScaleUpFactor, &p.ScaleDownFactor,
		&p.MaxScaleUpStepMB, &p.MaxScaleDownStepMB,
		&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, err
	}
	return p, nil
}

func (s *Store) ListScalingPolicies(ctx context.Context) ([]ScalingPolicy, error) {
	rows, err := s.db.Query(ctx, `
		SELECT
			id::text, name, server_id::text, enabled,
			min_memory_mb, max_memory_mb, min_cpu, max_cpu,
			target_cpu_percent, target_memory_percent,
			scale_up_threshold, scale_down_threshold,
			cooldown_seconds, poll_interval_seconds,
			scale_up_factor, scale_down_factor,
			max_scale_up_step_mb, max_scale_down_step_mb,
			created_at, updated_at
		FROM scaling_policies ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []ScalingPolicy
	for rows.Next() {
		var p ScalingPolicy
		if err := rows.Scan(
			&p.ID, &p.Name, &p.ServerID, &p.Enabled,
			&p.MinMemoryMB, &p.MaxMemoryMB, &p.MinCPU, &p.MaxCPU,
			&p.TargetCPUPercent, &p.TargetMemoryPercent,
			&p.ScaleUpThreshold, &p.ScaleDownThreshold,
			&p.CooldownSeconds, &p.PollIntervalSeconds,
			&p.ScaleUpFactor, &p.ScaleDownFactor,
			&p.MaxScaleUpStepMB, &p.MaxScaleDownStepMB,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *Store) ListScalingPoliciesByServer(ctx context.Context, serverID string) ([]ScalingPolicy, error) {
	rows, err := s.db.Query(ctx, `
		SELECT
			id::text, name, server_id::text, enabled,
			min_memory_mb, max_memory_mb, min_cpu, max_cpu,
			target_cpu_percent, target_memory_percent,
			scale_up_threshold, scale_down_threshold,
			cooldown_seconds, poll_interval_seconds,
			scale_up_factor, scale_down_factor,
			max_scale_up_step_mb, max_scale_down_step_mb,
			created_at, updated_at
		FROM scaling_policies WHERE server_id = $1 ORDER BY created_at
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []ScalingPolicy
	for rows.Next() {
		var p ScalingPolicy
		if err := rows.Scan(
			&p.ID, &p.Name, &p.ServerID, &p.Enabled,
			&p.MinMemoryMB, &p.MaxMemoryMB, &p.MinCPU, &p.MaxCPU,
			&p.TargetCPUPercent, &p.TargetMemoryPercent,
			&p.ScaleUpThreshold, &p.ScaleDownThreshold,
			&p.CooldownSeconds, &p.PollIntervalSeconds,
			&p.ScaleUpFactor, &p.ScaleDownFactor,
			&p.MaxScaleUpStepMB, &p.MaxScaleDownStepMB,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *Store) UpdateScalingPolicy(ctx context.Context, p *ScalingPolicy) error {
	p.UpdatedAt = time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		UPDATE scaling_policies SET
			name = $2, server_id = $3, enabled = $4,
			min_memory_mb = $5, max_memory_mb = $6, min_cpu = $7, max_cpu = $8,
			target_cpu_percent = $9, target_memory_percent = $10,
			scale_up_threshold = $11, scale_down_threshold = $12,
			cooldown_seconds = $13, poll_interval_seconds = $14,
			scale_up_factor = $15, scale_down_factor = $16,
			max_scale_up_step_mb = $17, max_scale_down_step_mb = $18,
			updated_at = $19
		WHERE id = $1
	`, p.ID, p.Name, p.ServerID, p.Enabled,
		p.MinMemoryMB, p.MaxMemoryMB, p.MinCPU, p.MaxCPU,
		p.TargetCPUPercent, p.TargetMemoryPercent,
		p.ScaleUpThreshold, p.ScaleDownThreshold,
		p.CooldownSeconds, p.PollIntervalSeconds,
		p.ScaleUpFactor, p.ScaleDownFactor,
		p.MaxScaleUpStepMB, p.MaxScaleDownStepMB,
		p.UpdatedAt)
	return err
}

func (s *Store) DeleteScalingPolicy(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM scaling_policies WHERE id = $1`, id)
	return err
}

func (s *Store) CreateScalingEvent(ctx context.Context, e *ScalingEvent) error {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	e.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO scaling_events (
			id, policy_id, server_id, direction,
			old_memory, new_memory, old_cpu, new_cpu,
			cpu_usage, mem_usage, reason, success, created_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6, $7, $8,
			$9, $10, $11, $12, $13
		)
	`, e.ID, e.PolicyID, e.ServerID, e.Direction,
		e.OldMemory, e.NewMemory, e.OldCPU, e.NewCPU,
		e.CPUUsage, e.MemUsage, e.Reason, e.Success, e.CreatedAt)
	return err
}

func (s *Store) GetLastScalingEvent(ctx context.Context, policyID string) (ScalingEvent, error) {
	var e ScalingEvent
	err := s.db.QueryRow(ctx, `
		SELECT
			id::text, policy_id::text, server_id::text, direction,
			old_memory, new_memory, old_cpu, new_cpu,
			cpu_usage, mem_usage, reason, success, created_at
		FROM scaling_events
		WHERE policy_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, policyID).Scan(
		&e.ID, &e.PolicyID, &e.ServerID, &e.Direction,
		&e.OldMemory, &e.NewMemory, &e.OldCPU, &e.NewCPU,
		&e.CPUUsage, &e.MemUsage, &e.Reason, &e.Success, &e.CreatedAt)
	if err != nil {
		return e, err
	}
	return e, nil
}
