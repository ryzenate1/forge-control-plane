package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	gitCloneDir  = ".git-clones"
	maxCloneSize = 1024 * 1024 * 1024
	maxCloneTime = 10 * time.Minute
	maxBuildTime = 30 * time.Minute
	maxPushTime  = 15 * time.Minute
)

var restrictedNetworks = []*net.IPNet{
	{IP: net.ParseIP("10.0.0.0"), Mask: net.CIDRMask(8, 32)},
	{IP: net.ParseIP("172.16.0.0"), Mask: net.CIDRMask(12, 32)},
	{IP: net.ParseIP("192.168.0.0"), Mask: net.CIDRMask(16, 32)},
	{IP: net.ParseIP("127.0.0.0"), Mask: net.CIDRMask(8, 32)},
	{IP: net.ParseIP("169.254.0.0"), Mask: net.CIDRMask(16, 32)},
	{IP: net.ParseIP("0.0.0.0"), Mask: net.CIDRMask(8, 32)},
	{IP: net.ParseIP("::1"), Mask: net.CIDRMask(128, 128)},
	{IP: net.ParseIP("fe80::"), Mask: net.CIDRMask(10, 128)},
}

type gitCloneRequest struct {
	RepoURL   string `json:"repoUrl"`
	Branch    string `json:"branch"`
	SourceID  string `json:"sourceId"`
	CommitSHA string `json:"commitSha,omitempty"`
}

type gitCloneResponse struct {
	WorkspaceID string `json:"workspaceId"`
	CommitSHA   string `json:"commitSha"`
}

type gitBuildRequest struct {
	WorkspaceID    string            `json:"workspaceId"`
	ImageTag       string            `json:"imageTag"`
	DockerfilePath string            `json:"dockerfilePath"`
	BuildArgs      map[string]string `json:"buildArgs"`
}

type gitBuildResponse struct {
	ImageTag string `json:"imageTag"`
}

type gitCleanupRequest struct {
	WorkspaceID string `json:"workspaceId"`
}

func (s *Server) handleGitClone(w http.ResponseWriter, r *http.Request) {
	var req gitCloneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	if req.RepoURL == "" || req.Branch == "" || req.SourceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "repoUrl, branch, and sourceId are required"})
		return
	}

	if strings.HasPrefix(req.RepoURL, "git@") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "SSH URLs are not supported"})
		return
	}

	if !strings.HasPrefix(req.RepoURL, "https://") && !strings.HasPrefix(req.RepoURL, "http://") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "only http/https URLs are supported"})
		return
	}

	if strings.Contains(req.RepoURL, "..") || strings.Contains(req.RepoURL, ";") || strings.Contains(req.RepoURL, "`") || strings.Contains(req.RepoURL, "@") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid repo URL"})
		return
	}

	if strings.Contains(req.Branch, "..") || strings.Contains(req.Branch, "/.") || strings.Contains(req.Branch, " ") || strings.Contains(req.Branch, "`") || strings.Contains(req.Branch, ";") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid branch name"})
		return
	}

	if strings.Contains(req.SourceID, "..") || strings.Contains(req.SourceID, "/") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid source id"})
		return
	}

	if req.CommitSHA != "" && !isHex40(req.CommitSHA) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid commit SHA"})
		return
	}

	// SSRF protection: validate resolved IP
	hostname := extractHost(req.RepoURL)
	if isRestrictedHost(hostname) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "repository host resolves to restricted address"})
		return
	}

	cloneBase := filepath.Join(s.dataDir, gitCloneDir)
	if err := os.MkdirAll(cloneBase, 0o750); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to create clone directory"})
		return
	}

	workspaceID := fmt.Sprintf("%s-%d", req.SourceID, time.Now().UnixNano())
	cloneDir := filepath.Join(cloneBase, workspaceID)

	if err := os.RemoveAll(cloneDir); err != nil && !os.IsNotExist(err) {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to clean existing workspace"})
		return
	}

	if err := os.MkdirAll(cloneDir, 0o750); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to create workspace"})
		return
	}

	cloneCtx, cancel := context.WithTimeout(r.Context(), maxCloneTime)
	defer cancel()

	var commitSHA string

	if req.CommitSHA != "" {
		// Exact commit checkout flow
		initCmd := exec.CommandContext(cloneCtx, "git", "-C", cloneDir, "init")
		initCmd.Env = gitEnv()
		if out, err := initCmd.CombinedOutput(); err != nil {
			log.Printf("[beacon] git init failed: %v (output: %s)", err, string(out))
			_ = os.RemoveAll(cloneDir)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("git init failed: %v", err)})
			return
		}

		remoteCmd := exec.CommandContext(cloneCtx, "git", "-C", cloneDir, "remote", "add", "origin", req.RepoURL)
		remoteCmd.Env = gitEnv()
		if out, err := remoteCmd.CombinedOutput(); err != nil {
			log.Printf("[beacon] git remote add failed: %v (output: %s)", err, string(out))
			_ = os.RemoveAll(cloneDir)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("remote add failed: %v", err)})
			return
		}

		fetchArgs := []string{"-C", cloneDir, "fetch", "--depth", "1", "origin", req.CommitSHA, "--no-tags", "--filter=blob:none"}
		fetchCmd := exec.CommandContext(cloneCtx, "git", fetchArgs...)
		fetchCmd.Env = gitEnv()
		if out, err := fetchCmd.CombinedOutput(); err != nil {
			log.Printf("[beacon] git fetch sha failed: %v (output: %s)", err, string(out))
			_ = os.RemoveAll(cloneDir)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("fetch commit failed: %v", err)})
			return
		}

		checkoutCmd := exec.CommandContext(cloneCtx, "git", "-C", cloneDir, "checkout", "--detach", "FETCH_HEAD")
		checkoutCmd.Env = gitEnv()
		if out, err := checkoutCmd.CombinedOutput(); err != nil {
			log.Printf("[beacon] git checkout sha failed: %v (output: %s)", err, string(out))
			_ = os.RemoveAll(cloneDir)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("checkout failed: %v", err)})
			return
		}

		commitSHA = req.CommitSHA

		// Verify checked-out HEAD matches requested SHA
		revCmd := exec.CommandContext(cloneCtx, "git", "-C", cloneDir, "rev-parse", "HEAD")
		revCmd.Env = gitEnv()
		revOut, err := revCmd.Output()
		if err != nil {
			_ = os.RemoveAll(cloneDir)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to verify commit SHA"})
			return
		}
		actualSHA := strings.TrimSpace(string(revOut))
		if actualSHA != req.CommitSHA {
			_ = os.RemoveAll(cloneDir)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("checked out %s does not match requested %s", actualSHA, req.CommitSHA)})
			return
		}
	} else {
		// Branch-head checkout (legacy, use exact SHA for reproducibility)
		cloneCmd := exec.CommandContext(cloneCtx, "git", "clone",
			"--depth", "1",
			"--single-branch",
			"--branch", req.Branch,
			"--no-tags",
			"--config", "core.symlinks=false",
			"-c", "filter.lfs.required=false",
			"-c", "protocol.file.allow=never",
			"-c", "protocol.ext.allow=never",
			"-c", "core.gitProxy=none",
			req.RepoURL, cloneDir,
		)
		cloneCmd.Env = gitEnv()
		out, err := cloneCmd.CombinedOutput()
		if err != nil {
			log.Printf("[beacon] git clone failed for %s: %v (output: %s)", req.RepoURL, err, string(out))
			_ = os.RemoveAll(cloneDir)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("clone failed: %v", err)})
			return
		}

		revCmd := exec.CommandContext(cloneCtx, "git", "-C", cloneDir, "rev-parse", "HEAD")
		revCmd.Env = gitEnv()
		revOut, err := revCmd.Output()
		if err != nil {
			_ = os.RemoveAll(cloneDir)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to resolve commit SHA"})
			return
		}
		commitSHA = strings.TrimSpace(string(revOut))
	}

	// Size check
	var totalSize int64
	_ = filepath.Walk(cloneDir, func(walkPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlinks not allowed in clone")
			}
			totalSize += info.Size()
		}
		return nil
	})

	if totalSize > maxCloneSize {
		_ = os.RemoveAll(cloneDir)
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "clone exceeds maximum size"})
		return
	}

	writeJSON(w, http.StatusOK, gitCloneResponse{
		WorkspaceID: workspaceID,
		CommitSHA:   commitSHA,
	})
}

func gitEnv() []string {
	return append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_LFS_SKIP_SMUDGE=1",
		"HOME="+os.TempDir(),
	)
}

func isHex40(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func extractHost(rawURL string) string {
	s := rawURL
	if idx := strings.Index(s, "://"); idx >= 0 {
		s = s[idx+3:]
	}
	if idx := strings.Index(s, "/"); idx >= 0 {
		s = s[:idx]
	}
	if idx := strings.Index(s, "@"); idx >= 0 {
		s = s[idx+1:]
	}
	if idx := strings.LastIndex(s, ":"); idx >= 0 {
		s = s[:idx]
	}
	return s
}

func isRestrictedHost(host string) bool {
	if host == "localhost" || host == "metadata.google.internal" {
		return true
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return true
	}
	for _, ip := range ips {
		if isRestrictedIP(ip) {
			return true
		}
	}
	return false
}

func isRestrictedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	for _, n := range restrictedNetworks {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func safePath(path, allowedRoot string) (string, error) {
	clean := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil {
		if os.IsNotExist(err) {
			resolved = clean
		} else {
			return "", fmt.Errorf("resolve path: %w", err)
		}
	}
	rel, err := filepath.Rel(allowedRoot, resolved)
	if err != nil {
		return "", fmt.Errorf("path not under allowed root")
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes allowed root")
	}
	return resolved, nil
}

func (s *Server) handleGitBuild(w http.ResponseWriter, r *http.Request) {
	var req gitBuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	if req.WorkspaceID == "" || req.ImageTag == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "workspaceId and imageTag are required"})
		return
	}

	cloneBase := filepath.Join(s.dataDir, gitCloneDir)
	sourceDir := filepath.Join(cloneBase, req.WorkspaceID)
	safeDir, err := safePath(sourceDir, cloneBase)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "workspace path validation failed"})
		return
	}

	if _, err := os.Stat(safeDir); os.IsNotExist(err) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "workspace directory not found"})
		return
	}

	dockerfilePath := req.DockerfilePath
	if dockerfilePath == "" {
		dockerfilePath = filepath.Join(safeDir, "Dockerfile")
	} else {
		dockerfilePath = filepath.Join(safeDir, filepath.Clean(dockerfilePath))
		if _, err := safePath(dockerfilePath, safeDir); err != nil {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "dockerfile path escapes workspace"})
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), maxBuildTime)
	defer cancel()

	args := []string{"build", "-t", req.ImageTag, "-f", dockerfilePath}
	for k, v := range req.BuildArgs {
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", k, v))
	}
	args = append(args, safeDir)

	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[beacon] docker build failed for %s: %v (output: %s)", req.ImageTag, err, string(out))
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("docker build failed: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, gitBuildResponse{
		ImageTag: req.ImageTag,
	})
}

func (s *Server) handleGitCleanup(w http.ResponseWriter, r *http.Request) {
	var req gitCleanupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	if req.WorkspaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "workspaceId is required"})
		return
	}

	cloneBase := filepath.Join(s.dataDir, gitCloneDir)
	workspaceDir := filepath.Join(cloneBase, req.WorkspaceID)
	safeDir, err := safePath(workspaceDir, cloneBase)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "workspace path validation failed"})
		return
	}

	if err := os.RemoveAll(safeDir); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("cleanup failed: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
