package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type MaintenanceSettings struct {
	Enabled      bool   `json:"enabled"`
	Message      string `json:"message"`
	BypassToken  string `json:"bypassToken,omitempty"`
	WhitelistIPs string `json:"whitelistIps,omitempty"`
	UpdatedAt    string `json:"updatedAt"`
}

func DefaultMaintenanceSettings() MaintenanceSettings {
	return MaintenanceSettings{
		Message: "the panel is currently under maintenance",
	}
}

func (s *Store) GetMaintenanceSettings(ctx context.Context) (MaintenanceSettings, error) {
	if s.db == nil {
		return DefaultMaintenanceSettings(), errors.New("no database connection")
	}
	var (
		raw       []byte
		updatedAt time.Time
	)
	err := s.db.QueryRow(ctx, `SELECT settings, updated_at FROM panel_maintenance_settings WHERE id = TRUE`).Scan(&raw, &updatedAt)
	if err != nil {
		return DefaultMaintenanceSettings(), err
	}
	if len(raw) == 0 {
		return DefaultMaintenanceSettings(), nil
	}
	ms := DefaultMaintenanceSettings()
	_ = json.Unmarshal(raw, &ms)
	ms.UpdatedAt = updatedAt.Format(time.RFC3339)
	return ms, nil
}

func (s *Store) UpdateMaintenanceSettings(ctx context.Context, ms MaintenanceSettings) error {
	if s.db == nil {
		return errors.New("no database connection")
	}
	body, err := json.Marshal(ms)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO panel_maintenance_settings (id, settings, updated_at)
		VALUES (TRUE, $1::jsonb, now())
		ON CONFLICT (id) DO UPDATE SET
			settings = EXCLUDED.settings,
			updated_at = now()
	`, string(body))
	return err
}
