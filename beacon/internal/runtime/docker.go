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
	"log"
	"net"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
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

// Per-call bounds for daemon round-trips. The Docker SDK client carries no
// default timeout, so a wedged or unreachable daemon blocks the caller
// indefinitely: a lifecycle request would never return, and the workload would
// keep running while the panel waited. Streams (logs, stats, console) are
// deliberately not bounded this way — they are long-lived by design and rely on
// the caller's context instead.
const (
	engineInspectTimeout = 15 * time.Second
	engineCallTimeout    = 60 * time.Second
	engineStopTimeout    = 2 * time.Minute
)

// withEngineDeadline bounds an engine call without shortening a deadline the
// caller already set. The returned cancel is always safe to defer.
func withEngineDeadline(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= timeout {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

var pinnedImagePattern = regexp.MustCompile(`@sha256:[a-fA-F0-9]{64}$`)

type DockerRuntime struct {
	client              *client.Client
	defaultNetwork      string
	hostSettings        dockerHostSettings
	allowUnpinnedImages bool
	workloadLocks       [64]sync.Mutex
}

type dockerHostSettings struct {
	rootless   bool
	logMaxSize string
	logMaxFile string
}

func NewDockerRuntime() (*DockerRuntime, error) {
	if err := validateDockerEndpoint(os.Getenv("DOCKER_HOST")); err != nil {
		return nil, err
	}
	// Pin the minimum API used by this runtime. Negotiation lets a compromised
	// endpoint influence the client surface and makes behavior daemon-version
	// dependent.
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithVersion("1.43"))
	if err != nil {
		return nil, err
	}
	networkName := strings.TrimSpace(os.Getenv("DAEMON_DOCKER_NETWORK"))
	if networkName == "" {
		networkName = "gamepanel"
	}
	return &DockerRuntime{
		client:              cli,
		defaultNetwork:      networkName,
		hostSettings:        loadDockerHostSettings(),
		allowUnpinnedImages: strings.TrimSpace(os.Getenv("DAEMON_ALLOW_UNPINNED_IMAGES")) == "true",
	}, nil
}

// ValidateDockerEndpoint reports whether raw is an acceptable Docker engine
// endpoint. Only the local platform default, Unix sockets, named pipes, and
// the least-privilege socket proxy are accepted; arbitrary TCP daemons would
// expose an unauthenticated Docker API and allow endpoint redirection.
func ValidateDockerEndpoint(raw string) error { return validateDockerEndpoint(raw) }

func validateDockerEndpoint(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil // Docker's local platform default (Unix socket or named pipe).
	}
	if strings.HasPrefix(raw, "unix://") || strings.HasPrefix(raw, "npipe://") {
		return nil
	}
	// Container deployments reach the least-privilege Docker socket proxy over
	// their private Compose network. Do not accept arbitrary TCP daemons: that
	// would expose an unauthenticated Docker API and allow endpoint redirection.
	if raw == "tcp://docker-proxy:2375" || raw == "tcp://traefik-docker-proxy:2375" {
		return nil
	}
	return fmt.Errorf("remote Docker endpoint %q is forbidden; use a local Unix socket or named pipe", raw)
}

func (r *DockerRuntime) Provider() string { return ProviderDocker }

func (r *DockerRuntime) Close() error {
	if r == nil || r.client == nil {
		return nil
	}
	return r.client.Close()
}

func (r *DockerRuntime) Ping(ctx context.Context) error {
	if r == nil || r.client == nil {
		return errors.New("docker runtime is not initialized")
	}
	_, err := r.client.Ping(ctx)
	return err
}

func (r *DockerRuntime) ensureImage(ctx context.Context, imageRef string, registryAuth *RegistryAuth) error {
	if strings.TrimSpace(imageRef) == "" {
		return errors.New("container image is required")
	}
	if inspect, _, err := r.client.ImageInspectWithRaw(ctx, imageRef); err == nil {
		// Even on a cache hit the reference must be digest-pinned unless the
		// operator explicitly opts into mutable tags. A cached tag pull is
		// still a mutable reference: without this check an attacker who can
		// influence the tag (or a stale cache entry) bypasses immutability.
		// Pinned references are additionally verified against the cached
		// image's RepoDigests so a stale cache entry for a different digest
		// cannot satisfy the request.
		if pinnedImagePattern.MatchString(imageRef) {
			want := imageRef[strings.LastIndex(imageRef, "@")+1:]
			for _, repoDigest := range inspect.RepoDigests {
				if strings.HasSuffix(repoDigest, "@"+want) || strings.HasSuffix(repoDigest, want) {
					return nil
				}
			}
			// Pinned but the local cache does not carry the requested digest:
			// fall through to pull the exact digest below.
		} else if r.allowUnpinnedImages {
			return nil
		} else if len(inspect.RepoDigests) > 0 {
			// Cached image that was originally resolved to a registry digest.
			// Still reject: the requested reference itself is mutable, so a
			// future pull could resolve differently. Operators who need
			// mutable tags must set DAEMON_ALLOW_UNPINNED_IMAGES=true.
			return fmt.Errorf("remote image %q is not digest-pinned; use name@sha256:<64 hex characters>", imageRef)
		} else {
			// Locally built images carry no RepoDigests because no remote
			// bytes were introduced; they are accepted without a pin.
			return nil
		}
	} else if !errdefs.IsNotFound(err) {
		return fmt.Errorf("inspect image %q: %w", imageRef, err)
	}
	if !r.allowUnpinnedImages && !pinnedImagePattern.MatchString(imageRef) {
		return fmt.Errorf("remote image %q is not digest-pinned; use name@sha256:<64 hex characters>", imageRef)
	}
	pullOptions, err := imagePullOptions(registryAuth)
	if err != nil {
		return err
	}
	pullCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	pull, err := r.client.ImagePull(pullCtx, imageRef, pullOptions)
	if err != nil {
		return fmt.Errorf("pull image %q: %w", imageRef, err)
	}
	_, copyErr := io.Copy(io.Discard, pull)
	closeErr := pull.Close()
	if copyErr != nil {
		return fmt.Errorf("read image pull response: %w", copyErr)
	}
	if closeErr != nil {
		return closeErr
	}
	if _, _, err := r.client.ImageInspectWithRaw(pullCtx, imageRef); err != nil {
		return fmt.Errorf("verify pulled image %q: %w", imageRef, err)
	}
	return nil
}

func (r *DockerRuntime) Create(ctx context.Context, req CreateRequest) error {
	lock := r.workloadLock(req.ServerID)
	lock.Lock()
	defer lock.Unlock()
	existing, err := r.Inspect(ctx, req.ServerID)
	if err != nil {
		return err
	}
	if existing.Exists {
		// Idempotent by existence: creating a workload that already exists
		// is a no-op. The panel re-provisions on boot to repair missing
		// workloads without disturbing running containers. Configuration
		// drift is deliberately not compared here — Reconcile compares the
		// configHashLabel and recreates on mismatch, while Create never
		// disturbs a live container.
		return nil
	}
	return r.reconcile(ctx, req)
}

// Reconcile applies req to a new or existing Docker workload. Because bind
// mounts cannot be changed in place, an existing workload is safely stopped,
// recreated from the complete desired request, and restarted if it was running.
func (r *DockerRuntime) Reconcile(ctx context.Context, req CreateRequest) error {
	lock := r.workloadLock(req.ServerID)
	lock.Lock()
	defer lock.Unlock()
	return r.reconcile(ctx, req)
}

func (r *DockerRuntime) reconcile(ctx context.Context, req CreateRequest) error {
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
	restartAfterCreate := true
	if existing, inspectErr := r.client.ContainerInspect(ctx, name); inspectErr == nil {
		if existing.Config != nil && existing.Config.Labels[configHashLabel] == hash {
			return nil
		}
		restartAfterCreate = existing.State != nil && existing.State.Running
		if restartAfterCreate {
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

	if err := r.ensureImage(ctx, req.Image, req.RegistryAuth); err != nil {
		return err
	}

	config := buildContainerConfig(req, exposedPorts, hash)
	createAndStart := func(ioWeight int64) (bool, error) {
		reqCopy := req
		reqCopy.IOWeight = ioWeight
		created, err := r.client.ContainerCreate(ctx, config, buildHostConfigWithSettings(reqCopy, mounts, portBindings, r.hostSettings), buildNetworkingConfig(reqCopy), nil, name)
		if err != nil {
			return false, err
		}
		if !restartAfterCreate {
			return true, nil
		}
		if err := r.client.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			_ = r.client.ContainerRemove(cleanupCtx, created.ID, container.RemoveOptions{Force: true, RemoveVolumes: true})
			return false, err
		}
		return true, nil
	}
	_, err = createAndStart(req.IOWeight)
	if err != nil && req.IOWeight > 0 && strings.Contains(err.Error(), "io.weight") {
		// Some hosts (e.g. Docker Desktop on macOS) lack the io controller
		// cgroup; retry once without the best-effort I/O weight limit. The
		// fallback is logged so a silently-dropped limit cannot be mistaken
		// for an enforced one.
		log.Printf("[runtime] io.weight unsupported by host, retrying workload %q without it: %v", req.ServerID, err)
		if _, err = createAndStart(0); err != nil {
			return err
		}
	}
	return err
}

func (r *DockerRuntime) workloadLock(serverID string) *sync.Mutex {
	sum := sha256.Sum256([]byte(serverID))
	return &r.workloadLocks[int(sum[0])%len(r.workloadLocks)]
}

func (r *DockerRuntime) Install(ctx context.Context, req InstallRequest) (InstallResult, error) {
	rootDir, err := validateRootDir(req.RootDir)
	if err != nil {
		return InstallResult{}, err
	}
	if req.Image == "" {
		req.Image = "docker.io/library/alpine:3.21@sha256:21a3deaa0d32a8057914f36584b5288d2e5da9845c690f493846b7b90a70dbcd"
	}
	if req.Entrypoint == "" {
		req.Entrypoint = "sh"
	}
	if err := validateInstallEnv(req.Env); err != nil {
		return InstallResult{}, err
	}
	// Bound the install so a hung script cannot hold the worker forever.
	installCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	ctx = installCtx
	if err := r.ensureExistingNetwork(ctx, r.defaultNetwork); err != nil {
		return InstallResult{}, err
	}
	if err := r.ensureImage(ctx, req.Image, nil); err != nil {
		return InstallResult{}, err
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
			// Installers run as an unprivileged user. Anything that
			// legitimately needs root must do so through the image's own
			// entrypoint, not through Beacon granting it.
			User: "65534:65534",
			Labels: map[string]string{
				"modern-game-panel.server_id": req.ServerID,
				"modern-game-panel.job":       "install",
			},
		},
		&container.HostConfig{
			Mounts: []mount.Mount{
				{Type: mount.TypeBind, Source: rootDir, Target: "/mnt/server", ReadOnly: false, BindOptions: &mount.BindOptions{CreateMountpoint: false}},
			},
			NetworkMode: container.NetworkMode(r.defaultNetwork),
			Resources: container.Resources{
				Memory:     512 * 1024 * 1024,
				MemorySwap: 512 * 1024 * 1024,
				PidsLimit:  ptrInt64(256),
			},
			CapDrop:        []string{"ALL"},
			Privileged:     false,
			Init:           ptrBool(true),
			ReadonlyRootfs: true,
			SecurityOpt:    []string{"no-new-privileges:true"},
			Tmpfs: map[string]string{
				"/tmp": "rw,noexec,nosuid,nodev,size=64M",
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
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = r.client.ContainerRemove(cleanupCtx, resp.ID, container.RemoveOptions{Force: true, RemoveVolumes: true})
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
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		_ = r.client.ContainerRemove(cleanupCtx, resp.ID, container.RemoveOptions{Force: true, RemoveVolumes: true})
		return InstallResult{}, ctx.Err()
	}
	logsReader, err := r.client.ContainerLogs(ctx, resp.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true})
	if err != nil {
		return InstallResult{}, err
	}
	defer logsReader.Close()
	// The installer is not a TTY container, so the daemon returns stdout and
	// stderr as framed records. Copying that stream verbatim handed the panel
	// eight-byte binary headers interleaved with the script output, and the
	// install transcript published to the user was unparseable. Demux it, and
	// only cap the retained size: a transcript that stops mid-way must say so
	// rather than look complete.
	capped := &cappedBuffer{limit: installLogLimit}
	if _, copyErr := stdcopy.StdCopy(capped, capped, logsReader); copyErr != nil {
		return InstallResult{ExitCode: int(statusCode)}, fmt.Errorf("read installer logs: %w", copyErr)
	}
	logs := capped.String()
	if capped.dropped > 0 {
		logs += fmt.Sprintf("\n[install log truncated: %d bytes omitted]", capped.dropped)
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	_ = r.client.ContainerRemove(cleanupCtx, resp.ID, container.RemoveOptions{Force: true, RemoveVolumes: true})
	return InstallResult{ExitCode: int(statusCode), Logs: logs}, nil
}

// installLogLimit bounds how much of an installer's transcript Beacon retains.
// The reader keeps draining past the limit so the engine sees a complete
// stream; only the retained copy stops growing, and the shortfall is reported
// in the transcript itself.
const installLogLimit = 1 << 20

type cappedBuffer struct {
	buf    bytes.Buffer
	limit  int64
	dropped int64
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	room := c.limit - int64(c.buf.Len())
	switch {
	case room <= 0:
		c.dropped += int64(len(p))
	case int64(len(p)) <= room:
		c.buf.Write(p)
	default:
		c.buf.Write(p[:room])
		c.dropped += int64(len(p)) - room
	}
	return len(p), nil
}

func (c *cappedBuffer) String() string { return c.buf.String() }

func (r *DockerRuntime) Inspect(ctx context.Context, serverID string) (ContainerState, error) {
	ctx, cancel := withEngineDeadline(ctx, engineInspectTimeout)
	defer cancel()
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
		state.StartedAt, _ = time.Parse(time.RFC3339Nano, inspection.State.StartedAt)
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
		// ContainerList only carries creation time. The real start time
		// comes from an inspect; fall back to Created when the inspect
		// fails so a list failure never hides the whole fleet.
		startedAt := time.Unix(item.Created, 0)
		if inspection, inspectErr := r.client.ContainerInspect(ctx, item.ID); inspectErr == nil && inspection.State != nil {
			if parsed, parseErr := time.Parse(time.RFC3339Nano, inspection.State.StartedAt); parseErr == nil && !parsed.IsZero() {
				startedAt = parsed
			}
		}
		states = append(states, ContainerState{
			ServerID:  serverID,
			ID:        item.ID,
			Exists:    true,
			Running:   strings.EqualFold(item.State, "running"),
			Status:    item.Status,
			StartedAt: startedAt,
		})
	}
	return states, nil
}

func (r *DockerRuntime) Start(ctx context.Context, serverID string) error {
	lock := r.workloadLock(serverID)
	lock.Lock()
	defer lock.Unlock()
	state, err := r.Inspect(ctx, serverID)
	if err != nil {
		return err
	}
	if !state.Exists {
		return fmt.Errorf("workload %q does not exist", serverID)
	}
	if state.Running {
		return nil
	}
	callCtx, cancel := withEngineDeadline(ctx, engineCallTimeout)
	defer cancel()
	return r.client.ContainerStart(callCtx, containerName(serverID), container.StartOptions{})
}

func (r *DockerRuntime) SendCommand(ctx context.Context, serverID, command string) error {
	state, err := r.Inspect(ctx, serverID)
	if err != nil {
		return err
	}
	if !state.Exists || !state.Running {
		return fmt.Errorf("workload %q is not running", serverID)
	}
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	attached, err := r.client.ContainerAttach(commandCtx, containerName(serverID), container.AttachOptions{
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
	_ = attached.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	written, err := io.WriteString(attached.Conn, command)
	if err == nil && written != len(command) {
		err = io.ErrShortWrite
	}
	return err
}

func (r *DockerRuntime) Stop(ctx context.Context, serverID string) error {
	lock := r.workloadLock(serverID)
	lock.Lock()
	defer lock.Unlock()
	state, err := r.Inspect(ctx, serverID)
	if err != nil {
		return err
	}
	if !state.Exists {
		return fmt.Errorf("workload %q does not exist", serverID)
	}
	if !state.Running {
		return nil
	}
	timeout := 30
	callCtx, cancel := withEngineDeadline(ctx, engineStopTimeout)
	defer cancel()
	return r.client.ContainerStop(callCtx, containerName(serverID), container.StopOptions{Timeout: &timeout})
}

func (r *DockerRuntime) WaitForStop(ctx context.Context, serverID string, duration time.Duration, terminate bool) error {
	state, err := r.Inspect(ctx, serverID)
	if err != nil {
		return err
	}
	if !state.Exists {
		return fmt.Errorf("workload %q does not exist", serverID)
	}
	if !state.Running {
		return nil
	}
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
	lock := r.workloadLock(serverID)
	lock.Lock()
	defer lock.Unlock()
	state, err := r.Inspect(ctx, serverID)
	if err != nil {
		return err
	}
	if !state.Exists {
		return fmt.Errorf("workload %q does not exist", serverID)
	}
	if !state.Running {
		return nil
	}
	return r.client.ContainerKill(ctx, containerName(serverID), "SIGKILL")
}

func (r *DockerRuntime) Signal(ctx context.Context, serverID, signal string) error {
	lock := r.workloadLock(serverID)
	lock.Lock()
	defer lock.Unlock()
	signal = strings.ToUpper(strings.TrimSpace(signal))
	if !validStopSignal(signal) {
		return fmt.Errorf("unsupported stop signal %q", signal)
	}
	state, err := r.Inspect(ctx, serverID)
	if err != nil {
		return err
	}
	if !state.Exists || !state.Running {
		return fmt.Errorf("workload %q is not running", serverID)
	}
	return r.client.ContainerKill(ctx, containerName(serverID), signal)
}

func (r *DockerRuntime) Restart(ctx context.Context, serverID string) error {
	lock := r.workloadLock(serverID)
	lock.Lock()
	defer lock.Unlock()
	state, err := r.Inspect(ctx, serverID)
	if err != nil {
		return err
	}
	if !state.Exists {
		return fmt.Errorf("workload %q does not exist", serverID)
	}
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
	// A container that was created but never started has no output. Docker
	// would return an empty stream with nil error, which reads as
	// "healthy but quiet"; refuse it instead so callers cannot mistake
	// never-ran for quiet.
	if inspection, err := r.client.ContainerInspect(ctx, containerName(serverID)); err == nil && inspection.State != nil {
		if started, parseErr := time.Parse(time.RFC3339Nano, inspection.State.StartedAt); (parseErr != nil || started.IsZero()) && !inspection.State.Running {
			return nil, fmt.Errorf("container %q has never started: no logs to report", containerName(serverID))
		}
	}
	// The bound is an explicit line tail, the same one LogsStream applies, and
	// it is visible to the caller as "the last N lines". It used to be a five
	// minute window, which silently dropped everything older and still returned
	// HTTP 200 with a short body: an absent reading presented as a quiet
	// container.
	return r.client.ContainerLogs(ctx, containerName(serverID), container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     false,
		Timestamps: true,
		Tail:       "10000",
	})
}

func (r *DockerRuntime) LogsStream(ctx context.Context, serverID string, tail string) (io.ReadCloser, error) {
	if strings.TrimSpace(tail) == "" || tail == "all" {
		tail = "10000"
	}
	reader, err := r.client.ContainerLogs(ctx, containerName(serverID), container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: true,
		Tail:       tail,
	})
	if err != nil {
		return nil, err
	}
	return &limitedReadCloser{Reader: io.LimitReader(reader, 64<<20), closer: reader}, nil
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
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, copyErr := stdcopy.StdCopy(writer, writer, attached.Reader)
		_ = writer.CloseWithError(copyErr)
	}()
	go func() {
		select {
		case <-ctx.Done():
			_ = session.Close()
			_ = writer.CloseWithError(ctx.Err())
		case <-done:
		}
	}()
	return session, nil
}

func (r *DockerRuntime) Delete(ctx context.Context, serverID string) error {
	lock := r.workloadLock(serverID)
	lock.Lock()
	defer lock.Unlock()
	err := r.client.ContainerRemove(ctx, containerName(serverID), container.RemoveOptions{
		Force:         true,
		RemoveVolumes: true,
	})
	if errdefs.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *DockerRuntime) WatchEvents(ctx context.Context) (<-chan ContainerEvent, <-chan error) {
	out := make(chan ContainerEvent, 128)
	errs := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errs)
		backoff := time.Second
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
					backoff = time.Second
					serverID := event.Actor.Attributes["modern-game-panel.server_id"]
					if serverID == "" {
						continue
					}
					exitCode, _ := strconv.Atoi(event.Actor.Attributes["exitCode"])
					oomKilled := false
					if strings.EqualFold(event.Actor.Attributes["oomKilled"], "true") {
						oomKilled = true
					}
					select {
					case out <- ContainerEvent{ServerID: serverID, Action: string(event.Action), ExitCode: exitCode, OOMKilled: oomKilled}:
					case <-ctx.Done():
						return
					default:
						select {
						case errs <- errors.New("Docker event dropped because the consumer is not keeping up"):
						default:
						}
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

type limitedReadCloser struct {
	io.Reader
	closer io.Closer
}

func (r *limitedReadCloser) Close() error { return r.closer.Close() }

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
	resolved, err := filepath.EvalSymlinks(rootDir)
	if err != nil {
		return "", fmt.Errorf("resolve root directory: %w", err)
	}
	rootDir = resolved
	info, err := os.Stat(rootDir)
	if err != nil {
		return "", fmt.Errorf("root directory is unavailable: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("root directory is not a directory")
	}
	return rootDir, nil
}

// canonicalMountSource cleans a host mount source exactly once and resolves it
// through symlinks exactly once, so every caller shares the same canonical
// form and a TOCTOU between two resolutions cannot smuggle in a different
// directory.
func canonicalMountSource(source string) (string, error) {
	if !filepath.IsAbs(source) {
		return "", errors.New("custom mount source must be absolute")
	}
	cleaned := filepath.Clean(source)
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return "", fmt.Errorf("resolve custom mount source %q: %w", cleaned, err)
	}
	return resolved, nil
}

// validateInstallEnv rejects malformed or dangerous installer environment
// entries before they reach the container config.
func validateInstallEnv(env []string) error {
	for _, entry := range env {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || strings.TrimSpace(name) == "" {
			return fmt.Errorf("invalid environment entry %q", entry)
		}
		if strings.ContainsAny(entry, "\x00\r\n") {
			return fmt.Errorf("invalid environment entry %q", name)
		}
		for _, r := range name {
			if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
				return fmt.Errorf("invalid environment name %q", name)
			}
		}
	}
	return nil
}

func buildContainerMounts(rootDir string, custom []Mount) ([]mount.Mount, error) {
	rootDir, err := validateRootDir(rootDir)
	if err != nil {
		return nil, err
	}
	// Docker bind mounts inherit the host's mount flags, so Beacon requests a
	// recursive read-only bind where asked and always runs workloads with a
	// read-only rootfs, no-new-privileges, and dropped capabilities (see
	// buildHostConfigWithSettings). The OCI runtimes (containerd) carry
	// explicit nosuid,nodev,noexec bind options; Docker's Mount API exposes no
	// equivalent flag, so the hardening here is a single canonicalization plus
	// strict target confinement.
	mounts := []mount.Mount{{
		Type:        mount.TypeBind,
		Source:      rootDir,
		Target:      serverContainerRoot,
		BindOptions: &mount.BindOptions{CreateMountpoint: false},
	}}
	for _, customMount := range custom {
		if customMount.Source == "" || customMount.Target == "" {
			continue
		}
		source, err := canonicalMountSource(customMount.Source)
		if err != nil {
			return nil, err
		}
		target := pathpkg.Clean(customMount.Target)
		if !pathpkg.IsAbs(target) || target == "/" {
			return nil, errors.New("custom mount target must be an absolute container path below /")
		}
		if target == serverContainerRoot {
			return nil, errors.New("custom mount cannot replace /home/container")
		}
		mounts = append(mounts, mount.Mount{
			Type:        mount.TypeBind,
			Source:      source,
			Target:      target,
			ReadOnly:    customMount.ReadOnly,
			BindOptions: &mount.BindOptions{CreateMountpoint: false, ReadOnlyForceRecursive: customMount.ReadOnly},
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
	return buildHostConfigWithSettings(req, mounts, portBindings, loadDockerHostSettings())
}

func loadDockerHostSettings() dockerHostSettings {
	settings := dockerHostSettings{
		rootless:   os.Getenv("DAEMON_DOCKER_ROOTLESS_ENABLED") == "true",
		logMaxSize: strings.TrimSpace(os.Getenv("DAEMON_LOG_MAX_SIZE")),
		logMaxFile: strings.TrimSpace(os.Getenv("DAEMON_LOG_MAX_FILE")),
	}
	if settings.logMaxSize == "" {
		settings.logMaxSize = "10m"
	}
	if settings.logMaxFile == "" {
		settings.logMaxFile = "3"
	}
	return settings
}

func buildHostConfigWithSettings(req CreateRequest, mounts []mount.Mount, portBindings nat.PortMap, settings dockerHostSettings) *container.HostConfig {
	securityOpts := []string{"no-new-privileges:true", "seccomp=builtin"}
	usernsMode := ""
	if settings.rootless {
		usernsMode = "host"
	}
	tmpfsSizeMB := int64(64)
	if req.MemoryMB > 0 {
		tmpfsSizeMB = req.MemoryMB / 4
		if tmpfsSizeMB < 16 {
			tmpfsSizeMB = 16
		}
		if tmpfsSizeMB > 1024 {
			tmpfsSizeMB = 1024
		}
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
		Tmpfs:          map[string]string{"/tmp": fmt.Sprintf("rw,exec,size=%dM", tmpfsSizeMB)},
		LogConfig: container.LogConfig{
			Type: "json-file",
			Config: map[string]string{
				"max-size": settings.logMaxSize,
				"max-file": settings.logMaxFile,
			},
		},
	}
}

func dockerPorts(bindings []PortBinding) (nat.PortSet, nat.PortMap, error) {
	exposed := nat.PortSet{}
	published := nat.PortMap{}
	hostBindings := make(map[string]struct{}, len(bindings))
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
		if binding.HostIP != "" && net.ParseIP(stripIPPrefix(binding.HostIP)) == nil {
			return nil, nil, fmt.Errorf("invalid allocation IP %q", binding.HostIP)
		}
		hostIP := stripIPPrefix(binding.HostIP)
		hostKey := net.JoinHostPort(hostIP, strconv.Itoa(binding.HostPort)) + "/" + protocol
		if _, exists := hostBindings[hostKey]; exists {
			return nil, nil, fmt.Errorf("duplicate host port binding %s", hostKey)
		}
		hostBindings[hostKey] = struct{}{}
		port := nat.Port(strconv.Itoa(containerPort) + "/" + protocol)
		exposed[port] = struct{}{}
		published[port] = append(published[port], nat.PortBinding{HostIP: hostIP, HostPort: strconv.Itoa(binding.HostPort)})
	}
	return exposed, published, nil
}

const configHashLabel = "modern-game-panel.config_hash"

func validateCreateRequest(req CreateRequest) error {
	if strings.TrimSpace(req.ServerID) == "" || strings.TrimSpace(req.Image) == "" {
		return errors.New("server ID and image are required")
	}
	if !validContainerID(req.ServerID) {
		return errors.New("server ID contains invalid characters or is too long")
	}
	if req.MemoryMB < 0 || req.SwapMB < 0 {
		return errors.New("memory and swap must not be negative")
	}
	if req.MemoryOverhead < 0 || req.MemoryOverhead > 100 {
		return errors.New("memory overhead must be between 0 and 100 percent")
	}
	if req.SwapMB > 0 && req.MemoryMB == 0 {
		return errors.New("swap requires a positive memory limit")
	}
	const maxMemoryMB = int64(8 * 1024 * 1024)
	if req.MemoryMB > maxMemoryMB || req.SwapMB > maxMemoryMB {
		return errors.New("memory and swap limits must not exceed 8 TiB")
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

// stripIPPrefix returns the bare IP when callers pass Postgres inet text
// ("127.0.0.1/32") instead of a plain address. Docker's PortBinding requires
// a bare IP; passing CIDR through fails closed at the daemon with "invalid
// allocation IP". Unknown is not zero here either: unparseable input is
// returned unchanged so validation still rejects it.
func stripIPPrefix(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if ip, _, err := net.ParseCIDR(value); err == nil {
		return ip.String()
	}
	if idx := strings.IndexByte(value, '/'); idx >= 0 {
		if ip := net.ParseIP(value[:idx]); ip != nil {
			return ip.String()
		}
	}
	return value
}

func validStopSignal(signal string) bool {
	switch strings.ToUpper(strings.TrimSpace(signal)) {
	case "SIGTERM", "SIGINT", "SIGQUIT", "SIGHUP", "SIGUSR1", "SIGUSR2", "SIGKILL":
		return true
	}
	return false
}

func validContainerID(value string) bool {
	if len(value) == 0 || len(value) > 120 {
		return false
	}
	for i, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || (i > 0 && (r == '_' || r == '.' || r == '-')) {
			continue
		}
		return false
	}
	return true
}

func buildContainerConfig(req CreateRequest, ports nat.PortSet, hash string) *container.Config {
	userSpec := ""
	if req.UID != 0 || req.GID != 0 {
		userSpec = fmt.Sprintf("%d:%d", req.UID, req.GID)
	}
	timeout := int(req.StopTimeout / time.Second)
	config := &container.Config{Image: req.Image, Cmd: req.Command, Env: req.Env, WorkingDir: serverContainerRoot, AttachStdin: true, AttachStdout: true, AttachStderr: true, OpenStdin: true, User: userSpec, StopSignal: strings.ToUpper(req.StopSignal), Labels: map[string]string{"modern-game-panel.server_id": req.ServerID, configHashLabel: hash}, ExposedPorts: ports}
	if req.StopTimeout > 0 {
		config.StopTimeout = &timeout
	}
	return config
}

func createRequestHash(req CreateRequest) (string, error) {
	copyReq := req
	if req.RegistryAuth != nil {
		copyReq.RegistryAuth = &RegistryAuth{
			Username:      req.RegistryAuth.Username,
			ServerAddress: req.RegistryAuth.ServerAddress,
		}
	}
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

// ImagePullOptions exposes imagePullOptions to the server package so node
// handlers that pull outside the runtime (database provisioning) encode
// registry credentials exactly the same way the runtime does.
func ImagePullOptions(auth *RegistryAuth) (image.PullOptions, error) {
	return imagePullOptions(auth)
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
	if errdefs.IsConflict(err) {
		// A concurrent creator won the race. Re-read and validate its network
		// rather than treating an equivalent, managed network as a failure.
		return r.ensureNetwork(ctx, req)
	}
	return err
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
