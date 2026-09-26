package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gamepanel/forge/internal/daemon"
)

const (
	DockerProvider      = "docker"
	ContainerdProvider  = "containerd"
	PodmanProvider      = "podman"
	FirecrackerProvider = "firecracker"
	KubernetesProvider  = "kubernetes"
	KVMProvider         = "kvm"
	LXCProvider         = "lxc"
)

type Target struct {
	NodeID    string
	NodeURL   string
	NodeToken string
	ServerID  string
	Provider  string
}

type Port struct {
	HostIP        string `json:"hostIp"`
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

type Mount struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only"`
}

type CreateServerRequest struct {
	ServerID       string        `json:"serverId"`
	Name           string        `json:"name"`
	Image          string        `json:"image"`
	Command        []string      `json:"command"`
	Env            []string      `json:"env"`
	Ports          []Port        `json:"ports"`
	Mounts         []Mount       `json:"mounts"`
	MemoryMB       int64         `json:"memoryMb"`
	SwapMB         int64         `json:"swapMb"`
	CPUShares      int64         `json:"cpuShares"`
	CPULimit       int64         `json:"cpuLimit"`
	DiskMB         int64         `json:"diskMb"`
	IOWeight       int64         `json:"ioWeight"`
	Threads        string        `json:"threads"`
	OOMDisabled    bool          `json:"oomDisabled"`
	PIDLimit       int64         `json:"pidLimit"`
	StopSignal     string        `json:"stopSignal"`
	StopTimeout    int64         `json:"stopTimeoutSeconds"`
	UID            int           `json:"uid"`
	GID            int           `json:"gid"`
	DNS            []string      `json:"dns"`
	NetworkName    string        `json:"networkName"`
	NetworkSubnet  string        `json:"networkSubnet"`
	NetworkGateway string        `json:"networkGateway"`
	NetworkIP      string        `json:"networkIp"`
	RegistryAuth   *RegistryAuth `json:"registryAuth,omitempty"`
	RuntimeType    string        `json:"runtimeType,omitempty"`
}

type RegistryAuth struct {
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	IdentityToken string `json:"identityToken,omitempty"`
	RegistryToken string `json:"registryToken,omitempty"`
	ServerAddress string `json:"serverAddress,omitempty"`
}

type InstallRequest struct {
	ServerID   string
	Image      string
	Entrypoint string
	Script     string
	Env        map[string]string
}

type InstallResponse struct {
	ServerID string
	Accepted bool
	Mode     string
	ExitCode int
	Logs     string
}

type ServerConfiguration struct {
	UUID        string
	Name        string
	Suspended   bool
	Environment map[string]string
	Invocation  string
	DockerImage string
	Egg         map[string]any
	Build       map[string]any
	Allocations map[string]any
	Config      map[string]any
	Mounts      []Mount
	UID         int
	GID         int
}

type PowerResponse struct {
	ServerID string `json:"serverId"`
	Signal   string `json:"signal"`
	Accepted bool   `json:"accepted"`
	Mode     string `json:"mode,omitempty"`
}

type CreateResponse struct {
	ServerID string `json:"serverId"`
	Accepted bool   `json:"accepted"`
	Mode     string `json:"mode,omitempty"`
}

type Stats struct {
	CPUPercent     float64 `json:"cpuPercent"`
	MemoryBytes    uint64  `json:"memoryBytes"`
	MemoryLimit    uint64  `json:"memoryLimit"`
	NetworkRxBytes uint64  `json:"networkRxBytes"`
	NetworkTxBytes uint64  `json:"networkTxBytes"`
}

type Inspection struct {
	ServerID string `json:"serverId"`
	Exists   bool   `json:"exists"`
	Provider string `json:"provider"`
	// Running and Status carry the workload's observed lifecycle. They are only
	// meaningful when StateKnown is true: an older Beacon that does not expose
	// container state yields an inspection where existence was inferred from
	// telemetry, and treating that as a lifecycle reading would let a stopped
	// container look missing or a crashed one look fine.
	Running    bool      `json:"running"`
	Status     string    `json:"status,omitempty"`
	StartedAt  time.Time `json:"startedAt,omitempty"`
	StateKnown bool      `json:"stateKnown"`
}

// inspectWorkload reads a workload's lifecycle straight from the node instead
// of inferring it from the presence of metrics. A container that has exited
// still exists but streams no stats, so the old "the stats call worked, so it
// is running" reading conflated running, stopped and missing into one signal.
//
// Against a Beacon that predates the state endpoint the call degrades to the
// stats heuristic and marks the result not state-known, so callers can say they
// observed less rather than claiming a lifecycle they never read.
func inspectWorkload(ctx context.Context, client *daemon.Client, target Target, provider string) (Inspection, error) {
	if client == nil {
		return Inspection{}, ErrRuntimeUnavailable
	}
	serverID := target.ServerID
	state, err := client.ContainerState(ctx, target.NodeURL, target.NodeToken, serverID)
	if errors.Is(err, daemon.ErrContainerStateUnsupported) {
		_, statsErr := client.Stats(ctx, target.NodeURL, target.NodeToken, serverID)
		if statsErr != nil {
			return Inspection{ServerID: serverID, Provider: provider}, statsErr
		}
		return Inspection{ServerID: serverID, Exists: true, Provider: provider, StateKnown: false}, nil
	}
	if err != nil {
		return Inspection{ServerID: serverID, Provider: provider}, err
	}
	if state.ServerID != "" {
		serverID = state.ServerID
	}
	return Inspection{
		ServerID:   serverID,
		Exists:     state.Exists,
		Running:    state.Running,
		Status:     state.Status,
		StartedAt:  state.StartedAt,
		Provider:   provider,
		StateKnown: true,
	}, nil
}

// existsWorkload reports only whether the node holds a container for the
// server, using the same state-first reading as inspectWorkload.
func existsWorkload(ctx context.Context, client *daemon.Client, target Target, provider string) (bool, error) {
	inspection, err := inspectWorkload(ctx, client, target, provider)
	if err != nil {
		return false, err
	}
	return inspection.Exists, nil
}

type MigrationRequest struct {
	MigrationID  string `json:"migrationId"`
	ServerID     string `json:"serverId"`
	SourceNodeID string `json:"sourceNodeId"`
	TargetNodeID string `json:"targetNodeId"`
}

type MigrationResponse struct {
	MigrationID string `json:"migrationId"`
	Accepted    bool   `json:"accepted"`
	Mode        string `json:"mode,omitempty"`
}

// Reinstaller is an optional runtime capability used when a daemon exposes a
// distinct reinstall endpoint. Install remains part of the base Runtime contract.
type Reinstaller interface {
	ReinstallServer(context.Context, Target, InstallRequest) (InstallResponse, error)
}

type Runtime interface {
	Name() string
	Capabilities() Capabilities
	SupportsMigration() bool
	CreateServer(context.Context, Target, CreateServerRequest) (CreateResponse, error)
	InstallServer(context.Context, Target, InstallRequest) (InstallResponse, error)
	SyncServerConfiguration(context.Context, Target, ServerConfiguration) error
	DeleteServer(context.Context, Target) (PowerResponse, error)
	StartServer(context.Context, Target) (PowerResponse, error)
	StopServer(context.Context, Target) (PowerResponse, error)
	RestartServer(context.Context, Target) (PowerResponse, error)
	KillServer(context.Context, Target) (PowerResponse, error)
	Stats(context.Context, Target) (Stats, error)
	Exists(context.Context, Target) (bool, error)
	Inspect(context.Context, Target) (Inspection, error)
	PrepareMigration(context.Context, MigrationRequest) (MigrationResponse, error)
	ExecuteMigration(context.Context, MigrationRequest) (MigrationResponse, error)
	CancelMigration(context.Context, MigrationRequest) (MigrationResponse, error)
	ResizeServer(context.Context, Target, int64, int64) error
}

var ErrRuntimeUnavailable = errors.New("runtime unavailable")
var ErrMigrationManagedByControlPlane = errors.New("migration is managed by the control-plane migration service")
var ErrUnsupportedRuntimeOperation = errors.New("runtime operation is unsupported")

// ErrUnsupportedProvider is returned for a runtime provider Forge cannot
// actually run. Handlers map it to HTTP 400.
var ErrUnsupportedProvider = errors.New("unsupported runtime provider")

// experimentalRuntimesEnabled reports whether providers that exist as adapters
// but have no verified execution path may be selected.
func experimentalRuntimesEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ENABLE_EXPERIMENTAL_RUNTIMES"))) {
	case "", "0", "false", "no":
		return false
	default:
		return true
	}
}

// NormalizeProvider trims and lowercases a provider name so that lookups and
// validation agree regardless of how a caller spelled it.
func NormalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

// IsSupportedProvider reports whether Forge can run a workload with the given
// provider. An empty provider means "use the node's default" and is allowed.
//
// KVM and LXC have adapters but advertise no capabilities and have no verified
// path through beacon, which still creates a Docker container whatever the
// request asked for. Accepting them by default let a user select a hypervisor
// and silently receive a container, so they are gated behind
// ENABLE_EXPERIMENTAL_RUNTIMES.
func IsSupportedProvider(provider string) bool {
	switch NormalizeProvider(provider) {
	case "", DockerProvider, ContainerdProvider, PodmanProvider, FirecrackerProvider, KubernetesProvider:
		return true
	case KVMProvider, LXCProvider:
		return experimentalRuntimesEnabled()
	default:
		return false
	}
}

// ValidateProvider returns ErrUnsupportedProvider for anything Forge cannot run.
func ValidateProvider(provider string) error {
	if IsSupportedProvider(provider) {
		return nil
	}
	return fmt.Errorf("%q: %w", NormalizeProvider(provider), ErrUnsupportedProvider)
}

// AllProviders is the canonical, ordered set of runtime providers Forge models.
// It is a superset of IsSupportedProvider: engines that exist as adapters but
// are gated or unwired still appear, so a reporting surface can say *why* each
// one is or is not usable rather than silently omitting it.
func AllProviders() []string {
	return []string{
		DockerProvider,
		ContainerdProvider,
		PodmanProvider,
		FirecrackerProvider,
		KubernetesProvider,
		KVMProvider,
		LXCProvider,
	}
}

// IsExperimentalProvider reports whether a provider is behind the
// ENABLE_EXPERIMENTAL_RUNTIMES opt-in. Kept adjacent to IsSupportedProvider so
// the two views of the same rule cannot drift.
func IsExperimentalProvider(provider string) bool {
	switch NormalizeProvider(provider) {
	case KVMProvider, LXCProvider:
		return true
	default:
		return false
	}
}
