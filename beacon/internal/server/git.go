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
	"regexp"
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
	RepoURL     string `json:"repoUrl"`
	Branch      string `json:"branch"`
	SourceID    string `json:"sourceId"`
	CommitSHA   string `json:"commitSha,omitempty"`
	Username    string `json:"username,omitempty"`
	AccessToken string `json:"accessToken,omitempty"`
}

// gitRefNamePattern is a strict allowlist for git ref-like inputs (branch names,
// tags, etc.) supplied by callers. It only allows the characters that make up a
// valid, unambiguous git ref component and is intentionally conservative: it does
// not attempt to implement git's full ref-name grammar, it just excludes anything
// that could be misinterpreted as a flag or shell/argument-injection vector.
var gitRefNamePattern = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

// validateGitRefName defends against argument/option injection when a user-supplied
// ref name (branch, tag, etc.) is later passed to git. It is used as the primary
// defense; callers should additionally avoid passing the value as a separate exec
// arg after a bare flag (prefer "--branch=<value>" or a "--" separator) as a
// defense-in-depth measure.
func validateGitRefName(name string) error {
	if name == "" {
		return fmt.Errorf("ref name must not be empty")
	}
	if len(name) > 256 {
		return fmt.Errorf("ref name too long")
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("ref name must not start with '-'")
	}
	if !gitRefNamePattern.MatchString(name) {
		return fmt.Errorf("ref name contains disallowed characters")
	}
	if strings.Contains(name, "..") || strings.Contains(name, "/.") {
		return fmt.Errorf("ref name contains disallowed sequence")
	}
	return nil
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

	if err := validateGitRefName(req.Branch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("invalid branch name: %v", err)})
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

	// Build a request-scoped environment, including a hardened, request-scoped
	// GIT_ASKPASS helper if HTTPS credentials were supplied. cleanupCreds MUST
	// run on every path out of this handler, including error returns, so it is
	// deferred immediately.
	credEnv, cleanupCreds, err := gitEnvironmentForRequest(req, cloneBase)
	if err != nil {
		_ = os.RemoveAll(cloneDir)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to prepare git credentials"})
		return
	}
	defer cleanupCreds()

	var commitSHA string

	if req.CommitSHA != "" {
		// Exact commit checkout flow
		initCmd := exec.CommandContext(cloneCtx, "git", "-C", cloneDir, "init")
		initCmd.Env = credEnv
		if out, err := initCmd.CombinedOutput(); err != nil {
			log.Printf("[beacon] git init failed: %v (output: %s)", err, string(out))
			_ = os.RemoveAll(cloneDir)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("git init failed: %v", err)})
			return
		}

		remoteCmd := exec.CommandContext(cloneCtx, "git", "-C", cloneDir, "remote", "add", "origin", req.RepoURL)
		remoteCmd.Env = credEnv
		if out, err := remoteCmd.CombinedOutput(); err != nil {
			log.Printf("[beacon] git remote add failed: %v (output: %s)", err, string(out))
			_ = os.RemoveAll(cloneDir)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("remote add failed: %v", err)})
			return
		}

		fetchArgs := []string{"-C", cloneDir, "fetch", "--depth", "1", "origin", req.CommitSHA, "--no-tags", "--filter=blob:none"}
		fetchCmd := exec.CommandContext(cloneCtx, "git", fetchArgs...)
		fetchCmd.Env = credEnv
		if out, err := fetchCmd.CombinedOutput(); err != nil {
			log.Printf("[beacon] git fetch sha failed: %v (output: %s)", err, string(out))
			_ = os.RemoveAll(cloneDir)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("fetch commit failed: %v", err)})
			return
		}

		checkoutCmd := exec.CommandContext(cloneCtx, "git", "-C", cloneDir, "checkout", "--detach", "FETCH_HEAD")
		checkoutCmd.Env = credEnv
		if out, err := checkoutCmd.CombinedOutput(); err != nil {
			log.Printf("[beacon] git checkout sha failed: %v (output: %s)", err, string(out))
			_ = os.RemoveAll(cloneDir)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("checkout failed: %v", err)})
			return
		}

		commitSHA = req.CommitSHA

		// Verify checked-out HEAD matches requested SHA
		revCmd := exec.CommandContext(cloneCtx, "git", "-C", cloneDir, "rev-parse", "HEAD")
		revCmd.Env = credEnv
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
		// Branch-head checkout (legacy, use exact SHA for reproducibility).
		// The branch name is validated against a strict allowlist above (primary
		// defense). As defense-in-depth, it is also passed as a single
		// "--branch=<value>" token (rather than "--branch", "<value>" as two
		// separate argv entries) so it cannot be split apart to smuggle in an
		// unrelated flag, and a "--" separator is used before the positional
		// repo/directory arguments so git never treats them as options.
		cloneCmd := exec.CommandContext(cloneCtx, "git", "clone",
			"--depth", "1",
			"--single-branch",
			"--branch="+req.Branch,
			"--no-tags",
			"--config", "core.symlinks=false",
			"-c", "filter.lfs.required=false",
			"-c", "protocol.file.allow=never",
			"-c", "protocol.ext.allow=never",
			"-c", "core.gitProxy=none",
			"--",
			req.RepoURL, cloneDir,
		)
		cloneCmd.Env = credEnv
		out, err := cloneCmd.CombinedOutput()
		if err != nil {
			log.Printf("[beacon] git clone failed for %s: %v (output: %s)", req.RepoURL, err, string(out))
			_ = os.RemoveAll(cloneDir)
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("clone failed: %v", err)})
			return
		}

		revCmd := exec.CommandContext(cloneCtx, "git", "-C", cloneDir, "rev-parse", "HEAD")
		revCmd.Env = credEnv
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

// gitAskPassDir is the name of the dedicated, per-request subdirectory created
// under the server's data directory (or the OS temp dir as a fallback) to hold
// the transient git askpass helper script. Using a dedicated 0700 directory,
// rather than dropping the script directly into the shared OS temp dir, means
// the script is never briefly readable by other local users/processes via a
// world-readable or group-readable temp directory.
const gitAskPassDir = "forge-git-askpass"

// gitEnvironmentForRequest builds the environment to use for git subprocesses
// for a single clone request, optionally configuring GIT_ASKPASS so that HTTPS
// credentials (username/access token) can be supplied without ever embedding
// them in the repository URL (which would otherwise leak them into process
// listings, shell history, and git's on-disk remote configuration).
//
// The returned cleanup function removes the askpass helper (and its dedicated
// directory) and must be called by the caller once the git subprocess(es) for
// this request have finished, including on every error path - callers should
// invoke it via "defer" immediately after this function returns successfully.
//
// Residual risk: git's askpass mechanism only supports pointing at an
// executable script/program on disk (there is no way to hand git credentials
// via an in-memory pipe), so there is an inherent, unavoidable window between
// when the helper is written and when it is removed during which the
// credential-bearing environment exists on disk for this process only. We
// minimize that window by using a dedicated 0700 directory, 0600 file
// permissions, and deferred removal that runs even on error paths, but the
// window itself cannot be fully eliminated while using GIT_ASKPASS.
func gitEnvironmentForRequest(req gitCloneRequest, dataDir string) ([]string, func(), error) {
	env := gitEnv()
	noop := func() {}

	if req.Username == "" && req.AccessToken == "" {
		return env, noop, nil
	}

	parentDir := dataDir
	if parentDir == "" {
		parentDir = os.TempDir()
	}
	if err := os.MkdirAll(parentDir, 0o700); err != nil {
		return nil, noop, fmt.Errorf("prepare askpass parent directory: %w", err)
	}

	// Dedicated, per-request 0700 directory: never share the OS-wide temp dir,
	// which may be world-readable/traversable on some platforms.
	askPassDir, err := os.MkdirTemp(parentDir, gitAskPassDir+"-*")
	if err != nil {
		return nil, noop, fmt.Errorf("create askpass directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(askPassDir) }

	scriptFile, err := os.CreateTemp(askPassDir, "forge-git-askpass-*.sh")
	if err != nil {
		cleanup()
		return nil, noop, fmt.Errorf("create askpass script: %w", err)
	}
	scriptPath := scriptFile.Name()
	// Harden permissions immediately, before any content (including the
	// credential-bearing environment variable names) is written, and ensure
	// removal happens on every subsequent error path via defer.
	if err := scriptFile.Chmod(0o700); err != nil {
		_ = scriptFile.Close()
		cleanup()
		return nil, noop, fmt.Errorf("chmod askpass script: %w", err)
	}

	// The script itself never contains the credential values - it only reads
	// them from environment variables that are scoped to this one git
	// subprocess invocation. This avoids having to shell-escape untrusted
	// username/token values into a script body.
	const script = "#!/bin/sh\ncase \"$1\" in\n\t*[Uu]sername*) printf '%s' \"$FORGE_GIT_ASKPASS_USERNAME\" ;;\n\t*) printf '%s' \"$FORGE_GIT_ASKPASS_PASSWORD\" ;;\nesac\n"
	if _, err := scriptFile.WriteString(script); err != nil {
		_ = scriptFile.Close()
		cleanup()
		return nil, noop, fmt.Errorf("write askpass script: %w", err)
	}
	if err := scriptFile.Close(); err != nil {
		cleanup()
		return nil, noop, fmt.Errorf("close askpass script: %w", err)
	}

	env = append(env,
		"GIT_ASKPASS="+scriptPath,
		"FORGE_GIT_ASKPASS_USERNAME="+req.Username,
		"FORGE_GIT_ASKPASS_PASSWORD="+req.AccessToken,
	)

	return env, cleanup, nil
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
