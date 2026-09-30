package redirects

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the persistence contract the Service is written against. Keeping it
// an interface means validation, circular-loop detection and gateway rendering
// can be driven from tests without a database; PostgresStore is the production
// implementation.
type Store interface {
	ListByApplication(ctx context.Context, applicationID string) ([]Redirect, error)
	Get(ctx context.Context, applicationID, redirectID string) (Redirect, error)
	Create(ctx context.Context, redirect Redirect) (Redirect, error)
	Update(ctx context.Context, redirect Redirect) (Redirect, error)
	Delete(ctx context.Context, applicationID, redirectID string) (bool, error)

	// ListApplicationDomains returns the hostnames bound to an application
	// (proxy_domains where service_type = 'app'). Presets and the gateway
	// renderer both need to know which hosts the application actually serves;
	// without it "Canonicalise www" would happily redirect to a host nothing
	// answers for.
	ListApplicationDomains(ctx context.Context, applicationID string) ([]string, error)
}

// PostgresStore implements Store over the shared connection pool, the same way
// the resource-limits phase does, so this feature adds no files to
// internal/store.
type PostgresStore struct {
	db *pgxpool.Pool
}

func NewPostgresStore(db *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{db: db}
}

var _ Store = (*PostgresStore)(nil)

var errNoPool = errors.New("redirects store: postgres pool is not configured")

// redirectColumns is shared by every read so a column rename cannot leave one
// reader behind. target_path and preset_type are deliberately nullable: the
// scanner reads them through intermediate values instead of pretending "no
// value" and "empty string" are the same thing.
const redirectColumns = `
	id::text, application_id::text, source_domain, target_domain, source_path,
	target_path, status_code, COALESCE(preset_type, ''), enabled, created_at, updated_at`

func scanRedirect(row pgx.Row) (Redirect, error) {
	var r Redirect
	var targetPath *string
	// preset_type is read as a plain string and converted: pgx scans named string
	// types through the generic path, and going via string keeps the scanner
	// explicit about the same nullable column the SQL already flattens with
	// COALESCE.
	var preset string
	err := row.Scan(
		&r.ID, &r.ApplicationID, &r.SourceDomain, &r.TargetDomain, &r.SourcePath,
		&targetPath, &r.StatusCode, &preset, &r.Enabled, &r.CreatedAt, &r.UpdatedAt,
	)
	r.TargetPath = targetPath
	r.PresetType = PresetType(preset)
	return r, err
}

func (s *PostgresStore) ListByApplication(ctx context.Context, applicationID string) ([]Redirect, error) {
	if s.db == nil {
		return nil, errNoPool
	}
	rows, err := s.db.Query(ctx, `
		SELECT `+redirectColumns+`
		FROM domain_redirects WHERE application_id = $1
		ORDER BY source_domain, source_path, target_domain
	`, applicationID)
	if err != nil {
		return nil, fmt.Errorf("list redirect rules: %w", err)
	}
	defer rows.Close()

	rules := make([]Redirect, 0, 8)
	for rows.Next() {
		rule, err := scanRedirect(rows)
		if err != nil {
			return nil, fmt.Errorf("scan redirect rule: %w", err)
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (s *PostgresStore) Get(ctx context.Context, applicationID, redirectID string) (Redirect, error) {
	if s.db == nil {
		return Redirect{}, errNoPool
	}
	rule, err := scanRedirect(s.db.QueryRow(ctx, `
		SELECT `+redirectColumns+`
		FROM domain_redirects WHERE id = $1 AND application_id = $2
	`, redirectID, applicationID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Redirect{}, ErrRedirectNotFound
		}
		return Redirect{}, fmt.Errorf("get redirect rule %s: %w", redirectID, err)
	}
	return rule, nil
}

// Create inserts one rule and reads it back, so the caller receives the stored
// id and timestamps rather than the values it guessed at.
func (s *PostgresStore) Create(ctx context.Context, rule Redirect) (Redirect, error) {
	if s.db == nil {
		return Redirect{}, errNoPool
	}
	id := rule.ID
	if id == "" {
		id = uuid.NewString()
	}
	var preset any
	if rule.PresetType != "" {
		preset = string(rule.PresetType)
	}
	// The row is written and read back in one statement. A UNIQUE violation
	// surfaces as an error the Service maps to ErrDuplicateRedirect, so the
	// pre-check in validateCandidate is a courtesy, not the enforcement.
	created, err := scanRedirect(s.db.QueryRow(ctx, `
		INSERT INTO domain_redirects (
			id, application_id, source_domain, target_domain, source_path,
			target_path, status_code, preset_type, enabled, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW()
		)
		RETURNING `+redirectColumns,
		id, rule.ApplicationID, rule.SourceDomain, rule.TargetDomain, rule.SourcePath,
		rule.TargetPath, rule.StatusCode, preset, rule.Enabled,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return Redirect{}, ErrDuplicateRedirect
		}
		return Redirect{}, fmt.Errorf("create redirect rule: %w", err)
	}
	return created, nil
}

func (s *PostgresStore) Update(ctx context.Context, rule Redirect) (Redirect, error) {
	if s.db == nil {
		return Redirect{}, errNoPool
	}
	var preset any
	if rule.PresetType != "" {
		preset = string(rule.PresetType)
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE domain_redirects SET
			source_domain = $3, target_domain = $4, source_path = $5,
			target_path = $6, status_code = $7, preset_type = $8, enabled = $9,
			updated_at = NOW()
		WHERE id = $1 AND application_id = $2
	`, rule.ID, rule.ApplicationID, rule.SourceDomain, rule.TargetDomain, rule.SourcePath,
		rule.TargetPath, rule.StatusCode, preset, rule.Enabled)
	if err != nil {
		if isUniqueViolation(err) {
			return Redirect{}, ErrDuplicateRedirect
		}
		return Redirect{}, fmt.Errorf("update redirect rule %s: %w", rule.ID, err)
	}
	if tag.RowsAffected() == 0 {
		// Either the row is gone or it belongs to a different application. Both
		// mean the caller's edit changed nothing, so say so instead of
		// returning the untouched row as if it were a success.
		return Redirect{}, ErrRedirectNotFound
	}
	return s.Get(ctx, rule.ApplicationID, rule.ID)
}

func (s *PostgresStore) Delete(ctx context.Context, applicationID, redirectID string) (bool, error) {
	if s.db == nil {
		return false, errNoPool
	}
	tag, err := s.db.Exec(ctx, `
		DELETE FROM domain_redirects WHERE id = $1 AND application_id = $2
	`, redirectID, applicationID)
	if err != nil {
		return false, fmt.Errorf("delete redirect rule %s: %w", redirectID, err)
	}
	return tag.RowsAffected() > 0, nil
}

// ListApplicationDomains reads the hostnames the application is configured to
// serve. They live in proxy_domains keyed by (service_id, service_type='app'),
// which is the same source /apps/:id/domains answers from; wildcards are
// excluded because a redirect needs a concrete host to match.
func (s *PostgresStore) ListApplicationDomains(ctx context.Context, applicationID string) ([]string, error) {
	if s.db == nil {
		return nil, errNoPool
	}
	applicationID = strings.TrimSpace(applicationID)
	if applicationID == "" {
		return nil, ErrEmptyAppID
	}
	rows, err := s.db.Query(ctx, `
		SELECT hostname
		FROM proxy_domains
		WHERE service_id = $1 AND service_type = 'app'
		  AND hostname NOT LIKE '*%'
		ORDER BY hostname
	`, applicationID)
	if err != nil {
		return nil, fmt.Errorf("list application domains: %w", err)
	}
	defer rows.Close()

	hostnames := make([]string, 0, 4)
	for rows.Next() {
		var hostname string
		if err := rows.Scan(&hostname); err != nil {
			return nil, fmt.Errorf("scan application domain: %w", err)
		}
		normalized, err := NormalizeDomain(hostname)
		if err != nil || normalized == "" {
			// A stored hostname we cannot parse is a data problem, not a request
			// problem: skip the unusable host rather than failing the whole list.
			continue
		}
		hostnames = append(hostnames, normalized)
	}
	return hostnames, rows.Err()
}

// isUniqueViolation detects the 23xxx integrity-constraint family without
// pulling in a pgconn dependency just for the error code. The messages Postgres
// and the pgx driver emit both name the constraint, so matching on text here is
// reliable enough for a boolean; anything else stays an internal error.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key value") || strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "unique violation") || strings.Contains(msg, "23505")
}
