package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Preview-environment lifecycle helpers (Phase 4). Backed by the
// preview_deployments table with TTL support added in migration
// 180_preview_ttl.sql (expires_at).

// previewLiveStatuses are exactly the statuses that hold a branch/PR slot and
// consume an organisation permit — so they are also exactly the statuses the
// reaper has to be able to expire. 'cleaned_up' and 'failed' release the slot.
const previewLiveStatuses = "('deploying', 'running', 'stopped')"

// defaultReapBatch bounds one reaper pass so a backlog cannot pin a goroutine
// (or a connection) for minutes at a time.
const defaultReapBatch = 200

// SetPreviewDeploymentExpiresAt sets the TTL deadline for a preview row. A nil
// deadline clears the deadline (no auto-destroy). A row that is not there is an
// error, not a success: the caller has to know whether the preview it just
// created is reapable.
func (s *Store) SetPreviewDeploymentExpiresAt(ctx context.Context, id string, expiresAt *time.Time) error {
	tag, err := s.db.Exec(ctx, `UPDATE preview_deployments SET expires_at = $2, updated_at = now() WHERE id = $1`, id, expiresAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrPreviewDeploymentNotFound
	}
	return nil
}

// ErrPreviewDeploymentNotFound is returned when a preview row the caller named
// does not exist (anymore).
var ErrPreviewDeploymentNotFound = errors.New("preview deployment not found")

// ListPreviewDeploymentsWithExpiry returns every preview row including its
// scheduled expiry timestamp, newest first.
func (s *Store) ListPreviewDeploymentsWithExpiry(ctx context.Context) ([]PreviewDeployment, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, service_id::text, pr_number,
		       COALESCE(pr_title, ''), COALESCE(pr_url, ''), COALESCE(branch, ''),
		       COALESCE(repo_owner, ''), COALESCE(repo_name, ''), COALESCE(commit_sha, ''),
		       status, COALESCE(preview_url, ''), COALESCE(deployment_url, ''),
		       source, COALESCE(unique_suffix, ''), COALESCE(is_isolated, true),
		       created_by::text, created_at, updated_at, cleaned_at, expires_at
		FROM preview_deployments
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []PreviewDeployment
	for rows.Next() {
		p, err := scanPreviewWithExpiry(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// CountActivePreviewDeploymentsForOrg counts live previews that belong to
// repo_owner — the per-org lifecycle limit guard (PREVIEW_MAX_PER_ORG). The
// owner is matched case-insensitively because service.Create deduplicates PRs
// with EqualFold: a case-sensitive count would let "Acme" and "acme" each
// accumulate a full quota.
func (s *Store) CountActivePreviewDeploymentsForOrg(ctx context.Context, repoOwner string) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM preview_deployments
		WHERE lower(repo_owner) = lower($1) AND status IN `+previewLiveStatuses+`
	`, repoOwner).Scan(&count)
	return count, err
}

// ListExpiredPreviewDeployments returns up to limit live preview rows whose TTL
// has elapsed, oldest deadline first. The reaper turns these into cleaned_up.
// Every status that consumes a quota slot is reapable, otherwise a 'stopped'
// preview would hold its org permit forever.
func (s *Store) ListExpiredPreviewDeployments(ctx context.Context, before time.Time, limit int) ([]PreviewDeployment, error) {
	if limit <= 0 {
		limit = defaultReapBatch
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, service_id::text, pr_number,
		       COALESCE(pr_title, ''), COALESCE(pr_url, ''), COALESCE(branch, ''),
		       COALESCE(repo_owner, ''), COALESCE(repo_name, ''), COALESCE(commit_sha, ''),
		       status, COALESCE(preview_url, ''), COALESCE(deployment_url, ''),
		       source, COALESCE(unique_suffix, ''), COALESCE(is_isolated, true),
		       created_by::text, created_at, updated_at, cleaned_at, expires_at
		FROM preview_deployments
		WHERE status IN `+previewLiveStatuses+` AND expires_at IS NOT NULL AND expires_at < $1
		ORDER BY expires_at ASC
		LIMIT $2
	`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []PreviewDeployment
	for rows.Next() {
		p, err := scanPreviewWithExpiry(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// ListReapablePreviewDeployments returns up to limit cleaned_up rows whose
// cleanup is older than before — the retention half of the reaper (row expiry).
func (s *Store) ListReapablePreviewDeployments(ctx context.Context, before time.Time, limit int) ([]PreviewDeployment, error) {
	if limit <= 0 {
		limit = defaultReapBatch
	}
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, service_id::text, pr_number,
		       COALESCE(pr_title, ''), COALESCE(pr_url, ''), COALESCE(branch, ''),
		       COALESCE(repo_owner, ''), COALESCE(repo_name, ''), COALESCE(commit_sha, ''),
		       status, COALESCE(preview_url, ''), COALESCE(deployment_url, ''),
		       source, COALESCE(unique_suffix, ''), COALESCE(is_isolated, true),
		       created_by::text, created_at, updated_at, cleaned_at, expires_at
		FROM preview_deployments
		WHERE status = 'cleaned_up' AND COALESCE(cleaned_at, updated_at) < $1
		ORDER BY COALESCE(cleaned_at, updated_at) ASC
		LIMIT $2
	`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []PreviewDeployment
	for rows.Next() {
		p, err := scanPreviewWithExpiry(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// FindServerIDByGitSource resolves the server backing a git source by
// scanning the reverse label map (servers.docker_labels["git_source_id"]).
// The reverse query is intentionally SQL-portable: labels are parsed in Go
// instead of using JSONB operators, so postgres and sqlite behave alike.
//
// More than one server carrying the same git_source_id is an error, not a
// coin flip: deploying a preview onto an arbitrary one of two hosts is exactly
// the ambiguous targeting this codebase refuses to resolve implicitly.
func (s *Store) FindServerIDByGitSource(ctx context.Context, gitSourceID string) (string, error) {
	if s.db == nil || gitSourceID == "" {
		return "", nil
	}
	rows, err := s.db.Query(ctx, `SELECT uuid, COALESCE(docker_labels, '{}') FROM servers`)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var matches []string
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return "", err
		}
		var labels map[string]string
		if err := json.Unmarshal([]byte(raw), &labels); err != nil {
			continue
		}
		if labels["git_source_id"] == gitSourceID {
			matches = append(matches, id)
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	switch len(matches) {
	case 0:
		return "", nil
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("%d servers declare git source %s; label the server that owns it before previewing", len(matches), gitSourceID)
	}
}

func scanPreviewWithExpiry(rows interface{ Scan(dest ...any) error }) (PreviewDeployment, error) {
	var p PreviewDeployment
	err := rows.Scan(&p.ID, &p.ServerID, &p.ServiceID, &p.PRNumber,
		&p.PRTitle, &p.PRURL, &p.Branch, &p.RepoOwner, &p.RepoName, &p.CommitSHA,
		&p.Status, &p.PreviewURL, &p.DeploymentURL, &p.Source, &p.UniqueSuffix,
		&p.IsIsolated, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
		&p.CleanedAt, &p.ExpiresAt)
	return p, err
}