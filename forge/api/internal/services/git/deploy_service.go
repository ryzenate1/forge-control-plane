package git

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gamepanel/forge/internal/store"
)

type CloneResult struct {
	Dir         string
	CommitSHA   string
	Branch      string
	ProjectType string
}

type DeployFromGitRequest struct {
	GitSourceID    string            `json:"gitSourceId"`
	ImageTag       string            `json:"imageTag"`
	DockerfilePath string            `json:"dockerfilePath"`
	BuildArgs      map[string]string `json:"buildArgs"`
}

type DeployFromGitResult struct {
	GitSource store.GitSource `json:"gitSource"`
	ImageTag  string          `json:"imageTag"`
	CommitSHA string          `json:"commitSha"`
}

type DeployService struct {
	store        *store.Store
	gitService   *Service
	logger       *slog.Logger
	registryHost string
	tempBaseDir  string
}

func NewDeployService(gitSvc *Service, s *store.Store, logger *slog.Logger, registryHost, tempBaseDir string) *DeployService {
	if tempBaseDir == "" {
		tempBaseDir = os.TempDir()
	}
	return &DeployService{
		store:        s,
		gitService:   gitSvc,
		logger:       logger,
		registryHost: registryHost,
		tempBaseDir:  tempBaseDir,
	}
}

func (d *DeployService) TriggerDeploy(ctx context.Context, sourceID string) error {
	_, err := d.store.GetGitSource(ctx, sourceID)
	return err
}

func (d *DeployService) CloneRepo(ctx context.Context, repoURL, branch, sourceID, credentialID string) (*CloneResult, error) {
	return d.cloneWithOptions(ctx, repoURL, branch, "", sourceID, credentialID)
}

func (d *DeployService) CloneAtCommit(ctx context.Context, repoURL, branch, commitSHA, sourceID, credentialID string) (*CloneResult, error) {
	if commitSHA == "" {
		return nil, fmt.Errorf("commitSHA is required for CloneAtCommit")
	}
	return d.cloneWithOptions(ctx, repoURL, branch, commitSHA, sourceID, credentialID)
}

func (d *DeployService) cloneWithOptions(ctx context.Context, repoURL, branch, commitSHA, sourceID, credentialID string) (*CloneResult, error) {
	var credential *store.GitCredential
	var sshKeyFile, askPassFile string

	if credentialID != "" && d.store != nil {
		cred, err := d.store.GetGitCredentialUnmasked(ctx, credentialID)
		if err != nil {
			return nil, fmt.Errorf("get credential: %w", err)
		}
		credential = &cred

		if cred.CredentialType == store.GitCredentialSSHKey {
			keyFile, err := writeSSHKeyFile(cred.Credential)
			if err != nil {
				return nil, fmt.Errorf("write ssh key: %w", err)
			}
			sshKeyFile = keyFile
			defer os.Remove(keyFile)
		} else if cred.CredentialType == store.GitCredentialHTTPSToken {
			af, err := writeAskPassScript(cred.Credential, "x-oauth-basic")
			if err != nil {
				return nil, fmt.Errorf("write askpass script: %w", err)
			}
			askPassFile = af
			defer os.Remove(af)
		} else if cred.CredentialType == store.GitCredentialHTTPSPass {
			parts := strings.SplitN(cred.Credential, ":", 2)
			user, pass := parts[0], ""
			if len(parts) == 2 {
				pass = parts[1]
			}
			af, err := writeAskPassScript(user, pass)
			if err != nil {
				return nil, fmt.Errorf("write askpass script: %w", err)
			}
			askPassFile = af
			defer os.Remove(af)
		}
	}

	if credential != nil && credential.CredentialType == store.GitCredentialSSHKey {
		if err := validateSSHRepoURL(repoURL); err != nil {
			return nil, err
		}
	} else {
		if err := ValidateRepoURL(repoURL); err != nil {
			return nil, err
		}
	}

	if !validateBranch(branch) {
		return nil, ErrInvalidBranch
	}

	targetDir := filepath.Join(d.tempBaseDir, "git-sources", sourceID)
	safeDir, err := safeClonePath(targetDir, d.tempBaseDir)
	if err != nil {
		return nil, err
	}

	if err := os.RemoveAll(safeDir); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("clean existing clone: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(safeDir), 0o750); err != nil {
		return nil, fmt.Errorf("create clone parent directory: %w", err)
	}

	var cleanup bool
	defer func() {
		if cleanup {
			os.RemoveAll(safeDir)
		}
	}()
	cleanup = true

	cloneCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	if err := checkGitBinary(); err != nil {
		return nil, err
	}

	args := []string{"clone", "--depth", "1", "--single-branch", "--branch", branch, "--no-tags", "--config", "core.symlinks=false"}

	args = append(args, repoURL, safeDir)

	cmd := exec.CommandContext(cloneCtx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if sshKeyFile != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %q -o StrictHostKeyChecking=accept-new", sshKeyFile))
	}
	if askPassFile != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_ASKPASS=%s", askPassFile))
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("%w: %v (output: %s)", ErrCloneFailed, err, string(out))
	}

	if commitSHA != "" {
		fetchCmd := exec.CommandContext(cloneCtx, "git", "-C", safeDir, "fetch", "--depth", "1", "origin", commitSHA)
		fetchCmd.Env = cmd.Env
		if out, err := fetchCmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("fetch commit %s: %v (output: %s)", commitSHA, err, string(out))
		}

		checkoutCmd := exec.CommandContext(cloneCtx, "git", "-C", safeDir, "checkout", commitSHA)
		checkoutCmd.Env = cmd.Env
		if out, err := checkoutCmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("checkout commit %s: %v (output: %s)", commitSHA, err, string(out))
		}
	}

	resolvedSHA, err := resolveCommitSHA(cloneCtx, safeDir)
	if err != nil {
		return nil, err
	}

	projectType := detectProjectType(safeDir)

	size, err := dirSize(safeDir)
	if err != nil {
		_ = os.RemoveAll(safeDir)
		return nil, fmt.Errorf("compute build context size: %w", err)
	}
	if size > maxBuildContextSize {
		_ = os.RemoveAll(safeDir)
		return nil, ErrBuildContextTooBig
	}

	cleanup = false
	return &CloneResult{
		Dir:         safeDir,
		CommitSHA:   resolvedSHA,
		Branch:      branch,
		ProjectType: projectType,
	}, nil
}

func writeSSHKeyFile(privateKey string) (string, error) {
	f, err := os.CreateTemp("", "git-ssh-key-*")
	if err != nil {
		return "", err
	}
	defer f.Close()

	if err := os.Chmod(f.Name(), 0600); err != nil {
		os.Remove(f.Name())
		return "", err
	}

	if _, err := f.WriteString(privateKey); err != nil {
		os.Remove(f.Name())
		return "", err
	}

	return f.Name(), nil
}

func writeAskPassScript(username, password string) (string, error) {
	script := fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\n    *Username*) echo %s ;;\n    *Password*) echo %s ;;\nesac\n", shellQuote(username), shellQuote(password))
	f, err := os.CreateTemp("", "git-askpass-*")
	if err != nil {
		return "", err
	}
	defer f.Close()

	if err := os.Chmod(f.Name(), 0700); err != nil {
		os.Remove(f.Name())
		return "", err
	}

	if _, err := f.WriteString(script); err != nil {
		os.Remove(f.Name())
		return "", err
	}

	return f.Name(), nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func validateSSHRepoURL(repoURL string) error {
	repoURL = strings.TrimSpace(repoURL)
	if repoURL == "" {
		return ErrInvalidRepoURL
	}
	if !strings.HasPrefix(repoURL, "git@") {
		return ValidateRepoURL(repoURL)
	}
	if strings.Contains(repoURL, ";") || strings.Contains(repoURL, "`") || strings.Contains(repoURL, "..") {
		return ErrInvalidRepoURL
	}
	return nil
}

func (d *DeployService) CleanupClone(cloneDir string) error {
	cloneDir = filepath.Clean(cloneDir)
	gitSourcesPrefix := filepath.Join(d.tempBaseDir, "git-sources")
	if !strings.HasPrefix(cloneDir, gitSourcesPrefix) {
		return fmt.Errorf("clone directory %q is not under the expected git-sources tree", cloneDir)
	}
	if err := os.RemoveAll(cloneDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove clone directory: %w", err)
	}
	return nil
}

func (d *DeployService) DeployFromGit(ctx context.Context, req DeployFromGitRequest) (*DeployFromGitResult, error) {
	gs, err := d.store.GetGitSource(ctx, req.GitSourceID)
	if err != nil {
		return nil, fmt.Errorf("get git source: %w", err)
	}

	cloneResult, err := d.CloneRepo(ctx, gs.RepositoryURL, gs.Branch, gs.ID, "")
	if err != nil {
		return nil, fmt.Errorf("clone repo: %w", err)
	}
	defer d.CleanupClone(cloneResult.Dir)

	if cloneResult.ProjectType == projectTypeUnknown {
		return nil, ErrNoProjectType
	}

	imageTag := req.ImageTag
	if imageTag == "" {
		imageTag = fmt.Sprintf("%s/%s:%s", strings.TrimRight(d.registryHost, "/"), gs.RepositoryName, cloneResult.CommitSHA[:8])
	}

	dockerfilePath := req.DockerfilePath
	if dockerfilePath == "" {
		dockerfilePath = filepath.Join(cloneResult.Dir, "Dockerfile")
	}

	if err := dockerBuild(ctx, cloneResult.Dir, dockerfilePath, imageTag, req.BuildArgs); err != nil {
		return nil, fmt.Errorf("docker build: %w", err)
	}

	if err := dockerPush(ctx, imageTag); err != nil {
		return nil, fmt.Errorf("docker push: %w", err)
	}

	_ = d.store.UpdateGitSourceDeploy(ctx, gs.ID, cloneResult.CommitSHA, "", "")

	return &DeployFromGitResult{
		GitSource: gs,
		ImageTag:  imageTag,
		CommitSHA: cloneResult.CommitSHA,
	}, nil
}

func dockerBuild(ctx context.Context, contextDir, dockerfilePath, imageTag string, buildArgs map[string]string) error {
	args := []string{"build", "-t", imageTag, "-f", dockerfilePath}
	for k, v := range buildArgs {
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", k, v))
	}
	args = append(args, contextDir)

	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker build error: %w (output: %s)", err, string(out))
	}
	return nil
}

func dockerPush(ctx context.Context, imageTag string) error {
	cmd := exec.CommandContext(ctx, "docker", "push", imageTag)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker push error: %w (output: %s)", err, string(out))
	}
	return nil
}

var allowedGitHosts = map[string]bool{
	"github.com":    true,
	"gitlab.com":    true,
	"bitbucket.org": true,
	"gitea.com":     true,
	"codeberg.org":  true,
}

func checkGitBinary() error {
	if _, err := exec.LookPath("git"); err != nil {
		return ErrCmdUnavailable
	}
	return nil
}

func allowedHost(repoURL string) bool {
	repoURL = strings.TrimSpace(repoURL)
	if strings.HasPrefix(repoURL, "git@") {
		return false
	}
	parsed, err := url.Parse(repoURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Host)
	if strings.Contains(host, ":") {
		host = host[:strings.Index(host, ":")]
	}
	if allowedGitHosts[host] {
		return true
	}
	for allowed := range allowedGitHosts {
		if strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

func validateBranch(branch string) bool {
	if branch == "" {
		return false
	}
	if strings.Contains(branch, "..") || strings.Contains(branch, "/.") || strings.Contains(branch, " ") {
		return false
	}
	for _, c := range branch {
		if c == '\\' || c == '\'' || c == '"' || c == '`' || c == '$' || c == '&' || c == '|' || c == ';' {
			return false
		}
	}
	return true
}

func ValidateRepoURL(repoURL string) error {
	repoURL = strings.TrimSpace(repoURL)
	if repoURL == "" {
		return ErrInvalidRepoURL
	}
	if strings.HasPrefix(repoURL, "git@") {
		return fmt.Errorf("SSH URLs are not supported for public cloning: %w", ErrInvalidRepoURL)
	}
	parsed, err := url.Parse(repoURL)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRepoURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: only http/https URLs are supported", ErrInvalidRepoURL)
	}
	if !allowedHost(repoURL) {
		return fmt.Errorf("%w: host %q is not in the allow-list", ErrHostNotAllowed, parsed.Host)
	}
	if strings.Contains(repoURL, "..") || strings.Contains(repoURL, ";") || strings.Contains(repoURL, "`") {
		return ErrInvalidRepoURL
	}
	return nil
}

func safeClonePath(target, tempBase string) (string, error) {
	cleaned := filepath.Clean(target)
	gitSourcesBase := filepath.Join(tempBase, "git-sources")
	if !strings.HasPrefix(cleaned, gitSourcesBase) {
		return "", fmt.Errorf("clone path must be under %s", gitSourcesBase)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Dir(cleaned))
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("resolve clone parent: %w", err)
	}
	if err == nil {
		if !strings.HasPrefix(resolved, gitSourcesBase) {
			return "", fmt.Errorf("resolved clone parent %q escapes git-sources tree", resolved)
		}
	}
	return cleaned, nil
}

func resolveCommitSHA(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve commit SHA: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

func detectProjectType(dir string) string {
	type ptEntry struct {
		filename string
		pt       string
	}
	entries := []ptEntry{
		{"Dockerfile", "dockerfile"},
		{"docker-compose.yml", "compose"},
		{"compose.yml", "compose"},
		{"docker-compose.yaml", "compose"},
		{"compose.yaml", "compose"},
	}
	for _, e := range entries {
		if fi, err := os.Stat(filepath.Join(dir, e.filename)); err == nil && !fi.IsDir() {
			return e.pt
		}
	}
	if fi, err := os.Stat(filepath.Join(dir, "index.html")); err == nil && !fi.IsDir() {
		return "static"
	}
	return projectTypeUnknown
}

func (d *DeployService) TriggerDeployment(ctx context.Context, req *DeployRequest) (*DeployFromGitResult, error) {
	return d.DeployFromGit(ctx, DeployFromGitRequest{
		GitSourceID:    req.GitSourceID,
		ImageTag:       req.ImageTag,
		DockerfilePath: req.DockerfilePath,
		BuildArgs:      req.BuildArgs,
	})
}

func (d *DeployService) GetDeploymentStatus(ctx context.Context, gitSourceID string) (*store.GitDeployment, error) {
	return d.store.GetLatestGitDeployment(ctx, gitSourceID)
}

func (d *DeployService) ListDeployments(ctx context.Context, gitSourceID string, limit int) ([]store.GitDeployment, error) {
	return d.store.ListGitDeployments(ctx, gitSourceID, limit)
}

func (d *DeployService) CancelDeployment(ctx context.Context, deploymentID string) error {
	return d.store.UpdateGitDeployment(ctx, deploymentID, "cancelled", "", "")
}

func dirSize(dir string) (int64, error) {
	var size int64
	err := filepath.Walk(dir, func(walkPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			if info.Mode()&os.ModeSymlink != 0 {
				target, readErr := os.Readlink(walkPath)
				if readErr != nil || !filepath.IsLocal(target) || strings.Contains(target, "..") {
					return fmt.Errorf("rejected symlink in build context: %q -> %q", info.Name(), target)
				}
				return nil
			}
			size += info.Size()
		}
		return nil
	})
	return size, err
}
