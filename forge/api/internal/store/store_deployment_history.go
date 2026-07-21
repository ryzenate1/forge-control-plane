package store

import (
	"context"
	"time"
)

type DeploymentRecord struct {
	ID            string     `json:"id"`
	ServerID      string     `json:"serverId"`
	ServiceID     *string    `json:"serviceId,omitempty"`
	Status        string     `json:"status"`
	LogPath       string     `json:"logPath,omitempty"`
	CommitHash    string     `json:"commitHash,omitempty"`
	CommitMessage string     `json:"commitMessage,omitempty"`
	ErrorMessage  string     `json:"errorMessage,omitempty"`
	RollbackID    *string    `json:"rollbackId,omitempty"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	FinishedAt    *time.Time `json:"finishedAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

type Rollback struct {
	ID           string    `json:"id"`
	DeploymentID string    `json:"deploymentId"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
}

type PreviewDeployment struct {
	ID             string     `json:"id"`
	ServerID       string     `json:"serverId"`
	ServiceID      *string    `json:"serviceId,omitempty"`
	PRNumber       int        `json:"prNumber"`
	PRTitle        string     `json:"prTitle,omitempty"`
	PRURL          string     `json:"prUrl,omitempty"`
	Branch         string     `json:"branch,omitempty"`
	RepoOwner      string     `json:"repoOwner,omitempty"`
	RepoName       string     `json:"repoName,omitempty"`
	CommitSHA      string     `json:"commitSha,omitempty"`
	Status         string     `json:"status"`
	PreviewURL     string     `json:"previewUrl,omitempty"`
	DeploymentURL  string     `json:"deploymentUrl,omitempty"`
	Source         string     `json:"source"`
	UniqueSuffix   string     `json:"uniqueSuffix,omitempty"`
	IsIsolated     bool       `json:"isIsolated"`
	CreatedBy      *string    `json:"createdBy,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	CleanedAt      *time.Time `json:"cleanedAt,omitempty"`
}

func (s *Store) CreateDeploymentRecord(ctx context.Context, d *DeploymentRecord) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO deployment_history (id, server_id, service_id, status, log_path, commit_hash, commit_message, error_message, rollback_id, started_at, finished_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, d.ID, d.ServerID, d.ServiceID, d.Status, d.LogPath, d.CommitHash, d.CommitMessage, d.ErrorMessage, d.RollbackID, d.StartedAt, d.FinishedAt, d.CreatedAt, d.UpdatedAt)
	return err
}

func (s *Store) GetDeploymentRecord(ctx context.Context, id string) (DeploymentRecord, error) {
	var d DeploymentRecord
	err := s.db.QueryRow(ctx, `
		SELECT id::text, server_id::text, service_id::text, status, COALESCE(log_path, ''), COALESCE(commit_hash, ''), COALESCE(commit_message, ''), COALESCE(error_message, ''), rollback_id::text, started_at, finished_at, created_at, updated_at
		FROM deployment_history
		WHERE id = $1
	`, id).Scan(&d.ID, &d.ServerID, &d.ServiceID, &d.Status, &d.LogPath, &d.CommitHash, &d.CommitMessage, &d.ErrorMessage, &d.RollbackID, &d.StartedAt, &d.FinishedAt, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func (s *Store) ListDeploymentRecords(ctx context.Context, serverID string) ([]DeploymentRecord, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, service_id::text, status, COALESCE(log_path, ''), COALESCE(commit_hash, ''), COALESCE(commit_message, ''), COALESCE(error_message, ''), rollback_id::text, started_at, finished_at, created_at, updated_at
		FROM deployment_history
		WHERE server_id = $1
		ORDER BY created_at DESC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DeploymentRecord
	for rows.Next() {
		var d DeploymentRecord
		if err := rows.Scan(&d.ID, &d.ServerID, &d.ServiceID, &d.Status, &d.LogPath, &d.CommitHash, &d.CommitMessage, &d.ErrorMessage, &d.RollbackID, &d.StartedAt, &d.FinishedAt, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (s *Store) ListAllDeploymentRecords(ctx context.Context) ([]DeploymentRecord, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, service_id::text, status, COALESCE(log_path, ''), COALESCE(commit_hash, ''), COALESCE(commit_message, ''), COALESCE(error_message, ''), rollback_id::text, started_at, finished_at, created_at, updated_at
		FROM deployment_history
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DeploymentRecord
	for rows.Next() {
		var d DeploymentRecord
		if err := rows.Scan(&d.ID, &d.ServerID, &d.ServiceID, &d.Status, &d.LogPath, &d.CommitHash, &d.CommitMessage, &d.ErrorMessage, &d.RollbackID, &d.StartedAt, &d.FinishedAt, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

func (s *Store) UpdateDeploymentRecordStatus(ctx context.Context, id string, status string, errMsg string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE deployment_history
		SET status = $2, error_message = CASE WHEN $3 = '' THEN error_message ELSE $3 END,
		    finished_at = CASE WHEN $2 IN ('done', 'error', 'cancelled') THEN now() ELSE finished_at END,
		    updated_at = now()
		WHERE id = $1
	`, id, status, errMsg)
	return err
}

func (s *Store) UpdateDeploymentRecord(ctx context.Context, d *DeploymentRecord) error {
	_, err := s.db.Exec(ctx, `
		UPDATE deployment_history
		SET server_id = $2, service_id = $3, status = $4, log_path = $5, commit_hash = $6, commit_message = $7,
		    error_message = $8, rollback_id = $9, started_at = $10, finished_at = $11, updated_at = now()
		WHERE id = $1
	`, d.ID, d.ServerID, d.ServiceID, d.Status, d.LogPath, d.CommitHash, d.CommitMessage, d.ErrorMessage, d.RollbackID, d.StartedAt, d.FinishedAt)
	return err
}

func (s *Store) DeleteDeploymentRecord(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM deployment_history WHERE id = $1`, id)
	return err
}

func (s *Store) CreateRollback(ctx context.Context, r *Rollback) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO rollbacks (id, deployment_id, status, created_at)
		VALUES ($1, $2, $3, $4)
	`, r.ID, r.DeploymentID, r.Status, r.CreatedAt)
	return err
}

func (s *Store) GetRollback(ctx context.Context, id string) (Rollback, error) {
	var r Rollback
	err := s.db.QueryRow(ctx, `
		SELECT id::text, deployment_id::text, status, created_at
		FROM rollbacks
		WHERE id = $1
	`, id).Scan(&r.ID, &r.DeploymentID, &r.Status, &r.CreatedAt)
	return r, err
}

func (s *Store) ListRollbacks(ctx context.Context, deploymentID string) ([]Rollback, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, deployment_id::text, status, created_at
		FROM rollbacks
		WHERE deployment_id = $1
		ORDER BY created_at DESC
	`, deploymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Rollback
	for rows.Next() {
		var r Rollback
		if err := rows.Scan(&r.ID, &r.DeploymentID, &r.Status, &r.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Store) UpdateRollbackStatus(ctx context.Context, id string, status string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE rollbacks
		SET status = $2, updated_at = now()
		WHERE id = $1
	`, id, status)
	return err
}

func (s *Store) CreatePreviewDeployment(ctx context.Context, p *PreviewDeployment) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO preview_deployments (id, server_id, service_id, pr_number, pr_title, pr_url, branch, repo_owner, repo_name, commit_sha, status, preview_url, deployment_url, source, unique_suffix, is_isolated, created_by, created_at, updated_at, cleaned_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
	`, p.ID, p.ServerID, p.ServiceID, p.PRNumber, p.PRTitle, p.PRURL, p.Branch, p.RepoOwner, p.RepoName, p.CommitSHA, p.Status, p.PreviewURL, p.DeploymentURL, p.Source, p.UniqueSuffix, p.IsIsolated, p.CreatedBy, p.CreatedAt, p.UpdatedAt, p.CleanedAt)
	return err
}

func (s *Store) GetPreviewDeployment(ctx context.Context, id string) (PreviewDeployment, error) {
	var p PreviewDeployment
	err := s.db.QueryRow(ctx, `
		SELECT id::text, server_id::text, service_id::text, pr_number, COALESCE(pr_title, ''), COALESCE(pr_url, ''), COALESCE(branch, ''), COALESCE(repo_owner, ''), COALESCE(repo_name, ''), COALESCE(commit_sha, ''), status, COALESCE(preview_url, ''), COALESCE(deployment_url, ''), source, COALESCE(unique_suffix, ''), COALESCE(is_isolated, true), created_by::text, created_at, updated_at, cleaned_at
		FROM preview_deployments
		WHERE id = $1
	`, id).Scan(&p.ID, &p.ServerID, &p.ServiceID, &p.PRNumber, &p.PRTitle, &p.PRURL, &p.Branch, &p.RepoOwner, &p.RepoName, &p.CommitSHA, &p.Status, &p.PreviewURL, &p.DeploymentURL, &p.Source, &p.UniqueSuffix, &p.IsIsolated, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt, &p.CleanedAt)
	return p, err
}

func (s *Store) ListPreviewDeployments(ctx context.Context, serverID string) ([]PreviewDeployment, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, service_id::text, pr_number, COALESCE(pr_title, ''), COALESCE(pr_url, ''), COALESCE(branch, ''), COALESCE(repo_owner, ''), COALESCE(repo_name, ''), COALESCE(commit_sha, ''), status, COALESCE(preview_url, ''), COALESCE(deployment_url, ''), source, COALESCE(unique_suffix, ''), COALESCE(is_isolated, true), created_by::text, created_at, updated_at, cleaned_at
		FROM preview_deployments
		WHERE server_id = $1
		ORDER BY created_at DESC
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []PreviewDeployment
	for rows.Next() {
		var p PreviewDeployment
		if err := rows.Scan(&p.ID, &p.ServerID, &p.ServiceID, &p.PRNumber, &p.PRTitle, &p.PRURL, &p.Branch, &p.RepoOwner, &p.RepoName, &p.CommitSHA, &p.Status, &p.PreviewURL, &p.DeploymentURL, &p.Source, &p.UniqueSuffix, &p.IsIsolated, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt, &p.CleanedAt); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (s *Store) ListAllPreviewDeployments(ctx context.Context) ([]PreviewDeployment, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, service_id::text, pr_number, COALESCE(pr_title, ''), COALESCE(pr_url, ''), COALESCE(branch, ''), COALESCE(repo_owner, ''), COALESCE(repo_name, ''), COALESCE(commit_sha, ''), status, COALESCE(preview_url, ''), COALESCE(deployment_url, ''), source, COALESCE(unique_suffix, ''), COALESCE(is_isolated, true), created_by::text, created_at, updated_at, cleaned_at
		FROM preview_deployments
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []PreviewDeployment
	for rows.Next() {
		var p PreviewDeployment
		if err := rows.Scan(&p.ID, &p.ServerID, &p.ServiceID, &p.PRNumber, &p.PRTitle, &p.PRURL, &p.Branch, &p.RepoOwner, &p.RepoName, &p.CommitSHA, &p.Status, &p.PreviewURL, &p.DeploymentURL, &p.Source, &p.UniqueSuffix, &p.IsIsolated, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt, &p.CleanedAt); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (s *Store) UpdatePreviewDeployment(ctx context.Context, p *PreviewDeployment) error {
	_, err := s.db.Exec(ctx, `
		UPDATE preview_deployments
		SET status = $2, preview_url = $3, deployment_url = $4, commit_sha = $5, pr_title = $6, updated_at = now()
		WHERE id = $1
	`, p.ID, p.Status, p.PreviewURL, p.DeploymentURL, p.CommitSHA, p.PRTitle)
	return err
}

func (s *Store) UpdatePreviewDeploymentStatus(ctx context.Context, id string, status string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE preview_deployments
		SET status = $2, cleaned_at = CASE WHEN $2 = 'cleaned_up' THEN now() ELSE cleaned_at END, updated_at = now()
		WHERE id = $1
	`, id, status)
	return err
}

func (s *Store) DeletePreviewDeployment(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM preview_deployments WHERE id = $1`, id)
	return err
}

func (s *Store) ListActivePreviewDeployments(ctx context.Context) ([]PreviewDeployment, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, server_id::text, service_id::text, pr_number, COALESCE(pr_title, ''), COALESCE(pr_url, ''), COALESCE(branch, ''), COALESCE(repo_owner, ''), COALESCE(repo_name, ''), COALESCE(commit_sha, ''), status, COALESCE(preview_url, ''), COALESCE(deployment_url, ''), source, COALESCE(unique_suffix, ''), COALESCE(is_isolated, true), created_by::text, created_at, updated_at, cleaned_at
		FROM preview_deployments
		WHERE status NOT IN ('cleaned_up', 'failed')
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []PreviewDeployment
	for rows.Next() {
		var p PreviewDeployment
		if err := rows.Scan(&p.ID, &p.ServerID, &p.ServiceID, &p.PRNumber, &p.PRTitle, &p.PRURL, &p.Branch, &p.RepoOwner, &p.RepoName, &p.CommitSHA, &p.Status, &p.PreviewURL, &p.DeploymentURL, &p.Source, &p.UniqueSuffix, &p.IsIsolated, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt, &p.CleanedAt); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
