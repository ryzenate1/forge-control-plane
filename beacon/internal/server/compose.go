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
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type composeLockEntry struct {
	mu sync.Mutex
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
}

func (cs *composeStack) unlock(stackID string) {
	cs.mu.RLock()
	entry := cs.stacks[stackID]
	cs.mu.RUnlock()
	if entry != nil {
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

// validateComposePolicy parses the compose file as YAML and enforces a strict
// schema. Substring matching is not used: values like "privileged: yes" or
// "network_mode: host" must be caught regardless of quoting or YAML scalar
// type, and only real compose keys are validated.
func validateComposePolicy(yamlContent string) error {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(yamlContent), &doc); err != nil {
		return fmt.Errorf("compose YAML is invalid: %w", err)
	}
	services, ok := doc["services"].(map[string]any)
	if !ok {
		return errors.New("compose file must define a 'services' map")
	}
	for name, raw := range services {
		svc, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("service %q must be a mapping", name)
		}
		for key, value := range svc {
			switch key {
			case "privileged":
				if composeTruthy(value) {
					return fmt.Errorf("service %q: privileged mode is not allowed in compose deployments", name)
				}
			case "network_mode":
				mode := composeString(value)
				if mode == "host" || strings.HasPrefix(mode, "service:") {
					return fmt.Errorf("service %q: host or shared-service network mode is not allowed", name)
				}
			case "pid":
				if composeString(value) == "host" {
					return fmt.Errorf("service %q: host pid namespace is not allowed", name)
				}
			case "userns_mode":
				if composeString(value) == "host" {
					return fmt.Errorf("service %q: host userns mode is not allowed", name)
				}
			case "cap_add":
				if composeNonEmpty(value) {
					return fmt.Errorf("service %q: cap_add is not allowed in compose deployments", name)
				}
			case "devices":
				if composeNonEmpty(value) {
					return fmt.Errorf("service %q: device passthrough is not allowed in compose deployments", name)
				}
			case "security_opt":
				if composeNonEmpty(value) {
					return fmt.Errorf("service %q: security_opt is not allowed in compose deployments", name)
				}
			case "ports":
				if err := validateComposePorts(name, value); err != nil {
					return err
				}
			case "volumes":
				if err := validateComposeVolumes(name, value); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// composeTruthy reports whether a YAML scalar is truthy, including the YAML
// boolean spellings yes/on/true/1 and their quoted string forms.
func composeTruthy(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "yes", "on", "1":
			return true
		}
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	}
	return false
}

func composeString(value any) string {
	if s, ok := value.(string); ok {
		return strings.ToLower(strings.TrimSpace(s))
	}
	return ""
}

func composeNonEmpty(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(v) != ""
	case []any:
		return len(v) > 0
	case []string:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	default:
		return true
	}
}

func validateComposePorts(service string, value any) error {
	entries, ok := value.([]any)
	if !ok {
		return nil
	}
	for _, entry := range entries {
		var published string
		switch v := entry.(type) {
		case string:
			published = shortFormHostPort(v)
		case map[string]any:
			if raw, ok := v["published"]; ok {
				published = fmt.Sprint(raw)
			}
		default:
			continue
		}
		if published == "" {
			continue
		}
		port, err := strconv.Atoi(strings.TrimSpace(published))
		if err != nil || port <= 0 {
			continue
		}
		if port < 1024 {
			return fmt.Errorf("service %q: publishing privileged host port %d is not allowed in compose deployments", service, port)
		}
	}
	return nil
}

// shortFormHostPort extracts the host-side port from a short-form ports
// entry ("80", "8080:80", "127.0.0.1:8080:80").
func shortFormHostPort(entry string) string {
	parts := strings.Split(entry, ":")
	switch len(parts) {
	case 2:
		return parts[0]
	case 3:
		return parts[1]
	default:
		return ""
	}
}

func validateComposeVolumes(service string, value any) error {
	entries, ok := value.([]any)
	if !ok {
		return nil
	}
	for _, entry := range entries {
		switch v := entry.(type) {
		case map[string]any:
			if composeString(v["type"]) == "bind" {
				return fmt.Errorf("service %q: long-form bind mounts are not allowed in compose deployments", service)
			}
		case string:
			source, _, hasSource := strings.Cut(v, ":")
			if !hasSource || source == "" {
				// Anonymous volume ("/data") or a bare container target.
				continue
			}
			if strings.HasPrefix(source, "/") || isPathTraversal(source) {
				return fmt.Errorf("service %q: host bind mount source %q is not allowed in compose deployments", service, source)
			}
		}
	}
	return nil
}

func isPathTraversal(source string) bool {
	clean := pathpkg.Clean(strings.ReplaceAll(source, "\\", "/"))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return true
	}
	for _, part := range strings.Split(clean, "/") {
		if part == ".." {
			return true
		}
	}
	return false
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
	cs.lock(stackID)
	defer cs.unlock(stackID)

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
