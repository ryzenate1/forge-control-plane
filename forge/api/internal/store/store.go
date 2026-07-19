package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gamepanel/forge/internal/secrets"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type Store struct {
	db      *pgxpool.Pool
	secrets *secrets.Keyring
}

type ScheduleTaskAction string

const (
	ScheduleTaskActionPower   ScheduleTaskAction = "power"
	ScheduleTaskActionBackup  ScheduleTaskAction = "backup"
	ScheduleTaskActionCommand ScheduleTaskAction = "command"
)

type User struct {
	ID              string  `json:"id"`
	ExternalID      string  `json:"externalId"`
	Email           string  `json:"email"`
	Username        string  `json:"username"`
	NameFirst       string  `json:"nameFirst"`
	NameLast        string  `json:"nameLast"`
	Role            string  `json:"role"`
	UseTOTP         bool    `json:"useTotp"`
	TOTPSecret      *string `json:"totpSecret,omitempty"`
	RootAdmin       bool    `json:"rootAdmin"`
	Language        string  `json:"language"`
	CPULimit        int     `json:"cpuLimit"`
	MemoryMBLimit   int     `json:"memoryMbLimit"`
	DiskMBLimit     int     `json:"diskMbLimit"`
	BackupLimit     int     `json:"backupLimit"`
	DatabaseLimit   int     `json:"databaseLimit"`
	AllocationLimit int     `json:"allocationLimit"`
	SubuserLimit    int     `json:"subuserLimit"`
	ScheduleLimit   int     `json:"scheduleLimit"`
	ServerLimit     int     `json:"serverLimit"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
	SessionVersion  int64   `json:"-"`
	Disabled        bool    `json:"disabled"`
}

type ServerSubuser struct {
	ID          string    `json:"id"`
	ServerID    string    `json:"serverId"`
	UserID      string    `json:"userId"`
	Email       string    `json:"email"`
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type UpsertServerSubuserRequest struct {
	Email       string
	Permissions []string
}

type SubuserInvitation struct {
	ID          string     `json:"id"`
	ServerID    string     `json:"serverId"`
	Email       string     `json:"email"`
	Permissions []string   `json:"permissions"`
	Token       string     `json:"token"`
	CreatedBy   *string    `json:"createdBy,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	ExpiresAt   time.Time  `json:"expiresAt"`
	AcceptedAt  *time.Time `json:"acceptedAt,omitempty"`
	RevokedAt   *time.Time `json:"revokedAt,omitempty"`
}

type CreateSubuserInvitationRequest struct {
	Email       string   `json:"email"`
	Permissions []string `json:"permissions"`
}

type SFTPAuthResult struct {
	UserID      string   `json:"user"`
	ServerID    string   `json:"server"`
	Permissions []string `json:"permissions"`
	DiskLimitMB int64    `json:"diskLimitMb"`
	Suspended   bool     `json:"suspended"`
	ReadOnly    bool     `json:"readOnly"`
}

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

type CreateUserRequest struct {
	Email           string
	Password        string
	Role            string
	CPULimit        int
	MemoryMBLimit   int
	DiskMBLimit     int
	BackupLimit     int
	DatabaseLimit   int
	AllocationLimit int
	SubuserLimit    int
	ScheduleLimit   int
	ServerLimit     int
}

type UpdateUserRequest struct {
	Email           string
	Password        string
	Role            string
	CPULimit        *int
	MemoryMBLimit   *int
	DiskMBLimit     *int
	BackupLimit     *int
	DatabaseLimit   *int
	AllocationLimit *int
	SubuserLimit    *int
	ScheduleLimit   *int
	ServerLimit     *int
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

type Allocation struct {
	ID            string  `json:"id"`
	Node          string  `json:"node"`
	Server        *string `json:"server,omitempty"`
	IP            string  `json:"ip"`
	Port          int     `json:"port"`
	ContainerPort int     `json:"containerPort"`
	Protocol      string  `json:"protocol"`
	Alias         *string `json:"alias,omitempty"`
	Notes         string  `json:"notes"`
}

type CreateAllocationRequest struct {
	NodeID        string
	IP            string
	Port          int
	ContainerPort int
	Protocol      string
	Alias         string
	Notes         string
}

type UpdateAllocationRequest struct {
	Alias string
	Notes string
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

// ToDTO converts ServerControlTarget to safe DTO
func (t ServerControlTarget) ToDTO() ServerControlTargetDTO {
	return ServerControlTargetDTO{
		ServerID: t.ServerID,
		NodeURL:  t.NodeURL,
	}
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

// ToDTO converts ServerProvisionTarget to safe DTO
func (t ServerProvisionTarget) ToDTO() ServerProvisionTargetDTO {
	return ServerProvisionTargetDTO{
		ServerID:          t.ServerID,
		EggID:             t.EggID,
		Name:              t.Name,
		NodeURL:           t.NodeURL,
		Image:             t.Image,
		StartupCommand:    t.StartupCommand,
		InstallScript:     t.InstallScript,
		InstallContainer:  t.InstallContainer,
		InstallEntrypoint: t.InstallEntrypoint,
		ConfigJSON:        t.ConfigJSON,
		FileDenylist:      t.FileDenylist,
		Environment:       t.Environment,
		Mounts:            t.Mounts,
		MemoryMB:          t.MemoryMB,
		SwapMB:            t.SwapMB,
		CPUShares:         t.CPUShares,
		CPULimit:          t.CPULimit,
		DiskMB:            t.DiskMB,
		IOWeight:          t.IOWeight,
		Threads:           t.Threads,
		OOMDisabled:       t.OOMDisabled,
		AllocationIP:      t.AllocationIP,
		AllocationPort:    t.AllocationPort,
		Allocations:       t.Allocations,
		Suspended:         t.Suspended,
		Installed:         t.Installed,
		Status:            t.Status,
		SkipScripts:       t.SkipScripts,
		DockerLabels:      t.DockerLabels,
	}
}

type AuditEvent struct {
	ID         string    `json:"id"`
	Action     string    `json:"action"`
	TargetType string    `json:"targetType"`
	TargetID   *string   `json:"targetId,omitempty"`
	Metadata   string    `json:"metadata"`
	CreatedAt  time.Time `json:"createdAt"`
	ActorEmail *string   `json:"actorEmail,omitempty"`
}

type Schedule struct {
	ID             string         `json:"id"`
	ServerID       string         `json:"serverId"`
	Name           string         `json:"name"`
	CronExpression string         `json:"cronExpression"`
	CronMinute     string         `json:"cronMinute"`
	CronHour       string         `json:"cronHour"`
	CronDayOfMonth string         `json:"cronDayOfMonth"`
	CronMonth      string         `json:"cronMonth"`
	CronDayOfWeek  string         `json:"cronDayOfWeek"`
	Timezone       string         `json:"timezone"`
	OnlyWhenOnline bool           `json:"onlyWhenOnline"`
	Enabled        bool           `json:"enabled"`
	Active         bool           `json:"active"`
	LastRunAt      *time.Time     `json:"lastRunAt,omitempty"`
	NextRunAt      *time.Time     `json:"nextRunAt,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	Tasks          []ScheduleTask `json:"tasks"`
}

type ScheduleTask struct {
	ID                string         `json:"id"`
	ScheduleID        string         `json:"scheduleId"`
	Sequence          int            `json:"sequence"`
	Action            string         `json:"action"`
	Payload           map[string]any `json:"payload"`
	TimeOffsetSeconds int            `json:"timeOffsetSeconds"`
	ContinueOnFailure bool           `json:"continueOnFailure"`
	CreatedAt         time.Time      `json:"createdAt"`
}

type StartupVariable struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	EnvVariable  string `json:"env_variable"`
	DefaultValue string `json:"default_value"`
	ServerValue  string `json:"server_value"`
	IsEditable   bool   `json:"is_editable"`
	Rules        string `json:"rules"`
}

type StartupDetails struct {
	StartupCommand    string            `json:"startup_command"`
	RawStartupCommand string            `json:"raw_startup_command"`
	DockerImages      map[string]string `json:"docker_images"`
	Variables         []StartupVariable `json:"variables"`
}

type ServerDatabase struct {
	ID                string  `json:"id"`
	DatabaseName      string  `json:"database"`
	Username          string  `json:"username"`
	Remote            string  `json:"remote"`
	Engine            string  `json:"engine"`
	Host              string  `json:"host"`
	Port              int     `json:"port"`
	MaxConnections    *int    `json:"maxConnections,omitempty"`
	ProvisioningState string  `json:"provisioningState"`
	ProvisioningError string  `json:"provisioningError,omitempty"`
	Password          *string `json:"password,omitempty"`
}

type Backup struct {
	UUID        string     `json:"uuid"`
	ServerID    string     `json:"serverId"`
	Name        string     `json:"name"`
	Checksum    string     `json:"checksum"`
	Size        int64      `json:"size"`
	Status      string     `json:"status"`
	UploadID    *string    `json:"uploadId,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	IsLocked    bool       `json:"isLocked"`
	// Status tracking fields
	StatusMessage  *string    `json:"statusMessage,omitempty"`
	StatusCallback *string    `json:"statusCallback,omitempty"`
	RetryCount     int        `json:"retryCount"`
	LastRetryAt    *time.Time `json:"lastRetryAt,omitempty"`
}

type UpsertBackupRequest struct {
	UUID           string
	Name           string
	Checksum       string
	Size           int64
	Status         string
	UploadID       *string
	CompletedAt    *time.Time
	StatusMessage  *string
	StatusCallback *string
	RetryCount     int
}

type DatabaseHost struct {
	ID            string   `json:"id"`
	NodeID        string   `json:"nodeId,omitempty"`
	NodeIDs       []string `json:"nodeIds,omitempty"`
	NodeName      string   `json:"nodeName,omitempty"`
	NodeNames     []string `json:"nodeNames,omitempty"`
	Engine        string   `json:"engine"`
	Name          string   `json:"name"`
	Host          string   `json:"host"`
	Port          int      `json:"port"`
	Username      string   `json:"username"`
	TLSMode       string   `json:"tlsMode"`
	TLSCA         string   `json:"-"`
	TLSServerName string   `json:"tlsServerName,omitempty"`
	MaxDatabases  *int     `json:"maxDatabases,omitempty"`
	Databases     int      `json:"databases"`
}

type CreateDatabaseHostRequest struct {
	NodeID        string
	NodeIDs       []string
	Engine        string
	Name          string
	Host          string
	Port          int
	Username      string
	Password      string
	TLSMode       string
	TLSCA         string
	TLSServerName string
	MaxDatabases  *int
}

type UpdateDatabaseHostRequest struct {
	NodeID   string
	NodeIDs  []string
	Engine   string
	Name     string
	Host     string
	Port     int
	Username string
	Password string
	TLSMode  string
	// TLSCA is nil when an update omits tlsCa, preserving the configured CA.
	// An empty string explicitly clears it.
	TLSCA         *string
	TLSServerName string
	MaxDatabases  *int
}

type Mount struct {
	ID            string   `json:"id"`
	UUID          string   `json:"uuid"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Source        string   `json:"source"`
	Target        string   `json:"target"`
	ReadOnly      bool     `json:"readOnly"`
	UserMountable bool     `json:"userMountable"`
	NodeIDs       []string `json:"nodeIds"`
	TemplateIDs   []string `json:"templateIds"`
	ServerIDs     []string `json:"serverIds"`
}

type CreateMountRequest struct {
	Name          string
	Description   string
	Source        string
	Target        string
	ReadOnly      bool
	UserMountable bool
	NodeIDs       []string
	TemplateIDs   []string
}

type ServerMount struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Source        string `json:"source"`
	Target        string `json:"target"`
	ReadOnly      bool   `json:"read_only"`
	UserMountable bool   `json:"user_mountable"`
}

type CreateServerDatabaseRequest struct {
	Database       string
	Remote         string
	MaxConnections *int
}

type CreateScheduleRequest struct {
	Name           string
	CronMinute     string
	CronHour       string
	CronDayOfMonth string
	CronMonth      string
	CronDayOfWeek  string
	Timezone       string
	OnlyWhenOnline bool
	Enabled        bool
}

type PatchScheduleRequest struct {
	Name           *string
	CronMinute     *string
	CronHour       *string
	CronDayOfMonth *string
	CronMonth      *string
	CronDayOfWeek  *string
	Timezone       *string
	OnlyWhenOnline *bool
	Enabled        *bool
}

type CreateScheduleTaskRequest struct {
	Sequence          int
	Action            string
	Payload           map[string]any
	TimeOffsetSeconds int
	ContinueOnFailure bool
}

type PatchScheduleTaskRequest struct {
	Sequence          *int
	Action            *string
	Payload           *map[string]any
	TimeOffsetSeconds *int
	ContinueOnFailure *bool
}

type ScheduleRunStatus string

const (
	ScheduleRunRunning ScheduleRunStatus = "running"
	ScheduleRunSuccess ScheduleRunStatus = "success"
	ScheduleRunFailed  ScheduleRunStatus = "failed"
	ScheduleRunPartial ScheduleRunStatus = "partial"
	ScheduleRunSkipped ScheduleRunStatus = "skipped"
)

type ScheduleTaskRunStatus string

const (
	ScheduleTaskRunSuccess ScheduleTaskRunStatus = "success"
	ScheduleTaskRunFailed  ScheduleTaskRunStatus = "failed"
	ScheduleTaskRunSkipped ScheduleTaskRunStatus = "skipped"
)

type ScheduleRun struct {
	ID         string            `json:"id"`
	ScheduleID string            `json:"scheduleId"`
	ServerID   string            `json:"serverId"`
	Status     ScheduleRunStatus `json:"status"`
	Trigger    string            `json:"trigger"`
	Error      *string           `json:"error,omitempty"`
	StartedAt  time.Time         `json:"startedAt"`
	FinishedAt *time.Time        `json:"finishedAt,omitempty"`
	Tasks      []ScheduleTaskRun `json:"tasks"`
}

type ScheduleTaskRun struct {
	ID             string                `json:"id"`
	ScheduleRunID  string                `json:"scheduleRunId"`
	ScheduleTaskID string                `json:"scheduleTaskId"`
	Status         ScheduleTaskRunStatus `json:"status"`
	Error          *string               `json:"error,omitempty"`
	ExecutedAt     time.Time             `json:"executedAt"`
}

func Connect(ctx context.Context, databaseURL string) (*Store, error) {
	return ConnectWithKeyring(ctx, databaseURL, nil)
}

func ConnectWithKeyring(ctx context.Context, databaseURL string, keyring *secrets.Keyring) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 8
	cfg.MinConns = 1
	cfg.MaxConnLifetime = time.Hour

	var lastErr error
	for attempt := 0; attempt < 20; attempt++ {
		pool, err := pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			if pingErr := pool.Ping(ctx); pingErr == nil {
				return &Store{db: pool, secrets: keyring}, nil
			} else {
				lastErr = pingErr
			}
			pool.Close()
		} else {
			lastErr = err
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil, fmt.Errorf("connect postgres: %w", lastErr)
}

func (s *Store) Close() {
	if s != nil && s.db != nil {
		s.db.Close()
	}
}

func (s *Store) DB() *pgxpool.Pool {
	return s.db
}

func isValidScheduleTaskAction(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case string(ScheduleTaskActionPower), string(ScheduleTaskActionBackup), string(ScheduleTaskActionCommand):
		return true
	default:
		return false
	}
}

func (s *Store) RunMigrations(ctx context.Context, dir string) error {
	if _, err := s.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var applied bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, name).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", name, err)
		}
		if applied {
			continue
		}

		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		tx, err := s.db.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", name, err)
		}
		for _, statement := range splitSQLStatements(string(body)) {
			if _, err := tx.Exec(ctx, statement); err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("run migration %s: %w", name, err)
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}
	return nil
}

func (s *Store) Seed(ctx context.Context) error {
	adminID := "11111111-1111-1111-1111-111111111111"
	nodeID := "22222222-2222-2222-2222-222222222222"
	templateID := "33333333-3333-3333-3333-333333333333"
	serverID := "44444444-4444-4444-4444-444444444444"
	allocationID := "55555555-5555-5555-5555-555555555555"
	spareAllocationID := "66666666-6666-6666-6666-666666666666"

	hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if _, err = s.db.Exec(ctx, `
		INSERT INTO users (id, email, password_hash, role)
		VALUES ($1, 'admin@example.com', $2, 'admin')
		ON CONFLICT (email) DO NOTHING
	`, adminID, string(hash)); err != nil {
		return err
	}
	if _, err = s.db.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, r.id FROM roles r WHERE r.key = 'admin'
		ON CONFLICT (user_id, role_id) DO NOTHING
	`, adminID); err != nil {
		return err
	}
	nodeToken := "dev-node-token"
	nodeTokenEncrypted, err := s.encryptSecret(nodeToken, secretAAD("nodes", nodeID, "daemon_token"))
	if err != nil {
		return err
	}
	nodeTokenHash, err := bcrypt.GenerateFromPassword([]byte(nodeToken), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if _, err = s.db.Exec(ctx, `
		INSERT INTO nodes (
			id, uuid, name, region, base_url, fqdn, scheme, status, token_hash,
			daemon_token_id, daemon_token, daemon_token_encrypted, daemon_listen, daemon_sftp, daemon_base, last_seen_at
		)
		VALUES ($1, $1, 'Ubuntu Demo Node', 'local-lab', 'http://daemon:9090', 'daemon', 'http', 'online',
		        $2, 'devnodetoken0001', '', $3, 9090, 2022, '/srv/game-panel/servers', now())
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			token_hash = EXCLUDED.token_hash,
			daemon_token_id = EXCLUDED.daemon_token_id,
			daemon_token = '',
			daemon_token_encrypted = EXCLUDED.daemon_token_encrypted,
			last_seen_at = EXCLUDED.last_seen_at
	`, nodeID, string(nodeTokenHash), nodeTokenEncrypted); err != nil {
		return err
	}
	if _, err = s.db.Exec(ctx, `
		INSERT INTO eggs (id, nest_id, name, description, docker_images, startup, config, default_memory_mb)
		SELECT $1, id, 'Minecraft Java', '', jsonb_build_object('Java', 'itzg/minecraft-server:latest'), '', '{}'::jsonb, 2048
		FROM nests WHERE name = 'Games'
		ON CONFLICT (id) DO UPDATE SET
			docker_images = EXCLUDED.docker_images,
			startup = EXCLUDED.startup,
			default_memory_mb = EXCLUDED.default_memory_mb
	`, templateID); err != nil {
		return err
	}
	if _, err = s.db.Exec(ctx, `
		INSERT INTO servers (id, node_id, owner_id, template_id, egg_id, name, status, memory_mb, cpu_shares, disk_mb)
		VALUES ($1, $2, $3, $4, $4, 'Survival SMP', 'stopped', 2048, 1024, 10240)
		ON CONFLICT (id) DO NOTHING
	`, serverID, nodeID, adminID, templateID); err != nil {
		return err
	}
	if _, err = s.db.Exec(ctx, `
		INSERT INTO allocations (id, node_id, server_id, ip, port, alias, notes)
		VALUES ($1, $2, $3, '0.0.0.0', 25565, 'minecraft.local', 'default Minecraft Java allocation')
		ON CONFLICT (node_id, ip, port) DO UPDATE SET server_id = EXCLUDED.server_id, alias = EXCLUDED.alias
	`, allocationID, nodeID, serverID); err != nil {
		return err
	}
	if _, err = s.db.Exec(ctx, `
		UPDATE servers SET primary_allocation_id = $1 WHERE id = $2 AND (primary_allocation_id IS NULL OR primary_allocation_id != $1)
	`, allocationID, serverID); err != nil {
		return err
	}
	if _, err = s.db.Exec(ctx, `
		INSERT INTO allocations (id, node_id, server_id, ip, port, alias, notes)
		VALUES ($1, $2, NULL, '0.0.0.0', 25566, 'minecraft-alt.local', 'spare Minecraft Java allocation')
		ON CONFLICT (node_id, ip, port) DO NOTHING
	`, spareAllocationID, nodeID); err != nil {
		return err
	}

	return s.AppendAudit(ctx, &adminID, "seeded prototype data", "system", nil, `{"source":"startup"}`)
}

// Evacuation types
type EvacuationPlanStatus string

const (
	EvacuationPlanStatusPending   EvacuationPlanStatus = "pending"
	EvacuationPlanStatusRunning   EvacuationPlanStatus = "running"
	EvacuationPlanStatusCompleted EvacuationPlanStatus = "completed"
	EvacuationPlanStatusCancelled EvacuationPlanStatus = "cancelled"
	EvacuationPlanStatusFailed    EvacuationPlanStatus = "failed"
)

type EvacuationPlan struct {
	ID        string               `json:"id"`
	NodeID    string               `json:"nodeId"`
	Status    EvacuationPlanStatus `json:"status"`
	Items     []EvacuationItem     `json:"items"`
	CreatedAt time.Time            `json:"createdAt"`
	UpdatedAt time.Time            `json:"updatedAt"`
}

type EvacuationItem struct {
	ID           string  `json:"id"`
	PlanID       string  `json:"planId"`
	ServerID     string  `json:"serverId"`
	SourceNodeID string  `json:"sourceNodeId"`
	TargetNodeID string  `json:"targetNodeId,omitempty"`
	Eligible     bool    `json:"eligible"`
	Reason       string  `json:"reason"`
	MigrationID  string  `json:"migrationId,omitempty"`
	Status       string  `json:"status"`
	Error        *string `json:"error,omitempty"`
}

// Heartbeat types
type NodeHeartbeatState string

const (
	NodeHeartbeatStateHealthy     NodeHeartbeatState = "healthy"
	NodeHeartbeatStateSuspected   NodeHeartbeatState = "suspected"
	NodeHeartbeatStateUnreachable NodeHeartbeatState = "unreachable"
	NodeHeartbeatStateOffline     NodeHeartbeatState = "offline"
	NodeHeartbeatStateRecovering  NodeHeartbeatState = "recovering"
)

// Migration types
type CreateMigrationRequest struct {
	ServerID       string
	SourceNodeID   string
	TargetNodeID   string
	InitiatedBy    string
	Priority       int
	TransferMethod string
}

type Migration struct {
	ID              string             `json:"id"`
	ServerID        string             `json:"serverId"`
	SourceNodeID    string             `json:"sourceNodeId"`
	TargetNodeID    string             `json:"targetNodeId"`
	Status          string             `json:"status"`
	InitiatedBy     string             `json:"initiatedBy"`
	Priority        int                `json:"priority"`
	TransferMethod  string             `json:"transferMethod"`
	FailureReason   *string            `json:"failureReason,omitempty"`
	TransferPhase   string             `json:"transferPhase,omitempty"`
	IdempotencyKey  string             `json:"idempotencyKey,omitempty"`
	ArchiveSize     int64              `json:"archiveSize,omitempty"`
	ArchiveChecksum string             `json:"archiveChecksum,omitempty"`
	CleanupPending  bool               `json:"cleanupPending,omitempty"`
	History         []MigrationHistory `json:"history,omitempty"`
	StartedAt       *time.Time         `json:"startedAt,omitempty"`
	CompletedAt     *time.Time         `json:"completedAt,omitempty"`
	Error           *string            `json:"error,omitempty"`
	CreatedAt       time.Time          `json:"createdAt"`
	UpdatedAt       time.Time          `json:"updatedAt"`
}

// Migration Status
type MigrationStatus string

const (
	MigrationStatusPending      MigrationStatus = "pending"
	MigrationStatusPlanned      MigrationStatus = "planned"
	MigrationStatusPreparing    MigrationStatus = "preparing"
	MigrationStatusTransferring MigrationStatus = "transferring"
	MigrationStatusRestoring    MigrationStatus = "restoring"
	MigrationStatusInProgress   MigrationStatus = "in_progress"
	MigrationStatusCompleted    MigrationStatus = "completed"
	MigrationStatusFailed       MigrationStatus = "failed"
	MigrationStatusCancelled    MigrationStatus = "cancelled"
)

type MigrationRun struct {
	MigrationID               string
	ProtocolVersion           string
	Phase                     string
	IdempotencyKey            string
	Attempt                   int
	LeaseOwner                string
	TargetAllocationID        string
	ArchiveSize               int64
	ArchiveChecksum           string
	SourceCredentialHash      string
	DestinationCredentialHash string
	CredentialExpiresAt       *time.Time
	CleanupPending            bool
	LastError                 string
}

type MigrationHistory struct {
	ID           string          `json:"id"`
	MigrationID  string          `json:"migrationId"`
	ServerID     string          `json:"serverId"`
	SourceNodeID string          `json:"sourceNodeId"`
	TargetNodeID string          `json:"targetNodeId"`
	Status       MigrationStatus `json:"status"`
	FromStatus   string          `json:"fromStatus"`
	ToStatus     string          `json:"toStatus"`
	Reason       string          `json:"reason"`
	StartedAt    *time.Time      `json:"startedAt,omitempty"`
	CompletedAt  *time.Time      `json:"completedAt,omitempty"`
	Error        *string         `json:"error,omitempty"`
	CreatedAt    time.Time       `json:"createdAt"`
}

// Observability/Timeline types
type CreateTimelineEventRequest struct {
	EventID       string
	ResourceType  string
	ResourceID    string
	EventType     string
	CorrelationID string
	Source        string
	Payload       map[string]any
	Timestamp     time.Time
}

type TimelineEvent struct {
	ID            string         `json:"id"`
	EventID       string         `json:"eventId,omitempty"`
	ResourceType  string         `json:"resourceType"`
	ResourceID    string         `json:"resourceId"`
	EventType     string         `json:"eventType"`
	CorrelationID string         `json:"correlationId"`
	Source        string         `json:"source"`
	Payload       map[string]any `json:"payload"`
	Timestamp     time.Time      `json:"timestamp"`
}

type TimelineQuery struct {
	ResourceType  string
	ResourceID    string
	CorrelationID string
	Limit         int
}

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

// Recovery Plan types
type RecoveryPlanStatus string

const (
	RecoveryPlanStatusPending   RecoveryPlanStatus = "pending"
	RecoveryPlanStatusPlanning  RecoveryPlanStatus = "planning"
	RecoveryPlanStatusPlanned   RecoveryPlanStatus = "planned"
	RecoveryPlanStatusExecuting RecoveryPlanStatus = "executing"
	RecoveryPlanStatusCompleted RecoveryPlanStatus = "completed"
	// Restored means the verified backup was restored on the target daemon. It
	// deliberately does not imply server ownership or allocation migration.
	RecoveryPlanStatusRestored  RecoveryPlanStatus = "restored"
	RecoveryPlanStatusCancelled RecoveryPlanStatus = "cancelled"
	RecoveryPlanStatusFailed    RecoveryPlanStatus = "failed"
)

type RecoveryPlan struct {
	ID        string             `json:"id"`
	NodeID    string             `json:"nodeId"`
	Status    RecoveryPlanStatus `json:"status"`
	Reason    string             `json:"reason"`
	Items     []RecoveryItem     `json:"items"`
	CreatedAt time.Time          `json:"createdAt"`
	UpdatedAt time.Time          `json:"updatedAt"`
}

type RecoveryStep struct {
	ID          string     `json:"id"`
	PlanID      string     `json:"planId"`
	StepType    string     `json:"stepType"`
	Status      string     `json:"status"`
	Description string     `json:"description"`
	ExecutedAt  *time.Time `json:"executedAt,omitempty"`
	Error       *string    `json:"error,omitempty"`
}

// Recovery Item types
type RecoveryItemStatus string

const (
	RecoveryItemStatusPending   RecoveryItemStatus = "pending"
	RecoveryItemStatusPlanned   RecoveryItemStatus = "planned"
	RecoveryItemStatusExecuting RecoveryItemStatus = "executing"
	RecoveryItemStatusCompleted RecoveryItemStatus = "completed"
	// Restored means backup data was restored, but no live migration was run.
	RecoveryItemStatusRestored  RecoveryItemStatus = "restored"
	RecoveryItemStatusCancelled RecoveryItemStatus = "cancelled"
	RecoveryItemStatusFailed    RecoveryItemStatus = "failed"
	RecoveryItemStatusSkipped   RecoveryItemStatus = "skipped"
)

type RecoveryItem struct {
	ID                   string    `json:"id"`
	PlanID               string    `json:"planId"`
	ServerID             string    `json:"serverId"`
	SourceNodeID         string    `json:"sourceNodeId"`
	TargetNodeID         string    `json:"targetNodeId,omitempty"`
	ReservationID        string    `json:"reservationId,omitempty"`
	MigrationID          string    `json:"migrationId,omitempty"`
	SourceBackupName     string    `json:"sourceBackupName,omitempty"`
	SourceBackupChecksum string    `json:"sourceBackupChecksum,omitempty"`
	SourceBackupSize     int64     `json:"sourceBackupSize,omitempty"`
	Status               string    `json:"status"`
	Reason               string    `json:"reason,omitempty"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

// Placement Reservation types
type PlacementReservationStatus string

const (
	PlacementReservationStatusPending   PlacementReservationStatus = "pending"
	PlacementReservationStatusActive    PlacementReservationStatus = "active"
	PlacementReservationStatusCompleted PlacementReservationStatus = "completed"
	PlacementReservationStatusExpired   PlacementReservationStatus = "expired"
	PlacementReservationStatusCancelled PlacementReservationStatus = "cancelled"
	PlacementReservationStatusCanceled  PlacementReservationStatus = "canceled"
	PlacementReservationStatusUsed      PlacementReservationStatus = "used"
)

type PlacementReservationType string

const (
	PlacementReservationTypePlacement  PlacementReservationType = "placement"
	PlacementReservationTypeMigration  PlacementReservationType = "migration"
	PlacementReservationTypeEvacuation PlacementReservationType = "evacuation"
	PlacementReservationTypeRecovery   PlacementReservationType = "recovery"
)

type CreatePlacementReservationRequest struct {
	ServerID        string
	NodeID          string
	MigrationID     string
	ReservationType PlacementReservationType
	Status          PlacementReservationStatus
	ReservedBy      string
	CPU             int
	Memory          int64
	Disk            int64
	ExpiresAt       time.Time
}

type PlacementReservation struct {
	ID              string                     `json:"id"`
	ServerID        *string                    `json:"serverId,omitempty"`
	NodeID          string                     `json:"nodeId"`
	MigrationID     *string                    `json:"migrationId,omitempty"`
	ReservationType string                     `json:"reservationType"`
	Status          PlacementReservationStatus `json:"status"`
	ReservedBy      string                     `json:"reservedBy"`
	CPU             int                        `json:"cpu"`
	Memory          int64                      `json:"memory"`
	Disk            int64                      `json:"disk"`
	ExpiresAt       time.Time                  `json:"expiresAt"`
	ConfirmedAt     *time.Time                 `json:"confirmedAt,omitempty"`
	CancelledAt     *time.Time                 `json:"cancelledAt,omitempty"`
	ExpiredAt       *time.Time                 `json:"expiredAt,omitempty"`
	UsedAt          *time.Time                 `json:"usedAt,omitempty"`
	CreatedAt       time.Time                  `json:"createdAt"`
	UpdatedAt       time.Time                  `json:"updatedAt"`
}

// Reserved Capacity type
type ReservedCapacity struct {
	NodeID   string `json:"nodeId"`
	CPU      int    `json:"cpu"`
	Memory   int    `json:"memory"`
	Disk     int    `json:"disk"`
	Reserved int    `json:"reserved"`
}

// Desired State types
type ServerDesiredState string

const (
	ServerDesiredStateRunning    ServerDesiredState = "running"
	ServerDesiredStateStopped    ServerDesiredState = "stopped"
	ServerDesiredStateTerminated ServerDesiredState = "terminated"
)

type ServerActualState string

const (
	ServerActualStateOffline         ServerActualState = "offline"
	ServerActualStateStarting        ServerActualState = "starting"
	ServerActualStateRunning         ServerActualState = "running"
	ServerActualStateStopping        ServerActualState = "stopping"
	ServerActualStateStopped         ServerActualState = "stopped"
	ServerActualStateInstalling      ServerActualState = "installing"
	ServerActualStateRestoringBackup ServerActualState = "restoring_backup"
	ServerActualStateCrashed         ServerActualState = "crashed"
	ServerActualStateTerminating     ServerActualState = "terminating"
	ServerActualStateTerminated      ServerActualState = "terminated"
	ServerActualStateUnknown         ServerActualState = "unknown"
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
	NodeActualStateDegraded NodeActualState = "degraded"
	NodeActualStateOffline  NodeActualState = "offline"
)
