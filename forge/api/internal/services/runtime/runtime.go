package runtime

import (
	"context"
	"io"
)

type RuntimeType string

const (
	RuntimeDocker RuntimeType = "docker"
	RuntimeBeacon RuntimeType = "beacon"
	RuntimeCustom RuntimeType = "custom"
)

type ContainerStats struct {
	MemoryMB      float64 `json:"memoryMb"`
	MemoryLimitMB float64 `json:"memoryLimitMb"`
	CPUPercent    float64 `json:"cpuPercent"`
	NetworkRx     int64   `json:"networkRx"`
	NetworkTx     int64   `json:"networkTx"`
	Uptime        int64   `json:"uptime"`
}

type LogLine struct {
	Timestamp int64  `json:"timestamp"`
	Line      string `json:"line"`
	Stream    string `json:"stream"`
}

type Runtime interface {
	Type() RuntimeType
	Start(ctx context.Context, serverID string, image string, env map[string]string) error
	Stop(ctx context.Context, serverID string) error
	Restart(ctx context.Context, serverID string) error
	Kill(ctx context.Context, serverID string) error
	Status(ctx context.Context, serverID string) (string, error)
	Stats(ctx context.Context, serverID string) (*ContainerStats, error)
	Logs(ctx context.Context, serverID string, tail int) ([]LogLine, error)
	ExecuteCommand(ctx context.Context, serverID string, command string) error
	ReadFile(ctx context.Context, serverID string, path string) (io.ReadCloser, error)
	WriteFile(ctx context.Context, serverID string, path string, reader io.Reader) error
	DeleteFiles(ctx context.Context, serverID string, paths []string) error
	CreateDirectory(ctx context.Context, serverID string, path string) error
	ListFiles(ctx context.Context, serverID string, path string) ([]FileInfo, error)
	GetInstallScript(ctx context.Context, serverID string) (string, error)
	ArchiveFiles(ctx context.Context, serverID string, paths []string, destPath string) error
}

type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	IsDir   bool   `json:"isDir"`
	ModTime int64  `json:"modTime"`
}

type Registry struct {
	runtimes map[RuntimeType]Runtime
}

func NewRegistry() *Registry {
	return &Registry{runtimes: make(map[RuntimeType]Runtime)}
}

func (r *Registry) Register(rt Runtime) {
	r.runtimes[rt.Type()] = rt
}

func (r *Registry) Get(rt RuntimeType) Runtime {
	return r.runtimes[rt]
}

func (r *Registry) Default() Runtime {
	if d, ok := r.runtimes[RuntimeDocker]; ok {
		return d
	}
	for _, rt := range r.runtimes {
		return rt
	}
	return nil
}
