package git

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/google/uuid"
)

type DeploymentManagementService struct {
	store         *store.Store
	logger        *slog.Logger
	deployService *DeployService
}

func NewDeploymentManagementService(s *store.Store, logger *slog.Logger, deployService *DeployService) *DeploymentManagementService {
	return &DeploymentManagementService{
		store:         s,
		logger:        logger,
		deployService: deployService,
	}
}

type WebhookPayload struct {
	Ref        string `json:"ref"`
	Before     string `json:"before"`
	After      string `json:"after"`
	Repository struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
		SSHURL   string `json:"ssh_url"`
		Name     string `json:"name"`
	} `json:"repository"`
	Commits []struct {
		ID      string `json:"id"`
		Message string `json:"message"`
		Author  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
	} `json:"commits"`
}

func (s *DeploymentManagementService) GenerateSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *DeploymentManagementService) InitiateDeployment(ctx context.Context, repoURL, branch, commitHash string) (*store.GitDeployment, error) {
	source, err := s.store.FindGitSourceByRepoAndBranch(ctx, repoURL, branch)
	if err != nil || source == nil {
		return nil, fmt.Errorf("git source not found for %s @ %s", repoURL, branch)
	}

	d, err := s.store.CreateGitDeployment(ctx, store.CreateGitDeploymentRequest{
		GitSourceID: source.ID,
		CommitSHA:   commitHash,
		Branch:      branch,
		Status:      "pending",
	})
	if err != nil {
		return nil, fmt.Errorf("create deployment: %w", err)
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		logSb := new(strings.Builder)

		shortHash := commitHash
		if len(commitHash) >= 8 {
			shortHash = commitHash[:8]
		}
		logSb.WriteString(fmt.Sprintf("[%s] Starting deployment for %s @ %s\n", time.Now().UTC().Format(time.RFC3339), repoURL, shortHash))

		if err := s.store.UpdateGitDeployment(ctx, d.ID, "building", logSb.String(), ""); err != nil && s.logger != nil {
			s.logger.Error("update deployment status", "err", err)
		}

		if s.deployService != nil {
			deployReq := DeployFromGitRequest{
				GitSourceID: source.ID,
				ImageTag:    fmt.Sprintf("git-%s:%s", source.RepositoryName, shortHash),
			}
			if _, err := s.deployService.DeployFromGit(ctx, deployReq); err != nil {
				errMsg := fmt.Sprintf("deployment failed: %v", err)
				logSb.WriteString(fmt.Sprintf("[%s] %s\n", time.Now().UTC().Format(time.RFC3339), errMsg))
				_ = s.store.UpdateGitDeployment(ctx, d.ID, "failed", logSb.String(), errMsg)
				return
			}
		}

		logSb.WriteString(fmt.Sprintf("[%s] Deployment completed successfully\n", time.Now().UTC().Format(time.RFC3339)))
		_ = s.store.CompleteGitDeployment(ctx, d.ID, logSb.String())
	}()

	deploy, err := s.store.GetGitDeployment(ctx, d.ID)
	if err != nil {
		return nil, err
	}
	if deploy == nil {
		return nil, fmt.Errorf("created deployment not found: %s", d.ID)
	}
	return deploy, nil
}

func (s *DeploymentManagementService) HandleWebhookPayload(ctx context.Context, serverID, secret string, body []byte, signature string) error {
	if secret != "" {
		if err := verifyHMACSignature(body, signature, secret); err != nil {
			return err
		}
	}

	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("invalid webhook payload: %w", err)
	}

	branch := strings.TrimPrefix(payload.Ref, "refs/heads/")
	if branch == "" {
		return fmt.Errorf("could not determine branch from ref: %s", payload.Ref)
	}

	commitHash := payload.After
	if commitHash == "" || commitHash == "0000000000000000000000000000000000000000" {
		if len(payload.Commits) > 0 {
			commitHash = payload.Commits[len(payload.Commits)-1].ID
		}
	}
	if commitHash == "" {
		return fmt.Errorf("could not determine commit hash")
	}

	repoURL := payload.Repository.CloneURL
	if repoURL == "" {
		repoURL = payload.Repository.SSHURL
	}

	deploy, err := s.InitiateDeployment(ctx, repoURL, branch, commitHash)
	if err != nil {
		return fmt.Errorf("initiate deployment: %w", err)
	}

	if s.logger != nil {
		shortHash := commitHash
		if len(commitHash) >= 8 {
			shortHash = commitHash[:8]
		}
		s.logger.Info("git deployment triggered by webhook",
			"deployment_id", deploy.ID,
			"server_id", serverID,
			"repo", repoURL,
			"branch", branch,
			"commit", shortHash,
		)
	}

	return nil
}

var uuidNamespace = uuid.MustParse("6ba7b811-9dad-11d1-80b4-00c04fd430c8")

func verifyHMACSignature(payload []byte, signatureHeader, secret string) error {
	if signatureHeader == "" {
		return fmt.Errorf("missing signature header")
	}
	sig := signatureHeader
	if strings.HasPrefix(sig, "sha256=") {
		sig = strings.TrimPrefix(sig, "sha256=")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return fmt.Errorf("invalid webhook signature")
	}
	return nil
}
