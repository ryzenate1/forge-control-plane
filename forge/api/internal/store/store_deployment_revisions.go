package store

import (
	"context"
	"encoding/json"
	"time"
)

type RevisionStatus string

const (
	RevisionStatusPending  RevisionStatus = "pending"
	RevisionStatusActive   RevisionStatus = "active"
	RevisionStatusSuperseded RevisionStatus = "superseded"
	RevisionStatusFailed   RevisionStatus = "failed"
)

type DeploymentRevision struct {
	ID                 string          `json:"id"`
	DeploymentID       string          `json:"deploymentId"`
	RevisionNumber     int             `json:"revisionNumber"`
	ImageRef           string          `json:"imageRef"`
	ComposeManifestRef string          `json:"composeManifestRef"`
	GitCommitSHA       string          `json:"gitCommitSha"`
	ConfigHash         string          `json:"configHash"`
	Status             string          `json:"status"`
	DeployedAt         *time.Time      `json:"deployedAt,omitempty"`
	Description        string          `json:"description"`
	Metadata           json.RawMessage `json:"metadata,omitempty"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}

func (s *Store) CreateDeploymentRevision(ctx context.Context, rev *DeploymentRevision) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO deployment_revisions (id, deployment_id, revision_number, image_ref, compose_manifest_ref,
			git_commit_sha, config_hash, status, deployed_at, description, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, rev.ID, rev.DeploymentID, rev.RevisionNumber, rev.ImageRef, rev.ComposeManifestRef,
		rev.GitCommitSHA, rev.ConfigHash, rev.Status, rev.DeployedAt, rev.Description, rev.Metadata, rev.CreatedAt, rev.UpdatedAt)
	return err
}

func (s *Store) ListDeploymentRevisions(ctx context.Context, deploymentID string) ([]DeploymentRevision, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, deployment_id::text, revision_number, image_ref, compose_manifest_ref,
			git_commit_sha, config_hash, status, deployed_at, description, COALESCE(metadata::text, '{}'),
			created_at, updated_at
		FROM deployment_revisions
		WHERE deployment_id = $1
		ORDER BY revision_number DESC
	`, deploymentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []DeploymentRevision
	for rows.Next() {
		var rev DeploymentRevision
		var metaStr string
		if err := rows.Scan(&rev.ID, &rev.DeploymentID, &rev.RevisionNumber, &rev.ImageRef,
			&rev.ComposeManifestRef, &rev.GitCommitSHA, &rev.ConfigHash, &rev.Status,
			&rev.DeployedAt, &rev.Description, &metaStr, &rev.CreatedAt, &rev.UpdatedAt); err != nil {
			return nil, err
		}
		rev.Metadata = json.RawMessage(metaStr)
		result = append(result, rev)
	}
	return result, rows.Err()
}

func (s *Store) GetDeploymentRevision(ctx context.Context, id string) (DeploymentRevision, error) {
	var rev DeploymentRevision
	var metaStr string
	err := s.db.QueryRow(ctx, `
		SELECT id::text, deployment_id::text, revision_number, image_ref, compose_manifest_ref,
			git_commit_sha, config_hash, status, deployed_at, description, COALESCE(metadata::text, '{}'),
			created_at, updated_at
		FROM deployment_revisions
		WHERE id = $1
	`, id).Scan(&rev.ID, &rev.DeploymentID, &rev.RevisionNumber, &rev.ImageRef,
		&rev.ComposeManifestRef, &rev.GitCommitSHA, &rev.ConfigHash, &rev.Status,
		&rev.DeployedAt, &rev.Description, &metaStr, &rev.CreatedAt, &rev.UpdatedAt)
	if err != nil {
		return rev, err
	}
	rev.Metadata = json.RawMessage(metaStr)
	return rev, nil
}

func (s *Store) GetPreviousDeploymentRevision(ctx context.Context, deploymentID string, currentRevNum int) (DeploymentRevision, error) {
	var rev DeploymentRevision
	var metaStr string
	err := s.db.QueryRow(ctx, `
		SELECT id::text, deployment_id::text, revision_number, image_ref, compose_manifest_ref,
			git_commit_sha, config_hash, status, deployed_at, description, COALESCE(metadata::text, '{}'),
			created_at, updated_at
		FROM deployment_revisions
		WHERE deployment_id = $1 AND revision_number < $2
		ORDER BY revision_number DESC
		LIMIT 1
	`, deploymentID, currentRevNum).Scan(&rev.ID, &rev.DeploymentID, &rev.RevisionNumber, &rev.ImageRef,
		&rev.ComposeManifestRef, &rev.GitCommitSHA, &rev.ConfigHash, &rev.Status,
		&rev.DeployedAt, &rev.Description, &metaStr, &rev.CreatedAt, &rev.UpdatedAt)
	if err != nil {
		return rev, err
	}
	rev.Metadata = json.RawMessage(metaStr)
	return rev, nil
}

func (s *Store) GetLatestDeploymentRevision(ctx context.Context, deploymentID string) (DeploymentRevision, error) {
	var rev DeploymentRevision
	var metaStr string
	err := s.db.QueryRow(ctx, `
		SELECT id::text, deployment_id::text, revision_number, image_ref, compose_manifest_ref,
			git_commit_sha, config_hash, status, deployed_at, description, COALESCE(metadata::text, '{}'),
			created_at, updated_at
		FROM deployment_revisions
		WHERE deployment_id = $1
		ORDER BY revision_number DESC
		LIMIT 1
	`, deploymentID).Scan(&rev.ID, &rev.DeploymentID, &rev.RevisionNumber, &rev.ImageRef,
		&rev.ComposeManifestRef, &rev.GitCommitSHA, &rev.ConfigHash, &rev.Status,
		&rev.DeployedAt, &rev.Description, &metaStr, &rev.CreatedAt, &rev.UpdatedAt)
	if err != nil {
		return rev, err
	}
	rev.Metadata = json.RawMessage(metaStr)
	return rev, nil
}

func (s *Store) UpdateDeploymentRevisionStatus(ctx context.Context, id string, status string, deployedAt *time.Time) error {
	_, err := s.db.Exec(ctx, `
		UPDATE deployment_revisions
		SET status = $2, deployed_at = $3, updated_at = now()
		WHERE id = $1
	`, id, status, deployedAt)
	return err
}

func (s *Store) SupersedeDeploymentRevisions(ctx context.Context, deploymentID string, exceptID string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE deployment_revisions
		SET status = 'superseded', updated_at = now()
		WHERE deployment_id = $1 AND id != $2 AND status = 'active'
	`, deploymentID, exceptID)
	return err
}

func (s *Store) UpdateDeploymentCurrentRevision(ctx context.Context, deploymentID string, revisionID *string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE deployments
		SET current_revision_id = $2, updated_at = now()
		WHERE id = $1
	`, deploymentID, revisionID)
	return err
}

func (s *Store) UpdateDeploymentRolloutStrategy(ctx context.Context, deploymentID string, strategy string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE deployments
		SET rollout_strategy = $2, updated_at = now()
		WHERE id = $1
	`, deploymentID, strategy)
	return err
}
