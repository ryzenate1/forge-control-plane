package runtime

import (
	"context"
	"errors"
)

const (
	DockerProvider      = "docker"
	ContainerdProvider  = "containerd"
	PodmanProvider      = "podman"
	FirecrackerProvider = "firecracker"
	KubernetesProvider  = "kubernetes"
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
var ErrNotImplemented = errors.New("runtime operation not implemented")
