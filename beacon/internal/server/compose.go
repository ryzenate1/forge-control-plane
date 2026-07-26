package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type composeLockEntry struct {
	mu     sync.Mutex
	active bool
}

type composeStack struct {
	mu     sync.RWMutex
	stacks map[string]*composeLockEntry
	dir    string
}

func newComposeStackManager(dataDir string) *composeStack {
	dir := filepath.Join(filepath.Dir(dataDir), "compose")
	_ = os.MkdirAll(dir, 0o750)
	return &composeStack{stacks: make(map[string]*composeLockEntry), dir: dir}
}

func (cs *composeStack) lock(stackID string) {
	cs.mu.Lock()
	entry, ok := cs.stacks[stackID]
	if !ok {
		entry = &composeLockEntry{}
		cs.stacks[stackID] = entry
	}
	cs.mu.Unlock()
	entry.mu.Lock()
	entry.active = true
}

func (cs *composeStack) unlock(stackID string) {
	cs.mu.RLock()
	entry := cs.stacks[stackID]
	cs.mu.RUnlock()
	if entry != nil {
		entry.active = false
		entry.mu.Unlock()
	}
}

func validStackID(stackID string) bool {
	if stackID == "" || len(stackID) > 128 || stackID[0] < 'a' || stackID[0] > 'z' {
		if stackID == "" || stackID[0] < '0' || stackID[0] > '9' {
			return false
		}
	}
	for _, r := range stackID {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func validateComposePolicy(yamlContent string) error {
	if strings.Contains(yamlContent, "privileged: true") || strings.Contains(yamlContent, "privileged: True") {
		return errors.New("privileged mode is not allowed in compose deployments")
	}
	if strings.Contains(yamlContent, "network_mode: \"host\"") || strings.Contains(yamlContent, "network_mode: host") {
		return errors.New("host network mode is not allowed")
	}
	if strings.Contains(yamlContent, "pid: \"host\"") || strings.Contains(yamlContent, "pid: host") {
		return errors.New("host pid namespace is not allowed")
	}
	if strings.Contains(yamlContent, "userns_mode: \"host\"") || strings.Contains(yamlContent, "userns_mode: host") {
		return errors.New("host userns mode is not allowed")
	}
	lines := strings.Split(yamlContent, "\n")
	inVolumes := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "volumes:") {
			inVolumes = true
			continue
		}
		if inVolumes && strings.HasPrefix(trimmed, "services:") {
			inVolumes = false
			continue
		}
		if inVolumes && strings.HasPrefix(trimmed, "- ") {
			volPath := strings.TrimPrefix(trimmed, "- ")
			if strings.HasPrefix(volPath, "/") {
				return errors.New("host bind mounts are not allowed in compose deployments")
			}
		}
	}
	return nil
}

func encodeComposeEnv(envVars map[string]string) ([]byte, error) {
	var buf bytes.Buffer
	for key, value := range envVars {
		if key == "" {
			return nil, errors.New("environment variable key must not be empty")
		}
		for _, c := range key {
			if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_') {
				return nil, fmt.Errorf("invalid environment variable key %q: only alphanumeric and underscore allowed", key)
			}
		}
		safeValue := strings.ReplaceAll(strings.ReplaceAll(value, "\n", ""), "\r", "")
		if _, err := fmt.Fprintf(&buf, "%s=%s\n", key, safeValue); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func (cs *composeStack) dirForID(stackID string) string {
	if !validStackID(stackID) {
		return filepath.Join(cs.dir, "_invalid_"+stackID)
	}
	return filepath.Join(cs.dir, stackID)
}

type composeDeployRequest struct {
	StackID     string            `json:"stackId"`
	ComposeYAML string            `json:"composeYaml"`
	EnvVars     map[string]string `json:"envVars,omitempty"`
	// RemoveOrphans is opt-in and defaults to false. When true, it passes
	// --remove-orphans to `docker compose up`, which removes any containers
	// Compose considers orphaned relative to the *current* compose file.
	// This can unexpectedly kill containers left over from a previous
	// version of the same project (or otherwise unrelated containers docker
	// compose associates with this project name) that the caller did not
	// intend to remove. Callers must explicitly request this behavior.
	RemoveOrphans bool `json:"removeOrphans,omitempty"`
}

type composeDeployResponse struct {
	StackID string `json:"stackId"`
	Output  string `json:"output,omitempty"`
}

type composeStatusResponse struct {
	StackID  string                `json:"stackId"`
	Services []composeServiceState `json:"services"`
}

type composeServiceState struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	Status string `json:"status"`
	State  string `json:"state"`
	Ports  string `json:"ports"`
}

type composeLogsRequest struct {
	StackID string `json:"stackId"`
	Service string `json:"service,omitempty"`
	Tail    int    `json:"tail,omitempty"`
	Follow  bool   `json:"follow,omitempty"`
}

type composeOperationResponse struct {
	StackID string `json:"stackId"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

type composePullResponse struct {
	StackID string `json:"stackId"`
	Output  string `json:"output,omitempty"`
}

func (s *Server) composeStackManager() *composeStack {
	return s.composeStacks
}

func (s *Server) handleComposeDeploy(w http.ResponseWriter, r *http.Request) {
	var req composeDeployRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4*1024*1024)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request: "+err.Error())
		return
	}
	if req.StackID == "" || req.ComposeYAML == "" {
		writeError(w, http.StatusBadRequest, "stackId and composeYaml are required")
		return
	}
	if !validStackID(req.StackID) {
		writeError(w, http.StatusBadRequest, "invalid stackId")
		return
	}
	if err := validateComposePolicy(req.ComposeYAML); err != nil {
		writeError(w, http.StatusBadRequest, "compose policy violation: "+err.Error())
		return
	}

	cs := s.composeStackManager()
	cs.lock(req.StackID)
	defer cs.unlock(req.StackID)

	stackDir := cs.dirForID(req.StackID)
	_ = os.RemoveAll(stackDir)
	if err := os.MkdirAll(stackDir, 0o750); err != nil {
		writeError(w, http.StatusInternalServerError, "create stack directory: "+err.Error())
		return
	}

	composePath := filepath.Join(stackDir, "compose.yaml")
	if err := os.WriteFile(composePath, []byte(req.ComposeYAML), 0o640); err != nil {
		writeError(w, http.StatusInternalServerError, "write compose file: "+err.Error())
		return
	}

	if len(req.EnvVars) > 0 {
		envContent, err := encodeComposeEnv(req.EnvVars)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid environment variables: "+err.Error())
			return
		}
		if err := os.WriteFile(filepath.Join(stackDir, ".env"), envContent, 0o640); err != nil {
			writeError(w, http.StatusInternalServerError, "write env file: "+err.Error())
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	upArgs := []string{"compose", "-f", composePath, "-p", req.StackID, "up", "-d"}
	if req.RemoveOrphans {
		upArgs = append(upArgs, "--remove-orphans")
	}
	cmd := exec.CommandContext(ctx, "docker", upArgs...)
	cmd.Dir = stackDir
	output, err := cmd.CombinedOutput()

	if err != nil {
		writeJSON(w, http.StatusConflict, composeOperationResponse{
			StackID: req.StackID,
			Output:  string(output),
			Error:   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, composeDeployResponse{
		StackID: req.StackID,
		Output:  string(output),
	})
}

func (s *Server) handleComposeStop(w http.ResponseWriter, r *http.Request) {
	stackID := r.PathValue("stackId")
	if stackID == "" {
		writeError(w, http.StatusBadRequest, "stackId is required")
		return
	}
	if !validStackID(stackID) {
		writeError(w, http.StatusBadRequest, "invalid stackId")
		return
	}

	cs := s.composeStackManager()
	cs.lock(stackID)
	defer cs.unlock(stackID)

	stackDir := cs.dirForID(stackID)
	composePath := filepath.Join(stackDir, "compose.yaml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "compose file not found for stack "+stackID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "-p", stackID, "stop")
	cmd.Dir = stackDir
	output, err := cmd.CombinedOutput()

	if err != nil {
		writeJSON(w, http.StatusConflict, composeOperationResponse{
			StackID: stackID,
			Output:  string(output),
			Error:   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, composeOperationResponse{
		StackID: stackID,
		Output:  string(output),
	})
}

func (s *Server) handleComposeStart(w http.ResponseWriter, r *http.Request) {
	stackID := r.PathValue("stackId")
	if stackID == "" {
		writeError(w, http.StatusBadRequest, "stackId is required")
		return
	}
	if !validStackID(stackID) {
		writeError(w, http.StatusBadRequest, "invalid stackId")
		return
	}

	cs := s.composeStackManager()
	cs.lock(stackID)
	defer cs.unlock(stackID)

	stackDir := cs.dirForID(stackID)
	composePath := filepath.Join(stackDir, "compose.yaml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "compose file not found for stack "+stackID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "-p", stackID, "start")
	cmd.Dir = stackDir
	output, err := cmd.CombinedOutput()

	if err != nil {
		writeJSON(w, http.StatusConflict, composeOperationResponse{
			StackID: stackID,
			Output:  string(output),
			Error:   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, composeOperationResponse{
		StackID: stackID,
		Output:  string(output),
	})
}

func (s *Server) handleComposeRestart(w http.ResponseWriter, r *http.Request) {
	stackID := r.PathValue("stackId")
	if stackID == "" {
		writeError(w, http.StatusBadRequest, "stackId is required")
		return
	}
	if !validStackID(stackID) {
		writeError(w, http.StatusBadRequest, "invalid stackId")
		return
	}

	cs := s.composeStackManager()
	cs.lock(stackID)
	defer cs.unlock(stackID)

	stackDir := cs.dirForID(stackID)
	composePath := filepath.Join(stackDir, "compose.yaml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "compose file not found for stack "+stackID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "-p", stackID, "restart")
	cmd.Dir = stackDir
	output, err := cmd.CombinedOutput()

	if err != nil {
		writeJSON(w, http.StatusConflict, composeOperationResponse{
			StackID: stackID,
			Output:  string(output),
			Error:   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, composeOperationResponse{
		StackID: stackID,
		Output:  string(output),
	})
}

func (s *Server) handleComposeDelete(w http.ResponseWriter, r *http.Request) {
	stackID := r.PathValue("stackId")
	if stackID == "" {
		writeError(w, http.StatusBadRequest, "stackId is required")
		return
	}
	if !validStackID(stackID) {
		writeError(w, http.StatusBadRequest, "invalid stackId")
		return
	}

	cs := s.composeStackManager()
	cs.lock(stackID)
	defer cs.unlock(stackID)

	stackDir := cs.dirForID(stackID)
	composePath := filepath.Join(stackDir, "compose.yaml")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	// RemoveOrphans is opt-in (see composeDeployRequest.RemoveOrphans doc)
	// and defaults to false to avoid unexpectedly killing unrelated
	// containers docker compose associates with this project name.
	removeOrphans := r.URL.Query().Get("removeOrphans") == "true"

	var output string
	if _, err := os.Stat(composePath); err == nil {
		downArgs := []string{"compose", "-f", composePath, "-p", stackID, "down", "-v"}
		if removeOrphans {
			downArgs = append(downArgs, "--remove-orphans")
		}
		cmd := exec.CommandContext(ctx, "docker", downArgs...)
		cmd.Dir = stackDir
		downOutput, downErr := cmd.CombinedOutput()
		output = string(downOutput)
		if downErr != nil {
			writeJSON(w, http.StatusConflict, composeOperationResponse{
				StackID: stackID,
				Output:  output,
				Error:   downErr.Error(),
			})
			return
		}
	}

	_ = os.RemoveAll(stackDir)

	writeJSON(w, http.StatusOK, composeOperationResponse{
		StackID: stackID,
		Output:  output,
	})
}

func (s *Server) handleComposeStatus(w http.ResponseWriter, r *http.Request) {
	stackID := r.PathValue("stackId")
	if stackID == "" {
		writeError(w, http.StatusBadRequest, "stackId is required")
		return
	}
	if !validStackID(stackID) {
		writeError(w, http.StatusBadRequest, "invalid stackId")
		return
	}

	cs := s.composeStackManager()
	stackDir := cs.dirForID(stackID)
	composePath := filepath.Join(stackDir, "compose.yaml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		writeJSON(w, http.StatusOK, composeStatusResponse{
			StackID:  stackID,
			Services: []composeServiceState{},
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "-p", stackID, "ps", "--format", "json")
	cmd.Dir = stackDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		writeError(w, http.StatusConflict, "compose ps failed: "+err.Error())
		return
	}

	services := []composeServiceState{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		var raw map[string]string
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		services = append(services, composeServiceState{
			Name:   raw["Name"],
			Image:  raw["Image"],
			Status: raw["Status"],
			State:  raw["State"],
			Ports:  raw["Ports"],
		})
	}

	writeJSON(w, http.StatusOK, composeStatusResponse{
		StackID:  stackID,
		Services: services,
	})
}

func (s *Server) handleComposeLogs(w http.ResponseWriter, r *http.Request) {
	stackID := r.PathValue("stackId")
	if stackID == "" {
		writeError(w, http.StatusBadRequest, "stackId is required")
		return
	}
	if !validStackID(stackID) {
		writeError(w, http.StatusBadRequest, "invalid stackId")
		return
	}

	cs := s.composeStackManager()
	stackDir := cs.dirForID(stackID)
	composePath := filepath.Join(stackDir, "compose.yaml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "compose file not found for stack "+stackID)
		return
	}

	tail := "100"
	if t := r.URL.Query().Get("tail"); t != "" {
		tail = t
	}
	service := r.URL.Query().Get("service")

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	args := []string{"compose", "-f", composePath, "-p", stackID, "logs", "--no-color", "--tail", tail}
	if service != "" {
		args = append(args, service)
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = stackDir
	output, err := cmd.CombinedOutput()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if err != nil {
		http.Error(w, string(output)+"\n"+err.Error(), http.StatusConflict)
		return
	}
	_, _ = w.Write(output)
}

func (s *Server) handleComposePull(w http.ResponseWriter, r *http.Request) {
	stackID := r.PathValue("stackId")
	if stackID == "" {
		writeError(w, http.StatusBadRequest, "stackId is required")
		return
	}
	if !validStackID(stackID) {
		writeError(w, http.StatusBadRequest, "invalid stackId")
		return
	}

	cs := s.composeStackManager()
	cs.lock(stackID)
	defer cs.unlock(stackID)

	stackDir := cs.dirForID(stackID)
	composePath := filepath.Join(stackDir, "compose.yaml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "compose file not found for stack "+stackID)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "-p", stackID, "pull")
	cmd.Dir = stackDir
	output, err := cmd.CombinedOutput()

	if err != nil {
		writeJSON(w, http.StatusConflict, composeOperationResponse{
			StackID: stackID,
			Output:  string(output),
			Error:   err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, composePullResponse{
		StackID: stackID,
		Output:  string(output),
	})
}
