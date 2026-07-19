package store

import (
	"time"
)

type Node struct {
	ID                     string           `json:"id"`
	UUID                   string           `json:"uuid"`
	Name                   string           `json:"name"`
	Description            string           `json:"description,omitempty"`
	Region                 string           `json:"region"`
	RegionID               *string          `json:"regionId,omitempty"`
	LocationID             *string          `json:"locationId,omitempty"`
	BaseURL                string           `json:"baseUrl"`
	FQDN                   string           `json:"fqdn,omitempty"`
	Scheme                 string           `json:"scheme,omitempty"`
	BehindProxy            bool             `json:"behindProxy"`
	Status                 string           `json:"status"`
	DesiredState           NodeDesiredState `json:"desiredState"`
	ActualState            string           `json:"actualState,omitempty"`
	Maintenance            bool             `json:"maintenanceMode"`
	Draining               bool             `json:"draining"`
	MemoryMB               int              `json:"memoryMb"`
	DiskMB                 int              `json:"diskMb"`
	UploadSizeMB           int              `json:"uploadSizeMb"`
	DaemonBase             string           `json:"daemonBase,omitempty"`
	DaemonListen           int              `json:"daemonListen"`
	DaemonSFTP             int              `json:"daemonSftp"`
	TokenID                string           `json:"tokenId,omitempty"`
	LastSeenAt             *time.Time       `json:"lastSeenAt,omitempty"`
	LastHeartbeatAt        time.Time        `json:"lastHeartbeatAt"`
	Version                *string          `json:"version,omitempty"`
	OS                     *string          `json:"os,omitempty"`
	Architecture           *string          `json:"architecture,omitempty"`
	CPUThreads             *int             `json:"cpuThreads,omitempty"`
	DockerStatus           *string          `json:"dockerStatus,omitempty"`
	RuntimeStatus          *string          `json:"runtimeStatus,omitempty"`
	NodeMemoryMB           *int             `json:"nodeMemoryMb,omitempty"`
	NodeDiskMB             *int             `json:"nodeDiskMB,omitempty"`
	HeartbeatErr           *string          `json:"heartbeatError,omitempty"`
	HeartbeatState         string           `json:"heartbeatState,omitempty"`
	HeartbeatRecoveryCount int              `json:"heartbeatRecoveryCount"`
	RuntimeProvider        string           `json:"runtimeProvider,omitempty"`
	Public                 bool             `json:"public"`
	DisplayName            string           `json:"displayName,omitempty"`
	PublicHostname         string           `json:"publicHostname,omitempty"`
	ListenPortMin          int              `json:"listenPortMin,omitempty"`
	ListenPortMax          int              `json:"listenPortMax,omitempty"`
	AllowedIPs             []string         `json:"allowedIps,omitempty"`
	NetworkInterface       string           `json:"networkInterface,omitempty"`
	DaemonSSLCert          string           `json:"daemonSslCert,omitempty"`
	DaemonSSLKey           string           `json:"daemonSslKey,omitempty"`
	AutoConnect            bool             `json:"autoConnect"`
	ConnectionRetries      int              `json:"connectionRetries"`
	HeartbeatInterval      int              `json:"heartbeatInterval"`
	CPUCores               int              `json:"cpuCores"`
	MemoryOverallocate     int              `json:"memoryOverallocate"`
	DiskOverallocate       int              `json:"diskOverallocate"`
	ReservedMemoryMB       int              `json:"reservedMemoryMb"`
	ReservedDiskMB         int              `json:"reservedDiskMb"`
	DefaultAllocationIP    string           `json:"defaultAllocationIp,omitempty"`
	AllocationPortMin      int              `json:"allocationPortMin"`
	AllocationPortMax      int              `json:"allocationPortMax"`
	AutoAllocate           bool             `json:"autoAllocate"`
	BackupDirectory        string           `json:"backupDirectory,omitempty"`
	TransferDirectory      string           `json:"transferDirectory,omitempty"`
	MountPoints            []map[string]any `json:"mountPoints,omitempty"`
	TokenRotationPolicy    string           `json:"tokenRotationPolicy,omitempty"`
	FirewallRules          []map[string]any `json:"firewallRules,omitempty"`
	TLSSetting             string           `json:"tlsSetting,omitempty"`
	EnableHealthChecks     bool             `json:"enableHealthChecks"`
	EnableMetrics          bool             `json:"enableMetrics"`
	PrometheusEndpoint     string           `json:"prometheusEndpoint,omitempty"`
	AlertThresholdCPU      int              `json:"alertThresholdCpu"`
	AlertThresholdMemory   int              `json:"alertThresholdMemory"`
	AlertThresholdDisk     int              `json:"alertThresholdDisk"`
	MaintenanceMessage     string           `json:"maintenanceMessage,omitempty"`
	DrainBeforeMaintenance bool             `json:"drainBeforeMaintenance"`
	Labels                 []LabelPair      `json:"labels,omitempty"`
	ClusterGroupID         string           `json:"clusterGroupId,omitempty"`
	DaemonSFTPAlias        string           `json:"daemonSftpAlias,omitempty"`
	DaemonConnect          int              `json:"daemonConnect"`
	CPUOverallocate        int              `json:"cpuOverallocate"`
	Tags                   []string         `json:"tags,omitempty"`
}

type CreateNodeRequest struct {
	Name                string
	Region              string
	RegionID            string
	LocationID          string
	Description         string
	BaseURL             string
	FQDN                string
	Scheme              string
	BehindProxy         bool
	MemoryMB            int
	DiskMB              int
	UploadSizeMB        int
	DaemonBase          string
	DaemonListen        int
	DaemonSFTP          int
	DisplayName         string
	PublicHostname      string
	Maintenance         bool
	Public              bool
	ListenPortMin       int
	ListenPortMax       int
	AllowedIPs          []string
	NetworkInterface    string
	DaemonSSLCert       string
	DaemonSSLKey        string
	AutoConnect         bool
	ConnectionRetries   int
	HeartbeatInterval   int
	CPUCores            int
	CPUThreads          int
	MemoryOverallocate  int
	DiskOverallocate    int
	ReservedMemoryMB    int
	ReservedDiskMB      int
	DefaultAllocationIP string
	AllocationPortMin   int
	AllocationPortMax   int
	AutoAllocate        bool
	BackupDirectory     string
	TransferDirectory   string
	MountPoints         []map[string]any
	TokenRotationPolicy string
	FirewallRules       []map[string]any
	TLSSetting          string
	EnableHealthChecks  bool
	EnableMetrics       bool
	PrometheusEndpoint  string
	AlertThresholdCPU   int
	AlertThresholdMem   int
	AlertThresholdDisk  int
	MaintenanceMessage  string
	DrainBeforeMaint    bool
	Labels              []LabelPair
	ClusterGroupID      string
	DaemonSFTPAlias     string
	DaemonConnect       int
	CPUOverallocate     int
	Tags                []string
}

type LabelPair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type UpdateNodeRequest struct {
	Name                 string
	Region               string
	RegionID             string
	LocationID           string
	Description          string
	BaseURL              string
	FQDN                 string
	Scheme               string
	BehindProxy          bool
	Public               bool
	Maintenance          bool
	DesiredState         NodeDesiredState
	Draining             bool
	MemoryMB             int
	DiskMB               int
	UploadSizeMB         int
	DaemonBase           string
	DaemonListen         int
	DaemonSFTP           int
	Status               string
	DisplayName          string
	PublicHostname       string
	ListenPortMin        int
	ListenPortMax        int
	AllowedIPs           []string
	NetworkInterface     string
	DaemonSSLCert        string
	DaemonSSLKey         string
	AutoConnect          bool
	ConnectionRetries    int
	HeartbeatInterval    int
	CPUCores             int
	CPUThreads           int
	MemoryOverallocate   int
	DiskOverallocate     int
	ReservedMemoryMB     int
	ReservedDiskMB       int
	DefaultAllocationIP  string
	AllocationPortMin    int
	AllocationPortMax    int
	AutoAllocate         bool
	BackupDirectory      string
	TransferDirectory    string
	MountPoints          []map[string]any
	TokenRotationPolicy  string
	FirewallRules        []map[string]any
	TLSSetting           string
	EnableHealthChecks   bool
	EnableMetrics        bool
	PrometheusEndpoint   string
	AlertThresholdCPU    int
	AlertThresholdMemory int
	AlertThresholdDisk   int
	MaintenanceMessage   string
	DrainBeforeMaint     bool
	Labels               []LabelPair
	ClusterGroupID       string
	DaemonSFTPAlias      string
	DaemonConnect        int
	CPUOverallocate      int
	Tags                 []string
}

// NodePatch represents fields the public PATCH endpoint may change. Pointers preserve
// the difference between an omitted value and an explicit zero/false/empty value.
type NodePatch struct {
	Name               *string
	Description        *string
	LocationID         *string
	BaseURL            *string
	FQDN               *string
	Scheme             *string
	BehindProxy        *bool
	Public             *bool
	Maintenance        *bool
	DesiredState       *NodeDesiredState
	Draining           *bool
	MemoryMB           *int
	DiskMB             *int
	UploadSizeMB       *int
	DaemonBase         *string
	DaemonListen       *int
	DaemonSFTP         *int
	Status             *string
	MemoryOverallocate *int
	DiskOverallocate   *int
	CPUCores           *int
	DisplayName        *string
	PublicHostname     *string
	DaemonSFTPAlias    *string
	DaemonConnect      *int
	CPUOverallocate    *int
	Tags               *[]string
}

type NodeHeartbeatRequest struct {
	Version         string
	OS              string
	Architecture    string
	CPUThreads      int
	MemoryMB        int
	DiskMB          int
	DockerStatus    string
	RuntimeStatus   string
	RuntimeProvider string
	Error           string
}

// Heartbeat types
type NodeHeartbeatState string

// Node Heartbeat and Health History types
type CreateNodeHeartbeatHistoryRequest struct {
	NodeID         string
	Success        bool
	FailureReason  string
	PreviousSeenAt *time.Time
	Version        string
	OS             string
	Architecture   string
	CPUThreads     int
	MemoryMB       int
	DiskMB         int
	RuntimeStatus  string
}

type NodeHeartbeatHistory struct {
	ID             string     `json:"id"`
	NodeID         string     `json:"nodeId"`
	ObservedAt     time.Time  `json:"observedAt"`
	PreviousSeenAt *time.Time `json:"previousSeenAt,omitempty"`
	GapSeconds     *int       `json:"gapSeconds,omitempty"`
	Success        bool       `json:"success"`
	FailureReason  string     `json:"failureReason,omitempty"`
	Version        string     `json:"version,omitempty"`
	OS             string     `json:"os,omitempty"`
	Architecture   string     `json:"architecture,omitempty"`
	CPUThreads     *int       `json:"cpuThreads,omitempty"`
	MemoryMB       *int       `json:"memoryMb,omitempty"`
	DiskMB         *int       `json:"diskMb,omitempty"`
	RuntimeStatus  string     `json:"runtimeStatus,omitempty"`
}

type CreateNodeHealthHistoryRequest struct {
	NodeID          string
	ActualState     string
	DesiredState    string
	HealthScore     float64
	CPUScore        float64
	MemoryScore     float64
	DiskScore       float64
	HeartbeatScore  float64
	StatusScore     float64
	AllocatedCPU    int
	AvailableCPU    int
	AllocatedMemory int64
	AvailableMemory int64
	AllocatedDisk   int64
	AvailableDisk   int64
	ServerCount     int
}

type NodeHealthHistory struct {
	ID              string    `json:"id"`
	NodeID          string    `json:"nodeId"`
	ObservedAt      time.Time `json:"observedAt"`
	ActualState     string    `json:"actualState"`
	DesiredState    string    `json:"desiredState"`
	HealthScore     float64   `json:"healthScore"`
	CPUScore        float64   `json:"cpuScore"`
	MemoryScore     float64   `json:"memoryScore"`
	DiskScore       float64   `json:"diskScore"`
	HeartbeatScore  float64   `json:"heartbeatScore"`
	StatusScore     float64   `json:"statusScore"`
	AllocatedCPU    int       `json:"allocatedCpu"`
	AvailableCPU    int       `json:"availableCpu"`
	AllocatedMemory int64     `json:"allocatedMemory"`
	AvailableMemory int64     `json:"availableMemory"`
	AllocatedDisk   int64     `json:"allocatedDisk"`
	AvailableDisk   int64     `json:"availableDisk"`
	ServerCount     int       `json:"serverCount"`
}

type NodeDesiredState string

type NodeActualState string
