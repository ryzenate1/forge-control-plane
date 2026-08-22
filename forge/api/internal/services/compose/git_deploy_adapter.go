package compose

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gitsvc "gamepanel/forge/internal/services/git"
)

// GitDeployAdapter implements gitsvc.ComposeServiceInterface by routing
// git-compose deployments through the live GitOpsService deploy path, so the
// GitDeploymentService compose branch performs a real deployment instead of
// validate-and-complete.
type GitDeployAdapter struct {
	gitops *GitOpsService
	// defaultNodeID is used when the deploy request does not select a node.
	defaultNodeID string
	// Default resource envelope for stacks created via git deployments.
	memoryMB  int64
	cpuShares int64
	diskMB    int64
}

// Compile-time check that the adapter satisfies the git package interface.
var _ gitsvc.ComposeServiceInterface = (*GitDeployAdapter)(nil)

// NewGitDeployAdapter creates an adapter around a GitOpsService. Zero resource
// values fall back to conservative defaults.
func NewGitDeployAdapter(gitops *GitOpsService, defaultNodeID string, memoryMB, cpuShares, diskMB int64) *GitDeployAdapter {
	if memoryMB <= 0 {
		memoryMB = 1024
	}
	if cpuShares <= 0 {
		cpuShares = 1024
	}
	if diskMB <= 0 {
		diskMB = 10240
	}
	return &GitDeployAdapter{
		gitops:        gitops,
		defaultNodeID: defaultNodeID,
		memoryMB:      memoryMB,
		cpuShares:     cpuShares,
		diskMB:        diskMB,
	}
}

// ValidateComposeContent validates compose content through the compose
// service, flattening validation errors into a single error.
func (a *GitDeployAdapter) ValidateComposeContent(content []byte, workingDir string) error {
	if a.gitops == nil || a.gitops.compose == nil {
		return errors.New("compose service is not configured")
	}
	result := a.gitops.compose.ValidateCompose(content, workingDir)
	if result == nil || result.Valid {
		return nil
	}
	messages := make([]string, 0, len(result.Errors))
	for _, verr := range result.Errors {
		messages = append(messages, fmt.Sprintf("%s: %s", verr.Field, verr.Message))
	}
	return fmt.Errorf("compose validation failed: %s", strings.Join(messages, "; "))
}

// DeployComposeFromGit deploys the git-backed compose project as a compose
// stack through GitOpsService.DeployFromGit and returns a deploy summary log.
func (a *GitDeployAdapter) DeployComposeFromGit(ctx context.Context, req gitsvc.ComposeGitDeployRequest) (string, error) {
	if a.gitops == nil {
		return "", errors.New("gitops service is not configured")
	}
	nodeID := req.NodeID
	if nodeID == "" {
		nodeID = a.defaultNodeID
	}
	if nodeID == "" {
		return "", errors.New("no node selected for compose deployment and no default node configured")
	}
	name := req.RepositoryName
	if name == "" {
		name = req.GitSourceID
	}
	result, err := a.gitops.DeployFromGit(ctx, GitDeployFromGitRequest{
		UserID:        req.UserID,
		NodeID:        nodeID,
		Name:          name,
		RepositoryURL: req.RepositoryURL,
		Branch:        req.Branch,
		CredentialID:  req.CredentialID,
		MemoryMB:      a.memoryMB,
		CPUShares:     a.cpuShares,
		DiskMB:        a.diskMB,
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("deployed compose stack %s (commit %s, branch %s, status %s, %d services)",
		result.StackID, result.CommitSHA, result.Branch, result.Status, len(result.Services)), nil
}
