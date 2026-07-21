package domain

import "time"

type Cluster struct {
	ID        string
	Name      string
	Regions   []Region
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Region struct {
	ID          string    `json:"id"`
	UUID        string    `json:"uuid"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type NodeStatus string

const (
	NodeStatusOnline      NodeStatus = "online"
	NodeStatusOffline     NodeStatus = "offline"
	NodeStatusDegraded    NodeStatus = "degraded"
	NodeStatusMaintenance NodeStatus = "maintenance"
	NodeStatusDraining    NodeStatus = "draining"
)

type NodeDesiredState string

const (
	NodeDesiredStateActive      NodeDesiredState = "active"
	NodeDesiredStateMaintenance NodeDesiredState = "maintenance"
	NodeDesiredStateDraining    NodeDesiredState = "draining"
)

type NodeActualState string

const (
	NodeActualStateOnline       NodeActualState = "online"
	NodeActualStateOffline      NodeActualState = "offline"
	NodeActualStateDegraded     NodeActualState = "degraded"
	NodeActualStateReconciling  NodeActualState = "reconciling"
)

type NodeHealth struct {
	CPU     string `json:"cpu"`
	Memory  string `json:"memory"`
	Disk    string `json:"disk"`
	Network string `json:"network"`
	Runtime string `json:"runtime"`
}

type NodeCapacity struct {
	CPUThreads int `json:"cpuThreads"`
	MemoryMB   int `json:"memoryMb"`
	DiskMB     int `json:"diskMb"`
}

type NodeCapacitySnapshot struct {
	NodeID          string    `json:"nodeId"`
	RegionID        string    `json:"regionId,omitempty"`
	AllocatedCPU    int       `json:"allocated_cpu"`
	AvailableCPU    int       `json:"available_cpu"`
	AllocatedMemory int       `json:"allocated_memory"`
	AvailableMemory int       `json:"available_memory"`
	AllocatedDisk   int       `json:"allocated_disk"`
	AvailableDisk   int       `json:"available_disk"`
	ServerCount     int       `json:"server_count"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Node struct {
	ID              string           `json:"id"`
	UUID            string           `json:"uuid"`
	Name            string           `json:"name"`
	RegionID        *string          `json:"regionId,omitempty"`
	Status          NodeStatus       `json:"status"`
	DesiredState    NodeDesiredState `json:"desiredState"`
	ActualState     NodeActualState  `json:"actualState"`
	Health          NodeHealth       `json:"health"`
	Capacity        NodeCapacity     `json:"capacity"`
	RuntimeProvider string           `json:"runtimeProvider,omitempty"`
}

type Server struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	RegionID     *string            `json:"regionId,omitempty"`
	NodeID       string             `json:"nodeId"`
	DesiredState ServerDesiredState `json:"desiredState"`
	ActualState  ServerActualState  `json:"actualState"`
}

type Allocation struct {
	ID       string  `json:"id"`
	NodeID   string  `json:"nodeId"`
	ServerID *string `json:"serverId,omitempty"`
	IP       string  `json:"ip"`
	Port     int     `json:"port"`
}

type Runtime struct {
	Kind         string   `json:"kind"`
	Version      string   `json:"version,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
}

type PlacementRequest struct {
	ServerID        string `json:"serverId,omitempty"`
	RegionID        string `json:"regionId,omitempty"`
	Region          string `json:"region,omitempty"`
	PreferredNode   string `json:"preferredNode,omitempty"`
	RequiredNode    string `json:"requiredNode,omitempty"`
	NodeID          string `json:"nodeId,omitempty"`
	AllocationID    string `json:"allocationId,omitempty"`
	SkipReservation bool   `json:"skipReservation,omitempty"`
	StorageLocality string `json:"storageLocality,omitempty"`
	MemoryMB        int    `json:"memoryMb,omitempty"`
	CPUShares       int    `json:"cpuShares,omitempty"`
	CPU             int    `json:"cpu,omitempty"`
	DiskMB          int    `json:"diskMb,omitempty"`
}

type PlacementDecision struct {
	RegionID      string   `json:"regionId,omitempty"`
	RegionIDRaw   string   `json:"region_id,omitempty"`
	NodeID        string   `json:"nodeId"`
	NodeIDRaw     string   `json:"node_id"`
	AllocationID  string   `json:"allocationId,omitempty"`
	ReservationID string   `json:"reservationId,omitempty"`
	Manual        bool     `json:"manual"`
	Score         float64  `json:"score"`
	Reasons       []string `json:"reasons"`
}

type ReplicaApplication struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Replicas  int       `json:"replicas"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type InstanceStatus string

const (
	InstanceStatusPending   InstanceStatus = "pending"
	InstanceStatusProvision InstanceStatus = "provisioning"
	InstanceStatusRunning   InstanceStatus = "running"
	InstanceStatusStopped   InstanceStatus = "stopped"
	InstanceStatusFailed    InstanceStatus = "failed"
	InstanceStatusRemoving  InstanceStatus = "removing"
)

type Instance struct {
	ID              string         `json:"id"`
	AppID           string         `json:"appId"`
	Index           int            `json:"index"`
	NodeID          string         `json:"nodeId"`
	Status          InstanceStatus `json:"status"`
	CPU             int            `json:"cpu"`
	MemoryMB        int            `json:"memoryMb"`
	DiskMB          int            `json:"diskMb"`
	AllocationID    *string        `json:"allocationId,omitempty"`
	PlacementID     string         `json:"placementId"`
	ReservationID   *string        `json:"reservationId,omitempty"`
	RuntimeProvider string         `json:"runtimeProvider,omitempty"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

type PlacementReason struct {
	InstanceID string   `json:"instanceId"`
	NodeID     string   `json:"nodeId"`
	Index      int      `json:"index"`
	Score      float64  `json:"score"`
	Accepted   bool     `json:"accepted"`
	Reasons    []string `json:"reasons"`
}

type PlaceReplicasRequest struct {
	AppID         string `json:"appId"`
	ReplicaCount  int    `json:"replicaCount"`
	CPU           int    `json:"cpu"`
	MemoryMB      int    `json:"memoryMb"`
	DiskMB        int    `json:"diskMb"`
	RegionID      string `json:"regionId,omitempty"`
	RequiredNode  string `json:"requiredNode,omitempty"`
	PreferredNode string `json:"preferredNode,omitempty"`
	RuntimeFilter string `json:"runtimeFilter,omitempty"`
}

type ScaleRequest struct {
	AppID        string `json:"appId"`
	ReplicaCount int    `json:"replicaCount"`
}

type ReplaceFailedInstanceRequest struct {
	InstanceID string `json:"instanceId"`
}

type ServerDesiredState string

const (
	ServerDesiredStateRunning ServerDesiredState = "running"
	ServerDesiredStateStopped ServerDesiredState = "stopped"
)

type ServerActualState string

const (
	ServerActualStateRunning    ServerActualState = "running"
	ServerActualStateStopped    ServerActualState = "stopped"
	ServerActualStateStarting   ServerActualState = "starting"
	ServerActualStateStopping   ServerActualState = "stopping"
	ServerActualStateInstalling ServerActualState = "installing"
	ServerActualStateCrashed    ServerActualState = "crashed"
	ServerActualStateUnknown    ServerActualState = "unknown"
)

type EvacuationPlanStatus string

const (
	EvacuationPlanStatusPending   EvacuationPlanStatus = "pending"
	EvacuationPlanStatusRunning   EvacuationPlanStatus = "running"
	EvacuationPlanStatusCompleted EvacuationPlanStatus = "completed"
	EvacuationPlanStatusFailed    EvacuationPlanStatus = "failed"
)

type EvacuationPlan struct {
	ID        string               `json:"id"`
	NodeID    string               `json:"nodeId"`
	Status    EvacuationPlanStatus `json:"status"`
	CreatedAt time.Time            `json:"createdAt"`
	UpdatedAt time.Time            `json:"updatedAt"`
}

type EvacuationItem struct {
	ServerID     string `json:"serverId"`
	SourceNodeID string `json:"sourceNodeId"`
	TargetNodeID string `json:"targetNodeId,omitempty"`
	Eligible     bool   `json:"eligible"`
	Reason       string `json:"reason,omitempty"`
}

type MigrationStatus string

const (
	MigrationStatusPlanned      MigrationStatus = "planned"
	MigrationStatusPreparing    MigrationStatus = "preparing"
	MigrationStatusTransferring MigrationStatus = "transferring"
	MigrationStatusRestoring    MigrationStatus = "restoring"
	MigrationStatusCompleted    MigrationStatus = "completed"
	MigrationStatusFailed       MigrationStatus = "failed"
	MigrationStatusCancelled    MigrationStatus = "cancelled"
)

type Migration struct {
	ID           string          `json:"id"`
	ServerID     string          `json:"serverId"`
	SourceNodeID string          `json:"sourceNodeId"`
	TargetNodeID string          `json:"targetNodeId"`
	Status       MigrationStatus `json:"status"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
}

type EndpointType string

const (
	EndpointTypeDocker EndpointType = "docker"
	EndpointTypeSwarm  EndpointType = "swarm"
	EndpointTypeK8s    EndpointType = "kubernetes"
	EndpointTypeEdge   EndpointType = "edge"
)

type ConnectionMode string

const (
	ConnectionModeDirect ConnectionMode = "direct"
	ConnectionModeTunnel ConnectionMode = "tunnel"
	ConnectionModeEdge   ConnectionMode = "edge"
)

type EndpointStatus string

const (
	EndpointStatusUnknown    EndpointStatus = "unknown"
	EndpointStatusOnline     EndpointStatus = "online"
	EndpointStatusDegraded   EndpointStatus = "degraded"
	EndpointStatusOffline    EndpointStatus = "offline"
	EndpointStatusProvisioning EndpointStatus = "provisioning"
)

type Endpoint struct {
	ID                string               `json:"id"`
	Name              string               `json:"name"`
	Description       string               `json:"description"`
	EndpointType      EndpointType          `json:"endpointType"`
	ConnectionMode    ConnectionMode        `json:"connectionMode"`
	Status            EndpointStatus        `json:"status"`
	EdgeID            string               `json:"edgeId,omitempty"`
	Tags              []string             `json:"tags"`
	Labels            []LabelPair           `json:"labels"`
	URL               string               `json:"url,omitempty"`
	ProjectID         string               `json:"projectId,omitempty"`
	GroupID           string               `json:"groupId,omitempty"`
	Reachable         bool                 `json:"reachable"`
	Version           string               `json:"version,omitempty"`
	NodeCount         int                  `json:"nodeCount"`
	TotalContainers   int                  `json:"totalContainers"`
	TotalImages       int                  `json:"totalImages"`
	TotalVolumes      int                  `json:"totalVolumes"`
	CreatedAt         time.Time            `json:"createdAt"`
	UpdatedAt         time.Time            `json:"updatedAt"`
}

type LabelPair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type EndpointDiagnostics struct {
	EndpointID    string        `json:"endpointId"`
	Reachable     bool          `json:"reachable"`
	Version       string        `json:"version,omitempty"`
	TotalMemoryMB int64         `json:"totalMemoryMb,omitempty"`
	UsedMemoryMB  int64         `json:"usedMemoryMb,omitempty"`
	TotalDiskMB   int64         `json:"totalDiskMb,omitempty"`
	UsedDiskMB    int64         `json:"usedDiskMb,omitempty"`
	CPUPercent    float64       `json:"cpuPercent,omitempty"`
	Nodes         []NodeSummary `json:"nodes"`
	CheckedAt     time.Time     `json:"checkedAt"`
}

type NodeSummary struct {
	NodeID       string `json:"nodeId"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	ServerCount  int    `json:"serverCount"`
	AllocatedMem int    `json:"allocatedMemMb"`
	AllocatedCPU int    `json:"allocatedCpu"`
	AllocatedDisk int   `json:"allocatedDiskMb"`
}

type InventorySummary struct {
	TotalServers   int `json:"totalServers"`
	TotalContainers int `json:"totalContainers"`
	TotalImages    int `json:"totalImages"`
	TotalVolumes   int `json:"totalVolumes"`
	TotalAllocations int `json:"totalAllocations"`
	UsedMemoryMB   int64 `json:"usedMemoryMb"`
	TotalMemoryMB  int64 `json:"totalMemoryMb"`
	UsedDiskMB     int64 `json:"usedDiskMb"`
	TotalDiskMB    int64 `json:"totalDiskMb"`
}

type HealthRecord struct {
	ID          string         `json:"id"`
	EndpointID  string         `json:"endpointId"`
	Status      EndpointStatus `json:"status"`
	Reachable   bool           `json:"reachable"`
	HealthScore float64        `json:"healthScore"`
	Version     string         `json:"version,omitempty"`
	Containers  int            `json:"containers"`
	Images      int            `json:"images"`
	Volumes     int            `json:"volumes"`
	Error       string         `json:"error,omitempty"`
	ObservedAt  time.Time      `json:"observedAt"`
}

type AccessPolicy struct {
	ID            string    `json:"id"`
	EndpointID    string    `json:"endpointId"`
	PrincipalType string    `json:"principalType"`
	PrincipalID   string    `json:"principalId"`
	Role          string    `json:"role"`
	CreatedAt     time.Time `json:"createdAt"`
}

type CreateEndpointRequest struct {
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	EndpointType   EndpointType   `json:"endpointType"`
	ConnectionMode ConnectionMode `json:"connectionMode"`
	NodeIDs        []string       `json:"nodeIds"`
	Tags           []string       `json:"tags"`
	Labels         []LabelPair    `json:"labels"`
	URL            string         `json:"url"`
	ProjectID      string         `json:"projectId"`
	GroupID        string         `json:"groupId"`
	EdgeID         string         `json:"edgeId"`
	TLSConfig      *TLSConfig     `json:"tlsConfig,omitempty"`
}

type UpdateEndpointRequest struct {
	Name           *string         `json:"name,omitempty"`
	Description    *string         `json:"description,omitempty"`
	EndpointType   *EndpointType   `json:"endpointType,omitempty"`
	ConnectionMode *ConnectionMode `json:"connectionMode,omitempty"`
	Tags           *[]string       `json:"tags,omitempty"`
	Labels         *[]LabelPair    `json:"labels,omitempty"`
	URL            *string         `json:"url,omitempty"`
	ProjectID      *string         `json:"projectId,omitempty"`
	GroupID        *string         `json:"groupId,omitempty"`
	TLSConfig      *TLSConfig      `json:"tlsConfig,omitempty"`
}

type TLSConfig struct {
	CACert  string `json:"caCert"`
	TLSCert string `json:"tlsCert"`
	TLSKey  string `json:"tlsKey"`
}
