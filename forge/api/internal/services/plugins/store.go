package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pgPluginStore struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) PluginStore {
	return &pgPluginStore{db: db}
}

func scanPlugin(scanner interface {
	Scan(dest ...any) error
}) (Plugin, error) {
	var p Plugin
	var manifestBytes, settingsBytes []byte
	var installedAt, updatedAt time.Time
	var state string
	var author, license, homepage, minVersion, maxVersion, errorMsg string
	var hooksJSON, depsJSON []byte

	err := scanner.Scan(
		&p.ID, &p.Name,
		&manifestBytes,
		&p.Source,
		&installedAt, &updatedAt,
		&state,
		&author, &license, &homepage,
		&minVersion, &maxVersion,
		&errorMsg,
		&hooksJSON, &depsJSON,
		&settingsBytes,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Plugin{}, ErrPluginNotFound
		}
		return Plugin{}, err
	}

	p.State = PluginState(state)
	p.InstalledAt = installedAt
	p.UpdatedAt = updatedAt
	p.Error = errorMsg

	// A stored manifest that cannot be decoded is corrupt data, not an empty
	// manifest. Surface it instead of handing back a plugin that looks installed
	// with no hooks, permissions or version.
	if err := json.Unmarshal(manifestBytes, &p.Manifest); err != nil {
		return Plugin{}, fmt.Errorf("plugin %s (%s) has an unreadable manifest: %w", p.ID, p.Name, err)
	}

	if p.Manifest.Author == "" {
		p.Manifest.Author = author
	}
	if p.Manifest.License == "" {
		p.Manifest.License = license
	}
	if p.Manifest.Homepage == "" {
		p.Manifest.Homepage = homepage
	}
	if p.Manifest.MinVersion == "" {
		p.Manifest.MinVersion = minVersion
	}
	if p.Manifest.MaxVersion == "" {
		p.Manifest.MaxVersion = maxVersion
	}
	if p.Manifest.Hooks == nil && len(hooksJSON) > 0 {
		if err := json.Unmarshal(hooksJSON, &p.Manifest.Hooks); err != nil {
			return Plugin{}, fmt.Errorf("plugin %s (%s) has unreadable hooks: %w", p.ID, p.Name, err)
		}
	}
	if p.Manifest.Dependencies == nil && len(depsJSON) > 0 {
		if err := json.Unmarshal(depsJSON, &p.Manifest.Dependencies); err != nil {
			return Plugin{}, fmt.Errorf("plugin %s (%s) has unreadable dependencies: %w", p.ID, p.Name, err)
		}
	}

	if len(settingsBytes) > 0 {
		p.Settings = settingsBytes
	}
	if len(p.Settings) == 0 || string(p.Settings) == "null" {
		var raw struct {
			Settings json.RawMessage `json:"settings"`
		}
		if err := json.Unmarshal(manifestBytes, &raw); err == nil && len(raw.Settings) > 0 {
			p.Settings = raw.Settings
		}
	}

	return p, nil
}

func (s *pgPluginStore) ListPlugins(ctx context.Context) ([]Plugin, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, manifest, COALESCE(source,''), created_at, updated_at,
		       COALESCE(state,'installed'),
		       COALESCE(author,''), COALESCE(license,''), COALESCE(homepage,''),
		       COALESCE(min_version,''), COALESCE(max_version,''),
		       COALESCE(error_message,''),
		       COALESCE(hooks,'{}'::jsonb), COALESCE(dependencies,'{}'::jsonb),
		       COALESCE(settings,'null'::jsonb)
		FROM plugins ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Plugin
	for rows.Next() {
		p, err := scanPlugin(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *pgPluginStore) GetPlugin(ctx context.Context, id string) (*Plugin, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id::text, name, manifest, COALESCE(source,''), created_at, updated_at,
		       COALESCE(state,'installed'),
		       COALESCE(author,''), COALESCE(license,''), COALESCE(homepage,''),
		       COALESCE(min_version,''), COALESCE(max_version,''),
		       COALESCE(error_message,''),
		       COALESCE(hooks,'{}'::jsonb), COALESCE(dependencies,'{}'::jsonb),
		       COALESCE(settings,'null'::jsonb)
		FROM plugins WHERE id = $1
	`, id)

	p, err := scanPlugin(row)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *pgPluginStore) CreatePlugin(ctx context.Context, plugin *Plugin) error {
	manifestBytes, _ := json.Marshal(plugin.Manifest)
	settingsBytes := plugin.Settings
	if len(settingsBytes) == 0 {
		settingsBytes = []byte("null")
	}
	hooksBytes, _ := json.Marshal(plugin.Manifest.Hooks)
	depsBytes, _ := json.Marshal(plugin.Manifest.Dependencies)

	_, err := s.db.Exec(ctx, `
		INSERT INTO plugins (id, name, manifest, source, created_at, updated_at, state,
		                     author, license, homepage, min_version, max_version, error_message,
		                     hooks, dependencies, settings)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`, plugin.ID, plugin.Name, manifestBytes, plugin.Source,
		plugin.InstalledAt, plugin.UpdatedAt, string(plugin.State),
		plugin.Manifest.Author, plugin.Manifest.License, plugin.Manifest.Homepage,
		plugin.Manifest.MinVersion, plugin.Manifest.MaxVersion, plugin.Error,
		hooksBytes, depsBytes, settingsBytes,
	)
	return err
}

func (s *pgPluginStore) UpdatePluginState(ctx context.Context, id string, state PluginState, errorMsg string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE plugins SET state = $1, error_message = $2, updated_at = now()
		WHERE id = $3
	`, string(state), errorMsg, id)
	return err
}

func (s *pgPluginStore) UpdatePluginSettings(ctx context.Context, id string, settings json.RawMessage) error {
	_, err := s.db.Exec(ctx, `
		UPDATE plugins SET settings = $1, updated_at = now()
		WHERE id = $2
	`, settings, id)
	return err
}

func (s *pgPluginStore) DeletePlugin(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM plugins WHERE id = $1`, id)
	return err
}

func (s *pgPluginStore) FindPluginByName(ctx context.Context, name string) (*Plugin, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id::text, name, manifest, COALESCE(source,''), created_at, updated_at,
		       COALESCE(state,'installed'),
		       COALESCE(author,''), COALESCE(license,''), COALESCE(homepage,''),
		       COALESCE(min_version,''), COALESCE(max_version,''),
		       COALESCE(error_message,''),
		       COALESCE(hooks,'{}'::jsonb), COALESCE(dependencies,'{}'::jsonb),
		       COALESCE(settings,'null'::jsonb)
		FROM plugins WHERE name = $1
	`, name)

	p, err := scanPlugin(row)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
