package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type GitSource struct {
	ID               string          `json:"id"`
	UserID           string          `json:"userId"`
	CredentialID     *string         `json:"credentialId,omitempty"`
	ProviderTokenID  *string         `json:"providerTokenId,omitempty"`
	Provider         string          `json:"provider"`
	RepositoryURL    string          `json:"repositoryUrl"`
	RepositoryName   string          `json:"repositoryName"`
	RepositoryOwner  string          `json:"repositoryOwner"`
	Branch           string          `json:"branch"`
	AutoDeploy       bool            `json:"autoDeploy"`
	WebhookSecret    string          `json:"webhookSecret,omitempty"`
	WebhookID        string          `json:"webhookId"`
	WebhookURL       string          `json:"webhookUrl"`
	LastCommitSHA    string          `json:"lastCommitSha"`
	LastCommitMsg    string          `json:"lastCommitMessage"`
	LastCommitAuthor string          `json:"lastCommitAuthor"`
	LastDeployedAt   *time.Time      `json:"lastDeployedAt,omitempty"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}

type CreateGitSourceRequest struct {
	UserID          string
	CredentialID    *string
	ProviderTokenID *string
	Provider        string
	RepositoryURL   string
	RepositoryName  string
	RepositoryOwner string
	Branch          string
	AutoDeploy      bool
	WebhookSecret   string
	WebhookID       string
	WebhookURL      string
}

func (s *Store) ListGitSources(ctx context.Context, userID string) ([]GitSource, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, user_id, credential_id, provider_token_id, COALESCE(provider,''),
		       repository_url, COALESCE(repository_name,''), COALESCE(repository_owner,''),
		       COALESCE(branch,'main'), auto_deploy,
		       (COALESCE(webhook_secret_plaintext,'') <> '' OR COALESCE(webhook_secret_encrypted,'') <> ''),
		       COALESCE(webhook_id,''), COALESCE(webhook_url,''),
		       COALESCE(last_commit_sha,''), COALESCE(last_commit_message,''), COALESCE(last_commit_author,''),
		       last_deployed_at, created_at, updated_at
		FROM git_sources WHERE user_id = $1 ORDER BY repository_name
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sources := []GitSource{}
	for rows.Next() {
		var gs GitSource
		var hasSecret bool
		if err := rows.Scan(&gs.ID, &gs.UserID, &gs.CredentialID, &gs.ProviderTokenID, &gs.Provider,
			&gs.RepositoryURL, &gs.RepositoryName, &gs.RepositoryOwner,
			&gs.Branch, &gs.AutoDeploy, &hasSecret,
			&gs.WebhookID, &gs.WebhookURL,
			&gs.LastCommitSHA, &gs.LastCommitMsg, &gs.LastCommitAuthor,
			&gs.LastDeployedAt, &gs.CreatedAt, &gs.UpdatedAt); err != nil {
			return nil, err
		}
		if hasSecret {
			gs.WebhookSecret = maskedStoreSecret
		}
		sources = append(sources, gs)
	}
	return sources, rows.Err()
}

func (s *Store) getGitSourceInternal(ctx context.Context, id string) (GitSource, error) {
	var gs GitSource
	var webhookPlain, webhookEncrypted string
	err := s.db.QueryRow(ctx, `
		SELECT id, user_id, credential_id, provider_token_id, COALESCE(provider,''),
		       repository_url, COALESCE(repository_name,''), COALESCE(repository_owner,''),
		       COALESCE(branch,'main'), auto_deploy,
		       COALESCE(webhook_secret_plaintext,''), COALESCE(webhook_secret_encrypted,''),
		       COALESCE(webhook_id,''), COALESCE(webhook_url,''),
		       COALESCE(last_commit_sha,''), COALESCE(last_commit_message,''), COALESCE(last_commit_author,''),
		       last_deployed_at, created_at, updated_at
		FROM git_sources WHERE id = $1
	`, id).Scan(&gs.ID, &gs.UserID, &gs.CredentialID, &gs.ProviderTokenID, &gs.Provider,
		&gs.RepositoryURL, &gs.RepositoryName, &gs.RepositoryOwner,
		&gs.Branch, &gs.AutoDeploy,
		&webhookPlain, &webhookEncrypted,
		&gs.WebhookID, &gs.WebhookURL,
		&gs.LastCommitSHA, &gs.LastCommitMsg, &gs.LastCommitAuthor,
		&gs.LastDeployedAt, &gs.CreatedAt, &gs.UpdatedAt)
	if err != nil {
		return GitSource{}, errors.New("git source not found")
	}
	gs.WebhookSecret, err = s.decryptSecret(webhookEncrypted, webhookPlain, secretAAD("git_sources", gs.ID, "webhook_secret"))
	if err != nil {
		return GitSource{}, err
	}
	return gs, nil
}

func (s *Store) GetGitSource(ctx context.Context, id string) (GitSource, error) {
	gs, err := s.getGitSourceInternal(ctx, id)
	if err != nil {
		return GitSource{}, err
	}
	if gs.WebhookSecret != "" {
		gs.WebhookSecret = maskedStoreSecret
	}
	return gs, nil
}

func (s *Store) CreateGitSource(ctx context.Context, req CreateGitSourceRequest) (GitSource, error) {
	if strings.TrimSpace(req.RepositoryURL) == "" {
		return GitSource{}, errors.New("repositoryUrl is required")
	}
	if req.Branch == "" {
		req.Branch = "main"
	}

	id := uuid.NewString()
	webhookEncrypted, err := s.encryptSecret(req.WebhookSecret, secretAAD("git_sources", id, "webhook_secret"))
	if err != nil {
		return GitSource{}, err
	}

	now := time.Now().UTC()
	if _, err := s.db.Exec(ctx, `
		INSERT INTO git_sources (id, user_id, credential_id, provider_token_id, provider,
			repository_url, repository_name, repository_owner, branch, auto_deploy,
			webhook_secret_encrypted, webhook_secret_plaintext,
			webhook_id, webhook_url, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, '', $12, $13, $14, $15)
	`, id, req.UserID, req.CredentialID, req.ProviderTokenID, req.Provider,
		req.RepositoryURL, req.RepositoryName, req.RepositoryOwner, req.Branch, req.AutoDeploy,
		webhookEncrypted, req.WebhookID, req.WebhookURL, now, now); err != nil {
		return GitSource{}, err
	}
	return s.GetGitSource(ctx, id)
}

func (s *Store) UpdateGitSourceDeploy(ctx context.Context, id, sha, message, author string) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(ctx, `
		UPDATE git_sources SET last_commit_sha = $1, last_commit_message = $2, last_commit_author = $3, last_deployed_at = $4, updated_at = $5 WHERE id = $6
	`, sha, message, author, now, now, id)
	return err
}

func (s *Store) FindGitSourceByRepoAndBranch(ctx context.Context, repoURL, branch string) (*GitSource, error) {
	var gs GitSource
	var webhookPlain, webhookEncrypted string
	err := s.db.QueryRow(ctx, `
		SELECT id, user_id, credential_id, provider_token_id, COALESCE(provider,''),
		       repository_url, COALESCE(repository_name,''), COALESCE(repository_owner,''),
		       COALESCE(branch,'main'), auto_deploy,
		       COALESCE(webhook_secret_plaintext,''), COALESCE(webhook_secret_encrypted,''),
		       COALESCE(webhook_id,''), COALESCE(webhook_url,''),
		       COALESCE(last_commit_sha,''), COALESCE(last_commit_message,''), COALESCE(last_commit_author,''),
		       last_deployed_at, created_at, updated_at
		FROM git_sources WHERE repository_url = $1 AND branch = $2 AND auto_deploy = true
		LIMIT 1
	`, repoURL, branch).Scan(&gs.ID, &gs.UserID, &gs.CredentialID, &gs.ProviderTokenID, &gs.Provider,
		&gs.RepositoryURL, &gs.RepositoryName, &gs.RepositoryOwner,
		&gs.Branch, &gs.AutoDeploy,
		&webhookPlain, &webhookEncrypted,
		&gs.WebhookID, &gs.WebhookURL,
		&gs.LastCommitSHA, &gs.LastCommitMsg, &gs.LastCommitAuthor,
		&gs.LastDeployedAt, &gs.CreatedAt, &gs.UpdatedAt)
	if err != nil {
		return nil, err
	}
	gs.WebhookSecret, err = s.decryptSecret(webhookEncrypted, webhookPlain, secretAAD("git_sources", gs.ID, "webhook_secret"))
	if err != nil {
		return nil, err
	}
	return &gs, nil
}

// GetGitSourceByServerID resolves a server ID to the associated GitSource.
// It looks up the server, then checks docker_labels for a stored "git_source_id"
// key. If found, returns that GitSource. Otherwise falls back to looking up the
// ID directly as a git source ID (to support frontends that already pass
// git source IDs through this route).
func (s *Store) GetGitSourceByServerID(ctx context.Context, serverID string) (GitSource, error) {
	server, err := s.GetServer(ctx, serverID)
	if err != nil {
		return GitSource{}, err
	}
	if gsID, ok := server.DockerLabels["git_source_id"]; ok && gsID != "" {
		return s.GetGitSource(ctx, gsID)
	}
	return s.GetGitSource(ctx, serverID)
}

func (s *Store) DeleteGitSource(ctx context.Context, id string) error {
	cmd, err := s.db.Exec(ctx, `DELETE FROM git_sources WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("git source not found")
	}
	return nil
}

func MatchesBaseURL(repoURL, targetHost string) bool {
	normalized := strings.TrimSuffix(strings.TrimSuffix(repoURL, ".git"), "/")
	normalized = strings.ToLower(normalized)
	targetHost = strings.ToLower(strings.TrimSuffix(targetHost, "/"))
	return strings.HasPrefix(normalized, targetHost)
}

type GitProviderRepo struct {
	Name          string `json:"name"`
	FullName      string `json:"fullName"`
	CloneURL      string `json:"cloneUrl"`
	SSHURL        string `json:"sshUrl"`
	DefaultBranch string `json:"defaultBranch"`
	Private       bool   `json:"private"`
	Description   string `json:"description"`
}

type GitProviderBranch struct {
	Name   string `json:"name"`
	SHA    string `json:"sha"`
	IsMain bool   `json:"isMain"`
}

type GitProviderCommit struct {
	SHA     string          `json:"sha"`
	Message string          `json:"message"`
	Author  string          `json:"author"`
	Date    *time.Time      `json:"date,omitempty"`
	Raw     json.RawMessage `json:"raw,omitempty"`
}
