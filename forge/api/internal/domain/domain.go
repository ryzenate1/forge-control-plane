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
	NodeActualStateOnline   NodeActualState = "online"
	NodeActualStateOffline  NodeActualState = "offline"
	NodeActualStateDegraded NodeActualState = "degraded"
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
	ServerID      string `json:"serverId,omitempty"`
	RegionID      string `json:"regionId,omitempty"`
	Region        string `json:"region,omitempty"`
	PreferredNode string `json:"preferredNode,omitempty"`
	RequiredNode  string `json:"requiredNode,omitempty"`
	NodeID        string `json:"nodeId,omitempty"`
	AllocationID  string `json:"allocationId,omitempty"`
	MemoryMB      int    `json:"memoryMb,omitempty"`
	CPUShares     int    `json:"cpuShares,omitempty"`
	CPU           int    `json:"cpu,omitempty"`
	DiskMB        int    `json:"diskMb,omitempty"`
}

type PlacementDecision struct {
	RegionID     string   `json:"regionId,omitempty"`
	RegionIDRaw  string   `json:"region_id,omitempty"`
	NodeID       string   `json:"nodeId"`
	NodeIDRaw    string   `json:"node_id"`
	AllocationID string   `json:"allocationId,omitempty"`
	Manual       bool     `json:"manual"`
	Score        float64  `json:"score"`
	Reasons      []string `json:"reasons"`
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
