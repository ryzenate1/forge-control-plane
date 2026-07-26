package git

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gamepanel/forge/internal/store"
)

// DeploymentStatus represents the current status of a Git deployment
type DeploymentStatus string

const (
	DeploymentStatusPending   DeploymentStatus = "pending"
	DeploymentStatusBuilding  DeploymentStatus = "building"
	DeploymentStatusDeploying DeploymentStatus = "deploying"
	DeploymentStatusCompleted DeploymentStatus = "completed"
	DeploymentStatusFailed    DeploymentStatus = "failed"
	DeploymentStatusCancelled DeploymentStatus = "cancelled"
)

// GitDeployment represents a Git-based deployment
type GitDeployment struct {
	ID            string           `json:"id"`
	GitSourceID   string           `json:"gitSourceId"`
	CommitSHA     string           `json:"commitSha"`
	Branch        string           `json:"branch"`
	Status        DeploymentStatus `json:"status"`
	StatusMessage string           `json:"statusMessage,omitempty"`
	ImageTag      string           `json:"imageTag,omitempty"`
	BuildLog      string           `json:"buildLog,omitempty"`
	DeployLog     string           `json:"deployLog,omitempty"`
	Error         string           `json:"error,omitempty"`
	StartedAt     time.Time        `json:"startedAt"`
	CompletedAt   *time.Time       `json:"completedAt,omitempty"`
	CreatedAt     time.Time        `json:"createdAt"`
	UpdatedAt     time.Time        `json:"updatedAt"`
}

// GitDeploymentService provides Git-based deployment functionality
type GitDeploymentService struct {
	store          *store.Store
	gitService     *Service
	deployService  *DeployService
	logger         *slog.Logger
	buildService   BuildServiceInterface
	composeService ComposeServiceInterface
}

// BuildServiceInterface defines the interface for build service integration
type BuildServiceInterface interface {
	CreateBuild(ctx context.Context, req interface{}) error
	GetBuildStatus(ctx context.Context, buildID string) (string, error)
}

// ComposeServiceInterface defines the interface for compose service integration.
// It is implemented by the compose package's GitDeployAdapter, which routes
// deployments through the live GitOpsService compose deploy path.
type ComposeServiceInterface interface {
	// ValidateComposeContent returns a non-nil error when the compose
	// content fails validation.
	ValidateComposeContent(content []byte, workingDir string) error
	// DeployComposeFromGit performs a real compose deployment for the
	// cloned repository and returns a human-readable deploy log.
	DeployComposeFromGit(ctx context.Context, req ComposeGitDeployRequest) (string, error)
}

// ComposeGitDeployRequest carries everything a compose deployer needs to turn
// a cloned git compose project into a running stack.
type ComposeGitDeployRequest struct {
	GitSourceID    string
	RepositoryURL  string
	RepositoryName string
	Branch         string
	CommitSHA      string
	CloneDir       string
	ComposeContent []byte
	CredentialID   string
	NodeID         string
	UserID         string
}

// NewGitDeploymentService creates a new Git deployment service
func NewGitDeploymentService(
	store *store.Store,
	gitService *Service,
	deployService *DeployService,
	logger *slog.Logger,
	buildService BuildServiceInterface,
	composeService ComposeServiceInterface,
) *GitDeploymentService {
	if logger == nil {
		logger = slog.Default()
	}
	return &GitDeploymentService{
		store:          store,
		gitService:     gitService,
		deployService:  deployService,
		logger:         logger,
		buildService:   buildService,
		composeService: composeService,
	}
}

// DeployRequest represents a request to deploy from a Git source
type DeployRequest struct {
	GitSourceID    string            `json:"gitSourceId"`
	CommitSHA      string            `json:"commitSha,omitempty"`
	Branch         string            `json:"branch,omitempty"`
	DockerfilePath string            `json:"dockerfilePath,omitempty"`
	BuildArgs      map[string]string `json:"buildArgs,omitempty"`
	ImageTag       string            `json:"imageTag,omitempty"`
	ForceRebuild   bool              `json:"forceRebuild,omitempty"`
	// NodeID selects the Beacon node for compose deployments. When empty the
	// compose deployer falls back to its configured node resolution.
	NodeID string `json:"nodeId,omitempty"`
	// UserID attributes the resulting compose stack to a panel user.
	UserID string `json:"userId,omitempty"`
}

// DeployResult represents the result of a Git deployment
type DeployResult struct {
	Deployment *GitDeployment   `json:"deployment"`
	GitSource  *store.GitSource `json:"gitSource,omitempty"`
	ImageTag   string           `json:"imageTag,omitempty"`
	CommitSHA  string           `json:"commitSha,omitempty"`
	Status     DeploymentStatus `json:"status"`
	Error      string           `json:"error,omitempty"`
}

// TriggerDeployment triggers a deployment from a Git source
func (s *GitDeploymentService) TriggerDeployment(ctx context.Context, req *DeployRequest) (*DeployResult, error) {
	if req == nil {
		return nil, fmt.Errorf("deploy request cannot be nil")
	}

	if req.GitSourceID == "" {
		return nil, fmt.Errorf("gitSourceId is required")
	}

	s.logger.Info("Triggering Git deployment", "gitSourceId", req.GitSourceID, "commitSHA", req.CommitSHA)

	// Get the Git source
	gitSource, err := s.store.GetGitSource(ctx, req.GitSourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get git source: %w", err)
	}

	// Use the provided commit SHA or fall back to the latest
	commitSHA := req.CommitSHA
	if commitSHA == "" {
		commitSHA = gitSource.LastCommitSHA
	}

	// Use the provided branch or fall back to the configured branch
	branch := req.Branch
	if branch == "" {
		branch = gitSource.Branch
		if branch == "" {
			branch = "main"
		}
	}

	// Create a deployment record
	deployment := &GitDeployment{
		ID:            "", // Will be generated
		GitSourceID:   req.GitSourceID,
		CommitSHA:     commitSHA,
		Branch:        branch,
		Status:        DeploymentStatusPending,
		StatusMessage: "Deployment queued",
		StartedAt:     time.Now().UTC(),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	// Save the deployment record
	if s.store != nil {
		created, cerr := s.store.CreateGitDeployment(ctx, store.CreateGitDeploymentRequest{
			GitSourceID:   deployment.GitSourceID,
			CommitSHA:     deployment.CommitSHA,
			Branch:        deployment.Branch,
			Status:        string(deployment.Status),
			StatusMessage: deployment.StatusMessage,
			StartedAt:     deployment.StartedAt,
		})
		if cerr != nil {
			return nil, fmt.Errorf("failed to persist deployment record: %w", cerr)
		}
		if created != nil {
			deployment.ID = created.ID
		}
		s.logger.Info("Created deployment record", "deploymentId", deployment.ID)
	}

	// Clone the repository
	credentialID := ""
	if gitSource.CredentialID != nil {
		credentialID = *gitSource.CredentialID
	}
	cloneResult, err := s.deployService.CloneRepo(ctx, gitSource.RepositoryURL, branch, gitSource.ID, credentialID)
	if err != nil {
		deployment.Status = DeploymentStatusFailed
		deployment.Error = fmt.Sprintf("failed to clone repository: %v", err)
		deployment.UpdatedAt = time.Now().UTC()
		s.persistDeploymentState(ctx, deployment)
		return &DeployResult{
			Deployment: deployment,
			GitSource:  &gitSource,
			Status:     DeploymentStatusFailed,
			Error:      deployment.Error,
		}, fmt.Errorf("failed to clone repository: %w", err)
	}
	defer s.deployService.CleanupClone(cloneResult.Dir)

	// Check if this is a Compose project
	if cloneResult.ProjectType == "compose" {
		return s.handleComposeDeployment(ctx, req, deployment, gitSource, cloneResult)
	}

	// Handle Dockerfile project
	return s.handleDockerfileDeployment(ctx, req, deployment, gitSource, cloneResult)
}

// persistDeploymentState mirrors in-memory deployment state to the store.
func (s *GitDeploymentService) persistDeploymentState(ctx context.Context, d *GitDeployment) {
	if s.store == nil || d == nil || d.ID == "" {
		return
	}
	var err error
	switch d.Status {
	case DeploymentStatusFailed:
		err = s.store.FailGitDeployment(ctx, d.ID, d.Error)
	case DeploymentStatusCompleted:
		if d.ImageTag != "" {
			if uerr := s.store.UpdateGitDeploymentWithImage(ctx, d.ID, d.ImageTag, d.BuildLog); uerr != nil {
				s.logger.Warn("Failed to persist deployment image", "deploymentId", d.ID, "error", uerr)
			}
		}
		err = s.store.CompleteGitDeployment(ctx, d.ID, d.DeployLog)
	default:
		err = s.store.UpdateGitDeployment(ctx, d.ID, string(d.Status), d.StatusMessage, d.Error)
	}
	if err != nil {
		s.logger.Warn("Failed to persist deployment state", "deploymentId", d.ID, "status", d.Status, "error", err)
	}
}

// handleComposeDeployment handles deployment for Compose projects
func (s *GitDeploymentService) handleComposeDeployment(
	ctx context.Context,
	req *DeployRequest,
	deployment *GitDeployment,
	gitSource store.GitSource,
	cloneResult *CloneResult,
) (*DeployResult, error) {
	// Read the Compose file
	composeContent, err := os.ReadFile(filepath.Join(cloneResult.Dir, "docker-compose.yml"))
	if err != nil {
		// Try other common Compose filenames
		composeFiles := []string{"compose.yml", "docker-compose.yaml", "compose.yaml"}
		for _, filename := range composeFiles {
			composeContent, err = os.ReadFile(filepath.Join(cloneResult.Dir, filename))
			if err == nil {
				break
			}
		}
		if err != nil {
			deployment.Status = DeploymentStatusFailed
			deployment.Error = fmt.Sprintf("no Compose file found: %v", err)
			deployment.UpdatedAt = time.Now().UTC()
			s.persistDeploymentState(ctx, deployment)
			return &DeployResult{
				Deployment: deployment,
				GitSource:  &gitSource,
				Status:     DeploymentStatusFailed,
				Error:      deployment.Error,
			}, fmt.Errorf("no Compose file found: %w", err)
		}
	}

	// Validate the Compose file and fail the deployment on validation errors.
	if s.composeService != nil {
		if verr := s.composeService.ValidateComposeContent(composeContent, cloneResult.Dir); verr != nil {
			deployment.Status = DeploymentStatusFailed
			deployment.Error = fmt.Sprintf("compose validation failed: %v", verr)
			deployment.UpdatedAt = time.Now().UTC()
			s.persistDeploymentState(ctx, deployment)
			return &DeployResult{
				Deployment: deployment,
				GitSource:  &gitSource,
				Status:     DeploymentStatusFailed,
				Error:      deployment.Error,
			}, fmt.Errorf("compose validation failed: %w", verr)
		}
	}

	// A compose deployer is required for a real deployment. Fail honestly
	// instead of reporting success for work that never happened.
	if s.composeService == nil {
		deployment.Status = DeploymentStatusFailed
		deployment.Error = "compose deployments are not configured for this service; deploy git-backed compose stacks through the compose git deploy endpoint instead"
		deployment.UpdatedAt = time.Now().UTC()
		s.persistDeploymentState(ctx, deployment)
		return &DeployResult{
			Deployment: deployment,
			GitSource:  &gitSource,
			Status:     DeploymentStatusFailed,
			Error:      deployment.Error,
		}, fmt.Errorf("compose deployer not configured")
	}

	deployment.Status = DeploymentStatusDeploying
	deployment.StatusMessage = "Deploying compose stack"
	deployment.UpdatedAt = time.Now().UTC()
	s.persistDeploymentState(ctx, deployment)

	credentialID := ""
	if gitSource.CredentialID != nil {
		credentialID = *gitSource.CredentialID
	}
	deployLog, err := s.composeService.DeployComposeFromGit(ctx, ComposeGitDeployRequest{
		GitSourceID:    gitSource.ID,
		RepositoryURL:  gitSource.RepositoryURL,
		RepositoryName: gitSource.RepositoryName,
		Branch:         cloneResult.Branch,
		CommitSHA:      cloneResult.CommitSHA,
		CloneDir:       cloneResult.Dir,
		ComposeContent: composeContent,
		CredentialID:   credentialID,
		NodeID:         req.NodeID,
		UserID:         req.UserID,
	})
	if err != nil {
		deployment.Status = DeploymentStatusFailed
		deployment.Error = fmt.Sprintf("compose deploy failed: %v", err)
		deployment.UpdatedAt = time.Now().UTC()
		s.persistDeploymentState(ctx, deployment)
		return &DeployResult{
			Deployment: deployment,
			GitSource:  &gitSource,
			Status:     DeploymentStatusFailed,
			Error:      deployment.Error,
		}, fmt.Errorf("compose deploy failed: %w", err)
	}

	deployment.Status = DeploymentStatusCompleted
	deployment.StatusMessage = "Compose stack deployed successfully"
	deployment.DeployLog = deployLog
	deployment.CompletedAt = timePtr(time.Now().UTC())
	deployment.UpdatedAt = time.Now().UTC()
	s.persistDeploymentState(ctx, deployment)

	if err := s.store.UpdateGitSourceDeploy(ctx, gitSource.ID, cloneResult.CommitSHA, "", ""); err != nil {
		s.logger.Warn("Failed to update git source deploy info", "error", err)
	}

	return &DeployResult{
		Deployment: deployment,
		GitSource:  &gitSource,
		CommitSHA:  cloneResult.CommitSHA,
		Status:     DeploymentStatusCompleted,
	}, nil
}

// handleDockerfileDeployment handles deployment for Dockerfile projects
func (s *GitDeploymentService) handleDockerfileDeployment(
	ctx context.Context,
	req *DeployRequest,
	deployment *GitDeployment,
	gitSource store.GitSource,
	cloneResult *CloneResult,
) (*DeployResult, error) {
	// Update deployment status
	deployment.Status = DeploymentStatusBuilding
	deployment.StatusMessage = "Building Docker image"
	deployment.UpdatedAt = time.Now().UTC()
	s.persistDeploymentState(ctx, deployment)

	// Build the Docker image
	dockerfilePath := req.DockerfilePath
	if dockerfilePath == "" {
		dockerfilePath = filepath.Join(cloneResult.Dir, "Dockerfile")
	}

	imageTag := req.ImageTag
	if imageTag == "" {
		// Generate a unique image tag
		imageTag = fmt.Sprintf("git-%s:%s", strings.ReplaceAll(gitSource.RepositoryName, "/", "-"), cloneResult.CommitSHA[:8])
	}

	// Use the deploy service to build and push
	deployReq := DeployFromGitRequest{
		GitSourceID:    gitSource.ID,
		ImageTag:       imageTag,
		DockerfilePath: dockerfilePath,
		BuildArgs:      req.BuildArgs,
	}

	_, err := s.deployService.DeployFromGit(ctx, deployReq)
	if err != nil {
		deployment.Status = DeploymentStatusFailed
		deployment.Error = fmt.Sprintf("failed to build and deploy: %v", err)
		deployment.UpdatedAt = time.Now().UTC()
		s.persistDeploymentState(ctx, deployment)
		return &DeployResult{
			Deployment: deployment,
			GitSource:  &gitSource,
			Status:     DeploymentStatusFailed,
			Error:      deployment.Error,
		}, fmt.Errorf("failed to build and deploy: %w", err)
	}

	// Update deployment status
	deployment.Status = DeploymentStatusCompleted
	deployment.StatusMessage = "Deployment completed successfully"
	deployment.ImageTag = imageTag
	deployment.CompletedAt = timePtr(time.Now().UTC())
	deployment.UpdatedAt = time.Now().UTC()
	s.persistDeploymentState(ctx, deployment)

	// Update the Git source with the latest deployment info
	if err := s.store.UpdateGitSourceDeploy(ctx, gitSource.ID, cloneResult.CommitSHA, "", ""); err != nil {
		s.logger.Warn("Failed to update git source deploy info", "error", err)
	}

	return &DeployResult{
		Deployment: deployment,
		GitSource:  &gitSource,
		ImageTag:   imageTag,
		CommitSHA:  cloneResult.CommitSHA,
		Status:     DeploymentStatusCompleted,
	}, nil
}

// HandleWebhookEvent handles a Git webhook event and triggers deployment if configured
func (s *GitDeploymentService) HandleWebhookEvent(
	ctx context.Context,
	gitSource *store.GitSource,
	commitSHA, commitMsg, commitAuthor string,
) error {
	if gitSource == nil {
		return fmt.Errorf("git source cannot be nil")
	}

	s.logger.Info("Handling webhook event",
		"gitSourceId", gitSource.ID,
		"repository", gitSource.RepositoryURL,
		"commitSHA", commitSHA,
		"branch", gitSource.Branch)

	// Check if auto-deploy is enabled
	if !gitSource.AutoDeploy {
		s.logger.Info("Auto-deploy is disabled for this source, skipping deployment")
		// Update the last commit info but don't deploy
		return s.store.UpdateGitSourceDeploy(ctx, gitSource.ID, commitSHA, commitMsg, commitAuthor)
	}

	// Create a deployment request
	deployReq := &DeployRequest{
		GitSourceID: gitSource.ID,
		CommitSHA:   commitSHA,
		Branch:      gitSource.Branch,
	}

	// Trigger the deployment
	_, err := s.TriggerDeployment(ctx, deployReq)
	if err != nil {
		s.logger.Error("Failed to trigger deployment from webhook", "error", err)
		return fmt.Errorf("failed to trigger deployment: %w", err)
	}

	return nil
}

// GetDeploymentStatus returns the current deployment status for a Git source
func (s *GitDeploymentService) GetDeploymentStatus(ctx context.Context, gitSourceID string) (*GitDeployment, error) {
	deploy, err := s.store.GetLatestGitDeployment(ctx, gitSourceID)
	if err != nil {
		return nil, fmt.Errorf("get latest deployment: %w", err)
	}
	if deploy == nil {
		return &GitDeployment{
			GitSourceID:   gitSourceID,
			Status:        DeploymentStatusPending,
			StatusMessage: "No active deployment",
			CreatedAt:     time.Now().UTC(),
			UpdatedAt:     time.Now().UTC(),
		}, nil
	}
	return &GitDeployment{
		ID:            deploy.ID,
		GitSourceID:   deploy.GitSourceID,
		CommitSHA:     deploy.CommitSHA,
		Branch:        deploy.Branch,
		Status:        DeploymentStatus(deploy.Status),
		StatusMessage: deploy.StatusMessage,
		ImageTag:      deploy.ImageTag,
		BuildLog:      deploy.BuildLog,
		DeployLog:     deploy.DeployLog,
		Error:         deploy.Error,
		StartedAt:     deploy.StartedAt,
		CompletedAt:   deploy.CompletedAt,
		CreatedAt:     deploy.CreatedAt,
		UpdatedAt:     deploy.UpdatedAt,
	}, nil
}

// ListDeployments returns the deployment history for a Git source
func (s *GitDeploymentService) ListDeployments(ctx context.Context, gitSourceID string, limit int) ([]GitDeployment, error) {
	deploys, err := s.store.ListGitDeployments(ctx, gitSourceID, limit)
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	result := make([]GitDeployment, len(deploys))
	for i, d := range deploys {
		result[i] = GitDeployment{
			ID:            d.ID,
			GitSourceID:   d.GitSourceID,
			CommitSHA:     d.CommitSHA,
			Branch:        d.Branch,
			Status:        DeploymentStatus(d.Status),
			StatusMessage: d.StatusMessage,
			ImageTag:      d.ImageTag,
			BuildLog:      d.BuildLog,
			DeployLog:     d.DeployLog,
			Error:         d.Error,
			StartedAt:     d.StartedAt,
			CompletedAt:   d.CompletedAt,
			CreatedAt:     d.CreatedAt,
			UpdatedAt:     d.UpdatedAt,
		}
	}
	return result, nil
}

// CancelDeployment cancels an ongoing deployment
func (s *GitDeploymentService) CancelDeployment(ctx context.Context, deploymentID string) error {
	if err := s.store.UpdateGitDeployment(ctx, deploymentID, "cancelled", "Deployment cancelled by user", ""); err != nil {
		return fmt.Errorf("cancel deployment: %w", err)
	}
	s.logger.Info("Cancelled deployment", "deploymentId", deploymentID)
	return nil
}

// timePtr is a helper function to convert time.Time to *time.Time
func timePtr(t time.Time) *time.Time {
	return &t
}

// Helper to resolve file paths
func resolvePath(base, name string) string {
	return filepath.Join(base, name)
}
