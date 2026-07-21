package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type RedirectRule struct {
	ID           string    `json:"id"`
	DomainID     string    `json:"domainId"`
	SourcePath   string    `json:"sourcePath"`
	TargetURL    string    `json:"targetUrl"`
	StatusCode   int       `json:"statusCode"`
	Regex        bool      `json:"regex"`
	PreservePath bool      `json:"preservePath"`
	Enabled      bool      `json:"enabled"`
	Priority     int       `json:"priority"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type RedirectRuleFilter struct {
	DomainID *string
	Enabled  *bool
	Limit    int
	Offset   int
}

func (s *Store) CreateRedirectRule(ctx context.Context, r RedirectRule) (*RedirectRule, error) {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	r.CreatedAt = now
	r.UpdatedAt = now

	_, err := s.db.Exec(ctx, `
		INSERT INTO redirect_rules (id, domain_id, source_path, target_url, status_code,
		                            regex, preserve_path, enabled, priority, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	`, r.ID, r.DomainID, r.SourcePath, r.TargetURL, r.StatusCode,
		r.Regex, r.PreservePath, r.Enabled, r.Priority, r.CreatedAt, r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) GetRedirectRule(ctx context.Context, id string) (*RedirectRule, error) {
	var r RedirectRule
	err := s.db.QueryRow(ctx, `
		SELECT id, domain_id, source_path, target_url, status_code,
		       regex, preserve_path, enabled, priority, created_at, updated_at
		FROM redirect_rules WHERE id = $1
	`, id).Scan(&r.ID, &r.DomainID, &r.SourcePath, &r.TargetURL, &r.StatusCode,
		&r.Regex, &r.PreservePath, &r.Enabled, &r.Priority, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

func (s *Store) ListRedirectRules(ctx context.Context, filter RedirectRuleFilter) ([]RedirectRule, error) {
	query := `SELECT id, domain_id, source_path, target_url, status_code,
	                 regex, preserve_path, enabled, priority, created_at, updated_at
	          FROM redirect_rules WHERE 1=1`
	args := []any{}
	argIdx := 1

	if filter.DomainID != nil {
		query += " AND domain_id = $" + itoa(argIdx)
		args = append(args, *filter.DomainID)
		argIdx++
	}
	if filter.Enabled != nil {
		query += " AND enabled = $" + itoa(argIdx)
		args = append(args, *filter.Enabled)
		argIdx++
	}

	query += " ORDER BY priority ASC"

	if filter.Limit > 0 {
		query += " LIMIT $" + itoa(argIdx)
		args = append(args, filter.Limit)
		argIdx++
	}
	if filter.Offset > 0 {
		query += " OFFSET $" + itoa(argIdx)
		args = append(args, filter.Offset)
		argIdx++
	}

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := make([]RedirectRule, 0)
	for rows.Next() {
		var r RedirectRule
		if err := rows.Scan(&r.ID, &r.DomainID, &r.SourcePath, &r.TargetURL, &r.StatusCode,
			&r.Regex, &r.PreservePath, &r.Enabled, &r.Priority, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (s *Store) UpdateRedirectRule(ctx context.Context, r RedirectRule) error {
	r.UpdatedAt = time.Now().UTC()
	tag, err := s.db.Exec(ctx, `
		UPDATE redirect_rules SET source_path=$1, target_url=$2, status_code=$3,
		       regex=$4, preserve_path=$5, enabled=$6, priority=$7, updated_at=$8
		WHERE id=$9
	`, r.SourcePath, r.TargetURL, r.StatusCode, r.Regex, r.PreservePath,
		r.Enabled, r.Priority, r.UpdatedAt, r.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("redirect rule not found")
	}
	return nil
}

func (s *Store) DeleteRedirectRule(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM redirect_rules WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("redirect rule not found")
	}
	return nil
}

func (s *Store) DeleteRedirectRulesByDomain(ctx context.Context, domainID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM redirect_rules WHERE domain_id = $1`, domainID)
	return err
}
