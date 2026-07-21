package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type SourceDeployment struct {
	ID                      string     `json:"id"`
	ServerID                *string    `json:"serverId,omitempty"`
	GitProviderID           *string    `json:"gitProviderId,omitempty"`
	Repository              string     `json:"repository"`
	Branch                  string     `json:"branch"`
	BuildType               string     `json:"buildType"`
	BuildContext            string     `json:"buildContext"`
	DockerfilePath          string     `json:"dockerfilePath"`
	Status                  string     `json:"status"`
	CommitHash              string     `json:"commitHash"`
	CommitMessage           string     `json:"commitMessage"`
	CommitAuthor            string     `json:"commitAuthor"`
	ImageTag                string     `json:"imageTag"`
	Registry                string     `json:"registry"`
	RegistryCredentialID    string     `json:"registryCredentialId"`
	AutoDeploy              bool       `json:"autoDeploy"`
	WebhookID               string     `json:"webhookId"`
	WebhookURL              string     `json:"webhookUrl"`
	HealthCheckPath         string     `json:"healthCheckPath"`
	HealthCheckPort         int        `json:"healthCheckPort"`
	RollbackOnHealthFailure bool       `json:"rollbackOnHealthFailure"`
	CreatedBy               *string    `json:"createdBy,omitempty"`
	CreatedAt               time.Time  `json:"createdAt"`
	UpdatedAt               time.Time  `json:"updatedAt"`
}

type CreateSourceDeploymentRequest struct {
	ServerID                *string
	GitProviderID           *string
	Repository              string
	Branch                  string
	BuildType               string
	BuildContext            string
	DockerfilePath          string
	AutoDeploy              bool
	Registry                string
	RegistryCredentialID    string
	HealthCheckPath         string
	HealthCheckPort         int
	RollbackOnHealthFailure bool
	CreatedBy               *string
}

type UpdateSourceDeploymentRequest struct {
	ServerID                *string
	GitProviderID           *string
	Repository              *string
	Branch                  *string
	BuildType               *string
	BuildContext            *string
	DockerfilePath          *string
	AutoDeploy              *bool
	Registry                *string
	RegistryCredentialID    *string
	HealthCheckPath         *string
	HealthCheckPort         *int
	RollbackOnHealthFailure *bool
}

type BuildLog struct {
	ID           string          `json:"id"`
	DeploymentID string          `json:"deploymentId"`
	Stage        string          `json:"stage"`
	Message      string          `json:"message"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	CreatedAt    time.Time       `json:"createdAt"`
}

func (s *Store) ListSourceDeployments(ctx context.Context) ([]SourceDeployment, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, server_id::text, git_provider_id::text, repository, branch, build_type,
		       COALESCE(build_context, '.'), COALESCE(dockerfile_path, 'Dockerfile'), status,
		       COALESCE(commit_hash, ''), COALESCE(commit_message, ''), COALESCE(commit_author, ''),
		       COALESCE(image_tag, ''), COALESCE(registry, ''), COALESCE(registry_credential_id, ''),
		       auto_deploy, COALESCE(webhook_id, ''), COALESCE(webhook_url, ''),
		       COALESCE(health_check_path, '/'), COALESCE(health_check_port, 80),
		       rollback_on_health_failure,
		       created_by::text, created_at, updated_at
		FROM source_deployments
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []SourceDeployment
	for rows.Next() {
		var d SourceDeployment
		if err := rows.Scan(&d.ID, &d.ServerID, &d.GitProviderID, &d.Repository, &d.Branch, &d.BuildType,
			&d.BuildContext, &d.DockerfilePath, &d.Status,
			&d.CommitHash, &d.CommitMessage, &d.CommitAuthor,
			&d.ImageTag, &d.Registry, &d.RegistryCredentialID,
			&d.AutoDeploy, &d.WebhookID, &d.WebhookURL,
			&d.HealthCheckPath, &d.HealthCheckPort,
			&d.RollbackOnHealthFailure,
			&d.CreatedBy, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		deps = append(deps, d)
	}
	return deps, rows.Err()
}

func (s *Store) GetSourceDeployment(ctx context.Context, id string) (SourceDeployment, error) {
	var d SourceDeployment
	err := s.db.QueryRow(ctx, `
		SELECT id, server_id::text, git_provider_id::text, repository, branch, build_type,
		       COALESCE(build_context, '.'), COALESCE(dockerfile_path, 'Dockerfile'), status,
		       COALESCE(commit_hash, ''), COALESCE(commit_message, ''), COALESCE(commit_author, ''),
		       COALESCE(image_tag, ''), COALESCE(registry, ''), COALESCE(registry_credential_id, ''),
		       auto_deploy, COALESCE(webhook_id, ''), COALESCE(webhook_url, ''),
		       COALESCE(health_check_path, '/'), COALESCE(health_check_port, 80),
		       rollback_on_health_failure,
		       created_by::text, created_at, updated_at
		FROM source_deployments WHERE id = $1
	`, id).Scan(&d.ID, &d.ServerID, &d.GitProviderID, &d.Repository, &d.Branch, &d.BuildType,
		&d.BuildContext, &d.DockerfilePath, &d.Status,
		&d.CommitHash, &d.CommitMessage, &d.CommitAuthor,
		&d.ImageTag, &d.Registry, &d.RegistryCredentialID,
		&d.AutoDeploy, &d.WebhookID, &d.WebhookURL,
		&d.HealthCheckPath, &d.HealthCheckPort,
		&d.RollbackOnHealthFailure,
		&d.CreatedBy, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return SourceDeployment{}, errors.New("source deployment not found")
	}
	return d, nil
}

func (s *Store) CreateSourceDeployment(ctx context.Context, req CreateSourceDeploymentRequest) (SourceDeployment, error) {
	if strings.TrimSpace(req.Repository) == "" {
		return SourceDeployment{}, errors.New("repository is required")
	}
	if req.Branch == "" {
		req.Branch = "main"
	}
	if req.BuildType == "" {
		req.BuildType = "dockerfile"
	}
	if req.BuildContext == "" {
		req.BuildContext = "."
	}
	if req.DockerfilePath == "" {
		req.DockerfilePath = "Dockerfile"
	}
	if req.HealthCheckPath == "" {
		req.HealthCheckPath = "/"
	}
	if req.HealthCheckPort == 0 {
		req.HealthCheckPort = 80
	}

	id := uuid.NewString()
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO source_deployments (
			id, server_id, git_provider_id, repository, branch, build_type,
			build_context, dockerfile_path, status,
			auto_deploy, registry, registry_credential_id,
			health_check_path, health_check_port, rollback_on_health_failure,
			created_by, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending',
		          $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`, id, req.ServerID, req.GitProviderID, req.Repository, req.Branch, req.BuildType,
		req.BuildContext, req.DockerfilePath,
		req.AutoDeploy, req.Registry, req.RegistryCredentialID,
		req.HealthCheckPath, req.HealthCheckPort, req.RollbackOnHealthFailure,
		req.CreatedBy, now, now)
	if err != nil {
		return SourceDeployment{}, err
	}
	return s.GetSourceDeployment(ctx, id)
}

func (s *Store) UpdateSourceDeployment(ctx context.Context, id string, req UpdateSourceDeploymentRequest) (SourceDeployment, error) {
	sets := []string{}
	args := []any{}
	add := func(column string, value any) {
		args = append(args, value)
		sets = append(sets, column+" = $"+fmt.Sprintf("%d", len(args)))
	}

	if req.ServerID != nil {
		add("server_id", *req.ServerID)
	}
	if req.GitProviderID != nil {
		add("git_provider_id", *req.GitProviderID)
	}
	if req.Repository != nil {
		add("repository", *req.Repository)
	}
	if req.Branch != nil {
		add("branch", *req.Branch)
	}
	if req.BuildType != nil {
		add("build_type", *req.BuildType)
	}
	if req.BuildContext != nil {
		add("build_context", *req.BuildContext)
	}
	if req.DockerfilePath != nil {
		add("dockerfile_path", *req.DockerfilePath)
	}
	if req.AutoDeploy != nil {
		add("auto_deploy", *req.AutoDeploy)
	}
	if req.Registry != nil {
		add("registry", *req.Registry)
	}
	if req.RegistryCredentialID != nil {
		add("registry_credential_id", *req.RegistryCredentialID)
	}
	if req.HealthCheckPath != nil {
		add("health_check_path", *req.HealthCheckPath)
	}
	if req.HealthCheckPort != nil {
		add("health_check_port", *req.HealthCheckPort)
	}
	if req.RollbackOnHealthFailure != nil {
		add("rollback_on_health_failure", *req.RollbackOnHealthFailure)
	}

	if len(sets) == 0 {
		return s.GetSourceDeployment(ctx, id)
	}

	add("updated_at", time.Now().UTC())
	args = append(args, id)
	_, err := s.db.Exec(ctx, "UPDATE source_deployments SET "+strings.Join(sets, ", ")+
		fmt.Sprintf(" WHERE id = $%d", len(args)), args...)
	if err != nil {
		return SourceDeployment{}, err
	}
	return s.GetSourceDeployment(ctx, id)
}

func (s *Store) UpdateSourceDeploymentStatus(ctx context.Context, id, status string) error {
	_, err := s.db.Exec(ctx, `UPDATE source_deployments SET status = $1, updated_at = NOW() WHERE id = $2`, status, id)
	return err
}

func (s *Store) UpdateSourceDeploymentCommit(ctx context.Context, id, hash, message, author string) error {
	_, err := s.db.Exec(ctx, `UPDATE source_deployments SET commit_hash = $1, commit_message = $2, commit_author = $3, updated_at = NOW() WHERE id = $4`, hash, message, author, id)
	return err
}

func (s *Store) UpdateSourceDeploymentImage(ctx context.Context, id, imageTag string) error {
	_, err := s.db.Exec(ctx, `UPDATE source_deployments SET image_tag = $1, updated_at = NOW() WHERE id = $2`, imageTag, id)
	return err
}

func (s *Store) DeleteSourceDeployment(ctx context.Context, id string) error {
	cmd, err := s.db.Exec(ctx, `DELETE FROM source_deployments WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("source deployment not found")
	}
	return nil
}

func (s *Store) CreateDeploymentBuildLog(ctx context.Context, deploymentID, stage, message string) (BuildLog, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		INSERT INTO build_logs (id, deployment_id, stage, message, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, id, deploymentID, stage, message, now)
	if err != nil {
		return BuildLog{}, err
	}
	return BuildLog{
		ID:           id,
		DeploymentID: deploymentID,
		Stage:        stage,
		Message:      message,
		CreatedAt:    now,
	}, nil
}

func (s *Store) ListDeploymentBuildLogs(ctx context.Context, deploymentID string) ([]BuildLog, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, deployment_id, stage, message, COALESCE(metadata, '{}'::jsonb), created_at
		FROM build_logs WHERE deployment_id = $1 ORDER BY created_at ASC
	`, deploymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []BuildLog
	for rows.Next() {
		var l BuildLog
		if err := rows.Scan(&l.ID, &l.DeploymentID, &l.Stage, &l.Message, &l.Metadata, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (s *Store) DeleteBuildLogs(ctx context.Context, deploymentID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM build_logs WHERE deployment_id = $1`, deploymentID)
	return err
}
