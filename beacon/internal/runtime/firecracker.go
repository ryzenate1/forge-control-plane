//go:build firecracker
// +build firecracker

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
	events    chan ContainerEvent
}

const statusRunning = "running"

type firecrackerInstance struct {
	vmID       string
	machineID  string
	socketPath string
	createReq  CreateRequest
	pid        int
	createdAt  time.Time
	// startedAt is the zero time until InstanceStart is accepted. It must
	// never be backfilled from createdAt: a VM that was created but never
	// booted has no uptime, and reporting createdAt as start time would
	// fabricate it.
	startedAt  time.Time
	running    bool
	cmd        *exec.Cmd
	stdout     io.ReadCloser
	stderr     io.ReadCloser
	done       chan struct{}
}

type cappedLogBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *cappedLogBuffer) Write(payload []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	originalLength := len(payload)
	b.data = append(b.data, payload...)
	if len(b.data) > 1<<20 {
		b.data = append([]byte(nil), b.data[len(b.data)-(1<<20):]...)
	}
	return originalLength, nil
}

func (b *cappedLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(append([]byte(nil), b.data...))
}

func NewFirecrackerRuntime(cfg FirecrackerConfig) (*FirecrackerRuntime, error) {
	if cfg.SocketPath == "" {
		cfg.SocketPath = "/run/gamepanel/firecracker"
	}
	if cfg.FirecrackerBin == "" {
		cfg.FirecrackerBin = "firecracker"
	}
	if cfg.KernelImage == "" {
		cfg.KernelImage = "/var/lib/gamepanel/firecracker/kernel.bin"
	}
	if cfg.RootfsImage == "" {
		cfg.RootfsImage = "/var/lib/gamepanel/firecracker/rootfs.ext4"
	}
	if cfg.JailerPath == "" {
		cfg.JailerPath = "jailer"
	}
	if err := validateJailerPath(cfg.JailerPath); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(cfg.SocketPath, 0o700); err != nil {
		return nil, fmt.Errorf("create Firecracker socket directory: %w", err)
	}
	if err := os.Chmod(cfg.SocketPath, 0o700); err != nil {
		return nil, fmt.Errorf("secure Firecracker socket directory: %w", err)
	}

	return &FirecrackerRuntime{
		config:    cfg,
		instances: make(map[string]*firecrackerInstance),
		events:    make(chan ContainerEvent, 128),
	}, nil
}

// validateJailerPath confines the jailer executable to an allowlist so a
// compromised control plane cannot point Beacon at an arbitrary binary. The
// bare "jailer" resolved via PATH and absolute paths under the known install
// prefixes are accepted; anything with shell metacharacters, traversal, or an
// unexpected directory is rejected.
func validateJailerPath(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return errors.New("firecracker jailer path is required")
	}
	if strings.ContainsAny(value, "\x00\r\n;|&$`'\"*?~#(){}[]!\\") || strings.Contains(value, "..") {
		return fmt.Errorf("firecracker jailer path %q contains disallowed characters", raw)
	}
	if !strings.Contains(value, "/") {
		if value != filepath.Base(value) {
			return fmt.Errorf("firecracker jailer path %q is not a plain binary name", raw)
		}
		return nil
	}
	if !filepath.IsAbs(value) {
		return fmt.Errorf("firecracker jailer path %q must be \"jailer\" or an absolute path", raw)
	}
	cleaned := filepath.Clean(value)
	allowedPrefixes := []string{"/usr/bin/", "/usr/local/bin/", "/opt/firecracker/", "/var/lib/gamepanel/firecracker/"}
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(cleaned, prefix) && len(strings.TrimPrefix(cleaned, prefix)) > 0 && !strings.Contains(strings.TrimPrefix(cleaned, prefix), "/../") {
			return nil
		}
	}
	return fmt.Errorf("firecracker jailer path %q is outside the allowed prefixes", raw)
}

// firecrackerMachineConfig maps workload limits onto microVM resources or
// rejects the request when the limits cannot be honoured. CPUShares are a
// relative weight, not a vCPU count: only an explicit, bounded vCPU count is
// accepted here.
func firecrackerMachineConfig(req CreateRequest) (vcpuCount, memSizeMib int, err error) {
	vcpuCount = 1
	memSizeMib = 512
	if req.CPUPercent > 0 {
		// 100 percent == 1 vCPU, rounded up, capped at the microVM ceiling.
		vcpuCount = int((req.CPUPercent + 99) / 100)
	} else if req.CPUShares > 0 {
		// Shares cannot be translated to vCPUs without the host's total, so
		// only the unambiguous 1:1 legacy values are honoured.
		if req.CPUShares < 1 || req.CPUShares > 32 {
			return 0, 0, fmt.Errorf("firecracker cpus must map to 1-32 vCPUs; got cpuShares=%d (use cpuPercent instead)", req.CPUShares)
		}
		vcpuCount = int(req.CPUShares)
	}
	if req.MemoryMB > 0 {
		if req.MemoryMB < 128 || req.MemoryMB > 65536 {
			return 0, 0, fmt.Errorf("firecracker memory must be between 128MiB and 64GiB; got %dMiB", req.MemoryMB)
		}
		memSizeMib = int(req.MemoryMB)
	}
	if vcpuCount < 1 || vcpuCount > 32 {
		return 0, 0, fmt.Errorf("firecracker vcpu count must be between 1 and 32; got %d", vcpuCount)
	}
	if len(req.Mounts) > 0 {
		return 0, 0, errors.New("firecracker runtime does not support custom host mounts")
	}
	return vcpuCount, memSizeMib, nil
}

func (r *FirecrackerRuntime) Provider() string {
	return ProviderFirecracker
}

func (r *FirecrackerRuntime) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	instances := make([]*firecrackerInstance, 0, len(r.instances))
	for _, instance := range r.instances {
		instances = append(instances, instance)
	}
	r.mu.Unlock()
	var closeErrors []error
	for _, instance := range instances {
		if instance.cmd != nil && instance.cmd.Process != nil {
			if err := instance.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				closeErrors = append(closeErrors, err)
			}
		}
		if instance.stdout != nil {
			_ = instance.stdout.Close()
		}
		if instance.stderr != nil {
			_ = instance.stderr.Close()
		}
	}
	return errors.Join(closeErrors...)
}

func (r *FirecrackerRuntime) Ping(ctx context.Context) error {
	if r == nil {
		return errors.New("firecracker runtime is not initialized")
	}
	if _, err := exec.LookPath(r.config.FirecrackerBin); err != nil {
		return fmt.Errorf("firecracker binary %q not found: %w", r.config.FirecrackerBin, err)
	}
	if _, err := exec.LookPath(r.config.JailerPath); err != nil {
		return fmt.Errorf("firecracker jailer %q not found: %w", r.config.JailerPath, err)
	}
	return nil
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
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
	}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	defer client.CloseIdleConnections()

	return client.Do(req)
}

// fcDoChecked performs a Firecracker API call, drains and closes the response
// body, and rejects non-2xx statuses as errors. Every caller must use this
// instead of fcDo directly: ignoring the status would mark a rejected
// microVM as running, and ignoring the body would leak connections.
func (r *FirecrackerRuntime) fcDoChecked(ctx context.Context, method, socketPath, path string, body interface{}) ([]byte, error) {
	resp, err := r.fcDo(ctx, method, socketPath, path, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := strings.TrimSpace(string(data))
		if detail == "" {
			detail = resp.Status
		}
		if len(detail) > 512 {
			detail = detail[:512]
		}
		return nil, fmt.Errorf("firecracker %s %s rejected: %s: %s", method, path, resp.Status, detail)
	}
	return data, nil
}

func (r *FirecrackerRuntime) Create(ctx context.Context, req CreateRequest) error {
	if err := validateCreateRequest(req); err != nil {
		return err
	}
	// Fail closed on limits that cannot be mapped onto a microVM before
	// recording any instance state.
	if _, _, err := firecrackerMachineConfig(req); err != nil {
		return err
	}
	if strings.TrimSpace(req.RootDir) != "" {
		if _, err := validateRootDir(req.RootDir); err != nil {
			return err
		}
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
		createReq:  req,
		createdAt:  time.Now(),
	}
	r.instances[vmID] = inst
	r.mu.Unlock()

	return nil
}

func (r *FirecrackerRuntime) startFirecrackerProcess(ctx context.Context, vmID, socketPath string) error {
	args := []string{
		"--id", vmID,
		"--exec-file", r.config.FirecrackerBin,
		"--node", "0",
		"--chroot-base-dir", r.config.SocketPath,
	}
	cmd := exec.CommandContext(ctx, r.config.JailerPath, args...)

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
		inst.done = make(chan struct{})
		go r.reapInstance(inst)
	}
	r.mu.Unlock()

	return nil
}

func (r *FirecrackerRuntime) reapInstance(inst *firecrackerInstance) {
	_ = inst.cmd.Wait()
	exitCode := 0
	if inst.cmd.ProcessState != nil {
		exitCode = inst.cmd.ProcessState.ExitCode()
	}
	r.mu.Lock()
	inst.running = false
	close(inst.done)
	r.mu.Unlock()
	select {
	case r.events <- ContainerEvent{ServerID: inst.machineID, Action: "die", ExitCode: exitCode}:
	default:
	}
}

func (r *FirecrackerRuntime) configureMicroVM(ctx context.Context, socketPath string, req CreateRequest, vmID string) error {
	kernelArgs := "console=ttyS0 noapic reboot=k panic=1 pci=off nomodules"
	if r.config.CPUTemplate != "" {
		kernelArgs += " random.trust_cpu=on"
	}

	if _, err := r.fcDoChecked(ctx, "PUT", socketPath, "/boot-source", map[string]interface{}{
		"kernel_image_path": r.config.KernelImage,
		"boot_args":         kernelArgs,
	}); err != nil {
		return fmt.Errorf("set boot source: %w", err)
	}

	if _, err := r.fcDoChecked(ctx, "PUT", socketPath, "/drives/rootfs", map[string]interface{}{
		"drive_id":       "rootfs",
		"path_on_host":   r.config.RootfsImage,
		"is_root_device": true,
		"is_read_only":   false,
	}); err != nil {
		return fmt.Errorf("set rootfs: %w", err)
	}

	vcpuCount, memSizeMib, err := firecrackerMachineConfig(req)
	if err != nil {
		return err
	}

	machineConfig := map[string]interface{}{
		"vcpu_count":   vcpuCount,
		"mem_size_mib": memSizeMib,
	}
	if r.config.CPUTemplate != "" {
		machineConfig["cpu_template"] = r.config.CPUTemplate
	}

	if _, err := r.fcDoChecked(ctx, "PUT", socketPath, "/machine-config", machineConfig); err != nil {
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
		if err := waitForUnixSocket(ctx, socketPath); err != nil {
			return err
		}
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
			if _, err := r.fcDoChecked(ctx, "PUT", socketPath, "/mmds", mmdsData); err != nil {
				return fmt.Errorf("set mmds: %w", err)
			}
		}
	}

	if _, err := r.fcDoChecked(ctx, "PUT", socketPath, "/actions", map[string]string{
		"action_type": "InstanceStart",
	}); err != nil {
		return fmt.Errorf("start instance: %w", err)
	}

	r.mu.Lock()
	if inst, ok := r.instances[vmID]; ok {
		inst.running = true
		if inst.startedAt.IsZero() {
			inst.startedAt = time.Now()
		}
	}
	r.mu.Unlock()

	return nil
}

func (r *FirecrackerRuntime) getSocketPath(serverID string) string {
	return filepath.Join(r.config.SocketPath, containerName(serverID)+".sock")
}

func waitForUnixSocket(ctx context.Context, socketPath string) error {
	delay := 10 * time.Millisecond
	for {
		info, err := os.Stat(socketPath)
		if err == nil && info.Mode()&os.ModeSocket != 0 {
			return nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect Firecracker socket: %w", err)
		}
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("wait for Firecracker socket: %w", ctx.Err())
		}
		if delay < 250*time.Millisecond {
			delay *= 2
		}
	}
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
	var installLogs cappedLogBuffer
	var drain sync.WaitGroup
	drain.Add(2)
	go func() {
		defer drain.Done()
		_, _ = io.Copy(&installLogs, inst.stdout)
	}()
	go func() {
		defer drain.Done()
		_, _ = io.Copy(&installLogs, inst.stderr)
	}()
	if err := waitForUnixSocket(ctx, socketPath); err != nil {
		return InstallResult{}, err
	}

	if req.Image == "" {
		req.Image = "docker.io/library/alpine:3.21@sha256:21a3deaa0d32a8057914f36584b5288d2e5da9845c690f493846b7b90a70dbcd"
	}
	if req.Entrypoint == "" {
		req.Entrypoint = "sh"
	}

	kernelArgs := "console=ttyS0 noapic reboot=k panic=1 pci=off nomodules"
	if _, err := r.fcDoChecked(ctx, "PUT", socketPath, "/boot-source", map[string]interface{}{
		"kernel_image_path": r.config.KernelImage,
		"boot_args":         kernelArgs,
	}); err != nil {
		return InstallResult{}, fmt.Errorf("set boot source: %w", err)
	}

	if _, err := r.fcDoChecked(ctx, "PUT", socketPath, "/drives/rootfs", map[string]interface{}{
		"drive_id":       "rootfs",
		"path_on_host":   r.config.RootfsImage,
		"is_root_device": true,
		"is_read_only":   false,
	}); err != nil {
		return InstallResult{}, fmt.Errorf("set rootfs: %w", err)
	}

	if _, err := r.fcDoChecked(ctx, "PUT", socketPath, "/machine-config", map[string]interface{}{
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
	if _, err := r.fcDoChecked(ctx, "PUT", socketPath, "/mmds", mmdsData); err != nil {
		return InstallResult{}, fmt.Errorf("set mmds: %w", err)
	}

	if _, err := r.fcDoChecked(ctx, "PUT", socketPath, "/actions", map[string]string{
		"action_type": "InstanceStart",
	}); err != nil {
		return InstallResult{}, fmt.Errorf("start instance: %w", err)
	}

	r.mu.Lock()
	if inst, ok := r.instances[vmID]; ok {
		inst.running = true
		if inst.startedAt.IsZero() {
			inst.startedAt = time.Now()
		}
	}
	r.mu.Unlock()

	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	select {
	case <-inst.done:
	case <-waitCtx.Done():
		_ = r.killInstance(vmID)
		return InstallResult{}, waitCtx.Err()
	}
	drain.Wait()
	logs := installLogs.String()

	// Report the installer's real exit code. Unknown is not zero: a missing
	// ProcessState means the outcome was not observed and must surface as an
	// error rather than a fabricated success.
	exitCode := -1
	r.mu.Lock()
	if inst.cmd != nil && inst.cmd.ProcessState != nil {
		exitCode = inst.cmd.ProcessState.ExitCode()
	}
	delete(r.instances, vmID)
	r.mu.Unlock()
	if exitCode < 0 {
		return InstallResult{ExitCode: exitCode, Logs: logs}, errors.New("firecracker installer exit code is unknown")
	}

	return InstallResult{ExitCode: exitCode, Logs: logs}, nil
}

func (r *FirecrackerRuntime) Inspect(ctx context.Context, serverID string) (ContainerState, error) {
	vmID := containerName(serverID)
	inst, ok := r.getInstance(serverID)
	if !ok {
		return ContainerState{ServerID: serverID, Exists: false}, nil
	}

	running := inst.running
	alive := false
	if inst.cmd != nil && inst.cmd.Process != nil {
		if err := inst.cmd.Process.Signal(unix.Signal(0)); err == nil {
			alive = true
		}
	}
	// The reaped flag is authoritative: a process that exited but has not
	// been reaped yet must not be reported as running.
	running = running && alive
	status := "created"
	if running {
		status = statusRunning
	} else if inst.cmd != nil {
		status = "stopped"
	}

	return ContainerState{
		ServerID:  serverID,
		ID:        vmID,
		Exists:    true,
		Running:   running,
		Status:    status,
		StartedAt: inst.startedAt,
	}, nil
}

func (r *FirecrackerRuntime) List(ctx context.Context) ([]ContainerState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	states := make([]ContainerState, 0, len(r.instances))
	for _, inst := range r.instances {
		alive := false
		if inst.cmd != nil && inst.cmd.Process != nil {
			if err := inst.cmd.Process.Signal(unix.Signal(0)); err == nil {
				alive = true
			}
		}
		running := inst.running && alive
		status := "created"
		if running {
			status = statusRunning
		} else if inst.cmd != nil {
			status = "stopped"
		}
		states = append(states, ContainerState{
			ServerID:  inst.machineID,
			ID:        inst.vmID,
			Exists:    true,
			Running:   running,
			Status:    status,
			StartedAt: inst.startedAt,
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

	// Booting a microVM requires the jailer process, the boot source / rootfs /
	// machine config and any MMDS payload to be applied before InstanceStart is
	// accepted, so reuse ensureInstanceRunning with the request recorded at
	// Create time instead of firing InstanceStart at an unconfigured VM.
	return r.ensureInstanceRunning(ctx, vmID, inst.socketPath, inst.createReq)
}

func (r *FirecrackerRuntime) SendCommand(ctx context.Context, serverID, command string) error {
	return errors.New("send command not supported for firecracker runtime")
}

func (r *FirecrackerRuntime) Stop(ctx context.Context, serverID string) error {
	inst, ok := r.getInstance(serverID)
	if !ok {
		return fmt.Errorf("workload %q does not exist", serverID)
	}
	if inst.cmd == nil || inst.cmd.Process == nil || inst.done == nil {
		r.mu.Lock()
		if inst, ok := r.instances[containerName(serverID)]; ok {
			inst.running = false
		}
		r.mu.Unlock()
		return nil
	}

	_ = inst.cmd.Process.Signal(unix.SIGTERM)

	select {
	case <-inst.done:
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
		return fmt.Errorf("workload %q does not exist", serverID)
	}

	if inst.cmd == nil || inst.cmd.Process == nil {
		return nil
	}

	waitCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	select {
	case <-inst.done:
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
		case <-inst.done:
			return nil
		case <-time.After(10 * time.Second):
			return inst.cmd.Process.Kill()
		}
	}
}

func (r *FirecrackerRuntime) Kill(ctx context.Context, serverID string) error {
	inst, ok := r.getInstance(serverID)
	if !ok {
		return fmt.Errorf("workload %q does not exist", serverID)
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
	signals := map[string]unix.Signal{
		"SIGTERM": unix.SIGTERM,
		"SIGKILL": unix.SIGKILL,
		"SIGINT":  unix.SIGINT,
		"SIGHUP":  unix.SIGHUP,
		"SIGUSR1": unix.SIGUSR1,
		"SIGUSR2": unix.SIGUSR2,
	}
	sig, ok := signals[signal]
	if !ok {
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
	if !inst.running {
		return Stats{}, fmt.Errorf("instance %s is not running: no metrics to report", serverID)
	}

	configData, err := r.fcDoChecked(ctx, "GET", inst.socketPath, "/vm/config", nil)
	if err != nil {
		return Stats{}, fmt.Errorf("get vm config: %w", err)
	}

	var vmConfig struct {
		VcpuCount  int `json:"vcpu_count"`
		MemSizeMib int `json:"mem_size_mib"`
	}
	if err := json.Unmarshal(configData, &vmConfig); err != nil {
		return Stats{}, fmt.Errorf("decode vm config: %w", err)
	}

	// Usage is only reported when the metrics endpoint answers. A limit-only
	// reading with zero usage would look like an idle VM; unknown usage must
	// surface as an error instead.
	metricsData, err := r.fcDoChecked(ctx, "GET", inst.socketPath, "/metrics", nil)
	if err != nil {
		return Stats{}, fmt.Errorf("get firecracker metrics: %w", err)
	}
	var metrics struct {
		MemoryUsageMB   float64 `json:"memory_usage_mb"`
		CPUUsagePercent float64 `json:"cpu_usage_percent"`
	}
	if err := json.Unmarshal(metricsData, &metrics); err != nil {
		return Stats{}, fmt.Errorf("decode firecracker metrics: %w", err)
	}

	return Stats{
		MemoryLimit: uint64(vmConfig.MemSizeMib) * 1024 * 1024,
		MemoryBytes: uint64(metrics.MemoryUsageMB) * 1024 * 1024,
		CPUPercent:  metrics.CPUUsagePercent,
	}, nil
}

func (r *FirecrackerRuntime) Logs(ctx context.Context, serverID string) (io.ReadCloser, error) {
	inst, ok := r.getInstance(serverID)
	if !ok {
		return nil, fmt.Errorf("instance %s not found", serverID)
	}
	// A VM that was created but never booted has no output. An empty
	// stream would read as "healthy but quiet", so refuse it instead.
	if inst.startedAt.IsZero() {
		return nil, fmt.Errorf("instance %s has never started: no logs to report", serverID)
	}

	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		var copies sync.WaitGroup
		for _, source := range []io.Reader{inst.stdout, inst.stderr} {
			if source == nil {
				continue
			}
			copies.Add(1)
			go func(source io.Reader) {
				defer copies.Done()
				_, _ = io.Copy(writer, source)
			}(source)
		}
		copies.Wait()
	}()
	go func() {
		<-ctx.Done()
		_ = reader.CloseWithError(ctx.Err())
	}()

	return reader, nil
}

func (r *FirecrackerRuntime) LogsStream(ctx context.Context, serverID string, tail string) (io.ReadCloser, error) {
	inst, ok := r.getInstance(serverID)
	if !ok {
		return nil, fmt.Errorf("instance %s not found", serverID)
	}
	if inst.startedAt.IsZero() {
		return nil, fmt.Errorf("instance %s has never started: no logs to report", serverID)
	}

	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		var copies sync.WaitGroup
		for _, source := range []io.Reader{inst.stdout, inst.stderr} {
			if source == nil {
				continue
			}
			copies.Add(1)
			go func(source io.Reader) {
				defer copies.Done()
				_, _ = io.Copy(writer, source)
			}(source)
		}
		copies.Wait()
	}()
	go func() {
		<-ctx.Done()
		_ = reader.CloseWithError(ctx.Err())
	}()

	return reader, nil
}

func (r *FirecrackerRuntime) StatsStream(ctx context.Context, serverID string) (io.ReadCloser, error) {
	if _, ok := r.getInstance(serverID); !ok {
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
				stats, err := r.Stats(ctx, serverID)
				if err != nil {
					// Propagate the failure instead of emitting a zero frame:
					// a silent EOF reads as "no load" rather than "no metrics".
					_ = writer.CloseWithError(err)
					return
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
	// Firecracker has no Docker-style attach stream. Console access requires a
	// vsock guest agent, which this runtime does not provision; returning a
	// pipe that silently drops input would be a dishonest console.
	return nil, errors.New("interactive console attachment is not supported by the firecracker runtime: configure vsock access")
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
		// Delete is idempotent: removing a workload that does not exist is
		// a no-op (matching Docker Delete on NotFound). Stop/Kill/Wait
		// above still return does-not-exist so an ambiguous target is
		// never reported as stopped.
		return nil
	}

	if inst.cmd != nil && inst.cmd.Process != nil {
		_ = inst.cmd.Process.Kill()
		if inst.done != nil {
			select {
			case <-inst.done:
			case <-time.After(10 * time.Second):
			}
		}
	}

	os.Remove(inst.socketPath)
	delete(r.instances, vmID)
	return nil
}

func (r *FirecrackerRuntime) WatchEvents(ctx context.Context) (<-chan ContainerEvent, <-chan error) {
	out := make(chan ContainerEvent, 128)
	errs := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errs)
		for {
			select {
			case event := <-r.events:
				select {
				case out <- event:
				case <-ctx.Done():
					return
				default:
					select {
					case errs <- errors.New("Firecracker event dropped because the consumer is not keeping up"):
					default:
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, errs
}

type firecrackerConsoleSession struct {
	reader    *io.PipeReader
	writer    *io.PipeWriter
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
