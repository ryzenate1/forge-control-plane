package http

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/auth"
	"gamepanel/forge/internal/cloud"
	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/events"
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
	TokenTTL          time.Duration
	AppEnv            string
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
	EventRegistry        *events.Registry

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

	AlertService                *alerting.Service
	NotificationService         *notificationsvc.Service
	EnhancedNotificationService *enhancednotifsvc.Service
	CronJobService              *cronjobsvc.Service
	ProcessService              *processsvc.Service
	ZeroDowntimeSvc             *zerodowntime.Service

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
	LangsDir   string
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

var (
	inMemLoginMu    sync.Mutex
	inMemLoginCount = map[string]int{}
)

// nodeHeartbeatNonces is a shared replay-protection cache for node heartbeat
// requests. It mirrors the /api/remote HMAC nonce store.
var nodeHeartbeatNonces = newRemoteNonceStore()

// verifyNodeTokenWithHMAC authenticates a v1 node-scoped request the way
// /nodes/:id/heartbeat does: the bearer node token is verified against the
// store, then the request must carry a fresh one-time HMAC signature.
func verifyNodeTokenWithHMAC(ctx context.Context, cfg Config, c *fiber.Ctx, nodeID, token string) error {
	if strings.TrimSpace(token) == "" {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid node token")
	}
	ok, err := cfg.Store.VerifyNodeToken(ctx, nodeID, token)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "token verification failed")
	}
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid node token")
	}
	if err := verifyRemoteHMAC(c, token, nodeHeartbeatNonces); err != nil {
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	}
	return nil
}

// 2FA checkpoint hardening: per-account attempt counting with lockout and a
// single-use consumed set for confirmation tokens.
const (
	twoFactorMaxAttempts = 10
	twoFactorLockout     = 15 * time.Minute
	twoFactorTokenTTL    = 5 * time.Minute
)

var (
	inMem2FAMu        sync.Mutex
	inMem2FACount     = map[string]int{}
	inMem2FALockUntil = map[string]time.Time{}
	inMem2FAConsumed  = map[string]time.Time{}
)

func checkLoginRateLimit(ctx context.Context, cfg Config, c *fiber.Ctx, email string) error {
	keys := []string{
		loginRateLimitKey("ip", c.IP()),
		loginRateLimitKey("email", email),
	}
	if cfg.Redis != nil && cfg.RedisEnabled {
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
	// In-memory fallback when Redis is unavailable
	inMemLoginMu.Lock()
	defer inMemLoginMu.Unlock()
	for _, key := range keys {
		if count, ok := inMemLoginCount[key]; ok && count >= 5 {
			return fiber.NewError(fiber.StatusTooManyRequests, "too many login attempts; try again later")
		}
	}
	return nil
}

func recordLoginFailure(ctx context.Context, cfg Config, c *fiber.Ctx, email string) {
	keys := []string{
		loginRateLimitKey("ip", c.IP()),
		loginRateLimitKey("email", email),
	}
	if cfg.Redis != nil && cfg.RedisEnabled {
		for _, key := range keys {
			count, err := cfg.Redis.Incr(ctx, key).Result()
			if err == nil && count == 1 {
				if err := cfg.Redis.Expire(ctx, key, time.Minute).Err(); err != nil && cfg.Logger != nil {
					cfg.Logger.Error("failed to set login rate limit expiry", "key", key, "error", err)
				}
			}
		}
		return
	}
	// In-memory fallback when Redis is unavailable
	inMemLoginMu.Lock()
	defer inMemLoginMu.Unlock()
	now := time.Now()
	for _, key := range keys {
		inMemLoginCount[key]++
		if inMemLoginCount[key] == 1 {
			expiry := now.Add(time.Minute)
			k := key
			go func() {
				defer func() {
					if r := recover(); r != nil {
						if cfg.Logger != nil {
							cfg.Logger.Error("login rate limit cleanup panic", "panic", r)
						}
					}
				}()
				time.Sleep(time.Until(expiry))
				inMemLoginMu.Lock()
				delete(inMemLoginCount, k)
				inMemLoginMu.Unlock()
			}()
		}
	}
}

func clearLoginFailures(ctx context.Context, cfg Config, c *fiber.Ctx, email string) {
	keys := []string{
		loginRateLimitKey("ip", c.IP()),
		loginRateLimitKey("email", email),
	}
	if cfg.Redis != nil && cfg.RedisEnabled {
		if err := cfg.Redis.Del(ctx, keys[0], keys[1]).Err(); err != nil && cfg.Logger != nil {
			cfg.Logger.Error("failed to clear login rate limit", "error", err)
		}
		return
	}
	// In-memory fallback when Redis is unavailable
	inMemLoginMu.Lock()
	defer inMemLoginMu.Unlock()
	for _, key := range keys {
		delete(inMemLoginCount, key)
	}
}

func twoFactorAttemptKey(userID string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(userID))))
	return "2fa:attempts:" + hex.EncodeToString(sum[:])
}

func twoFactorLockKey(userID string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(userID))))
	return "2fa:lock:" + hex.EncodeToString(sum[:])
}

// checkTwoFactorLockout rejects 2FA checkpoint attempts while the per-account
// lockout (10 failures → 15 minutes) is active.
func checkTwoFactorLockout(ctx context.Context, cfg Config, userID string) error {
	if cfg.Redis != nil && cfg.RedisEnabled {
		locked, err := cfg.Redis.Get(ctx, twoFactorLockKey(userID)).Int()
		if err == nil && locked == 1 {
			return fiber.NewError(fiber.StatusTooManyRequests, "too many verification attempts; try again later")
		}
		return nil
	}
	inMem2FAMu.Lock()
	defer inMem2FAMu.Unlock()
	if until, ok := inMem2FALockUntil[userID]; ok && time.Now().Before(until) {
		return fiber.NewError(fiber.StatusTooManyRequests, "too many verification attempts; try again later")
	}
	return nil
}

func recordTwoFactorFailure(ctx context.Context, cfg Config, userID string) {
	now := time.Now()
	if cfg.Redis != nil && cfg.RedisEnabled {
		key := twoFactorAttemptKey(userID)
		count, err := cfg.Redis.Incr(ctx, key).Result()
		if err != nil {
			return
		}
		if count == 1 {
			_ = cfg.Redis.Expire(ctx, key, twoFactorLockout).Err()
		}
		if count >= twoFactorMaxAttempts {
			_ = cfg.Redis.Set(ctx, twoFactorLockKey(userID), 1, twoFactorLockout).Err()
			_ = cfg.Redis.Del(ctx, key).Err()
		}
		return
	}
	inMem2FAMu.Lock()
	defer inMem2FAMu.Unlock()
	inMem2FACount[userID]++
	if inMem2FACount[userID] >= twoFactorMaxAttempts {
		inMem2FALockUntil[userID] = now.Add(twoFactorLockout)
		delete(inMem2FACount, userID)
	}
}

func clearTwoFactorFailures(ctx context.Context, cfg Config, userID string) {
	if cfg.Redis != nil && cfg.RedisEnabled {
		_ = cfg.Redis.Del(ctx, twoFactorAttemptKey(userID), twoFactorLockKey(userID)).Err()
		return
	}
	inMem2FAMu.Lock()
	defer inMem2FAMu.Unlock()
	delete(inMem2FACount, userID)
	delete(inMem2FALockUntil, userID)
}

// consume2FAConfirmation atomically marks a confirmation-token jti as used so
// the token cannot be replayed. Returns false when it was already consumed.
func consume2FAConfirmation(ctx context.Context, cfg Config, jti string, ttl time.Duration) bool {
	if cfg.Redis != nil && cfg.RedisEnabled {
		key := "2fa:consumed:" + jti
		set, err := cfg.Redis.SetNX(ctx, key, 1, ttl).Result()
		return err == nil && set
	}
	inMem2FAMu.Lock()
	defer inMem2FAMu.Unlock()
	now := time.Now()
	for value, expiry := range inMem2FAConsumed {
		if !expiry.After(now) {
			delete(inMem2FAConsumed, value)
		}
	}
	if _, ok := inMem2FAConsumed[jti]; ok {
		return false
	}
	inMem2FAConsumed[jti] = now.Add(ttl)
	return true
}

// recordUserSession persists a JWT session row so /account/sessions listing
// and revocation operate on real JWT sessions. session_token_hash holds the
// SHA-256 of the JWT's jti claim.
func recordUserSession(ctx context.Context, cfg Config, c *fiber.Ctx, token string) {
	if cfg.Store == nil {
		return
	}
	claims, err := parseToken(cfg.AuthSecret, token)
	if err != nil || claims.Sub == "" || claims.JTI == "" {
		return
	}
	sum := sha256.Sum256([]byte(claims.JTI))
	_, _ = cfg.Store.CreateUserSession(ctx, claims.Sub, hex.EncodeToString(sum[:]), c.IP(), c.Get("User-Agent"), configuredTokenTTL(cfg))
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
	LocationID          string           `json:"locationId" validate:"required"`
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
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		BodyLimit:         32 * 1024 * 1024,
		StreamRequestBody: false,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			var e *fiber.Error
			if errors.As(err, &e) {
				code = e.Code
			}
			msg := err.Error()
			// Sanitize every 5xx in production. 4xx messages may legitimately
			// carry validation details, but 5xx bodies must never echo internal
			// error text (SQL errors, host paths, provider responses).
			if cfg.AppEnv == "production" && code >= fiber.StatusInternalServerError {
				msg = "an internal error occurred"
			}
			return c.Status(code).JSON(fiber.Map{"error": msg})
		},
	})

	// Panic recovery middleware - catches panics and returns 500
	// Stack traces are only enabled in non-production for debugging
	app.Use(fiberrecover.New(fiberrecover.Config{EnableStackTrace: cfg.AppEnv != "production"}))

	// Attach the Config to every request so request-scoped helpers
	// (respondInternalError, logInternalError, isProductionEnv) can log
	// internal failures and decide whether error details may be exposed.
	app.Use(func(c *fiber.Ctx) error {
		c.Locals(configLocalKey, cfg)
		return c.Next()
	})

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
			if cfg.AppEnv == "production" && cfg.Logger != nil {
				cfg.Logger.Warn("API_CORS_ALLOWED_ORIGINS is not set; production CORS will block non-localhost origins")
			}
			corsCfg = DefaultCORSConfig()
		}
	}
	app.Use(CORSMiddleware(corsCfg))

	// Maintenance mode middleware - checks FORGE_MAINTENANCE_MODE env var
	app.Use(MaintenanceModeMiddleware(cfg))

	// Security headers middleware - prevents XSS, clickjacking, MIME sniffing
	// Added as part of comprehensive security audit fixes
	app.Use(SecurityHeaders(cfg.AppEnv))

	if cfg.Logger != nil {
		app.Use(StructuredLogger(cfg.Logger))
	}

	registerSwaggerRoutes(app, cfg.AppEnv)

	registerWellKnownVerifyRoute(app, cfg.DomainService)

	// Internationalization middleware
	if cfg.Translator != nil {
		app.Use(I18nMiddleware(cfg.Translator))
	}

	// Create rate limiters for different endpoint types
	// Added as part of comprehensive security audit fixes
	requireSharedRateLimiter := cfg.RedisEnabled && strings.EqualFold(strings.TrimSpace(cfg.AppEnv), "production")
	authLimiter := RateLimiter(GetRateLimitForEndpoint("auth", cfg.Redis, requireSharedRateLimiter))
	mutationLimiter := RateLimiter(GetRateLimitForEndpoint("mutation", cfg.Redis, requireSharedRateLimiter))
	readLimiter := RateLimiter(GetRateLimitForEndpoint("read", cfg.Redis, requireSharedRateLimiter))

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
	if cfg.Store != nil {
		mtlsCfg.RevocationLookup = cfg.Store.IsMTLSCertificateRevoked
	}
	mtlsMw := MTLSAuthMiddleware(mtlsCfg)

	v1 := app.Group("/api/v1", apiIPAccess, mtlsMw)
	// Panel origin is set before any route so public mutations (login, setup,
	// password reset, session exchange/refresh) can run origin validation.
	v1.Use(func(c *fiber.Ctx) error {
		if cfg.PanelURL != "" {
			c.Locals("panelOrigin", cfg.PanelURL)
		}
		return c.Next()
	})
	v1.Post("/csp-report", authLimiter, func(c *fiber.Ctx) error {
		contentType := strings.ToLower(strings.TrimSpace(strings.SplitN(c.Get(fiber.HeaderContentType), ";", 2)[0]))
		switch contentType {
		case "application/csp-report", "application/reports+json", "application/json":
		default:
			return fiber.NewError(fiber.StatusUnsupportedMediaType, "unsupported report content type")
		}
		if len(c.Body()) > 64<<10 {
			return fiber.NewError(fiber.StatusRequestEntityTooLarge, "CSP report is too large")
		}
		var report any
		if err := json.Unmarshal(c.Body(), &report); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid CSP report")
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
	v1.Post("/onboarding/exchange", authLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Token string `json:"token"`
		}
		if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.Token) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		nodeID, err := cfg.Store.ConsumeOnboardingToken(ctx, req.Token)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired onboarding token")
		}
		credential, err := cfg.Store.GetNodeDaemonCredential(ctx, nodeID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "node credential is unavailable")
		}
		return c.JSON(fiber.Map{"nodeId": nodeID, "nodeToken": credential})
	})
	v1.Get("/panel/settings/public", func(c *fiber.Ctx) error {
		settings := defaultPanelSettings()
		if cfg.Store != nil {
			ctx, cancel := requestContext()
			defer cancel()
			if stored, err := cfg.Store.GetPanelSettings(ctx); err == nil {
				settings = stored
			} else if cfg.Logger != nil {
				cfg.Logger.Error("failed to load panel settings", "error", err)
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
	allowedLocales := map[string]bool{
		"en": true, "de": true, "fr": true, "es": true, "pt": true,
		"ru": true, "zh": true, "ja": true, "ko": true, "it": true,
		"nl": true, "pl": true, "sv": true, "nb": true, "da": true,
		"fi": true, "cs": true, "hu": true, "ro": true, "uk": true,
		"tr": true, "ar": true, "th": true, "vi": true, "ms": true,
	}
	v1.Get("/i18n/:locale", func(c *fiber.Ctx) error {
		locale := c.Params("locale")
		if !allowedLocales[locale] {
			return fiber.NewError(fiber.StatusBadRequest, "invalid locale")
		}
		langsDir := cfg.LangsDir
		if langsDir == "" {
			langsDir = "lang"
		}
		path := langsDir + "/" + locale + ".json"
		if _, err := os.Stat(path); os.IsNotExist(err) {
			path = langsDir + "/en.json"
			if _, err := os.Stat(path); os.IsNotExist(err) {
				return fiber.NewError(fiber.StatusNotFound, "translation not found")
			}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return respondInternalError(c, err)
		}
		var jsonData map[string]any
		if err := json.Unmarshal(data, &jsonData); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(jsonData)
	})

	// CSRF token endpoint for clients to fetch CSRF token
	v1.Get("/csrf-token", GetCSRFTokenHandler())
	// Session exchange endpoint — exchanges a single-use code for a session token.
	// Used after social auth redirects to avoid placing the token in the URL.
	v1.Post("/auth/session/exchange", ExchangeCodeHandler(cfg))

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
		token := strings.TrimSpace(os.Getenv("METRICS_TOKEN"))
		expected := "Bearer " + token
		if token == "" || subtle.ConstantTimeCompare([]byte(c.Get("Authorization")), []byte(expected)) != 1 {
			return fiber.NewError(fiber.StatusUnauthorized, "authentication required")
		}
		var body strings.Builder
		body.WriteString("# HELP game_panel_api_up API process is serving the metrics endpoint, 1 when available.\n")
		body.WriteString("# TYPE game_panel_api_up gauge\n")
		body.WriteString("game_panel_api_up 1\n")
		body.WriteString("# HELP game_panel_api_uptime_seconds API process uptime.\n")
		body.WriteString("# TYPE game_panel_api_uptime_seconds gauge\n")
		body.WriteString("game_panel_api_uptime_seconds " + strconv.FormatFloat(time.Since(started).Seconds(), 'f', 3, 64) + "\n")

		// Reconciliation counters tracked by the reconciler service. Emitted
		// only when the reconciler is wired so alert rules referencing them
		// never see a missing series from a partially configured API.
		if cfg.Reconciler != nil {
			metrics := cfg.Reconciler.Metrics()
			counters := []struct {
				name  string
				help  string
				value uint64
			}{
				{"game_panel_api_reconciliation_total", "Cumulative reconciliation runs.", metrics.ReconciliationCount},
				{"game_panel_api_reconciliation_failures_total", "Cumulative reconciliation runs that failed.", metrics.ReconciliationFailures},
				{"game_panel_api_node_refresh_failures_total", "Cumulative node refresh failures.", metrics.NodeRefreshFailures},
				{"game_panel_api_server_sync_failures_total", "Cumulative server sync failures.", metrics.ServerSyncFailures},
				{"game_panel_api_server_reconciliation_total", "Cumulative reconciled servers.", metrics.ServerReconciliationTotal},
				{"game_panel_api_node_reconciliation_total", "Cumulative reconciled nodes.", metrics.NodeReconciliationTotal},
				{"game_panel_api_health_recovery_attempts_total", "Cumulative unhealthy target recovery attempts.", metrics.HealthRecoveryAttempts},
				{"game_panel_api_health_recovery_failures_total", "Cumulative unhealthy target recovery failures.", metrics.HealthRecoveryFailures},
				{"game_panel_api_drifts_detected_total", "Cumulative drifts detected.", metrics.DriftsDetected},
				{"game_panel_api_plans_generated_total", "Cumulative reconcile plans generated.", metrics.PlansGenerated},
				{"game_panel_api_auto_reconcile_runs_total", "Cumulative automatic reconcile runs.", metrics.AutoReconcileRuns},
			}
			for _, counter := range counters {
				body.WriteString("# HELP " + counter.name + " " + counter.help + "\n")
				body.WriteString("# TYPE " + counter.name + " counter\n")
				body.WriteString(counter.name + " " + strconv.FormatUint(counter.value, 10) + "\n")
			}
		}

		if cfg.EventRegistry != nil {
			em := cfg.EventRegistry.Metrics()
			eventCounters := []struct {
				name  string
				help  string
				value uint64
			}{
				{"game_panel_api_events_published_total", "Cumulative events published to the event registry.", em.EventsPublishedTotal},
				{"game_panel_api_events_delivered_total", "Cumulative events delivered to subscribers.", em.EventsDeliveredTotal},
				{"game_panel_api_event_handler_failures_total", "Cumulative event handler failures.", em.EventHandlerFailuresTotal},
				{"game_panel_api_events_dead_lettered_total", "Cumulative events moved to the dead-letter queue.", em.EventsDeadLetteredTotal},
			}
			for _, counter := range eventCounters {
				body.WriteString("# HELP " + counter.name + " " + counter.help + "\n")
				body.WriteString("# TYPE " + counter.name + " counter\n")
				body.WriteString(counter.name + " " + strconv.FormatUint(counter.value, 10) + "\n")
			}
			if len(em.EventsByType) > 0 {
				body.WriteString("# HELP game_panel_api_events_by_type_total Events published per event type.\n")
				body.WriteString("# TYPE game_panel_api_events_by_type_total counter\n")
				for eventType, count := range em.EventsByType {
					labels := "event_type=\"" + strings.ReplaceAll(eventType, `"`, `\"`) + "\""
					body.WriteString("game_panel_api_events_by_type_total{" + labels + "} " + strconv.FormatUint(count, 10) + "\n")
				}
			}
		}

		c.Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		return c.SendString(body.String())
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
		// Replay protection: heartbeats must carry a fresh timestamp (within
		// ±5 minutes), a one-time nonce, and an HMAC over method, URI,
		// timestamp, nonce, and body keyed with the node token — the same
		// scheme /api/remote endpoints use.
		if err := verifyRemoteHMAC(c, token, nodeHeartbeatNonces); err != nil {
			return fiber.NewError(fiber.StatusForbidden, err.Error())
		}
		var req NodeHeartbeatRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		heartbeat := store.NodeHeartbeatRequest{
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
		}
		node, err := cfg.Store.UpdateNodeHeartbeat(ctx, c.Params("id"), heartbeat)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if cfg.Observability != nil {
			cfg.Observability.RecordNodeHeartbeat(ctx, node, heartbeat)
		}
		if cfg.HeartbeatMonitor != nil {
			if evaluation, evalErr := cfg.HeartbeatMonitor.EvaluateNode(ctx, node.ID); evalErr == nil {
				node = evaluation.Node
			}
		}
		return c.JSON(fiber.Map{"ok": true, "node": node})
	})

	// POST /api/v1/nodes/capabilities — Beacon capability report webhook.
	// Beacon signs the request with the same HMAC scheme as the heartbeat
	// (method, URI, timestamp, nonce, body keyed with the node token), so the
	// route authenticates with VerifyNodeToken + verifyRemoteHMAC instead of
	// the panel JWT middleware.
	v1.Post("/nodes/capabilities", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		token := strings.TrimPrefix(c.Get("Authorization"), "Bearer ")
		if token == c.Get("Authorization") {
			token = c.Get("X-Node-Token")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var report struct {
			NodeID        string          `json:"nodeId"`
			BeaconVersion string          `json:"beaconVersion"`
			OS            string          `json:"os"`
			Architecture  string          `json:"architecture"`
			CPUThreads    int             `json:"cpuThreads"`
			MemoryMB      uint64          `json:"memoryMb"`
			DiskMB        uint64          `json:"diskMb"`
			UptimeSeconds int64           `json:"uptimeSeconds"`
			Capabilities  json.RawMessage `json:"capabilities"`
			RuntimeInfo   *struct {
				DockerVersion   string `json:"dockerVersion"`
				DockerAvailable bool   `json:"dockerAvailable"`
				DockerStatus    string `json:"dockerStatus"`
				RuntimeProvider string `json:"runtimeProvider"`
			} `json:"runtimeInfo"`
			BuildInfo *struct {
				DockerBuildEnabled bool `json:"dockerBuildEnabled"`
				NixpacksEnabled    bool `json:"nixpacksEnabled"`
			} `json:"buildInfo"`
			ComposeInfo *struct {
				ComposeVersion string `json:"composeVersion"`
				ComposeEnabled bool   `json:"composeEnabled"`
				StackCount     int    `json:"stackCount"`
			} `json:"composeInfo"`
			StorageInfo *struct {
				LocalBackups    bool `json:"localBackups"`
				S3Backups       bool `json:"s3Backups"`
				TransferEnabled bool `json:"transferEnabled"`
			} `json:"storageInfo"`
			GatewayInfo *struct {
				SFTPEnabled      bool `json:"sftpEnabled"`
				WebSocketEnabled bool `json:"webSocketEnabled"`
				ConsoleEnabled   bool `json:"consoleEnabled"`
			} `json:"gatewayInfo"`
			DatabaseInfo *struct {
				ProvisioningEnabled bool `json:"provisioningEnabled"`
			} `json:"databaseInfo"`
			FetchedAt string `json:"fetchedAt"`
		}
		if err := c.BodyParser(&report); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid report")
		}
		if strings.TrimSpace(report.NodeID) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "nodeId is required")
		}
		// Auth checks mirror /nodes/:id/heartbeat: node token first, then HMAC.
		if err := verifyNodeTokenWithHMAC(ctx, cfg, c, report.NodeID, token); err != nil {
			return err
		}
		fetchedAt := time.Now().UTC()
		if report.FetchedAt != "" {
			if t, err := time.Parse(time.RFC3339, report.FetchedAt); err == nil {
				fetchedAt = t
			}
		}
		var runtimeStatus, runtimeVersion, runtimeProvider, composeVersion string
		var stackCount int
		if report.RuntimeInfo != nil {
			runtimeStatus = report.RuntimeInfo.DockerStatus
			runtimeVersion = report.RuntimeInfo.DockerVersion
			runtimeProvider = report.RuntimeInfo.RuntimeProvider
		}
		if report.ComposeInfo != nil {
			composeVersion = report.ComposeInfo.ComposeVersion
			stackCount = report.ComposeInfo.StackCount
		}
		nc := &store.NodeCapability{
			NodeID:                      report.NodeID,
			BeaconVersion:               report.BeaconVersion,
			OS:                          report.OS,
			Architecture:                report.Architecture,
			CPUThreads:                  report.CPUThreads,
			MemoryMB:                    int64(report.MemoryMB),
			DiskMB:                      int64(report.DiskMB),
			UptimeSeconds:               report.UptimeSeconds,
			RawReport:                   report.Capabilities,
			FetchedAt:                   fetchedAt,
			RuntimeAvailable:            report.RuntimeInfo != nil && report.RuntimeInfo.DockerAvailable,
			RuntimeStatus:               runtimeStatus,
			RuntimeVersion:              runtimeVersion,
			RuntimeProvider:             runtimeProvider,
			DockerBuildEnabled:          report.BuildInfo != nil && report.BuildInfo.DockerBuildEnabled,
			NixpacksEnabled:             report.BuildInfo != nil && report.BuildInfo.NixpacksEnabled,
			ComposeEnabled:              report.ComposeInfo != nil && report.ComposeInfo.ComposeEnabled,
			ComposeVersion:              composeVersion,
			StackCount:                  stackCount,
			LocalBackups:                report.StorageInfo != nil && report.StorageInfo.LocalBackups,
			S3Backups:                   report.StorageInfo != nil && report.StorageInfo.S3Backups,
			TransferEnabled:             report.StorageInfo != nil && report.StorageInfo.TransferEnabled,
			SFTPEnabled:                 report.GatewayInfo != nil && report.GatewayInfo.SFTPEnabled,
			WebSocketEnabled:            report.GatewayInfo != nil && report.GatewayInfo.WebSocketEnabled,
			ConsoleEnabled:              report.GatewayInfo != nil && report.GatewayInfo.ConsoleEnabled,
			DatabaseProvisioningEnabled: report.DatabaseInfo != nil && report.DatabaseInfo.ProvisioningEnabled,
		}
		if err := cfg.Store.UpsertNodeCapability(ctx, nc); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"accepted": true})
	})

	type CheckpointRequest struct {
		ConfirmationToken string `json:"confirmationToken"`
		Code              string `json:"code"`
		RecoveryToken     string `json:"recoveryToken"`
	}

	v1.Post("/auth/login", publicMutationOriginCheck(LoadSessionCookieConfig()), authLimiter, CaptchaMiddleware(cfg), func(c *fiber.Ctx) error {
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
			if err := cfg.Store.AppendAudit(ctx, nil, "login.failed", "user", nil, safeAuditMeta(map[string]string{"email": req.Email})); err != nil {
				cfg.Logger.Error("audit append failed", "action", "login.failed", "error", err)
			}
			return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
		}
		clearLoginFailures(ctx, cfg, c, req.Email)
		if err := cfg.Store.AppendAudit(ctx, &user.ID, "login.success", "user", &user.ID, "{}"); err != nil {
			cfg.Logger.Error("audit append failed", "action", "login.success", "error", err)
		}

		if user.UseTOTP {
			confToken, err := issue2FAConfirmationToken(cfg.AuthSecret, user.ID, c.IP(), c.Get("User-Agent"))
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "could not issue confirmation token")
			}
			return c.JSON(fiber.Map{
				"complete":          false,
				"confirmationToken": confToken,
			})
		}

		token, err := issueConfiguredToken(cfg, user)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not issue token")
		}
		recordUserSession(ctx, cfg, c, token)

		// Always set HttpOnly session and CSRF cookies for browser clients
		csrfToken, err := generateCSRFToken()
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not generate CSRF token")
		}
		expires := tokenExpiry(cfg)
		setSessionCookies(c, token, csrfToken, expires)

		return c.JSON(fiber.Map{
			"complete": true,
			"user":     user,
		})
	})

	v1.Post("/auth/login/checkpoint", publicMutationOriginCheck(LoadSessionCookieConfig()), authLimiter, func(c *fiber.Ctx) error {
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

		claims, err := parse2FAConfirmationToken(cfg.AuthSecret, req.ConfirmationToken)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, err.Error())
		}

		// The confirmation token is bound to the IP/user agent it was issued
		// to; replaying it from another client is rejected.
		if claims.IP != "" && claims.IP != c.IP() {
			return fiber.NewError(fiber.StatusUnauthorized, "confirmation token was issued to a different client")
		}
		if claims.UA != "" && claims.UA != truncateUserAgent(c.Get("User-Agent")) {
			return fiber.NewError(fiber.StatusUnauthorized, "confirmation token was issued to a different client")
		}

		// Per-account brute-force lockout (10 failures → 15 minutes).
		if err := checkTwoFactorLockout(c.Context(), cfg, claims.Sub); err != nil {
			return err
		}

		// Single-use: consume the token's jti before verification so the same
		// token cannot be replayed to brute force the code.
		if !consume2FAConfirmation(c.Context(), cfg, claims.JTI, twoFactorTokenTTL) {
			return fiber.NewError(fiber.StatusUnauthorized, "confirmation token has already been used")
		}

		ctx, cancel := requestContext()
		defer cancel()

		// The confirmation token alone proves nothing beyond knowledge of the
		// password: it is handed to the client by /auth/login. The actual 2FA
		// factor (TOTP code or single-use recovery/backup code) must be
		// verified here before a session can be minted.
		if req.Code == "" && req.RecoveryToken == "" {
			return fiber.NewError(fiber.StatusBadRequest, "verification code is required")
		}
		if err := cfg.Store.VerifyTwoFactorCheckpoint(ctx, claims.Sub, req.Code, req.RecoveryToken); err != nil {
			recordTwoFactorFailure(c.Context(), cfg, claims.Sub)
			return fiber.NewError(fiber.StatusUnauthorized, err.Error())
		}
		clearTwoFactorFailures(ctx, cfg, claims.Sub)

		user, err := cfg.Store.GetUserByID(ctx, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not retrieve user details")
		}

		token, err := issueConfiguredToken(cfg, user)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not issue token")
		}
		recordUserSession(ctx, cfg, c, token)

		// Always set HttpOnly session and CSRF cookies for browser clients
		csrfToken, err := generateCSRFToken()
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not generate CSRF token")
		}
		expires := tokenExpiry(cfg)
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
		newToken, err := issueConfiguredToken(cfg, store.User{ID: current.Sub, Email: current.Email, Role: current.Role, SessionVersion: current.SessionVersion})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not issue session token")
		}
		if claims.JTI != "" {
			_ = cfg.Store.RevokeJWT(ctx, claims.JTI, time.Unix(claims.Exp, 0))
		}
		csrfToken, err := generateCSRFToken()
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not generate CSRF token")
		}
		expires := tokenExpiry(cfg)
		setSessionCookies(c, newToken, csrfToken, expires)
		return c.SendStatus(fiber.StatusNoContent)
	})

	v1.Get("/servers/:id/ws/stats", requireRealtimeServices(cfg), fiberws.New(realtimeProxy(cfg, wsTickets, "stats"), fiberws.Config{
		RecoverHandler: func(conn *fiberws.Conn) {
			defer func() {
				if err := recover(); err != nil {
					_ = conn.WriteJSON(fiber.Map{"error": "internal error"})
					_ = conn.Close()
				}
			}()
		},
		Origins: getWebSocketAllowedOrigins(cfg),
	}))
	v1.Get("/servers/:id/ws/logs", requireRealtimeServices(cfg), fiberws.New(realtimeProxy(cfg, wsTickets, "logs"), fiberws.Config{
		RecoverHandler: func(conn *fiberws.Conn) {
			defer func() {
				if err := recover(); err != nil {
					_ = conn.WriteJSON(fiber.Map{"error": "internal error"})
					_ = conn.Close()
				}
			}()
		},
		Origins: getWebSocketAllowedOrigins(cfg),
	}))
	v1.Get("/servers/:id/ws/console", requireRealtimeServices(cfg), fiberws.New(realtimeProxy(cfg, wsTickets, "console"), fiberws.Config{
		RecoverHandler: func(conn *fiberws.Conn) {
			defer func() {
				if err := recover(); err != nil {
					_ = conn.WriteJSON(fiber.Map{"error": "internal error"})
					_ = conn.Close()
				}
			}()
		},
		Origins: getWebSocketAllowedOrigins(cfg),
	}))

	// POST /api/edge/connect — Beacon edge-agent registration. The beacon
	// posts here (Bearer node token, no HMAC) when it wants a live edge
	// channel for console/control-plane events.
	app.Post("/api/edge/connect", apiIPAccess, mtlsMw, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		token := strings.TrimPrefix(c.Get("Authorization"), "Bearer ")
		if token == c.Get("Authorization") {
			token = c.Get("X-Node-Token")
		}
		var req struct {
			NodeID string `json:"nodeId"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"connected": false,
				"message":   "invalid request body",
			})
		}
		if strings.TrimSpace(req.NodeID) == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"connected": false,
				"message":   "nodeId is required",
			})
		}
		ctx, cancel := requestContext()
		defer cancel()
		ok, err := cfg.Store.VerifyNodeToken(ctx, req.NodeID, token)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "token verification failed")
		}
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"connected": false,
				"message":   "invalid node token",
			})
		}
		return c.JSON(fiber.Map{
			"connected": true,
			"nodeId":    req.NodeID,
			"message":   "connected",
		})
	})

	remote := app.Group("/api/remote", apiIPAccess, mtlsMw, remoteNodeMiddleware(cfg, nodeRegistry))
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
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
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
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		belongs, err := cfg.Store.ServerBelongsToNode(ctx, c.Params("id"), node.ID)
		if err != nil || !belongs {
			return fiber.NewError(fiber.StatusForbidden, "requesting node cannot access this server")
		}
		if body.ActualState != "" {
			if err := cfg.Store.SetServerActualState(ctx, c.Params("id"), store.ServerActualState(body.ActualState), body.Status); err != nil {
				cfg.Logger.Error("failed to set server actual state", "serverId", c.Params("id"), "error", err)
			}
		}
		if body.Status != "" {
			if err := cfg.Store.SetServerStatus(ctx, c.Params("id"), body.Status, body.Error); err != nil {
				cfg.Logger.Error("failed to set server status", "serverId", c.Params("id"), "error", err)
			}
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
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
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
		if err := cfg.Store.AppendAudit(ctx, nil, body.Action, "server", &serverID, body.Metadata); err != nil {
			cfg.Logger.Error("audit append failed", "action", body.Action, "error", err)
		}
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
		crashCtx := cfg.BackgroundContext
		if crashCtx == nil {
			crashCtx = context.Background()
		}
		_, err := cfg.Store.Exec(crashCtx, `INSERT INTO server_crash_events
			(id, server_id, node_id, exit_code, oom_killed, auto_restarted, created_at)
			SELECT $1, s.id, s.node_id, $3, $4, $5, NOW() FROM servers s WHERE s.id = $2`,
			crashID, c.Params("id"), req.ExitCode, req.OOMKilled, req.AutoRestart)
		if err != nil {
			return respondInternalError(c, err)
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
	v1.Post("/oauth2/token", authLimiter, IssueOAuth2Token(cfg))
	v1.Post("/oauth/token", authLimiter, IssueOAuth2Token(cfg)) // alias

	// Social authentication (Discord, Steam, Authentik)
	registerSocialAuthRoutes(v1, cfg, mutationLimiter, authLimiter)

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
		// Dual-session-model guard: the panel issues JWT session cookies signed
		// with AuthSecret. Opaque sessions are only consulted when the cookie
		// does NOT parse as a panel JWT, so the JWT model keeps working and the
		// opaque middleware no longer 401s every authenticated request.
		sessMw = auth.SessionMiddleware(cfg.SessionStore, func(token string) bool {
			if cfg.AuthSecret == "" {
				return false
			}
			_, err := parseToken(cfg.AuthSecret, token)
			return err == nil
		})
	} else {
		sessMw = func(c *fiber.Ctx) error { return c.Next() }
	}

	methodLimiter := func(c *fiber.Ctx) error {
		if c.Method() == fiber.MethodGet || c.Method() == fiber.MethodHead {
			return readLimiter(c)
		}
		return mutationLimiter(c)
	}
	protected := v1.Group("", authMiddleware(cfg.AuthSecret, cfg.Store), sessMw, requireTwoFactorAuthentication(cfg), csrfMiddleware(LoadSessionCookieConfig()), methodLimiter)
	protected.Post("/servers/:id/ws/ticket", IssueWSTicket(cfg, wsTickets))
	protected.Post("/servers/:id/files/download-ticket", mutationLimiter, issueFileDownloadTicket(cfg, fileDownloadTickets))
	protected.Post("/servers/:id/backups/download-ticket", mutationLimiter, issueBackupDownloadTicket(cfg, fileDownloadTickets))

	// Get signed download URL for a file
	protected.Get("/servers/:id/files/download-url", requireServerPermission(cfg, store.PermFileReadContent), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		_ = ctx

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
		if err := cfg.Store.AppendAudit(ctx, actorID, "server:console.command", "server", &target.ServerID, safeAuditMeta(map[string]string{"command": body.Command})); err != nil {
			cfg.Logger.Error("audit append failed", "action", "server:console.command", "error", err)
		}
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

	protected.Get("/servers/:id/users/:userId", requireRole("admin"), requireAdminScope("servers.read"), func(c *fiber.Ctx) error {
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
	registerAlertRoutes(protected, cfg.AlertService, cfg.Observability, mutationLimiter)
	registerNotificationRoutes(protected, cfg.NotificationService, mutationLimiter)
	if cfg.EnhancedNotificationService != nil {
		registerEnhancedNotificationRoutes(protected, cfg.EnhancedNotificationService, mutationLimiter)
	}
	registerMailSettingsRoutes(protected, cfg, mutationLimiter, adminIPAccess)
	registerSFTPRoutes(protected, cfg, mutationLimiter)
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
	RegisterGitDeploymentRoutes(protected, cfg, mutationLimiter)
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
		registerCronJobRoutes(protected, cfg, cfg.CronJobService, mutationLimiter)
	}

	// Procfile process management
	if cfg.ProcessService != nil {
		registerProcessRoutes(protected, cfg, cfg.ProcessService, mutationLimiter)
	}

	// Proxy domain management (reverse proxy level, distinct from per-server domains)
	registerProxyDomainRoutes(protected, cfg, adminIPAccess, mutationLimiter)
	registerProxyCertificateRoutes(protected, cfg, adminIPAccess, mutationLimiter)
	registerSecurityHeadersRoutes(protected, cfg, adminIPAccess, mutationLimiter)
	registerRedirectRulesRoutes(protected, cfg, adminIPAccess, mutationLimiter)

	// Host management
	registerHostRoutes(protected, cfg)
	registerFirewallRoutes(protected, cfg, mutationLimiter)

	// Cluster membership + cleanup routes
	registerClusterMembershipRoutes(protected, cfg.ClusterMembershipService, mutationLimiter)
	registerCleanupRoutes(protected, cfg.CleanupService, mutationLimiter)

	// Cross-node routing and service discovery routes
	registerServiceDiscoveryRoutes(protected, cfg, cfg.ServiceDiscovery, adminIPAccess, mutationLimiter)
	registerCrossNodeRoutes(protected, cfg, cfg.CrossNodeResolver, cfg.IngressSynchronizer, adminIPAccess, mutationLimiter)

	// Phase registrars (each of the 8 build phases registers here)
	registerPhaseHooks(v1, protected, &cfg)

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
