package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ReconcilePlanRow struct {
	ID           string          `json:"id"`
	ResourceID   string          `json:"resourceId"`
	ResourceKind string          `json:"resourceKind"`
	State        string          `json:"state"`
	Destructive  bool            `json:"destructive"`
	Confirmed    bool            `json:"confirmed"`
	DiffCount    int             `json:"diffCount"`
	DriftCount   int             `json:"driftCount"`
	DiffData     json.RawMessage `json:"diffData"`
	DriftData    json.RawMessage `json:"driftData"`
	Error        string          `json:"error,omitempty"`
	CreatedAt    time.Time       `json:"createdAt"`
	ExecutedAt   *time.Time      `json:"executedAt,omitempty"`
	ExpiresAt    *time.Time      `json:"expiresAt,omitempty"`
}

type ReconcileEventRow struct {
	ID           string    `json:"id"`
	PlanID       string    `json:"planId"`
	ResourceID   string    `json:"resourceId"`
	ResourceKind string    `json:"resourceKind"`
	EventType    string    `json:"eventType"`
	Summary      string    `json:"summary"`
	CreatedAt    time.Time `json:"createdAt"`
}

func (s *Store) CreateReconcilePlan(ctx context.Context, plan *ReconcilePlanRow) error {
	diffJSON, _ := json.Marshal(plan.DiffData)
	driftJSON, _ := json.Marshal(plan.DriftData)
	if plan.ID == "" {
		plan.ID = uuid.NewString()
	}
	plan.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO reconcile_plans (id, resource_id, resource_kind, state, destructive, confirmed, diff_count, drift_count, diff_data, drift_data, error, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, plan.ID, plan.ResourceID, plan.ResourceKind, plan.State, plan.Destructive, plan.Confirmed,
		plan.DiffCount, plan.DriftCount, diffJSON, driftJSON, plan.Error, plan.CreatedAt)
	return err
}

func (s *Store) GetReconcilePlan(ctx context.Context, id string) (*ReconcilePlanRow, error) {
	var row ReconcilePlanRow
	var diffData, driftData []byte
	err := s.db.QueryRow(ctx, `
		SELECT id, resource_id, resource_kind, state, destructive, confirmed, diff_count, drift_count, diff_data, drift_data,
		       COALESCE(error, ''), created_at, executed_at
		FROM reconcile_plans WHERE id = $1
	`, id).Scan(&row.ID, &row.ResourceID, &row.ResourceKind, &row.State, &row.Destructive, &row.Confirmed,
		&row.DiffCount, &row.DriftCount, &diffData, &driftData, &row.Error, &row.CreatedAt, &row.ExecutedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	row.DiffData = diffData
	row.DriftData = driftData
	return &row, nil
}

func (s *Store) ListReconcilePlansByResource(ctx context.Context, resourceID, resourceKind string) ([]ReconcilePlanRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, resource_id, resource_kind, state, destructive, confirmed, diff_count, drift_count, diff_data, drift_data,
		       COALESCE(error, ''), created_at, executed_at
		FROM reconcile_plans
		WHERE resource_id = $1 AND ($2 = '' OR resource_kind = $2)
		ORDER BY created_at DESC LIMIT 100
	`, resourceID, resourceKind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []ReconcilePlanRow
	for rows.Next() {
		var row ReconcilePlanRow
		var diffData, driftData []byte
		if err := rows.Scan(&row.ID, &row.ResourceID, &row.ResourceKind, &row.State, &row.Destructive, &row.Confirmed,
			&row.DiffCount, &row.DriftCount, &diffData, &driftData, &row.Error, &row.CreatedAt, &row.ExecutedAt); err != nil {
			return nil, err
		}
		row.DiffData = diffData
		row.DriftData = driftData
		plans = append(plans, row)
	}
	return plans, rows.Err()
}

func (s *Store) ListReconcilePlans(ctx context.Context, offset, limit int) ([]ReconcilePlanRow, int, error) {
	if limit <= 0 {
		limit = 50
	}
	var total int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM reconcile_plans`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(ctx, `
		SELECT id, resource_id, resource_kind, state, destructive, confirmed, diff_count, drift_count, diff_data, drift_data,
		       COALESCE(error, ''), created_at, executed_at
		FROM reconcile_plans
		ORDER BY created_at DESC LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var plans []ReconcilePlanRow
	for rows.Next() {
		var row ReconcilePlanRow
		var diffData, driftData []byte
		if err := rows.Scan(&row.ID, &row.ResourceID, &row.ResourceKind, &row.State, &row.Destructive, &row.Confirmed,
			&row.DiffCount, &row.DriftCount, &diffData, &driftData, &row.Error, &row.CreatedAt, &row.ExecutedAt); err != nil {
			return nil, 0, err
		}
		row.DiffData = diffData
		row.DriftData = driftData
		plans = append(plans, row)
	}
	return plans, total, rows.Err()
}

func (s *Store) UpdateReconcilePlanState(ctx context.Context, id, state, execError string) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		UPDATE reconcile_plans SET state = $1, error = $2, executed_at = CASE WHEN $3 THEN $4 ELSE executed_at END
		WHERE id = $5
	`, state, execError, state == "succeeded" || state == "failed", now, id)
	return err
}

func (s *Store) ConfirmReconcilePlan(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `UPDATE reconcile_plans SET confirmed = true, state = 'confirmed' WHERE id = $1`, id)
	return err
}

func (s *Store) RecordReconcileEvent(ctx context.Context, event *ReconcileEventRow) error {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	event.CreatedAt = time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO reconcile_events (id, plan_id, resource_id, resource_kind, event_type, summary, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, event.ID, event.PlanID, event.ResourceID, event.ResourceKind, event.EventType, event.Summary, event.CreatedAt)
	return err
}

func (s *Store) ListReconcileEvents(ctx context.Context, resourceID string, limit int) ([]ReconcileEventRow, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, plan_id, resource_id, resource_kind, event_type, summary, created_at
		FROM reconcile_events
		WHERE ($1 = '' OR resource_id = $1)
		ORDER BY created_at DESC LIMIT $2
	`, resourceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []ReconcileEventRow
	for rows.Next() {
		var e ReconcileEventRow
		if err := rows.Scan(&e.ID, &e.PlanID, &e.ResourceID, &e.ResourceKind, &e.EventType, &e.Summary, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

type ReconcileSummary struct {
	TotalPlans    int `json:"totalPlans"`
	PendingPlans  int `json:"pendingPlans"`
	FailedPlans   int `json:"failedPlans"`
	TotalDrifts   int `json:"totalDrifts"`
	Unresolved    int `json:"unresolved"`
}

func (s *Store) ReconcileSummary(ctx context.Context) (*ReconcileSummary, error) {
	var summary ReconcileSummary
	err := s.db.QueryRow(ctx, `
		SELECT
			COALESCE((SELECT COUNT(*) FROM reconcile_plans), 0),
			COALESCE((SELECT COUNT(*) FROM reconcile_plans WHERE state = 'pending' OR state = 'confirmed'), 0),
			COALESCE((SELECT COUNT(*) FROM reconcile_plans WHERE state = 'failed'), 0),
			COALESCE((SELECT SUM(drift_count) FROM reconcile_plans), 0),
			COALESCE((SELECT COUNT(*) FROM reconcile_plans WHERE state = 'pending' AND destructive = false), 0)
	`).Scan(&summary.TotalPlans, &summary.PendingPlans, &summary.FailedPlans, &summary.TotalDrifts, &summary.Unresolved)
	if err != nil {
		return nil, err
	}
	return &summary, nil
}
