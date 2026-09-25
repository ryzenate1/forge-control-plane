package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// UpgradePlan is a planned or executed control-plane self-upgrade. Mirrors
// services/upgrade.UpgradePlan and the upgrade_plans table (migration 228).
type UpgradePlan struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	FromVersion string          `json:"fromVersion"`
	ToVersion   string          `json:"toVersion"`
	Components  json.RawMessage `json:"components"`
	Status      string          `json:"status"`
	Progress    int             `json:"progress"`
	TotalSteps  int             `json:"totalSteps"`
	CurrentStep string          `json:"currentStep"`
	Error       string          `json:"error,omitempty"`
	BackupPath  string          `json:"backupPath,omitempty"`
	StartedAt   time.Time       `json:"startedAt"`
	CompletedAt *time.Time      `json:"completedAt,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

var ErrUpgradePlanNotFound = errors.New("upgrade plan not found")

func (s *Store) CreateUpgradePlan(ctx context.Context, p UpgradePlan) (*UpgradePlan, error) {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if len(p.Components) == 0 {
		p.Components = json.RawMessage("[]")
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	p.StartedAt = now
	if _, err := s.db.Exec(ctx, `
		INSERT INTO upgrade_plans (id, type, from_version, to_version, components, status, progress, total_steps, current_step, error, backup_path, started_at, completed_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		p.ID, p.Type, p.FromVersion, p.ToVersion, []byte(p.Components), p.Status, p.Progress, p.TotalSteps, p.CurrentStep, p.Error, p.BackupPath,
		p.StartedAt, p.CompletedAt, p.CreatedAt, p.UpdatedAt); err != nil {
		return nil, fmt.Errorf("insert upgrade plan: %w", err)
	}
	return &p, nil
}

func scanUpgradePlan(row interface{ Scan(dest ...any) error }) (UpgradePlan, error) {
	var p UpgradePlan
	var comps []byte
	err := row.Scan(&p.ID, &p.Type, &p.FromVersion, &p.ToVersion, &comps, &p.Status, &p.Progress, &p.TotalSteps, &p.CurrentStep, &p.Error, &p.BackupPath, &p.StartedAt, &p.CompletedAt, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, err
	}
	p.Components = comps
	return p, nil
}

const upgradePlanCols = "id, type, from_version, to_version, components, status, progress, total_steps, current_step, error, backup_path, started_at, completed_at, created_at, updated_at"

func (s *Store) GetUpgradePlan(ctx context.Context, id string) (*UpgradePlan, error) {
	row := s.db.QueryRow(ctx, "SELECT "+upgradePlanCols+" FROM upgrade_plans WHERE id = $1", id)
	p, err := scanUpgradePlan(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUpgradePlanNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (s *Store) ListUpgradePlans(ctx context.Context, limit int) ([]UpgradePlan, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, "SELECT "+upgradePlanCols+" FROM upgrade_plans ORDER BY created_at DESC LIMIT $1", limit)
	if err != nil {
		return nil, fmt.Errorf("list upgrade plans: %w", err)
	}
	defer rows.Close()
	out := make([]UpgradePlan, 0, limit)
	for rows.Next() {
		p, err := scanUpgradePlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) UpdateUpgradePlan(ctx context.Context, p UpgradePlan) (*UpgradePlan, error) {
	if len(p.Components) == 0 {
		p.Components = json.RawMessage("[]")
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE upgrade_plans SET type=$2, from_version=$3, to_version=$4, components=$5::jsonb, status=$6, progress=$7, total_steps=$8, current_step=$9, error=$10, backup_path=$11, completed_at=$12, updated_at=now()
		WHERE id=$1`,
		p.ID, p.Type, p.FromVersion, p.ToVersion, []byte(p.Components), p.Status, p.Progress, p.TotalSteps, p.CurrentStep, p.Error, p.BackupPath, p.CompletedAt)
	if err != nil {
		return nil, fmt.Errorf("update upgrade plan: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrUpgradePlanNotFound
	}
	return s.GetUpgradePlan(ctx, p.ID)
}

func (s *Store) DeleteUpgradePlan(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, "DELETE FROM upgrade_plans WHERE id=$1", id)
	if err != nil {
		return fmt.Errorf("delete upgrade plan: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUpgradePlanNotFound
	}
	return nil
}
