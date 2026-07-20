package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gamepanel/beacon/internal/runtime"
	"gamepanel/beacon/internal/serverid"
)

const maxApplicationBuildBody = 128 * 1024

var gitBranchPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,254}$`)

func (s *Server) buildApplication(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		http.Error(w, errRuntimeUnavailable.Error(), http.StatusServiceUnavailable)
		return
	}
	builder, ok := s.runtime.(runtime.Builder)
	if !ok {
		http.Error(w, "configured runtime does not support Dockerfile builds", http.StatusNotImplemented)
		return
	}
	var request struct {
		WorkloadID     string            `json:"workloadId"`
		RepositoryURL  string            `json:"repositoryUrl"`
		Branch         string            `json:"branch"`
		BaseDirectory  string            `json:"baseDirectory"`
		DockerfilePath string            `json:"dockerfilePath"`
		BuildArgs      map[string]string `json:"buildArgs"`
		ImageTag       string            `json:"imageTag"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxApplicationBuildBody)
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "invalid build request", http.StatusBadRequest)
		return
	}
	if err := validateApplicationBuildRequest(request.WorkloadID, request.RepositoryURL, request.Branch, request.BaseDirectory, request.DockerfilePath, request.ImageTag); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	buildRoot := filepath.Join(s.dataDir, ".builds", request.WorkloadID, fmt.Sprintf("%d", time.Now().UTC().UnixNano()))
	if err := os.MkdirAll(filepath.Dir(buildRoot), 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(buildRoot)
	if err := clonePublicRepository(r.Context(), request.RepositoryURL, request.Branch, buildRoot); err != nil {
		http.Error(w, "clone application source: "+err.Error(), http.StatusBadGateway)
		return
	}
	commit, err := gitCommit(r.Context(), buildRoot)
	if err != nil {
		http.Error(w, "read source revision: "+err.Error(), http.StatusBadGateway)
		return
	}
	contextDir := filepath.Join(buildRoot, request.BaseDirectory)
	result, err := builder.Build(r.Context(), runtime.BuildRequest{ContextDir: contextDir, DockerfilePath: request.DockerfilePath, ImageTag: request.ImageTag, BuildArgs: request.BuildArgs})
	if err != nil {
		http.Error(w, "build application image: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"image": result.Image, "commit": commit, "log": result.Log})
}

func validateApplicationBuildRequest(workloadID, repositoryURL, branch, baseDirectory, dockerfilePath, imageTag string) error {
	if err := serverid.Validate(workloadID); err != nil {
		return err
	}
	parsed, err := url.Parse(strings.TrimSpace(repositoryURL))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("repository must be an HTTPS URL without embedded credentials")
	}
	if !allowedGitHost(parsed.Hostname()) {
		return errors.New("repository host is not allowed on this Beacon")
	}
	if branch != "" && (!gitBranchPattern.MatchString(branch) || strings.Contains(branch, "..")) {
		return errors.New("branch contains unsupported characters")
	}
	if err := safeBuildRelativePath(baseDirectory, "base directory"); err != nil {
		return err
	}
	if err := safeBuildRelativePath(dockerfilePath, "dockerfile path"); err != nil {
		return err
	}
	if strings.TrimSpace(imageTag) == "" || strings.ContainsAny(imageTag, " \t\n") {
		return errors.New("image tag is required")
	}
	return nil
}

func allowedGitHost(host string) bool {
	allowed := os.Getenv("DAEMON_GIT_ALLOWED_HOSTS")
	if strings.TrimSpace(allowed) == "" {
		allowed = "github.com,gitlab.com,bitbucket.org"
	}
	host = strings.ToLower(strings.TrimSpace(host))
	for _, raw := range strings.Split(allowed, ",") {
		pattern := strings.ToLower(strings.TrimSpace(raw))
		if pattern == host || (strings.HasPrefix(pattern, "*.") && strings.HasSuffix(host, pattern[1:])) {
			return true
		}
	}
	return false
}

func safeBuildRelativePath(value, field string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	clean := filepath.Clean(value)
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || strings.Contains(value, "\\") {
		return errors.New(field + " must stay inside the cloned repository")
	}
	return nil
}

func clonePublicRepository(ctx context.Context, repositoryURL, branch, destination string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return errors.New("git is not installed on this Beacon")
	}
	args := []string{"clone", "--depth", "1"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, "--", repositoryURL, destination)
	output, err := exec.CommandContext(ctx, "git", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %s", truncateBuildOutput(string(output)))
	}
	return nil
}

func gitCommit(ctx context.Context, repository string) (string, error) {
	output, err := exec.CommandContext(ctx, "git", "-C", repository, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %s", truncateBuildOutput(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func truncateBuildOutput(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 4096 {
		return value[:4096] + "…"
	}
	return value
}
