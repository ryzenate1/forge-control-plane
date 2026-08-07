package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gamepanel/forge/internal/services/tags"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// This file implements tags.Repository on *Store. The domain types and all
// validation live in the tags service package; here we only translate between
// those types and rows. resource_type is never interpolated into SQL — every
// branch uses a fixed literal per kind, so the switch below cannot be injected.

// isTagUniqueViolation reports whether err is a Postgres 23505 constraint error.
// (Named distinctly from the loose string-matching helper in store_envvars.go.)
func isTagUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func (s *Store) CreateTag(ctx context.Context, name, color, description string) (tags.Tag, error) {
	id := uuid.NewString()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO tags (id, name, color, description)
		VALUES ($1, $2, $3, $4)
	`, id, name, color, description); err != nil {
		if isTagUniqueViolation(err) {
			return tags.Tag{}, tags.ErrDuplicateName
		}
		return tags.Tag{}, err
	}
	return s.GetTag(ctx, id)
}

func (s *Store) UpdateTag(ctx context.Context, id string, req tags.UpdateRequest) (tags.Tag, error) {
	setClauses := []string{}
	args := []any{}
	argIdx := 1
	if req.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if req.Color != nil {
		setClauses = append(setClauses, fmt.Sprintf("color = $%d", argIdx))
		args = append(args, *req.Color)
		argIdx++
	}
	if req.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *req.Description)
		argIdx++
	}
	if len(setClauses) == 0 {
		return s.GetTag(ctx, id)
	}
	args = append(args, id)
	tag, err := s.db.Exec(ctx, fmt.Sprintf("UPDATE tags SET %s WHERE id = $%d", strings.Join(setClauses, ", "), argIdx), args...)
	if err != nil {
		if isTagUniqueViolation(err) {
			return tags.Tag{}, tags.ErrDuplicateName
		}
		return tags.Tag{}, err
	}
	if tag.RowsAffected() == 0 {
		return tags.Tag{}, tags.ErrNotFound
	}
	return s.GetTag(ctx, id)
}

func (s *Store) DeleteTag(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM tags WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return tags.ErrNotFound
	}
	return nil
}

func scanTag(row interface{ Scan(dest ...any) error }) (tags.Tag, error) {
	var tag tags.Tag
	if err := row.Scan(&tag.ID, &tag.Name, &tag.Color, &tag.Description, &tag.CreatedAt); err != nil {
		return tags.Tag{}, err
	}
	return tag, nil
}

func (s *Store) GetTag(ctx context.Context, id string) (tags.Tag, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id::text, name, color, description, created_at
		FROM tags
		WHERE id = $1
	`, id)
	tag, err := scanTag(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return tags.Tag{}, tags.ErrNotFound
	}
	return tag, err
}

func (s *Store) ListTags(ctx context.Context) ([]tags.Tag, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, name, color, description, created_at
		FROM tags
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []tags.Tag{}
	for rows.Next() {
		tag, err := scanTag(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, tag)
	}
	return out, rows.Err()
}

func (s *Store) TagExists(ctx context.Context, tagID string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tags WHERE id = $1)`, tagID).Scan(&exists)
	return exists, err
}

// resourceExistsSQL maps each supported resource kind to a fixed existence
// query. Unknown kinds fail closed.
func resourceExistsSQL(resourceType string) (string, error) {
	switch resourceType {
	case string(tags.ResourceApplication):
		return `SELECT EXISTS(SELECT 1 FROM applications WHERE id = $1)`, nil
	case string(tags.ResourceServer):
		return `SELECT EXISTS(SELECT 1 FROM servers WHERE id = $1)`, nil
	case string(tags.ResourceEnvironment):
		return `SELECT EXISTS(SELECT 1 FROM environments WHERE id = $1)`, nil
	default:
		return "", tags.ErrInvalidResourceType
	}
}

func (s *Store) ResourceExists(ctx context.Context, resourceType, resourceID string) (bool, error) {
	query, err := resourceExistsSQL(resourceType)
	if err != nil {
		return false, err
	}
	var exists bool
	if err := s.db.QueryRow(ctx, query, resourceID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (s *Store) AssignTag(ctx context.Context, resourceType, resourceID, tagID string) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO resource_tags (id, tag_id, resource_type, resource_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (tag_id, resource_type, resource_id) DO NOTHING
	`, uuid.NewString(), tagID, resourceType, resourceID)
	return err
}

func (s *Store) UnassignTag(ctx context.Context, resourceType, resourceID, tagID string) error {
	tag, err := s.db.Exec(ctx, `
		DELETE FROM resource_tags
		WHERE tag_id = $1 AND resource_type = $2 AND resource_id = $3
	`, tagID, resourceType, resourceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("tag assignment not found")
	}
	return nil
}

func (s *Store) ListTagsForResource(ctx context.Context, resourceType, resourceID string) ([]tags.Tag, error) {
	rows, err := s.db.Query(ctx, `
		SELECT t.id::text, t.name, t.color, t.description, t.created_at
		FROM tags t
		JOIN resource_tags rt ON rt.tag_id = t.id
		WHERE rt.resource_type = $1 AND rt.resource_id = $2
		ORDER BY t.name
	`, resourceType, resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []tags.Tag{}
	for rows.Next() {
		tag, err := scanTag(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, tag)
	}
	return out, rows.Err()
}

// ListResourcesByTag returns the mixed set of resources carrying a tag. The
// UNION walks each kind's own table so callers get display names alongside the
// (type, id) pair needed for dispatch.
func (s *Store) ListResourcesByTag(ctx context.Context, tagID string) ([]tags.TaggedResource, error) {
	rows, err := s.db.Query(ctx, `
		SELECT 'application'::varchar, rt.resource_id::text, a.name
		FROM resource_tags rt
		JOIN applications a ON a.id = rt.resource_id
		WHERE rt.tag_id = $1 AND rt.resource_type = 'application'
		UNION ALL
		SELECT 'server'::varchar, rt.resource_id::text, s.name
		FROM resource_tags rt
		JOIN servers s ON s.id = rt.resource_id
		WHERE rt.tag_id = $1 AND rt.resource_type = 'server'
		UNION ALL
		SELECT 'environment'::varchar, rt.resource_id::text, e.name
		FROM resource_tags rt
		JOIN environments e ON e.id = rt.resource_id
		WHERE rt.tag_id = $1 AND rt.resource_type = 'environment'
		ORDER BY 2, 1
	`, tagID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []tags.TaggedResource{}
	for rows.Next() {
		var resource tags.TaggedResource
		if err := rows.Scan(&resource.ResourceType, &resource.ResourceID, &resource.Name); err != nil {
			return nil, err
		}
		out = append(out, resource)
	}
	return out, rows.Err()
}

// compile-time guarantee that *Store satisfies the service's repository port.
var _ tags.Repository = (*Store)(nil)
