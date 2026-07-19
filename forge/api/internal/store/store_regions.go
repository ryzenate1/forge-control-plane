package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Region struct {
	ID          string    `json:"id"`
	UUID        string    `json:"uuid"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	NodeCount   int       `json:"nodeCount"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type CreateRegionRequest struct {
	Name        string
	Slug        string
	Description string
	Enabled     bool
}

type UpdateRegionRequest struct {
	Name        string
	Slug        string
	Description string
	Enabled     bool
}

func (s *Store) ListRegions(ctx context.Context) ([]Region, error) {
	rows, err := s.db.Query(ctx, `
		SELECT r.id::text, COALESCE(r.uuid, r.id)::text, r.name, r.slug,
		       COALESCE(r.description, ''), r.enabled, r.created_at, r.updated_at,
		       (SELECT COUNT(*)::int FROM nodes n WHERE n.region_id = r.id)
		FROM regions r
		ORDER BY r.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	regions := []Region{}
	for rows.Next() {
		var region Region
		if err := rows.Scan(
			&region.ID,
			&region.UUID,
			&region.Name,
			&region.Slug,
			&region.Description,
			&region.Enabled,
			&region.CreatedAt,
			&region.UpdatedAt,
			&region.NodeCount,
		); err != nil {
			return nil, err
		}
		regions = append(regions, region)
	}
	return regions, rows.Err()
}

func (s *Store) GetRegion(ctx context.Context, id string) (Region, error) {
	var region Region
	err := s.db.QueryRow(ctx, `
		SELECT r.id::text, COALESCE(r.uuid, r.id)::text, r.name, r.slug,
		       COALESCE(r.description, ''), r.enabled, r.created_at, r.updated_at,
		       (SELECT COUNT(*)::int FROM nodes n WHERE n.region_id = r.id)
		FROM regions r
		WHERE r.id = $1
	`, id).Scan(
		&region.ID,
		&region.UUID,
		&region.Name,
		&region.Slug,
		&region.Description,
		&region.Enabled,
		&region.CreatedAt,
		&region.UpdatedAt,
		&region.NodeCount,
	)
	if err != nil {
		return Region{}, err
	}
	return region, nil
}

func (s *Store) CreateRegion(ctx context.Context, req CreateRegionRequest, actorID *string) (Region, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Slug = normalizeRegionSlug(req.Slug, req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if req.Name == "" || req.Slug == "" {
		return Region{}, errors.New("name and slug are required")
	}
	if !isValidRegionSlug(req.Slug) {
		return Region{}, errors.New("slug must contain lowercase letters, numbers, and single hyphens only")
	}
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO regions (id, uuid, name, slug, description, enabled)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, id, uuid.NewString(), req.Name, req.Slug, req.Description, req.Enabled)
	if err != nil {
		return Region{}, err
	}
	_ = s.AppendAudit(ctx, actorID, "region created", "region", &id, fmt.Sprintf(`{"slug":"%s"}`, req.Slug))
	return s.GetRegion(ctx, id)
}

func (s *Store) UpdateRegion(ctx context.Context, id string, req UpdateRegionRequest, actorID *string) (Region, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Slug = normalizeRegionSlug(req.Slug, req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if req.Name == "" || req.Slug == "" {
		return Region{}, errors.New("name and slug are required")
	}
	if !isValidRegionSlug(req.Slug) {
		return Region{}, errors.New("slug must contain lowercase letters, numbers, and single hyphens only")
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE regions
		SET name = $1, slug = $2, description = $3, enabled = $4, updated_at = now()
		WHERE id = $5
	`, req.Name, req.Slug, req.Description, req.Enabled, id)
	if err != nil {
		return Region{}, err
	}
	if tag.RowsAffected() == 0 {
		return Region{}, errors.New("region not found")
	}
	_ = s.AppendAudit(ctx, actorID, "region updated", "region", &id, fmt.Sprintf(`{"slug":"%s"}`, req.Slug))
	return s.GetRegion(ctx, id)
}

func (s *Store) DeleteRegion(ctx context.Context, id string, actorID *string) error {
	var nodeCount int
	_ = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM nodes WHERE region_id = $1`, id).Scan(&nodeCount)
	if nodeCount > 0 {
		return fmt.Errorf("cannot delete region with %d attached node(s)", nodeCount)
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM regions WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("region not found")
	}
	return s.AppendAudit(ctx, actorID, "region deleted", "region", &id, `{"reason":"admin delete"}`)
}

var regionSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var invalidRegionSlugCharacters = regexp.MustCompile(`[^a-z0-9]+`)

func normalizeRegionSlug(slug, name string) string {
	value := strings.TrimSpace(slug)
	if value == "" {
		value = strings.TrimSpace(name)
	}
	value = strings.ToLower(value)
	value = invalidRegionSlugCharacters.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

func isValidRegionSlug(slug string) bool {
	return regionSlugPattern.MatchString(slug)
}
