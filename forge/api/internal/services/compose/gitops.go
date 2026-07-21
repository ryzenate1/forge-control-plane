package compose

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/events"
	gitsvc "gamepanel/forge/internal/services/git"
	"gamepanel/forge/internal/store"
)

type GitOpsStore interface {
	GetComposeStack(ctx context.Context, id string) (store.ComposeStack, error)
	UpdateComposeStack(ctx context.Context, s *store.ComposeStack) error
	GetComposeStackByWebhookID(ctx context.Context, webhookID string) (*store.ComposeStack, error)
	ListComposeStacksDueForPoll(ctx context.Context) ([]store.ComposeStack, error)
	ListComposeStacksPendingUpdate(ctx context.Context) ([]store.ComposeStack, error)
	ClaimComposeStackForUpdate(ctx context.Context, stackID, workerID string) (*store.ComposeStack, error)
	ReleaseComposeStackClaim(ctx context.Context, stackID, workerID string) error
	GetNode(ctx context.Context, id string) (store.Node, error)
	GetNodeDaemonCredential(ctx context.Context, nodeID string) (string, error)
	GetGitCredential(ctx context.Context, id string) (store.GitCredential, error)
	CreatePlacementReservation(ctx context.Context, req store.CreatePlacementReservationRequest) (store.PlacementReservation, error)
	UpdatePlacementReservationStatus(ctx context.Context, id string, status store.PlacementReservationStatus) (store.PlacementReservation, error)
	DispatchWebhookEvent(event string, payload map[string]any)
}

type GitOpsService struct {
	store      GitOpsStore
	compose    *Service
	gitSvc     GitCloneService
	publisher  events.Publisher
	logger     *slog.Logger
	httpClient *http.Client
	tempDir    string
	mu         sync.Mutex
	webhookMu  sync.Mutex
}

type GitCloneService interface {
	CloneRepo(ctx context.Context, repoURL, branch, sourceID, credentialID string) (*gitsvc.CloneResult, error)
	CloneAtCommit(ctx context.Context, repoURL, branch, commitSHA, sourceID, credentialID string) (*gitsvc.CloneResult, error)
	CleanupClone(cloneDir string) error
}

type WebhookPayload struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	HeadCommit *struct {
		Message string `json:"message"`
		Author  *struct {
			Name string `json:"name"`
		} `json:"author"`
	} `json:"head_commit,omitempty"`
}

type ServiceDiff struct {
	Service string `json:"service"`
	Change  string `json:"change"`
	Detail  string `json:"detail,omitempty"`
}

type DriftCheckResult struct {
	StackID      string        `json:"stackId"`
	DeployedSHA  string        `json:"deployedSha"`
	CurrentSHA   string        `json:"currentSha"`
	DeployedYAML string        `json:"deployedYaml"`
	CurrentYAML  string        `json:"currentYaml"`
	HasDrift     bool          `json:"hasDrift"`
	ServicesDiff []ServiceDiff `json:"servicesDiff,omitempty"`
}

type PreviousDeploymentManifest struct {
	ComposeYAML  string            `json:"composeYaml"`
	CommitSHA    string            `json:"commitSha"`
	ImageDigests map[string]string `json:"imageDigests,omitempty"`
	Timestamp    time.Time         `json:"timestamp"`
	EnvVars      map[string]string `json:"envVars,omitempty"`
}

func sanitizeEnvVars(env map[string]string) map[string]string {
	if env == nil {
		return nil
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		lowerK := strings.ToLower(k)
		if strings.Contains(lowerK, "password") || strings.Contains(lowerK, "secret") ||
			strings.Contains(lowerK, "token") || strings.Contains(lowerK, "key") ||
			strings.Contains(lowerK, "credential") || strings.Contains(lowerK, "pass") {
			out[k] = "***"
		} else {
			out[k] = v
		}
	}
	return out
}

type RuntimeDrift struct {
	Service  string `json:"service"`
	Issue    string `json:"issue"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	State    string `json:"state,omitempty"`
}

type RuntimeDriftResult struct {
	StackID  string         `json:"stackId"`
	HasDrift bool           `json:"hasDrift"`
	Drifts   []RuntimeDrift `json:"drifts"`
}

type GitDeployFromGitRequest struct {
	UserID          string            `json:"userId"`
	NodeID          string            `json:"nodeId"`
	Name            string            `json:"name"`
	RepositoryURL   string            `json:"repositoryUrl"`
	RepositoryPath  string            `json:"repositoryPath,omitempty"`
	ComposePath     string            `json:"composePath,omitempty"`
	Branch          string            `json:"branch"`
	EnvVars         map[string]string `json:"envVars,omitempty"`
	MemoryMB        int64             `json:"memoryMb"`
	CPUShares       int64             `json:"cpuShares"`
	DiskMB          int64             `json:"diskMb"`
	PollIntervalSec int               `json:"pollIntervalSec,omitempty"`
	AutoUpdate      bool              `json:"autoUpdate"`
	CredentialID    string            `json:"credentialId,omitempty"`
}

var (
	ErrStackNotGitBacked       = errors.New("compose stack is not git-backed")
	ErrNoGitUpdateAvailable    = errors.New("no git update available for this stack")
	ErrWebhookStale            = errors.New("webhook payload is stale; desired commit has not changed")
	ErrCommitAlreadyDeployed   = errors.New("commit already deployed")
	ErrWebhookSignatureInvalid = errors.New("webhook signature is invalid")
	ErrGitNotConfigured        = errors.New("git service is not configured")
	ErrAlreadyUpdating         = errors.New("stack is already being updated")
	ErrNoPreviousDeployment    = errors.New("no previous deployment to roll back to")
)

func NewGitOpsService(s GitOpsStore, cs *Service, gitSvc GitCloneService, publisher events.Publisher, logger *slog.Logger) *GitOpsService {
	if logger == nil {
		logger = slog.Default()
	}
	return &GitOpsService{
		store:      s,
		compose:    cs,
		gitSvc:     gitSvc,
		publisher:  publisher,
		logger:     logger,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		tempDir:    os.TempDir(),
	}
}

func (g *GitOpsService) getStack(ctx context.Context, stackID string) (*ComposeStack, error) {
	if g.store == nil {
		return nil, ErrStackNotFound
	}
	s, err := g.store.GetComposeStack(ctx, stackID)
	if err != nil {
		return nil, ErrStackNotFound
	}
	return fromStoreComposeStack(s), nil
}

func (g *GitOpsService) requireGitBacked(stack *ComposeStack) error {
	if stack.GitRepositoryURL == "" {
		return ErrStackNotGitBacked
	}
	return nil
}

func (g *GitOpsService) requireNotUpdating(stack *ComposeStack) error {
	if stack.GitUpdateStatus == "updating" || stack.GitUpdateStatus == "deploying" {
		return ErrAlreadyUpdating
	}
	return nil
}

func findComposeFile(dir string) (string, error) {
	candidates := []string{
		"compose.yml",
		"compose.yaml",
		"docker-compose.yml",
		"docker-compose.yaml",
	}
	for _, name := range candidates {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no compose file found in %s", dir)
}

func readComposeFromDir(dir string, composePath string) (string, error) {
	if composePath != "" {
		if strings.Contains(composePath, "..") {
			return "", fmt.Errorf("invalid compose path: contains '..'")
		}
		resolved := filepath.Clean(filepath.Join(dir, composePath))
		if !strings.HasPrefix(resolved, filepath.Clean(dir)+string(filepath.Separator)) && resolved != filepath.Clean(dir) {
			return "", fmt.Errorf("compose path escapes working directory")
		}
		content, err := os.ReadFile(resolved)
		if err != nil {
			return "", fmt.Errorf("read compose file at %s: %w", composePath, err)
		}
		return string(content), nil
	}

	candidates := []string{
		"compose.yml",
		"compose.yaml",
		"docker-compose.yml",
		"docker-compose.yaml",
	}

	var bestPath string
	bestDepth := 0

	walkErr := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip hidden directories but not the root
			if path != dir && strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		base := filepath.Base(path)
		for _, candidate := range candidates {
			if base == candidate {
				rel, _ := filepath.Rel(dir, path)
				depth := len(strings.Split(rel, string(filepath.Separator)))
				if bestPath == "" || depth < bestDepth {
					bestPath = path
					bestDepth = depth
				}
				break
			}
		}
		return nil
	})

	if bestPath != "" {
		content, err := os.ReadFile(bestPath)
		if err != nil {
			return "", fmt.Errorf("read compose file: %w", err)
		}
		return string(content), nil
	}

	if walkErr != nil {
		return "", fmt.Errorf("walk directory %s: %w", dir, walkErr)
	}

	return "", fmt.Errorf("no compose file found in %s", dir)
}

func (g *GitOpsService) DeployFromGit(ctx context.Context, req GitDeployFromGitRequest) (*GitDeployResult, error) {
	if req.Name == "" || req.UserID == "" || req.RepositoryURL == "" || req.Branch == "" {
		return nil, fmt.Errorf("name, userId, repositoryUrl, and branch are required")
	}
	if g.gitSvc == nil {
		return nil, ErrGitNotConfigured
	}
	if g.compose == nil {
		return nil, errors.New("compose service not available")
	}
	clone, err := g.gitSvc.CloneRepo(ctx, req.RepositoryURL, req.Branch, req.Name, req.CredentialID)
	if err != nil {
		return nil, fmt.Errorf("clone repository: %w", err)
	}
	defer func() {
		if cleanupErr := g.gitSvc.CleanupClone(clone.Dir); cleanupErr != nil {
			g.logger.Error("failed to cleanup clone directory", "dir", clone.Dir, "error", cleanupErr)
		}
	}()
	composeYAML, err := readComposeFromDir(clone.Dir, req.ComposePath)
	if err != nil {
		return nil, fmt.Errorf("read compose from repository: %w", err)
	}
	validation := g.compose.Validate([]byte(composeYAML), clone.Dir)
	if !validation.Valid {
		errMsg := "invalid compose yaml"
		if len(validation.Errors) > 0 {
			errMsg = validation.Errors[0].Message
		}
		return nil, fmt.Errorf("%w: %s", ErrInvalidCompose, errMsg)
	}
	hash := computeHash(composeYAML)
	node, err := g.store.GetNode(ctx, req.NodeID)
	if err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}
	nodeCredential, err := g.store.GetNodeDaemonCredential(ctx, req.NodeID)
	if err != nil {
		return nil, fmt.Errorf("node credential not found: %w", err)
	}
	reservation, err := g.store.CreatePlacementReservation(ctx, store.CreatePlacementReservationRequest{
		NodeID:          req.NodeID,
		CPU:             int(req.CPUShares / 100),
		Memory:          req.MemoryMB,
		Disk:            req.DiskMB,
		ReservationType: store.PlacementReservationTypePlacement,
	})
	if err != nil {
		return nil, fmt.Errorf("create reservation: %w", err)
	}
	stackID := g.compose.createStackID()
	now := time.Now().UTC()
	repoPath := req.RepositoryPath
	if repoPath == "" {
		repoPath = clone.Dir
	}
	stack := &ComposeStack{
		ID:                  stackID,
		UserID:              req.UserID,
		Name:                req.Name,
		NodeID:              req.NodeID,
		Status:              StackStatusDeploying,
		ComposeYAML:         composeYAML,
		ComposeHash:         hash,
		EnvVars:             req.EnvVars,
		MemoryMB:            req.MemoryMB,
		CPUShares:           req.CPUShares,
		DiskMB:              req.DiskMB,
		ReservationID:       reservation.ID,
		CreatedAt:           now,
		UpdatedAt:           now,
		GitSourceID:         req.RepositoryURL,
		GitRepositoryURL:    req.RepositoryURL,
		GitRepositoryPath:   repoPath,
		ComposePath:         req.ComposePath,
		GitBranch:           clone.Branch,
		GitCommitSHA:        clone.CommitSHA,
		GitDesiredCommitSHA: clone.CommitSHA,
		GitAutoUpdate:       true,
		GitPollIntervalSec:  300,
		GitUpdateStatus:     "idle",
		GitCredentialID:     req.CredentialID,
	}
	if err := g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack)); err != nil {
		_, _ = g.store.UpdatePlacementReservationStatus(ctx, reservation.ID, store.PlacementReservationStatusCancelled)
		return nil, fmt.Errorf("create compose stack record: %w", err)
	}
	client := daemon.NewClient()
	deployResp, deployErr := client.ComposeDeploy(ctx, node.BaseURL, nodeCredential, daemon.ComposeDeployRequest{
		StackID:     stackID,
		ComposeYAML: composeYAML,
		EnvVars:     req.EnvVars,
	})
	if deployErr != nil {
		_, _ = g.store.UpdatePlacementReservationStatus(ctx, reservation.ID, store.PlacementReservationStatusCancelled)
		g.compose.markFailed(ctx, stack, deployErr.Error())
		return nil, fmt.Errorf("compose deploy: %w", deployErr)
	}
	_ = deployResp
	stack.Status = StackStatusAwaitingHealth
	stack.UpdatedAt = time.Now().UTC()
	_ = g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
	_, _ = g.store.UpdatePlacementReservationStatus(ctx, reservation.ID, store.PlacementReservationStatusCompleted)

	if err := g.compose.WaitForHealthy(ctx, stackID, node.BaseURL, nodeCredential, 2*time.Minute); err != nil {
		stack.Status = StackStatusDegraded
		stack.Error = "health check failed: " + err.Error()
		stack.UpdatedAt = time.Now().UTC()
		_ = g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
	} else {
		stack.Status = StackStatusRunning
		stack.Error = ""
		stack.UpdatedAt = time.Now().UTC()
		_ = g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
	}
	if g.publisher != nil {
		_ = g.publisher.Publish(ctx, events.NewEnvelope(events.EventComposeDeployed, "compose", "stack", stackID, map[string]any{
			"name": req.Name, "nodeId": req.NodeID,
		}))
	}
	g.store.DispatchWebhookEvent(string(events.EventComposeDeployed), map[string]any{"stackId": stackID, "name": req.Name, "nodeId": req.NodeID})
	services, _ := g.getPerServiceStatus(ctx, stack)
	return &GitDeployResult{
		StackID:   stackID,
		CommitSHA: clone.CommitSHA,
		Branch:    clone.Branch,
		Services:  services,
		Status:    "success",
	}, nil
}

func (g *GitOpsService) CheckForUpdates(ctx context.Context, stackID string) (*GitUpdatePreview, error) {
	stack, err := g.getStack(ctx, stackID)
	if err != nil {
		return nil, err
	}
	if err := g.requireGitBacked(stack); err != nil {
		return nil, err
	}
	if err := g.requireNotUpdating(stack); err != nil {
		return nil, err
	}
	if g.gitSvc == nil {
		return nil, ErrGitNotConfigured
	}
	clone, err := g.gitSvc.CloneRepo(ctx, stack.GitRepositoryURL, stack.GitBranch, stack.ID, stack.GitCredentialID)
	if err != nil {
		return nil, fmt.Errorf("clone repository for check: %w", err)
	}
	defer func() {
		if cleanupErr := g.gitSvc.CleanupClone(clone.Dir); cleanupErr != nil {
			g.logger.Error("cleanup clone failed", "dir", clone.Dir, "error", cleanupErr)
		}
	}()
	hasUpdate := clone.CommitSHA != stack.GitCommitSHA
	preview := &GitUpdatePreview{
		StackID:       stackID,
		CurrentCommit: stack.GitCommitSHA,
		DesiredCommit: stack.GitCommitSHA,
		CurrentBranch: stack.GitBranch,
		HasUpdate:     hasUpdate,
		CheckedAt:     time.Now().UTC(),
	}
	if hasUpdate {
		preview.DesiredCommit = clone.CommitSHA
		composeYAML, err := readComposeFromDir(clone.Dir, stack.ComposePath)
		if err == nil {
			preview.PreviewYAML = composeYAML
			parsed, parseErr := ParseSummary([]byte(composeYAML), clone.Dir)
			if parseErr == nil {
				changed := make([]string, 0)
				for _, svc := range parsed.Services {
					changed = append(changed, svc.Name)
				}
				preview.ServicesChanged = changed
			}
		}
	}
	return preview, nil
}

func (g *GitOpsService) GetUpdatePreview(ctx context.Context, stackID string) (*GitUpdatePreview, error) {
	stack, err := g.getStack(ctx, stackID)
	if err != nil {
		return nil, err
	}
	if err := g.requireGitBacked(stack); err != nil {
		return nil, err
	}
	hasUpdate := stack.GitDesiredCommitSHA != "" && stack.GitDesiredCommitSHA != stack.GitCommitSHA
	preview := &GitUpdatePreview{
		StackID:       stackID,
		CurrentCommit: stack.GitCommitSHA,
		DesiredCommit: stack.GitDesiredCommitSHA,
		CurrentBranch: stack.GitBranch,
		HasUpdate:     hasUpdate,
		CheckedAt:     time.Now().UTC(),
	}
	if hasUpdate {
		preview.DesiredCommit = stack.GitDesiredCommitSHA
	}
	return preview, nil
}

func (g *GitOpsService) RedeployFromGit(ctx context.Context, stackID string) (*GitDeployResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	stack, err := g.getStack(ctx, stackID)
	if err != nil {
		return nil, err
	}
	if err := g.requireGitBacked(stack); err != nil {
		return nil, err
	}
	if err := g.requireNotUpdating(stack); err != nil {
		return nil, err
	}
	if g.gitSvc == nil {
		return nil, ErrGitNotConfigured
	}
	stack.GitUpdateStatus = "deploying"
	_ = g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
	clone, err := g.gitSvc.CloneRepo(ctx, stack.GitRepositoryURL, stack.GitBranch, stack.ID, stack.GitCredentialID)
	if err != nil {
		return nil, fmt.Errorf("clone repository: %w", err)
	}
	defer func() {
		if cleanupErr := g.gitSvc.CleanupClone(clone.Dir); cleanupErr != nil {
			g.logger.Error("cleanup clone failed", "dir", clone.Dir, "error", cleanupErr)
		}
	}()
	composeYAML, err := readComposeFromDir(clone.Dir, stack.ComposePath)
	if err != nil {
		return nil, fmt.Errorf("read compose from repository: %w", err)
	}
	if g.compose != nil {
		validation := g.compose.Validate([]byte(composeYAML), clone.Dir)
		if !validation.Valid {
			errMsg := "invalid compose yaml"
			if len(validation.Errors) > 0 {
				errMsg = validation.Errors[0].Message
			}
			return nil, fmt.Errorf("%w: %s", ErrInvalidCompose, errMsg)
		}
	}
	return g.deployFromClone(ctx, stack, clone, composeYAML)
}

func (g *GitOpsService) PullAndRedeploy(ctx context.Context, stackID string) (*GitDeployResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	stack, err := g.getStack(ctx, stackID)
	if err != nil {
		return nil, err
	}
	if err := g.requireGitBacked(stack); err != nil {
		return nil, err
	}
	if err := g.requireNotUpdating(stack); err != nil {
		return nil, err
	}
	if g.gitSvc == nil {
		return nil, ErrGitNotConfigured
	}
	stack.GitUpdateStatus = "deploying"
	_ = g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
	clone, err := g.gitSvc.CloneRepo(ctx, stack.GitRepositoryURL, stack.GitBranch, stack.ID, stack.GitCredentialID)
	if err != nil {
		return nil, fmt.Errorf("clone repository: %w", err)
	}
	defer func() {
		if cleanupErr := g.gitSvc.CleanupClone(clone.Dir); cleanupErr != nil {
			g.logger.Error("cleanup clone failed", "dir", clone.Dir, "error", cleanupErr)
		}
	}()
	if clone.CommitSHA == stack.GitCommitSHA {
		return nil, ErrNoGitUpdateAvailable
	}
	composeYAML, err := readComposeFromDir(clone.Dir, stack.ComposePath)
	if err != nil {
		return nil, fmt.Errorf("read compose from repository: %w", err)
	}
	if g.compose != nil {
		validation := g.compose.Validate([]byte(composeYAML), clone.Dir)
		if !validation.Valid {
			errMsg := "invalid compose yaml"
			if len(validation.Errors) > 0 {
				errMsg = validation.Errors[0].Message
			}
			return nil, fmt.Errorf("%w: %s", ErrInvalidCompose, errMsg)
		}
	}
	return g.deployFromClone(ctx, stack, clone, composeYAML)
}

func (g *GitOpsService) RollbackToPrevious(ctx context.Context, stackID string) (*GitDeployResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	stack, err := g.getStack(ctx, stackID)
	if err != nil {
		return nil, err
	}
	if err := g.requireGitBacked(stack); err != nil {
		return nil, err
	}
	if err := g.requireNotUpdating(stack); err != nil {
		return nil, err
	}
	if stack.GitPreviousCommitSHA == "" || stack.GitPreviousCompose == "" {
		return nil, ErrNoPreviousDeployment
	}
	prevCommit := stack.GitPreviousCommitSHA
	prevCompose := stack.GitPreviousCompose
	prevBranch := stack.GitBranch
	if g.compose == nil {
		return nil, errors.New("compose service not available")
	}
	validation := g.compose.Validate([]byte(prevCompose), "")
	if !validation.Valid {
		return nil, ErrInvalidCompose
	}
	stack.GitUpdateStatus = "deploying"
	_ = g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
	stack.GitUpdateStatus = "rolling_back"
	_ = g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
	node, err := g.store.GetNode(ctx, stack.NodeID)
	if err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}
	nodeCredential, err := g.store.GetNodeDaemonCredential(ctx, stack.NodeID)
	if err != nil {
		return nil, fmt.Errorf("node credential not found: %w", err)
	}
	client := daemon.NewClient()
	deployResp, deployErr := client.ComposeDeploy(ctx, node.BaseURL, nodeCredential, daemon.ComposeDeployRequest{
		StackID:     stackID,
		ComposeYAML: prevCompose,
		EnvVars:     stack.EnvVars,
	})
	if deployErr != nil {
		g.compose.markFailed(ctx, stack, "rollback deploy failed: "+deployErr.Error())
		return nil, fmt.Errorf("deploy rollback: %w", deployErr)
	}
	_ = deployResp
	now := time.Now().UTC()
	stack.Status = StackStatusAwaitingHealth
	oldCompose := stack.ComposeYAML
	oldCommit := stack.GitCommitSHA
	stack.ComposeYAML = prevCompose
	stack.ComposeHash = computeHash(prevCompose)
	stack.GitDesiredCommitSHA = prevCommit
	stack.GitReconcileMode = "rollback_hold"
	stack.GitFailedSHA = oldCommit
	stack.GitPreviousCompose = oldCompose
	stack.GitPreviousCommitSHA = oldCommit
	stack.GitCommitSHA = prevCommit
	stack.GitUpdateStatus = "idle"
	stack.GitUpdateError = ""
	stack.UpdatedAt = now
	_ = g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))

	if err := g.compose.WaitForHealthy(ctx, stackID, node.BaseURL, nodeCredential, 2*time.Minute); err != nil {
		stack.Status = StackStatusDegraded
		stack.Error = "rollback health check failed: " + err.Error()
		stack.UpdatedAt = time.Now().UTC()
		_ = g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
	} else {
		stack.Status = StackStatusRunning
		stack.Error = ""
		stack.UpdatedAt = time.Now().UTC()
		_ = g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
	}
	if g.publisher != nil {
		_ = g.publisher.Publish(ctx, events.NewEnvelope(events.EventComposeRollbackCompleted, "compose", "stack", stackID, map[string]any{
			"commit": prevCommit, "branch": prevBranch,
		}))
	}
	g.store.DispatchWebhookEvent(string(events.EventComposeRollbackCompleted), map[string]any{"stackId": stackID, "commit": prevCommit, "branch": prevBranch})
	services, _ := g.getPerServiceStatus(ctx, stack)
	return &GitDeployResult{
		StackID:   stackID,
		CommitSHA: prevCommit,
		Branch:    prevBranch,
		Services:  services,
		Status:    "success",
	}, nil
}

func (g *GitOpsService) DetectDrift(ctx context.Context, stackID string) (*DriftCheckResult, error) {
	stack, err := g.getStack(ctx, stackID)
	if err != nil {
		return nil, err
	}
	if err := g.requireGitBacked(stack); err != nil {
		return nil, err
	}
	if g.gitSvc == nil {
		return nil, ErrGitNotConfigured
	}
	clone, err := g.gitSvc.CloneRepo(ctx, stack.GitRepositoryURL, stack.GitBranch, stack.ID, stack.GitCredentialID)
	if err != nil {
		return nil, fmt.Errorf("clone repository for drift check: %w", err)
	}
	defer func() {
		if cleanupErr := g.gitSvc.CleanupClone(clone.Dir); cleanupErr != nil {
			g.logger.Error("cleanup clone failed", "dir", clone.Dir, "error", cleanupErr)
		}
	}()
	composeInRepo, err := readComposeFromDir(clone.Dir, stack.ComposePath)
	if err != nil {
		return nil, fmt.Errorf("read compose from repository: %w", err)
	}

	parsedDeployed, err := g.compose.ParseComposeYAML([]byte(stack.ComposeYAML), "", nil)
	if err != nil {
		hasDrift := composeInRepo != stack.ComposeYAML
		diff := []ServiceDiff{{Service: "*", Change: "parse error", Detail: err.Error()}}
		return &DriftCheckResult{
			StackID:      stackID,
			DeployedSHA:  stack.GitCommitSHA,
			CurrentSHA:   clone.CommitSHA,
			DeployedYAML: stack.ComposeYAML,
			CurrentYAML:  composeInRepo,
			HasDrift:     hasDrift,
			ServicesDiff: diff,
		}, nil
	}

	parsedRepo, err := g.compose.ParseComposeYAML([]byte(composeInRepo), "", nil)
	if err != nil {
		hasDrift := composeInRepo != stack.ComposeYAML
		diff := []ServiceDiff{{Service: "*", Change: "parse repo error", Detail: err.Error()}}
		return &DriftCheckResult{
			StackID:      stackID,
			DeployedSHA:  stack.GitCommitSHA,
			CurrentSHA:   clone.CommitSHA,
			DeployedYAML: stack.ComposeYAML,
			CurrentYAML:  composeInRepo,
			HasDrift:     hasDrift,
			ServicesDiff: diff,
		}, nil
	}

	diffs := computeServiceDiffs(parsedDeployed, parsedRepo)

	return &DriftCheckResult{
		StackID:      stackID,
		DeployedSHA:  stack.GitCommitSHA,
		CurrentSHA:   clone.CommitSHA,
		DeployedYAML: stack.ComposeYAML,
		CurrentYAML:  composeInRepo,
		HasDrift:     len(diffs) > 0,
		ServicesDiff: diffs,
	}, nil
}

func (g *GitOpsService) DetectRuntimeDrift(ctx context.Context, stackID string) (*RuntimeDriftResult, error) {
	stack, err := g.getStack(ctx, stackID)
	if err != nil {
		return nil, err
	}

	node, err := g.store.GetNode(ctx, stack.NodeID)
	if err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}
	nodeCredential, err := g.store.GetNodeDaemonCredential(ctx, stack.NodeID)
	if err != nil {
		return nil, fmt.Errorf("node credential not found: %w", err)
	}

	client := daemon.NewClient()
	daemonStatus, err := client.ComposeStatus(ctx, node.BaseURL, nodeCredential, stackID)
	if err != nil {
		return nil, fmt.Errorf("get compose status: %w", err)
	}

	parsed, err := g.compose.ParseComposeYAML([]byte(stack.ComposeYAML), "", nil)
	if err != nil {
		return nil, fmt.Errorf("parse compose yaml: %w", err)
	}

	desiredByName := make(map[string]ServiceSummary)
	for _, svc := range parsed.Services {
		desiredByName[svc.Name] = svc
	}

	actualByName := make(map[string]daemon.ComposeServiceState)
	for _, svc := range daemonStatus.Services {
		actualByName[svc.Name] = svc
	}

	var drifts []RuntimeDrift

	for _, svc := range parsed.Services {
		actual, exists := actualByName[svc.Name]
		if !exists {
			drifts = append(drifts, RuntimeDrift{
				Service: svc.Name,
				Issue:   "missing",
			})
			continue
		}
		if actual.Image != "" && svc.Image != "" && actual.Image != svc.Image {
			drifts = append(drifts, RuntimeDrift{
				Service:  svc.Name,
				Issue:    "image mismatch",
				Expected: svc.Image,
				Actual:   actual.Image,
			})
		}
		if actual.Status != "running" {
			drifts = append(drifts, RuntimeDrift{
				Service: svc.Name,
				Issue:   "not running",
				State:   actual.State,
			})
		} else if strings.Contains(strings.ToLower(actual.State), "restart") || strings.Contains(strings.ToLower(actual.State), "unhealthy") {
			drifts = append(drifts, RuntimeDrift{
				Service: svc.Name,
				Issue:   "unstable",
				State:   actual.State,
			})
		}
	}

	for _, svc := range daemonStatus.Services {
		if _, exists := desiredByName[svc.Name]; !exists {
			drifts = append(drifts, RuntimeDrift{
				Service: svc.Name,
				Issue:   "unexpected service",
				Actual:  svc.Image,
			})
		}
	}

	return &RuntimeDriftResult{
		StackID:  stackID,
		HasDrift: len(drifts) > 0,
		Drifts:   drifts,
	}, nil
}

func computeServiceDiffs(deployed *ParsedCompose, repo *ParsedCompose) []ServiceDiff {
	deployedByName := make(map[string]ServiceSummary)
	for _, svc := range deployed.Services {
		deployedByName[svc.Name] = svc
	}
	repoByName := make(map[string]ServiceSummary)
	for _, svc := range repo.Services {
		repoByName[svc.Name] = svc
	}

	var diffs []ServiceDiff

	for _, svc := range deployed.Services {
		repoSvc, exists := repoByName[svc.Name]
		if !exists {
			diffs = append(diffs, ServiceDiff{
				Service: svc.Name,
				Change:  "removed",
				Detail:  "service removed from repository",
			})
			continue
		}
		changes := compareServiceSummaries(svc, repoSvc)
		for _, ch := range changes {
			diffs = append(diffs, ServiceDiff{
				Service: svc.Name,
				Change:  ch,
			})
		}
	}

	for _, svc := range repo.Services {
		if _, exists := deployedByName[svc.Name]; !exists {
			diffs = append(diffs, ServiceDiff{
				Service: svc.Name,
				Change:  "added",
				Detail:  "new service in repository",
			})
		}
	}

	return diffs
}

func compareServiceSummaries(a, b ServiceSummary) []string {
	var changes []string

	if a.Image != b.Image {
		changes = append(changes, fmt.Sprintf("image changed: %s -> %s", a.Image, b.Image))
	}
	if !stringSlicesEqual(a.Ports, b.Ports) {
		changes = append(changes, "ports changed")
	}
	if !mapsEqual(a.Environment, b.Environment) {
		changes = append(changes, "environment variables changed")
	}
	if !stringSlicesEqual(a.Volumes, b.Volumes) {
		changes = append(changes, "volume mounts changed")
	}
	if a.Restart != b.Restart {
		changes = append(changes, fmt.Sprintf("restart policy changed: %s -> %s", a.Restart, b.Restart))
	}
	if a.Command != b.Command {
		changes = append(changes, "command changed")
	}
	if a.Entrypoint != b.Entrypoint {
		changes = append(changes, "entrypoint changed")
	}
	if !stringSlicesEqual(a.DependsOn, b.DependsOn) {
		changes = append(changes, "dependencies changed")
	}
	if a.Deploy != nil && b.Deploy != nil {
		if a.Deploy.Replicas != b.Deploy.Replicas {
			changes = append(changes, fmt.Sprintf("replicas changed: %d -> %d", a.Deploy.Replicas, b.Deploy.Replicas))
		}
		aRes := a.Deploy.Resources
		bRes := b.Deploy.Resources
		if aRes != nil && bRes != nil {
			if !mapsEqual(aRes.Limits, bRes.Limits) {
				changes = append(changes, "resource limits changed")
			}
			if !mapsEqual(aRes.Reservations, bRes.Reservations) {
				changes = append(changes, "resource reservations changed")
			}
		} else if (aRes == nil) != (bRes == nil) {
			changes = append(changes, "resource configuration changed")
		}
	} else if (a.Deploy == nil) != (b.Deploy == nil) {
		changes = append(changes, "deploy configuration changed")
	}

	return changes
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int)
	for _, v := range a {
		seen[v]++
	}
	for _, v := range b {
		if c, ok := seen[v]; !ok || c == 0 {
			return false
		}
		seen[v]--
	}
	return true
}

func (g *GitOpsService) SetBranch(ctx context.Context, stackID string, branch string) (*ComposeStack, error) {
	stack, err := g.getStack(ctx, stackID)
	if err != nil {
		return nil, err
	}
	if err := g.requireGitBacked(stack); err != nil {
		return nil, err
	}
	if branch == "" || strings.Contains(branch, "..") || strings.Contains(branch, "/") {
		return nil, gitsvc.ErrInvalidBranch
	}
	stack.GitBranch = branch
	stack.GitCommitSHA = ""
	stack.GitDesiredCommitSHA = ""
	stack.GitUpdateStatus = "branch_changed"
	stack.UpdatedAt = time.Now().UTC()
	if err := g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack)); err != nil {
		return nil, fmt.Errorf("update stack branch: %w", err)
	}
	return stack, nil
}

func (g *GitOpsService) SetAutoUpdate(ctx context.Context, stackID string, enabled bool, intervalSec int) (*ComposeStack, error) {
	stack, err := g.getStack(ctx, stackID)
	if err != nil {
		return nil, err
	}
	if err := g.requireGitBacked(stack); err != nil {
		return nil, err
	}
	stack.GitAutoUpdate = enabled
	if enabled && intervalSec > 0 {
		stack.GitPollIntervalSec = intervalSec
	}
	if !enabled {
		stack.GitPollIntervalSec = 0
	}
	stack.UpdatedAt = time.Now().UTC()
	if err := g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack)); err != nil {
		return nil, fmt.Errorf("update auto-update setting: %w", err)
	}
	return stack, nil
}

func (g *GitOpsService) GetGitStatus(ctx context.Context, stackID string) (map[string]any, error) {
	stack, err := g.getStack(ctx, stackID)
	if err != nil {
		return nil, err
	}
	if err := g.requireGitBacked(stack); err != nil {
		return nil, err
	}
	return map[string]any{
		"gitRepositoryUrl":    stack.GitRepositoryURL,
		"gitRepositoryPath":   stack.GitRepositoryPath,
		"gitBranch":           stack.GitBranch,
		"gitCommitSha":        stack.GitCommitSHA,
		"gitDesiredCommitSha": stack.GitDesiredCommitSHA,
		"gitAutoUpdate":       stack.GitAutoUpdate,
		"gitPollIntervalSec":  float64(stack.GitPollIntervalSec),
		"gitUpdateStatus":     stack.GitUpdateStatus,
		"gitUpdateError":      stack.GitUpdateError,
		"gitCredentialId":     stack.GitCredentialID,
	}, nil
}

func (g *GitOpsService) GetLastWebhookAt(ctx context.Context, stackID string) (*time.Time, error) {
	stack, err := g.getStack(ctx, stackID)
	if err != nil {
		return nil, err
	}
	return stack.GitLastWebhookAt, nil
}

func (g *GitOpsService) SetWebhookSecret(ctx context.Context, stackID, secret, webhookID string) error {
	stack, err := g.getStack(ctx, stackID)
	if err != nil {
		return err
	}
	stack.GitWebhookSecret = secret
	stack.GitWebhookID = webhookID
	stack.UpdatedAt = time.Now().UTC()
	return g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
}

func (g *GitOpsService) deployFromClone(ctx context.Context, stack *ComposeStack, clone *gitsvc.CloneResult, composeYAML string) (*GitDeployResult, error) {
	stackID := stack.ID
	prevCompose := stack.ComposeYAML
	prevCommit := stack.GitCommitSHA
	prevHash := stack.ComposeHash
	now := time.Now().UTC()
	stack.GitUpdateStatus = "updating"
	stack.UpdatedAt = now
	_ = g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
	node, err := g.store.GetNode(ctx, stack.NodeID)
	if err != nil {
		g.compose.markFailed(ctx, stack, "deploy: node not found: "+err.Error())
		return nil, fmt.Errorf("node not found: %w", err)
	}
	nodeCredential, err := g.store.GetNodeDaemonCredential(ctx, stack.NodeID)
	if err != nil {
		g.compose.markFailed(ctx, stack, "deploy: credential not found: "+err.Error())
		return nil, fmt.Errorf("node credential not found: %w", err)
	}
	client := daemon.NewClient()
	deployResp, deployErr := client.ComposeDeploy(ctx, node.BaseURL, nodeCredential, daemon.ComposeDeployRequest{
		StackID:     stackID,
		ComposeYAML: composeYAML,
		EnvVars:     stack.EnvVars,
	})
	if deployErr != nil {
		g.compose.rollbackStack(ctx, stack, prevCompose, prevHash, stack.EnvVars)
		return nil, fmt.Errorf("deploy updated stack: %w", deployErr)
	}
	_ = deployResp
	now = time.Now().UTC()
	stack.GitPreviousCommitSHA = prevCommit
	stack.GitPreviousCompose = prevCompose

	manifest := PreviousDeploymentManifest{
		ComposeYAML: prevCompose,
		CommitSHA:   prevCommit,
		Timestamp:   now,
		EnvVars:     sanitizeEnvVars(stack.EnvVars),
	}
	manifestBytes, _ := json.Marshal(manifest)
	raw := json.RawMessage(manifestBytes)
	stack.GitPreviousManifest = &raw

	stack.GitCommitSHA = clone.CommitSHA
	stack.GitDesiredCommitSHA = clone.CommitSHA
	stack.ComposeYAML = composeYAML
	stack.ComposeHash = computeHash(composeYAML)
	stack.Status = StackStatusAwaitingHealth
	stack.GitUpdateStatus = "idle"
	stack.GitUpdateError = ""
	stack.Error = ""
	stack.UpdatedAt = now
	_ = g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))

	if err := g.compose.WaitForHealthy(ctx, stackID, node.BaseURL, nodeCredential, 2*time.Minute); err != nil {
		stack.Status = StackStatusDegraded
		stack.Error = "health check failed: " + err.Error()
		stack.UpdatedAt = time.Now().UTC()
		_ = g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
	} else {
		stack.Status = StackStatusRunning
		stack.Error = ""
		stack.UpdatedAt = time.Now().UTC()
		_ = g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
	}
	if g.publisher != nil {
		_ = g.publisher.Publish(ctx, events.NewEnvelope(events.EventComposeUpdated, "compose", "stack", stackID, map[string]any{
			"commit": clone.CommitSHA, "branch": clone.Branch,
		}))
	}
	g.store.DispatchWebhookEvent(string(events.EventComposeUpdated), map[string]any{"stackId": stackID, "commit": clone.CommitSHA, "branch": clone.Branch})
	services, _ := g.getPerServiceStatus(ctx, stack)
	return &GitDeployResult{
		StackID:   stackID,
		CommitSHA: clone.CommitSHA,
		Branch:    clone.Branch,
		Services:  services,
		Status:    "success",
	}, nil
}

func (g *GitOpsService) getPerServiceStatus(ctx context.Context, stack *ComposeStack) ([]GitServiceState, error) {
	node, err := g.store.GetNode(ctx, stack.NodeID)
	if err != nil {
		return nil, nil
	}
	nodeCredential, err := g.store.GetNodeDaemonCredential(ctx, stack.NodeID)
	if err != nil {
		return nil, nil
	}
	client := daemon.NewClient()
	statusResp, err := client.ComposeStatus(ctx, node.BaseURL, nodeCredential, stack.ID)
	if err != nil {
		return nil, nil
	}
	services := make([]GitServiceState, 0, len(statusResp.Services))
	for _, svc := range statusResp.Services {
		updateOK := svc.Status == "running"
		updateMsg := ""
		if !updateOK {
			updateMsg = fmt.Sprintf("service is %s (%s)", svc.Status, svc.State)
		}
		services = append(services, GitServiceState{
			Name:      svc.Name,
			Image:     svc.Image,
			Status:    svc.Status,
			State:     svc.State,
			Ports:     svc.Ports,
			UpdateOK:  updateOK,
			UpdateMsg: updateMsg,
		})
	}
	return services, nil
}

func (g *GitOpsService) HandleWebhook(ctx context.Context, webhookID string, payload []byte, signature string, deliveryID string) (bool, error) {
	g.webhookMu.Lock()
	defer g.webhookMu.Unlock()
	if g.store == nil {
		return false, ErrStackNotFound
	}
	storeStack, err := g.store.GetComposeStackByWebhookID(ctx, webhookID)
	if err != nil || storeStack == nil {
		return false, ErrStackNotFound
	}
	stack := fromStoreComposeStack(*storeStack)
	if stack.GitWebhookSecret == "" {
		return false, errors.New("webhook secret not configured for this stack")
	}
	if signature == "" {
		return false, ErrWebhookSignatureInvalid
	}
	sig := strings.TrimPrefix(signature, "sha256=")
	mac := hmac.New(sha256.New, []byte(stack.GitWebhookSecret))
	mac.Write(payload)
	expectedSig := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return false, ErrWebhookSignatureInvalid
	}

	if deliveryID != "" && deliveryID == stack.GitLastDeliveryID {
		return false, nil
	}

	var whPayload struct {
		Ref        string `json:"ref"`
		After      string `json:"after"`
		HeadCommit *struct {
			Message string `json:"message"`
			Author  *struct {
				Name string `json:"name"`
			} `json:"author"`
		} `json:"head_commit,omitempty"`
	}
	if err := json.Unmarshal(payload, &whPayload); err != nil {
		return false, fmt.Errorf("invalid webhook payload: %w", err)
	}
	if whPayload.Ref == "" || whPayload.After == "" {
		return false, errors.New("webhook payload missing ref or after fields")
	}
	if whPayload.After == "0000000000000000000000000000000000000000" {
		return false, errors.New("webhook indicates branch deletion")
	}
	branch := strings.TrimPrefix(whPayload.Ref, "refs/heads/")
	if branch != stack.GitBranch {
		return false, nil
	}
	if whPayload.After == stack.GitDesiredCommitSHA {
		return false, ErrWebhookStale
	}
	if whPayload.After == stack.GitCommitSHA {
		return false, ErrCommitAlreadyDeployed
	}
	if whPayload.After == stack.GitDesiredCommitSHA {
		return false, ErrWebhookStale
	}
	if stack.GitUpdateStatus != "" && stack.GitUpdateStatus != "idle" && stack.GitUpdateStatus != "update_available" {
		return false, fmt.Errorf("stack is currently %s, cannot accept webhook", stack.GitUpdateStatus)
	}
	now := time.Now().UTC()
	stack.GitDesiredCommitSHA = whPayload.After
	stack.GitLastWebhookAt = &now
	stack.GitUpdateStatus = "pending"
	stack.GitLastDeliveryID = deliveryID
	stack.UpdatedAt = now
	if err := g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack)); err != nil {
		return false, fmt.Errorf("update stack for webhook: %w", err)
	}
	if g.publisher != nil {
		_ = g.publisher.Publish(ctx, events.NewEnvelope(events.EventComposeWebhookReceived, "compose", "stack", stack.ID, map[string]any{
			"branch": branch, "commit": whPayload.After, "deliveryId": deliveryID,
		}))
	}
	g.store.DispatchWebhookEvent(string(events.EventComposeWebhookReceived), map[string]any{"stackId": stack.ID, "branch": branch, "commit": whPayload.After, "deliveryId": deliveryID})

	// Trigger async deployment so the webhook returns quickly
	go func() {
		deployCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		// Reset status from "pending" to "idle" so PullAndRedeploy can proceed
		refreshed, err := g.store.GetComposeStack(deployCtx, stack.ID)
		if err == nil {
			st := fromStoreComposeStack(refreshed)
			if st.GitUpdateStatus == "pending" {
				st.GitUpdateStatus = "idle"
				st.UpdatedAt = time.Now().UTC()
				if err := g.store.UpdateComposeStack(deployCtx, toStoreComposeStack(st)); err != nil {
					g.logger.Error("webhook deploy: failed to reset status",
						"stack_id", stack.ID, "error", err)
					return
				}
			}
		}

		if _, err := g.PullAndRedeploy(deployCtx, stack.ID); err != nil {
			g.logger.Error("webhook-triggered deployment failed",
				"stack_id", stack.ID, "commit", whPayload.After, "error", err)
		}
	}()

	return true, nil
}

func (g *GitOpsService) PollForUpdates(ctx context.Context) {
	if g.store == nil {
		return
	}
	stacks, err := g.store.ListComposeStacksDueForPoll(ctx)
	if err != nil {
		g.logger.Error("list stacks due for poll", "error", err)
		return
	}
	for _, st := range stacks {
		stack := fromStoreComposeStack(st)
		if !stack.GitAutoUpdate || stack.GitRepositoryURL == "" {
			continue
		}
		if err := g.checkAndPollStack(ctx, stack); err != nil {
			g.logger.Error("poll stack update failed", "stack", stack.ID, "error", err)
		}
	}
}

func (g *GitOpsService) checkAndPollStack(ctx context.Context, stack *ComposeStack) error {
	if g.gitSvc == nil {
		return ErrGitNotConfigured
	}
	clone, err := g.gitSvc.CloneRepo(ctx, stack.GitRepositoryURL, stack.GitBranch, stack.ID, stack.GitCredentialID)
	if err != nil {
		return fmt.Errorf("clone for poll: %w", err)
	}
	defer func() {
		if cleanupErr := g.gitSvc.CleanupClone(clone.Dir); cleanupErr != nil {
			g.logger.Error("cleanup clone for poll failed", "dir", clone.Dir, "error", cleanupErr)
		}
	}()
	if clone.CommitSHA == stack.GitCommitSHA {
		now := time.Now().UTC()
		nextPoll := now.Add(time.Duration(stack.GitPollIntervalSec) * time.Second)
		if stack.GitPollIntervalSec <= 0 {
			nextPoll = now.Add(5 * time.Minute)
		}
		stack.GitNextPollAt = &nextPoll
		stack.UpdatedAt = now
		_ = g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
		return nil
	}
	desired := clone.CommitSHA
	if stack.GitDesiredCommitSHA == desired {
		return nil
	}
	composeYAML, err := readComposeFromDir(clone.Dir, stack.ComposePath)
	if err != nil {
		return fmt.Errorf("read compose for poll: %w", err)
	}
	stack.GitDesiredCommitSHA = desired
	stack.GitUpdateStatus = "update_available"
	stack.GitUpdateError = ""
	stack.UpdatedAt = time.Now().UTC()
	_ = g.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
	if g.publisher != nil {
		_ = g.publisher.Publish(ctx, events.NewEnvelope(events.EventComposeUpdateAvailable, "compose", "stack", stack.ID, map[string]any{
			"commit": desired, "branch": stack.GitBranch,
			"preview": composeYAML,
		}))
	}
	g.store.DispatchWebhookEvent(string(events.EventComposeUpdateAvailable), map[string]any{"stackId": stack.ID, "commit": desired, "branch": stack.GitBranch})
	return nil
}

func (g *GitOpsService) StartPolling(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	g.logger.Info("starting gitops polling loop", "interval", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			g.logger.Info("gitops polling stopped")
			return
		case <-ticker.C:
			pollCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			g.PollForUpdates(pollCtx)
			cancel()
		}
	}
}
