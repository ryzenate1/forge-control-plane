package scheduler

import "context"

type SchedulerType string

const (
	// SchedulerTypeDocker matches the nodes.scheduler_type column default and
	// the admin UI vocabulary, but docker is deliberately NOT served by this
	// package: docker workloads run through Beacon's runtime adapters over
	// the Panel→Beacon HTTP client. NewScheduler/NodeScheduler report it with
	// ErrDockerHandledByBeacon instead of pretending to build one.
	SchedulerTypeDocker SchedulerType = "docker"
	SchedulerTypeK3s    SchedulerType = "k3s"
	SchedulerTypeNomad  SchedulerType = "nomad"
)

type SchedulerConfig struct {
	Type   SchedulerType `json:"type"`
	K3s    *K3sConfig    `json:"k3s,omitempty"`
	Nomad  *NomadConfig  `json:"nomad,omitempty"`
}

type K3sConfig struct {
	KubeconfigPath string `json:"kubeconfigPath"`
	Namespace      string `json:"namespace"`
	KubeAPI        string `json:"kubeApi,omitempty"`
}

type NomadConfig struct {
	Addr      string `json:"addr"`
	Region    string `json:"region"`
	Datacenter string `json:"datacenter"`
	Namespace string `json:"namespace"`
}

type DeployRequest struct {
	Name       string            `json:"name"`
	Image      string            `json:"image"`
	Command    []string          `json:"command"`
	Env        map[string]string `json:"env"`
	Ports      []SchedulerPort   `json:"ports"`
	Mounts     []SchedulerMount  `json:"mounts"`
	MemoryMB   int64             `json:"memoryMb"`
	CPUMHz     int64             `json:"cpuMHz"`
	DiskMB     int64             `json:"diskMb"`
	Replicas   int               `json:"replicas"`
	Labels     map[string]string `json:"labels"`
}

type SchedulerPort struct {
	Name       string `json:"name"`
	Port       int    `json:"port"`
	Protocol   string `json:"protocol"`
	TargetPort int    `json:"targetPort"`
	NodePort   int    `json:"nodePort,omitempty"`
}

type SchedulerMount struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"readOnly"`
}

type DeployResponse struct {
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	Endpoints []ServiceEndpoint `json:"endpoints,omitempty"`
}

type ServiceEndpoint struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

type ScaleRequest struct {
	Name     string `json:"name"`
	Replicas int    `json:"replicas"`
}

// ResourceUsage carries only what the platform actually measured. Fields are
// pointers because "not reported" must never be encoded as zero: a query that
// did not answer for a metric leaves that field nil (e.g. metrics-server
// reports no disk usage; a Nomad job spec reports configured CPU, never a
// live percentage).
type ResourceUsage struct {
	CPUPercent *float64 `json:"cpuPercent,omitempty"`
	CPUMHz     *int64   `json:"cpuMHz,omitempty"`
	MemoryMB   *int64   `json:"memoryMb,omitempty"`
	DiskMB     *int64   `json:"diskMb,omitempty"`
}

type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
	Source    string `json:"source"`
}

type Event struct {
	Type      string `json:"type"`
	Reason    string `json:"reason"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

type Scheduler interface {
	Type() SchedulerType
	Name() string
	Deploy(ctx context.Context, req DeployRequest) (DeployResponse, error)
	Stop(ctx context.Context, name string) error
	Start(ctx context.Context, name string) error
	Restart(ctx context.Context, name string) error
	Scale(ctx context.Context, req ScaleRequest) error
	GetStatus(ctx context.Context, name string) (string, error)
	GetLogs(ctx context.Context, name string, tail int) ([]LogEntry, error)
	GetEvents(ctx context.Context, name string) ([]Event, error)
	GetResources(ctx context.Context, name string) (ResourceUsage, error)
}
