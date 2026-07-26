package installer

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (s *PostgresStore) CreateWorkflow(ctx context.Context, wf *Workflow) error {
	steps, _ := json.Marshal(wf.Steps)
	meta, _ := json.Marshal(wf.Metadata)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO install_workflows (id, server_id, type, status, steps, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, wf.ID, wf.ServerID, string(wf.Type), string(wf.Status), steps, meta, wf.CreatedAt)
	return err
}

func (s *PostgresStore) GetWorkflow(ctx context.Context, id string) (*Workflow, error) {
	var wf Workflow
	var stepsJSON []byte
	var metaJSON []byte
	var status, wfType string

	err := s.pool.QueryRow(ctx, `
		SELECT id::text, server_id::text, type, status, steps, metadata, created_at, completed_at
		FROM install_workflows WHERE id = $1
	`, id).Scan(&wf.ID, &wf.ServerID, &wfType, &status, &stepsJSON, &metaJSON, &wf.CreatedAt, &wf.CompletedAt)

	if err != nil {
		return nil, err
	}
	wf.Type = WorkflowType(wfType)
	wf.Status = InstallStatus(status)
	json.Unmarshal(stepsJSON, &wf.Steps)
	json.Unmarshal(metaJSON, &wf.Metadata)
	return &wf, nil
}

func (s *PostgresStore) ListWorkflows(ctx context.Context, serverID string) ([]Workflow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id::text, server_id::text, type, status, steps, metadata, created_at, completed_at
		FROM install_workflows WHERE server_id = $1 ORDER BY created_at DESC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workflows []Workflow
	for rows.Next() {
		var wf Workflow
		var stepsJSON, metaJSON []byte
		var status, wfType string
		if err := rows.Scan(&wf.ID, &wf.ServerID, &wfType, &status, &stepsJSON, &metaJSON, &wf.CreatedAt, &wf.CompletedAt); err != nil {
			return nil, err
		}
		wf.Type = WorkflowType(wfType)
		wf.Status = InstallStatus(status)
		json.Unmarshal(stepsJSON, &wf.Steps)
		json.Unmarshal(metaJSON, &wf.Metadata)
		workflows = append(workflows, wf)
	}
	return workflows, rows.Err()
}

func (s *PostgresStore) UpdateStep(ctx context.Context, stepID string, status InstallStatus, errMsg string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var workflowID string
	var stepsJSON []byte
	if err := tx.QueryRow(ctx, `
		SELECT id::text, steps
		FROM install_workflows
		WHERE EXISTS (SELECT 1 FROM jsonb_array_elements(steps) step WHERE step->>'id' = $1)
		FOR UPDATE
	`, stepID).Scan(&workflowID, &stepsJSON); err != nil {
		return err
	}
	var steps []InstallStep
	if err := json.Unmarshal(stepsJSON, &steps); err != nil {
		return err
	}
	found := false
	now := time.Now().UTC()
	for index := range steps {
		if steps[index].ID != stepID {
			continue
		}
		found = true
		steps[index].Status = status
		steps[index].Error = errMsg
		if status == InstallRunning && steps[index].StartedAt == nil {
			steps[index].StartedAt = &now
		}
		if status == InstallCompleted || status == InstallFailed {
			steps[index].CompletedAt = &now
		}
		break
	}
	if !found {
		return pgx.ErrNoRows
	}
	workflowStatus := InstallRunning
	allCompleted := true
	for _, step := range steps {
		if step.Status == InstallFailed {
			workflowStatus = InstallFailed
			allCompleted = false
			break
		}
		if step.Status != InstallCompleted {
			allCompleted = false
		}
	}
	var completedAt *time.Time
	if allCompleted {
		workflowStatus = InstallCompleted
		completedAt = &now
	} else if workflowStatus == InstallFailed {
		completedAt = &now
	}
	encoded, err := json.Marshal(steps)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE install_workflows
		SET steps = $2, status = $3, completed_at = $4
		WHERE id = $1
	`, workflowID, encoded, string(workflowStatus), completedAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
