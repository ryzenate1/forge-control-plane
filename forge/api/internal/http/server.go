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
	gpruntime "gamepanel/forge/internal/runtime"
	"gamepanel/forge/internal/services"
	acmesvc "gamepanel/forge/internal/services/acme"
	"gamepanel/forge/internal/services/activity"
	alerting "gamepanel/forge/internal/services/alerting"
	apphostingsvc "gamepanel/forge/internal/services/apphosting"
	appstoresvc "gamepanel/forge/internal/services/appstore"
	"gamepanel/forge/internal/services/auditlog"
	"gamepanel/forge/internal/services/autoscaler"
	"gamepanel/forge/internal/services/backup"
	backupenginesvc "gamepanel/forge/internal/services/backupengine"
	billingsvc "gamepanel/forge/internal/services/billing"
	"gamepanel/forge/internal/services/build"
	buildpacksvc "gamepanel/forge/internal/services/buildpack"
	catalogsvc "gamepanel/forge/internal/services/catalog"
	cleanupsvc "gamepanel/forge/internal/services/cleanup"
	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/services/clustermembership"
	composesvc "gamepanel/forge/internal/services/compose"
	composetemplatessvc "gamepanel/forge/internal/services/composetemplates"
	"gamepanel/forge/internal/services/crashdetector"
	cronjobsvc "gamepanel/forge/internal/services/cronjob"
	"gamepanel/forge/internal/services/crossnode"
	dbbackupsvc "gamepanel/forge/internal/services/dbbackup"
	"gamepanel/forge/internal/services/dbprovisioner"
	"gamepanel/forge/internal/services/deployment"
	dnssvc "gamepanel/forge/internal/services/dns"
	"gamepanel/forge/internal/services/domains"
	domainsenv "gamepanel/forge/internal/services/domainsenv"
	drainsvc "gamepanel/forge/internal/services/drain"
	eggseeder "gamepanel/forge/internal/services/eggseeder"
	envaffinitysvc "gamepanel/forge/internal/services/envaffinity"
	"gamepanel/forge/internal/services/environments"
	envgroupssvc "gamepanel/forge/internal/services/envgroups"
	envmanifestsvc "gamepanel/forge/internal/services/envmanifest"
	envvarsvc "gamepanel/forge/internal/services/envvars"
	"gamepanel/forge/internal/services/evacuationplanner"
	"gamepanel/forge/internal/services/failover"
	fencingsvc "gamepanel/forge/internal/services/fencing"
	"gamepanel/forge/internal/services/forgefile"
	gitsvc "gamepanel/forge/internal/services/git"
	gitpushsvc "gamepanel/forge/internal/services/gitpush"
	"gamepanel/forge/internal/services/gitprovider"
	"gamepanel/forge/internal/services/health"
	healthchecksvc "gamepanel/forge/internal/services/healthcheckrunner"
	"gamepanel/forge/internal/services/heartbeatmonitor"
	"gamepanel/forge/internal/services/i18n"
	incussvc "gamepanel/forge/internal/services/incus"
	installersvc "gamepanel/forge/internal/services/installer"
	"gamepanel/forge/internal/services/loadbalancer"
	mailservice "gamepanel/forge/internal/services/mail"
	"gamepanel/forge/internal/services/migration"
	mountssvc "gamepanel/forge/internal/services/mounts"
	netbirdsvc "gamepanel/forge/internal/services/netbird"
	"gamepanel/forge/internal/services/nodeautoscale"
	"gamepanel/forge/internal/services/nodeprobe"
	"gamepanel/forge/internal/services/noderegistry"
	nomadsvc "gamepanel/forge/internal/services/nomad"
	notificationsvc "gamepanel/forge/internal/services/notification"
	enhancednotifsvc "gamepanel/forge/internal/services/notifications"
	"gamepanel/forge/internal/services/observability"
	"gamepanel/forge/internal/services/onboarding"
	operationsvc "gamepanel/forge/internal/services/operation"
	"gamepanel/forge/internal/services/pipeline"
	"gamepanel/forge/internal/services/plugins"
	previewenvsvc "gamepanel/forge/internal/services/previewenv"
	phase1gitsvc "gamepanel/forge/internal/services/phase1git"
	proceduresvc "gamepanel/forge/internal/services/procedure"
	processsvc "gamepanel/forge/internal/services/process"
	"gamepanel/forge/internal/services/queue"
	"gamepanel/forge/internal/services/reconciler"
	recoverysvc "gamepanel/forge/internal/services/recovery"
	registrationssvc "gamepanel/forge/internal/services/registrations"
	replicamanager "gamepanel/forge/internal/services/replicamanager"
	redirectssvc "gamepanel/forge/internal/services/redirects"
	"gamepanel/forge/internal/services/reservations"
	"gamepanel/forge/internal/services/resourcelimits"
	runtimesvc "gamepanel/forge/internal/services/runtime"
	scheduledtaskssvc "gamepanel/forge/internal/services/scheduledtasks"
	"gamepanel/forge/internal/services/scheduler"
	"gamepanel/forge/internal/services/servicediscovery"
	"gamepanel/forge/internal/services/tenancy"
	"gamepanel/forge/internal/services/trafficmanager"
	upgradesvc "gamepanel/forge/internal/services/upgrade"
	"gamepanel/forge/internal/services/vaultprovider"
	"gamepanel/forge/internal/services/webauthn"
	webhooksvc "gamepanel/forge/internal/services/webhook"
	"gamepanel/forge/internal/services/zerodowntime"
	"gamepanel/forge/internal/store"

	fiberws "github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
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
	// WorkloadRuntime is the runtime dispatcher the create-workload surface
	// reports from. It is the authority on which engines can actually be
	// dispatched to; offering a runtime with no adapter behind it would put a
	// control in the UI that cannot do its job. Nil-safe: the endpoint degrades
	// to reporting only what the provider allow-list allows.
	WorkloadRuntime *gpruntime.MultiRuntimeAdapter
	// IncusService drives container/VM hosts (runtime=incus) over the Incus
	// REST API; NomadService fronts a Nomad workload orchestrator control plane.
	// Both are nil-safe — routes return 503 when unset.
	IncusService    *incussvc.Service
	NomadService    *nomadsvc.Service
	WebAuthnService *webauthn.Service
	EventRelay      *eventstore.Relay
	EventRegistry   *events.Registry

	BackupSvc                  *backup.Service
	AutoScaler                 *autoscaler.Service
	NodeAutoscaler             *nodeautoscale.Service
	CrashDetector              *crashdetector.Detector
	DeploymentSvc              *deployment.Service
	PreviewDeploymentSvc       previewDeploymentService
	CloudManager               *cloud.Manager
	AcmeService                *acmesvc.Service
	LoadBalancer               *loadbalancer.Service
	FailoverSvc                *failover.Service
	TrafficManager             *trafficmanager.Service
	CaddyTLS                   *trafficmanager.CaddyTLSManager
	DomainService              *domains.Service
	DNSService                 *dnssvc.Service
	VaultService               *vaultprovider.Service
	DBContainerService         *dbprovisioner.DBContainerService
	DatabaseServiceProvisioner *services.DatabaseServiceProvisioner
	DBBackupService            *dbbackupsvc.Service
	PredictiveScorer           *scheduler.PredictiveScorer
	ConstraintScheduler        *scheduler.ConstraintScheduler
	GitService                 *gitsvc.Service
	GitDeployService           *gitsvc.DeployService
	GitDeployMgmtService       *gitsvc.DeploymentManagementService
	GitProviderService         *gitprovider.Service
	// GitPushService backs the Dokku-style "git push to deploy" flow: it owns
	// the per-app bare repository record and the HMAC-verified receive hook.
	// Nil-safe: without it the routes do not register.
	GitPushService *gitpushsvc.Service
	ComposeService             *composesvc.Service
	ComposeTemplateService     *composetemplatessvc.Service
	// ComposeGitOpsService owns git-backed stack deploy/redeploy/drift/webhook.
	// Injected via Config (handlers->services->store); when nil the compose
	// registrar falls back to inline construction for dev-mode.
	ComposeGitOpsService *composesvc.GitOpsService
	BuildService               *build.Service
	BuildpackService           *buildpacksvc.Service
	InstallerService           *installersvc.Service

	TenancyService  *tenancy.Service
	EnvVarService   *envvarsvc.Service
	EndpointService *environments.Service
	// Injected environment-engine + per-app surfaces so phase registrars do
	// not construct services inline. All nil-safe: registrars fall back to
	// local construction when the field is unset (dev/tests), preferring the
	// injected instance when present.
	MountService       *mountssvc.Service
	EnvManifestService *envmanifestsvc.Service
	EnvGroupsService   *envgroupssvc.Service
	RedirectService    *redirectssvc.Service
	DomainsEnvService  *domainsenv.Service

	AppHostingService *apphostingsvc.Service

	ProcedureService *proceduresvc.Service

	// BackupEngineService adds Restic and Kopia as selectable backup engines on
	// top of the classic backup pipeline. Nil-safe: the /admin/backup-engines
	// routes are registered only when main populates the field.
	BackupEngineService *backupenginesvc.Service

	// PipelineService drives multi-stage CI/CD pipelines (definitions, runs,
	// logs, artifacts, manual approvals). Previously unwired to any route.
	PipelineService   *pipeline.Service
	ReplicaManager    *replicamanager.Manager
	AppStoreService   *appstoresvc.Service
	CatalogService    *catalogsvc.Service
	ForgefileSvc      *forgefile.Service
	OnboardingService *onboarding.Service

	AlertService                *alerting.Service
	NotificationService         *notificationsvc.Service
	EnhancedNotificationService *enhancednotifsvc.Service
	// NotificationRouter is the notifications engine (channels + subscriptions
	// + templated fan-out). Previously the plural package was only reachable via
	// the dormant enhanced service; the engine gives it a live wiring.
	NotificationRouter   *enhancednotifsvc.Router
	CronJobService       *cronjobsvc.Service
	ScheduledTaskService *scheduledtaskssvc.Service
	ProcessService       *processsvc.Service
	ZeroDowntimeSvc      *zerodowntime.Service

	ClusterMembershipService *clustermembership.Service
	CleanupService           *cleanupsvc.Service

	// Orphan-wiring additions: services that previously lived behind internal
	// callers only and now back admin endpoints (see handlers_billing.go and
	// handlers_placement.go). All are nil-safe: routes are registered only when
	// the field is populated in main.
	BillingService   *billingsvc.Service
	PlacementService *envaffinitysvc.EnvAffinity
	// DrainLedger is the durable drain-progress recorder. It reads only; the
	// begin/cancel orchestration stays owned by clustermembership, so there is
	// no route collision.
	DrainLedger *drainsvc.Service
	// HealthCheckRunner is the live target-health prober already started in
	// main; exposed read-only for the admin health dashboard.
	HealthCheckRunner *healthchecksvc.Service

	// FencingSvc bumps workload generations on a node when it recovers from an
	// unexpected offline; exposed for manual admin fence operations.
	FencingSvc *fencingsvc.Service
	// UpgradeSvc is the control-plane self-upgrade orchestrator (version check,
	// plan creation, execution with backup + rollback, health verification).
	UpgradeSvc *upgradesvc.Service

	// Cross-node routing services
	ServiceDiscovery    *servicediscovery.Service
	CrossNodeResolver   *crossnode.Resolver
	IngressSynchronizer *crossnode.IngressSynchronizer

	// NetBirdService is the WireGuard mesh VPN control plane client. It is
	// nil-safe: without NETBIRD_API_URL/NETBIRD_API_TOKEN it reads empty and
	// rejects mutations, so the admin routes always register cleanly.
	NetBirdService *netbirdsvc.Service

	// Preview + onboarding surfaces: injected so registrars do not build them
	// inline. PreviewEnvService backs /projects/:id/previews; Phase1GitBridge
	// backs /git/*; RegistrationService/EggSeederService/WebhookService are
	// the wiring points main populates for node onboarding/seed/webhook flows.
	// All nil-safe: registrars fall back to local construction when unset.
	PreviewEnvService   *previewenvsvc.Service
	Phase1GitBridge     *phase1gitsvc.Bridge
	RegistrationService *registrationssvc.Service
	EggSeederService    *eggseeder.Service
	WebhookService      *webhooksvc.Service

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
		loginRateLimitKey("ip", ExtractClientIP(c)),
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
		loginRateLimitKey("ip", ExtractClientIP(c)),
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
		loginRateLimitKey("ip", ExtractClientIP(c)),
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
	_, _ = cfg.Store.CreateUserSession(ctx, claims.Sub, hex.EncodeToString(sum[:]), ExtractClientIP(c), c.Get("User-Agent"), configuredTokenTTL(cfg))
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
	// RuntimeProvider selects the workload engine (docker, containerd, podman,
	// firecracker, kubernetes, kvm, lxc). Empty means "use the node default"
	// which resolves to docker. Placement filters nodes by this field.
	RuntimeProvider string `json:"runtimeProvider"`
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
	Version         string  `json:"version"`
	OS              string  `json:"os"`
	Architecture    string  `json:"architecture"`
	CPUThreads      int     `json:"cpuThreads"`
	MemoryMB        int     `json:"memoryMb"`
	DiskMB          int     `json:"diskMb"`
	DockerStatus    string  `json:"dockerStatus,omitempty"`
	RuntimeStatus   string  `json:"runtimeStatus"`
	RuntimeProvider string  `json:"runtimeProvider"`
	Error           string  `json:"error"`
	LoadAverage     float64 `json:"load_average"`
	Uptime          int64   `json:"uptime_seconds"`
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

	// Fail fast on an insecure session-cookie configuration: without Secure
	// cookies (or with SameSite=None over HTTP) session and CSRF cookies can
	// be intercepted or sent cross-site. This is a startup panic, not a
	// request-time error, so a misconfigured production panel never serves.
	if err := ValidateSessionCookieConfig(LoadSessionCookieConfig(), cfg.AppEnv); err != nil {
		panic("invalid session cookie configuration: " + err.Error())
	}

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
			corsCfg.AllowHeaders = "Origin,Content-Type,Accept,Authorization,X-API-Key,X-CSRF-Token,X-Forge-Session-Mode"
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

	// HTTP RED metrics (rate/errors/duration). Mounted here so the collector
	// observes every request and /metrics can emit http_requests_total /
	// http_request_duration_seconds without a second instrumentation path.
	httpMetrics := NewMetricsCollector()
	app.Use(MetricsMiddleware(httpMetrics))

	registerSwaggerRoutes(app, cfg.AppEnv)

	registerWellKnownVerifyRoute(app, cfg.DomainService)

	// ACME HTTP-01 challenge solver. Let's Encrypt fetches
	// http://<domain>/.well-known/acme-challenge/<token>; without this mounted the
	// default HTTP-01 issuance path can never validate. Served by the acme
	// service's in-memory challenger (public, no auth).
	if cfg.AcmeService != nil {
		app.All("/.well-known/acme-challenge/*", adaptor.HTTPHandler(cfg.AcmeService.HTTPSolver()))
	}

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
	v1.Get("/csrf-token", GetCSRFTokenHandler(cfg))
	// Session exchange endpoint — exchanges a single-use code for a session token.
	// Used after social auth redirects to avoid placing the token in the URL.
	// Rate-limited and origin-checked like the rest of the public auth surface;
	// without a limiter a stolen exchange code could be brute-forced.
	v1.Post("/auth/session/exchange", publicMutationOriginCheck(LoadSessionCookieConfig()), authLimiter, ExchangeCodeHandler(cfg))

	// Liveness, readiness, and diagnostics handlers exposed both on /api/v1/health
	// and directly at root /health for external health probes, load balancers, and scripts.
	liveHandler := func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	}
	readyHandler := func(c *fiber.Ctx) error {
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
	}
	healthHandler := func(c *fiber.Ctx) error {
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
	}

	app.Get("/health/live", liveHandler)
	app.Get("/health/ready", readyHandler)
	app.Get("/health", healthHandler)

	v1.Get("/health/live", liveHandler)
	v1.Get("/health/ready", readyHandler)
	v1.Get("/health", healthHandler)
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

		// Dark service metrics: expose the cumulative counters (and the handful
		// of instantaneous gauges) reported by long-lived services that
		// implement Metrics() but were previously unobserved. Every block is
		// nil-guarded so a partially configured API emits no missing series.
		// Series are named game_panel_api_<service>_<metric> and carry no
		// high-cardinality labels.
		type mCounter struct {
			name  string
			help  string
			value uint64
		}
		writeCounters := func(counters []mCounter) {
			for _, counter := range counters {
				body.WriteString("# HELP " + counter.name + " " + counter.help + "\n")
				body.WriteString("# TYPE " + counter.name + " counter\n")
				body.WriteString(counter.name + " " + strconv.FormatUint(counter.value, 10) + "\n")
			}
		}

		if cfg.ClusterMembershipService != nil {
			cmm := cfg.ClusterMembershipService.Metrics()
			writeCounters([]mCounter{
				{"game_panel_api_cluster_membership_nodes_joined_total", "Cumulative cluster nodes that joined.", cmm.NodesJoinedTotal},
				{"game_panel_api_cluster_membership_nodes_left_total", "Cumulative cluster nodes that left.", cmm.NodesLeftTotal},
				{"game_panel_api_cluster_membership_drain_started_total", "Cumulative node drains started.", cmm.DrainStartedTotal},
				{"game_panel_api_cluster_membership_drain_completed_total", "Cumulative node drains completed.", cmm.DrainCompletedTotal},
				{"game_panel_api_cluster_membership_maintenance_started_total", "Cumulative maintenance windows started.", cmm.MaintStartedTotal},
				{"game_panel_api_cluster_membership_maintenance_ended_total", "Cumulative maintenance windows ended.", cmm.MaintEndedTotal},
			})
		}

		if cfg.ReservationManager != nil {
			rm := cfg.ReservationManager.Metrics()
			writeCounters([]mCounter{
				{"game_panel_api_reservations_placement_total", "Cumulative placement reservations made.", rm.PlacementReservationsTotal},
				{"game_panel_api_reservations_conflicts_total", "Cumulative reservation conflicts.", rm.ReservationConflictsTotal},
				{"game_panel_api_reservations_expirations_total", "Cumulative reservation expirations.", rm.ReservationExpirationsTotal},
			})
		}

		if cfg.ReplicaManager != nil {
			rpm := cfg.ReplicaManager.Metrics()
			writeCounters([]mCounter{
				{"game_panel_api_replica_manager_create_app_total", "Cumulative replica apps created.", rpm.CreateAppTotal},
				{"game_panel_api_replica_manager_delete_app_total", "Cumulative replica apps deleted.", rpm.DeleteAppTotal},
				{"game_panel_api_replica_manager_scale_up_total", "Cumulative replica scale-up operations.", rpm.ScaleUpTotal},
				{"game_panel_api_replica_manager_scale_down_total", "Cumulative replica scale-down operations.", rpm.ScaleDownTotal},
				{"game_panel_api_replica_manager_replacement_total", "Cumulative replica replacements.", rpm.ReplacementTotal},
				{"game_panel_api_replica_manager_reservation_errors_total", "Cumulative replica reservation errors.", rpm.ReservationErrors},
				{"game_panel_api_replica_manager_dispatch_errors_total", "Cumulative replica dispatch errors.", rpm.DispatchErrors},
				{"game_panel_api_replica_manager_reconcile_total", "Cumulative replica reconciles.", rpm.ReconcileTotal},
				{"game_panel_api_replica_manager_no_double_reservation_total", "Cumulative double-reservation preventions.", rpm.NoDoubleReservation},
				{"game_panel_api_replica_manager_no_duplicate_alloc_total", "Cumulative duplicate-allocation preventions.", rpm.NoDuplicateAlloc},
			})
		}

		if cfg.RecoveryCoordinator != nil {
			rcm := cfg.RecoveryCoordinator.Metrics()
			writeCounters([]mCounter{
				{"game_panel_api_recovery_plans_total", "Cumulative recovery plans created.", rcm.RecoveryPlansTotal},
				{"game_panel_api_recovery_items_total", "Cumulative recovery items processed.", rcm.RecoveryItemsTotal},
				{"game_panel_api_recovery_failures_total", "Cumulative recovery failures.", rcm.RecoveryFailuresTotal},
			})
		}

		if cfg.EvacuationPlanner != nil {
			ep := cfg.EvacuationPlanner.Metrics()
			writeCounters([]mCounter{
				{"game_panel_api_evacuation_planner_plans_total", "Cumulative evacuation plans created.", ep.EvacuationPlansTotal},
				{"game_panel_api_evacuation_planner_candidates_total", "Cumulative evacuation candidates evaluated.", ep.EvacuationCandidatesTotal},
				{"game_panel_api_evacuation_planner_validation_failures_total", "Cumulative evacuation validation failures.", ep.EvacuationValidationFailuresTotal},
				{"game_panel_api_evacuation_planner_storage_local_skipped_total", "Cumulative local-storage workloads skipped during evacuation.", ep.StorageLocalSkippedTotal},
				{"game_panel_api_evacuation_planner_orphan_detection_total", "Cumulative orphan detections run.", ep.OrphanDetectionTotal},
			})
		}

		if cfg.CleanupService != nil {
			cl := cfg.CleanupService.Metrics()
			writeCounters([]mCounter{
				{"game_panel_api_cleanup_stale_reservations_cleaned_total", "Cumulative stale reservations cleaned.", cl.StaleReservationsCleaned},
				{"game_panel_api_cleanup_orphaned_allocations_cleaned_total", "Cumulative orphaned allocations cleaned.", cl.OrphanedAllocationsCleaned},
				{"game_panel_api_cleanup_runs_total", "Cumulative cleanup runs.", cl.CleanupRunsTotal},
				{"game_panel_api_cleanup_errors_total", "Cumulative cleanup errors.", cl.CleanupErrorsTotal},
			})
		}

		if cfg.AutoScaler != nil {
			as := cfg.AutoScaler.Metrics()
			writeCounters([]mCounter{
				{"game_panel_api_autoscaler_scale_up_events_total", "Cumulative autoscaler scale-up events.", as.ScaleUpEventsTotal},
				{"game_panel_api_autoscaler_scale_down_events_total", "Cumulative autoscaler scale-down events.", as.ScaleDownEventsTotal},
				{"game_panel_api_autoscaler_scaling_errors_total", "Cumulative autoscaler scaling errors.", as.ScalingErrorsTotal},
			})
			// ActivePolicies is an instantaneous count, not a cumulative counter.
			body.WriteString("# HELP game_panel_api_autoscaler_active_policies Current number of active autoscaling policies.\n")
			body.WriteString("# TYPE game_panel_api_autoscaler_active_policies gauge\n")
			body.WriteString("game_panel_api_autoscaler_active_policies " + strconv.Itoa(as.ActivePolicies) + "\n")
		}

		if cfg.FailoverSvc != nil {
			fo := cfg.FailoverSvc.Metrics()
			writeCounters([]mCounter{
				{"game_panel_api_failover_failures_detected_total", "Cumulative failover failures detected.", fo.FailuresDetected},
				{"game_panel_api_failover_evacuations_triggered_total", "Cumulative failover evacuations triggered.", fo.EvacuationsTriggered},
				{"game_panel_api_failover_restarts_triggered_total", "Cumulative failover restarts triggered.", fo.RestartsTriggered},
				{"game_panel_api_failover_notifications_sent_total", "Cumulative failover notifications sent.", fo.NotificationsSent},
			})
		}

		if cfg.MigrationService != nil {
			mg := cfg.MigrationService.Metrics()
			writeCounters([]mCounter{
				{"game_panel_api_migration_total", "Cumulative migrations started.", mg.MigrationTotal},
				{"game_panel_api_migration_completed_total", "Cumulative migrations completed.", mg.MigrationCompletedTotal},
				{"game_panel_api_migration_failed_total", "Cumulative migrations failed.", mg.MigrationFailedTotal},
				{"game_panel_api_migration_reconciliation_total", "Cumulative migration reconciliations.", mg.ReconciliationTotal},
				{"game_panel_api_migration_reconciliation_failures_total", "Cumulative migration reconciliation failures.", mg.ReconciliationFailures},
				{"game_panel_api_migration_cleanup_completed_total", "Cumulative migration cleanups completed.", mg.CleanupCompletedTotal},
			})
		}

		if cfg.HeartbeatMonitor != nil {
			hb := cfg.HeartbeatMonitor.Metrics()
			writeCounters([]mCounter{
				{"game_panel_api_heartbeat_monitor_evaluations_total", "Cumulative heartbeat evaluations.", hb.HeartbeatEvaluationsTotal},
				{"game_panel_api_heartbeat_monitor_nodes_suspected_total", "Cumulative nodes marked suspected.", hb.NodesSuspectedTotal},
				{"game_panel_api_heartbeat_monitor_nodes_unreachable_total", "Cumulative nodes marked unreachable.", hb.NodesUnreachableTotal},
				{"game_panel_api_heartbeat_monitor_nodes_offline_total", "Cumulative nodes marked offline.", hb.NodesOfflineTotal},
				{"game_panel_api_heartbeat_monitor_nodes_recovered_total", "Cumulative nodes recovered.", hb.NodesRecoveredTotal},
				{"game_panel_api_heartbeat_monitor_nodes_reconciling_total", "Cumulative nodes placed into reconciling.", hb.NodesReconcilingTotal},
				{"game_panel_api_heartbeat_monitor_nodes_unavailable_total", "Cumulative nodes marked unavailable.", hb.NodesUnavailableTotal},
			})
		}

		// HTTP request RED metrics recorded by MetricsMiddleware.
		body.WriteString(httpMetrics.FormatPrometheus())

		c.Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		return c.SendString(body.String())
	})

	v1.Post("/nodes/:id/heartbeat", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		// Node credentials travel in the Authorization header only. A second
		// header (X-Node-Token) would create two credential paths with
		// different logging/redaction handling; Beacon sends Bearer on every
		// node-authenticated call, so the fallback is removed, not widened.
		token := strings.TrimSpace(strings.TrimPrefix(c.Get("Authorization"), "Bearer "))
		if token == "" || token == strings.TrimSpace(c.Get("Authorization")) {
			return fiber.NewError(fiber.StatusUnauthorized, "missing daemon bearer token")
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
			LoadAverage:     req.LoadAverage,
			Uptime:          req.Uptime,
		}
		node, err := cfg.Store.UpdateNodeHeartbeat(ctx, c.Params("id"), heartbeat)
		if err != nil {
			// Unknown node is 404, not 400: the heartbeat path names a
			// resource, and a missing resource must not look like a bad
			// request. Internal text is never echoed; respondInternalError
			// handles prod sanitization.
			if strings.Contains(strings.ToLower(err.Error()), "not found") || strings.Contains(strings.ToLower(err.Error()), "no rows") {
				return fiber.NewError(fiber.StatusNotFound, "node not found")
			}
			return respondInternalError(c, err)
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
		// Node credentials travel in the Authorization header only. A second
		// header (X-Node-Token) would create two credential paths with
		// different logging/redaction handling; Beacon sends Bearer on every
		// node-authenticated call, so the fallback is removed, not widened.
		token := strings.TrimSpace(strings.TrimPrefix(c.Get("Authorization"), "Bearer "))
		if token == "" || token == strings.TrimSpace(c.Get("Authorization")) {
			return fiber.NewError(fiber.StatusUnauthorized, "missing daemon bearer token")
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
		if err := checkLoginRateLimit(ctx, cfg, c, req.Email); err != nil {
			return err
		}
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
			confToken, err := issue2FAConfirmationToken(cfg.AuthSecret, user.ID, ExtractClientIP(c), c.Get("User-Agent"))
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

		// Always set HttpOnly session and CSRF cookies for browser clients.
		// The CSRF token is bound to this session (HMAC) so it cannot be
		// replayed against a different session.
		expires := tokenExpiry(cfg)
		setSessionCookies(c, token, deriveSessionCSRFToken(cfg.AuthSecret, token), expires)

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
		// to; replaying it from another client is rejected. Both ends resolve the
		// address the same way, otherwise the binding either never matches or
		// matches whatever header the replaying client sends.
		if claims.IP != "" && claims.IP != ExtractClientIP(c) {
			return fiber.NewError(fiber.StatusUnauthorized, "confirmation token was issued to a different client")
		}
		if claims.UA != "" && claims.UA != truncateUserAgent(c.Get("User-Agent")) {
			return fiber.NewError(fiber.StatusUnauthorized, "confirmation token was issued to a different client")
		}

		// Per-account brute-force lockout (10 failures → 15 minutes).
		if err := checkTwoFactorLockout(c.Context(), cfg, claims.Sub); err != nil {
			return err
		}
		// The checkpoint is part of the login flow: the same IP+account
		// rate limit that guards /auth/login applies here so the TOTP step
		// cannot be used to bypass it.
		ctx0, cancel0 := requestContext()
		defer cancel0()
		if err := checkLoginRateLimit(ctx0, cfg, c, claims.Sub); err != nil {
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

		// Always set HttpOnly session and CSRF cookies for browser clients.
		// The CSRF token is bound to this session (HMAC).
		expires := tokenExpiry(cfg)
		setSessionCookies(c, token, deriveSessionCSRFToken(cfg.AuthSecret, token), expires)

		return c.JSON(fiber.Map{
			"complete": true,
			"user":     user,
		})
	})

	// Session refresh – re-issue cookie with new expiry. Rate-limited and
	// origin-checked like login; requires the double-submit CSRF token so a
	// cross-site POST cannot rotate a victim's session.
	v1.Post("/auth/session/refresh", publicMutationOriginCheck(LoadSessionCookieConfig()), authLimiter, func(c *fiber.Ctx) error {
		sessionToken, ok := getSessionCookie(c)
		if !ok || sessionToken == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session cookie")
		}
		// CSRF: the refresh cookie is HttpOnly, so the caller must prove
		// same-origin by echoing the session-bound CSRF cookie value in the
		// header. Constant-time, and bound to the presented session.
		csrfCfg := LoadSessionCookieConfig()
		csrfCookie := c.Cookies(secureCookieName(csrfCookieName, csrfCfg.Secure))
		csrfHeader := c.Get("X-CSRF-Token")
		if !validSessionBoundCSRF(cfg.AuthSecret, sessionToken, csrfCookie, csrfHeader) {
			return fiber.NewError(fiber.StatusForbidden, "invalid CSRF token")
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
		expires := tokenExpiry(cfg)
		setSessionCookies(c, newToken, deriveSessionCSRFToken(cfg.AuthSecret, newToken), expires)
		return c.SendStatus(fiber.StatusNoContent)
	})

	v1.Get("/servers/:id/ws/stats", requireRealtimeServices(cfg), readLimiter, fiberws.New(realtimeProxy(cfg, wsTickets, "stats"), fiberws.Config{
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
	v1.Get("/servers/:id/ws/logs", requireRealtimeServices(cfg), readLimiter, fiberws.New(realtimeProxy(cfg, wsTickets, "logs"), fiberws.Config{
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
	v1.Get("/servers/:id/ws/console", requireRealtimeServices(cfg), readLimiter, fiberws.New(realtimeProxy(cfg, wsTickets, "console"), fiberws.Config{
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
	// Backup progress streaming — proxies Beacon's GET /servers/:id/ws/backup so
	// the UI receives live backup/restore progress instead of polling every 3s.
	v1.Get("/servers/:id/ws/backup", requireRealtimeServices(cfg), readLimiter, fiberws.New(realtimeProxy(cfg, wsTickets, "backup"), fiberws.Config{
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
	// Install streaming — admin-only WebSocket for Beacon's GET /servers/:id/install/ws
	// Proxied via both normalized /ws/install and legacy /install/ws aliases. Tickets
	// are issued via POST /servers/:id/ws/ticket?stream=install (handlers_ws_ticket).
	// Execution is gated by INSTALLER_WORKFLOW_ENABLED on the workflow service;
	// when disabled, workflows remain visible (DB→UI) but live streaming defers.
	// See forge/web/lib/api/install-ws.ts createInstallWSManager for frontend.
	v1.Get("/servers/:id/ws/install", requireRealtimeServices(cfg), readLimiter, fiberws.New(realtimeProxy(cfg, wsTickets, "install"), fiberws.Config{
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
	v1.Get("/servers/:id/install/ws", requireRealtimeServices(cfg), readLimiter, fiberws.New(realtimeProxy(cfg, wsTickets, "install"), fiberws.Config{
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

	// POST /api/edge/connect — Beacon edge-agent registration. Authenticates
	// like /nodes/:id/heartbeat: bearer node token plus a fresh one-time HMAC
	// (method, URI, timestamp, nonce, body keyed with the node token) so a
	// stolen token alone cannot open an edge channel and replays are rejected.
	app.Post("/api/edge/connect", apiIPAccess, mtlsMw, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		// Node credentials travel in the Authorization header only. A second
		// header (X-Node-Token) would create two credential paths with
		// different logging/redaction handling; Beacon sends Bearer on every
		// node-authenticated call, so the fallback is removed, not widened.
		token := strings.TrimSpace(strings.TrimPrefix(c.Get("Authorization"), "Bearer "))
		if token == "" || token == strings.TrimSpace(c.Get("Authorization")) {
			return fiber.NewError(fiber.StatusUnauthorized, "missing daemon bearer token")
		}
		var req struct {
			NodeID string `json:"nodeId"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.NodeID) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "nodeId is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := verifyNodeTokenWithHMAC(ctx, cfg, c, req.NodeID, token); err != nil {
			return err
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
					"total":         len(data),
					"count":         len(data),
					"per_page":      len(data),
					"current":       1,
					"total_records": len(data),
					"current_page":  1,
					"total_pages":   1,
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
			// Node-authenticated callers carry no user session:
			// remoteNodeMiddleware sets remoteNode, never user. The actor
			// stays nil (a Beacon report is not a user action); the node is
			// identifiable via the server's node_id, not the actor column.
			_, err = cfg.Store.UpsertBackup(ctx, c.Params("id"), store.UpsertBackupRequest{
				UUID:        body.UUID,
				Name:        body.Name,
				Checksum:    body.Checksum,
				Size:        body.Size,
				Status:      body.Status,
				CompletedAt: &completedAt,
			}, nil)
		} else {
			err = cfg.Store.MarkBackupStatus(ctx, c.Params("id"), body.Name, body.Status, nil)
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
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			ExitCode    int  `json:"exit_code"`
			OOMKilled   bool `json:"oom_killed"`
			AutoRestart bool `json:"auto_restart"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		belongs, err := cfg.Store.ServerBelongsToNode(ctx, c.Params("id"), node.ID)
		if err != nil || !belongs {
			return fiber.NewError(fiber.StatusForbidden, "requesting node cannot access this server")
		}
		crashID := uuid.NewString()
		crashCtx := cfg.BackgroundContext
		if crashCtx == nil {
			crashCtx = context.Background()
		}
		_, err = cfg.Store.Exec(crashCtx, `INSERT INTO server_crash_events
			(id, server_id, node_id, exit_code, oom_killed, auto_restarted, created_at)
			SELECT $1, s.id, s.node_id, $3, $4, $5, NOW() FROM servers s WHERE s.id = $2`,
			crashID, c.Params("id"), req.ExitCode, req.OOMKilled, req.AutoRestart)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true, "id": crashID})
	})

	// remote.Post("/servers/:id/health") is the control-plane ingest seam for
	// Beacon's post-deploy health probes: it records a HealthObservation through
	// the resource-limits service so HealthStatus/ShouldRollback can gate a
	// release. Beacon runs the probe; the observation store lives here, so this
	// handler is a thin, node-authenticated forward into resourcelimits.Service.
	remote.Post("/servers/:id/health", func(c *fiber.Ctx) error {
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		pool := cfg.Store.GetDB()
		if pool == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "resource limits store is not configured")
		}
		var body struct {
			Healthy      bool   `json:"healthy"`
			Detail       string `json:"detail"`
			ProcessType  string `json:"processType"`
			ProcessType2 string `json:"process_type"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		// Accept both camelCase (the panel-facing twin) and snake_case (the
		// daemon's convention) for the process type; it is required because a
		// probe that names no process type cannot be attributed to a release.
		processType := strings.TrimSpace(body.ProcessType)
		if processType == "" {
			processType = strings.TrimSpace(body.ProcessType2)
		}
		if processType == "" {
			return fiber.NewError(fiber.StatusBadRequest, "processType is required")
		}
		serverID := strings.TrimSpace(c.Params("id"))
		if serverID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "server id is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		belongs, err := cfg.Store.ServerBelongsToNode(ctx, serverID, node.ID)
		if err != nil || !belongs {
			return fiber.NewError(fiber.StatusForbidden, "requesting node cannot access this server")
		}
		svc := resourcelimits.NewFromPool(pool)
		if err := svc.ReportHealth(ctx, serverID, processType, body.Healthy, body.Detail); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// Additional /api/remote/* routes for daemon parity
	// (cluster-wide activity, backup lifecycle, archive completion, transfer state).
	registerRemoteExtras(remote, cfg)

	// Node-reported container lifecycle feed. Mounted here, on the guarded
	// /api/remote group, so it can never carry a weaker middleware chain than the
	// rest of the node-facing API.
	registerDockerEventIngest(remote, cfg)

	// Setup wizard routes (public, gated to "no admin exists" server-side)
	registerSetupRoutes(v1, cfg, authLimiter)

	// OAuth2 token endpoint (PufferPanel parity). Mounted on the public
	// `/api/v1/oauth2/token` and `/oauth2/token` paths so external
	// integrations can reach it without an admin JWT.
	v1.Post("/oauth2/token", authLimiter, IssueOAuth2Token(cfg))
	v1.Post("/oauth/token", authLimiter, IssueOAuth2Token(cfg)) // alias

	// Social authentication (Discord, Steam, Authentik). The public OAuth
	// redirect/callback routes mount on v1; the account/admin half must ride the
	// canonical protected router, so the full registration is deferred until
	// just after `protected` is defined below.

	// Git webhook endpoints (public, verified by HMAC signatures)
	registerGitWebhookRoutes(v1, cfg)

	// Git-push deploy hook (public by design: it is called by a repository's
	// post-receive hook on a node, which has no session; the HMAC of the raw
	// body against the app's shared secret is the only credential).
	registerGitPushWebhookRoutes(v1, cfg)

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
	protected := v1.Group("", authMiddleware(cfg.AuthSecret, cfg.Store), sessMw, requireTwoFactorAuthentication(cfg), csrfMiddleware(cfg.AuthSecret, LoadSessionCookieConfig()), methodLimiter)

	// Log every successful mutating request into the activity feed. Runs after
	// auth so the actor is known; the read endpoints (/admin/activity) depend on
	// this, otherwise the feed is permanently empty.
	protected.Use(func(c *fiber.Ctx) error {
		herr := c.Next()
		if cfg.ActivityService == nil || c.Method() == fiber.MethodGet || herr != nil {
			return herr
		}
		if c.Response().StatusCode() >= 400 {
			return herr
		}
		var actorID, actorEmail string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID, actorEmail = claims.Sub, claims.Email
		}
		routePath := ""
		if r := c.Route(); r != nil {
			routePath = r.Path
		}
		_ = cfg.ActivityService.NewEvent("http:"+c.Method()+" "+routePath).
			Actor(actorID, actorEmail, "user").
			IP(ExtractClientIP(c)).
			Description(c.Method()+" "+c.OriginalURL()).
			Save(c.Context(), cfg.ActivityService)
		return herr
	})

	// Social authentication: the canonical protected router now exists, so mount
	// the public (v1) and protected/admin social routes on it.
	registerSocialAuthRoutes(v1, protected, cfg, mutationLimiter, authLimiter)

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

		// Issue a download ticket for the file, bound to the issuing user and IP
		// so a leaked URL cannot be replayed from another account or network.
		var issuerID, issuerIP string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			issuerID = claims.Sub
		}
		issuerIP = ExtractClientIP(c)
		ticket, err := fileDownloadTickets.issue(fileDownloadTicket{
			serverID: serverID,
			filePath: filePath,
			expires:  time.Now().Add(5 * time.Minute),
			userID:   issuerID,
			ip:       issuerIP,
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

	v1.Get("/download/file", authLimiter, downloadFileWithTicket(cfg, fileDownloadTickets))

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

	protected.Get("/users/:id", requireRole("admin"), requireAdminScope("users.read"), func(c *fiber.Ctx) error {
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

	// GET /mounts/:id is registered by registerAdminRoutes with
	// requireAdminScope("mounts.read"). It used to be duplicated here with only
	// requireRole("admin"); because Fiber resolves overlapping paths in
	// registration order and this block runs first, the copy below shadowed the
	// scoped one and let any admin API key read a mount regardless of its
	// scopes. The duplicate is gone so the scoped registration is the live one.

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
	registerAccountRecoveryRoutes(v1, protected, cfg, authLimiter)
	// Register fixed plugin subroutes before admin's /admin/plugins/:id route.
	// Otherwise paths such as /discover are parsed as a plugin identifier.
	registerPluginRoutes(protected, cfg)
	// /api-keys is owned by registerAuthRoutes above: keys belong to the
	// authenticated user, not to admins. A second /api-keys group used to be
	// registered here; it was unreachable (registerAuthRoutes runs first) and
	// read the caller from a "userId" local that authMiddleware never sets.
	registerNestRoutes(protected, cfg, mutationLimiter, adminIPAccess)
	registerLocationRoutes(protected, cfg, mutationLimiter, adminIPAccess)
	registerRegionRoutes(protected, cfg, mutationLimiter, adminIPAccess)
	registerTemplateRoutes(protected, cfg, mutationLimiter, adminIPAccess)
	registerAdminRoutes(protected, cfg, nodeRegistry, clusterManager, evacuationPlanner, migrationService, reservationManager, recoveryCoordinator, mutationLimiter, adminIPAccess)
	registerServerRoutes(protected, cfg, runner, clusterManager, mutationLimiter, adminIPAccess)
	registerSettingsRoutes(protected, cfg, mutationLimiter, adminIPAccess)
	registerSettingsExtras(protected, cfg, mutationLimiter, adminIPAccess)
	registerRateLimitSettingsRoutes(protected, cfg, mutationLimiter, adminIPAccess)
	registerActivityRoutes(protected, cfg)
	registerAuditLogRoutes(protected, cfg)
	registerOrphanRemediationRoutes(protected, cfg, mutationLimiter, adminIPAccess)
	registerAdminExtras(protected, cfg, nodeProbe)
	// The following three registrars were defined but never invoked, so the
	// admin Operations-timeline, Kubernetes and Installer pages (advertised as
	// "available" in the admin registry and backed by lib/api modules) 404'd.
	registerOperationsTimelineRoutes(protected, cfg)
	registerKubernetesRoutes(protected, cfg, adminIPAccess)
	registerIncusRoutes(protected, cfg, adminIPAccess)
	registerNomadRoutes(protected, cfg, adminIPAccess)
	registerInstallerRoutes(protected, cfg)
	registerObservabilityRoutes(protected, cfg, cfg.Observability, cfg.HeartbeatMonitor)
	registerAlertRoutes(protected, cfg.AlertService, cfg.Observability, mutationLimiter)
	registerNotificationRoutes(protected, cfg.NotificationService, mutationLimiter)
	// User-facing notifications engine routes. Registered before the enhanced
	// admin-only registrar so /notifications/channels is the user-scoped list.
	registerNotificationCrudRoutes(protected, cfg.NotificationRouter, mutationLimiter)
	if cfg.EnhancedNotificationService != nil {
		registerEnhancedNotificationRoutes(protected, cfg.EnhancedNotificationService, mutationLimiter)
	}
	registerMailSettingsRoutes(protected, cfg, mutationLimiter, adminIPAccess)
	registerSFTPRoutes(protected, cfg, mutationLimiter)
	registerWebAuthnRoutes(v1, protected, cfg, authLimiter, mutationLimiter, cfg.WebAuthnService)
	registerAutoScalerRoutes(protected, cfg, cfg.AutoScaler, adminIPAccess, mutationLimiter)
	registerNodeAutoscaleRoutes(protected, cfg, cfg.NodeAutoscaler, adminIPAccess, mutationLimiter)
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
	registerBackupEngineRoutes(protected, cfg, cfg.BackupEngineService, mutationLimiter)
	registerDNSRoutes(protected, cfg, cfg.DNSService, mutationLimiter)
	registerMaintenanceRoutes(protected, cfg, mutationLimiter)
	registerComposeRoutes(protected, cfg, mutationLimiter)
	registerComposeTemplateRoutes(protected, cfg, mutationLimiter)
	registerDBContainerRoutes(protected, cfg, mutationLimiter)
	registerDatabaseServiceRoutes(protected, cfg, mutationLimiter)
	registerDBDiagnosticRoutes(protected, cfg)
	registerManagedDatabaseRoutes(protected, cfg, mutationLimiter)
	registerGitRoutes(protected, cfg, adminIPAccess, mutationLimiter)
	registerGitPushRoutes(protected, cfg, mutationLimiter, adminIPAccess)
	RegisterGitDeploymentRoutes(protected, cfg, mutationLimiter)
	registerBuildRoutes(protected, cfg, cfg.BuildService, mutationLimiter)
	registerBuildpackRoutes(protected, cfg, cfg.BuildpackService, mutationLimiter)
	registerZeroDowntimeRoutes(protected, cfg, cfg.ZeroDowntimeSvc, mutationLimiter)
	registerSourceDeploymentRoutes(protected, cfg, mutationLimiter)
	registerRegistryRoutes(protected, cfg, adminIPAccess, mutationLimiter)
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
	registerPipelineRoutes(protected, cfg.PipelineService, mutationLimiter)

	// Portainer-inspired container/image/network/volume administration
	registerPortainerRoutes(protected, cfg, mutationLimiter, adminIPAccess)
	registerAppStoreRoutes(protected, cfg, cfg.AppStoreService, mutationLimiter)
	registerCatalogRoutes(protected, cfg, cfg.CatalogService, adminIPAccess, mutationLimiter)
	registerForgefileRoutes(protected, cfg, cfg.ForgefileSvc, mutationLimiter)

	// Docker management routes (cleaner replacement for Portainer admin routes)
	registerDockerRoutes(protected, cfg, mutationLimiter, adminIPAccess)

	// Host-level file management and terminal
	registerHostFileRoutes(protected, cfg, mutationLimiter)
	registerHostTerminalRoute(protected, cfg)

	// Cron job management
	if cfg.CronJobService != nil {
		registerCronJobRoutes(protected, cfg, cfg.CronJobService, mutationLimiter)
	}

	// Per-app scheduled tasks (cron-expression commands dispatched to Beacon)
	if cfg.ScheduledTaskService != nil {
		registerScheduledTaskRoutes(protected, cfg, cfg.ScheduledTaskService, mutationLimiter)
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

	// Billing / placement (previously-orphan services now wired). Drain is not
	// registered: /nodes/:id/drain is already served by clustermembership.
	registerBillingRoutes(protected, cfg, mutationLimiter)
	registerBillingWebhookRoute(v1, cfg, mutationLimiter)
	registerPlacementRoutes(protected, cfg, cfg.PlacementService, mutationLimiter)
	registerDrainRoutes(protected, cfg, cfg.DrainLedger)
	registerHealthCheckRoutes(protected, cfg, cfg.HealthCheckRunner)
	registerFencingRoutes(protected, cfg, cfg.FencingSvc, mutationLimiter)
	registerUpgradeRoutes(protected, cfg, cfg.UpgradeSvc, mutationLimiter)

	// Cross-node routing and service discovery routes
	registerServiceDiscoveryRoutes(protected, cfg, cfg.ServiceDiscovery, adminIPAccess, mutationLimiter)
	registerCrossNodeRoutes(protected, cfg, cfg.CrossNodeResolver, cfg.IngressSynchronizer, adminIPAccess, mutationLimiter)
	registerNetBirdRoutes(protected, cfg, adminIPAccess, mutationLimiter)

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
