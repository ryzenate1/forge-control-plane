package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	gitCloneDir    = ".git-clones"
	maxCloneSize   = 1024 * 1024 * 1024
	maxCloneTime   = 10 * time.Minute
	maxBuildTime   = 30 * time.Minute
	maxPushTime    = 15 * time.Minute
)

type gitCloneRequest struct {
	RepoURL    string `json:"repoUrl"`
	Branch     string `json:"branch"`
	SourceID   string `json:"sourceId"`
}

type gitCloneResponse struct {
	CloneDir string `json:"cloneDir"`
	CommitSHA string `json:"commitSha"`
}

type gitBuildRequest struct {
	CloneDir       string            `json:"cloneDir"`
	ImageTag       string            `json:"imageTag"`
	DockerfilePath string            `json:"dockerfilePath"`
	BuildArgs      map[string]string `json:"buildArgs"`
}

type gitBuildResponse struct {
	ImageTag string `json:"imageTag"`
}

type gitCleanupRequest struct {
	CloneDir string `json:"cloneDir"`
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

	if strings.Contains(req.RepoURL, "..") || strings.Contains(req.RepoURL, ";") || strings.Contains(req.RepoURL, "`") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid repo URL"})
		return
	}

	if strings.Contains(req.Branch, "..") || strings.Contains(req.Branch, "/.") || strings.Contains(req.Branch, " ") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid branch name"})
		return
	}

	if strings.Contains(req.SourceID, "..") || strings.Contains(req.SourceID, "/") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid source id"})
		return
	}

	cloneBase := filepath.Join(s.dataDir, gitCloneDir)
	if err := os.MkdirAll(cloneBase, 0o750); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to create clone directory"})
		return
	}

	targetDir := filepath.Join(cloneBase, req.SourceID)
	safeDir := filepath.Clean(targetDir)
	if !strings.HasPrefix(safeDir, filepath.Clean(cloneBase)) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "clone path escapes clone directory"})
		return
	}

	if err := os.RemoveAll(safeDir); err != nil && !os.IsNotExist(err) {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to clean existing clone"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), maxCloneTime)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "clone",
		"--depth", "1",
		"--single-branch",
		"--branch", req.Branch,
		"--no-tags",
		"--config", "core.symlinks=false",
		req.RepoURL, safeDir,
	)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[beacon] git clone failed for %s: %v (output: %s)", req.RepoURL, err, string(out))
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("clone failed: %v", err)})
		return
	}

	revCmd := exec.CommandContext(ctx, "git", "-C", safeDir, "rev-parse", "HEAD")
	revOut, err := revCmd.Output()
	if err != nil {
		_ = os.RemoveAll(safeDir)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to resolve commit SHA"})
		return
	}

	commitSHA := strings.TrimSpace(string(revOut))

	var totalSize int64
	_ = filepath.Walk(safeDir, func(_ string, info os.FileInfo, err error) error {
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
		_ = os.RemoveAll(safeDir)
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{"error": "clone exceeds maximum size"})
		return
	}

	writeJSON(w, http.StatusOK, gitCloneResponse{
		CloneDir: safeDir,
		CommitSHA: commitSHA,
	})
}

func (s *Server) handleGitBuild(w http.ResponseWriter, r *http.Request) {
	var req gitBuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}

	if req.CloneDir == "" || req.ImageTag == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "cloneDir and imageTag are required"})
		return
	}

	cloneBase := filepath.Join(s.dataDir, gitCloneDir)
	safeDir := filepath.Clean(req.CloneDir)
	if !strings.HasPrefix(safeDir, filepath.Clean(cloneBase)) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "clone path escapes clone directory"})
		return
	}

	if _, err := os.Stat(safeDir); os.IsNotExist(err) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "clone directory not found"})
		return
	}

	dockerfilePath := req.DockerfilePath
	if dockerfilePath == "" {
		dockerfilePath = filepath.Join(safeDir, "Dockerfile")
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

	if req.CloneDir == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "cloneDir is required"})
		return
	}

	cloneBase := filepath.Join(s.dataDir, gitCloneDir)
	safeDir := filepath.Clean(req.CloneDir)
	if !strings.HasPrefix(safeDir, filepath.Clean(cloneBase)) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "clone path escapes clone directory"})
		return
	}

	if err := os.RemoveAll(safeDir); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": fmt.Sprintf("cleanup failed: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
