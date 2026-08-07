package store

import (
	"context"
	"time"
)

type TargetGroupRow struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Algorithm   string    `json:"algorithm"`
	Port        int       `json:"port"`
	Protocol    string    `json:"protocol"`
	HealthCheck []byte    `json:"healthCheck,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type TargetRow struct {
	ID          string    `json:"id"`
	GroupID     string    `json:"groupId"`
	ServerID    string    `json:"serverId"`
	NodeID      string    `json:"nodeId"`
	IP          string    `json:"ip"`
	Port        int       `json:"port"`
	Weight      int       `json:"weight"`
	Status      string    `json:"status"`
	Connections int       `json:"connections"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (s *Store) ListTargetGroups(ctx context.Context) ([]TargetGroupRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, algorithm, port, protocol, health_check, created_at, updated_at
		FROM target_groups
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	groups := make([]TargetGroupRow, 0)
	for rows.Next() {
		var g TargetGroupRow
		if err := rows.Scan(&g.ID, &g.Name, &g.Algorithm, &g.Port, &g.Protocol, &g.HealthCheck, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (s *Store) GetTargetGroup(ctx context.Context, id string) (*TargetGroupRow, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, name, algorithm, port, protocol, health_check, created_at, updated_at
		FROM target_groups
		WHERE id = $1
	`, id)

	var g TargetGroupRow
	if err := row.Scan(&g.ID, &g.Name, &g.Algorithm, &g.Port, &g.Protocol, &g.HealthCheck, &g.CreatedAt, &g.UpdatedAt); err != nil {
		return nil, err
	}
	return &g, nil
}

func (s *Store) CreateTargetGroup(ctx context.Context, group TargetGroupRow) error {
	var hc any
	if len(group.HealthCheck) > 0 {
		hc = string(group.HealthCheck)
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO target_groups (id, name, algorithm, port, protocol, health_check, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)
	`, group.ID, group.Name, group.Algorithm, group.Port, group.Protocol, hc, group.CreatedAt, group.UpdatedAt)
	return err
}

func (s *Store) UpdateTargetGroup(ctx context.Context, group TargetGroupRow) error {
	var hc any
	if len(group.HealthCheck) > 0 {
		hc = string(group.HealthCheck)
	}
	_, err := s.db.Exec(ctx, `
		UPDATE target_groups
		SET name = $2, algorithm = $3, port = $4, protocol = $5, health_check = $6::jsonb, updated_at = $7
		WHERE id = $1
	`, group.ID, group.Name, group.Algorithm, group.Port, group.Protocol, hc, group.UpdatedAt)
	return err
}

func (s *Store) DeleteTargetGroup(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM target_groups WHERE id = $1`, id)
	return err
}

func (s *Store) ListTargetsByGroup(ctx context.Context, groupID string) ([]TargetRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, group_id, server_id, node_id, ip, port, weight, status, connections, created_at
		FROM target_group_targets
		WHERE group_id = $1
		ORDER BY id
	`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets := make([]TargetRow, 0)
	for rows.Next() {
		var t TargetRow
		if err := rows.Scan(&t.ID, &t.GroupID, &t.ServerID, &t.NodeID, &t.IP, &t.Port, &t.Weight, &t.Status, &t.Connections, &t.CreatedAt); err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

func (s *Store) CreateTarget(ctx context.Context, target TargetRow) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO target_group_targets (id, group_id, server_id, node_id, ip, port, weight, status, connections, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, target.ID, target.GroupID, target.ServerID, target.NodeID, target.IP, target.Port, target.Weight, target.Status, target.Connections, target.CreatedAt)
	return err
}

func (s *Store) DeleteTarget(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM target_group_targets WHERE id = $1`, id)
	return err
}

func (s *Store) UpdateTargetStatus(ctx context.Context, id string, status string) error {
	_, err := s.db.Exec(ctx, `UPDATE target_group_targets SET status = $2 WHERE id = $1`, id, status)
	return err
}

func (s *Store) UpdateTargetConnections(ctx context.Context, id string, connections int) error {
	_, err := s.db.Exec(ctx, `UPDATE target_group_targets SET connections = $2 WHERE id = $1`, id, connections)
	return err
}

func (s *Store) ListTargetsByNode(ctx context.Context, nodeID string) ([]TargetRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, group_id, server_id, node_id, ip, port, weight, status, connections, created_at
		FROM target_group_targets
		WHERE node_id = $1
		ORDER BY id
	`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets := make([]TargetRow, 0)
	for rows.Next() {
		var t TargetRow
		if err := rows.Scan(&t.ID, &t.GroupID, &t.ServerID, &t.NodeID, &t.IP, &t.Port, &t.Weight, &t.Status, &t.Connections, &t.CreatedAt); err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}
