package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type DomainRow struct {
	ID                string     `json:"id"`
	ServerID          string     `json:"serverId"`
	Domain            string     `json:"domain"`
	Wildcard          bool       `json:"wildcard"`
	Verified          bool       `json:"verified"`
	VerifiedAt        *time.Time `json:"verifiedAt,omitempty"`
	VerificationToken *string    `json:"verificationToken,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
}

func (s *Store) ListDomainsByServer(ctx context.Context, serverID string) ([]DomainRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, server_id, domain, wildcard, verified, verified_at,
		       COALESCE(verification_token, ''), created_at
		FROM domains
		WHERE server_id = $1
		ORDER BY created_at DESC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	domains := make([]DomainRow, 0)
	for rows.Next() {
		var d DomainRow
		var token string
		if err := rows.Scan(&d.ID, &d.ServerID, &d.Domain, &d.Wildcard, &d.Verified,
			&d.VerifiedAt, &token, &d.CreatedAt); err != nil {
			return nil, err
		}
		if token != "" {
			d.VerificationToken = &token
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}

func (s *Store) ListAllDomains(ctx context.Context) ([]DomainRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, server_id, domain, wildcard, verified, verified_at,
		       COALESCE(verification_token, ''), created_at
		FROM domains
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	domains := make([]DomainRow, 0)
	for rows.Next() {
		var d DomainRow
		var token string
		if err := rows.Scan(&d.ID, &d.ServerID, &d.Domain, &d.Wildcard, &d.Verified,
			&d.VerifiedAt, &token, &d.CreatedAt); err != nil {
			return nil, err
		}
		if token != "" {
			d.VerificationToken = &token
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}

func (s *Store) GetDomain(ctx context.Context, id string) (*DomainRow, error) {
	var d DomainRow
	var token string
	err := s.db.QueryRow(ctx, `
		SELECT id, server_id, domain, wildcard, verified, verified_at,
		       COALESCE(verification_token, ''), created_at
		FROM domains
		WHERE id = $1
	`, id).Scan(&d.ID, &d.ServerID, &d.Domain, &d.Wildcard, &d.Verified,
		&d.VerifiedAt, &token, &d.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if token != "" {
		d.VerificationToken = &token
	}
	return &d, nil
}

func (s *Store) GetDomainByDomain(ctx context.Context, domain string) (*DomainRow, error) {
	var d DomainRow
	var token string
	err := s.db.QueryRow(ctx, `
		SELECT id, server_id, domain, wildcard, verified, verified_at,
		       COALESCE(verification_token, ''), created_at
		FROM domains
		WHERE domain = $1
	`, domain).Scan(&d.ID, &d.ServerID, &d.Domain, &d.Wildcard, &d.Verified,
		&d.VerifiedAt, &token, &d.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if token != "" {
		d.VerificationToken = &token
	}
	return &d, nil
}

func (s *Store) CreateDomain(ctx context.Context, domain DomainRow) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO domains (id, server_id, domain, wildcard, verified, verified_at,
		                     verification_token, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, domain.ID, domain.ServerID, domain.Domain, domain.Wildcard, domain.Verified,
		domain.VerifiedAt, domain.VerificationToken, domain.CreatedAt)
	return err
}

func (s *Store) UpdateDomainVerification(ctx context.Context, id string, verified bool, verifiedAt *time.Time) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE domains
		SET verified = $1, verified_at = $2
		WHERE id = $3
	`, verified, verifiedAt, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("domain not found")
	}
	return nil
}

func (s *Store) SetDomainVerificationToken(ctx context.Context, id string, token string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE domains
		SET verification_token = $1
		WHERE id = $2
	`, token, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("domain not found")
	}
	return nil
}

func (s *Store) DeleteDomain(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM domains WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("domain not found")
	}
	return nil
}

func (s *Store) ListUnverifiedDomains(ctx context.Context) ([]DomainRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, server_id, domain, wildcard, verified, verified_at,
		       COALESCE(verification_token, ''), created_at
		FROM domains
		WHERE verified = FALSE
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	domains := make([]DomainRow, 0)
	for rows.Next() {
		var d DomainRow
		var token string
		if err := rows.Scan(&d.ID, &d.ServerID, &d.Domain, &d.Wildcard, &d.Verified,
			&d.VerifiedAt, &token, &d.CreatedAt); err != nil {
			return nil, err
		}
		if token != "" {
			d.VerificationToken = &token
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}

func (s *Store) ListVerifiedDomains(ctx context.Context) ([]DomainRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, server_id, domain, wildcard, verified, verified_at,
		       COALESCE(verification_token, ''), created_at
		FROM domains
		WHERE verified = TRUE
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	domains := make([]DomainRow, 0)
	for rows.Next() {
		var d DomainRow
		var token string
		if err := rows.Scan(&d.ID, &d.ServerID, &d.Domain, &d.Wildcard, &d.Verified,
			&d.VerifiedAt, &token, &d.CreatedAt); err != nil {
			return nil, err
		}
		if token != "" {
			d.VerificationToken = &token
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}
