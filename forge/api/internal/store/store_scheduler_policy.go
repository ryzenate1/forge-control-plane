package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ---- Scheduler policy persistence ----

type AffinityRuleRow struct {
	ID        string    `json:"id"`
	NodeID    string    `json:"nodeId"`
	TargetTag string    `json:"targetTag"`
	Weight    float64   `json:"weight"`
	CreatedAt time.Time `json:"createdAt"`
}

type AntiAffinityRuleRow struct {
	ID        string    `json:"id"`
	NodeID    string    `json:"nodeId"`
	TargetTag string    `json:"targetTag"`
	CreatedAt time.Time `json:"createdAt"`
}

type ServerConstraintRow struct {
	ID        string    `json:"id"`
	NodeID    string    `json:"nodeId"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Operator  string    `json:"operator"`
	CreatedAt time.Time `json:"createdAt"`
}

// ---- Affinity rules ----

func (s *Store) ListAffinityRulesDB(ctx context.Context) ([]AffinityRuleRow, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `SELECT id, node_id, target_tag, weight, created_at FROM server_affinity_rules ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list affinity rules: %w", err)
	}
	defer rows.Close()
	var out []AffinityRuleRow
	for rows.Next() {
		var r AffinityRuleRow
		if err := rows.Scan(&r.ID, &r.NodeID, &r.TargetTag, &r.Weight, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) UpsertAffinityRuleDB(ctx context.Context, r AffinityRuleRow) (AffinityRuleRow, error) {
	if s == nil || s.db == nil {
		return r, nil
	}
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	r.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO server_affinity_rules (id, node_id, target_tag, weight, created_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET node_id=$2, target_tag=$3, weight=$4`,
		r.ID, r.NodeID, r.TargetTag, r.Weight, r.CreatedAt)
	if err != nil {
		return r, fmt.Errorf("upsert affinity rule: %w", err)
	}
	return r, nil
}

func (s *Store) DeleteAffinityRuleDB(ctx context.Context, id string) error {
	if s == nil || s.db == nil {
		return nil
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM server_affinity_rules WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete affinity rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.New("affinity rule not found")
	}
	return nil
}

// ---- Anti-affinity rules ----

func (s *Store) ListAntiAffinityRulesDB(ctx context.Context) ([]AntiAffinityRuleRow, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `SELECT id, node_id, target_tag, created_at FROM server_anti_affinity_rules ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list anti-affinity rules: %w", err)
	}
	defer rows.Close()
	var out []AntiAffinityRuleRow
	for rows.Next() {
		var r AntiAffinityRuleRow
		if err := rows.Scan(&r.ID, &r.NodeID, &r.TargetTag, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) UpsertAntiAffinityRuleDB(ctx context.Context, r AntiAffinityRuleRow) (AntiAffinityRuleRow, error) {
	if s == nil || s.db == nil {
		return r, nil
	}
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	r.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO server_anti_affinity_rules (id, node_id, target_tag, created_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET node_id=$2, target_tag=$3`,
		r.ID, r.NodeID, r.TargetTag, r.CreatedAt)
	if err != nil {
		return r, fmt.Errorf("upsert anti-affinity rule: %w", err)
	}
	return r, nil
}

func (s *Store) DeleteAntiAffinityRuleDB(ctx context.Context, id string) error {
	if s == nil || s.db == nil {
		return nil
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM server_anti_affinity_rules WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete anti-affinity rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.New("anti-affinity rule not found")
	}
	return nil
}

// ---- Constraints ----

func (s *Store) ListConstraintsDB(ctx context.Context) ([]ServerConstraintRow, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `SELECT id, node_id, key, value, operator, created_at FROM server_constraints ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list constraints: %w", err)
	}
	defer rows.Close()
	var out []ServerConstraintRow
	for rows.Next() {
		var r ServerConstraintRow
		if err := rows.Scan(&r.ID, &r.NodeID, &r.Key, &r.Value, &r.Operator, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) UpsertConstraintDB(ctx context.Context, r ServerConstraintRow) (ServerConstraintRow, error) {
	if s == nil || s.db == nil {
		return r, nil
	}
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	r.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO server_constraints (id, node_id, key, value, operator, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (node_id, key) DO UPDATE SET value=$4, operator=$5`,
		r.ID, r.NodeID, r.Key, r.Value, r.Operator, r.CreatedAt)
	if err != nil {
		return r, fmt.Errorf("upsert constraint: %w", err)
	}
	return r, nil
}

func (s *Store) DeleteConstraintDB(ctx context.Context, id string) error {
	if s == nil || s.db == nil {
		return nil
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM server_constraints WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete constraint: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.New("constraint not found")
	}
	return nil
}
