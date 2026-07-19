//go:build ignore
// +build ignore

package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

type FirecrackerRuntime struct {
	config    FirecrackerConfig
	mu        sync.Mutex
	instances map[string]*firecrackerInstance
	fcClient  *http.Client
}

type firecrackerInstance struct {
	vmID      string
	machineID string
	socketPath string
	pid       int
	createdAt time.Time
	running   bool
	cmd       *exec.Cmd
	stdout    io.ReadCloser
	stderr    io.ReadCloser
}

func NewFirecrackerRuntime(cfg FirecrackerConfig) (*FirecrackerRuntime, error) {
	if cfg.SocketPath == "" {
		cfg.SocketPath = "/tmp/firecracker"
	}
	if cfg.FirecrackerBin == "" {
		cfg.FirecrackerBin = "firecracker"
	}
	if cfg.KernelImage == "" {
		cfg.KernelImage = "/var/lib/forge/kernel/hello-vmlinux.bin"
	}
	if cfg.RootfsImage == "" {
		cfg.RootfsImage = "/var/lib/forge/rootfs/rootfs.ext4"
	}

	_ = os.MkdirAll(cfg.SocketPath, 0755)

	return &FirecrackerRuntime{
		config:    cfg,
		instances: make(map[string]*firecrackerInstance),
		fcClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return net.Dial("unix", addr)
				},
			},
			Timeout: 30 * time.Second,
		},
	}, nil
}

func (r *FirecrackerRuntime) Provider() string {
	return ProviderFirecracker
}

func (r *FirecrackerRuntime) Ping(ctx context.Context) error {
	if r == nil {
		return errors.New("firecracker runtime is not initialized")
	}
	if _, err := exec.LookPath(r.config.FirecrackerBin); err != nil {
		return fmt.Errorf("firecracker binary %q not found: %w", r.config.FirecrackerBin, err)
	}
	return nil
}

func (r *FirecrackerRuntime) firecrackerURL(vmID, path string) string {
	r.mu.Lock()
	inst, ok := r.instances[vmID]
	r.mu.Unlock()
	if !ok {
		return "http://localhost" + path
	}
	return "http://localhost" + path
}

func (r *FirecrackerRuntime) fcDo(ctx context.Context, method, socketPath, path string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, "http://localhost"+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	defer client.CloseIdleConnections()

	return client.Do(req)
}

func (r *FirecrackerRuntime) Create(ctx context.Context, req CreateRequest) error {
	if err := validateCreateRequest(req); err != nil {
		return err
	}

	vmID := containerName(req.ServerID)
	socketPath := filepath.Join(r.config.SocketPath, vmID+".sock")

	r.mu.Lock()
	if _, exists := r.instances[vmID]; exists {
		r.mu.Unlock()
		return nil
	}

	inst := &firecrackerInstance{
		vmID:       vmID,
		machineID:  req.ServerID,
		socketPath: socketPath,
		createdAt:  time.Now(),
	}
	r.instances[vmID] = inst
	r.mu.Unlock()

	return nil
}

func (r *FirecrackerRuntime) startFirecrackerProcess(ctx context.Context, vmID, socketPath string) error {
	jailerBin := r.config.JailerPath
	var cmd *exec.Cmd

	if jailerBin != "" {
		args := []string{
			"--id", vmID,
			"--exec-file", r.config.FirecrackerBin,
			"--node", "0",
			"--chroot-base-dir", r.config.SocketPath,
		}
		cmd = exec.CommandContext(ctx, jailerBin, args...)
	} else {
		cmd = exec.CommandContext(ctx, r.config.FirecrackerBin,
			"--api-sock", socketPath,
			"--id", vmID,
		)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start firecracker: %w", err)
	}

	r.mu.Lock()
	if inst, ok := r.instances[vmID]; ok {
		inst.cmd = cmd
		inst.pid = cmd.Process.Pid
		inst.stdout = stdout
		inst.stderr = stderr
	}
	r.mu.Unlock()

	return nil
}

func (r *FirecrackerRuntime) configureMicroVM(ctx context.Context, socketPath string, req CreateRequest, vmID string) error {
	kernelArgs := "console=ttyS0 noapic reboot=k panic=1 pci=off nomodules"
	if r.config.CPUTemplate != "" {
		kernelArgs += " random.trust_cpu=on"
	}

	if _, err := r.fcDo(ctx, "PUT", socketPath, "/boot-source", map[string]interface{}{
		"kernel_image_path": r.config.KernelImage,
		"boot_args":         kernelArgs,
	}); err != nil {
		return fmt.Errorf("set boot source: %w", err)
	}

	if _, err := r.fcDo(ctx, "PUT", socketPath, "/drives/rootfs", map[string]interface{}{
		"drive_id":      "rootfs",
		"path_on_host":  r.config.RootfsImage,
		"is_root_device": true,
		"is_read_only":  false,
	}); err != nil {
		return fmt.Errorf("set rootfs: %w", err)
	}

	vcpuCount := 1
	memSizeMib := 512
	if req.CPUShares > 0 {
		vcpuCount = int(req.CPUShares)
	}
	if req.MemoryMB > 0 {
		memSizeMib = int(req.MemoryMB)
	}

	machineConfig := map[string]interface{}{
		"vcpu_count":   vcpuCount,
		"mem_size_mib": memSizeMib,
	}
	if r.config.CPUTemplate != "" {
		machineConfig["cpu_template"] = r.config.CPUTemplate
	}

	if _, err := r.fcDo(ctx, "PUT", socketPath, "/machine-config", machineConfig); err != nil {
		return fmt.Errorf("set machine config: %w", err)
	}

	return nil
}

func (r *FirecrackerRuntime) ensureInstanceRunning(ctx context.Context, vmID, socketPath string, req CreateRequest) error {
	r.mu.Lock()
	inst, exists := r.instances[vmID]
	r.mu.Unlock()

	if !exists {
		return fmt.Errorf("instance %s not found", vmID)
	}

	if inst.running {
		return nil
	}

	if inst.cmd == nil {
		if err := r.startFirecrackerProcess(ctx, vmID, socketPath); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
	}

	if err := r.configureMicroVM(ctx, socketPath, req, vmID); err != nil {
		return err
	}

	if req.Env != nil {
		mmdsData := make(map[string]string)
		for _, env := range req.Env {
			if parts := strings.SplitN(env, "=", 2); len(parts) == 2 {
				mmdsData[parts[0]] = parts[1]
			}
		}
		if len(mmdsData) > 0 {
			if _, err := r.fcDo(ctx, "PUT", socketPath, "/mmds", mmdsData); err != nil {
				return fmt.Errorf("set mmds: %w", err)
			}
		}
	}

	if _, err := r.fcDo(ctx, "PUT", socketPath, "/actions", map[string]string{
		"action_type": "InstanceStart",
	}); err != nil {
		return fmt.Errorf("start instance: %w", err)
	}

	r.mu.Lock()
	if inst, ok := r.instances[vmID]; ok {
		inst.running = true
	}
	r.mu.Unlock()

	return nil
}

func (r *FirecrackerRuntime) getSocketPath(serverID string) string {
	return filepath.Join(r.config.SocketPath, containerName(serverID)+".sock")
}

func (r *FirecrackerRuntime) getInstance(serverID string) (*firecrackerInstance, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	inst, ok := r.instances[containerName(serverID)]
	return inst, ok
}

func (r *FirecrackerRuntime) Install(ctx context.Context, req InstallRequest) (InstallResult, error) {
	rootDir, err := validateRootDir(req.RootDir)
	if err != nil {
		return InstallResult{}, err
	}

	vmID := containerName(req.ServerID) + "-installer"
	socketPath := filepath.Join(r.config.SocketPath, vmID+".sock")

	r.mu.Lock()
	inst := &firecrackerInstance{
		vmID:       vmID,
		machineID:  req.ServerID,
		socketPath: socketPath,
		createdAt:  time.Now(),
	}
	r.instances[vmID] = inst
	r.mu.Unlock()

	if err := r.startFirecrackerProcess(ctx, vmID, socketPath); err != nil {
		return InstallResult{}, err
	}
	time.Sleep(500 * time.Millisecond)

	if req.Image == "" {
		req.Image = "alpine:3.21"
	}
	if req.Entrypoint == "" {
		req.Entrypoint = "sh"
	}

	kernelArgs := "console=ttyS0 noapic reboot=k panic=1 pci=off nomodules"
	if _, err := r.fcDo(ctx, "PUT", socketPath, "/boot-source", map[string]interface{}{
		"kernel_image_path": r.config.KernelImage,
		"boot_args":         kernelArgs,
	}); err != nil {
		return InstallResult{}, fmt.Errorf("set boot source: %w", err)
	}

	if _, err := r.fcDo(ctx, "PUT", socketPath, "/drives/rootfs", map[string]interface{}{
		"drive_id":       "rootfs",
		"path_on_host":   r.config.RootfsImage,
		"is_root_device": true,
		"is_read_only":   false,
	}); err != nil {
		return InstallResult{}, fmt.Errorf("set rootfs: %w", err)
	}

	if _, err := r.fcDo(ctx, "PUT", socketPath, "/machine-config", map[string]interface{}{
		"vcpu_count":   1,
		"mem_size_mib": 512,
	}); err != nil {
		return InstallResult{}, fmt.Errorf("set machine config: %w", err)
	}

	scriptMount := map[string]string{
		"source": rootDir,
		"target": "/mnt/server",
	}
	mmdsData := map[string]interface{}{
		"script":   req.Script,
		"mounts":   []interface{}{scriptMount},
		"env":      req.Env,
		"root_dir": rootDir,
	}
	if _, err := r.fcDo(ctx, "PUT", socketPath, "/mmds", mmdsData); err != nil {
		return InstallResult{}, fmt.Errorf("set mmds: %w", err)
	}

	if _, err := r.fcDo(ctx, "PUT", socketPath, "/actions", map[string]string{
		"action_type": "InstanceStart",
	}); err != nil {
		return InstallResult{}, fmt.Errorf("start instance: %w", err)
	}

	r.mu.Lock()
	if inst, ok := r.instances[vmID]; ok {
		inst.running = true
	}
	r.mu.Unlock()

	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	done := make(chan struct{})
	go func() {
		_ = inst.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-waitCtx.Done():
		_ = r.killInstance(vmID)
		return InstallResult{}, waitCtx.Err()
	}

	logs := ""
	if inst.stdout != nil {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, inst.stdout)
		logs = buf.String()
	}

	r.mu.Lock()
	delete(r.instances, vmID)
	r.mu.Unlock()

	return InstallResult{ExitCode: 0, Logs: logs}, nil
}

func (r *FirecrackerRuntime) Inspect(ctx context.Context, serverID string) (ContainerState, error) {
	vmID := containerName(serverID)
	inst, ok := r.getInstance(serverID)
	if !ok {
		return ContainerState{ServerID: serverID, Exists: false}, nil
	}

	running := false
	if inst.cmd != nil && inst.cmd.Process != nil {
		if err := inst.cmd.Process.Signal(unix.Signal(0)); err == nil {
			running = true
		}
	}

	return ContainerState{
		ServerID: serverID,
		ID:       vmID,
		Exists:   true,
		Running:  running,
		Status:   statusRunning,
	}, nil
}

func (r *FirecrackerRuntime) List(ctx context.Context) ([]ContainerState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	states := make([]ContainerState, 0, len(r.instances))
	for _, inst := range r.instances {
		running := false
		if inst.cmd != nil && inst.cmd.Process != nil {
			if err := inst.cmd.Process.Signal(unix.Signal(0)); err == nil {
				running = true
			}
		}
		states = append(states, ContainerState{
			ServerID: inst.machineID,
			ID:       inst.vmID,
			Exists:   true,
			Running:  running,
			Status:   statusRunning,
		})
	}
	return states, nil
}

func (r *FirecrackerRuntime) Start(ctx context.Context, serverID string) error {
	vmID := containerName(serverID)
	inst, ok := r.getInstance(serverID)
	if !ok {
		return fmt.Errorf("instance %s not found: create it first", serverID)
	}

	socketPath := inst.socketPath
	if inst.cmd == nil {
		if err := r.startFirecrackerProcess(ctx, vmID, socketPath); err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
	}

	if _, err := r.fcDo(ctx, "PUT", socketPath, "/actions", map[string]string{
		"action_type": "InstanceStart",
	}); err != nil {
		return fmt.Errorf("start instance: %w", err)
	}

	r.mu.Lock()
	if inst, ok := r.instances[vmID]; ok {
		inst.running = true
	}
	r.mu.Unlock()

	return nil
}

func (r *FirecrackerRuntime) SendCommand(ctx context.Context, serverID, command string) error {
	return errors.New("send command not supported for firecracker runtime")
}

func (r *FirecrackerRuntime) Stop(ctx context.Context, serverID string) error {
	inst, ok := r.getInstance(serverID)
	if !ok {
		return nil
	}

	_ = inst.cmd.Process.Signal(unix.SIGTERM)

	done := make(chan struct{})
	go func() {
		_ = inst.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		_ = inst.cmd.Process.Kill()
	case <-ctx.Done():
		return ctx.Err()
	}

	r.mu.Lock()
	if inst, ok := r.instances[containerName(serverID)]; ok {
		inst.running = false
	}
	r.mu.Unlock()

	return nil
}

func (r *FirecrackerRuntime) WaitForStop(ctx context.Context, serverID string, duration time.Duration, terminate bool) error {
	if duration <= 0 {
		duration = 30 * time.Second
	}

	inst, ok := r.getInstance(serverID)
	if !ok {
		return nil
	}

	if inst.cmd == nil || inst.cmd.Process == nil {
		return nil
	}

	waitCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	done := make(chan struct{})
	go func() {
		_ = inst.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-waitCtx.Done():
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !terminate {
			return context.DeadlineExceeded
		}
		_ = inst.cmd.Process.Signal(unix.SIGTERM)
		select {
		case <-done:
			return nil
		case <-time.After(10 * time.Second):
			return inst.cmd.Process.Kill()
		}
	}
}

func (r *FirecrackerRuntime) Kill(ctx context.Context, serverID string) error {
	inst, ok := r.getInstance(serverID)
	if !ok {
		return nil
	}

	if inst.cmd != nil && inst.cmd.Process != nil {
		return inst.cmd.Process.Kill()
	}
	return nil
}

func (r *FirecrackerRuntime) Signal(ctx context.Context, serverID, signal string) error {
	inst, ok := r.getInstance(serverID)
	if !ok {
		return fmt.Errorf("instance %s not found", serverID)
	}

	signal = strings.ToUpper(strings.TrimSpace(signal))
	sig, err := unix.SignalNumber(signal)
	if err != nil || sig == 0 {
		return fmt.Errorf("unsupported signal %q", signal)
	}

	if inst.cmd != nil && inst.cmd.Process != nil {
		return inst.cmd.Process.Signal(sig)
	}
	return fmt.Errorf("instance %s has no running process", serverID)
}

func (r *FirecrackerRuntime) Restart(ctx context.Context, serverID string) error {
	if err := r.Stop(ctx, serverID); err != nil {
		return err
	}
	return r.Start(ctx, serverID)
}

func (r *FirecrackerRuntime) Stats(ctx context.Context, serverID string) (Stats, error) {
	inst, ok := r.getInstance(serverID)
	if !ok {
		return Stats{}, fmt.Errorf("instance %s not found", serverID)
	}

	resp, err := r.fcDo(ctx, "GET", inst.socketPath, "/vm/config", nil)
	if err != nil {
		return Stats{}, fmt.Errorf("get vm config: %w", err)
	}
	defer resp.Body.Close()

	var vmConfig struct {
		VcpuCount int    `json:"vcpu_count"`
		MemSizeMib int   `json:"mem_size_mib"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&vmConfig); err != nil {
		return Stats{}, fmt.Errorf("decode vm config: %w", err)
	}

	stats := Stats{
		MemoryLimit: uint64(vmConfig.MemSizeMib) * 1024 * 1024,
	}

	metricsResp, err := r.fcDo(ctx, "GET", inst.socketPath, "/metrics", nil)
	if err == nil {
		defer metricsResp.Body.Close()
		var metricsData struct {
			MemoryUsageMB float64 `json:"memory_usage_mb"`
			CPUUsagePercent float64 `json:"cpu_usage_percent"`
		}
		if err := json.NewDecoder(metricsResp.Body).Decode(&metricsData); err == nil {
			stats.MemoryBytes = uint64(metricsData.MemoryUsageMB) * 1024 * 1024
			stats.CPUPercent = metricsData.CPUUsagePercent
		}
	}

	return stats, nil
}

func (r *FirecrackerRuntime) Logs(ctx context.Context, serverID string) (io.ReadCloser, error) {
	inst, ok := r.getInstance(serverID)
	if !ok {
		return nil, fmt.Errorf("instance %s not found", serverID)
	}

	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		if inst.stdout != nil {
			_, _ = io.Copy(writer, inst.stdout)
		}
		if inst.stderr != nil {
			_, _ = io.Copy(writer, inst.stderr)
		}
	}()

	return reader, nil
}

func (r *FirecrackerRuntime) LogsStream(ctx context.Context, serverID string, tail string) (io.ReadCloser, error) {
	inst, ok := r.getInstance(serverID)
	if !ok {
		return nil, fmt.Errorf("instance %s not found", serverID)
	}

	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		buf := make([]byte, 4096)
		for {
			if inst.stdout != nil {
				n, err := inst.stdout.Read(buf)
				if n > 0 {
					_, _ = writer.Write(buf[:n])
				}
				if err != nil {
					return
				}
			}
			if inst.stderr != nil {
				n, err := inst.stderr.Read(buf)
				if n > 0 {
					_, _ = writer.Write(buf[:n])
				}
				if err != nil {
					return
				}
			}
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()

	return reader, nil
}

func (r *FirecrackerRuntime) StatsStream(ctx context.Context, serverID string) (io.ReadCloser, error) {
	inst, ok := r.getInstance(serverID)
	if !ok {
		return nil, fmt.Errorf("instance %s not found", serverID)
	}

	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				resp, err := r.fcDo(ctx, "GET", inst.socketPath, "/vm/config", nil)
				if err != nil {
					continue
				}
				var vmConfig struct {
					VcpuCount  int `json:"vcpu_count"`
					MemSizeMib int `json:"mem_size_mib"`
				}
				_ = json.NewDecoder(resp.Body).Decode(&vmConfig)
				resp.Body.Close()

				stats := Stats{
					MemoryLimit: uint64(vmConfig.MemSizeMib) * 1024 * 1024,
				}

				metricsResp, err := r.fcDo(ctx, "GET", inst.socketPath, "/metrics", nil)
				if err == nil {
					var metricsData struct {
						MemoryUsageMB    float64 `json:"memory_usage_mb"`
						CPUUsagePercent  float64 `json:"cpu_usage_percent"`
					}
					_ = json.NewDecoder(metricsResp.Body).Decode(&metricsData)
					metricsResp.Body.Close()
					stats.MemoryBytes = uint64(metricsData.MemoryUsageMB) * 1024 * 1024
					stats.CPUPercent = metricsData.CPUUsagePercent
				}

				body, _ := json.Marshal(stats)
				_, _ = writer.Write(body)
				_, _ = writer.Write([]byte("\n"))
			case <-ctx.Done():
				return
			}
		}
	}()

	return reader, nil
}

func (r *FirecrackerRuntime) AttachConsole(ctx context.Context, serverID string) (ConsoleSession, error) {
	inst, ok := r.getInstance(serverID)
	if !ok {
		return nil, fmt.Errorf("instance %s not found", serverID)
	}

	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		buf := make([]byte, 4096)
		for {
			if inst.stdout != nil {
				n, err := inst.stdout.Read(buf)
				if n > 0 {
					_, _ = writer.Write(buf[:n])
				}
				if err != nil {
					return
				}
			}
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()

	return &firecrackerConsoleSession{
		reader: reader,
		writer: writer,
	}, nil
}

func (r *FirecrackerRuntime) Delete(ctx context.Context, serverID string) error {
	vmID := containerName(serverID)
	return r.killInstance(vmID)
}

func (r *FirecrackerRuntime) killInstance(vmID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	inst, ok := r.instances[vmID]
	if !ok {
		return nil
	}

	if inst.cmd != nil && inst.cmd.Process != nil {
		_ = inst.cmd.Process.Kill()
		_ = inst.cmd.Wait()
	}

	os.Remove(inst.socketPath)
	delete(r.instances, vmID)
	return nil
}

func (r *FirecrackerRuntime) WatchEvents(ctx context.Context) (<-chan ContainerEvent, <-chan error) {
	out := make(chan ContainerEvent)
	errs := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errs)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r.mu.Lock()
				for vmID, inst := range r.instances {
					if inst.cmd == nil || inst.cmd.Process == nil {
						continue
					}
					if err := inst.cmd.Process.Signal(unix.Signal(0)); err != nil {
						inst.running = false
						exitCode := 0
						if inst.cmd.ProcessState != nil {
							exitCode = inst.cmd.ProcessState.ExitCode()
						}
						select {
						case out <- ContainerEvent{
							ServerID: inst.machineID,
							Action:   "die",
							ExitCode: exitCode,
						}:
						case <-ctx.Done():
							r.mu.Unlock()
							return
						}
						delete(r.instances, vmID)
					}
				}
				r.mu.Unlock()
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, errs
}

type firecrackerConsoleSession struct {
	reader *io.PipeReader
	writer *io.PipeWriter
	closeOnce sync.Once
}

func (s *firecrackerConsoleSession) Read(p []byte) (int, error) {
	return s.reader.Read(p)
}

func (s *firecrackerConsoleSession) Write(p []byte) (int, error) {
	return s.writer.Write(p)
}

func (s *firecrackerConsoleSession) Close() error {
	s.closeOnce.Do(func() {
		_ = s.reader.Close()
		_ = s.writer.Close()
	})
	return nil
}
