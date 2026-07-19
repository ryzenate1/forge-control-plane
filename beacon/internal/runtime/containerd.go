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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	containerdclient "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"golang.org/x/sys/unix"
)

type ContainerdRuntime struct {
	client    *containerdclient.Client
	namespace string
	mu        sync.Mutex
	taskIOs   map[string]*containerdTaskIO
}

type containerdTaskIO struct {
	task    containerdclient.Task
	stdout  *bytes.Buffer
	stderr  *bytes.Buffer
	console cio.IO
}

func NewContainerdRuntime(cfg ContainerdConfig) (*ContainerdRuntime, error) {
	if cfg.Address == "" {
		cfg.Address = "/run/containerd/containerd.sock"
	}
	if cfg.Namespace == "" {
		cfg.Namespace = "default"
	}
	client, err := containerdclient.New(cfg.Address, containerdclient.WithDefaultNamespace(cfg.Namespace))
	if err != nil {
		return nil, fmt.Errorf("connect to containerd: %w", err)
	}
	return &ContainerdRuntime{
		client:    client,
		namespace: cfg.Namespace,
		taskIOs:   make(map[string]*containerdTaskIO),
	}, nil
}

func (r *ContainerdRuntime) Provider() string {
	return ProviderContainerd
}

func (r *ContainerdRuntime) Ping(ctx context.Context) error {
	if r == nil || r.client == nil {
		return errors.New("containerd runtime is not initialized")
	}
	serving, err := r.client.IsServing(ctx)
	if err != nil {
		return err
	}
	if !serving {
		return errors.New("containerd is not serving")
	}
	return nil
}

func (r *ContainerdRuntime) Create(ctx context.Context, req CreateRequest) error {
	if err := validateCreateRequest(req); err != nil {
		return err
	}
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	image, err := r.client.Pull(ctx, req.Image, containerdclient.WithPullUnpack)
	if err != nil {
		return fmt.Errorf("pull image %q: %w", req.Image, err)
	}

	name := containerName(req.ServerID)
	labels := map[string]string{
		"modern-game-panel.server_id": req.ServerID,
		configHashLabel:               "",
	}

	specOpts := []oci.SpecOpts{
		oci.WithImageConfig(image),
		oci.WithEnv(req.Env),
	}

	if len(req.Command) > 0 {
		specOpts = append(specOpts, oci.WithProcessArgs(req.Command...))
	}

	if req.UID > 0 || req.GID > 0 {
		uid := req.UID
		gid := req.GID
		if uid == 0 && gid == 0 {
			uid = 998
			gid = 998
		}
		specOpts = append(specOpts, oci.WithUserID(uint32(uid)), oci.WithAdditionalGIDs(uint32(gid)))
	}

	specOpts = append(specOpts, oci.WithMounts(containerdMounts(req.Mounts)...))

	container, err := r.client.NewContainer(ctx, name,
		containerdclient.WithImage(image),
		containerdclient.WithNewSnapshot(name, image),
		containerdclient.WithNewSpec(specOpts...),
		containerdclient.WithContainerLabels(labels),
	)
	if err != nil {
		return fmt.Errorf("create container: %w", err)
	}
	_ = container
	return nil
}

func (r *ContainerdRuntime) Install(ctx context.Context, req InstallRequest) (InstallResult, error) {
	rootDir, err := validateRootDir(req.RootDir)
	if err != nil {
		return InstallResult{}, err
	}
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	if req.Image == "" {
		req.Image = "alpine:3.21"
	}
	if req.Entrypoint == "" {
		req.Entrypoint = "sh"
	}

	image, err := r.client.Pull(ctx, req.Image, containerdclient.WithPullUnpack)
	if err != nil {
		return InstallResult{}, fmt.Errorf("pull image: %w", err)
	}

	name := containerName(req.ServerID) + "-installer"
	labels := map[string]string{
		"modern-game-panel.server_id": req.ServerID,
		"modern-game-panel.job":       "install",
	}

	mounts := []specs.Mount{
		{Type: "bind", Source: rootDir, Destination: "/mnt/server", Options: []string{"rbind", "rw"}},
	}

	container, err := r.client.NewContainer(ctx, name,
		containerdclient.WithImage(image),
		containerdclient.WithNewSnapshot(name, image),
		containerdclient.WithNewSpec(
			oci.WithImageConfig(image),
			oci.WithProcessArgs(req.Entrypoint, "-lc", req.Script),
			oci.WithEnv(req.Env),
			oci.WithMounts(mounts),
		),
		containerdclient.WithContainerLabels(labels),
	)
	if err != nil {
		return InstallResult{}, fmt.Errorf("create installer container: %w", err)
	}
	defer r.client.ContainerService().Delete(ctx, name)

	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		return InstallResult{}, fmt.Errorf("create task: %w", err)
	}
	defer task.Delete(ctx)

	if err := task.Start(ctx); err != nil {
		return InstallResult{}, fmt.Errorf("start task: %w", err)
	}

	exitStatusC, err := task.Wait(ctx)
	if err != nil {
		return InstallResult{}, fmt.Errorf("wait task: %w", err)
	}

	var statusCode int64
	select {
	case exitStatus := <-exitStatusC:
		statusCode = exitStatus.ExitCode()
	case <-ctx.Done():
		_ = task.Kill(ctx, unix.SIGKILL)
		return InstallResult{}, ctx.Err()
	}

	logsReader, err := r.Logs(ctx, req.ServerID+"-installer")
	if err != nil {
		return InstallResult{ExitCode: int(statusCode)}, nil
	}
	defer logsReader.Close()
	var raw bytes.Buffer
	_, _ = io.Copy(&raw, io.LimitReader(logsReader, 1024*1024))

	return InstallResult{ExitCode: int(statusCode), Logs: raw.String()}, nil
}

func (r *ContainerdRuntime) Inspect(ctx context.Context, serverID string) (ContainerState, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)
	name := containerName(serverID)

	container, err := r.client.LoadContainer(ctx, name)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return ContainerState{ServerID: serverID, Exists: false}, nil
		}
		return ContainerState{}, err
	}

	info, err := container.Info(ctx)
	if err != nil {
		return ContainerState{}, err
	}

	state := ContainerState{
		ServerID: serverID,
		ID:       info.ID,
		Exists:   true,
	}

	task, err := container.Task(ctx, nil)
	if err == nil {
		taskStatus, err := task.Status(ctx)
		if err == nil {
			state.Running = taskStatus.Status == containerdclient.Running
			state.Status = string(taskStatus.Status)
		}
	} else {
		state.Status = "created"
	}

	return state, nil
}

func (r *ContainerdRuntime) List(ctx context.Context) ([]ContainerState, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	containerList, err := r.client.Containers(ctx)
	if err != nil {
		return nil, err
	}

	states := make([]ContainerState, 0, len(containerList))
	for _, c := range containerList {
		info, err := c.Info(ctx)
		if err != nil {
			continue
		}
		serverID := info.Labels["modern-game-panel.server_id"]
		if serverID == "" {
			continue
		}

		state := ContainerState{
			ServerID: serverID,
			ID:       info.ID,
			Exists:   true,
		}

		task, err := c.Task(ctx, nil)
		if err == nil {
			taskStatus, err := task.Status(ctx)
			if err == nil {
				state.Running = taskStatus.Status == containerdclient.Running
				state.Status = string(taskStatus.Status)
			}
		} else {
			state.Status = "created"
		}

		states = append(states, state)
	}
	return states, nil
}

func (r *ContainerdRuntime) Start(ctx context.Context, serverID string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)
	name := containerName(serverID)

	container, err := r.client.LoadContainer(ctx, name)
	if err != nil {
		return fmt.Errorf("load container: %w", err)
	}

	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			task, err = container.Task(ctx, nil)
			if err != nil {
				return fmt.Errorf("get existing task: %w", err)
			}
			return task.Start(ctx)
		}
		return fmt.Errorf("create task: %w", err)
	}

	r.mu.Lock()
	r.taskIOs[serverID] = &containerdTaskIO{task: task}
	r.mu.Unlock()

	return task.Start(ctx)
}

func (r *ContainerdRuntime) SendCommand(ctx context.Context, serverID, command string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)
	name := containerName(serverID)

	container, err := r.client.LoadContainer(ctx, name)
	if err != nil {
		return fmt.Errorf("load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	spec, err := container.Spec(ctx)
	if err != nil {
		return fmt.Errorf("get spec: %w", err)
	}

	args := []string{"/bin/sh", "-c", command}
	if spec.Process != nil && len(spec.Process.Args) > 0 {
		shell := spec.Process.Args[0]
		if strings.Contains(shell, "bash") {
			args = []string{"/bin/bash", "-c", command}
		}
	}

	process, err := task.Exec(ctx, fmt.Sprintf("exec-%d", time.Now().UnixNano()), &specs.Process{
		Args: args,
		Cwd:  "/",
	}, cio.NewCreator(cio.WithStdio))
	if err != nil {
		return fmt.Errorf("exec: %w", err)
	}
	defer process.Delete(ctx)

	exitStatusC, err := process.Wait(ctx)
	if err != nil {
		return err
	}

	if err := process.Start(ctx); err != nil {
		return err
	}

	select {
	case <-exitStatusC:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *ContainerdRuntime) Stop(ctx context.Context, serverID string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)
	name := containerName(serverID)

	container, err := r.client.LoadContainer(ctx, name)
	if err != nil {
		return fmt.Errorf("load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return nil
	}

	timeout := 30
	signal, _ := unix.SignalNumber("SIGTERM")
	_ = task.Kill(ctx, signal, containerdclient.WithKillAll)

	done := make(chan struct{})
	go func() {
		waitCtx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()
		_, _ = task.Wait(waitCtx)
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(time.Duration(timeout) * time.Second):
		sigkill, _ := unix.SignalNumber("SIGKILL")
		return task.Kill(ctx, sigkill, containerdclient.WithKillAll)
	}
}

func (r *ContainerdRuntime) WaitForStop(ctx context.Context, serverID string, duration time.Duration, terminate bool) error {
	if duration <= 0 {
		duration = 30 * time.Second
	}
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, containerName(serverID))
	if err != nil {
		return nil
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return nil
	}

	waitCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	exitStatusC, err := task.Wait(waitCtx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			if !terminate {
				return context.DeadlineExceeded
			}
			stopTimeout := 10
			sigterm, _ := unix.SignalNumber("SIGTERM")
			_ = task.Kill(ctx, sigterm, containerdclient.WithKillAll)
			select {
			case <-exitStatusC:
				return nil
			case <-time.After(time.Duration(stopTimeout) * time.Second):
				sigkill, _ := unix.SignalNumber("SIGKILL")
				return task.Kill(ctx, sigkill, containerdclient.WithKillAll)
			}
		}
		return err
	}

	select {
	case <-exitStatusC:
		return nil
	case <-waitCtx.Done():
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !terminate {
			return context.DeadlineExceeded
		}
		stopTimeout := 10
		sigterm, _ := unix.SignalNumber("SIGTERM")
		_ = task.Kill(ctx, sigterm, containerdclient.WithKillAll)
		select {
		case <-exitStatusC:
			return nil
		case <-time.After(time.Duration(stopTimeout) * time.Second):
			sigkill, _ := unix.SignalNumber("SIGKILL")
			return task.Kill(ctx, sigkill, containerdclient.WithKillAll)
		}
	}
}

func (r *ContainerdRuntime) Kill(ctx context.Context, serverID string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)
	name := containerName(serverID)

	container, err := r.client.LoadContainer(ctx, name)
	if err != nil {
		return fmt.Errorf("load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return nil
	}

	sigkill, _ := unix.SignalNumber("SIGKILL")
	return task.Kill(ctx, sigkill, containerdclient.WithKillAll)
}

func (r *ContainerdRuntime) Signal(ctx context.Context, serverID, signal string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)
	name := containerName(serverID)

	signal = strings.ToUpper(strings.TrimSpace(signal))
	if !validStopSignal(signal) {
		return fmt.Errorf("unsupported stop signal %q", signal)
	}

	container, err := r.client.LoadContainer(ctx, name)
	if err != nil {
		return fmt.Errorf("load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return fmt.Errorf("get task: %w", err)
	}

	sig, err := signalToUnix(signal)
	if err != nil {
		return err
	}

	return task.Kill(ctx, sig, containerdclient.WithKillAll)
}

func (r *ContainerdRuntime) Restart(ctx context.Context, serverID string) error {
	if err := r.Stop(ctx, serverID); err != nil {
		return err
	}
	return r.Start(ctx, serverID)
}

func (r *ContainerdRuntime) Stats(ctx context.Context, serverID string) (Stats, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)
	name := containerName(serverID)

	container, err := r.client.LoadContainer(ctx, name)
	if err != nil {
		return Stats{}, fmt.Errorf("load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return Stats{}, fmt.Errorf("get task: %w", err)
	}

	metric, err := task.Metrics(ctx)
	if err != nil {
		return Stats{}, fmt.Errorf("get metrics: %w", err)
	}

	var cpuPercent float64
	var memBytes, memLimit uint64

	data := metric.Data
	if len(data) > 0 {
		var metricsData struct {
			CPUUsage    uint64 `json:"cpu_usage"`
			SystemUsage uint64 `json:"system_cpu"`
			MemoryUsage uint64 `json:"memory_usage"`
			MemoryLimit uint64 `json:"memory_limit"`
		}
		if err := json.Unmarshal(data, &metricsData); err == nil {
			memBytes = metricsData.MemoryUsage
			memLimit = metricsData.MemoryLimit
			if metricsData.SystemUsage > 0 {
				cpuPercent = float64(metricsData.CPUUsage) / float64(metricsData.SystemUsage) * 100
			}
		}
	}

	return Stats{
		CPUPercent:  cpuPercent,
		MemoryBytes: memBytes,
		MemoryLimit: memLimit,
	}, nil
}

func (r *ContainerdRuntime) Logs(ctx context.Context, serverID string) (io.ReadCloser, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)
	name := containerName(serverID)

	container, err := r.client.LoadContainer(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}

	ioObj, err := task.IO(ctx)
	if err != nil {
		return nil, fmt.Errorf("get task IO: %w", err)
	}

	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		if ioObj.Stdout != nil {
			_, _ = io.Copy(writer, ioObj.Stdout)
		}
		if ioObj.Stderr != nil {
			_, _ = io.Copy(writer, ioObj.Stderr)
		}
	}()

	return reader, nil
}

func (r *ContainerdRuntime) LogsStream(ctx context.Context, serverID string, tail string) (io.ReadCloser, error) {
	return r.Logs(ctx, serverID)
}

func (r *ContainerdRuntime) StatsStream(ctx context.Context, serverID string) (io.ReadCloser, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)
	name := containerName(serverID)

	container, err := r.client.LoadContainer(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}

	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				metric, err := task.Metrics(ctx)
				if err != nil {
					return
				}
				data := metric.Data
				if len(data) > 0 {
					var metricsData struct {
						CPUUsage    uint64 `json:"cpu_usage"`
						SystemUsage uint64 `json:"system_cpu"`
						MemoryUsage uint64 `json:"memory_usage"`
						MemoryLimit uint64 `json:"memory_limit"`
					}
					if err := json.Unmarshal(data, &metricsData); err == nil {
						cpuPercent := float64(0)
						if metricsData.SystemUsage > 0 {
							cpuPercent = float64(metricsData.CPUUsage) / float64(metricsData.SystemUsage) * 100
						}
						payload := Stats{
							CPUPercent:  cpuPercent,
							MemoryBytes: metricsData.MemoryUsage,
							MemoryLimit: metricsData.MemoryLimit,
						}
						body, _ := json.Marshal(payload)
						_, _ = writer.Write(body)
						_, _ = writer.Write([]byte("\n"))
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return reader, nil
}

func (r *ContainerdRuntime) AttachConsole(ctx context.Context, serverID string) (ConsoleSession, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)
	name := containerName(serverID)

	container, err := r.client.LoadContainer(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("load container: %w", err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}

	ioObj, err := task.IO(ctx)
	if err != nil {
		return nil, fmt.Errorf("get task IO: %w", err)
	}

	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		if ioObj.Stdout != nil {
			_, _ = io.Copy(writer, ioObj.Stdout)
		}
	}()

	return &containerdConsoleSession{
		reader: reader,
		writer: writer,
		stdin:  ioObj.Stdin,
	}, nil
}

func (r *ContainerdRuntime) Delete(ctx context.Context, serverID string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)
	name := containerName(serverID)

	container, err := r.client.LoadContainer(ctx, name)
	if err != nil {
		return nil
	}

	task, err := container.Task(ctx, nil)
	if err == nil {
		sigkill, _ := unix.SignalNumber("SIGKILL")
		_ = task.Kill(ctx, sigkill, containerdclient.WithKillAll)
		_, _ = task.Wait(ctx)
		_ = task.Delete(ctx)
	}

	r.mu.Lock()
	delete(r.taskIOs, serverID)
	r.mu.Unlock()

	return container.Delete(ctx, containerdclient.WithSnapshotCleanup)
}

func (r *ContainerdRuntime) WatchEvents(ctx context.Context) (<-chan ContainerEvent, <-chan error) {
	out := make(chan ContainerEvent)
	errs := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errs)
		backoff := 100 * time.Millisecond
		for ctx.Err() == nil {
			filter := fmt.Sprintf(`labels."modern-game-panel.server_id"~=.*`)
			eventCh, errCh := r.client.EventService().Subscribe(ctx, filter)

			connected := true
			for connected {
				select {
				case event := <-eventCh:
					if event == nil {
						connected = false
						continue
					}
					backoff = 100 * time.Millisecond
					eventPayload, err := json.Marshal(event.Event)
					if err != nil {
						continue
					}
					var eventData struct {
						ID     string `json:"id"`
						Status string `json:"status"`
						Exit   struct {
							Code    int  `json:"code"`
							OOMKill bool `json:"oom_kill"`
						} `json:"exit"`
					}
					if err := json.Unmarshal(eventPayload, &eventData); err != nil {
						continue
					}

					container, loadErr := r.client.LoadContainer(ctx, eventData.ID)
					if loadErr != nil {
						continue
					}
					info, infoErr := container.Info(ctx)
					if infoErr != nil {
						continue
					}
					serverID := info.Labels["modern-game-panel.server_id"]
					if serverID == "" {
						continue
					}

					select {
					case out <- ContainerEvent{
						ServerID:  serverID,
						Action:    strings.ToLower(event.Topic),
						ExitCode:  eventData.Exit.Code,
						OOMKilled: eventData.Exit.OOMKill,
					}:
					case <-ctx.Done():
						return
					}
				case err, ok := <-errCh:
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

type containerdConsoleSession struct {
	reader *io.PipeReader
	writer *io.PipeWriter
	stdin  io.Reader
	closeOnce sync.Once
}

func (s *containerdConsoleSession) Read(p []byte) (int, error) {
	return s.reader.Read(p)
}

func (s *containerdConsoleSession) Write(p []byte) (int, error) {
	if w, ok := s.stdin.(io.Writer); ok {
		return w.Write(p)
	}
	return 0, errors.New("stdin not writable")
}

func (s *containerdConsoleSession) Close() error {
	s.closeOnce.Do(func() {
		_ = s.reader.Close()
		_ = s.writer.Close()
	})
	return nil
}

func containerdMounts(custom []Mount) []specs.Mount {
	mounts := []specs.Mount{}
	for _, m := range custom {
		if m.Source == "" || m.Target == "" {
			continue
		}
		options := []string{"rbind", "rw"}
		if m.ReadOnly {
			options = []string{"rbind", "ro"}
		}
		mounts = append(mounts, specs.Mount{
			Type:        "bind",
			Source:      m.Source,
			Destination: m.Target,
			Options:     options,
		})
	}
	return mounts
}

func signalToUnix(signal string) (unix.Signal, error) {
	signal = strings.ToUpper(strings.TrimSpace(signal))
	sig, err := unix.SignalNumber(signal)
	if err != nil || sig == 0 {
		return 0, fmt.Errorf("invalid signal %q", signal)
	}
	return sig, nil
}
