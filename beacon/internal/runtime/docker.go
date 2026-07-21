package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

func ptrInt64(v int64) *int64 { return &v }
func ptrBool(v bool) *bool    { return &v }

type DockerRuntime struct {
	client         *client.Client
	defaultNetwork string
}

func NewDockerRuntime() (*DockerRuntime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	networkName := strings.TrimSpace(os.Getenv("DAEMON_DOCKER_NETWORK"))
	if networkName == "" {
		networkName = "gamepanel"
	}
	return &DockerRuntime{client: cli, defaultNetwork: networkName}, nil
}

func (r *DockerRuntime) Provider() string { return ProviderDocker }

func (r *DockerRuntime) Ping(ctx context.Context) error {
	if r == nil || r.client == nil {
		return errors.New("docker runtime is not initialized")
	}
	_, err := r.client.Ping(ctx)
	return err
}

func (r *DockerRuntime) Create(ctx context.Context, req CreateRequest) error {
	existing, err := r.Inspect(ctx, req.ServerID)
	if err != nil {
		return err
	}
	if existing.Exists {
		return fmt.Errorf("workload %q already exists", req.ServerID)
	}
	return r.Reconcile(ctx, req)
}

// Reconcile applies req to a new or existing Docker workload. Because bind
// mounts cannot be changed in place, an existing workload is safely stopped,
// recreated from the complete desired request, and restarted if it was running.
func (r *DockerRuntime) Reconcile(ctx context.Context, req CreateRequest) error {
	if err := validateCreateRequest(req); err != nil {
		return err
	}
	mounts, err := buildContainerMounts(req.RootDir, req.Mounts)
	if err != nil {
		return err
	}
	exposedPorts, portBindings, err := dockerPorts(req.Ports)
	if err != nil {
		return err
	}
	if err := r.ensureNetwork(ctx, req); err != nil {
		return err
	}
	hash, err := createRequestHash(req)
	if err != nil {
		return err
	}
	name := containerName(req.ServerID)
	restartAfterCreate := false
	if existing, inspectErr := r.client.ContainerInspect(ctx, name); inspectErr == nil {
		if existing.Config != nil && existing.Config.Labels[configHashLabel] == hash {
			return nil
		}
		if existing.State != nil && existing.State.Running {
			restartAfterCreate = true
			timeout := 30
			if err := r.client.ContainerStop(ctx, existing.ID, container.StopOptions{Timeout: &timeout}); err != nil {
				return fmt.Errorf("stop workload for configuration reconciliation: %w", err)
			}
		}
		if err := r.client.ContainerRemove(ctx, existing.ID, container.RemoveOptions{RemoveVolumes: false}); err != nil {
			return fmt.Errorf("remove stale stopped container: %w", err)
		}
	} else if !errdefs.IsNotFound(inspectErr) {
		return inspectErr
	}

	if _, _, err := r.client.ImageInspectWithRaw(ctx, req.Image); err != nil {
		pullOptions, err := imagePullOptions(req.RegistryAuth)
		if err != nil {
			return err
		}
		pull, err := r.client.ImagePull(ctx, req.Image, pullOptions)
		if err != nil {
			return fmt.Errorf("pull image %q: %w", req.Image, err)
		}
		_, copyErr := io.Copy(io.Discard, pull)
		closeErr := pull.Close()
		if copyErr != nil {
			return fmt.Errorf("read image pull response: %w", copyErr)
		}
		if closeErr != nil {
			return closeErr
		}
	}

	config := buildContainerConfig(req, exposedPorts, hash)
	created, err := r.client.ContainerCreate(ctx, config, buildHostConfig(req, mounts, portBindings), buildNetworkingConfig(req), nil, name)
	if err != nil {
		return err
	}
	if restartAfterCreate {
		if err := r.client.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
			return fmt.Errorf("restart workload after configuration reconciliation: %w", err)
		}
	}
	return nil
}

func (r *DockerRuntime) Install(ctx context.Context, req InstallRequest) (InstallResult, error) {
	rootDir, err := validateRootDir(req.RootDir)
	if err != nil {
		return InstallResult{}, err
	}
	if req.Image == "" {
		req.Image = "alpine:3.21"
	}
	if req.Entrypoint == "" {
		req.Entrypoint = "sh"
	}
	if err := r.ensureExistingNetwork(ctx, r.defaultNetwork); err != nil {
		return InstallResult{}, err
	}
	if _, _, err := r.client.ImageInspectWithRaw(ctx, req.Image); err != nil {
		pull, err := r.client.ImagePull(ctx, req.Image, image.PullOptions{})
		if err != nil {
			return InstallResult{}, err
		}
		_, _ = io.Copy(io.Discard, pull)
		_ = pull.Close()
	}

	name := containerName(req.ServerID) + "-installer"
	_ = r.client.ContainerRemove(ctx, name, container.RemoveOptions{Force: true, RemoveVolumes: true})
	resp, err := r.client.ContainerCreate(
		ctx,
		&container.Config{
			Image:      req.Image,
			Cmd:        []string{req.Entrypoint, "-lc", req.Script},
			Env:        req.Env,
			WorkingDir: "/mnt/server",
			Labels: map[string]string{
				"modern-game-panel.server_id": req.ServerID,
				"modern-game-panel.job":       "install",
			},
		},
		&container.HostConfig{
			Mounts: []mount.Mount{
				{Type: mount.TypeBind, Source: rootDir, Target: "/mnt/server"},
			},
			NetworkMode:    container.NetworkMode(r.defaultNetwork),
			CapDrop:        []string{"ALL"},
			Privileged:     false,
			Init:           ptrBool(true),
			ReadonlyRootfs: true,
			SecurityOpt:    []string{"no-new-privileges:true"},
			Tmpfs: map[string]string{
				"/tmp": "rw,exec,size=64M",
			},
		},
		nil,
		nil,
		name,
	)
	if err != nil {
		return InstallResult{}, err
	}
	if err := r.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return InstallResult{}, err
	}
	waitCh, errCh := r.client.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
	var statusCode int64
	select {
	case wait := <-waitCh:
		statusCode = wait.StatusCode
	case err := <-errCh:
		return InstallResult{}, err
	case <-ctx.Done():
		_ = r.client.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{Force: true, RemoveVolumes: true})
		return InstallResult{}, ctx.Err()
	}
	logsReader, err := r.client.ContainerLogs(context.Background(), resp.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return InstallResult{}, err
	}
	defer logsReader.Close()
	var raw bytes.Buffer
	_, _ = io.Copy(&raw, io.LimitReader(logsReader, 1024*1024))
	_ = r.client.ContainerRemove(context.Background(), resp.ID, container.RemoveOptions{Force: true, RemoveVolumes: true})
	return InstallResult{ExitCode: int(statusCode), Logs: raw.String()}, nil
}

func (r *DockerRuntime) Inspect(ctx context.Context, serverID string) (ContainerState, error) {
	inspection, err := r.client.ContainerInspect(ctx, containerName(serverID))
	if err != nil {
		if errdefs.IsNotFound(err) {
			return ContainerState{ServerID: serverID, Exists: false}, nil
		}
		return ContainerState{}, err
	}
	state := ContainerState{ServerID: serverID, ID: inspection.ID, Exists: true}
	if inspection.State != nil {
		state.Running = inspection.State.Running
		state.Status = inspection.State.Status
	}
	return state, nil
}

func (r *DockerRuntime) List(ctx context.Context) ([]ContainerState, error) {
	args := filters.NewArgs(filters.Arg("label", "modern-game-panel.server_id"))
	containers, err := r.client.ContainerList(ctx, container.ListOptions{All: true, Filters: args})
	if err != nil {
		return nil, err
	}
	states := make([]ContainerState, 0, len(containers))
	for _, item := range containers {
		serverID := item.Labels["modern-game-panel.server_id"]
		if serverID == "" {
			continue
		}
		states = append(states, ContainerState{
			ServerID: serverID,
			ID:       item.ID,
			Exists:   true,
			Running:  strings.EqualFold(item.State, "running"),
			Status:   item.Status,
		})
	}
	return states, nil
}

func (r *DockerRuntime) Start(ctx context.Context, serverID string) error {
	return r.client.ContainerStart(ctx, containerName(serverID), container.StartOptions{})
}

func (r *DockerRuntime) SendCommand(ctx context.Context, serverID, command string) error {
	attached, err := r.client.ContainerAttach(ctx, containerName(serverID), container.AttachOptions{
		Stream: true,
		Stdin:  true,
	})
	if err != nil {
		return err
	}
	defer attached.Close()
	if !strings.HasSuffix(command, "\n") {
		command += "\n"
	}
	_, err = io.WriteString(attached.Conn, command)
	return err
}

func (r *DockerRuntime) Stop(ctx context.Context, serverID string) error {
	timeout := 30
	return r.client.ContainerStop(ctx, containerName(serverID), container.StopOptions{Timeout: &timeout})
}

func (r *DockerRuntime) WaitForStop(ctx context.Context, serverID string, duration time.Duration, terminate bool) error {
	if duration <= 0 {
		duration = 30 * time.Second
	}
	waitCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	waitCh, errCh := r.client.ContainerWait(waitCtx, containerName(serverID), container.WaitConditionNotRunning)
	select {
	case result := <-waitCh:
		if result.Error != nil {
			return errors.New(result.Error.Message)
		}
		return nil
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
	case <-waitCtx.Done():
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	if !terminate {
		return context.DeadlineExceeded
	}
	stopTimeout := 10
	if err := r.client.ContainerStop(ctx, containerName(serverID), container.StopOptions{Timeout: &stopTimeout}); err == nil {
		return nil
	}
	return r.Kill(ctx, serverID)
}

func (r *DockerRuntime) Kill(ctx context.Context, serverID string) error {
	return r.client.ContainerKill(ctx, containerName(serverID), "SIGKILL")
}

func (r *DockerRuntime) Signal(ctx context.Context, serverID, signal string) error {
	signal = strings.ToUpper(strings.TrimSpace(signal))
	if !validStopSignal(signal) {
		return fmt.Errorf("unsupported stop signal %q", signal)
	}
	return r.client.ContainerKill(ctx, containerName(serverID), signal)
}

func (r *DockerRuntime) Restart(ctx context.Context, serverID string) error {
	timeout := 30
	return r.client.ContainerRestart(ctx, containerName(serverID), container.StopOptions{Timeout: &timeout})
}

func (r *DockerRuntime) Stats(ctx context.Context, serverID string) (Stats, error) {
	response, err := r.client.ContainerStatsOneShot(ctx, containerName(serverID))
	if err != nil {
		return Stats{}, err
	}
	defer response.Body.Close()
	return DecodeDockerStats(response.Body)
}

func (r *DockerRuntime) Logs(ctx context.Context, serverID string) (io.ReadCloser, error) {
	return r.client.ContainerLogs(ctx, containerName(serverID), container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     false,
		Timestamps: true,
		Since:      time.Now().Add(-5 * time.Minute).Format(time.RFC3339),
	})
}

func (r *DockerRuntime) LogsStream(ctx context.Context, serverID string, tail string) (io.ReadCloser, error) {
	return r.client.ContainerLogs(ctx, containerName(serverID), container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: true,
		Tail:       tail,
	})
}

func (r *DockerRuntime) StatsStream(ctx context.Context, serverID string) (io.ReadCloser, error) {
	resp, err := r.client.ContainerStats(ctx, containerName(serverID), true)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (r *DockerRuntime) AttachConsole(ctx context.Context, serverID string) (ConsoleSession, error) {
	inspection, err := r.client.ContainerInspect(ctx, containerName(serverID))
	if err != nil {
		return nil, err
	}
	attached, err := r.client.ContainerAttach(ctx, containerName(serverID), container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
		Logs:   true,
	})
	if err != nil {
		return nil, err
	}
	if inspection.Config != nil && inspection.Config.Tty {
		return &dockerConsoleSession{response: attached, reader: attached.Reader}, nil
	}
	reader, writer := io.Pipe()
	session := &dockerConsoleSession{response: attached, reader: reader, pipeReader: reader}
	go func() {
		_, copyErr := stdcopy.StdCopy(writer, writer, attached.Reader)
		_ = writer.CloseWithError(copyErr)
	}()
	return session, nil
}

func (r *DockerRuntime) Delete(ctx context.Context, serverID string) error {
	return r.client.ContainerRemove(ctx, containerName(serverID), container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
}

func (r *DockerRuntime) WatchEvents(ctx context.Context) (<-chan ContainerEvent, <-chan error) {
	out := make(chan ContainerEvent)
	errs := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errs)
		backoff := 100 * time.Millisecond
		for ctx.Err() == nil {
			args := filters.NewArgs(filters.Arg("type", string(events.ContainerEventType)), filters.Arg("label", "modern-game-panel.server_id"))
			rawEvents, rawErrs := r.client.Events(ctx, events.ListOptions{Filters: args})
			connected := true
			for connected {
				select {
				case event, ok := <-rawEvents:
					if !ok {
						connected = false
						continue
					}
					backoff = 100 * time.Millisecond
					serverID := event.Actor.Attributes["modern-game-panel.server_id"]
					if serverID == "" {
						continue
					}
					exitCode, _ := strconv.Atoi(event.Actor.Attributes["exitCode"])
					oomKilled := false
					if strings.EqualFold(string(event.Action), "die") {
						if inspected, err := r.client.ContainerInspect(ctx, event.Actor.ID); err == nil && inspected.State != nil {
							oomKilled = inspected.State.OOMKilled
						}
					}
					select {
					case out <- ContainerEvent{ServerID: serverID, Action: string(event.Action), ExitCode: exitCode, OOMKilled: oomKilled}:
					case <-ctx.Done():
						return
					}
				case err, ok := <-rawErrs:
					if ok && err != nil {
						select {
						case errs <- err:
						default:
						}
					}
					connected = false
				case <-ctx.Done():
					return
				}
			}
			timer := time.NewTimer(backoff)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return
			}
			if backoff < 5*time.Second {
				backoff *= 2
				if backoff > 5*time.Second {
					backoff = 5 * time.Second
				}
			}
		}
	}()
	return out, errs
}

func containerName(serverID string) string {
	return "mgp-" + serverID
}

const serverContainerRoot = "/home/container"

func validateRootDir(rootDir string) (string, error) {
	if strings.TrimSpace(rootDir) == "" {
		return "", errors.New("root directory is required")
	}
	if !filepath.IsAbs(rootDir) {
		return "", errors.New("root directory must be absolute")
	}
	rootDir = filepath.Clean(rootDir)
	info, err := os.Stat(rootDir)
	if err != nil {
		return "", fmt.Errorf("root directory is unavailable: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("root directory is not a directory")
	}
	return rootDir, nil
}

func buildContainerMounts(rootDir string, custom []Mount) ([]mount.Mount, error) {
	rootDir, err := validateRootDir(rootDir)
	if err != nil {
		return nil, err
	}
	mounts := []mount.Mount{{
		Type:   mount.TypeBind,
		Source: rootDir,
		Target: serverContainerRoot,
	}}
	for _, customMount := range custom {
		if customMount.Source == "" || customMount.Target == "" {
			continue
		}
		if pathpkg.Clean(customMount.Target) == serverContainerRoot {
			return nil, errors.New("custom mount cannot replace /home/container")
		}
		mounts = append(mounts, mount.Mount{
			Type:     mount.TypeBind,
			Source:   customMount.Source,
			Target:   customMount.Target,
			ReadOnly: customMount.ReadOnly,
		})
	}
	return mounts, nil
}

func buildResources(req CreateRequest) container.Resources {
	period := int64(100000)
	quota := int64(0)
	if req.CPUPercent > 0 {
		quota = period * req.CPUPercent / 100
	}
	pids := req.PIDLimit
	if pids == 0 {
		pids = 256
	}
	memory := req.MemoryMB * 1024 * 1024
	if memory > 0 && req.MemoryOverhead > 0 {
		memory = int64(float64(memory) * (1 + req.MemoryOverhead/100))
	}
	memorySwap := int64(0)
	if memory > 0 {
		memorySwap = memory + req.SwapMB*1024*1024
	}
	return container.Resources{
		Memory: memory, MemorySwap: memorySwap, CPUShares: req.CPUShares,
		CPUPeriod: period, CPUQuota: quota, CpusetCpus: req.CPUSet,
		BlkioWeight: uint16(req.IOWeight), OomKillDisable: ptrBool(req.OOMKillDisabled), PidsLimit: ptrInt64(pids),
	}
}

func buildHostConfig(req CreateRequest, mounts []mount.Mount, portBindings nat.PortMap) *container.HostConfig {
	securityOpts := []string{"no-new-privileges:true"}
	usernsMode := ""
	if os.Getenv("DAEMON_DOCKER_ROOTLESS_ENABLED") == "true" {
		usernsMode = "host"
	}
	logMaxSize := os.Getenv("DAEMON_LOG_MAX_SIZE")
	if logMaxSize == "" {
		logMaxSize = "10m"
	}
	logMaxFile := os.Getenv("DAEMON_LOG_MAX_FILE")
	if logMaxFile == "" {
		logMaxFile = "3"
	}
	return &container.HostConfig{
		Resources:      buildResources(req),
		Mounts:         mounts,
		PortBindings:   portBindings,
		NetworkMode:    container.NetworkMode(req.NetworkName),
		UsernsMode:     container.UsernsMode(usernsMode),
		CapDrop:        []string{"ALL"},
		Privileged:     false,
		Init:           ptrBool(true),
		ReadonlyRootfs: true,
		SecurityOpt:    securityOpts,
		Tmpfs:          map[string]string{"/tmp": "rw,exec,size=64M"},
		LogConfig: container.LogConfig{
			Type: "json-file",
			Config: map[string]string{
				"max-size": logMaxSize,
				"max-file": logMaxFile,
			},
		},
	}
}

func dockerPorts(bindings []PortBinding) (nat.PortSet, nat.PortMap, error) {
	exposed := nat.PortSet{}
	published := nat.PortMap{}
	for _, binding := range bindings {
		protocol := strings.ToLower(strings.TrimSpace(binding.Protocol))
		if protocol == "" {
			protocol = "tcp"
		}
		if protocol != "tcp" && protocol != "udp" {
			return nil, nil, fmt.Errorf("unsupported port protocol %q", binding.Protocol)
		}
		containerPort := binding.ContainerPort
		if containerPort == 0 {
			containerPort = binding.HostPort
		}
		if binding.HostPort < 1 || binding.HostPort > 65535 || containerPort < 1 || containerPort > 65535 {
			return nil, nil, errors.New("host and container ports must be between 1 and 65535")
		}
		if binding.HostIP != "" && net.ParseIP(binding.HostIP) == nil {
			return nil, nil, fmt.Errorf("invalid allocation IP %q", binding.HostIP)
		}
		port := nat.Port(strconv.Itoa(containerPort) + "/" + protocol)
		exposed[port] = struct{}{}
		published[port] = append(published[port], nat.PortBinding{HostIP: binding.HostIP, HostPort: strconv.Itoa(binding.HostPort)})
	}
	return exposed, published, nil
}

const configHashLabel = "modern-game-panel.config_hash"

func validateCreateRequest(req CreateRequest) error {
	if strings.TrimSpace(req.ServerID) == "" || strings.TrimSpace(req.Image) == "" {
		return errors.New("server ID and image are required")
	}
	if req.MemoryMB < 0 || req.SwapMB < 0 {
		return errors.New("memory and swap must not be negative")
	}
	if req.SwapMB > 0 && req.MemoryMB == 0 {
		return errors.New("swap requires a positive memory limit")
	}
	if req.CPUShares != 0 && (req.CPUShares < 2 || req.CPUShares > 262144) {
		return errors.New("CPU shares must be 0 or between 2 and 262144")
	}
	if req.CPUPercent < 0 || req.CPUPercent > 100000 {
		return errors.New("CPU percentage must be between 0 and 100000")
	}
	if req.IOWeight != 0 && (req.IOWeight < 10 || req.IOWeight > 1000) {
		return errors.New("IO weight must be 0 or between 10 and 1000")
	}
	if req.PIDLimit < -1 {
		return errors.New("PID limit must be -1, 0, or positive")
	}
	if req.UID < 0 || req.GID < 0 {
		return errors.New("UID and GID must not be negative")
	}
	if (req.UID == 0) != (req.GID == 0) {
		return errors.New("UID and GID must either both be omitted or both be positive")
	}
	if req.StopTimeout < 0 || req.StopTimeout > 24*time.Hour {
		return errors.New("stop timeout must be between 0 and 24 hours")
	}
	if req.StopSignal != "" && !validStopSignal(req.StopSignal) {
		return fmt.Errorf("unsupported stop signal %q", req.StopSignal)
	}
	if strings.TrimSpace(req.NetworkName) == "" {
		return errors.New("an explicit Docker network name is required")
	}
	if (req.NetworkSubnet == "") != (req.NetworkGateway == "") {
		return errors.New("managed network subnet and gateway must be configured together")
	}
	if req.NetworkSubnet != "" {
		_, subnet, err := net.ParseCIDR(req.NetworkSubnet)
		if err != nil {
			return fmt.Errorf("invalid network subnet: %w", err)
		}
		gateway := net.ParseIP(req.NetworkGateway)
		if gateway == nil || !subnet.Contains(gateway) {
			return errors.New("network gateway must be inside the configured subnet")
		}
	}
	if req.NetworkIP != "" {
		ip := net.ParseIP(req.NetworkIP)
		if ip == nil {
			return errors.New("invalid container network IP")
		}
		if req.NetworkSubnet != "" {
			_, subnet, _ := net.ParseCIDR(req.NetworkSubnet)
			if !subnet.Contains(ip) {
				return errors.New("container network IP must be inside the configured subnet")
			}
		}
	}
	for _, dns := range req.DNS {
		if net.ParseIP(dns) == nil {
			return fmt.Errorf("invalid DNS server %q", dns)
		}
	}
	_, _, err := dockerPorts(req.Ports)
	return err
}

func validStopSignal(signal string) bool {
	switch strings.ToUpper(strings.TrimSpace(signal)) {
	case "SIGTERM", "SIGINT", "SIGQUIT", "SIGHUP", "SIGUSR1", "SIGUSR2", "SIGKILL":
		return true
	}
	return false
}

func buildContainerConfig(req CreateRequest, ports nat.PortSet, hash string) *container.Config {
	uid, gid := req.UID, req.GID
	if uid == 0 && gid == 0 {
		uid, gid = 998, 998
	}
	timeout := int(req.StopTimeout / time.Second)
	config := &container.Config{Image: req.Image, Cmd: req.Command, Env: req.Env, WorkingDir: serverContainerRoot, AttachStdin: true, AttachStdout: true, AttachStderr: true, OpenStdin: true, User: fmt.Sprintf("%d:%d", uid, gid), StopSignal: strings.ToUpper(req.StopSignal), Labels: map[string]string{"modern-game-panel.server_id": req.ServerID, configHashLabel: hash}, ExposedPorts: ports}
	if req.StopTimeout > 0 {
		config.StopTimeout = &timeout
	}
	return config
}

func createRequestHash(req CreateRequest) (string, error) {
	copyReq := req
	copyReq.RegistryAuth = nil
	body, err := json.Marshal(copyReq)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func imagePullOptions(auth *RegistryAuth) (image.PullOptions, error) {
	if auth == nil {
		return image.PullOptions{}, nil
	}
	body, err := json.Marshal(registry.AuthConfig{Username: auth.Username, Password: auth.Password, IdentityToken: auth.IdentityToken, RegistryToken: auth.RegistryToken, ServerAddress: auth.ServerAddress})
	if err != nil {
		return image.PullOptions{}, err
	}
	return image.PullOptions{RegistryAuth: base64.URLEncoding.EncodeToString(body)}, nil
}

func buildNetworkingConfig(req CreateRequest) *network.NetworkingConfig {
	settings := &network.EndpointSettings{}
	if req.NetworkIP != "" {
		settings.IPAMConfig = &network.EndpointIPAMConfig{IPv4Address: req.NetworkIP}
	}
	return &network.NetworkingConfig{EndpointsConfig: map[string]*network.EndpointSettings{req.NetworkName: settings}}
}

func (r *DockerRuntime) ensureExistingNetwork(ctx context.Context, name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("an explicit Docker network is required")
	}
	networks, err := r.client.NetworkList(ctx, network.ListOptions{Filters: filters.NewArgs(filters.Arg("name", "^"+name+"$"))})
	if err != nil {
		return err
	}
	if len(networks) == 0 {
		return fmt.Errorf("Docker network %q does not exist", name)
	}
	return nil
}

func (r *DockerRuntime) ensureNetwork(ctx context.Context, req CreateRequest) error {
	networks, err := r.client.NetworkList(ctx, network.ListOptions{Filters: filters.NewArgs(filters.Arg("name", "^"+req.NetworkName+"$"))})
	if err != nil {
		return err
	}
	if len(networks) > 0 {
		if req.NetworkIP != "" {
			contained := false
			for _, config := range networks[0].IPAM.Config {
				if _, subnet, parseErr := net.ParseCIDR(config.Subnet); parseErr == nil && subnet.Contains(net.ParseIP(req.NetworkIP)) {
					contained = true
					break
				}
			}
			if !contained {
				return fmt.Errorf("container network IP is outside network %q IPAM", req.NetworkName)
			}
		}
		if req.NetworkSubnet != "" {
			if networks[0].Labels["modern-game-panel.managed"] != "true" {
				return fmt.Errorf("network %q exists but is not managed by Beacon", req.NetworkName)
			}
			matched := false
			for _, config := range networks[0].IPAM.Config {
				if config.Subnet == req.NetworkSubnet && config.Gateway == req.NetworkGateway {
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Errorf("managed network %q IPAM does not match configured subnet/gateway", req.NetworkName)
			}
		}
		return nil
	}
	if req.NetworkSubnet == "" {
		return fmt.Errorf("Docker network %q does not exist and no managed subnet/gateway was configured", req.NetworkName)
	}
	_, err = r.client.NetworkCreate(ctx, req.NetworkName, network.CreateOptions{Driver: "bridge", IPAM: &network.IPAM{Config: []network.IPAMConfig{{Subnet: req.NetworkSubnet, Gateway: req.NetworkGateway}}}, Labels: map[string]string{"modern-game-panel.managed": "true"}})
	return err
}

func nonResourceConfigMatches(existing types.ContainerJSON, req CreateRequest) bool {
	if existing.Config == nil || existing.HostConfig == nil {
		return false
	}
	return existing.Config.Image == req.Image && strings.Join(existing.Config.Cmd, "\x00") == strings.Join(req.Command, "\x00") && strings.Join(existing.Config.Env, "\x00") == strings.Join(req.Env, "\x00") && string(existing.HostConfig.NetworkMode) == req.NetworkName
}

type dockerConsoleSession struct {
	response   types.HijackedResponse
	reader     io.Reader
	pipeReader *io.PipeReader
	closeOnce  sync.Once
}

func (s *dockerConsoleSession) Read(p []byte) (int, error) {
	return s.reader.Read(p)
}

func (s *dockerConsoleSession) Write(p []byte) (int, error) {
	return s.response.Conn.Write(p)
}

func (s *dockerConsoleSession) Close() error {
	s.closeOnce.Do(func() {
		s.response.Close()
		if s.pipeReader != nil {
			_ = s.pipeReader.Close()
		}
	})
	return nil
}
