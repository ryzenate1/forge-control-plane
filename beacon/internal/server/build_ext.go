package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type imagePushRequest struct {
	ImageRef     string           `json:"imageRef"`
	RegistryAuth *registryAuthReq `json:"registryAuth,omitempty"`
}

type registryAuthReq struct {
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	ServerAddress string `json:"serverAddress,omitempty"`
}

type pushResult struct {
	Digest string `json:"digest"`
}

func (s *Server) handleImagePush(w http.ResponseWriter, r *http.Request) {
	var req imagePushRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.ImageRef == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "imageRef is required"})
		return
	}
	if strings.Contains(req.ImageRef, "..") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid imageRef"})
		return
	}
	for _, c := range req.ImageRef {
		if c < 32 || c == 127 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid imageRef"})
			return
		}
	}

	// The DOCKER_CONFIG directory may contain docker's config.json, which
	// can embed base64-encoded registry credentials on disk. Create it
	// with explicit 0700 permissions (owner-only) so it is not
	// world-discoverable via the shared /tmp namespace, and always remove
	// it once we're done with it.
	dockerConfigDir, err := os.MkdirTemp("", "docker-config-*")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create docker config"})
		return
	}
	defer os.RemoveAll(dockerConfigDir)
	if err := os.Chmod(dockerConfigDir, 0o700); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to secure docker config"})
		return
	}
	dockerEnv := append(os.Environ(), "DOCKER_CONFIG="+dockerConfigDir)

	if req.RegistryAuth != nil {
		args := []string{"login"}
		if req.RegistryAuth.Username != "" {
			args = append(args, "-u", req.RegistryAuth.Username, "--password-stdin")
		} else {
			args = append(args, "--password-stdin")
		}
		if req.RegistryAuth.ServerAddress != "" {
			args = append(args, req.RegistryAuth.ServerAddress)
		}

		// The password is passed exclusively via cmd.Stdin below
		// (--password-stdin), never via argv, so it never appears in
		// cmd.Args, /proc/<pid>/cmdline, or process listings.
		cmd := exec.Command("docker", args...)
		cmd.Env = dockerEnv
		cmd.SysProcAttr = getSysProcAttr()
		cmd.Stdin = strings.NewReader(req.RegistryAuth.Password)
		out, err := cmd.CombinedOutput()
		req.RegistryAuth.Password = ""
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("docker login failed: %v", err),
				"log":   string(out),
			})
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Minute)
	defer cancel()

	// docker push runs as a subprocess of this agent. getSysProcAttr() puts
	// it in its own process group so it can be signaled/reaped
	// independently, but this alone is NOT subprocess sandboxing: Go's
	// exec.Cmd cannot itself apply seccomp filters, drop Linux
	// capabilities, or apply resource limits (cgroups/rlimits) to the
	// child. For real isolation, run the whole beacon agent under a
	// hardened systemd unit (NoNewPrivileges=yes, ProtectSystem=strict,
	// ProtectHome=yes, CapabilityBoundingSet=, MemoryMax=/CPUQuota=,
	// PrivateTmp=yes) or an equivalent container/namespace policy, so
	// that docker (and everything it spawns) inherits those restrictions
	// as defense in depth.
	cmd := exec.CommandContext(ctx, "docker", "push", req.ImageRef)
	cmd.Env = dockerEnv
	cmd.SysProcAttr = getSysProcAttr()
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("push failed: %v", err),
			"log":   out.String(),
		})
		return
	}

	digest := ""
	inspectCmd := exec.Command("docker", "inspect", "--format", "{{index .RepoDigests 0}}", req.ImageRef)
	inspectCmd.Env = dockerEnv
	digestOut, inspectErr := inspectCmd.Output()
	if inspectErr == nil {
		fullRef := strings.TrimSpace(string(digestOut))
		if idx := strings.Index(fullRef, "@"); idx >= 0 {
			digest = fullRef[idx+1:]
		}
	}

	if digest == "" {
		for _, line := range strings.Split(out.String(), "\n") {
			if strings.Contains(line, "digest:") {
				parts := strings.Split(line, "digest:")
				if len(parts) == 2 {
					digest = strings.TrimSpace(parts[1])
					break
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, pushResult{Digest: digest})
}

type imageInspectRequest struct {
	Ref string `json:"ref"`
}

func (s *Server) handleImageInspect(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	if ref == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ref query parameter required"})
		return
	}

	digestCmd := exec.Command("docker", "inspect", "--format", "{{index .RepoDigests 0}}", ref)
	digestOut, digestErr := digestCmd.Output()
	digest := ""
	if digestErr == nil {
		if d := strings.TrimSpace(string(digestOut)); d != "" {
			if idx := strings.LastIndex(d, "@"); idx >= 0 {
				digest = d[idx+1:]
			} else {
				digest = d
			}
		}
	}

	idCmd := exec.Command("docker", "inspect", "--format", "{{.Id}}", ref)
	idOut, idErr := idCmd.Output()

	result := map[string]string{}
	if digest != "" {
		result["digest"] = digest
	}
	if idErr == nil {
		if id := strings.TrimSpace(string(idOut)); id != "" {
			result["id"] = id
		}
	}
	if len(result) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("image not found: %s", ref)})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleRegistryLogin(w http.ResponseWriter, r *http.Request) {
	var req registryAuthReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	dockerConfigDir, err := os.MkdirTemp("", "docker-config-*")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create docker config"})
		return
	}
	defer os.RemoveAll(dockerConfigDir)
	if err := os.Chmod(dockerConfigDir, 0o700); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to secure docker config"})
		return
	}

	args := []string{"login"}
	if req.Username != "" {
		args = append(args, "-u", req.Username, "--password-stdin")
	} else {
		args = append(args, "--password-stdin")
	}
	if req.ServerAddress != "" {
		args = append(args, req.ServerAddress)
	}

	// The password is passed exclusively via cmd.Stdin (--password-stdin),
	// never via argv, so it never appears in cmd.Args or process listings.
	cmd := exec.Command("docker", args...)
	cmd.Env = append(os.Environ(), "DOCKER_CONFIG="+dockerConfigDir)
	cmd.SysProcAttr = getSysProcAttr()
	cmd.Stdin = strings.NewReader(req.Password)
	out, err := cmd.CombinedOutput()
	req.Password = ""
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("docker login failed: %v", err),
			"log":   string(out),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type buildCleanupRequest struct {
	WorkspaceID string `json:"workspaceId"`
}

func (s *Server) handleBuildCleanup(w http.ResponseWriter, r *http.Request) {
	var req buildCleanupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	cloneBase := filepath.Join(s.dataDir, gitCloneDir)
	workspaceDir := filepath.Join(cloneBase, req.WorkspaceID)
	safeDir, err := safePath(workspaceDir, cloneBase)
	if err != nil {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "workspace path validation failed"})
		return
	}

	if err := removeAll(safeDir); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("cleanup failed: %v", err)})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleBuildStatus(w http.ResponseWriter, r *http.Request) {
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

	job.mu.RLock()
	writeJSON(w, http.StatusOK, map[string]any{
		"id":          job.id,
		"status":      job.status,
		"exitCode":    job.exitCode,
		"imageRef":    job.imageRef,
		"workspaceId": job.workspaceID,
	})
	job.mu.RUnlock()
}

func removeAll(path string) error {
	return os.RemoveAll(path)
}

func getSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
