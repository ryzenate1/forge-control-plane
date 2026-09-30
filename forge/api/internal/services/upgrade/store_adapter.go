package upgrade

import (
	"context"
	"encoding/json"

	"gamepanel/forge/internal/store"
)

// StoreAdapter adapts *store.Store to the Store interface. The store package
// owns the upgrade_plans table mapping (components persisted as a JSON array)
// while the service works with a decoded []string, so the adapter converts at
// the boundary in both directions.
type StoreAdapter struct {
	db *store.Store
}

// AdaptStore wraps db so it can be passed to New. It returns nil when db is
// nil so the service keeps its existing nil-store guard behaviour.
func AdaptStore(db *store.Store) Store {
	if db == nil {
		return nil
	}
	return &StoreAdapter{db: db}
}

func toStorePlan(p *UpgradePlan) store.UpgradePlan {
	var sp store.UpgradePlan
	if p == nil {
		return sp
	}
	comps, err := json.Marshal(p.Components)
	if err != nil || comps == nil {
		comps = []byte("[]")
	}
	sp.ID = p.ID
	sp.Type = string(p.Type)
	sp.FromVersion = p.FromVersion
	sp.ToVersion = p.ToVersion
	sp.Components = comps
	sp.Status = string(p.Status)
	sp.Progress = p.Progress
	sp.TotalSteps = p.TotalSteps
	sp.CurrentStep = p.CurrentStep
	sp.Error = p.Error
	sp.BackupPath = p.BackupPath
	sp.StartedAt = p.StartedAt
	sp.CompletedAt = p.CompletedAt
	sp.CreatedAt = p.CreatedAt
	sp.UpdatedAt = p.UpdatedAt
	return sp
}

func fromStorePlan(sp *store.UpgradePlan) *UpgradePlan {
	if sp == nil {
		return nil
	}
	var comps []string
	if len(sp.Components) > 0 {
		_ = json.Unmarshal(sp.Components, &comps)
	}
	return &UpgradePlan{
		ID:          sp.ID,
		Type:        UpgradeType(sp.Type),
		FromVersion: sp.FromVersion,
		ToVersion:   sp.ToVersion,
		Components:  comps,
		Status:      UpgradeStatus(sp.Status),
		Progress:    sp.Progress,
		TotalSteps:  sp.TotalSteps,
		CurrentStep: sp.CurrentStep,
		Error:       sp.Error,
		BackupPath:  sp.BackupPath,
		StartedAt:   sp.StartedAt,
		CompletedAt: sp.CompletedAt,
		CreatedAt:   sp.CreatedAt,
		UpdatedAt:   sp.UpdatedAt,
	}
}

func (a *StoreAdapter) CreateUpgradePlan(ctx context.Context, plan *UpgradePlan) error {
	saved, err := a.db.CreateUpgradePlan(ctx, toStorePlan(plan))
	if err != nil {
		return err
	}
	*plan = *fromStorePlan(saved)
	return nil
}

func (a *StoreAdapter) GetUpgradePlan(ctx context.Context, id string) (*UpgradePlan, error) {
	sp, err := a.db.GetUpgradePlan(ctx, id)
	if err != nil {
		return nil, err
	}
	return fromStorePlan(sp), nil
}

func (a *StoreAdapter) ListUpgradePlans(ctx context.Context, limit int) ([]UpgradePlan, error) {
	rows, err := a.db.ListUpgradePlans(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]UpgradePlan, 0, len(rows))
	for i := range rows {
		if p := fromStorePlan(&rows[i]); p != nil {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (a *StoreAdapter) UpdateUpgradePlan(ctx context.Context, plan *UpgradePlan) error {
	saved, err := a.db.UpdateUpgradePlan(ctx, toStorePlan(plan))
	if err != nil {
		return err
	}
	*plan = *fromStorePlan(saved)
	return nil
}

func (a *StoreAdapter) DeleteUpgradePlan(ctx context.Context, id string) error {
	return a.db.DeleteUpgradePlan(ctx, id)
}

// GetLatestUpgrade returns the most recently created plan that includes the
// named component, or (nil, nil) when no plan mentions it.
func (a *StoreAdapter) GetLatestUpgrade(ctx context.Context, component string) (*UpgradePlan, error) {
	rows, err := a.db.ListUpgradePlans(ctx, 100)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		var comps []string
		if len(rows[i].Components) > 0 {
			_ = json.Unmarshal(rows[i].Components, &comps)
		}
		for _, c := range comps {
			if c == component {
				return fromStorePlan(&rows[i]), nil
			}
		}
	}
	return nil, nil
}
