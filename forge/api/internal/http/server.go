package http

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	stdruntime "runtime"
	"strconv"
	"strings"
	"time"

	"gamepanel/forge/internal/auth"
	"gamepanel/forge/internal/cloud"
	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/eventstore"
	"gamepanel/forge/internal/services"
	acmesvc "gamepanel/forge/internal/services/acme"
	"gamepanel/forge/internal/services/activity"
	alerting "gamepanel/forge/internal/services/alerting"
	apphostingsvc "gamepanel/forge/internal/services/apphosting"
	appstoresvc "gamepanel/forge/internal/services/appstore"
	"gamepanel/forge/internal/services/auditlog"
	"gamepanel/forge/internal/services/autoscaler"
	"gamepanel/forge/internal/services/backup"
	"gamepanel/forge/internal/services/build"
	buildpacksvc "gamepanel/forge/internal/services/buildpack"
	cleanupsvc "gamepanel/forge/internal/services/cleanup"
	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/services/clustermembership"
	composesvc "gamepanel/forge/internal/services/compose"
	"gamepanel/forge/internal/services/crashdetector"
	cronjobsvc "gamepanel/forge/internal/services/cronjob"
	"gamepanel/forge/internal/services/crossnode"
	dbbackupsvc "gamepanel/forge/internal/services/dbbackup"
	"gamepanel/forge/internal/services/dbprovisioner"
	"gamepanel/forge/internal/services/deployment"
	dnssvc "gamepanel/forge/internal/services/dns"
	"gamepanel/forge/internal/services/domains"
	"gamepanel/forge/internal/services/environments"
	envvarsvc "gamepanel/forge/internal/services/envvars"
	"gamepanel/forge/internal/services/evacuationplanner"
	"gamepanel/forge/internal/services/failover"
	gitsvc "gamepanel/forge/internal/services/git"
	"gamepanel/forge/internal/services/gitprovider"
	"gamepanel/forge/internal/services/health"
	"gamepanel/forge/internal/services/heartbeatmonitor"
	"gamepanel/forge/internal/services/i18n"
	"gamepanel/forge/internal/services/loadbalancer"
	mailservice "gamepanel/forge/internal/services/mail"
	"gamepanel/forge/internal/services/migration"
	"gamepanel/forge/internal/services/nodeprobe"
	"gamepanel/forge/internal/services/noderegistry"
	notificationsvc "gamepanel/forge/internal/services/notification"
	enhancednotifsvc "gamepanel/forge/internal/services/notifications"
	"gamepanel/forge/internal/services/observability"
	operationsvc "gamepanel/forge/internal/services/operation"
	"gamepanel/forge/internal/services/plugins"
	previewsvc "gamepanel/forge/internal/services/preview"
	proceduresvc "gamepanel/forge/internal/services/procedure"
	processsvc "gamepanel/forge/internal/services/process"
	"gamepanel/forge/internal/services/queue"
	"gamepanel/forge/internal/services/reconciler"
	recoverysvc "gamepanel/forge/internal/services/recovery"
	replicamanager "gamepanel/forge/internal/services/replicamanager"
	"gamepanel/forge/internal/services/reservations"
	runtimesvc "gamepanel/forge/internal/services/runtime"
	"gamepanel/forge/internal/services/scheduler"
	"gamepanel/forge/internal/services/servicediscovery"
	"gamepanel/forge/internal/services/tenancy"
	"gamepanel/forge/internal/services/trafficmanager"
	"gamepanel/forge/internal/services/webauthn"
	"gamepanel/forge/internal/services/zerodowntime"
	"gamepanel/forge/internal/store"

	fiberws "github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	fiberrecover "github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type readinessResponse struct {
	Status    string               `json:"status"`
	Checks    []health.CheckResult `json:"checks"`
	CheckedAt time.Time            `json:"checkedAt"`
}

type Config struct {
	Addr              string
	ReadTimeout       time.Duration
	AuthSecret        string
	Store             *store.Store
	Redis             *redis.Client
	RedisEnabled      bool
	Daemon            *daemon.Client
	BackgroundContext context.Context
	// PanelURL is the publicly reachable base URL of the web UI used to
	// construct links emailed to users.
	PanelURL string
	// PluginsDir is the on-disk directory where installed plugin manifests
	// are written. If empty, plugins are tracked in the database only.
	PluginsDir string

	PluginService *plugins.Service

	// Services — built in main and passed in via Config so NewServer stays
	// free of complex dependency wiring.
	ActivityService      *activity.Service
	AuditLogService      auditlog.AuditLogger
	ClusterManager       *clustermanager.Service
	EvacuationPlanner    *evacuationplanner.Service
	MigrationService     *migration.Service
	ReservationManager   *reservations.Manager
	RecoveryCoordinator  *recoverysvc.Coordinator
	RecoveryTokenService *recoverysvc.TokenService
	HeartbeatMonitor     *heartbeatmonitor.Service
	Observability        *observability.Service
	Reconciler           *reconciler.Service
	NodeRegistry         *noderegistry.Service
	NodeProbe            *nodeprobe.Service
	DBProvisioner        *dbprovisioner.Service
	Translator           *i18n.TranslationService
	HealthService        *health.Service
	SessionStore         auth.SessionStore
	MailTriggerService   *mailservice.TriggerService
	QueueService         *queue.Service
	OperationService     *operationsvc.Service
	RuntimeRegistry      *runtimesvc.Registry
	WebAuthnService      *webauthn.Service
	EventRelay           *eventstore.Relay

	BackupSvc                  *backup.Service
	AutoScaler                 *autoscaler.Service
	CrashDetector              *crashdetector.Detector
	DeploymentSvc              *deployment.Service
	PreviewDeploymentSvc       *previewsvc.Service
	CloudManager               *cloud.Manager
	AcmeService                *acmesvc.Service
	LoadBalancer               *loadbalancer.Service
	FailoverSvc                *failover.Service
	TrafficManager             *trafficmanager.Service
	DomainService              *domains.Service
	DNSService                 *dnssvc.Service
	DBContainerService         *dbprovisioner.DBContainerService
	DatabaseServiceProvisioner *services.DatabaseServiceProvisioner
	DBBackupService            *dbbackupsvc.Service
	PredictiveScorer           *scheduler.PredictiveScorer
	ConstraintScheduler        *scheduler.ConstraintScheduler
	GitService                 *gitsvc.Service
	GitDeployService           *gitsvc.DeployService
	GitDeployMgmtService       *gitsvc.DeploymentManagementService
	GitProviderService         *gitprovider.Service
	ComposeService             *composesvc.Service
	BuildService               *build.Service
	BuildpackService           *buildpacksvc.Service

	TenancyService  *tenancy.Service
	EnvVarService   *envvarsvc.Service
	EndpointService *environments.Service

	AppHostingService *apphostingsvc.Service

	ProcedureService *proceduresvc.Service
	ReplicaManager   *replicamanager.Manager
	AppStoreService  *appstoresvc.Service

	AlertService              *alerting.Service
	NotificationService       *notificationsvc.Service
	EnhancedNotificationService *enhancednotifsvc.Service
	CronJobService            *cronjobsvc.Service
	ProcessService      *processsvc.Service
	ZeroDowntimeSvc     *zerodowntime.Service

	ClusterMembershipService *clustermembership.Service
	CleanupService           *cleanupsvc.Service

	// Cross-node routing services
	ServiceDiscovery    *servicediscovery.Service
	CrossNodeResolver   *crossnode.Resolver
	IngressSynchronizer *crossnode.IngressSynchronizer

	MTLSEnabled    bool
	MTLSCACertPath string
	MTLSCertPath   string
	MTLSKeyPath    string
	MTLSDevBypass  bool
	CertService    *services.CertService
	MTLSMigrator   *services.MTLSMigrator

	Logger     *slog.Logger
	CORSConfig CORSConfig
}

type PowerRequest struct {
	Signal string `json:"signal"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func loginRateLimitKey(prefix, value string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(value))))
	return "login:" + prefix + ":" + hex.EncodeToString(sum[:])
}

func checkLoginRateLimit(ctx context.Context, cfg Config, c *fiber.Ctx, email string) error {
	if cfg.Redis == nil || !cfg.RedisEnabled {
		return nil
	}
	keys := []string{
		loginRateLimitKey("ip", c.IP()),
		loginRateLimitKey("email", email),
	}
	for _, key := range keys {
		count, err := cfg.Redis.Get(ctx, key).Int()
		if err == nil && count >= 5 {
			return fiber.NewError(fiber.StatusTooManyRequests, "too many login attempts; try again later")
		}
		if err != nil && err != redis.Nil {
			continue
		}
	}
	return nil
}

func recordLoginFailure(ctx context.Context, cfg Config, c *fiber.Ctx, email string) {
	if cfg.Redis == nil || !cfg.RedisEnabled {
		return
	}
	keys := []string{
		loginRateLimitKey("ip", c.IP()),
		loginRateLimitKey("email", email),
	}
	for _, key := range keys {
		count, err := cfg.Redis.Incr(ctx, key).Result()
		if err == nil && count == 1 {
			_ = cfg.Redis.Expire(ctx, key, time.Minute).Err()
		}
	}
}

func clearLoginFailures(ctx context.Context, cfg Config, c *fiber.Ctx, email string) {
	if cfg.Redis == nil || !cfg.RedisEnabled {
		return
	}
	_ = cfg.Redis.Del(ctx, loginRateLimitKey("ip", c.IP()), loginRateLimitKey("email", email)).Err()
}

type CreateServerRequest struct {
	Name                    string            `json:"name" validate:"required"`
	NodeID                  string            `json:"nodeId"`
	RegionID                string            `json:"regionId"`
	Region                  string            `json:"region"`
	RequiredNode            string            `json:"requiredNode"`
	PreferredNode           string            `json:"preferredNode"`
	OwnerID                 string            `json:"ownerId" validate:"required"`
	TemplateID              string            `json:"templateId"`
	AllocationID            string            `json:"allocationId"`
	AdditionalAllocationIDs []string          `json:"additionalAllocationIds"`
	MemoryMB                *int              `json:"memoryMb"`
	CPUShares               *int              `json:"cpuShares"`
	CPU                     *int              `json:"cpu"`
	DiskMB                  *int              `json:"diskMb"`
	DatabaseLimit           *int              `json:"databaseLimit"`
	BackupLimit             *int              `json:"backupLimit"`
	AllocationLimit         *int              `json:"allocationLimit"`
	IOWeight                *int              `json:"ioWeight"`
	SwapMB                  *int              `json:"swapMb"`
	Threads                 string            `json:"threads"`
	OOMDisabled             bool              `json:"oomDisabled"`
	DockerImage             string            `json:"dockerImage"`
	StartupCommand          string            `json:"startupCommand"`
	StartupVariables        map[string]string `json:"startupVariables"`
}

type CreateUserRequest struct {
	Email           string `json:"email" validate:"required,email"`
	Password        string `json:"password" validate:"required"`
	Role            string `json:"role"`
	CPULimit        int    `json:"cpuLimit"`
	MemoryMBLimit   int    `json:"memoryMbLimit"`
	DiskMBLimit     int    `json:"diskMbLimit"`
	BackupLimit     int    `json:"backupLimit"`
	DatabaseLimit   int    `json:"databaseLimit"`
	AllocationLimit int    `json:"allocationLimit"`
	SubuserLimit    int    `json:"subuserLimit"`
	ScheduleLimit   int    `json:"scheduleLimit"`
	ServerLimit     int    `json:"serverLimit"`
}

type CreateTemplateRequest struct {
	Name            string `json:"name" validate:"required"`
	Image           string `json:"image" validate:"required"`
	StartupCommand  string `json:"startupCommand"`
	DefaultMemoryMB int    `json:"defaultMemoryMb"`
}

type CreateAllocationRequest struct {
	NodeID        string `json:"nodeId" validate:"required"`
	IP            string `json:"ip"`
	Port          int    `json:"port"`
	Ports         string `json:"ports"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
	Alias         string `json:"alias"`
	Notes         string `json:"notes"`
}

type UpdateAllocationRequest struct {
	Alias string `json:"alias"`
	Notes string `json:"notes"`
}

type RenameFileRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type UpdateServerRequest struct {
	Name            *string `json:"name"`
	Description     *string `json:"description"`
	OwnerID         *string `json:"ownerId"`
	MemoryMB        *int    `json:"memoryMb"`
	CPUShares       *int    `json:"cpuShares"`
	CPULimit        *int    `json:"cpuLimit"`
	DiskMB          *int    `json:"diskMb"`
	DatabaseLimit   *int    `json:"databaseLimit"`
	BackupLimit     *int    `json:"backupLimit"`
	AllocationLimit *int    `json:"allocationLimit"`
	IOWeight        *int    `json:"ioWeight"`
	SwapMB          *int    `json:"swapMb"`
	Threads         *string `json:"threads"`
	OOMDisabled     *bool   `json:"oomDisabled"`
	DockerImage     *string `json:"dockerImage"`
	StartupCommand  *string `json:"startupCommand"`
	PrimaryAlloc    *string `json:"primaryAllocationId"`
}

type UpdateUserRequest struct {
	Email           string `json:"email"`
	Password        string `json:"password"`
	Role            string `json:"role"`
	CPULimit        *int   `json:"cpuLimit"`
	MemoryMBLimit   *int   `json:"memoryMbLimit"`
	DiskMBLimit     *int   `json:"diskMbLimit"`
	BackupLimit     *int   `json:"backupLimit"`
	DatabaseLimit   *int   `json:"databaseLimit"`
	AllocationLimit *int   `json:"allocationLimit"`
	SubuserLimit    *int   `json:"subuserLimit"`
	ScheduleLimit   *int   `json:"scheduleLimit"`
	ServerLimit     *int   `json:"serverLimit"`
}

type TransferServerRequest struct {
	TargetNodeID            string   `json:"targetNodeId"`
	PrimaryAllocationID     string   `json:"primaryAllocationId"`
	AdditionalAllocationIDs []string `json:"additionalAllocationIds"`
}

type CreateScheduleRequest struct {
	Name           string `json:"name"`
	CronMinute     string `json:"cronMinute"`
	CronHour       string `json:"cronHour"`
	CronDayOfMonth string `json:"cronDayOfMonth"`
	CronMonth      string `json:"cronMonth"`
	CronDayOfWeek  string `json:"cronDayOfWeek"`
	OnlyWhenOnline bool   `json:"onlyWhenOnline"`
	Enabled        bool   `json:"enabled"`
}

type PatchScheduleRequest struct {
	Name           *string `json:"name"`
	CronMinute     *string `json:"cronMinute"`
	CronHour       *string `json:"cronHour"`
	CronDayOfMonth *string `json:"cronDayOfMonth"`
	CronMonth      *string `json:"cronMonth"`
	CronDayOfWeek  *string `json:"cronDayOfWeek"`
	OnlyWhenOnline *bool   `json:"onlyWhenOnline"`
	Enabled        *bool   `json:"enabled"`
}

type CreateScheduleTaskRequest struct {
	Sequence          int            `json:"sequence"`
	Action            string         `json:"action"`
	Payload           map[string]any `json:"payload"`
	TimeOffsetSeconds int            `json:"timeOffsetSeconds"`
	ContinueOnFailure bool           `json:"continueOnFailure"`
}

type PatchScheduleTaskRequest struct {
	Sequence          *int            `json:"sequence"`
	Action            *string         `json:"action"`
	Payload           *map[string]any `json:"payload"`
	TimeOffsetSeconds *int            `json:"timeOffsetSeconds"`
	ContinueOnFailure *bool           `json:"continueOnFailure"`
}

type UpdateStartupVariableRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type CreateServerDatabaseRequest struct {
	Database       string `json:"database" validate:"required"`
	Remote         string `json:"remote" validate:"required"`
	MaxConnections *int   `json:"maxConnections"`
}

type CreateDatabaseHostRequest struct {
	NodeID        string   `json:"nodeId"`
	NodeIDs       []string `json:"nodeIds"`
	Engine        string   `json:"engine" validate:"required"`
	Name          string   `json:"name" validate:"required"`
	Host          string   `json:"host" validate:"required"`
	Port          int      `json:"port"`
	Username      string   `json:"username" validate:"required"`
	Password      string   `json:"password" validate:"required"`
	TLSMode       string   `json:"tlsMode"`
	TLSCA         string   `json:"tlsCa"`
	TLSServerName string   `json:"tlsServerName"`
	MaxDatabases  *int     `json:"maxDatabases"`
}

type CreateMountRequest struct {
	Name          string   `json:"name" validate:"required"`
	Description   string   `json:"description"`
	Source        string   `json:"source" validate:"required"`
	Target        string   `json:"target" validate:"required"`
	ReadOnly      bool     `json:"readOnly"`
	UserMountable bool     `json:"userMountable"`
	NodeIDs       []string `json:"nodeIds"`
	TemplateIDs   []string `json:"templateIds"`
}

type AssignMountRequest struct {
	MountID string `json:"mountId"`
}

type UpsertSubuserRequest struct {
	Email       string   `json:"email"`
	Permissions []string `json:"permissions"`
}

type CreateLocationRequest struct {
	Short string `json:"short" validate:"required"`
	Long  string `json:"long" validate:"required"`
}

type UpdateLocationRequest struct {
	Short string `json:"short"`
	Long  string `json:"long"`
}

type CreateNestRequest struct {
	Name        string `json:"name" validate:"required"`
	Description string `json:"description"`
}

type UpdateNestRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type CreateEggRequest struct {
	NestID            string          `json:"nestId" validate:"required"`
	Name              string          `json:"name" validate:"required"`
	Description       string          `json:"description"`
	DockerImages      json.RawMessage `json:"dockerImages"`
	Startup           string          `json:"startup"`
	Config            json.RawMessage `json:"config"`
	DefaultMemoryMB   int             `json:"defaultMemoryMb"`
	InstallScript     string          `json:"installScript"`
	InstallContainer  string          `json:"installContainer"`
	InstallEntrypoint string          `json:"installEntrypoint"`
	FileDenylist      json.RawMessage `json:"fileDenylist"`
	ConfigFrom        *string         `json:"configFrom,omitempty"`
	CopyScriptFrom    *string         `json:"copyScriptFrom,omitempty"`
	UpdateURL         string          `json:"updateUrl"`
	Author            string          `json:"author"`
	Features          json.RawMessage `json:"features"`
	StartupCommands   json.RawMessage `json:"startupCommands"`
}

type UpdateEggRequest struct {
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	DockerImages      json.RawMessage `json:"dockerImages"`
	Startup           string          `json:"startup"`
	Config            json.RawMessage `json:"config"`
	DefaultMemoryMB   int             `json:"defaultMemoryMb"`
	InstallScript     string          `json:"installScript"`
	InstallContainer  string          `json:"installContainer"`
	InstallEntrypoint string          `json:"installEntrypoint"`
	FileDenylist      json.RawMessage `json:"fileDenylist"`
	ConfigFrom        *string         `json:"configFrom,omitempty"`
	CopyScriptFrom    *string         `json:"copyScriptFrom,omitempty"`
	UpdateURL         string          `json:"updateUrl"`
	Author            string          `json:"author"`
	Features          json.RawMessage `json:"features"`
	StartupCommands   json.RawMessage `json:"startupCommands"`
}

type EggVariableRequest struct {
	Name         string `json:"name" validate:"required"`
	Description  string `json:"description"`
	EnvVariable  string `json:"envVariable" validate:"required"`
	DefaultValue string `json:"defaultValue"`
	UserViewable bool   `json:"userViewable"`
	UserEditable bool   `json:"userEditable"`
	Rules        string `json:"rules"`
	Sort         int    `json:"sort"`
}

type CreateNodeRequest struct {
	Name                string           `json:"name" validate:"required"`
	Region              string           `json:"region"`
	RegionID            string           `json:"regionId"`
	Description         string           `json:"description"`
	LocationID          string           `json:"locationId"`
	BaseURL             string           `json:"baseUrl"`
	FQDN                string           `json:"fqdn"`
	Scheme              string           `json:"scheme"`
	BehindProxy         bool             `json:"behindProxy"`
	Public              *bool            `json:"public"`
	MaintenanceMode     *bool            `json:"maintenanceMode"`
	MemoryMB            int              `json:"memoryMb"`
	DiskMB              int              `json:"diskMb"`
	UploadSizeMB        int              `json:"uploadSizeMb"`
	DaemonBase          string           `json:"daemonBase"`
	DaemonListen        int              `json:"daemonListen"`
	DaemonSFTP          int              `json:"daemonSftp"`
	MemoryOverallocate  *int             `json:"memoryOverallocate"`
	DiskOverallocate    *int             `json:"diskOverallocate"`
	CPUCores            *int             `json:"cpuCores"`
	DisplayName         string           `json:"displayName"`
	PublicHostname      string           `json:"publicHostname"`
	DaemonSFTPAlias     string           `json:"daemonSftpAlias"`
	DaemonConnect       *int             `json:"daemonConnect"`
	CPUOverallocate     *int             `json:"cpuOverallocate"`
	Tags                []string         `json:"tags"`
	SchedulerType       string           `json:"schedulerType,omitempty"`
	SchedulerConfig     *json.RawMessage `json:"schedulerConfig,omitempty"`
	AllowedIPs          []string         `json:"allowedIps,omitempty"`
	NetworkInterface    string           `json:"networkInterface,omitempty"`
	ReservedMemoryMB    *int             `json:"reservedMemoryMb,omitempty"`
	ReservedDiskMB      *int             `json:"reservedDiskMb,omitempty"`
	DefaultAllocationIP string           `json:"defaultAllocationIp,omitempty"`
	AllocationPortMin   *int             `json:"allocationPortMin,omitempty"`
	AllocationPortMax   *int             `json:"allocationPortMax,omitempty"`
	AutoAllocate        *bool            `json:"autoAllocate,omitempty"`
	EnableHealthChecks  *bool            `json:"enableHealthChecks,omitempty"`
	EnableMetrics       *bool            `json:"enableMetrics,omitempty"`
	PrometheusEndpoint  string           `json:"prometheusEndpoint,omitempty"`
	AlertThresholdCPU   *int             `json:"alertThresholdCpu,omitempty"`
	AlertThresholdMem   *int             `json:"alertThresholdMemory,omitempty"`
	AlertThresholdDisk  *int             `json:"alertThresholdDisk,omitempty"`
	MaintenanceMessage  string           `json:"maintenanceMessage,omitempty"`
	DrainBeforeMaint    *bool            `json:"drainBeforeMaintenance,omitempty"`
	TokenRotationPolicy string           `json:"tokenRotationPolicy,omitempty"`
	TLSSetting          string           `json:"tlsSetting,omitempty"`
}

// UpdateNodeRequest is a true PATCH DTO. Pointers retain explicit false, zero,
// and empty-string values while omitted fields remain untouched.
type UpdateNodeRequest struct {
	Name               *string                 `json:"name"`
	Description        *string                 `json:"description"`
	LocationID         *string                 `json:"locationId"`
	BaseURL            *string                 `json:"baseUrl"`
	FQDN               *string                 `json:"fqdn"`
	Scheme             *string                 `json:"scheme"`
	BehindProxy        *bool                   `json:"behindProxy"`
	Public             *bool                   `json:"public"`
	Maintenance        *bool                   `json:"maintenanceMode"`
	DesiredState       *store.NodeDesiredState `json:"desiredState"`
	Draining           *bool                   `json:"draining"`
	MemoryMB           *int                    `json:"memoryMb"`
	DiskMB             *int                    `json:"diskMb"`
	UploadSizeMB       *int                    `json:"uploadSizeMb"`
	DaemonBase         *string                 `json:"daemonBase"`
	DaemonListen       *int                    `json:"daemonListen"`
	DaemonSFTP         *int                    `json:"daemonSftp"`
	Status             *string                 `json:"status"`
	MemoryOverallocate *int                    `json:"memoryOverallocate"`
	DiskOverallocate   *int                    `json:"diskOverallocate"`
	CPUCores           *int                    `json:"cpuCores"`
	DisplayName        *string                 `json:"displayName"`
	PublicHostname     *string                 `json:"publicHostname"`
	DaemonSFTPAlias    *string                 `json:"daemonSftpAlias"`
	DaemonConnect      *int                    `json:"daemonConnect"`
	CPUOverallocate    *int                    `json:"cpuOverallocate"`
	Tags               *[]string               `json:"tags"`
	SchedulerType      *string                 `json:"schedulerType,omitempty"`
	SchedulerConfig    *json.RawMessage        `json:"schedulerConfig,omitempty"`
}

type NodeHeartbeatRequest struct {
	Version         string `json:"version"`
	OS              string `json:"os"`
	Architecture    string `json:"architecture"`
	CPUThreads      int    `json:"cpuThreads"`
	MemoryMB        int    `json:"memoryMb"`
	DiskMB          int    `json:"diskMb"`
	DockerStatus    string `json:"dockerStatus,omitempty"`
	RuntimeStatus   string `json:"runtimeStatus"`
	RuntimeProvider string `json:"runtimeProvider"`
	Error           string `json:"error"`
}

type CreateRegionRequest struct {
	Name        string `json:"name" validate:"required"`
	Slug        string `json:"slug" validate:"required"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type UpdateRegionRequest struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type UpdateDatabaseHostRequest struct {
	NodeID        string   `json:"nodeId"`
	NodeIDs       []string `json:"nodeIds"`
	Engine        string   `json:"engine"`
	Name          string   `json:"name"`
	Host          string   `json:"host"`
	Port          int      `json:"port"`
	Username      string   `json:"username"`
	Password      string   `json:"password"`
	TLSMode       string   `json:"tlsMode"`
	TLSCA         *string  `json:"tlsCa"`
	TLSServerName string   `json:"tlsServerName"`
	MaxDatabases  *int     `json:"maxDatabases"`
}

func NewServer(cfg Config) *fiber.App {
	started := time.Now()
	runner := newScheduleRunner(cfg)

	if cfg.HealthService != nil {
		cfg.HealthService.AddCheck(health.NewQueueCheck("Queue Worker", runner.Health))
	}

	// WebSocket ticket store (in-memory; tickets are short-lived and single-use).
	wsTickets := newWSTicketStore(cfg)
	fileDownloadTickets := newFileDownloadTicketStore()

	// Services are fully constructed in main.go and injected via Config.
	nodeRegistry := cfg.NodeRegistry
	nodeProbe := cfg.NodeProbe
	clusterManager := cfg.ClusterManager
	evacuationPlanner := cfg.EvacuationPlanner
	migrationService := cfg.MigrationService
	reservationManager := cfg.ReservationManager
	recoveryCoordinator := cfg.RecoveryCoordinator

	app := fiber.New(fiber.Config{
		AppName:           "modern-game-panel-api",
		ReadTimeout:       cfg.ReadTimeout,
		StreamRequestBody: true,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			var e *fiber.Error
			if errors.As(err, &e) {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		},
	})

	// Panic recovery middleware - catches panics and returns 500 with stack trace
	app.Use(fiberrecover.New(fiberrecover.Config{EnableStackTrace: true}))

	// CORS middleware — registered before any route-specific middleware so that
	// preflight (OPTIONS) requests and CORS headers are applied to every route
	// including swagger, well-known, and health endpoints.
	corsCfg := cfg.CORSConfig
	if len(corsCfg.AllowedOrigins) == 0 {
		if raw := os.Getenv("API_CORS_ALLOWED_ORIGINS"); raw != "" {
			corsCfg.AllowedOrigins = parseAllowedOrigins(raw)
			corsCfg.AllowMethods = "GET,POST,PUT,PATCH,DELETE,OPTIONS"
			corsCfg.AllowHeaders = "Origin,Content-Type,Accept,Authorization,X-CSRF-Token,X-Forge-Session-Mode"
			corsCfg.AllowCredentials = true
			corsCfg.MaxAge = 86400
		} else {
			corsCfg = DefaultCORSConfig()
		}
	}
	app.Use(CORSMiddleware(corsCfg))

	// Maintenance mode middleware - checks FORGE_MAINTENANCE_MODE env var
	app.Use(MaintenanceModeMiddleware(cfg))

	// Security headers middleware - prevents XSS, clickjacking, MIME sniffing
	// Added as part of comprehensive security audit fixes
	app.Use(SecurityHeaders())

	if cfg.Logger != nil {
		app.Use(StructuredLogger(cfg.Logger))
	}

	registerSwaggerRoutes(app)

	registerWellKnownVerifyRoute(app, cfg.DomainService)

	// Internationalization middleware
	if cfg.Translator != nil {
		app.Use(I18nMiddleware(cfg.Translator))
	}

	// Create rate limiters for different endpoint types
	// Added as part of comprehensive security audit fixes
	authLimiter := RateLimiter(GetRateLimitForEndpoint("auth", cfg.Redis))
	mutationLimiter := RateLimiter(GetRateLimitForEndpoint("mutation", cfg.Redis))
	readLimiter := RateLimiter(GetRateLimitForEndpoint("read", cfg.Redis))

	// Create IP access control middleware
	// Added as part of comprehensive security audit fixes
	adminIPAccess := IPAccessControl(AdminIPAccessConfig(cfg))
	apiIPAccess := IPAccessControl(APIIPAccessConfig(cfg))

	// mTLS authentication middleware (no-op when disabled)
	mtlsCfg := MTLSAuthConfig{
		Enabled:    cfg.MTLSEnabled,
		CACertPath: cfg.MTLSCACertPath,
		CertPath:   cfg.MTLSCertPath,
		KeyPath:    cfg.MTLSKeyPath,
		DevBypass:  cfg.MTLSDevBypass,
	}
	mtlsMw := MTLSAuthMiddleware(mtlsCfg)

	v1 := app.Group("/api/v1", apiIPAccess, mtlsMw)
	v1.Get("/panel/settings/public", func(c *fiber.Ctx) error {
		settings := defaultPanelSettings()
		if cfg.Store != nil {
			ctx, cancel := requestContext()
			defer cancel()
			if stored, err := cfg.Store.GetPanelSettings(ctx); err == nil {
				settings = stored
			}
		}
		resp := fiber.Map{
			"companyName":        settings.CompanyName,
			"shortName":          settings.ShortName,
			"productName":        settings.ProductName,
			"browserTitle":       settings.BrowserTitle,
			"footerText":         settings.FooterText,
			"logoUrl":            settings.LogoURL,
			"faviconUrl":         settings.FaviconURL,
			"loginBackgroundUrl": settings.LoginBackgroundURL,
			"themePreset":        settings.ThemePreset,
			"defaultLocale":      settings.DefaultLocale,
		}
		if cfg.Store != nil {
			ctx, cancel := requestContext()
			defer cancel()
			if providers, err := cfg.Store.GetEnabledSocialProviders(ctx); err == nil {
				social := make([]fiber.Map, 0, len(providers))
				for _, p := range providers {
					social = append(social, fiber.Map{
						"name":        p.Name,
						"displayName": p.DisplayName,
						"buttonStyle": p.ButtonStyle,
						"iconClass":   p.IconClass,
					})
				}
				resp["socialProviders"] = social
			}
		}
		return c.JSON(resp)
	})

	// Available locales endpoint
	v1.Get("/i18n/locales", func(c *fiber.Ctx) error {
		if cfg.Translator == nil {
			return c.JSON([]string{"en"})
		}
		return c.JSON(cfg.Translator.AvailableLocales())
	})

	// Translation file endpoint — serves locale JSON for the frontend
	v1.Get("/i18n/:locale", func(c *fiber.Ctx) error {
		locale := c.Params("locale")
		if len(locale) != 2 {
			return fiber.NewError(fiber.StatusBadRequest, "invalid locale")
		}
		langsDir := "lang"
		path := langsDir + "/" + locale + ".json"
		if _, err := os.Stat(path); os.IsNotExist(err) {
			// Fallback to English
			path = langsDir + "/en.json"
			if _, err := os.Stat(path); os.IsNotExist(err) {
				return fiber.NewError(fiber.StatusNotFound, "translation not found")
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		var jsonData map[string]any
		if err := json.Unmarshal(data, &jsonData); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(jsonData)
	})

	// CSRF token endpoint for clients to fetch CSRF token
	v1.Get("/csrf-token", GetCSRFTokenHandler())
	// Session exchange endpoint — exchanges a single-use code for a session token.
	// Used after social auth redirects to avoid placing the token in the URL.
	v1.Post("/auth/session/exchange", ExchangeCodeHandler())

	// Liveness only confirms that this API process can serve requests; it does
	// not probe external dependencies and therefore remains safe for restarts.
	v1.Get("/health/live", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})
	// Readiness reports unavailable only when a configured critical dependency
	// fails. Warnings and non-critical diagnostic failures do not evict the API.
	v1.Get("/health/ready", func(c *fiber.Ctx) error {
		if cfg.HealthService == nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "not_ready"})
		}
		report := cfg.HealthService.RunAll(c.Context())
		response := readinessResponse{
			Status:    "ready",
			Checks:    report.Checks,
			CheckedAt: report.CheckedAt,
		}
		if !report.Ready() {
			response.Status = "not_ready"
			return c.Status(fiber.StatusServiceUnavailable).JSON(response)
		}
		return c.JSON(response)
	})
	// Diagnostics always return their structured report with HTTP 200. Consumers
	// should use its explicit status field; automated health checks use readiness.
	v1.Get("/health", func(c *fiber.Ctx) error {
		if cfg.HealthService != nil {
			report := cfg.HealthService.RunAll(c.Context())
			return c.JSON(report)
		}
		// Keep the diagnostics response shape stable even when monitoring has not
		// been configured yet. The Monitoring Center can then distinguish an empty
		// report from an unavailable endpoint without relying on mock data.
		return c.JSON(health.HealthReport{
			Status:    health.StatusOK,
			OK:        true,
			Service:   "api",
			Checks:    []health.CheckResult{},
			CheckedAt: time.Now(),
		})
	})
	if cfg.HealthService != nil {
		v1.Get("/health/:check", func(c *fiber.Ctx) error {
			result := cfg.HealthService.RunCheck(c.Context(), c.Params("check"))
			if result == nil {
				return fiber.NewError(fiber.StatusNotFound, "unknown check")
			}
			return c.JSON(result)
		})
		v1.Get("/health/:check/history", func(c *fiber.Ctx) error {
			history := cfg.HealthService.GetCheckHistory(c.Params("check"))
			return c.JSON(history)
		})
	}
	v1.Get("/metrics", func(c *fiber.Ctx) error {
		var mem stdruntime.MemStats
		stdruntime.ReadMemStats(&mem)
		redisEnabled := 0
		if cfg.RedisEnabled {
			redisEnabled = 1
		}
		postgresEnabled := 0
		if cfg.Store != nil {
			postgresEnabled = 1
		}
		c.Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		return c.SendString(
			"# HELP game_panel_api_up API process is serving the metrics endpoint, 1 when available.\n" +
				"# TYPE game_panel_api_up gauge\n" +
				"game_panel_api_up 1\n" +
				"# HELP game_panel_api_uptime_seconds API process uptime.\n" +
				"# TYPE game_panel_api_uptime_seconds gauge\n" +
				"game_panel_api_uptime_seconds " + strconv.FormatFloat(time.Since(started).Seconds(), 'f', 3, 64) + "\n" +
				"# HELP game_panel_api_redis_enabled Redis configuration status, 1 when enabled.\n" +
				"# TYPE game_panel_api_redis_enabled gauge\n" +
				"game_panel_api_redis_enabled " + strconv.Itoa(redisEnabled) + "\n" +
				"# HELP game_panel_api_postgres_enabled Postgres configuration status, 1 when enabled.\n" +
				"# TYPE game_panel_api_postgres_enabled gauge\n" +
				"game_panel_api_postgres_enabled " + strconv.Itoa(postgresEnabled) + "\n" +
				"# HELP game_panel_api_goroutines Current goroutine count.\n" +
				"# TYPE game_panel_api_goroutines gauge\n" +
				"game_panel_api_goroutines " + strconv.Itoa(stdruntime.NumGoroutine()) + "\n" +
				"# HELP game_panel_api_memory_alloc_bytes Current Go heap allocation.\n" +
				"# TYPE game_panel_api_memory_alloc_bytes gauge\n" +
				"game_panel_api_memory_alloc_bytes " + strconv.FormatUint(mem.Alloc, 10) + "\n",
		)
	})

	v1.Post("/nodes/:id/heartbeat", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		token := strings.TrimPrefix(c.Get("Authorization"), "Bearer ")
		if token == c.Get("Authorization") {
			token = c.Get("X-Node-Token")
		}
		ctx, cancel := requestContext()
		defer cancel()
		ok, err := cfg.Store.VerifyNodeToken(ctx, c.Params("id"), token)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "token verification failed")
		}
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid node token")
		}
		var req NodeHeartbeatRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		node, err := cfg.Store.UpdateNodeHeartbeat(ctx, c.Params("id"), store.NodeHeartbeatRequest{
			Version:         req.Version,
			OS:              req.OS,
			Architecture:    req.Architecture,
			CPUThreads:      req.CPUThreads,
			MemoryMB:        req.MemoryMB,
			DiskMB:          req.DiskMB,
			DockerStatus:    req.DockerStatus,
			RuntimeStatus:   req.RuntimeStatus,
			RuntimeProvider: req.RuntimeProvider,
			Error:           req.Error,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "node": node})
	})

	type CheckpointRequest struct {
		ConfirmationToken string `json:"confirmationToken"`
		Code              string `json:"code"`
		RecoveryToken     string `json:"recoveryToken"`
	}

	v1.Post("/auth/login", authLimiter, CaptchaMiddleware(cfg), func(c *fiber.Ctx) error {
		var req LoginRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		user, err := cfg.Store.Authenticate(ctx, req.Email, req.Password)
		if err != nil {
			recordLoginFailure(ctx, cfg, c, req.Email)
			_ = cfg.Store.AppendAudit(ctx, nil, "login.failed", "user", nil, safeAuditMeta(map[string]string{"email": req.Email}))
			return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
		}
		clearLoginFailures(ctx, cfg, c, req.Email)
		_ = cfg.Store.AppendAudit(ctx, &user.ID, "login.success", "user", &user.ID, "{}")

		if user.UseTOTP {
			confToken, err := issue2FAConfirmationToken(cfg.AuthSecret, user.ID)
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "could not issue confirmation token")
			}
			return c.JSON(fiber.Map{
				"complete":          false,
				"confirmationToken": confToken,
			})
		}

		token, err := issueToken(cfg.AuthSecret, user)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not issue token")
		}

		// Always set HttpOnly session and CSRF cookies for browser clients
		csrfToken, err := generateCSRFToken()
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not generate CSRF token")
		}
		expires := time.Now().Add(tokenTTL)
		setSessionCookies(c, token, csrfToken, expires)

		return c.JSON(fiber.Map{
			"complete": true,
			"user":     user,
		})
	})

	v1.Post("/auth/login/checkpoint", authLimiter, func(c *fiber.Ctx) error {
		var req CheckpointRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.ConfirmationToken == "" {
			return fiber.NewError(fiber.StatusBadRequest, "missing confirmation token")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}

		userID, err := parse2FAConfirmationToken(cfg.AuthSecret, req.ConfirmationToken)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, err.Error())
		}

		ctx, cancel := requestContext()
		defer cancel()

		err = cfg.Store.VerifyTwoFactorCheckpoint(ctx, userID, req.Code, req.RecoveryToken)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, err.Error())
		}

		user, err := cfg.Store.GetUserByID(ctx, userID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not retrieve user details")
		}

		token, err := issueToken(cfg.AuthSecret, user)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not issue token")
		}

		// Always set HttpOnly session and CSRF cookies for browser clients
		csrfToken, err := generateCSRFToken()
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not generate CSRF token")
		}
		expires := time.Now().Add(tokenTTL)
		setSessionCookies(c, token, csrfToken, expires)

		return c.JSON(fiber.Map{
			"complete": true,
			"user":     user,
		})
	})

	// Session refresh – re-issue cookie with new expiry
	v1.Post("/auth/session/refresh", func(c *fiber.Ctx) error {
		sessionToken, ok := getSessionCookie(c)
		if !ok || sessionToken == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session cookie")
		}
		claims, err := parseToken(cfg.AuthSecret, sessionToken)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid session token")
		}
		ctx, cancel := requestContext()
		defer cancel()
		current, err := validateCurrentSession(ctx, cfg.Store, claims)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid or revoked session")
		}
		newToken, err := issueToken(cfg.AuthSecret, store.User{ID: current.Sub, Email: current.Email, Role: current.Role, SessionVersion: current.SessionVersion})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not issue session token")
		}
		csrfToken, _ := generateCSRFToken()
		expires := time.Now().Add(tokenTTL)
		setSessionCookies(c, newToken, csrfToken, expires)
		return c.SendStatus(fiber.StatusNoContent)
	})

	v1.Get("/servers/:id/ws/stats", requireRealtimeServices(cfg), fiberws.New(realtimeProxy(cfg, wsTickets, "stats"), fiberws.Config{
		RecoverHandler: func(conn *fiberws.Conn) {
			if err := recover(); err != nil {
				_ = conn.WriteJSON(fiber.Map{"error": "internal error"})
				_ = conn.Close()
			}
		},
		Origins: getWebSocketAllowedOrigins(cfg),
	}))
	v1.Get("/servers/:id/ws/logs", requireRealtimeServices(cfg), fiberws.New(realtimeProxy(cfg, wsTickets, "logs"), fiberws.Config{
		RecoverHandler: func(conn *fiberws.Conn) {
			if err := recover(); err != nil {
				_ = conn.WriteJSON(fiber.Map{"error": "internal error"})
				_ = conn.Close()
			}
		},
		Origins: getWebSocketAllowedOrigins(cfg),
	}))
	v1.Get("/servers/:id/ws/console", requireRealtimeServices(cfg), fiberws.New(realtimeProxy(cfg, wsTickets, "console"), fiberws.Config{
		RecoverHandler: func(conn *fiberws.Conn) {
			if err := recover(); err != nil {
				_ = conn.WriteJSON(fiber.Map{"error": "internal error"})
				_ = conn.Close()
			}
		},
		Origins: getWebSocketAllowedOrigins(cfg),
	}))

	remote := app.Group("/api/remote", remoteNodeMiddleware(cfg, nodeRegistry))
	remote.Get("/servers", func(c *fiber.Ctx) error {
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		targets, err := cfg.Store.RemoteServerConfigurations(ctx, node.ID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		data := make([]fiber.Map, 0, len(targets))
		for _, target := range targets {
			data = append(data, remoteServerPayload(target))
		}
		return c.JSON(fiber.Map{
			"data": data,
			"meta": fiber.Map{
				"pagination": fiber.Map{
					"total":        len(data),
					"count":        len(data),
					"per_page":     len(data),
					"current_page": 1,
					"total_pages":  1,
				},
			},
		})
	})
	remote.Post("/servers/reset", func(c *fiber.Ctx) error {
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.ResetNodeServerStates(ctx, node.ID); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
	remote.Post("/sftp/auth", authLimiter, func(c *fiber.Ctx) error {
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}
		var body struct {
			Type      string `json:"type"`
			Username  string `json:"username"`
			Password  string `json:"password"`
			PublicKey string `json:"publicKey"`
			User      string `json:"user"`
			Server    string `json:"server"`
			IP        string `json:"ip"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var result store.SFTPAuthResult
		var err error
		switch body.Type {
		case "", "password":
			result, err = cfg.Store.AuthenticateSFTP(ctx, node.ID, body.Username, body.Password)
		case "public_key":
			result, err = cfg.Store.AuthenticateSFTPPublicKey(ctx, node.ID, body.Username, body.PublicKey)
		case "check":
			result, err = cfg.Store.AuthorizeSFTPSession(ctx, node.ID, body.User, body.Server)
		default:
			return fiber.NewError(fiber.StatusForbidden, "unsupported sftp authentication type")
		}
		if err != nil {
			return fiber.NewError(fiber.StatusForbidden, "authorization credentials were not correct")
		}
		return c.JSON(result)
	})
	remote.Get("/servers/:id", func(c *fiber.Ctx) error {
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}
		ctx, cancel := requestContext()
		defer cancel()
		belongs, err := cfg.Store.ServerBelongsToNode(ctx, c.Params("id"), node.ID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if !belongs {
			return fiber.NewError(fiber.StatusForbidden, "requesting node cannot access this server")
		}
		target, err := cfg.Store.ServerProvisionTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		payload := remoteServerPayload(target)
		return c.JSON(fiber.Map{
			"settings":              payload["settings"],
			"process_configuration": payload["process_configuration"],
		})
	})
	remote.Get("/servers/:id/install", func(c *fiber.Ctx) error {
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}
		ctx, cancel := requestContext()
		defer cancel()
		belongs, err := cfg.Store.ServerBelongsToNode(ctx, c.Params("id"), node.ID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if !belongs {
			return fiber.NewError(fiber.StatusForbidden, "requesting node cannot access this server")
		}
		target, err := cfg.Store.ServerProvisionTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		return c.JSON(fiber.Map{
			"container_image": target.InstallContainer,
			"entrypoint":      target.InstallEntrypoint,
			"script":          target.InstallScript,
		})
	})
	remote.Post("/servers/:id/install", func(c *fiber.Ctx) error {
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}
		var body struct {
			Successful bool   `json:"successful"`
			Reinstall  bool   `json:"reinstall"`
			Error      string `json:"error"`
		}
		_ = c.BodyParser(&body)
		ctx, cancel := requestContext()
		defer cancel()
		belongs, err := cfg.Store.ServerBelongsToNode(ctx, c.Params("id"), node.ID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if !belongs {
			return fiber.NewError(fiber.StatusForbidden, "requesting node cannot access this server")
		}
		state := "installed"
		if !body.Successful {
			state = "failed"
		}
		if err := cfg.Store.SetServerInstallState(ctx, c.Params("id"), state, body.Error); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if body.Successful && cfg.MailTriggerService != nil {
			if srv, e := cfg.Store.GetServer(ctx, c.Params("id")); e == nil {
				cfg.MailTriggerService.SendInstallComplete(ctx, srv.Owner, srv.Owner, srv.Name, srv.ID)
			}
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
	// Legacy daemon transfer callbacks cannot safely drive the migration state
	// machine and are intentionally retired. Real migration execution owns its
	// lifecycle through MigrationService.
	remote.Post("/servers/:id/transfer/success", legacyServerTransferCallbackUnavailable)
	remote.Post("/servers/:id/transfer/failure", legacyServerTransferCallbackUnavailable)

	// ---- Tier 1 remote endpoints (Beacon reports status back to Forge) ----

	remote.Post("/servers/:id/backups/status", func(c *fiber.Ctx) error {
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}
		var body struct {
			Name     string `json:"name"`
			UUID     string `json:"uuid"`
			Status   string `json:"status"`
			Checksum string `json:"checksum"`
			Size     int64  `json:"size"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		belongs, err := cfg.Store.ServerBelongsToNode(ctx, c.Params("id"), node.ID)
		if err != nil || !belongs {
			return fiber.NewError(fiber.StatusForbidden, "requesting node cannot access this server")
		}
		if body.Status == "completed" && body.Checksum != "" {
			completedAt := time.Now().UTC()
			var actorID *string
			if claims, ok := c.Locals("user").(tokenClaims); ok {
				actorID = &claims.Sub
			}
			_, err = cfg.Store.UpsertBackup(ctx, c.Params("id"), store.UpsertBackupRequest{
				UUID:        body.UUID,
				Name:        body.Name,
				Checksum:    body.Checksum,
				Size:        body.Size,
				Status:      body.Status,
				CompletedAt: &completedAt,
			}, actorID)
		} else {
			var actorID *string
			err = cfg.Store.MarkBackupStatus(ctx, c.Params("id"), body.Name, body.Status, actorID)
		}
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	remote.Post("/servers/:id/status", func(c *fiber.Ctx) error {
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}
		var body struct {
			ActualState string `json:"actualState"`
			Status      string `json:"status"`
			Error       string `json:"error"`
		}
		_ = c.BodyParser(&body)
		ctx, cancel := requestContext()
		defer cancel()
		belongs, err := cfg.Store.ServerBelongsToNode(ctx, c.Params("id"), node.ID)
		if err != nil || !belongs {
			return fiber.NewError(fiber.StatusForbidden, "requesting node cannot access this server")
		}
		if body.ActualState != "" {
			_ = cfg.Store.SetServerActualState(ctx, c.Params("id"), store.ServerActualState(body.ActualState), body.Status)
		}
		if body.Status != "" {
			_ = cfg.Store.SetServerStatus(ctx, c.Params("id"), body.Status, body.Error)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	remote.Post("/servers/:id/activity", func(c *fiber.Ctx) error {
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}
		var body struct {
			Action   string `json:"action"`
			Metadata string `json:"metadata"`
		}
		_ = c.BodyParser(&body)
		if body.Action == "" {
			return fiber.NewError(fiber.StatusBadRequest, "action is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		belongs, err := cfg.Store.ServerBelongsToNode(ctx, c.Params("id"), node.ID)
		if err != nil || !belongs {
			return fiber.NewError(fiber.StatusForbidden, "requesting node cannot access this server")
		}
		serverID := c.Params("id")
		_ = cfg.Store.AppendAudit(ctx, nil, body.Action, "server", &serverID, body.Metadata)
		return c.SendStatus(fiber.StatusNoContent)
	})

	remote.Post("/servers/:id/crash", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return c.Status(503).JSON(fiber.Map{"error": "database not available"})
		}
		var req struct {
			ExitCode    int  `json:"exit_code"`
			OOMKilled   bool `json:"oom_killed"`
			AutoRestart bool `json:"auto_restart"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		crashID := uuid.NewString()
		_, err := cfg.Store.Exec(c.Context(), `INSERT INTO server_crash_events
			(id, server_id, node_id, exit_code, oom_killed, auto_restarted, created_at)
			VALUES ($1, $2, '', $3, $4, $5, NOW())`,
			crashID, c.Params("id"), req.ExitCode, req.OOMKilled, req.AutoRestart)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true, "id": crashID})
	})

	// Additional /api/remote/* routes for daemon parity
	// (cluster-wide activity, backup lifecycle, archive completion, transfer state).
	registerRemoteExtras(remote, cfg)

	// Setup wizard routes (public, gated to "no admin exists" server-side)
	registerSetupRoutes(v1, cfg, authLimiter)

	// OAuth2 token endpoint (PufferPanel parity). Mounted on the public
	// `/api/v1/oauth2/token` and `/oauth2/token` paths so external
	// integrations can reach it without an admin JWT.
	v1.Post("/oauth2/token", IssueOAuth2Token(cfg))
	v1.Post("/oauth/token", IssueOAuth2Token(cfg)) // alias

	// Social authentication (Discord, Steam, Authentik)
	registerSocialAuthRoutes(v1, cfg, mutationLimiter)

	// Git webhook endpoints (public, verified by HMAC signatures)
	registerGitWebhookRoutes(v1, cfg)

	// Set panel origin for CSRF validation
	v1.Use(func(c *fiber.Ctx) error {
		if cfg.PanelURL != "" {
			c.Locals("panelOrigin", cfg.PanelURL)
		}
		return c.Next()
	})

	var sessMw fiber.Handler
	if cfg.SessionStore != nil {
		sessMw = auth.SessionMiddleware(cfg.SessionStore)
	} else {
		sessMw = func(c *fiber.Ctx) error { return c.Next() }
	}

	protected := v1.Group("", authMiddleware(cfg.AuthSecret, cfg.Store), sessMw, requireTwoFactorAuthentication(cfg.Store), csrfMiddleware(LoadSessionCookieConfig()), readLimiter)
	protected.Post("/servers/:id/ws/ticket", IssueWSTicket(cfg, wsTickets))
	protected.Post("/servers/:id/files/download-ticket", mutationLimiter, issueFileDownloadTicket(cfg, fileDownloadTickets))
	protected.Post("/servers/:id/backups/download-ticket", mutationLimiter, issueBackupDownloadTicket(cfg, fileDownloadTickets))

	// Get signed download URL for a file
	protected.Get("/servers/:id/files/download-url", requireServerPermission(cfg, store.PermFileReadContent), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		_, cancel := requestContext()
		defer cancel()

		serverID := c.Params("id")
		filePath := c.Query("path")
		if filePath == "" {
			return fiber.NewError(fiber.StatusBadRequest, "path parameter is required")
		}

		// Issue a download ticket for the file
		ticket, err := fileDownloadTickets.issue(fileDownloadTicket{
			serverID: serverID,
			filePath: filePath,
			expires:  time.Now().Add(5 * time.Minute),
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to issue download ticket")
		}

		// Return signed URL
		baseURL := c.BaseURL()
		downloadURL := fmt.Sprintf("%s/api/v1/download/file?token=%s", baseURL, ticket)

		return c.JSON(fiber.Map{
			"url":     downloadURL,
			"expires": time.Now().Add(5 * time.Minute).Format(time.RFC3339),
		})
	})

	v1.Get("/download/file", downloadFileWithTicket(cfg, fileDownloadTickets))

	// ---- Tier 1: Console command ----

	protected.Post("/servers/:id/command", mutationLimiter, requireServerPermission(cfg, store.PermControlConsole), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		var body struct {
			Command string `json:"command"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(body.Command) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "command is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if err := cfg.Daemon.SendCommand(ctx, target.NodeURL, target.NodeToken, target.ServerID, body.Command); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:console.command", "server", &target.ServerID, safeAuditMeta(map[string]string{"command": body.Command}))
		return c.JSON(fiber.Map{"ok": true})
	})

	// ---- Tier 2: Single-GET routes ----

	protected.Get("/users/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		user, err := cfg.Store.GetUserByID(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "user not found")
		}
		return c.JSON(user)
	})

	protected.Get("/mounts/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		mount, err := cfg.Store.GetMount(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "mount not found")
		}
		return c.JSON(mount)
	})

	protected.Get("/servers/:id/users/:userId", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		subuser, err := cfg.Store.GetServerSubuser(ctx, c.Params("id"), c.Params("userId"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "subuser not found")
		}
		return c.JSON(subuser)
	})

	// Register domain-specific route handlers
	registerExternalLookupRoutes(protected, cfg)
	registerAuthRoutes(protected, cfg, mutationLimiter)
	registerPasswordResetRoutes(v1, cfg, authLimiter)
	registerAccountRecoveryRoutes(v1, cfg, authLimiter)
	// Register fixed plugin subroutes before admin's /admin/plugins/:id route.
	// Otherwise paths such as /discover are parsed as a plugin identifier.
	registerPluginRoutes(protected, cfg)
	registerAdminRoutes(protected, cfg, nodeRegistry, clusterManager, evacuationPlanner, migrationService, reservationManager, recoveryCoordinator, mutationLimiter, adminIPAccess)
	registerServerRoutes(protected, cfg, runner, clusterManager, mutationLimiter, adminIPAccess)
	registerSettingsRoutes(protected, cfg, mutationLimiter, adminIPAccess)
	registerSettingsExtras(protected, cfg, mutationLimiter, adminIPAccess)
	registerRateLimitSettingsRoutes(protected, cfg, mutationLimiter, adminIPAccess)
	registerActivityRoutes(protected, cfg)
	registerAuditLogRoutes(protected, cfg)
	registerOrphanRemediationRoutes(protected, cfg, mutationLimiter, adminIPAccess)
	registerAdminExtras(protected, cfg, nodeProbe)
	registerObservabilityRoutes(protected, cfg, cfg.Observability, cfg.HeartbeatMonitor)
	registerAlertRoutes(protected, cfg.AlertService, cfg.Observability)
	registerNotificationRoutes(protected, cfg.NotificationService)
	if cfg.EnhancedNotificationService != nil {
		registerEnhancedNotificationRoutes(protected, cfg.EnhancedNotificationService)
	}
	registerMailSettingsRoutes(protected, cfg, mutationLimiter, adminIPAccess)
	registerSFTPRoutes(protected, cfg)
	registerWebAuthnRoutes(protected, cfg, mutationLimiter, cfg.WebAuthnService)
	registerAutoScalerRoutes(protected, cfg, cfg.AutoScaler, adminIPAccess, mutationLimiter)
	registerDeploymentRoutes(protected, cfg, cfg.DeploymentSvc, adminIPAccess, mutationLimiter)
	registerDeploymentHistoryRoutes(protected, cfg, adminIPAccess, mutationLimiter)
	registerRevisionRoutes(protected, cfg, cfg.DeploymentSvc, adminIPAccess, mutationLimiter)
	registerPreviewDeploymentRoutes(protected, cfg, cfg.PreviewDeploymentSvc, adminIPAccess, mutationLimiter)
	registerCloudRoutes(protected, cfg, cfg.CloudManager, adminIPAccess, mutationLimiter)
	registerLoadBalancerRoutes(protected, cfg, cfg.LoadBalancer, adminIPAccess, mutationLimiter)
	registerFailoverRoutes(protected, cfg, cfg.FailoverSvc, adminIPAccess, mutationLimiter)
	registerTrafficManagerRoutes(protected, cfg, cfg.TrafficManager, adminIPAccess, mutationLimiter)
	registerDomainRoutes(protected, cfg, cfg.DomainService, mutationLimiter)
	registerCertificateRoutes(protected, cfg, cfg.AcmeService, adminIPAccess, mutationLimiter)
	registerCertificateRoutesExt(protected, cfg, adminIPAccess, mutationLimiter)
	registerAcmeAccountRoutes(protected, cfg, adminIPAccess, mutationLimiter)
	registerMTLSRoutes(protected, cfg, cfg.CertService, cfg.MTLSMigrator, adminIPAccess, mutationLimiter)
	registerSchedulerRoutes(protected, cfg, cfg.PredictiveScorer, cfg.ConstraintScheduler, adminIPAccess, mutationLimiter)
	registerCrashDetectionRoutes(protected, cfg, cfg.CrashDetector, mutationLimiter)
	registerBackupRoutes(protected, cfg, cfg.BackupSvc, mutationLimiter)
	registerDNSRoutes(protected, cfg, cfg.DNSService, mutationLimiter)
	registerMaintenanceRoutes(protected, cfg, mutationLimiter)
	registerComposeRoutes(protected, cfg, mutationLimiter)
	registerDBContainerRoutes(protected, cfg, mutationLimiter)
	registerDatabaseServiceRoutes(protected, cfg, mutationLimiter)
	registerManagedDatabaseRoutes(protected, cfg, mutationLimiter)
	registerGitRoutes(protected, cfg, adminIPAccess, mutationLimiter)
	RegisterGitDeploymentRoutes(protected, cfg)
	registerBuildRoutes(protected, cfg, cfg.BuildService, mutationLimiter)
	registerBuildpackRoutes(protected, cfg, cfg.BuildpackService, mutationLimiter)
	registerZeroDowntimeRoutes(protected, cfg, cfg.ZeroDowntimeSvc, mutationLimiter)
	registerSourceDeploymentRoutes(protected, cfg, mutationLimiter)
	registerReconcileRoutes(protected, cfg, cfg.Reconciler, adminIPAccess, mutationLimiter)

	// Capability inventory and onboarding token management
	registerCapabilityRoutes(protected, cfg, cfg.NodeProbe)

	// Team tenancy routes (Org → Project → Environment hierarchy)
	if cfg.TenancyService != nil {
		registerTenancyRoutes(protected, cfg, cfg.TenancyService, cfg.EnvVarService)
	}

	// Infrastructure endpoint routes (Portainer-style Environment abstraction)
	registerEndpointRoutes(protected, cfg, cfg.EndpointService)

	// App hosting routes (Application → Service model)
	registerAppHostingRoutes(protected, cfg, cfg.AppHostingService, mutationLimiter)
	registerProcedureRoutes(protected, cfg, cfg.ProcedureService, mutationLimiter)

	// Portainer-inspired container/image/network/volume administration
	registerPortainerRoutes(protected, cfg, mutationLimiter, adminIPAccess)
	registerAppStoreRoutes(protected, cfg, cfg.AppStoreService, mutationLimiter)

	// Docker management routes (cleaner replacement for Portainer admin routes)
	registerDockerRoutes(protected, cfg, mutationLimiter, adminIPAccess)

	// Host-level file management and terminal
	registerHostFileRoutes(protected, cfg, mutationLimiter)
	registerHostTerminalRoute(protected, cfg)

	// Cron job management
	if cfg.CronJobService != nil {
		registerCronJobRoutes(protected, cfg, cfg.CronJobService)
	}

	// Procfile process management
	if cfg.ProcessService != nil {
		registerProcessRoutes(protected, cfg, cfg.ProcessService)
	}

	// Proxy domain management (reverse proxy level, distinct from per-server domains)
	registerProxyDomainRoutes(protected, cfg, adminIPAccess, mutationLimiter)
	registerProxyCertificateRoutes(protected, cfg, adminIPAccess, mutationLimiter)
	registerSecurityHeadersRoutes(protected, cfg, adminIPAccess, mutationLimiter)
	registerRedirectRulesRoutes(protected, cfg, adminIPAccess, mutationLimiter)

	// Host management
	registerHostRoutes(protected, cfg)
	registerFirewallRoutes(protected, cfg)

	// Cluster membership + cleanup routes
	registerClusterMembershipRoutes(protected, cfg.ClusterMembershipService)
	registerCleanupRoutes(protected, cfg.CleanupService)

	// Cross-node routing and service discovery routes
	registerServiceDiscoveryRoutes(protected, cfg, cfg.ServiceDiscovery, adminIPAccess, mutationLimiter)
	registerCrossNodeRoutes(protected, cfg, cfg.CrossNodeResolver, cfg.IngressSynchronizer, adminIPAccess, mutationLimiter)

	// Start schedule runner
	if cfg.Store != nil {
		workerCtx := cfg.BackgroundContext
		if workerCtx == nil {
			workerCtx = context.Background()
		}
		runner.Start(workerCtx)
		app.Hooks().OnShutdown(func() error { runner.Wait(); return nil })
	}

	return app
}
