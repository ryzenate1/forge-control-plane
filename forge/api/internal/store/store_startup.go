package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) GetServerStartup(ctx context.Context, serverID string) (StartupDetails, error) {
	var details StartupDetails
	var image string
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(NULLIF(s.startup_command, ''), e.startup),
		       COALESCE(NULLIF(s.docker_image, ''), (SELECT value FROM jsonb_each_text(e.docker_images) ORDER BY key LIMIT 1), '')
		FROM servers s
		JOIN eggs e ON e.id = s.egg_id
		WHERE s.id = $1
	`, serverID).Scan(&details.RawStartupCommand, &image)
	if err != nil {
		return StartupDetails{}, err
	}
	details.DockerImages = map[string]string{image: image}
	rows, err := s.db.Query(ctx, `
		SELECT ev.name, ev.description, ev.env_variable, ev.default_value,
		       COALESCE(sv.variable_value, ev.default_value), ev.user_editable, ev.rules
		FROM servers s
		JOIN egg_variables ev ON ev.egg_id = s.egg_id
		LEFT JOIN server_variables sv ON sv.server_id = s.id AND sv.variable_id = ev.id
		WHERE s.id = $1 AND ev.user_viewable = true
		ORDER BY ev.name
	`, serverID)
	if err != nil {
		return StartupDetails{}, err
	}
	defer rows.Close()
	env := map[string]string{}
	for rows.Next() {
		var variable StartupVariable
		if err := rows.Scan(&variable.Name, &variable.Description, &variable.EnvVariable, &variable.DefaultValue, &variable.ServerValue, &variable.IsEditable, &variable.Rules); err != nil {
			return StartupDetails{}, err
		}
		env[variable.EnvVariable] = variable.ServerValue
		details.Variables = append(details.Variables, variable)
	}
	if err := rows.Err(); err != nil {
		return StartupDetails{}, err
	}
	details.StartupCommand = resolveStartupCommand(details.RawStartupCommand, env)
	return details, nil
}

func (s *Store) UpdateServerStartupVariable(ctx context.Context, serverID, key, value string, actorID *string) (StartupDetails, error) {
	var variableID string
	var editable bool
	var rules string
	err := s.db.QueryRow(ctx, `
		SELECT ev.id::text, ev.user_editable, ev.rules
		FROM servers s
		JOIN egg_variables ev ON ev.egg_id = s.egg_id
		WHERE s.id = $1 AND ev.env_variable = $2 AND ev.user_viewable = true
	`, serverID, key).Scan(&variableID, &editable, &rules)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return StartupDetails{}, errors.New("startup variable not found")
		}
		return StartupDetails{}, err
	}
	if !editable {
		return StartupDetails{}, errors.New("startup variable is read-only")
	}
	if err := validateVariableValue(value, rules); err != nil {
		return StartupDetails{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return StartupDetails{}, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `
		INSERT INTO server_variables (server_id, variable_id, variable_value, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (server_id, variable_id)
		DO UPDATE SET variable_value = EXCLUDED.variable_value, updated_at = now()
	`, serverID, variableID, value)
	if err != nil {
		return StartupDetails{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE servers SET config_sync_pending = true, config_sync_error = NULL, updated_at = now() WHERE id = $1`, serverID); err != nil {
		return StartupDetails{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events (id, actor_id, action, target_type, target_id, metadata) VALUES ($4, $1, 'server startup variable updated', 'server', $2, $3::jsonb)`, actorID, serverID, fmt.Sprintf(`{"variable":"%s"}`, key), uuid.NewString()); err != nil {
		return StartupDetails{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return StartupDetails{}, err
	}
	return s.GetServerStartup(ctx, serverID)
}
