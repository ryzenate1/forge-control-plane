package store

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// SetupPendingState flags setup-wizard sections that still hold default
// values after setup completed. It is derived from live state — no extra
// persistence — so the login page and dashboard can nudge the operator to
// finish skipped sections later.
type SetupPendingState struct {
	Organization bool `json:"organization"`
	Node         bool `json:"node"`
	Email        bool `json:"email"`
}

// GetSetupPendingState reports which wizard sections are still unconfigured.
// A nil state with a nil error is impossible; a nil state always comes with
// an error, so callers omit the field rather than claim "all configured"
// when the database could not be read.
func (s *Store) GetSetupPendingState(ctx context.Context) (*SetupPendingState, error) {
	if s.db == nil {
		return nil, errors.New("no database connection")
	}
	var orgs, nodes int
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM organizations`).Scan(&orgs); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM nodes`).Scan(&nodes); err != nil {
		return nil, err
	}
	var smtpHost string
	err := s.db.QueryRow(ctx, `SELECT COALESCE(smtp_host,'') FROM panel_mail_settings WHERE id = TRUE`).Scan(&smtpHost)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		smtpHost = ""
	}
	return &SetupPendingState{
		Organization: orgs == 0,
		Node:         nodes == 0,
		Email:        strings.TrimSpace(smtpHost) == "",
	}, nil
}

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
