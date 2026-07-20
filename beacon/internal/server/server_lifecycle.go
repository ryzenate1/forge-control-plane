package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gamepanel/beacon/internal/runtime"
	"gamepanel/beacon/internal/serverid"
)

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		http.Error(w, errRuntimeUnavailable.Error(), http.StatusServiceUnavailable)
		return
	}
	var body struct {
		ServerID string   `json:"serverId"`
		Image    string   `json:"image"`
		Command  []string `json:"command"`
		Env      []string `json:"env"`
		Ports    []struct {
			HostIP        string `json:"hostIp"`
			HostPort      int    `json:"hostPort"`
			ContainerPort int    `json:"containerPort"`
			Protocol      string `json:"protocol"`
		} `json:"ports"`
		Mounts          []mountConfiguration  `json:"mounts"`
		MemoryMB        int64                 `json:"memoryMb"`
		SwapMB          int64                 `json:"swapMb"`
		CPUShares       int64                 `json:"cpuShares"`
		CPUPercent      int64                 `json:"cpuPercent"`
		CPUSet          string                `json:"cpuSet"`
		IOWeight        int64                 `json:"ioWeight"`
		OOMKillDisabled bool                  `json:"oomKillDisabled"`
		PIDLimit        int64                 `json:"pidLimit"`
		StopSignal      string                `json:"stopSignal"`
		StopTimeout     int64                 `json:"stopTimeoutSeconds"`
		UID             int                   `json:"uid"`
		GID             int                   `json:"gid"`
		DNS             []string              `json:"dns"`
		NetworkName     string                `json:"networkName"`
		NetworkSubnet   string                `json:"networkSubnet"`
		NetworkGateway  string                `json:"networkGateway"`
		NetworkIP       string                `json:"networkIp"`
		Start           bool                  `json:"start"`
		RegistryAuth    *runtime.RegistryAuth `json:"registryAuth"`
		DiskMB          int64                 `json:"diskMb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if body.ServerID == "" || body.Image == "" {
		http.Error(w, "serverId and image are required", http.StatusBadRequest)
		return
	}
	if err := serverid.Validate(body.ServerID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	rootDir, err := s.safePath(body.ServerID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(rootDir, 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ports := make([]runtime.PortBinding, 0, len(body.Ports))
	for _, port := range body.Ports {
		ports = append(ports, runtime.PortBinding{
			HostIP:        port.HostIP,
			HostPort:      port.HostPort,
			ContainerPort: port.ContainerPort,
			Protocol:      port.Protocol,
		})
	}
	mounts, err := s.runtimeMounts(body.Mounts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	createRequest := runtime.CreateRequest{
		ServerID: body.ServerID,
		Image:    body.Image,
		Command:  body.Command,
		Env:      s.effectiveEnvList(body.ServerID, body.Env),
		Ports:    ports,
		Mounts:   mounts,
		MemoryMB: body.MemoryMB, SwapMB: body.SwapMB, CPUShares: body.CPUShares, CPUPercent: body.CPUPercent,
		CPUSet: body.CPUSet, IOWeight: body.IOWeight, OOMKillDisabled: body.OOMKillDisabled, PIDLimit: body.PIDLimit,
		StopSignal: body.StopSignal, StopTimeout: time.Duration(body.StopTimeout) * time.Second, UID: body.UID, GID: body.GID,
		DNS: body.DNS, NetworkName: body.NetworkName, NetworkSubnet: body.NetworkSubnet, NetworkGateway: body.NetworkGateway,
		NetworkIP: body.NetworkIP, RegistryAuth: body.RegistryAuth, RootDir: rootDir,
	}
	if reconciler, ok := s.runtime.(runtime.Reconciler); ok {
		err = reconciler.Reconcile(r.Context(), createRequest)
	} else {
		err = s.runtime.Create(r.Context(), createRequest)
	}
	if err != nil {
		http.Error(w, err.Error(), runtimeErrorStatus(err, http.StatusConflict))
		return
	}
	if body.Start {
		if err := s.runtime.Start(r.Context(), body.ServerID); err != nil {
			http.Error(w, fmt.Sprintf("start workload: %v", err), runtimeErrorStatus(err, http.StatusConflict))
			return
		}
	}

	createRequest.RegistryAuth = nil
	if err := s.persistRuntimeRequest(body.ServerID, createRequest); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.manager.MarkCreated(body.ServerID, rootDir, body.DiskMB)
	writeJSON(w, http.StatusAccepted, map[string]any{"serverId": body.ServerID, "accepted": true, "mode": "docker"})
}

func (s *Server) syncConfiguration(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(r.Body, 2*1024*1024)).Decode(&payload); err != nil {
		http.Error(w, "invalid configuration", http.StatusBadRequest)
		return
	}
	configPath, err := s.safePath(serverID, filepath.Join(".config", "server.json"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.WriteFile(configPath, body, 0o640); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.applyConfigurationFiles(serverID, payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if s.runtime != nil {
		if desired, ok, err := s.runtimeRequestFromConfiguration(serverID, payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		} else if ok {
			if err := s.reconcileRuntimeConfiguration(r.Context(), desired); err != nil {
				http.Error(w, err.Error(), runtimeErrorStatus(err, http.StatusConflict))
				return
			}
			if err := s.persistRuntimeRequest(serverID, desired); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	s.manager.UpdateRuntimeConfig(serverID, memoryMBFromConfiguration(payload), allocationIPFromConfiguration(payload), allocationPortFromConfiguration(payload), stopTypeFromConfiguration(payload), stopValueFromConfiguration(payload), stopTimeoutFromConfiguration(payload))
	s.manager.MarkConfigurationSynced(serverID, diskLimitMBFromConfiguration(payload))
	writeJSON(w, http.StatusOK, map[string]any{"serverId": serverID, "synced": true})
}

func (s *Server) persistRuntimeRequest(serverID string, req runtime.CreateRequest) error {
	path, err := s.safePath(serverID, filepath.Join(".config", "runtime.json"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	req.RegistryAuth = nil
	body, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}

func (s *Server) runtimeRequestFromConfiguration(serverID string, payload map[string]any) (runtime.CreateRequest, bool, error) {
	path, err := s.safePath(serverID, filepath.Join(".config", "runtime.json"))
	if err != nil {
		return runtime.CreateRequest{}, false, err
	}
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return runtime.CreateRequest{}, false, nil
	}
	if err != nil {
		return runtime.CreateRequest{}, false, err
	}
	var req runtime.CreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return runtime.CreateRequest{}, false, err
	}
	if image, ok := payload["dockerImage"].(string); ok && strings.TrimSpace(image) != "" {
		req.Image = image
	}
	if invocation, ok := payload["invocation"].(string); ok {
		req.Command = []string{"/bin/sh", "-lc", invocation}
	}
	if environment, ok := payload["environment"].(map[string]any); ok {
		req.Env = req.Env[:0]
		keys := make([]string, 0, len(environment))
		for key := range environment {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			req.Env = append(req.Env, key+"="+fmt.Sprint(environment[key]))
		}
	}
	build, _ := payload["build"].(map[string]any)
	req.MemoryMB = int64Value(build, "memoryLimit", "memory_limit")
	req.SwapMB = int64Value(build, "swapMb", "swap")
	req.CPUShares = int64Value(build, "cpuShares", "cpu_shares")
	req.CPUPercent = int64Value(build, "cpuLimit", "cpu_limit")
	if threads, ok := firstMapValue(build, "threads").(string); ok {
		req.CPUSet = threads
	}
	if value := int64Value(build, "ioWeight", "io_weight"); value != 0 {
		req.IOWeight = value
	}
	if value, ok := firstMapValue(build, "oomDisabled", "oom_disabled").(bool); ok {
		req.OOMKillDisabled = value
	}
	allocations, _ := payload["allocations"].(map[string]any)
	ports := make([]runtime.PortBinding, 0)
	if detailed, ok := allocations["ports"].([]any); ok {
		for _, raw := range detailed {
			entry, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			hostPort := int(anyInt64(firstMapValue(entry, "port", "hostPort")))
			containerPort := int(anyInt64(firstMapValue(entry, "containerPort")))
			if containerPort == 0 {
				containerPort = hostPort
			}
			protocol, _ := firstMapValue(entry, "protocol").(string)
			if protocol == "" {
				protocol = "tcp"
			}
			hostIP, _ := firstMapValue(entry, "ip", "hostIP").(string)
			ports = append(ports, runtime.PortBinding{HostIP: hostIP, HostPort: hostPort, ContainerPort: containerPort, Protocol: protocol})
		}
	}
	if len(ports) == 0 {
		mappings, _ := allocations["mappings"].(map[string]any)
		for ip, raw := range mappings {
			if values, ok := raw.([]any); ok {
				for _, value := range values {
					port := int(anyInt64(value))
					ports = append(ports, runtime.PortBinding{HostIP: ip, HostPort: port, ContainerPort: port, Protocol: "tcp"})
				}
			}
		}
	}
	if len(ports) > 0 {
		req.Ports = ports
	}
	mounts, err := s.runtimeMountsFromConfiguration(payload)
	if err != nil {
		return runtime.CreateRequest{}, false, err
	}
	if mounts != nil {
		req.Mounts = mounts
	}
	return req, true, nil
}

func (s *Server) getConfiguration(w http.ResponseWriter, r *http.Request) {
	configPath, err := s.safePath(r.PathValue("id"), filepath.Join(".config", "server.json"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	body, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "configuration not synced", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func (s *Server) install(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		http.Error(w, errRuntimeUnavailable.Error(), http.StatusServiceUnavailable)
		return
	}
	serverID := r.PathValue("id")
	var body struct {
		ServerID   string            `json:"serverId"`
		Image      string            `json:"image"`
		Entrypoint string            `json:"entrypoint"`
		Script     string            `json:"script"`
		Env        map[string]string `json:"env"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2*1024*1024)).Decode(&body); err != nil {
		http.Error(w, "invalid install request", http.StatusBadRequest)
		return
	}
	if body.ServerID != "" && body.ServerID != serverID {
		http.Error(w, "server id mismatch", http.StatusBadRequest)
		return
	}
	rootDir, err := s.safePath(serverID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(rootDir, 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	installDir, err := s.safePath(serverID, ".install")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(installDir, 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	scriptPath := filepath.Join(installDir, "install.sh")
	script := body.Script
	if strings.TrimSpace(script) == "" {
		script = "#!/bin/sh\nset -eu\necho \"No install script configured.\"\n"
	}
	if err := os.WriteFile(scriptPath, []byte(script), 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	env := s.effectiveEnvMapList(serverID, body.Env)

	// Mark as installing
	s.manager.MarkInstalling(serverID, true)

	// Execute installation
	result, err := s.runtime.Install(r.Context(), runtime.InstallRequest{
		ServerID:   serverID,
		Image:      body.Image,
		Entrypoint: body.Entrypoint,
		Script:     script,
		Env:        env,
		RootDir:    rootDir,
	})
	if err != nil {
		s.manager.MarkInstalling(serverID, false)
		s.notifyPanelInstallStatus(serverID, false, err.Error())
		http.Error(w, err.Error(), runtimeErrorStatus(err, http.StatusConflict))
		return
	}

	// Save logs
	logPath := filepath.Join(installDir, "install.log")
	_ = os.WriteFile(logPath, []byte(result.Logs), 0o640)

	// Mark installation complete
	s.manager.MarkInstalling(serverID, false)

	// Notify Panel of installation status
	success := result.ExitCode == 0
	errorMsg := ""
	if !success {
		errorMsg = "install script failed with exit code " + strconv.Itoa(result.ExitCode)
	}
	s.notifyPanelInstallStatus(serverID, success, errorMsg)

	if result.ExitCode != 0 {
		http.Error(w, "install script failed with exit code "+strconv.Itoa(result.ExitCode), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"serverId": serverID, "accepted": true, "mode": "docker", "exitCode": result.ExitCode, "logs": result.Logs})
}

func (s *Server) installWS(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		http.Error(w, errRuntimeUnavailable.Error(), http.StatusServiceUnavailable)
		return
	}
	conn, err := websocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	defer s.trackWebSocket(r, conn)()

	serverID := r.PathValue("id")

	// Send initial status
	conn.WriteJSON(map[string]interface{}{
		"type": "status",
		"data": "Starting installation...",
	})

	// Read installation request from WebSocket
	var body struct {
		Image      string            `json:"image"`
		Entrypoint string            `json:"entrypoint"`
		Script     string            `json:"script"`
		Env        map[string]string `json:"env"`
	}
	if err := conn.ReadJSON(&body); err != nil {
		conn.WriteJSON(map[string]interface{}{
			"type":  "error",
			"data":  "Invalid install request",
			"error": err.Error(),
		})
		return
	}

	rootDir, err := s.safePath(serverID, "")
	if err != nil {
		conn.WriteJSON(map[string]interface{}{
			"type": "error",
			"data": err.Error(),
		})
		return
	}

	installDir, err := s.safePath(serverID, ".install")
	if err != nil {
		conn.WriteJSON(map[string]interface{}{
			"type": "error",
			"data": err.Error(),
		})
		return
	}

	if err := os.MkdirAll(installDir, 0o750); err != nil {
		conn.WriteJSON(map[string]interface{}{
			"type": "error",
			"data": err.Error(),
		})
		return
	}

	scriptPath := filepath.Join(installDir, "install.sh")
	script := body.Script
	if strings.TrimSpace(script) == "" {
		script = "#!/bin/sh\nset -eu\necho \"No install script configured.\"\n"
	}

	if err := os.WriteFile(scriptPath, []byte(script), 0o750); err != nil {
		conn.WriteJSON(map[string]interface{}{
			"type": "error",
			"data": err.Error(),
		})
		return
	}

	env := s.effectiveEnvMapList(serverID, body.Env)

	s.manager.MarkInstalling(serverID, true)

	conn.WriteJSON(map[string]interface{}{
		"type": "status",
		"data": "Running install script...",
	})

	result, err := s.runtime.Install(r.Context(), runtime.InstallRequest{
		ServerID:   serverID,
		Image:      body.Image,
		Entrypoint: body.Entrypoint,
		Script:     script,
		Env:        env,
		RootDir:    rootDir,
	})
	if err != nil {
		s.manager.MarkInstalling(serverID, false)
		conn.WriteJSON(map[string]interface{}{
			"type": "error",
			"data": err.Error(),
		})
		s.notifyPanelInstallStatus(serverID, false, err.Error())
		return
	}

	// Stream logs
	for _, line := range strings.Split(result.Logs, "\n") {
		if line != "" {
			conn.WriteJSON(map[string]interface{}{
				"type": "log",
				"data": line,
			})
		}
	}

	// Save logs
	logPath := filepath.Join(installDir, "install.log")
	_ = os.WriteFile(logPath, []byte(result.Logs), 0o640)

	s.manager.MarkInstalling(serverID, false)

	success := result.ExitCode == 0
	errorMsg := ""
	if !success {
		errorMsg = "install script failed with exit code " + strconv.Itoa(result.ExitCode)
	}
	s.notifyPanelInstallStatus(serverID, success, errorMsg)

	conn.WriteJSON(map[string]interface{}{
		"type":     "complete",
		"success":  success,
		"exitCode": result.ExitCode,
		"error":    errorMsg,
	})
}

func (s *Server) reinstall(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")

	// Check if server is running
	state := s.manager.State(serverID)
	if state.PowerState == PowerStateRunning || state.PowerState == PowerStateStarting {
		http.Error(w, "server must be stopped before reinstalling", http.StatusConflict)
		return
	}

	// Forward to install handler (reinstall is just install with server stopped)
	s.install(w, r)
}

// notifyPanelInstallStatus notifies the Panel API of installation completion
func (s *Server) notifyPanelInstallStatus(serverID string, success bool, errorMsg string) {
	if s.panelClient == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.panelClient.SetInstallationStatus(ctx, serverID, success); err != nil {
		log.Printf("[beacon] failed to notify panel of install status for %s: %v", serverID, err)
	}
}

func (s *Server) power(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	var body struct {
		Signal string `json:"signal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	switch body.Signal {
	case "start", "stop", "restart", "kill":
		if s.runtime == nil {
			http.Error(w, errRuntimeUnavailable.Error(), http.StatusServiceUnavailable)
			return
		}
		commandID := strings.TrimSpace(r.Header.Get("X-Forge-Command-ID"))
		if commandID == "" {
			commandID = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		}
		op, err := s.operations.EnqueueCommand(r.Context(), commandID, serverID, OperationType(body.Signal))
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		// Preserve the existing error contract while execution continues on the
		// server context if the caller disconnects.
		for {
			status, statusErr := s.operations.GetStatus(op.ID)
			if statusErr != nil {
				http.Error(w, statusErr.Error(), http.StatusInternalServerError)
				return
			}
			switch status.Status {
			case StatusCompleted:
				writeJSON(w, http.StatusAccepted, map[string]any{"serverId": serverID, "signal": body.Signal, "accepted": true, "mode": "docker", "operationId": op.ID})
				return
			case StatusFailed:
				err := errors.New(status.Error)
				http.Error(w, err.Error(), runtimeErrorStatus(err, http.StatusConflict))
				return
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	default:
		http.Error(w, "invalid power signal", http.StatusBadRequest)
	}
}

func (s *Server) getOperation(w http.ResponseWriter, r *http.Request) {
	op, err := s.operations.GetStatus(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, op)
}

func (s *Server) listOperations(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.operations.ListByServer(r.PathValue("id")))
}

func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		http.Error(w, errRuntimeUnavailable.Error(), http.StatusServiceUnavailable)
		return
	}
	serverID := r.PathValue("id")
	s.consoles.Stop(serverID)
	if err := s.runtime.Delete(r.Context(), serverID); err != nil {
		http.Error(w, err.Error(), runtimeErrorStatus(err, http.StatusConflict))
		return
	}
	s.manager.Delete(serverID)
	writeJSON(w, http.StatusAccepted, map[string]any{"serverId": serverID, "signal": "delete", "accepted": true, "mode": "docker"})
}

func (s *Server) applyPower(r *http.Request, serverID, signal string) (string, error) {
	if s.runtime == nil {
		return "", errRuntimeUnavailable
	}
	if signal != "start" && signal != "stop" && signal != "restart" && signal != "kill" {
		return "", errors.New("invalid power signal")
	}
	err := s.manager.HandlePower(r.Context(), serverID, signal)
	if err == nil {
		return "docker", nil
	}
	return "", err
}

func isContainerMissing(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such container") || strings.Contains(message, "not found")
}

func isImageMissing(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such image") || strings.Contains(message, "pull access denied")
}

func isContainerExists(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "already in use") || strings.Contains(message, "already exists")
}

func runtimeErrorStatus(err error, fallback int) int {
	if errors.Is(err, errRuntimeUnavailable) {
		return http.StatusServiceUnavailable
	}
	if isContainerMissing(err) || isImageMissing(err) {
		return http.StatusNotFound
	}
	if isContainerExists(err) {
		return http.StatusConflict
	}
	return fallback
}
