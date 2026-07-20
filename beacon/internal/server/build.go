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
)

type buildJob struct {
	id        string
	cmd       *exec.Cmd
	logBuf    bytes.Buffer
	logCh     chan string
	cancel    context.CancelFunc
	status    string
	exitCode  int
	imageRef  string
	startedAt time.Time
	mu        sync.RWMutex
}

type buildManager struct {
	mu     sync.RWMutex
	active map[string]*buildJob
}

var builds = &buildManager{active: make(map[string]*buildJob)}

type dockerfileBuildRequest struct {
	SourceDir  string   `json:"sourceDir"`
	Dockerfile string   `json:"dockerfile"`
	ImageName  string   `json:"imageName"`
	BuildArgs  []string `json:"buildArgs"`
	Labels     []string `json:"labels"`
	Tags       []string `json:"tags"`
	NoCache    bool     `json:"noCache"`
}

type nixpacksBuildRequest struct {
	SourceDir string   `json:"sourceDir"`
	ImageName string   `json:"imageName"`
	BuildArgs []string `json:"buildArgs"`
	Tags      []string `json:"tags"`
	NoCache   bool     `json:"noCache"`
}

func (s *Server) handleDockerfileBuild(w http.ResponseWriter, r *http.Request) {
	var req dockerfileBuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.SourceDir == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sourceDir is required"})
		return
	}
	if _, err := os.Stat(req.SourceDir); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("source directory not found: %v", err)})
		return
	}

	imageName := req.ImageName
	if imageName == "" {
		imageName = fmt.Sprintf("beacon-build-%d", time.Now().UnixNano())
	}

	dockerfile := req.Dockerfile
	if dockerfile == "" {
		dockerfile = filepath.Join(req.SourceDir, "Dockerfile")
	}

	args := []string{"docker", "build", "-f", dockerfile}
	if req.NoCache {
		args = append(args, "--no-cache")
	}
	for _, arg := range req.BuildArgs {
		args = append(args, "--build-arg", arg)
	}
	for _, label := range req.Labels {
		args = append(args, "--label", label)
	}
	for _, tag := range req.Tags {
		args = append(args, "-t", tag)
	}
	args = append(args, "-t", imageName)
	args = append(args, req.SourceDir)

	job := builds.startBuild(r.Context(), imageName, "docker", args[1:]...)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":        job.id,
		"imageName": imageName,
		"status":    job.status,
	})
}

func (s *Server) handleNixpacksBuild(w http.ResponseWriter, r *http.Request) {
	var req nixpacksBuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.SourceDir == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sourceDir is required"})
		return
	}

	imageName := req.ImageName
	if imageName == "" {
		imageName = fmt.Sprintf("beacon-build-%d", time.Now().UnixNano())
	}

	args := []string{"nixpacks", "build", req.SourceDir, "--name", imageName}
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

	job := builds.startBuild(r.Context(), imageName, "nixpacks", args[1:]...)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":        job.id,
		"imageName": imageName,
		"status":    job.status,
	})
}

func (s *Server) handleBuildLogs(w http.ResponseWriter, r *http.Request) {
	buildID := r.URL.Query().Get("id")
	if buildID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id query parameter required"})
		return
	}

	builds.mu.RLock()
	job, ok := builds.active[buildID]
	builds.mu.RUnlock()

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

	builds.mu.RLock()
	job, ok := builds.active[body.ID]
	builds.mu.RUnlock()

	if !ok || job.isTerminal() {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "active build not found"})
		return
	}

	job.cancel()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (m *buildManager) startBuild(ctx context.Context, imageRef string, command string, args ...string) *buildJob {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	buildCtx, cancel := context.WithCancel(context.Background())
	job := &buildJob{
		id:        fmt.Sprintf("build-%d", time.Now().UnixNano()),
		cmd:       cmd,
		cancel:    cancel,
		status:    "running",
		imageRef:  imageRef,
		startedAt: time.Now(),
		logCh:     make(chan string, 256),
	}

	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout

	m.mu.Lock()
	m.active[job.id] = job
	m.mu.Unlock()

	if err := cmd.Start(); err != nil {
		job.status = "failed"
		job.exitCode = -1
		_, _ = fmt.Fprintf(&job.logBuf, "ERROR: %v\n", err)
		return job
	}

	pid := cmd.Process.Pid
	go func() {
		<-buildCtx.Done()
		if cmd.Process != nil {
			_ = syscall.Kill(-pid, syscall.SIGINT)
			time.Sleep(2 * time.Second)
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			job.mu.Lock()
			_, _ = fmt.Fprintln(&job.logBuf, line)
			job.mu.Unlock()
		}
	}()

	go func() {
		err := cmd.Wait()
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

func (j *buildJob) isTerminal() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.status == "succeeded" || j.status == "failed" || j.status == "canceled"
}

func (m *buildManager) reapAbandoned() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, job := range m.active {
		if job.status == "running" && time.Since(job.startedAt) > 12*time.Hour {
			job.mu.Lock()
			job.status = "abandoned"
			job.exitCode = -1
			job.mu.Unlock()
			job.cancel()
			delete(m.active, id)
		}
	}
}
