package compose

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"
)

type DaemonClient interface {
	ComposeDeploy(ctx context.Context, baseURL, nodeToken string, req daemon.ComposeDeployRequest) (daemon.ComposeDeployResponse, error)
	ComposeStop(ctx context.Context, baseURL, nodeToken, stackID string) (daemon.ComposeOperationResponse, error)
	ComposeDelete(ctx context.Context, baseURL, nodeToken, stackID string) (daemon.ComposeOperationResponse, error)
	ComposeStatus(ctx context.Context, baseURL, nodeToken, stackID string) (daemon.ComposeStatusResponse, error)
}

type GitOpsControllerStore interface {
	ListComposeStacksPendingUpdate(ctx context.Context) ([]store.ComposeStack, error)
	ClaimComposeStackForUpdate(ctx context.Context, stackID, workerID string) (*store.ComposeStack, error)
	ReleaseComposeStackClaim(ctx context.Context, stackID, workerID string) error
	UpdateComposeStack(ctx context.Context, stack *store.ComposeStack) error
	GetNode(ctx context.Context, nodeID string) (store.Node, error)
	GetNodeDaemonCredential(ctx context.Context, nodeID string) (string, error)
	ListComposeStacksStaleClaims(ctx context.Context, staleDuration time.Duration) ([]store.ComposeStack, error)
}

type GitOpsController struct {
	store     GitOpsControllerStore
	gitSvc    GitCloneService
	compose   *Service
	daemon    DaemonClient
	publisher events.Publisher
	logger    *slog.Logger
	interval  time.Duration
	workerID  string
	stopCh    chan struct{}
	stopOnce  sync.Once
}

func NewGitOpsController(
	store GitOpsControllerStore,
	gitSvc GitCloneService,
	compose *Service,
	daemon DaemonClient,
	publisher events.Publisher,
	logger *slog.Logger,
	workerID string,
) (*GitOpsController, error) {
	if store == nil {
		return nil, errors.New("store required")
	}
	if gitSvc == nil {
		return nil, errors.New("git clone service required")
	}
	if compose == nil {
		return nil, errors.New("compose service required")
	}
	if daemon == nil {
		return nil, errors.New("daemon client required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	if workerID == "" {
		hostname, _ := os.Hostname()
		workerID = fmt.Sprintf("gitops-controller-%s-%d", hostname, time.Now().UnixNano())
	}
	return &GitOpsController{
		store:     store,
		gitSvc:    gitSvc,
		compose:   compose,
		daemon:    daemon,
		publisher: publisher,
		logger:    logger,
		interval:  15 * time.Second,
		workerID:  workerID,
		stopCh:    make(chan struct{}),
		stopOnce:  sync.Once{},
	}, nil
}

func (c *GitOpsController) Start(ctx context.Context) {
	c.logger.Info("gitops controller started", "workerID", c.workerID, "interval", c.interval)
	go c.run(ctx)
}

func (c *GitOpsController) Stop() {
	c.logger.Info("gitops controller stopping")
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})
}

func (c *GitOpsController) run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	if err := c.processPending(ctx); err != nil {
		c.logger.Error("gitops controller initial processing failed", "error", err)
	}

	if err := c.recoverStaleClaims(ctx); err != nil {
		c.logger.Error("gitops controller stale claim recovery failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("gitops controller context cancelled")
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			processCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			if err := c.processPending(processCtx); err != nil {
				c.logger.Error("gitops controller processing failed", "error", err)
			}
			cancel()
		}
	}
}

func (c *GitOpsController) processPending(ctx context.Context) error {
	stacks, err := c.store.ListComposeStacksPendingUpdate(ctx)
	if err != nil {
		return fmt.Errorf("list pending stacks: %w", err)
	}
	if len(stacks) == 0 {
		return nil
	}

	for _, s := range stacks {
		stackID := s.ID
		claimed, err := c.store.ClaimComposeStackForUpdate(ctx, stackID, c.workerID)
		if err != nil {
			c.logger.Error("claim stack for update failed", "stackID", stackID, "error", err)
			continue
		}
		if claimed == nil {
			continue
		}

		stack := fromStoreComposeStack(*claimed)
		if err := c.deployStack(ctx, stack); err != nil {
			c.logger.Error("deploy stack failed", "stackID", stack.ID, "error", err)
			stack.GitUpdateStatus = "failed"
			stack.GitUpdateError = err.Error()
			stack.GitUpdateClaimedBy = nil
			stack.GitUpdateClaimedAt = nil
			stack.UpdatedAt = time.Now().UTC()
			_ = c.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
		}
	}

	return nil
}

func (c *GitOpsController) deployStack(ctx context.Context, stack *ComposeStack) error {
	if c.gitSvc == nil {
		_ = c.store.ReleaseComposeStackClaim(ctx, stack.ID, c.workerID)
		return fmt.Errorf("git clone service not configured")
	}

	desiredSHA := stack.GitDesiredCommitSHA
	if desiredSHA == "" {
		_ = c.store.ReleaseComposeStackClaim(ctx, stack.ID, c.workerID)
		return fmt.Errorf("no desired commit SHA for stack %s", stack.ID)
	}

	clone, err := c.gitSvc.CloneRepo(ctx, stack.GitRepositoryURL, stack.GitBranch, stack.ID, stack.GitCredentialID)
	if err != nil {
		_ = c.store.ReleaseComposeStackClaim(ctx, stack.ID, c.workerID)
		return fmt.Errorf("clone repository: %w", err)
	}
	defer func() {
		if cleanupErr := c.gitSvc.CleanupClone(clone.Dir); cleanupErr != nil {
			c.logger.Error("cleanup clone failed", "dir", clone.Dir, "error", cleanupErr)
		}
	}()

	if clone.CommitSHA != desiredSHA {
		_ = c.store.ReleaseComposeStackClaim(ctx, stack.ID, c.workerID)
		return fmt.Errorf("cloned commit %s does not match desired %s", clone.CommitSHA, desiredSHA)
	}

	composeYAML, err := readComposeFromDir(clone.Dir, stack.ComposePath)
	if err != nil {
		_ = c.store.ReleaseComposeStackClaim(ctx, stack.ID, c.workerID)
		return fmt.Errorf("read compose from repository: %w", err)
	}

	if c.compose != nil {
		validation := c.compose.Validate([]byte(composeYAML), clone.Dir)
		if !validation.Valid {
			errMsg := "invalid compose yaml"
			if len(validation.Errors) > 0 {
				errMsg = validation.Errors[0].Message
			}
			_ = c.store.ReleaseComposeStackClaim(ctx, stack.ID, c.workerID)
			return fmt.Errorf("%w: %s", ErrInvalidCompose, errMsg)
		}
	}

	node, err := c.store.GetNode(ctx, stack.NodeID)
	if err != nil {
		_ = c.store.ReleaseComposeStackClaim(ctx, stack.ID, c.workerID)
		return fmt.Errorf("node not found: %w", err)
	}

	nodeCredential, err := c.store.GetNodeDaemonCredential(ctx, stack.NodeID)
	if err != nil {
		_ = c.store.ReleaseComposeStackClaim(ctx, stack.ID, c.workerID)
		return fmt.Errorf("node credential not found: %w", err)
	}

	prevCompose := stack.ComposeYAML
	prevCommit := stack.GitCommitSHA

	client := daemon.NewClient()
	deployResp, deployErr := client.ComposeDeploy(ctx, node.BaseURL, nodeCredential, daemon.ComposeDeployRequest{
		StackID:     stack.ID,
		ComposeYAML: composeYAML,
		EnvVars:     stack.EnvVars,
	})
	if deployErr != nil {
		_ = c.store.ReleaseComposeStackClaim(ctx, stack.ID, c.workerID)
		return c.rollbackDeploy(ctx, stack, prevCompose, prevCommit, fmt.Errorf("deploy updated stack: %w", deployErr))
	}
	_ = deployResp

	statusResp, statusErr := client.ComposeStatus(ctx, node.BaseURL, nodeCredential, stack.ID)
	if statusErr != nil {
		_ = c.store.ReleaseComposeStackClaim(ctx, stack.ID, c.workerID)
		return fmt.Errorf("health check after deploy failed: %w", statusErr)
	}

	allHealthy := true
	healthMsg := ""
	for _, svc := range statusResp.Services {
		if svc.Status != "running" {
			allHealthy = false
			healthMsg = fmt.Sprintf("service %s is %s (%s)", svc.Name, svc.Status, svc.State)
			break
		}
	}

	if !allHealthy {
		_ = c.store.ReleaseComposeStackClaim(ctx, stack.ID, c.workerID)
		return c.rollbackDeploy(ctx, stack, prevCompose, prevCommit, fmt.Errorf("health check failed: %s", healthMsg))
	}

	now := time.Now().UTC()
	stack.GitPreviousCommitSHA = prevCommit
	stack.GitPreviousCompose = prevCompose
	stack.GitCommitSHA = desiredSHA
	stack.GitDesiredCommitSHA = desiredSHA
	stack.ComposeYAML = composeYAML
	stack.ComposeHash = computeHash(composeYAML)
	stack.Status = StackStatusRunning
	stack.GitUpdateStatus = "idle"
	stack.GitUpdateError = ""
	stack.Error = ""
	stack.GitUpdateClaimedBy = nil
	stack.GitUpdateClaimedAt = nil
	stack.UpdatedAt = now
	_ = c.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
	_ = c.store.ReleaseComposeStackClaim(ctx, stack.ID, c.workerID)

	if c.publisher != nil {
		_ = c.publisher.Publish(ctx, events.NewEnvelope(events.EventComposeUpdated, "compose", "stack", stack.ID, map[string]any{
			"commit": desiredSHA, "branch": stack.GitBranch,
		}))
	}

	return nil
}

func (c *GitOpsController) rollbackDeploy(ctx context.Context, stack *ComposeStack, prevCompose, prevCommit string, originalErr error) error {
	c.logger.Error("attempting rollback after deploy failure", "stackID", stack.ID, "error", originalErr)
	stack.GitUpdateStatus = "rolling_back"
	stack.UpdatedAt = time.Now().UTC()
	_ = c.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))

	node, err := c.store.GetNode(ctx, stack.NodeID)
	if err != nil {
		return fmt.Errorf("rollback: node not found: %w (original: %v)", err, originalErr)
	}

	nodeCredential, err := c.store.GetNodeDaemonCredential(ctx, stack.NodeID)
	if err != nil {
		return fmt.Errorf("rollback: node credential not found: %w (original: %v)", err, originalErr)
	}

	client := daemon.NewClient()
	stopResp, stopErr := client.ComposeStop(ctx, node.BaseURL, nodeCredential, stack.ID)
	if stopErr != nil {
		return fmt.Errorf("rollback stop failed: %w (original: %v)", stopErr, originalErr)
	}
	_ = stopResp

	deleteResp, delErr := client.ComposeDelete(ctx, node.BaseURL, nodeCredential, stack.ID)
	if delErr != nil && !isNotFoundError(delErr) {
		return fmt.Errorf("rollback delete failed: %w (original: %v)", delErr, originalErr)
	}
	_ = deleteResp

	deployResp, deployErr := client.ComposeDeploy(ctx, node.BaseURL, nodeCredential, daemon.ComposeDeployRequest{
		StackID:     stack.ID,
		ComposeYAML: prevCompose,
		EnvVars:     stack.EnvVars,
	})
	if deployErr != nil {
		return fmt.Errorf("rollback deploy failed: %w (original: %v)", deployErr, originalErr)
	}
	_ = deployResp

	now := time.Now().UTC()
	stack.Status = StackStatusRunning
	stack.ComposeYAML = prevCompose
	stack.ComposeHash = computeHash(prevCompose)
	stack.GitCommitSHA = prevCommit
	stack.GitDesiredCommitSHA = prevCommit
	stack.GitUpdateStatus = "idle"
	stack.GitUpdateError = ""
	stack.GitUpdateClaimedBy = nil
	stack.GitUpdateClaimedAt = nil
	stack.Error = "rolled back after deploy failure: " + originalErr.Error()
	stack.UpdatedAt = now
	_ = c.store.UpdateComposeStack(ctx, toStoreComposeStack(stack))
	_ = c.store.ReleaseComposeStackClaim(ctx, stack.ID, c.workerID)

	if c.publisher != nil {
		_ = c.publisher.Publish(ctx, events.NewEnvelope(events.EventComposeRollbackCompleted, "compose", "stack", stack.ID, map[string]any{
			"commit": prevCommit, "branch": stack.GitBranch,
		}))
	}

	return nil
}

func (c *GitOpsController) recoverStaleClaims(ctx context.Context) error {
	stacks, err := c.store.ListComposeStacksStaleClaims(ctx, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("list stale claims: %w", err)
	}
	for _, s := range stacks {
		c.logger.Warn("recovering stale claim", "stackID", s.ID, "status", s.GitUpdateStatus, "claimedBy", s.GitUpdateClaimedBy)
		s.GitUpdateClaimedBy = nil
		s.GitUpdateClaimedAt = nil
		s.GitUpdateStatus = "idle"
		s.UpdatedAt = time.Now().UTC()
		_ = c.store.UpdateComposeStack(ctx, &s)
	}
	return nil
}
