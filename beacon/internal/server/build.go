package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"gamepanel/beacon/internal/serverid"
	"github.com/google/uuid"
)

type buildJob struct {
	id          string
	workspaceID string
	cmd         *exec.Cmd
	logBuf      bytes.Buffer
	logCh       chan string
	cancel      context.CancelFunc
	status      string
	exitCode    int
	imageRef    string
	startedAt   time.Time
	truncated   bool
	mu          sync.RWMutex
}

type buildManager struct {
	mu     sync.RWMutex
	active map[string]*buildJob
}

const maxLogBufferSize = 100 * 1024 * 1024

type dockerfileBuildRequest struct {
	WorkspaceID        string   `json:"workspaceId"`
	SourceDir          string   `json:"sourceDir"` // deprecated: use workspaceId
	Dockerfile         string   `json:"dockerfile"`
	ImageName          string   `json:"imageName"`
	BuildArgs          []string `json:"buildArgs"`
	Labels             []string `json:"labels"`
	Tags               []string `json:"tags"`
	NoCache            bool     `json:"noCache"`
	SecretArgs         []string `json:"secretArgs,omitempty"`
	MaxCPU             int      `json:"maxCpu,omitempty"`
	MaxMemoryMB        int      `json:"maxMemoryMb,omitempty"`
	MaxLogBytes        int      `json:"maxLogBytes,omitempty"`
	CredentialPatterns []string `json:"credentialPatterns,omitempty"`
	TenantID           string   `json:"tenantId,omitempty"`
}

type nixpacksBuildRequest struct {
	WorkspaceID        string   `json:"workspaceId"`
	SourceDir          string   `json:"sourceDir"` // deprecated: use workspaceId
	ImageName          string   `json:"imageName"`
	BuildArgs          []string `json:"buildArgs"`
	Tags               []string `json:"tags"`
	NoCache            bool     `json:"noCache"`
	SecretArgs         []string `json:"secretArgs,omitempty"`
	MaxCPU             int      `json:"maxCpu,omitempty"`
	MaxMemoryMB        int      `json:"maxMemoryMb,omitempty"`
	MaxLogBytes        int      `json:"maxLogBytes,omitempty"`
	CredentialPatterns []string `json:"credentialPatterns,omitempty"`
	TenantID           string   `json:"tenantId,omitempty"`
}

func (s *Server) handleDockerfileBuild(w http.ResponseWriter, r *http.Request) {
	if _, err := exec.LookPath("docker"); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "docker build CLI is unavailable"})
		return
	}
	var req dockerfileBuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	sourceDir, err := s.resolveWorkspace(req.WorkspaceID, req.SourceDir)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid workspace: " + err.Error()})
		return
	}

	if _, err := os.Stat(sourceDir); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("source directory not found: %v", err)})
		return
	}

	tid := req.TenantID
	if tid == "" {
		tid = "_system"
	}

	imageName := req.ImageName
	if imageName == "" {
		imageName = fmt.Sprintf("forge/%s/build-%d", tid, time.Now().UnixNano())
	}

	dockerfile := req.Dockerfile
	if dockerfile == "" {
		dockerfile = filepath.Join(sourceDir, "Dockerfile")
	}

	args := []string{"buildx", "build", "-f", dockerfile}

	if req.NoCache {
		args = append(args, "--no-cache")
	}

	for _, arg := range req.BuildArgs {
		args = append(args, "--build-arg", arg)
	}

	for _, secret := range req.SecretArgs {
		args = append(args, "--secret", secret)
	}

	for _, label := range req.Labels {
		args = append(args, "--label", label)
	}

	for _, tag := range req.Tags {
		args = append(args, "-t", tag)
	}
	args = append(args, "-t", imageName)

	if req.MaxCPU > 0 {
		args = append(args, "--cpu-shares", fmt.Sprintf("%d", req.MaxCPU*1024/100))
	}
	if req.MaxMemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", req.MaxMemoryMB))
		args = append(args, "--memory-swap", fmt.Sprintf("%dm", req.MaxMemoryMB))
	}
	args = append(args, "--shm-size", "256m")
	args = append(args, sourceDir)

	job := s.builds.startBuild(r.Context(), imageName, req.WorkspaceID, "docker", req.CredentialPatterns, args[1:]...)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":          job.id,
		"imageName":   imageName,
		"workspaceId": req.WorkspaceID,
		"status":      job.status,
	})
}

func (s *Server) resolveWorkspace(workspaceID, legacySourceDir string) (string, error) {
	if workspaceID != "" {
		cloneBase := filepath.Join(s.dataDir, gitCloneDir)
		sourceDir := filepath.Join(cloneBase, workspaceID)
		return safePath(sourceDir, cloneBase)
	}
	const serverPrefix = "server:"
	if strings.HasPrefix(legacySourceDir, serverPrefix) {
		serverID := strings.TrimPrefix(legacySourceDir, serverPrefix)
		if err := serverid.Validate(serverID); err != nil {
			return "", fmt.Errorf("invalid server workspace: %w", err)
		}
		return safePath(filepath.Join(s.dataDir, serverID), s.dataDir)
	}
	return "", fmt.Errorf("workspace must be a managed clone or server root")
}

func (s *Server) handleNixpacksBuild(w http.ResponseWriter, r *http.Request) {
	if _, err := exec.LookPath("nixpacks"); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "nixpacks build CLI is unavailable"})
		return
	}
	var req nixpacksBuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	sourceDir, err := s.resolveWorkspace(req.WorkspaceID, req.SourceDir)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid workspace: " + err.Error()})
		return
	}

	tid := req.TenantID
	if tid == "" {
		tid = "_system"
	}

	imageName := req.ImageName
	if imageName == "" {
		imageName = fmt.Sprintf("forge/%s/build-%d", tid, time.Now().UnixNano())
	}

	args := []string{"nixpacks", "build", sourceDir, "--name", imageName}
	if req.NoCache {
		args = append(args, "--no-cache")
	}
	for _, arg := range req.BuildArgs {
		args = append(args, "--build-env", arg)
	}
	for _, tag := range req.Tags {
		args = append(args, "-t", tag)
	}
	args = append(args, "-t", imageName)

	job := s.builds.startBuild(r.Context(), imageName, req.WorkspaceID, "nixpacks", req.CredentialPatterns, args[1:]...)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":          job.id,
		"imageName":   imageName,
		"workspaceId": req.WorkspaceID,
		"status":      job.status,
	})
}

func (s *Server) handleBuildLogs(w http.ResponseWriter, r *http.Request) {
	buildID := r.URL.Query().Get("id")
	if buildID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id query parameter required"})
		return
	}

	s.builds.mu.RLock()
	job, ok := s.builds.active[buildID]
	s.builds.mu.RUnlock()

	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "build not found"})
		return
	}

	follow := r.URL.Query().Get("follow") == "true"

	if job.isTerminal() || !follow {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(job.logBuf.Bytes())
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	offset := job.logBuf.Len()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ticker.C:
			job.mu.RLock()
			currentLen := job.logBuf.Len()
			if currentLen > offset {
				chunk := job.logBuf.Bytes()[offset:currentLen]
				offset = currentLen
				job.mu.RUnlock()
				for _, line := range strings.Split(string(chunk), "\n") {
					if line != "" {
						_, _ = fmt.Fprintf(w, "data: %s\n\n", line)
						flusher.Flush()
					}
				}
			} else {
				job.mu.RUnlock()
			}

			if job.isTerminal() {
				_, _ = fmt.Fprintf(w, "event: done\ndata: %s\n\n", job.status)
				flusher.Flush()
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (s *Server) handleBuildCancel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if body.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}

	s.builds.mu.RLock()
	job, ok := s.builds.active[body.ID]
	s.builds.mu.RUnlock()

	if !ok || job.isTerminal() {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "active build not found"})
		return
	}

	job.cancel()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (m *buildManager) startBuild(ctx context.Context, imageRef, workspaceID string, command string, credPatterns []string, args ...string) *buildJob {
	buildCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(buildCtx, command, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	buildHome, homeErr := os.MkdirTemp("", "beacon-build-home-*")
	if homeErr == nil {
		_ = os.Chmod(buildHome, 0o700)
	}
	cmd.Env = buildEnvironment(buildHome)

	job := &buildJob{
		id:          "build-" + uuid.NewString(),
		workspaceID: workspaceID,
		cmd:         cmd,
		cancel:      cancel,
		status:      "running",
		imageRef:    imageRef,
		startedAt:   time.Now(),
		logCh:       make(chan string, 256),
	}

	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout

	m.mu.Lock()
	m.active[job.id] = job
	m.mu.Unlock()

	if err := cmd.Start(); err != nil {
		if buildHome != "" {
			_ = os.RemoveAll(buildHome)
		}
		job.status = "failed"
		job.exitCode = -1
		_, _ = fmt.Fprintf(&job.logBuf, "ERROR: %v\n", err)
		return job
	}

	pid := cmd.Process.Pid
	processDone := make(chan struct{})
	go func() {
		select {
		case <-buildCtx.Done():
			if cmd.Process != nil {
				_ = syscall.Kill(-pid, syscall.SIGINT)
				timer := time.NewTimer(2 * time.Second)
				select {
				case <-timer.C:
					_ = syscall.Kill(-pid, syscall.SIGKILL)
				case <-processDone:
					timer.Stop()
				}
			}
		case <-processDone:
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			for _, pattern := range credPatterns {
				if pattern != "" {
					line = strings.ReplaceAll(line, pattern, "****")
				}
			}
			job.mu.Lock()
			if job.logBuf.Len()+len(line) < maxLogBufferSize {
				_, _ = fmt.Fprintln(&job.logBuf, line)
			} else if !job.truncated {
				_, _ = fmt.Fprintln(&job.logBuf, "[LOG TRUNCATED: exceeded 100MB limit]")
				job.truncated = true
			}
			job.mu.Unlock()
		}
	}()

	go func() {
		err := cmd.Wait()
		close(processDone)
		if buildHome != "" {
			_ = os.RemoveAll(buildHome)
		}
		job.mu.Lock()
		defer job.mu.Unlock()

		if err != nil {
			if buildCtx.Err() != nil {
				job.status = "canceled"
				job.exitCode = -1
				_, _ = fmt.Fprintln(&job.logBuf, "Build canceled")
			} else if exitErr, ok := err.(*exec.ExitError); ok {
				job.status = "failed"
				job.exitCode = exitErr.ExitCode()
				_, _ = fmt.Fprintf(&job.logBuf, "Build failed with exit code %d\n", job.exitCode)
			} else {
				job.status = "failed"
				job.exitCode = -1
				_, _ = fmt.Fprintf(&job.logBuf, "Build error: %v\n", err)
			}
		} else {
			job.status = "succeeded"
			job.exitCode = 0
			_, _ = fmt.Fprintln(&job.logBuf, "Build succeeded")
		}
	}()

	return job
}

func buildEnvironment(home string) []string {
	env := []string{"DOCKER_BUILDKIT=1"}
	for _, key := range []string{"PATH", "DOCKER_HOST", "DOCKER_CONFIG", "XDG_RUNTIME_DIR", "TMPDIR", "SSL_CERT_FILE", "SSL_CERT_DIR"} {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	if home != "" {
		env = append(env, "HOME="+home)
	}
	return env
}

func (j *buildJob) isTerminal() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.status == "succeeded" || j.status == "failed" || j.status == "canceled"
}

func (m *buildManager) reapAbandoned() {
	m.mu.Lock()
	defer m.mu.Unlock()
	cutoff := time.Now()
	for id, job := range m.active {
		job.mu.RLock()
		status := job.status
		startedAt := job.startedAt
		job.mu.RUnlock()
		if status == "running" {
			if cutoff.Sub(startedAt) > 12*time.Hour {
				job.mu.Lock()
				job.status = "abandoned"
				job.exitCode = -1
				job.mu.Unlock()
				job.cancel()
				delete(m.active, id)
			}
			continue
		}
		// Terminal jobs (succeeded/failed/canceled/abandoned) are retained for
		// a bounded window so clients can poll their status, then evicted so
		// the daemon does not accumulate log buffers (up to 100MB each)
		// indefinitely.
		if cutoff.Sub(startedAt) > maxCompletedRetention {
			delete(m.active, id)
		}
	}
}

const maxCompletedRetention = time.Hour

func (s *Server) startBuildReaper(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()

		cleanupOrphanedClones(s.dataDir)

		for {
			select {
			case <-ticker.C:
				s.builds.reapAbandoned()
				cleanupStaleClones(s.dataDir)
			case <-ctx.Done():
				return
			}
		}
	}()
}

func cleanupOrphanedClones(dataDir string) {
	cloneBase := filepath.Join(dataDir, gitCloneDir)
	entries, err := os.ReadDir(cloneBase)
	if err != nil {
		return
	}
	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if now.Sub(info.ModTime()) > 24*time.Hour {
				_ = os.RemoveAll(filepath.Join(cloneBase, entry.Name()))
			}
		}
	}
}

func cleanupStaleClones(dataDir string) {
	cleanupOrphanedClones(dataDir)
}
