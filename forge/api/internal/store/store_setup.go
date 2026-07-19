package store

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

func (s *Store) HasAnyAdmin(ctx context.Context) (bool, error) {
	if s.db == nil {
		return false, errors.New("no database connection")
	}
	var count int
	err := s.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM users u
		WHERE NOT u.disabled
		  AND EXISTS (
		      SELECT 1 FROM user_roles ur
		      JOIN roles r ON r.id = ur.role_id
		      WHERE ur.user_id = u.id AND r.is_admin
		  )
	`).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) CreateSetupAdmin(ctx context.Context, email, passwordHash string) (User, error) {
	if s.db == nil {
		return User{}, errors.New("no database connection")
	}
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return User{}, errors.New("email is required")
	}
	id := uuid.NewString()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, role)
		VALUES ($1, $2, $3, 'admin')
		ON CONFLICT (email) DO NOTHING
	`, id, email, passwordHash); err != nil {
		return User{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, r.id FROM roles r WHERE r.key = 'admin'
		ON CONFLICT (user_id, role_id) DO NOTHING
	`, id); err != nil {
		return User{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}

	return User{ID: id, Email: email, Role: "admin"}, nil
}
