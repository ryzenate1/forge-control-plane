package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// GitDeployment represents a Git-based deployment in the database
type GitDeployment struct {
	ID             string    `json:"id"`
	GitSourceID    string    `json:"gitSourceId"`
	CommitSHA      string    `json:"commitSha"`
	Branch         string    `json:"branch"`
	Status         string    `json:"status"`
	StatusMessage  string    `json:"statusMessage,omitempty"`
	ImageTag       string    `json:"imageTag,omitempty"`
	BuildLog       string    `json:"buildLog,omitempty"`
	DeployLog      string    `json:"deployLog,omitempty"`
	Error          string    `json:"error,omitempty"`
	StartedAt      time.Time `json:"startedAt"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// CreateGitDeploymentRequest represents a request to create a Git deployment record
type CreateGitDeploymentRequest struct {
	GitSourceID    string
	CommitSHA      string
	Branch         string
	Status         string
	StatusMessage  string
	ImageTag       string
	BuildLog       string
	DeployLog      string
	Error          string
	StartedAt      time.Time
	CompletedAt    *time.Time
}

// CreateGitDeployment creates a new Git deployment record
func (s *Store) CreateGitDeployment(ctx context.Context, req CreateGitDeploymentRequest) (*GitDeployment, error) {
	if s.db == nil {
		return nil, nil
	}

	id := uuid.NewString()
	now := time.Now().UTC()

	deployment := &GitDeployment{
		ID:             id,
		GitSourceID:    req.GitSourceID,
		CommitSHA:      req.CommitSHA,
		Branch:         req.Branch,
		Status:         req.Status,
		StatusMessage:  req.StatusMessage,
		ImageTag:       req.ImageTag,
		BuildLog:       req.BuildLog,
		DeployLog:      req.DeployLog,
		Error:          req.Error,
		StartedAt:      req.StartedAt,
		CompletedAt:    req.CompletedAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	query := `
		INSERT INTO git_deployments (
			id, git_source_id, commit_sha, branch, status, status_message,
			image_tag, build_log, deploy_log, error, started_at, completed_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`

	_, err := s.db.Exec(ctx, query,
		id, req.GitSourceID, req.CommitSHA, req.Branch, req.Status, req.StatusMessage,
		req.ImageTag, req.BuildLog, req.DeployLog, req.Error, req.StartedAt, req.CompletedAt, now, now)
	if err != nil {
		return nil, err
	}

	return deployment, nil
}

// GetGitDeployment retrieves a Git deployment by ID
func (s *Store) GetGitDeployment(ctx context.Context, id string) (*GitDeployment, error) {
	if s.db == nil {
		return nil, nil
	}

	query := `
		SELECT id, git_source_id, commit_sha, branch, status, status_message,
			image_tag, build_log, deploy_log, error, started_at, completed_at, created_at, updated_at
		FROM git_deployments WHERE id = $1
	`

	var deployment GitDeployment
	var completedAt time.Time

	err := s.db.QueryRow(ctx, query, id).Scan(
		&deployment.ID, &deployment.GitSourceID, &deployment.CommitSHA, &deployment.Branch,
		&deployment.Status, &deployment.StatusMessage, &deployment.ImageTag,
		&deployment.BuildLog, &deployment.DeployLog, &deployment.Error,
		&deployment.StartedAt, &completedAt, &deployment.CreatedAt, &deployment.UpdatedAt)
	if err != nil {
		return nil, err
	}

	if completedAt != (time.Time{}) {
		deployment.CompletedAt = &completedAt
	}

	return &deployment, nil
}

// ListGitDeployments retrieves deployment history for a Git source
func (s *Store) ListGitDeployments(ctx context.Context, gitSourceID string, limit int) ([]GitDeployment, error) {
	if s.db == nil {
		return []GitDeployment{}, nil
	}

	query := `
		SELECT id, git_source_id, commit_sha, branch, status, status_message,
			image_tag, build_log, deploy_log, error, started_at, completed_at, created_at, updated_at
		FROM git_deployments
		WHERE git_source_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := s.db.Query(ctx, query, gitSourceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	deployments := []GitDeployment{}
	for rows.Next() {
		var deployment GitDeployment
		var completedAt time.Time

		if err := rows.Scan(
			&deployment.ID, &deployment.GitSourceID, &deployment.CommitSHA, &deployment.Branch,
			&deployment.Status, &deployment.StatusMessage, &deployment.ImageTag,
			&deployment.BuildLog, &deployment.DeployLog, &deployment.Error,
			&deployment.StartedAt, &completedAt, &deployment.CreatedAt, &deployment.UpdatedAt); err != nil {
			return nil, err
		}

		if completedAt != (time.Time{}) {
			deployment.CompletedAt = &completedAt
		}

		deployments = append(deployments, deployment)
	}

	return deployments, rows.Err()
}

// UpdateGitDeployment updates a Git deployment record
func (s *Store) UpdateGitDeployment(ctx context.Context, id string, status, statusMessage, error string) error {
	if s.db == nil {
		return nil
	}

	query := `
		UPDATE git_deployments
		SET status = $1, status_message = $2, error = $3, updated_at = NOW()
		WHERE id = $4
	`

	_, err := s.db.Exec(ctx, query, status, statusMessage, error, id)
	return err
}

// UpdateGitDeploymentWithImage updates a Git deployment record with image information
func (s *Store) UpdateGitDeploymentWithImage(ctx context.Context, id, imageTag, buildLog string) error {
	if s.db == nil {
		return nil
	}

	query := `
		UPDATE git_deployments
		SET image_tag = $1, build_log = $2, updated_at = NOW()
		WHERE id = $3
	`

	_, err := s.db.Exec(ctx, query, imageTag, buildLog, id)
	return err
}

// CompleteGitDeployment marks a Git deployment as completed
func (s *Store) CompleteGitDeployment(ctx context.Context, id string, deployLog string) error {
	if s.db == nil {
		return nil
	}

	query := `
		UPDATE git_deployments
		SET status = 'completed', deploy_log = $1, completed_at = NOW(), updated_at = NOW()
		WHERE id = $2
	`

	_, err := s.db.Exec(ctx, query, deployLog, id)
	return err
}

// FailGitDeployment marks a Git deployment as failed
func (s *Store) FailGitDeployment(ctx context.Context, id, errorMsg string) error {
	if s.db == nil {
		return nil
	}

	query := `
		UPDATE git_deployments
		SET status = 'failed', error = $1, completed_at = NOW(), updated_at = NOW()
		WHERE id = $2
	`

	_, err := s.db.Exec(ctx, query, errorMsg, id)
	return err
}

// GetLatestGitDeployment retrieves the latest deployment for a Git source
func (s *Store) GetLatestGitDeployment(ctx context.Context, gitSourceID string) (*GitDeployment, error) {
	if s.db == nil {
		return nil, nil
	}

	query := `
		SELECT id, git_source_id, commit_sha, branch, status, status_message,
			image_tag, build_log, deploy_log, error, started_at, completed_at, created_at, updated_at
		FROM git_deployments
		WHERE git_source_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`

	var deployment GitDeployment
	var completedAt time.Time

	err := s.db.QueryRow(ctx, query, gitSourceID).Scan(
		&deployment.ID, &deployment.GitSourceID, &deployment.CommitSHA, &deployment.Branch,
		&deployment.Status, &deployment.StatusMessage, &deployment.ImageTag,
		&deployment.BuildLog, &deployment.DeployLog, &deployment.Error,
		&deployment.StartedAt, &completedAt, &deployment.CreatedAt, &deployment.UpdatedAt)
	if err != nil {
		return nil, err
	}

	if completedAt != (time.Time{}) {
		deployment.CompletedAt = &completedAt
	}

	return &deployment, nil
}

// IncrementGitSourceDeploymentCount increments the deployment count for a Git source
func (s *Store) IncrementGitSourceDeploymentCount(ctx context.Context, gitSourceID string) error {
	if s.db == nil {
		return nil
	}

	query := `
		UPDATE git_sources
		SET deployment_count = deployment_count + 1,
			last_deployment_at = NOW(),
			updated_at = NOW()
		WHERE id = $1
	`

	_, err := s.db.Exec(ctx, query, gitSourceID)
	return err
}

// UpdateGitSourceLastDeployment updates the last deployment info for a Git source
func (s *Store) UpdateGitSourceLastDeployment(ctx context.Context, gitSourceID, deploymentID, status string) error {
	if s.db == nil {
		return nil
	}

	query := `
		UPDATE git_sources
		SET last_deployment_id = $1,
			last_deployment_status = $2,
			last_deployment_at = NOW(),
			updated_at = NOW()
		WHERE id = $3
	`

	_, err := s.db.Exec(ctx, query, deploymentID, status, gitSourceID)
	return err
}

// GitDeploymentHook represents a webhook for git deployment triggers
type GitDeploymentHook struct {
	ID         string    `json:"id"`
	GitSourceID string    `json:"gitSourceId"`
	Secret     string    `json:"secret,omitempty"`
	Events     []string  `json:"events"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (s *Store) ListGitDeploymentHooks(ctx context.Context, gitSourceID string) ([]GitDeploymentHook, error) {
	if s.db == nil {
		return []GitDeploymentHook{}, nil
	}

	query := `
		SELECT id, git_source_id, secret, events, created_at, updated_at
		FROM git_deployment_hooks
		WHERE git_source_id = $1
		ORDER BY created_at DESC
	`

	rows, err := s.db.Query(ctx, query, gitSourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hooks := []GitDeploymentHook{}
	for rows.Next() {
		var hook GitDeploymentHook
		if err := rows.Scan(&hook.ID, &hook.GitSourceID, &hook.Secret, &hook.Events, &hook.CreatedAt, &hook.UpdatedAt); err != nil {
			return nil, err
		}
		hooks = append(hooks, hook)
	}

	return hooks, rows.Err()
}

func (s *Store) CreateGitDeploymentHook(ctx context.Context, gitSourceID, secret string, events []string) (*GitDeploymentHook, error) {
	if s.db == nil {
		return nil, nil
	}

	id := uuid.NewString()
	now := time.Now().UTC()

	query := `
		INSERT INTO git_deployment_hooks (id, git_source_id, secret, events, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err := s.db.Exec(ctx, query, id, gitSourceID, secret, events, now, now)
	if err != nil {
		return nil, err
	}

	return &GitDeploymentHook{
		ID:          id,
		GitSourceID: gitSourceID,
		Secret:      secret,
		Events:      events,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (s *Store) DeleteGitDeploymentHook(ctx context.Context, hookID string) error {
	if s.db == nil {
		return nil
	}

	query := `DELETE FROM git_deployment_hooks WHERE id = $1`
	_, err := s.db.Exec(ctx, query, hookID)
	return err
}

func (s *Store) RegenerateGitDeploymentHookSecret(ctx context.Context, hookID, secret string) (*GitDeploymentHook, error) {
	if s.db == nil {
		return nil, nil
	}

	now := time.Now().UTC()
	query := `
		UPDATE git_deployment_hooks
		SET secret = $1, updated_at = $2
		WHERE id = $3
		RETURNING id, git_source_id, secret, events, created_at, updated_at
	`

	var hook GitDeploymentHook
	err := s.db.QueryRow(ctx, query, secret, now, hookID).Scan(
		&hook.ID, &hook.GitSourceID, &hook.Secret, &hook.Events, &hook.CreatedAt, &hook.UpdatedAt)
	if err != nil {
		return nil, err
	}

	return &hook, nil
}
