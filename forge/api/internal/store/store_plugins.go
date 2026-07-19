package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Plugin struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Kind        string          `json:"kind"`
	Version     string          `json:"version"`
	Manifest    json.RawMessage `json:"manifest"`
	InstallPath string          `json:"installPath"`
	Installed   bool            `json:"installed"`
	Enabled     bool            `json:"enabled"`
	Source      string          `json:"source"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

type CreatePluginRequest struct {
	Name        string
	Description string
	Kind        string
	Version     string
	Manifest    json.RawMessage
	InstallPath string
	Source      string
}

type UpdatePluginRequest struct {
	Name        *string
	Description *string
	Kind        *string
	Version     *string
	Manifest    *json.RawMessage
	InstallPath *string
	Source      *string
}

func (s *Store) CreatePlugin(ctx context.Context, req CreatePluginRequest) (Plugin, error) {
	if req.Name == "" {
		return Plugin{}, errors.New("name is required")
	}
	if req.Kind == "" {
		req.Kind = "integration"
	}
	if req.Version == "" {
		req.Version = "0.0.0"
	}
	if len(req.Manifest) == 0 {
		req.Manifest = json.RawMessage("{}")
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO plugins (id, name, description, kind, version, manifest, install_path, installed, enabled, source, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, false, false, $8, $9, $10)
	`, id, req.Name, req.Description, req.Kind, req.Version, req.Manifest, req.InstallPath, req.Source, now, now); err != nil {
		return Plugin{}, err
	}
	return Plugin{
		ID: id, Name: req.Name, Description: req.Description, Kind: req.Kind, Version: req.Version,
		Manifest: req.Manifest, InstallPath: req.InstallPath, Installed: false, Enabled: false,
		Source: req.Source, CreatedAt: now, UpdatedAt: now,
	}, nil
}

func (s *Store) ListPlugins(ctx context.Context) ([]Plugin, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, COALESCE(description,''), COALESCE(kind,'integration'), COALESCE(version,'0.0.0'),
		       COALESCE(manifest,'{}'::jsonb), COALESCE(install_path,''), installed, enabled, COALESCE(source,''),
		       created_at, updated_at
		FROM plugins ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Plugin{}
	for rows.Next() {
		var p Plugin
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.Kind, &p.Version, &p.Manifest, &p.InstallPath, &p.Installed, &p.Enabled, &p.Source, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetPlugin(ctx context.Context, id string) (Plugin, error) {
	var p Plugin
	err := s.db.QueryRow(ctx, `
		SELECT id::text, name, COALESCE(description,''), COALESCE(kind,'integration'), COALESCE(version,'0.0.0'),
		       COALESCE(manifest,'{}'::jsonb), COALESCE(install_path,''), installed, enabled, COALESCE(source,''),
		       created_at, updated_at
		FROM plugins WHERE id = $1
	`, id).Scan(&p.ID, &p.Name, &p.Description, &p.Kind, &p.Version, &p.Manifest, &p.InstallPath, &p.Installed, &p.Enabled, &p.Source, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return Plugin{}, errors.New("plugin not found")
	}
	return p, nil
}

func (s *Store) UpdatePluginState(ctx context.Context, id string, installed, enabled *bool) (Plugin, error) {
	updates := []string{}
	args := []any{}
	if installed != nil {
		updates = append(updates, "installed = $"+itoa(len(args)+1))
		args = append(args, *installed)
	}
	if enabled != nil {
		updates = append(updates, "enabled = $"+itoa(len(args)+1))
		args = append(args, *enabled)
	}
	if len(updates) == 0 {
		return s.GetPlugin(ctx, id)
	}
	updates = append(updates, "updated_at = now()")
	args = append(args, id)
	q := "UPDATE plugins SET " + updates[0]
	for i := 1; i < len(updates); i++ {
		q += ", " + updates[i]
	}
	q += " WHERE id = $" + itoa(len(args))
	if _, err := s.db.Exec(ctx, q, args...); err != nil {
		return Plugin{}, err
	}
	return s.GetPlugin(ctx, id)
}

func (s *Store) UpdatePlugin(ctx context.Context, id string, req UpdatePluginRequest) (Plugin, error) {
	updates := []string{}
	args := []any{}
	if req.Name != nil {
		updates = append(updates, "name = $"+itoa(len(args)+1))
		args = append(args, *req.Name)
	}
	if req.Description != nil {
		updates = append(updates, "description = $"+itoa(len(args)+1))
		args = append(args, *req.Description)
	}
	if req.Kind != nil {
		updates = append(updates, "kind = $"+itoa(len(args)+1))
		args = append(args, *req.Kind)
	}
	if req.Version != nil {
		updates = append(updates, "version = $"+itoa(len(args)+1))
		args = append(args, *req.Version)
	}
	if req.Manifest != nil {
		updates = append(updates, "manifest = $"+itoa(len(args)+1))
		args = append(args, *req.Manifest)
	}
	if req.InstallPath != nil {
		updates = append(updates, "install_path = $"+itoa(len(args)+1))
		args = append(args, *req.InstallPath)
	}
	if req.Source != nil {
		updates = append(updates, "source = $"+itoa(len(args)+1))
		args = append(args, *req.Source)
	}
	if len(updates) == 0 {
		return s.GetPlugin(ctx, id)
	}
	updates = append(updates, "updated_at = now()")
	args = append(args, id)
	q := "UPDATE plugins SET " + updates[0]
	for i := 1; i < len(updates); i++ {
		q += ", " + updates[i]
	}
	q += " WHERE id = $" + itoa(len(args))
	if _, err := s.db.Exec(ctx, q, args...); err != nil {
		return Plugin{}, err
	}
	return s.GetPlugin(ctx, id)
}

func (s *Store) DeletePlugin(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM plugins WHERE id = $1`, id)
	return err
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	negative := false
	if i < 0 {
		negative = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if negative {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
