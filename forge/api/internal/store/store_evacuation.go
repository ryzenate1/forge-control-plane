package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

func (s *Store) CreateEvacuationPlan(ctx context.Context, nodeID string, status EvacuationPlanStatus, items []EvacuationItem) (EvacuationPlan, error) {
	planID := uuid.NewString()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return EvacuationPlan{}, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO evacuation_plans (id, node_id, status)
		VALUES ($1, $2, $3::evacuation_plan_status)
	`, planID, nodeID, string(status)); err != nil {
		return EvacuationPlan{}, err
	}
	for _, item := range items {
		var target any
		if item.TargetNodeID != "" {
			target = item.TargetNodeID
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO evacuation_items (id, plan_id, server_id, source_node_id, target_node_id, eligible, reason, status, error)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, uuid.NewString(), planID, item.ServerID, item.SourceNodeID, target, item.Eligible, item.Reason, evacuationItemInitialStatus(item), evacuationItemInitialError(item)); err != nil {
			return EvacuationPlan{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return EvacuationPlan{}, err
	}
	return s.GetEvacuationPlan(ctx, planID)
}

func (s *Store) GetEvacuationPlan(ctx context.Context, planID string) (EvacuationPlan, error) {
	var plan EvacuationPlan
	if err := s.db.QueryRow(ctx, `
		SELECT id::text, node_id::text, status::text, created_at, updated_at
		FROM evacuation_plans
		WHERE id = $1
	`, planID).Scan(&plan.ID, &plan.NodeID, &plan.Status, &plan.CreatedAt, &plan.UpdatedAt); err != nil {
		return EvacuationPlan{}, err
	}
	items, err := s.ListEvacuationItems(ctx, planID)
	if err != nil {
		return EvacuationPlan{}, err
	}
	plan.Items = items
	return plan, nil
}

// ListEvacuationPlansByStatus returns persisted plans in a state which needs
// executor attention. Callers should still atomically claim individual plans.
func (s *Store) ListEvacuationPlansByStatus(ctx context.Context, status EvacuationPlanStatus) ([]EvacuationPlan, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text
		FROM evacuation_plans
		WHERE status = $1::evacuation_plan_status
		ORDER BY created_at, id
	`, string(status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	plans := []EvacuationPlan{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		plan, err := s.GetEvacuationPlan(ctx, id)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, rows.Err()
}

func (s *Store) ListEvacuationItems(ctx context.Context, planID string) ([]EvacuationItem, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, plan_id::text, server_id::text, source_node_id::text, target_node_id::text, eligible, reason, COALESCE(migration_id::text, ''), status, error
		FROM evacuation_items
		WHERE plan_id = $1
		ORDER BY created_at, id
	`, planID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []EvacuationItem{}
	for rows.Next() {
		var item EvacuationItem
		var target, itemError sql.NullString
		if err := rows.Scan(&item.ID, &item.PlanID, &item.ServerID, &item.SourceNodeID, &target, &item.Eligible, &item.Reason, &item.MigrationID, &item.Status, &itemError); err != nil {
			return nil, err
		}
		if target.Valid {
			item.TargetNodeID = target.String
		}
		if itemError.Valid {
			item.Error = &itemError.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func evacuationItemInitialStatus(item EvacuationItem) string {
	if item.Eligible {
		return "pending"
	}
	return "failed"
}

func evacuationItemInitialError(item EvacuationItem) any {
	if item.Eligible || item.Reason == "" {
		return nil
	}
	return item.Reason
}

// StartEvacuationPlan atomically claims a pending plan for execution. The
// returned boolean is false when another worker has already claimed the plan.
func (s *Store) StartEvacuationPlan(ctx context.Context, planID string) (EvacuationPlan, bool, error) {
	commandTag, err := s.db.Exec(ctx, `
		UPDATE evacuation_plans
		SET status = 'running'::evacuation_plan_status, updated_at = now()
		WHERE id = $1 AND status = 'pending'::evacuation_plan_status
	`, planID)
	if err != nil {
		return EvacuationPlan{}, false, err
	}
	plan, err := s.GetEvacuationPlan(ctx, planID)
	if err != nil {
		return EvacuationPlan{}, false, err
	}
	return plan, commandTag.RowsAffected() == 1, nil
}

// CancelEvacuationPlan prevents new work from starting. Active migration jobs
// are cancelled by the executor before this terminal state is persisted.
func (s *Store) CancelEvacuationPlan(ctx context.Context, planID string) (EvacuationPlan, error) {
	commandTag, err := s.db.Exec(ctx, `
		UPDATE evacuation_plans
		SET status = 'cancelled'::evacuation_plan_status, updated_at = now()
		WHERE id = $1 AND status = 'running'::evacuation_plan_status
	`, planID)
	if err != nil {
		return EvacuationPlan{}, err
	}
	if commandTag.RowsAffected() != 1 {
		return EvacuationPlan{}, fmt.Errorf("evacuation plan is not running")
	}
	return s.GetEvacuationPlan(ctx, planID)
}

func (s *Store) UpdateEvacuationPlanStatus(ctx context.Context, planID string, status EvacuationPlanStatus) (EvacuationPlan, error) {
	if _, err := s.db.Exec(ctx, `
		UPDATE evacuation_plans SET status = $2::evacuation_plan_status, updated_at = now() WHERE id = $1
	`, planID, string(status)); err != nil {
		return EvacuationPlan{}, err
	}
	return s.GetEvacuationPlan(ctx, planID)
}

func (s *Store) UpdateEvacuationItemExecution(ctx context.Context, itemID, migrationID, status string, itemErr error) error {
	var errorMessage any
	if itemErr != nil {
		errorMessage = itemErr.Error()
	}
	_, err := s.db.Exec(ctx, `
		UPDATE evacuation_items
		SET migration_id = NULLIF($2, '')::uuid, status = $3, error = $4
		WHERE id = $1
	`, itemID, migrationID, status, errorMessage)
	return err
}

func (s *Store) EvacuationPlansTotal(ctx context.Context) (int64, error) {
	var total int64
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM evacuation_plans`).Scan(&total)
	return total, err
}
