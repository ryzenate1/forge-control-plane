package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Role struct {
	ID        string    `json:"id"`
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	IsAdmin   bool      `json:"isAdmin"`
	CreatedAt time.Time `json:"createdAt"`
}

type RoleRule struct {
	ID        string    `json:"id"`
	RoleID    string    `json:"roleId"`
	RuleKey   string    `json:"ruleKey"`
	Effect    string    `json:"effect"` // "allow" or "deny"
	CreatedAt time.Time `json:"createdAt"`
}

func (s *Store) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := s.db.Query(ctx, `SELECT id::text, key, name, is_admin, created_at FROM roles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Role{}
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.Key, &r.Name, &r.IsAdmin, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetRole(ctx context.Context, id string) (Role, error) {
	var r Role
	err := s.db.QueryRow(ctx, `SELECT id::text, key, name, is_admin, created_at FROM roles WHERE id = $1`, id).
		Scan(&r.ID, &r.Key, &r.Name, &r.IsAdmin, &r.CreatedAt)
	if err != nil {
		return Role{}, errors.New("role not found")
	}
	return r, nil
}

func (s *Store) CreateRole(ctx context.Context, key, name string, isAdmin bool) (Role, error) {
	if key == "" || name == "" {
		return Role{}, errors.New("key and name are required")
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO roles (id, key, name, is_admin, created_at) VALUES ($1, $2, $3, $4, $5)
	`, id, key, name, isAdmin, now); err != nil {
		return Role{}, err
	}
	return Role{ID: id, Key: key, Name: name, IsAdmin: isAdmin, CreatedAt: now}, nil
}

func (s *Store) UpdateRole(ctx context.Context, id, key, name string, isAdmin bool) (Role, error) {
	if key == "" || name == "" {
		return Role{}, errors.New("key and name are required")
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE roles SET key = $1, name = $2, is_admin = $3 WHERE id = $4
	`, key, name, isAdmin, id)
	if err != nil {
		return Role{}, err
	}
	if tag.RowsAffected() == 0 {
		return Role{}, errors.New("role not found")
	}
	return s.GetRole(ctx, id)
}

func (s *Store) DeleteRole(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM roles WHERE id = $1`, id)
	return err
}

// AssignRole grants a user a role. Replaces any prior assignment of the
// same role (idempotent).
func (s *Store) AssignRole(ctx context.Context, userID, roleKey string) error {
	if _, err := s.db.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, r.id FROM roles r WHERE r.key = $2
		ON CONFLICT (user_id, role_id) DO NOTHING
	`, userID, roleKey); err != nil {
		return err
	}
	return nil
}

// RemoveRole revokes a user's role.
func (s *Store) RemoveRole(ctx context.Context, userID, roleKey string) error {
	if _, err := s.db.Exec(ctx, `
		DELETE FROM user_roles USING roles
		WHERE roles.id = user_roles.role_id
		  AND user_roles.user_id = $1
		  AND roles.key = $2
	`, userID, roleKey); err != nil {
		return err
	}
	return nil
}

// UserRoles returns the keys of all roles assigned to a user.
func (s *Store) UserRoles(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT r.key FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1
		ORDER BY r.key
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func uuidString() string { return uuid.NewString() }
