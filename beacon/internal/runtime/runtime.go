package runtime

import (
	"context"
	"errors"
	"io"
	"time"
)

type BuildRequest struct {
	ContextDir     string
	DockerfilePath string
	ImageTag       string
	BuildArgs      map[string]string
}

type BuildResult struct {
	Image string
	Log   string
}

// Builder is optional because only a Docker-capable Beacon can build
// Dockerfiles. Other runtime providers continue to expose lifecycle actions.
type Builder interface {
	Build(context.Context, BuildRequest) (BuildResult, error)
}

const (
	ProviderDocker      = "docker"
	ProviderContainerd  = "containerd"
	ProviderPodman      = "podman"
	ProviderFirecracker = "firecracker"
	ProviderKubernetes  = "kubernetes"
)

type RuntimeConfig struct {
	Provider    string
	Containerd  ContainerdConfig
	Podman      PodmanConfig
	Firecracker FirecrackerConfig
	Kubernetes  KubernetesConfig
}

type ProviderConfig struct {
	DefaultProvider string `mapstructure:"default_provider"`
}

type ContainerdConfig struct {
	Address   string
	Namespace string
}

type PodmanConfig struct {
	URI      string
	Identity string
}

type FirecrackerConfig struct {
	SocketPath     string
	KernelImage    string
	RootfsImage    string
	CPUTemplate    string
	JailerPath     string
	FirecrackerBin string
}

type KubernetesConfig struct {
	KubeconfigPath string
	Namespace      string
	InCluster      bool
}

type CreateRequest struct {
	ServerID        string
	Image           string
	Command         []string
	Env             []string
	Ports           []PortBinding
	Mounts          []Mount
	MemoryMB        int64
	SwapMB          int64
	CPUShares       int64
	CPUPercent      int64
	CPUSet          string
	IOWeight        int64
	OOMKillDisabled bool
	PIDLimit        int64
	StopSignal      string
	StopTimeout     time.Duration
	UID             int
	GID             int
	DNS             []string
	NetworkName     string
	NetworkSubnet   string
	NetworkGateway  string
	NetworkIP       string
	RegistryAuth    *RegistryAuth
	RootDir         string
}

type RegistryAuth struct {
	Username      string
	Password      string
	IdentityToken string
	RegistryToken string
	ServerAddress string
}

var ErrRestartRequired = errors.New("container configuration changed; restart required")

type Mount struct {
	Source   string
	Target   string
	ReadOnly bool
}

type PortBinding struct {
	HostIP        string
	HostPort      int
	ContainerPort int
	Protocol      string
}

type Stats struct {
	CPUPercent     float64 `json:"cpuPercent"`
	MemoryBytes    uint64  `json:"memoryBytes"`
	MemoryLimit    uint64  `json:"memoryLimit"`
	NetworkRxBytes uint64  `json:"networkRxBytes"`
	NetworkTxBytes uint64  `json:"networkTxBytes"`
}

type InstallRequest struct {
	ServerID   string
	Image      string
	Entrypoint string
	Script     string
	Env        []string
	RootDir    string
}

type InstallResult struct {
	ExitCode int
	Logs     string
}

type ContainerState struct {
	ServerID string
	ID       string
	Exists   bool
	Running  bool
	Status   string
}

// Reconciler is implemented by runtimes that can safely apply a desired
// workload configuration to an existing workload.
type Reconciler interface {
	Reconcile(ctx context.Context, req CreateRequest) error
}

type Runtime interface {
	Create(ctx context.Context, req CreateRequest) error
	Install(ctx context.Context, req InstallRequest) (InstallResult, error)
	Inspect(ctx context.Context, serverID string) (ContainerState, error)
	List(ctx context.Context) ([]ContainerState, error)
	Start(ctx context.Context, serverID string) error
	SendCommand(ctx context.Context, serverID, command string) error
	Stop(ctx context.Context, serverID string) error
	WaitForStop(ctx context.Context, serverID string, duration time.Duration, terminate bool) error
	Kill(ctx context.Context, serverID string) error
	Signal(ctx context.Context, serverID, signal string) error
	Restart(ctx context.Context, serverID string) error
	Stats(ctx context.Context, serverID string) (Stats, error)
	Logs(ctx context.Context, serverID string) (io.ReadCloser, error)
	LogsStream(ctx context.Context, serverID string, tail string) (io.ReadCloser, error)
	StatsStream(ctx context.Context, serverID string) (io.ReadCloser, error)
	AttachConsole(ctx context.Context, serverID string) (ConsoleSession, error)
	Delete(ctx context.Context, serverID string) error
}

type Pinger interface {
	Ping(ctx context.Context) error
}

type ConsoleSession interface {
	io.Reader
	io.Writer
	io.Closer
}

type ContainerEvent struct {
	ServerID  string
	Action    string
	ExitCode  int
	OOMKilled bool
}

type EventWatcher interface {
	WatchEvents(ctx context.Context) (<-chan ContainerEvent, <-chan error)
}
