package store

import (
	"context"
	"time"
)

type Buildpack struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	BuilderType string    `json:"builderType"`
	CreatedAt   time.Time `json:"createdAt"`
}

type ServerBuildpack struct {
	ID          string     `json:"id"`
	ServerID    string     `json:"serverId"`
	BuildpackID string     `json:"buildpackId"`
	Priority    int        `json:"priority"`
	Buildpack   *Buildpack `json:"buildpack,omitempty"`
}

type AppBuild struct {
	ID          string     `json:"id"`
	ServerID    string     `json:"serverId"`
	BuildpackID *string    `json:"buildpackId,omitempty"`
	Status      string     `json:"status"`
	BuildLog    string     `json:"buildLog"`
	ImageTag    string     `json:"imageTag"`
	CreatedAt   time.Time  `json:"createdAt"`
	Buildpack   *Buildpack `json:"buildpack,omitempty"`
}

type CreateBuildpackRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	URL         string `json:"url"`
	BuilderType string `json:"builderType"`
}

type CreateAppBuildRequest struct {
	BuildpackID *string `json:"buildpackId"`
}

func (s *Store) CreateBuildpack(ctx context.Context, req CreateBuildpackRequest) (*Buildpack, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO buildpacks (name, description, url, builder_type)
		VALUES ($1, $2, $3, $4)
		RETURNING id, name, description, url, builder_type, created_at
	`, req.Name, req.Description, req.URL, req.BuilderType)
	var bp Buildpack
	err := row.Scan(&bp.ID, &bp.Name, &bp.Description, &bp.URL, &bp.BuilderType, &bp.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &bp, nil
}

func (s *Store) ListBuildpacks(ctx context.Context) ([]Buildpack, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, description, url, builder_type, created_at
		FROM buildpacks ORDER BY name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Buildpack
	for rows.Next() {
		var bp Buildpack
		if err := rows.Scan(&bp.ID, &bp.Name, &bp.Description, &bp.URL, &bp.BuilderType, &bp.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, bp)
	}
	return out, rows.Err()
}

func (s *Store) GetBuildpack(ctx context.Context, id string) (*Buildpack, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, name, description, url, builder_type, created_at
		FROM buildpacks WHERE id = $1
	`, id)
	var bp Buildpack
	err := row.Scan(&bp.ID, &bp.Name, &bp.Description, &bp.URL, &bp.BuilderType, &bp.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &bp, nil
}

func (s *Store) DeleteBuildpack(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM buildpacks WHERE id = $1`, id)
	return err
}

func (s *Store) AssignBuildpackToServer(ctx context.Context, serverID, buildpackID string, priority int) (*ServerBuildpack, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO server_buildpacks (server_id, buildpack_id, priority)
		VALUES ($1, $2, $3)
		RETURNING id, server_id, buildpack_id, priority
	`, serverID, buildpackID, priority)
	var sb ServerBuildpack
	err := row.Scan(&sb.ID, &sb.ServerID, &sb.BuildpackID, &sb.Priority)
	if err != nil {
		return nil, err
	}
	return &sb, nil
}

func (s *Store) ListServerBuildpacks(ctx context.Context, serverID string) ([]ServerBuildpack, error) {
	rows, err := s.db.Query(ctx, `
		SELECT sb.id, sb.server_id, sb.buildpack_id, sb.priority,
			bp.id, bp.name, bp.description, bp.url, bp.builder_type, bp.created_at
		FROM server_buildpacks sb
		JOIN buildpacks bp ON bp.id = sb.buildpack_id
		WHERE sb.server_id = $1
		ORDER BY sb.priority ASC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ServerBuildpack
	for rows.Next() {
		var sb ServerBuildpack
		var bp Buildpack
		if err := rows.Scan(&sb.ID, &sb.ServerID, &sb.BuildpackID, &sb.Priority,
			&bp.ID, &bp.Name, &bp.Description, &bp.URL, &bp.BuilderType, &bp.CreatedAt); err != nil {
			return nil, err
		}
		sb.Buildpack = &bp
		out = append(out, sb)
	}
	return out, rows.Err()
}

func (s *Store) RemoveServerBuildpack(ctx context.Context, serverID, buildpackID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM server_buildpacks WHERE server_id = $1 AND buildpack_id = $2`, serverID, buildpackID)
	return err
}

func (s *Store) CreateAppBuild(ctx context.Context, serverID string, req CreateAppBuildRequest) (*AppBuild, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO app_builds (server_id, buildpack_id, status)
		VALUES ($1, $2, 'pending')
		RETURNING id, server_id, buildpack_id, status, build_log, image_tag, created_at
	`, serverID, req.BuildpackID)
	var b AppBuild
	err := row.Scan(&b.ID, &b.ServerID, &b.BuildpackID, &b.Status, &b.BuildLog, &b.ImageTag, &b.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Store) ListAppBuilds(ctx context.Context, serverID string) ([]AppBuild, error) {
	rows, err := s.db.Query(ctx, `
		SELECT ab.id, ab.server_id, ab.buildpack_id, ab.status, ab.build_log, ab.image_tag, ab.created_at,
			bp.id, bp.name, bp.description, bp.url, bp.builder_type, bp.created_at
		FROM app_builds ab
		LEFT JOIN buildpacks bp ON bp.id = ab.buildpack_id
		WHERE ab.server_id = $1
		ORDER BY ab.created_at DESC
		LIMIT 50
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AppBuild
	for rows.Next() {
		var b AppBuild
		var bpID, bpName, bpDesc, bpURL, bpType *string
		var bpCreatedAt *time.Time
		if err := rows.Scan(&b.ID, &b.ServerID, &b.BuildpackID, &b.Status, &b.BuildLog, &b.ImageTag, &b.CreatedAt,
			&bpID, &bpName, &bpDesc, &bpURL, &bpType, &bpCreatedAt); err != nil {
			return nil, err
		}
		if bpID != nil {
			b.Buildpack = &Buildpack{
				ID:          *bpID,
				Name:        *bpName,
				Description: *bpDesc,
				URL:         *bpURL,
				BuilderType: *bpType,
				CreatedAt:   *bpCreatedAt,
			}
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) GetAppBuild(ctx context.Context, id string) (*AppBuild, error) {
	row := s.db.QueryRow(ctx, `
		SELECT ab.id, ab.server_id, ab.buildpack_id, ab.status, ab.build_log, ab.image_tag, ab.created_at,
			bp.id, bp.name, bp.description, bp.url, bp.builder_type, bp.created_at
		FROM app_builds ab
		LEFT JOIN buildpacks bp ON bp.id = ab.buildpack_id
		WHERE ab.id = $1
	`, id)
	var b AppBuild
	var bpID, bpName, bpDesc, bpURL, bpType *string
	var bpCreatedAt *time.Time
	err := row.Scan(&b.ID, &b.ServerID, &b.BuildpackID, &b.Status, &b.BuildLog, &b.ImageTag, &b.CreatedAt,
		&bpID, &bpName, &bpDesc, &bpURL, &bpType, &bpCreatedAt)
	if err != nil {
		return nil, err
	}
	if bpID != nil {
		b.Buildpack = &Buildpack{
			ID:          *bpID,
			Name:        *bpName,
			Description: *bpDesc,
			URL:         *bpURL,
			BuilderType: *bpType,
			CreatedAt:   *bpCreatedAt,
		}
	}
	return &b, nil
}

func (s *Store) UpdateAppBuildStatus(ctx context.Context, id, status, buildLog, imageTag string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE app_builds SET status = $1, build_log = $2, image_tag = $3
		WHERE id = $4
	`, status, buildLog, imageTag, id)
	return err
}
