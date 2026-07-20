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

type DeployRequest struct {
	GitSourceID    string            `json:"gitSourceId"`
	ImageTag       string            `json:"imageTag"`
	DockerfilePath string            `json:"dockerfilePath"`
	BuildArgs      map[string]string `json:"buildArgs"`
}

type DeployResult struct {
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

func (d *DeployService) CloneRepo(ctx context.Context, repoURL, branch, sourceID string) (*CloneResult, error) {
	if err := validateRepoURL(repoURL); err != nil {
		return nil, err
	}
	if !validateBranch(branch) {
		return nil, ErrInvalidBranch
	}

	targetDir := filepath.Join(d.tempBaseDir, "git-sources", sourceID)
	safeDir, err := safeClonePath(targetDir)
	if err != nil {
		return nil, err
	}

	if err := os.RemoveAll(safeDir); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("clean existing clone: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(safeDir), 0o750); err != nil {
		return nil, fmt.Errorf("create clone parent directory: %w", err)
	}

	cloneCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	if err := checkGitBinary(); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(cloneCtx, "git", "clone",
		"--depth", "1",
		"--single-branch",
		"--branch", branch,
		"--no-tags",
		"--config", "core.symlinks=false",
		repoURL, safeDir,
	)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("%w: %v (output: %s)", ErrCloneFailed, err, string(out))
	}

	commitSHA, err := resolveCommitSHA(cloneCtx, safeDir)
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

	return &CloneResult{
		Dir:         safeDir,
		CommitSHA:   commitSHA,
		Branch:      branch,
		ProjectType: projectType,
	}, nil
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

func (d *DeployService) DeployFromGit(ctx context.Context, req DeployRequest) (*DeployResult, error) {
	gs, err := d.store.GetGitSource(ctx, req.GitSourceID)
	if err != nil {
		return nil, fmt.Errorf("get git source: %w", err)
	}

	cloneResult, err := d.CloneRepo(ctx, gs.RepositoryURL, gs.Branch, gs.ID)
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

	return &DeployResult{
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

func validateRepoURL(repoURL string) error {
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

func safeClonePath(target string) (string, error) {
	cleaned := filepath.Clean(target)
	base := os.TempDir()
	gitSourcesBase := filepath.Join(base, "git-sources")
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
	entries := map[string]string{
		"Dockerfile":              "dockerfile",
		"docker-compose.yml":      "compose",
		"compose.yml":             "compose",
		"docker-compose.yaml":     "compose",
		"compose.yaml":            "compose",
	}
	for relPath, pt := range entries {
		if fi, err := os.Stat(filepath.Join(dir, relPath)); err == nil && !fi.IsDir() {
			return pt
		}
	}
	if fi, err := os.Stat(filepath.Join(dir, "index.html")); err == nil && !fi.IsDir() {
		return "static"
	}
	return projectTypeUnknown
}

func dirSize(dir string) (int64, error) {
	var size int64
	err := filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			if info.Mode()&os.ModeSymlink != 0 {
				target, readErr := os.Readlink(info.Name())
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
