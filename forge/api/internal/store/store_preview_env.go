package store

import (
	"context"
	"encoding/json"
	"time"
)

// Preview-environment lifecycle helpers (Phase 4). Backed by the
// preview_deployments table with TTL support added in migration
// 180_preview_ttl.sql (expires_at).

const previewActiveStatuses = "('deploying', 'running', 'stopped', 'failed')"

// SetPreviewDeploymentExpiresAt sets the TTL deadline for a preview row. A nil
// deadline clears the deadline (no auto-destroy).
func (s *Store) SetPreviewDeploymentExpiresAt(ctx context.Context, id string, expiresAt *time.Time) error {
	_, err := s.db.Exec(ctx, `UPDATE preview_deployments SET expires_at = $2, updated_at = now() WHERE id = $1`, id, expiresAt)
	return err
}

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

// CountActivePreviewDeploymentsForOrg counts live previews ("deploying",
// "running", "stopped") that belong to repo_owner — the per-org lifecycle
// limit guard (PREVIEW_MAX_PER_ORG).
func (s *Store) CountActivePreviewDeploymentsForOrg(ctx context.Context, repoOwner string) (int, error) {
	var count int
	err := s.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM preview_deployments
		WHERE repo_owner = $1 AND status IN ('deploying', 'running', 'stopped')
	`, repoOwner).Scan(&count)
	return count, err
}

// ListExpiredPreviewDeployments returns live preview rows whose TTL has
// elapsed. The reaper turns these into cleaned_up.
func (s *Store) ListExpiredPreviewDeployments(ctx context.Context, before time.Time) ([]PreviewDeployment, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, service_id::text, pr_number,
		       COALESCE(pr_title, ''), COALESCE(pr_url, ''), COALESCE(branch, ''),
		       COALESCE(repo_owner, ''), COALESCE(repo_name, ''), COALESCE(commit_sha, ''),
		       status, COALESCE(preview_url, ''), COALESCE(deployment_url, ''),
		       source, COALESCE(unique_suffix, ''), COALESCE(is_isolated, true),
		       created_by::text, created_at, updated_at, cleaned_at, expires_at
		FROM preview_deployments
		WHERE status IN ('deploying', 'running') AND expires_at IS NOT NULL AND expires_at < $1
		ORDER BY expires_at ASC
	`, before)
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

// ListReapablePreviewDeployments returns cleaned_up rows whose cleanup is
// older than before — the retention half of the reaper (row expiry).
func (s *Store) ListReapablePreviewDeployments(ctx context.Context, before time.Time) ([]PreviewDeployment, error) {
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
	`, before)
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
func (s *Store) FindServerIDByGitSource(ctx context.Context, gitSourceID string) (string, error) {
	if s.db == nil || gitSourceID == "" {
		return "", nil
	}
	rows, err := s.db.Query(ctx, `SELECT uuid, COALESCE(docker_labels, '{}') FROM servers`)
	if err != nil {
		return "", err
	}
	defer rows.Close()

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
			return id, nil
		}
	}
	return "", rows.Err()
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