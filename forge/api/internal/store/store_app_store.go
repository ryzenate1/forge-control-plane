package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type AppStoreApp struct {
	ID             string    `json:"id"`
	Key            string    `json:"key"`
	Name           string    `json:"name"`
	ShortDesc      string    `json:"shortDesc"`
	Description    string    `json:"description"`
	Icon           string    `json:"icon"`
	Category       string    `json:"category"`
	Tags           []string  `json:"tags"`
	Version        string    `json:"version"`
	ComposeContent string    `json:"composeContent"`
	Params         []byte    `json:"params,omitempty"`
	MinMemoryMB    int       `json:"minMemoryMb"`
	MinDiskMB      int       `json:"minDiskMb"`
	Maintainer     string    `json:"maintainer"`
	SourceURL      string    `json:"sourceUrl"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type AppStoreInstall struct {
	ID               string    `json:"id"`
	AppKey           string    `json:"appKey"`
	AppVersion       string    `json:"appVersion"`
	ProjectID        string    `json:"projectId"`
	EnvironmentID    string    `json:"environmentId"`
	Name             string    `json:"name"`
	Status           string    `json:"status"`
	Params           []byte    `json:"params,omitempty"`
	ComposeContent   string    `json:"composeContent"`
	ComposeProjectID string    `json:"composeProjectId"`
	ErrorMessage     string    `json:"errorMessage"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

func (s *Store) ListAppStoreApps(ctx context.Context, category, search string) ([]AppStoreApp, error) {
	query := `SELECT id::text, key, name, COALESCE(short_desc, ''), COALESCE(description, ''), COALESCE(icon, ''), COALESCE(category, ''), COALESCE(tags, '{}'), version, compose_content, params, COALESCE(min_memory_mb, 0), COALESCE(min_disk_mb, 0), COALESCE(maintainer, ''), COALESCE(source_url, ''), created_at, updated_at FROM app_store_apps WHERE 1=1`
	args := []any{}
	argIdx := 1
	if category != "" {
		query += ` AND category = $` + fmt.Sprintf("%d", argIdx)
		args = append(args, category)
		argIdx++
	}
	if search != "" {
		query += ` AND (name ILIKE $` + fmt.Sprintf("%d", argIdx) + ` OR short_desc ILIKE $` + fmt.Sprintf("%d", argIdx) + `)`
		args = append(args, "%"+search+"%")
		argIdx++
	}
	query += ` ORDER BY name ASC`
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var apps []AppStoreApp
	for rows.Next() {
		var a AppStoreApp
		var tags []string
		if err := rows.Scan(&a.ID, &a.Key, &a.Name, &a.ShortDesc, &a.Description, &a.Icon, &a.Category, &tags, &a.Version, &a.ComposeContent, &a.Params, &a.MinMemoryMB, &a.MinDiskMB, &a.Maintainer, &a.SourceURL, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		a.Tags = tags
		apps = append(apps, a)
	}
	return apps, nil
}

func (s *Store) GetAppStoreApp(ctx context.Context, key string) (*AppStoreApp, error) {
	var a AppStoreApp
	var tags []string
	err := s.db.QueryRow(ctx, `SELECT id::text, key, name, COALESCE(short_desc, ''), COALESCE(description, ''), COALESCE(icon, ''), COALESCE(category, ''), COALESCE(tags, '{}'), version, compose_content, params, COALESCE(min_memory_mb, 0), COALESCE(min_disk_mb, 0), COALESCE(maintainer, ''), COALESCE(source_url, ''), created_at, updated_at FROM app_store_apps WHERE key = $1`, key).Scan(&a.ID, &a.Key, &a.Name, &a.ShortDesc, &a.Description, &a.Icon, &a.Category, &tags, &a.Version, &a.ComposeContent, &a.Params, &a.MinMemoryMB, &a.MinDiskMB, &a.Maintainer, &a.SourceURL, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	a.Tags = tags
	return &a, nil
}

func (s *Store) UpsertAppStoreApp(ctx context.Context, a *AppStoreApp) error {
	tagsJSON, _ := json.Marshal(a.Tags)
	_, err := s.db.Exec(ctx, `
		INSERT INTO app_store_apps (key, name, short_desc, description, icon, category, tags, version, compose_content, params, min_memory_mb, min_disk_mb, maintainer, source_url, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9,$10,$11,$12,$13,$14,NOW(),NOW())
		ON CONFLICT (key) DO UPDATE SET
			name=EXCLUDED.name, short_desc=EXCLUDED.short_desc, description=EXCLUDED.description,
			icon=EXCLUDED.icon, category=EXCLUDED.category, tags=EXCLUDED.tags,
			version=EXCLUDED.version, compose_content=EXCLUDED.compose_content,
			params=EXCLUDED.params, min_memory_mb=EXCLUDED.min_memory_mb,
			min_disk_mb=EXCLUDED.min_disk_mb, maintainer=EXCLUDED.maintainer,
			source_url=EXCLUDED.source_url, updated_at=NOW()
	`, a.Key, a.Name, a.ShortDesc, a.Description, a.Icon, a.Category, string(tagsJSON), a.Version, a.ComposeContent, a.Params, a.MinMemoryMB, a.MinDiskMB, a.Maintainer, a.SourceURL)
	return err
}

func (s *Store) DeleteAppStoreApp(ctx context.Context, key string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM app_store_apps WHERE key = $1`, key)
	return err
}

func (s *Store) CreateAppStoreInstall(ctx context.Context, inst *AppStoreInstall) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO app_store_installs (id, app_key, app_version, project_id, environment_id, name, status, params, compose_content, compose_project_id, error_message, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW(),NOW())
	`, inst.ID, inst.AppKey, inst.AppVersion, nullIfEmpty(inst.ProjectID), nullIfEmpty(inst.EnvironmentID), inst.Name, inst.Status, inst.Params, inst.ComposeContent, nullIfEmpty(inst.ComposeProjectID), nullIfEmpty(inst.ErrorMessage))
	return err
}

func (s *Store) GetAppStoreInstall(ctx context.Context, id string) (*AppStoreInstall, error) {
	var inst AppStoreInstall
	err := s.db.QueryRow(ctx, `
		SELECT id::text, app_key, app_version, COALESCE(project_id::text, ''), COALESCE(environment_id::text, ''), name, status, params, compose_content, COALESCE(compose_project_id::text, ''), COALESCE(error_message, ''), created_at, updated_at
		FROM app_store_installs WHERE id = $1
	`, id).Scan(&inst.ID, &inst.AppKey, &inst.AppVersion, &inst.ProjectID, &inst.EnvironmentID, &inst.Name, &inst.Status, &inst.Params, &inst.ComposeContent, &inst.ComposeProjectID, &inst.ErrorMessage, &inst.CreatedAt, &inst.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &inst, nil
}

func (s *Store) ListAppStoreInstalls(ctx context.Context) ([]AppStoreInstall, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, app_key, app_version, COALESCE(project_id::text, ''), COALESCE(environment_id::text, ''), name, status, params, compose_content, COALESCE(compose_project_id::text, ''), COALESCE(error_message, ''), created_at, updated_at
		FROM app_store_installs ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var insts []AppStoreInstall
	for rows.Next() {
		var inst AppStoreInstall
		if err := rows.Scan(&inst.ID, &inst.AppKey, &inst.AppVersion, &inst.ProjectID, &inst.EnvironmentID, &inst.Name, &inst.Status, &inst.Params, &inst.ComposeContent, &inst.ComposeProjectID, &inst.ErrorMessage, &inst.CreatedAt, &inst.UpdatedAt); err != nil {
			return nil, err
		}
		insts = append(insts, inst)
	}
	return insts, nil
}

func (s *Store) UpdateAppStoreInstallStatus(ctx context.Context, id, status, errMsg string) error {
	_, err := s.db.Exec(ctx, `UPDATE app_store_installs SET status = $2, error_message = $3, updated_at = NOW() WHERE id = $1`, id, status, errMsg)
	return err
}

func (s *Store) UpdateAppStoreInstallComposeProject(ctx context.Context, id, composeProjectID string) error {
	_, err := s.db.Exec(ctx, `UPDATE app_store_installs SET compose_project_id = $2, updated_at = NOW() WHERE id = $1`, id, composeProjectID)
	return err
}

func (s *Store) DeleteAppStoreInstall(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM app_store_installs WHERE id = $1`, id)
	return err
}
