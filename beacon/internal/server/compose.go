package server

import (
	"context"
	"encoding/json"
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
	mu       sync.RWMutex
	stacks   map[string]*composeLockEntry
	dir      string
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
	return !strings.Contains(stackID, "..") && !strings.Contains(stackID, "/") && stackID != "" && stackID != "."
}

func (cs *composeStack) dirForID(stackID string) string {
	if !validStackID(stackID) {
		return filepath.Join(cs.dir, "_invalid_"+stackID)
	}
	return filepath.Join(cs.dir, stackID)
}

type composeDeployRequest struct {
	StackID       string `json:"stackId"`
	ComposeYAML   string `json:"composeYaml"`
	EnvVars       map[string]string `json:"envVars,omitempty"`
}

type composeDeployResponse struct {
	StackID string `json:"stackId"`
	Output  string `json:"output,omitempty"`
}

type composeStatusResponse struct {
	StackID  string              `json:"stackId"`
	Services []composeServiceState `json:"services"`
}

type composeServiceState struct {
	Name    string `json:"name"`
	Image   string `json:"image"`
	Status  string `json:"status"`
	State   string `json:"state"`
	Ports   string `json:"ports"`
}

type composeLogsRequest struct {
	StackID  string `json:"stackId"`
	Service  string `json:"service,omitempty"`
	Tail     int    `json:"tail,omitempty"`
	Follow   bool   `json:"follow,omitempty"`
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
		envContent := ""
		for key, value := range req.EnvVars {
			envContent += fmt.Sprintf("%s=%s\n", key, value)
		}
		if err := os.WriteFile(filepath.Join(stackDir, ".env"), []byte(envContent), 0o640); err != nil {
			writeError(w, http.StatusInternalServerError, "write env file: "+err.Error())
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "-p", req.StackID, "up", "-d", "--remove-orphans")
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

	var output string
	if _, err := os.Stat(composePath); err == nil {
		cmd := exec.CommandContext(ctx, "docker", "compose", "-f", composePath, "-p", stackID, "down", "-v", "--remove-orphans")
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
