package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ---------- Location types ----------

type Location struct {
	ID          string    `json:"id"`
	Short       string    `json:"short"`
	Long        string    `json:"long"`
	NodeCount   int       `json:"nodeCount"`
	ServerCount int       `json:"serverCount"`
	CreatedAt   time.Time `json:"createdAt"`
}

type CreateLocationRequest struct {
	Short string
	Long  string
}

type UpdateLocationRequest struct {
	Short string
	Long  string
}

// ---------- Location CRUD ----------

func (s *Store) ListLocations(ctx context.Context) ([]Location, error) {
	rows, err := s.db.Query(ctx, `
		SELECT l.id::text, l.short, l.long, l.created_at,
		       (SELECT COUNT(*)::int FROM nodes n WHERE n.location_id = l.id),
		       (SELECT COUNT(*)::int FROM servers s JOIN nodes n ON n.id = s.node_id WHERE n.location_id = l.id)
		FROM locations l
		ORDER BY l.short
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	locations := []Location{}
	for rows.Next() {
		var loc Location
		if err := rows.Scan(&loc.ID, &loc.Short, &loc.Long, &loc.CreatedAt, &loc.NodeCount, &loc.ServerCount); err != nil {
			return nil, err
		}
		locations = append(locations, loc)
	}
	return locations, rows.Err()
}

func (s *Store) GetLocation(ctx context.Context, id string) (Location, error) {
	var loc Location
	err := s.db.QueryRow(ctx, `
		SELECT l.id::text, l.short, l.long, l.created_at,
		       (SELECT COUNT(*)::int FROM nodes n WHERE n.location_id = l.id),
		       (SELECT COUNT(*)::int FROM servers s JOIN nodes n ON n.id = s.node_id WHERE n.location_id = l.id)
		FROM locations l
		WHERE l.id = $1
	`, id).Scan(&loc.ID, &loc.Short, &loc.Long, &loc.CreatedAt, &loc.NodeCount, &loc.ServerCount)
	if err != nil {
		return Location{}, fmt.Errorf("location not found: %w", err)
	}
	return loc, nil
}

func (s *Store) CreateLocation(ctx context.Context, req CreateLocationRequest, actorID *string) (Location, error) {
	short := strings.TrimSpace(req.Short)
	long := strings.TrimSpace(req.Long)
	if short == "" || long == "" {
		return Location{}, errors.New("short and long name are required")
	}
	id := uuid.NewString()
	_, err := s.db.Exec(ctx, `
		INSERT INTO locations (id, short, long)
		VALUES ($1, $2, $3)
	`, id, short, long)
	if err != nil {
		return Location{}, fmt.Errorf("create location: %w", err)
	}
	_ = s.AppendAudit(ctx, actorID, "location created", "location", &id, fmt.Sprintf(`{"short":"%s","long":"%s"}`, short, long))
	return s.GetLocation(ctx, id)
}

func (s *Store) UpdateLocation(ctx context.Context, id string, req UpdateLocationRequest, actorID *string) (Location, error) {
	short := strings.TrimSpace(req.Short)
	long := strings.TrimSpace(req.Long)
	if short == "" || long == "" {
		return Location{}, errors.New("short and long name are required")
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE locations SET short = $1, long = $2 WHERE id = $3
	`, short, long, id)
	if err != nil {
		return Location{}, fmt.Errorf("update location: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Location{}, errors.New("location not found")
	}
	_ = s.AppendAudit(ctx, actorID, "location updated", "location", &id, fmt.Sprintf(`{"short":"%s","long":"%s"}`, short, long))
	return s.GetLocation(ctx, id)
}

func (s *Store) DeleteLocation(ctx context.Context, id string, actorID *string) error {
	// Prevent deletion if nodes still reference this location.
	var nodeCount int
	_ = s.db.QueryRow(ctx, `SELECT COUNT(*) FROM nodes WHERE location_id = $1`, id).Scan(&nodeCount)
	if nodeCount > 0 {
		return fmt.Errorf("cannot delete location with %d attached node(s)", nodeCount)
	}
	tag, err := s.db.Exec(ctx, `DELETE FROM locations WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("location not found")
	}
	return s.AppendAudit(ctx, actorID, "location deleted", "location", &id, `{"reason":"admin delete"}`)
}
