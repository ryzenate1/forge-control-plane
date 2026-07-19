package store

import (
	"time"
)

type Template struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Image             string `json:"image"`
	StartupCommand    string `json:"startupCommand"`
	DefaultMemoryMB   int    `json:"defaultMemoryMb"`
	InstallScript     string `json:"installScript,omitempty"`
	InstallContainer  string `json:"installContainer,omitempty"`
	InstallEntrypoint string `json:"installEntrypoint,omitempty"`
	ConfigJSON        string `json:"configJson,omitempty"`
	FileDenylist      string `json:"fileDenylist,omitempty"`
}

type CreateTemplateRequest struct {
	Name            string
	Image           string
	StartupCommand  string
	DefaultMemoryMB int
}

type Server struct {
	ID                   string             `json:"id"`
	ExternalID           string             `json:"externalId"`
	Uuid                 string             `json:"uuid"`
	UuidShort            string             `json:"uuidShort"`
	Name                 string             `json:"name"`
	Description          string             `json:"description"`
	Status               string             `json:"status"`
	DesiredState         ServerDesiredState `json:"desiredState"`
	ActualState          ServerActualState  `json:"actualState"`
	Suspended            bool               `json:"suspended"`
	Transferring         bool               `json:"transferring"`
	TransferTargetNodeID *string            `json:"transferTargetNodeId,omitempty"`
	TransferState        string             `json:"transferState"`
	TransferError        *string            `json:"transferError,omitempty"`
	TransferRunToken     *string            `json:"transferRunToken,omitempty"`
	MemoryMB             int                `json:"memoryMb"`
	CPUShares            int                `json:"cpuShares"`
	CPULimit             int                `json:"cpuLimit"`
	DiskMB               int                `json:"diskMb"`
	DatabaseLimit        int                `json:"databaseLimit"`
	BackupLimit          int                `json:"backupLimit"`
	AllocationLimit      int                `json:"allocationLimit"`
	IOWeight             int                `json:"ioWeight"`
	SwapMB               int                `json:"swapMb"`
	Threads              string             `json:"threads"`
	OOMDisabled          bool               `json:"oomDisabled"`
	DockerImage          string             `json:"dockerImage"`
	StartupCommand       string             `json:"startupCommand"`
	PrimaryAllocationID  *string            `json:"primaryAllocationId,omitempty"`
	ConfigSyncPending    bool               `json:"configSyncPending"`
	ConfigSyncError      *string            `json:"configSyncError,omitempty"`
	Node                 string             `json:"node"`
	NodeID               string             `json:"nodeId,omitempty"`
	SFTPHost             string             `json:"sftpHost,omitempty"`
	SFTPPort             int                `json:"sftpPort,omitempty"`
	Owner                string             `json:"owner"`
	OwnerID              string             `json:"ownerId,omitempty"`
	Template             string             `json:"template"`
	InstalledAt          *time.Time         `json:"installedAt,omitempty"`
	SkipScripts          bool               `json:"skipScripts"`
	DockerLabels         map[string]string  `json:"dockerLabels,omitempty"`
	Permissions          []string           `json:"permissions,omitempty"`
}

type UpdateServerRequest struct {
	Name                *string
	Description         *string
	OwnerID             *string
	MemoryMB            *int
	CPUShares           *int
	CPULimit            *int
	DiskMB              *int
	DatabaseLimit       *int
	BackupLimit         *int
	AllocationLimit     *int
	IOWeight            *int
	SwapMB              *int
	Threads             *string
	OOMDisabled         *bool
	DockerImage         *string
	StartupCommand      *string
	PrimaryAllocationID *string
}

type CreateServerRequest struct {
	Name                    string
	NodeID                  string
	OwnerID                 string
	TemplateID              string
	AllocationID            string
	AdditionalAllocationIDs []string
	MemoryMB                int
	CPUShares               int
	CPULimit                int
	DiskMB                  int
	DatabaseLimit           int
	BackupLimit             int
	AllocationLimit         int
	IOWeight                int
	SwapMB                  int
	Threads                 string
	OOMDisabled             bool
	DockerImage             string
	StartupCommand          string
	StartupVariables        map[string]string
	SkipScripts             bool
	DockerLabels            map[string]string
}

type ServerControlTarget struct {
	ServerID  string
	NodeURL   string
	NodeToken string
}

// ServerControlTargetDTO is a safe DTO for API responses (excludes NodeToken)
type ServerControlTargetDTO struct {
	ServerID string
	NodeURL  string
}

type ServerRuntimeAllocation struct {
	ID            string `json:"id"`
	IP            string `json:"ip"`
	Port          int    `json:"port"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

type ServerProvisionTarget struct {
	ServerID          string
	EggID             string
	Name              string
	NodeURL           string
	NodeToken         string
	Image             string
	StartupCommand    string
	InstallScript     string
	InstallContainer  string
	InstallEntrypoint string
	ConfigJSON        string
	FileDenylist      string
	Environment       map[string]string
	Mounts            []ServerMount
	MemoryMB          int64
	SwapMB            int64
	CPUShares         int64
	CPULimit          int64
	DiskMB            int64
	IOWeight          int64
	Threads           string
	OOMDisabled       bool
	AllocationIP      string
	AllocationPort    int
	Allocations       []ServerRuntimeAllocation
	Suspended         bool
	Installed         bool
	Status            string
	SkipScripts       bool
	DockerLabels      map[string]string
}

// ServerProvisionTargetDTO is a safe DTO for API responses (excludes NodeToken)
type ServerProvisionTargetDTO struct {
	ServerID          string
	EggID             string
	Name              string
	NodeURL           string
	Image             string
	StartupCommand    string
	InstallScript     string
	InstallContainer  string
	InstallEntrypoint string
	ConfigJSON        string
	FileDenylist      string
	Environment       map[string]string
	Mounts            []ServerMount
	MemoryMB          int64
	SwapMB            int64
	CPUShares         int64
	CPULimit          int64
	DiskMB            int64
	IOWeight          int64
	Threads           string
	OOMDisabled       bool
	AllocationIP      string
	AllocationPort    int
	Allocations       []ServerRuntimeAllocation
	Suspended         bool
	Installed         bool
	Status            string
	SkipScripts       bool
	DockerLabels      map[string]string
}

// Desired State types
type ServerDesiredState string

type ServerActualState string
