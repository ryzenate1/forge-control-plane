package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	nethttp "net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	forgecfg "gamepanel/forge/config"
	"gamepanel/forge/internal/auth"
	"gamepanel/forge/internal/cloud"
	"gamepanel/forge/internal/config"
	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/eventstore"
	"gamepanel/forge/internal/http"
	"gamepanel/forge/internal/placement"
	gpruntime "gamepanel/forge/internal/runtime"
	"gamepanel/forge/internal/secrets"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"gamepanel/forge/internal/services"
	acmesvc "gamepanel/forge/internal/services/acme"
	"gamepanel/forge/internal/services/activity"
	alerting "gamepanel/forge/internal/services/alerting"
	apphostingsvc "gamepanel/forge/internal/services/apphosting"
	appstoresvc "gamepanel/forge/internal/services/appstore"
	auditlogsvc "gamepanel/forge/internal/services/auditlog"
	"gamepanel/forge/internal/services/autoscaler"
	"gamepanel/forge/internal/services/backup"
	backupenginesvc "gamepanel/forge/internal/services/backupengine"
	billingsvc "gamepanel/forge/internal/services/billing"
	buildsvc "gamepanel/forge/internal/services/build"
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
	drainsvc "gamepanel/forge/internal/services/drain"
	envaffinitysvc "gamepanel/forge/internal/services/envaffinity"
	"gamepanel/forge/internal/services/environments"
	envvarsvc "gamepanel/forge/internal/services/envvars"
	"gamepanel/forge/internal/services/evacuationplanner"
	"gamepanel/forge/internal/services/failover"
	fencing "gamepanel/forge/internal/services/fencing"
	"gamepanel/forge/internal/services/forgefile"
	gitsvc "gamepanel/forge/internal/services/git"
	gitprovidersvc "gamepanel/forge/internal/services/gitprovider"
	"gamepanel/forge/internal/services/health"
	healthchecksvc "gamepanel/forge/internal/services/healthcheckrunner"
	"gamepanel/forge/internal/services/heartbeatmonitor"
	"gamepanel/forge/internal/services/i18n"
	incussvc "gamepanel/forge/internal/services/incus"
	installersvc "gamepanel/forge/internal/services/installer"
	"gamepanel/forge/internal/services/loadbalancer"
	"gamepanel/forge/internal/services/logger"
	mailservice "gamepanel/forge/internal/services/mail"
	"gamepanel/forge/internal/services/migration"
	netbirdsvc "gamepanel/forge/internal/services/netbird"
	"gamepanel/forge/internal/services/nodeautoscale"
	"gamepanel/forge/internal/services/nodeprobe"
	"gamepanel/forge/internal/services/noderegistry"
	nomadsvc "gamepanel/forge/internal/services/nomad"
	notification "gamepanel/forge/internal/services/notification"
	notifs "gamepanel/forge/internal/services/notifications"
	"gamepanel/forge/internal/services/observability"
	onboardingsvc "gamepanel/forge/internal/services/onboarding"
	operationsvc "gamepanel/forge/internal/services/operation"
	pipelinesvc "gamepanel/forge/internal/services/pipeline"
	"gamepanel/forge/internal/services/plugins"
	previewenv "gamepanel/forge/internal/services/previewenv"
	proceduresvc "gamepanel/forge/internal/services/procedure"
	processsvc "gamepanel/forge/internal/services/process"
	"gamepanel/forge/internal/services/queue"
	"gamepanel/forge/internal/services/reconciler"
	recoverysvc "gamepanel/forge/internal/services/recovery"
	"gamepanel/forge/internal/services/replicamanager"
	"gamepanel/forge/internal/services/reservations"
	runtimesvc "gamepanel/forge/internal/services/runtime"
	scheduledtaskssvc "gamepanel/forge/internal/services/scheduledtasks"
	"gamepanel/forge/internal/services/scheduler"
	"gamepanel/forge/internal/services/servicediscovery"
	"gamepanel/forge/internal/services/tenancy"
	"gamepanel/forge/internal/services/trafficmanager"
	upgradesvc "gamepanel/forge/internal/services/upgrade"
	"gamepanel/forge/internal/services/vaultprovider"
	"gamepanel/forge/internal/services/webauthn"
	"gamepanel/forge/internal/services/webhook"
	"gamepanel/forge/internal/services/zerodowntime"
	"gamepanel/forge/internal/store"
	"gamepanel/forge/internal/version"

	"github.com/redis/go-redis/v9"
)

const readinessHealthPath = "/api/v1/health/ready"

func main() {
	if err := run(); err != nil {
		log.Printf("forge API startup failed: %v", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 && os.Args[1] == "--healthcheck" {
		return healthcheck("http://127.0.0.1" + healthcheckPort(env("API_ADDR", ":8080")) + readinessHealthPath)
	}
	// Database migrations and operational-secret rotations can legitimately take
	// longer than 30 seconds on a large installation. Bound startup, but do not
	// cancel it halfway through a normal migration.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	appEnv := env("APP_ENV", "development")
	production := strings.EqualFold(strings.TrimSpace(appEnv), "production")
	panelURL := strings.TrimSpace(env("PANEL_URL", "http://localhost:3000"))
	if production {
		parsedPanelURL, parseErr := url.Parse(panelURL)
		if parseErr != nil || parsedPanelURL.Scheme != "https" || parsedPanelURL.Hostname() == "" || parsedPanelURL.User != nil {
			return errors.New("PANEL_URL must be an absolute HTTPS URL without credentials in production")
		}
	}
	seedDemo, err := demoSeedEnabled(appEnv, os.Getenv("API_SEED_DEMO"))
	if err != nil {
		return err
	}
	authSecret := strings.TrimSpace(os.Getenv("API_AUTH_SECRET"))
	if authSecret == "" {
		if production {
			return errors.New("API_AUTH_SECRET must be set to a production secret with at least 32 characters")
		}
		authSecret, err = randomEncodedSecret(32)
		if err != nil {
			return fmt.Errorf("generate development API auth secret: %w", err)
		}
	}
	if production && len(authSecret) < 32 {
		return errors.New("API_AUTH_SECRET must be at least 32 characters")
	}
	if production && strings.Contains(authSecret, "CHANGE_ME") {
		return errors.New("API_AUTH_SECRET must not contain a deployment placeholder")
	}
	if production && strings.Contains(os.Getenv("APP_KEY"), "CHANGE_ME") {
		return errors.New("APP_KEY must not contain a deployment placeholder")
	}

	slogLogger := logger.New(logger.Config{
		Level:  env("LOG_LEVEL", "info"),
		Format: env("LOG_FORMAT", "text"),
		Output: env("LOG_OUTPUT", "stdout"),
	})

	var db *store.Store
	var masterKeyring *secrets.Keyring
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		kr, ephemeral, err := masterKeyringFromEnvironment(production)
		if err != nil {
			return err
		}
		masterKeyring = kr
		keyring := kr
		if ephemeral {
			slogLogger.Warn("FORGE_ALLOW_EPHEMERAL_MASTER_KEY is enabled; encrypted data will be unrecoverable after this process exits")
		}
		connected, err := store.ConnectWithKeyring(ctx, databaseURL, keyring)
		if err != nil {
			return err
		}
		defer connected.Close()
		if err := connected.RunMigrations(ctx, env("MIGRATIONS_DIR", "migrations")); err != nil {
			return err
		}
		// Migration filenames are immutable primary keys in schema_migrations,
		// so an edit to an already-applied migration is never re-run and the
		// deployed schema can diverge from this build. Drift is reported
		// rather than fatal: refusing to boot would not repair it. It must
		// not pass silently either — rows applied before the checksum column
		// existed are unverifiable, which is not the same as clean.
		//
		// This check previously existed only in internal/app/container.go,
		// which nothing imported, so drift went unreported in production
		// despite docs/migration-rollback-policy.md documenting it.
		if integrity := connected.MigrationIntegrity(); len(integrity.Drift) > 0 || integrity.Unverified > 0 {
			for _, d := range integrity.Drift {
				slogLogger.Warn("migration file changed after it was applied; deployed schema may diverge from this build",
					slog.String("migration", d.Version),
					slog.String("applied_checksum", d.Applied),
					slog.String("on_disk_checksum", d.OnDisk))
			}
			slogLogger.Warn("migration integrity check found unverified or drifted migrations",
				slog.Int("drifted", len(integrity.Drift)),
				slog.Int("unverifiable", integrity.Unverified))
		}
		if err := eventstore.Migrate(connected.GetDB()); err != nil {
			return err
		}
		if err := connected.MigrateOperationalSecrets(ctx); err != nil {
			return err
		}
		if len(os.Args) > 1 && os.Args[1] == "rotate-master-key" {
			slogLogger.Info("secret rotation completed", slog.String("active_key", keyring.ActiveKeyID()))
			return nil
		}
		if len(os.Args) > 1 && os.Args[1] == "restore-plaintext-secrets" {
			if err := connected.RestoreOperationalSecrets(ctx); err != nil {
				return err
			}
			slogLogger.Info("legacy plaintext secret columns restored; ciphertext retained")
			return nil
		}
		if seedDemo {
			if err := connected.Seed(ctx); err != nil {
				return err
			}
		}
		db = connected
	}

	var redisClient *redis.Client
	redisEnabled := false
	if redisAddr := os.Getenv("REDIS_ADDR"); redisAddr != "" {
		redisEnabled = true
		redisPassword := strings.TrimSpace(os.Getenv("REDIS_PASSWORD"))
		redisTLS := envBool("REDIS_TLS", production)
		if production && redisPassword == "" {
			return errors.New("REDIS_PASSWORD is required when Redis is enabled in production")
		}
		if production && !redisTLS {
			return errors.New("REDIS_TLS must be enabled when Redis is used in production")
		}
		redisOptions := &redis.Options{Addr: redisAddr, Password: redisPassword}
		if redisTLS {
			redisOptions.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
		}
		redisClient = redis.NewClient(redisOptions)
		defer redisClient.Close()
		if err := redisClient.Ping(ctx).Err(); err != nil {
			slogLogger.Warn("redis ping failed at startup", slog.String("error", err.Error()))
		}
	}

	baseURL := env("BEACON_BASE_URL", "http://127.0.0.1:9090")
	nodeToken := strings.TrimSpace(os.Getenv("DAEMON_NODE_TOKEN"))
	daemonClient, err := daemon.NewClient(baseURL, nodeToken)
	if err != nil {
		if !production && nodeToken == "" {
			// Dev mode without a token: create a client with loopback URL
			// and a placeholder. Outbound API calls always pass the actual
			// target node credential at call time.
			daemonClient, err = daemon.NewClient("http://127.0.0.1:9090", "dev-placeholder")
			if err != nil {
				return fmt.Errorf("create dev daemon client: %w", err)
			}
		} else {
			return fmt.Errorf("create daemon client: %w", err)
		}
	}

	// Build the service graph. All services are nil-safe when db == nil;
	// handler nil-guards already handle the "no database" dev-mode case.
	var (
		nr                 *noderegistry.Service
		np                 *nodeprobe.Service
		cm                 *clustermanager.Service
		workloadRuntime    *gpruntime.MultiRuntimeAdapter
		ep                 *evacuationplanner.Service
		mig                *migration.Service
		resMgr             *reservations.Manager
		rcv                *recoverysvc.Coordinator
		rts                *recoverysvc.TokenService
		hbm                *heartbeatmonitor.Service
		obs                *observability.Service
		rec                *reconciler.Service
		dbProv             *dbprovisioner.Service
		whSvc              *webhook.Service
		mailWorker         *mailservice.Worker
		mailTriggerSvc     *mailservice.TriggerService
		actSvc             *activity.Service
		auditLogSvc        auditlogsvc.AuditLogger
		pluginSvc          *plugins.Service
		queueSvc           *queue.Service
		opSvc              *operationsvc.Service
		runtimeRegistry    *runtimesvc.Registry
		waSvc              *webauthn.Service
		autoSvc            *autoscaler.Service
		bkSvc              *backup.Service
		bkWorker           *backup.Worker
		dnsSvc             *dnssvc.Service
		vaultSvc           *vaultprovider.Service
		acmeSvc            *acmesvc.Service
		domainSvc          *domains.Service
		buildSvc           *buildsvc.Service
		deploySvc          *deployment.Service
		previewDeploySvc   *previewenv.Service
		cloudMgr           *cloud.Manager
		lbSvc              *loadbalancer.Service
		failSvc            *failover.Service
		crashDetector      *crashdetector.Detector
		tmSvc              *trafficmanager.Service
		caddyTLS           *trafficmanager.CaddyTLSManager
		predictiveScorer   *scheduler.PredictiveScorer
		constraintSched    *scheduler.ConstraintScheduler
		healthCheckRunner  *healthchecksvc.Service
		tenancySvc         *tenancy.Service
		dbContainerSvc     *dbprovisioner.DBContainerService
		composeLifecycle   *composesvc.Service
		composeTemplateSvc *composetemplatessvc.Service
		procedureSvc       *proceduresvc.Service
		apphostingSvc      *apphostingsvc.Service
		endpointSvc        *environments.Service
		pipelineSvc        *pipelinesvc.Service
		alertSvc           *alerting.Service
		notifSvc           *notification.Service
		notifRouter        *notifs.Router
		installerSvc       *installersvc.Service
		billingSvc         *billingsvc.Service
		drainLedger        *drainsvc.Service
		placementSvc       *envaffinitysvc.EnvAffinity
		fenceSvc           *fencing.Service
		upgradeSvc         *upgradesvc.Service
		membershipSvc      *clustermembership.Service
		nodeAutoSvc        *nodeautoscale.Service
		cleanupSvc         *cleanupsvc.Service
		gitSvc             *gitsvc.Service
		gitDeploySvc       *gitsvc.DeployService
		gitProviderSvc     *gitprovidersvc.Service
		gitOpsController   *composesvc.GitOpsController
		sessionStore       *auth.PostgresSessionStore
		replicaMgr         *replicamanager.Manager
		discoverySvc       *servicediscovery.Service
		crossNodeResolver  *crossnode.Resolver
		ingressSync        *crossnode.IngressSynchronizer
		netbirdSvc         *netbirdsvc.Service
		healthFilter       *crossnode.HealthFilter
		appStoreSvc        *appstoresvc.Service
		catalogSvc         *catalogsvc.Service
		forgefileSvc       *forgefile.Service
		onboardingSvc      *onboardingsvc.Service
		cronJobSvc         *cronjobsvc.Service
		scheduledTaskSvc   *scheduledtaskssvc.Service
		gitDeployMgmtSvc   *gitsvc.DeploymentManagementService
		zdSvc              *zerodowntime.Service
		dbSvcProv          *services.DatabaseServiceProvisioner
		dbBackupSvc        *dbbackupsvc.Service
		backupEngineSvc    *backupenginesvc.Service
		buildpackSvc       *buildpacksvc.Service
		processSvc         *processsvc.Service
		certSvc            *services.CertService
		mtlsMigrator       *services.MTLSMigrator
		mtlsCfg            forgecfg.MTLS
		eventRegistry      *events.Registry
	)

	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	started := time.Now()

	// Provider credentials are intentionally resolved by the AWS SDK's standard
	// chain (environment, profile, or workload identity) and are never stored by
	// the panel. Do not advertise a provider unless its target region is explicit.
	cloudMgr = cloud.NewManager()
	if strings.TrimSpace(env("AWS_REGION", env("AWS_DEFAULT_REGION", ""))) != "" {
		awsProvider, err := cloud.NewAWSProvider(appCtx)
		if err != nil {
			slogLogger.Warn("AWS cloud provider is not configured", slog.String("error", err.Error()))
		} else {
			cloudMgr.RegisterProvider(awsProvider)
		}
	}

	var eventRelay *eventstore.Relay
	var placeEngine *placement.Engine

	if db != nil {
		eventRegistry = events.NewRegistry("forge-api")

		es := eventstore.New(db.GetDB())
		eventRelay = eventstore.NewRelay(es, 5*time.Second)
		outboxPub := eventstore.NewOutboxPublisher(es, eventRegistry)

		placeEngine = placement.NewEngine(placement.NewScorer(placement.StrategyLeastLoaded), placement.NewConstraintChecker())

		predictiveScorer = scheduler.NewPredictiveScorer(predictiveStore{db})
		predictiveScorer.LoadRules(appCtx)
		constraintSched = scheduler.NewConstraintScheduler(db)

		resMgr = reservations.New(db, outboxPub)
		sched := scheduler.New(db, placeEngine, outboxPub).
			WithPredictiveScorer(predictiveScorer).
			WithConstraintScheduler(constraintSched).
			WithReservations(resMgr)

		// Build the multi-runtime adapter: one dispatcher that routes operations
		// to the correct engine based on Target.Provider. Docker is always available;
		// other adapters are registered unconditionally — Beacon's own provider check
		// (409 Conflict) is the enforcement point at runtime.
		dockerRT := gpruntime.NewDockerAdapter(daemonClient)
		multiRT := gpruntime.NewMultiRuntimeAdapter(dockerRT)
		multiRT.Register(gpruntime.DockerProvider, dockerRT)
		multiRT.Register(gpruntime.ContainerdProvider, gpruntime.NewContainerdAdapter(daemonClient))
		multiRT.Register(gpruntime.PodmanProvider, gpruntime.NewPodmanAdapter(daemonClient))
		multiRT.Register(gpruntime.FirecrackerProvider, gpruntime.NewFirecrackerAdapter(daemonClient))
		multiRT.Register(gpruntime.KubernetesProvider, gpruntime.NewKubernetesAdapter(daemonClient))
		multiRT.Register(gpruntime.KVMProvider, gpruntime.NewKVMAdapter(daemonClient))
		multiRT.Register(gpruntime.LXCProvider, gpruntime.NewLXCAdapter(daemonClient))

		cm = clustermanager.New(db, multiRT, sched, resMgr, outboxPub)
		// The HTTP layer reports workload kinds from the same dispatcher that
		// executes them, so the create menu cannot drift ahead of the wiring.
		workloadRuntime = multiRT

		// Dev/demo: the seeded demo server is inserted directly into the
		// database, bypassing the normal create-provision flow, so its
		// workload would never exist at the beacon. Provision it here so the
		// demo server can actually start (idempotent: beacon Create treats an
		// existing workload as a no-op). The beacon may still be starting, so
		// retry briefly.
		if seedDemo {
			const demoServerID = "44444444-4444-4444-8444-444444444444"
			var provisionErr error
			for attempt := 1; attempt <= 15; attempt++ {
				if provisionErr = cm.ProvisionRecoveredServer(appCtx, demoServerID); provisionErr == nil {
					slogLogger.Info("demo seed: workload provisioned", slog.String("server_id", demoServerID))
					break
				}
				select {
				case <-appCtx.Done():
					provisionErr = appCtx.Err()
				case <-time.After(2 * time.Second):
				}
			}
			if provisionErr != nil {
				slogLogger.Warn("demo seed: workload provision skipped",
					slog.String("server_id", demoServerID),
					slog.String("error", provisionErr.Error()))
			}
		}

		// Initialize Beacon HTTP client for replicamanager
		beaconHTTPClient := replicamanager.NewBeaconHTTPClient(db, daemonClient, slogLogger)

		// Initialize replicamanager with all required dependencies. Passing a
		// nil dispatcher here used to fall back to a no-op that reported every
		// replica command as "pending" without contacting any beacon.
		instanceDispatcher := replicamanager.NewRemoteCommandDispatcher(db, daemonClient, slogLogger)
		replicaMgr = replicamanager.New(db, placeEngine, sched, resMgr, instanceDispatcher, beaconHTTPClient, slogLogger, outboxPub)
		hbm = heartbeatmonitor.New(db, outboxPub)
		rec = reconciler.New(db, cm, 0, outboxPub)
		ep = evacuationplanner.New(db, sched, outboxPub)
		mig = migration.New(db, sched, ep, resMgr, dockerRT, outboxPub)
		ep.SetMigrationExecutor(mig)
		ep.SetServerMountStore(db)
		fenceSvc = fencing.New(db, outboxPub)
		eventRegistry.Subscribe(events.EventNodeRecovered, fenceSvc)
		upgradeSvc = upgradesvc.New(upgradesvc.AdaptStore(db), slogLogger,
			env("FORGE_INSTALL_DIR", "."),
			env("FORGE_BACKUP_DIR", env("DATA_DIR", ".")+"/backups"),
			env("FORGE_VERSION_FILE", "VERSION"),
		)
		rcv = recoverysvc.NewWithMigrationExecutor(db, sched, resMgr, mig, outboxPub)
		recTokenStore := recoverysvc.NewStore(db.GetDB())
		rts = recoverysvc.NewTokenService(recTokenStore)
		obs = observability.New(db)
		obs.StartMetricsCollection(appCtx, 30*time.Second)
		obs.StartNodeMetricsCollection(appCtx, 60*time.Second)
		nr = noderegistry.New(db)
		np = nodeprobe.NewService(db, daemonClient)
		dbProv = dbprovisioner.NewService(db)
		whSvc = webhook.NewService(db)
		mailWorker = mailservice.NewWorker(db)

		mailRenderer := mailservice.NewTemplateRenderer()
		panelURL := env("PANEL_URL", "http://localhost:3000")
		mailTriggerSvc = mailservice.NewTriggerService(mailRenderer, mailWorker, panelURL, "GamePanel", "GamePanel")

		pluginStore := plugins.NewStore(db.GetDB())
		pluginSvc = plugins.New(pluginStore, env("PLUGINS_DIR", ""))

		actStore := activity.NewStore(db.GetDB())
		actSvc = activity.New(actStore)

		auditLogSvc = auditlogsvc.NewDBAuditLogger(db)

		qStore := queue.NewPostgresStore(db.GetDB())
		queueSvc = queue.New(qStore, 5)
		registerPowerJob := func(jobType queue.JobType, signal string) {
			queueSvc.RegisterHandler(jobType, func(ctx context.Context, job *queue.Job) error {
				commandCtx := daemon.ContextWithCommandID(ctx, job.ID)
				_, _, err := cm.RequestServerPower(commandCtx, job.ServerID, signal)
				if err == nil {
					event := map[string]string{"start": "server:started", "stop": "server:stopped", "restart": "server:restarted", "kill": "server:stopped"}[signal]
					if event != "" {
						if err := db.DispatchWebhookEvent(ctx, event, map[string]any{"subject_type": "server", "subject_id": job.ServerID, "signal": signal, "operation_id": job.ID}); err != nil {
							slogLogger.Error("webhook dispatch failed", slog.String("event", event), slog.String("error", err.Error()))
						}
					}
				}
				return err
			})
		}
		registerPowerJob(queue.JobServerStart, "start")
		registerPowerJob(queue.JobServerStop, "stop")
		registerPowerJob(queue.JobServerRestart, "restart")
		registerPowerJob(queue.JobServerKill, "kill")
		composeLifecycle, err = composesvc.New(db, daemonClient, outboxPub)
		if err != nil {
			return fmt.Errorf("create compose service: %w", err)
		}
		composeLifecycle.WithReservationManager(resMgr).WithScheduler(sched)
		composeTemplateSvc = composetemplatessvc.New(db, composeLifecycle)
		composeQH, err := composesvc.NewQueueHandler(composeLifecycle)
		if err != nil {
			return fmt.Errorf("create compose queue handler: %w", err)
		}
		queueSvc.RegisterHandler(queue.JobComposeDeploy, func(ctx context.Context, job *queue.Job) error {
			return composeQH.HandleDeploy(ctx, job.Payload)
		})
		queueSvc.RegisterHandler(queue.JobComposeUpdate, func(ctx context.Context, job *queue.Job) error {
			return composeQH.HandleUpdate(ctx, job.Payload)
		})
		queueSvc.RegisterHandler(queue.JobComposeDelete, func(ctx context.Context, job *queue.Job) error {
			return composeQH.HandleDelete(ctx, job.Payload)
		})
		queueSvc.RegisterHandler(queue.JobComposeStart, func(ctx context.Context, job *queue.Job) error {
			return composeQH.HandleStart(ctx, job.Payload)
		})
		queueSvc.RegisterHandler(queue.JobComposeStop, func(ctx context.Context, job *queue.Job) error {
			return composeQH.HandleStop(ctx, job.Payload)
		})
		queueSvc.RegisterHandler(queue.JobComposeRestart, func(ctx context.Context, job *queue.Job) error {
			return composeQH.HandleRestart(ctx, job.Payload)
		})

		// server.install, server.uninstall, backup.create, backup.restore and
		// server.transfer are defined job types that had no executor at all, so
		// dispatching one only ever produced a "no handler registered" failure.
		// Each executor below runs the same code path as the corresponding working
		// HTTP endpoint: the node base URL and daemon token are resolved for that
		// specific server through store.ServerControlTarget (never a first-node
		// guess), the real work is performed inline, and every failure is returned
		// so the job is recorded as failed instead of completing silently.
		jobServerID := func(primary string, fallback string) (string, error) {
			serverID := strings.TrimSpace(primary)
			if serverID == "" {
				serverID = strings.TrimSpace(fallback)
			}
			if serverID == "" {
				return "", errors.New("serverId is required")
			}
			return serverID, nil
		}
		resolveControlTarget := func(ctx context.Context, serverID string) (store.ServerControlTarget, error) {
			target, err := db.ServerControlTarget(ctx, serverID)
			if err != nil {
				return store.ServerControlTarget{}, fmt.Errorf("resolve the node running server %s: %w", serverID, err)
			}
			if strings.TrimSpace(target.NodeURL) == "" || strings.TrimSpace(target.NodeToken) == "" {
				return store.ServerControlTarget{}, fmt.Errorf("server %s is assigned to a node without a base url or daemon token", serverID)
			}
			return target, nil
		}
		jobActor := func(id string) *string {
			id = strings.TrimSpace(id)
			if id == "" {
				return nil
			}
			return &id
		}

		executeServerInstall := func(ctx context.Context, jobID string, serverID string, raw json.RawMessage) error {
			var payload serverInstallPayload
			if err := decodeJobPayload(raw, &payload); err != nil {
				return fmt.Errorf("server.install: %w", err)
			}
			id, err := jobServerID(serverID, payload.ServerID)
			if err != nil {
				return fmt.Errorf("server.install: %w", err)
			}
			if cm == nil {
				return errors.New("server.install: workload lifecycle service is unavailable")
			}
			// POST /servers/:id/install and /servers/:id/reinstall are the real
			// installer entry points; they resolve the server's own node and turn a
			// rejected or non-zero installer run into an error.
			installCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
			defer cancel()
			if payload.Reinstall {
				_, err = cm.ReinstallServer(installCtx, id)
			} else {
				_, err = cm.InstallServer(installCtx, id)
			}
			if err != nil {
				return fmt.Errorf("install server %s: %w", id, err)
			}
			if webhookErr := db.DispatchWebhookEvent(ctx, "server:installed", map[string]any{"subject_type": "server", "subject_id": id, "operation_id": jobID}); webhookErr != nil {
				slogLogger.Error("webhook dispatch failed", slog.String("event", "server:installed"), slog.String("error", webhookErr.Error()))
			}
			return nil
		}

		executeServerUninstall := func(ctx context.Context, jobID string, serverID string, raw json.RawMessage) error {
			var payload serverUninstallPayload
			if err := decodeJobPayload(raw, &payload); err != nil {
				return fmt.Errorf("server.uninstall: %w", err)
			}
			id, err := jobServerID(serverID, payload.ServerID)
			if err != nil {
				return fmt.Errorf("server.uninstall: %w", err)
			}
			if cm == nil {
				return errors.New("server.uninstall: workload lifecycle service is unavailable")
			}
			// There is no HTTP "uninstall" endpoint; the destructive cleanup path the
			// panel actually uses is DELETE /servers/:id, which stops and removes the
			// workload at its node and then hard-deletes the record (recording an
			// orphan first when force is requested and the node refused the delete).
			uninstallCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()
			response, err := cm.DeleteServer(uninstallCtx, id, payload.Force)
			if err != nil && payload.Force && response.Accepted && response.Mode == "force" {
				slogLogger.Warn("server uninstall completed with a recorded orphan workload",
					slog.String("job_id", jobID),
					slog.String("server_id", id),
					slog.String("error", err.Error()))
				return nil
			}
			if err != nil {
				return fmt.Errorf("uninstall server %s: %w", id, err)
			}
			return nil
		}

		executeBackupCreate := func(ctx context.Context, jobID string, serverID string, raw json.RawMessage) error {
			var payload backupCreatePayload
			if err := decodeJobPayload(raw, &payload); err != nil {
				return fmt.Errorf("backup.create: %w", err)
			}
			id, err := jobServerID(serverID, payload.ServerID)
			if err != nil {
				return fmt.Errorf("backup.create: %w", err)
			}
			if daemonClient == nil {
				return errors.New("backup.create: daemon client is unavailable")
			}
			target, err := resolveControlTarget(ctx, id)
			if err != nil {
				return fmt.Errorf("backup.create: %w", err)
			}
			actor := jobActor(payload.ActorID)
			// Same record POST /servers/:id/backups writes before it asks the node
			// to build the archive, so the client can track progress meanwhile.
			name := strings.TrimSpace(payload.Name)
			if name == "" {
				name = fmt.Sprintf("backup-%s", time.Now().UTC().Format("20060102T150405Z"))
			}
			stored, storeErr := db.UpsertBackup(ctx, target.ServerID, store.UpsertBackupRequest{Name: name, Status: "pending"}, actor)
			if storeErr != nil {
				return fmt.Errorf("backup.create: record pending backup: %w", storeErr)
			}
			backupCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
			defer cancel()
			entry, daemonErr := daemonClient.CreateBackup(backupCtx, target.NodeURL, target.NodeToken, target.ServerID, append([]string(nil), payload.IgnoredFiles...))
			persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer persistCancel()
			if daemonErr != nil {
				now := time.Now().UTC()
				if _, upsertErr := db.UpsertBackup(persistCtx, target.ServerID, store.UpsertBackupRequest{
					UUID: stored.UUID, Name: stored.Name, Status: "failed", CompletedAt: &now,
				}, actor); upsertErr != nil {
					slogLogger.Error("failed to mark backup as failed", slog.String("server_id", target.ServerID), slog.String("error", upsertErr.Error()))
				}
				return fmt.Errorf("backup.create: create backup %s for server %s: %w", stored.Name, target.ServerID, daemonErr)
			}
			completedAt := time.Now().UTC()
			if entry.Completed != "" {
				if parsed, parseErr := time.Parse(time.RFC3339, entry.Completed); parseErr == nil {
					completedAt = parsed
				}
			}
			// The node names archives "<name>.zip" and may return its own uuid; fall
			// back to the pending record so a successful archive is never lost to an
			// incomplete daemon response.
			completedName := strings.TrimSpace(entry.Name)
			if completedName == "" {
				completedName = stored.Name
			}
			completedUUID := strings.TrimSpace(entry.UUID)
			if completedUUID == "" {
				completedUUID = stored.UUID
			}
			if _, updateErr := db.UpsertBackup(persistCtx, target.ServerID, store.UpsertBackupRequest{
				UUID: completedUUID, Name: completedName, Checksum: entry.Checksum, Size: entry.Size,
				Status: "completed", CompletedAt: &completedAt,
			}, actor); updateErr != nil {
				return fmt.Errorf("backup.create: persist completed backup %s: %w", completedName, updateErr)
			}
			if completedName != stored.Name {
				// The completed record landed under the node's "<name>.zip" key, so close
				// out the pending row as well; unlike the HTTP endpoint this job does not
				// return early to an async callback.
				if _, staleErr := db.UpsertBackup(persistCtx, target.ServerID, store.UpsertBackupRequest{
					UUID: stored.UUID, Name: stored.Name, Checksum: entry.Checksum, Size: entry.Size,
					Status: "completed", CompletedAt: &completedAt,
				}, actor); staleErr != nil {
					slogLogger.Warn("failed to close out pending backup record",
						slog.String("job_id", jobID), slog.String("server_id", target.ServerID),
						slog.String("backup", stored.Name), slog.String("error", staleErr.Error()))
				}
			}
			return nil
		}

		executeBackupRestore := func(ctx context.Context, jobID string, serverID string, raw json.RawMessage) error {
			var payload backupRestorePayload
			if err := decodeJobPayload(raw, &payload); err != nil {
				return fmt.Errorf("backup.restore: %w", err)
			}
			id, err := jobServerID(serverID, payload.ServerID)
			if err != nil {
				return fmt.Errorf("backup.restore: %w", err)
			}
			if daemonClient == nil {
				return errors.New("backup.restore: daemon client is unavailable")
			}
			target, err := resolveControlTarget(ctx, id)
			if err != nil {
				return fmt.Errorf("backup.restore: %w", err)
			}
			actor := jobActor(payload.ActorID)
			backup, err := resolveJobBackupName(ctx, db, target.ServerID, payload.Name)
			if err != nil {
				return fmt.Errorf("backup.restore: backup %q is not available for server %s: %w", payload.Name, target.ServerID, err)
			}
			if backup.Status != "completed" {
				return fmt.Errorf("backup.restore: backup %s status is %q, cannot restore", backup.Name, backup.Status)
			}
			if statusErr := db.MarkBackupStatus(ctx, target.ServerID, backup.Name, "restoring", actor); statusErr != nil {
				slogLogger.Warn("failed to mark backup as restoring",
					slog.String("job_id", jobID), slog.String("server_id", target.ServerID),
					slog.String("backup", backup.Name), slog.String("error", statusErr.Error()))
			}
			restoreCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
			defer cancel()
			if restoreErr := daemonClient.RestoreBackup(restoreCtx, target.NodeURL, target.NodeToken, target.ServerID, backup.Name, payload.Truncate); restoreErr != nil {
				markCtx, markCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				defer markCancel()
				if statusErr := db.MarkBackupStatus(markCtx, target.ServerID, backup.Name, "restore_failed", actor); statusErr != nil {
					slogLogger.Error("failed to mark backup restore as failed",
						slog.String("server_id", target.ServerID), slog.String("backup", backup.Name), slog.String("error", statusErr.Error()))
				}
				return fmt.Errorf("backup.restore: restore backup %s on server %s: %w", backup.Name, target.ServerID, restoreErr)
			}
			if statusErr := db.MarkBackupStatus(ctx, target.ServerID, backup.Name, "restored", actor); statusErr != nil {
				return fmt.Errorf("backup.restore: mark backup %s restored: %w", backup.Name, statusErr)
			}
			return nil
		}

		executeServerTransfer := func(ctx context.Context, jobID string, serverID string, raw json.RawMessage) error {
			var payload serverTransferPayload
			if err := decodeJobPayload(raw, &payload); err != nil {
				return fmt.Errorf("server.transfer: %w", err)
			}
			id, err := jobServerID(serverID, payload.ServerID)
			if err != nil {
				return fmt.Errorf("server.transfer: %w", err)
			}
			if mig == nil {
				return errors.New("server.transfer: migration service is unavailable")
			}
			if !mig.ExecutorAvailable() {
				return errors.New("server.transfer: migration executor (daemon client and runtime) is unavailable")
			}
			// The legacy transfer endpoints were retired in favour of durable
			// migrations, so this mirrors POST /servers/:id/transfer: plan the
			// migration, then hand it to the migration reconciler that main.go
			// starts (mig.Start). A migration that cannot be planned or started is
			// returned as an error, never a silent success.
			created, err := mig.CreateMigration(ctx, migration.CreateMigrationRequest{
				ServerID:     id,
				SourceNodeID: strings.TrimSpace(payload.SourceNodeID),
				TargetNodeID: strings.TrimSpace(payload.TargetNodeID),
			})
			if err != nil {
				return fmt.Errorf("server.transfer: plan migration for server %s: %w", id, err)
			}
			executed, err := mig.ExecuteMigration(ctx, created.ID)
			if err != nil {
				return fmt.Errorf("server.transfer: execute migration %s: %w", created.ID, err)
			}
			switch store.MigrationStatus(executed.Status) {
			case store.MigrationStatusFailed, store.MigrationStatusCancelled:
				return fmt.Errorf("server.transfer: migration %s finished as %s", created.ID, executed.Status)
			}
			slogLogger.Info("server transfer handed to the migration reconciler",
				slog.String("job_id", jobID),
				slog.String("server_id", id),
				slog.String("migration_id", created.ID),
				slog.String("status", executed.Status))
			return nil
		}

		queueSvc.RegisterHandler(queue.JobServerInstall, func(ctx context.Context, job *queue.Job) error {
			return executeServerInstall(ctx, job.ID, job.ServerID, job.Payload)
		})
		queueSvc.RegisterHandler(queue.JobServerUninstall, func(ctx context.Context, job *queue.Job) error {
			return executeServerUninstall(ctx, job.ID, job.ServerID, job.Payload)
		})
		queueSvc.RegisterHandler(queue.JobBackupCreate, func(ctx context.Context, job *queue.Job) error {
			return executeBackupCreate(ctx, job.ID, job.ServerID, job.Payload)
		})
		queueSvc.RegisterHandler(queue.JobBackupRestore, func(ctx context.Context, job *queue.Job) error {
			return executeBackupRestore(ctx, job.ID, job.ServerID, job.Payload)
		})
		queueSvc.RegisterHandler(queue.JobServerTransfer, func(ctx context.Context, job *queue.Job) error {
			return executeServerTransfer(ctx, job.ID, job.ServerID, job.Payload)
		})

		gitSvc = gitsvc.NewService(db, slogLogger)
		gitDeploySvc = gitsvc.NewDeployService(gitSvc, db, slogLogger,
			env("REGISTRY_HOST", ""),
			env("GIT_TEMP_DIR", ""),
		)
		gitDeployMgmtSvc = gitsvc.NewDeploymentManagementService(db, slogLogger, gitDeploySvc)
		gitProviderSvc = gitprovidersvc.NewService(db, slogLogger)
		gitOpsController, err = composesvc.NewGitOpsController(
			db,
			gitDeploySvc,
			composeLifecycle,
			daemonClient,
			outboxPub,
			slogLogger,
			env("GITOPS_WORKER_ID", ""),
		)
		if err != nil {
			return fmt.Errorf("create gitops controller: %w", err)
		}
		gitOpsController.Start(appCtx)

		appStoreSvc, err = appstoresvc.New(db, composeLifecycle)
		if err != nil {
			return fmt.Errorf("create app store service: %w", err)
		}
		// Populate the built-in app-store catalog (nginx/postgres/redis/…). Without
		// this, GET /app-store/apps returns null and installs fail "app not found".
		// Idempotent upsert; a failure is logged, not fatal to boot.
		if err := appStoreSvc.SeedDefaultApps(appCtx); err != nil {
			slogLogger.Warn("seed app store catalog failed", slog.String("error", err.Error()))
		}
		// Upsert the embedded Coolify compose catalog (371 one-click services).
		// Runs after the hand-written seed so shared keys keep their richer
		// defaults; idempotent by key. Non-fatal to boot.
		if _, err := appStoreSvc.SeedBundledTemplates(appCtx); err != nil {
			slogLogger.Warn("seed bundled app-store templates failed", slog.String("error", err.Error()))
		}

		// One-click service catalog (postgres/redis/rabbitmq/…).
		// Initialized after dbContainerSvc/composeLifecycle are ready (see below).

		queueSvc.Start(appCtx)

		opStore := operationsvc.NewPostgresStore(db.GetDB())
		opSvc = operationsvc.New(opStore)
		// Installer workflow visibility (DB->UI) is always on; execution stays
		// gated by INSTALLER_WORKFLOW_ENABLED plus an attached executor, so no
		// executor is set here and the UI honestly reports executionEnabled=false.
		installerSvc = installersvc.New(installersvc.NewPostgresStore(db.GetDB()))
		registerPowerOp := func(opType operationsvc.OperationType, signal string) {
			opSvc.RegisterHandler(opType, func(ctx context.Context, op *operationsvc.Operation) error {
				commandCtx := daemon.ContextWithCommandID(ctx, op.ID)
				_, _, err := cm.RequestServerPower(commandCtx, op.ResourceID, signal)
				if err == nil {
					event := map[string]string{"start": "server:started", "stop": "server:stopped", "restart": "server:restarted", "kill": "server:stopped"}[signal]
					if event != "" {
						if err := db.DispatchWebhookEvent(ctx, event, map[string]any{"subject_type": "server", "subject_id": op.ResourceID, "signal": signal, "operation_id": op.ID}); err != nil {
							slogLogger.Error("webhook dispatch failed", slog.String("event", event), slog.String("error", err.Error()))
						}
					}
				}
				return err
			})
		}
		registerPowerOp(operationsvc.OpServerStart, "start")
		registerPowerOp(operationsvc.OpServerStop, "stop")
		registerPowerOp(operationsvc.OpServerRestart, "restart")
		registerPowerOp(operationsvc.OpServerKill, "kill")
		opSvc.RegisterHandler(operationsvc.OpComposeDeploy, func(ctx context.Context, op *operationsvc.Operation) error {
			return composeQH.HandleDeploy(ctx, op.Input)
		})
		opSvc.RegisterHandler(operationsvc.OpComposeUpdate, func(ctx context.Context, op *operationsvc.Operation) error {
			return composeQH.HandleUpdate(ctx, op.Input)
		})
		opSvc.RegisterHandler(operationsvc.OpComposeDelete, func(ctx context.Context, op *operationsvc.Operation) error {
			return composeQH.HandleDelete(ctx, op.Input)
		})
		opSvc.RegisterHandler(operationsvc.OpComposeStart, func(ctx context.Context, op *operationsvc.Operation) error {
			return composeQH.HandleStart(ctx, op.Input)
		})
		opSvc.RegisterHandler(operationsvc.OpComposeStop, func(ctx context.Context, op *operationsvc.Operation) error {
			return composeQH.HandleStop(ctx, op.Input)
		})
		opSvc.RegisterHandler(operationsvc.OpComposeRestart, func(ctx context.Context, op *operationsvc.Operation) error {
			return composeQH.HandleRestart(ctx, op.Input)
		})
		// The durable operation service declares the same server install,
		// uninstall, transfer and backup kinds, so they get the identical executors
		// reading the operation's resource id and input instead of the job's.
		opSvc.RegisterHandler(operationsvc.OpServerInstall, func(ctx context.Context, op *operationsvc.Operation) error {
			return executeServerInstall(ctx, op.ID, op.ResourceID, op.Input)
		})
		opSvc.RegisterHandler(operationsvc.OpServerUninstall, func(ctx context.Context, op *operationsvc.Operation) error {
			return executeServerUninstall(ctx, op.ID, op.ResourceID, op.Input)
		})
		opSvc.RegisterHandler(operationsvc.OpBackupCreate, func(ctx context.Context, op *operationsvc.Operation) error {
			return executeBackupCreate(ctx, op.ID, op.ResourceID, op.Input)
		})
		opSvc.RegisterHandler(operationsvc.OpBackupRestore, func(ctx context.Context, op *operationsvc.Operation) error {
			return executeBackupRestore(ctx, op.ID, op.ResourceID, op.Input)
		})
		opSvc.RegisterHandler(operationsvc.OpServerTransfer, func(ctx context.Context, op *operationsvc.Operation) error {
			return executeServerTransfer(ctx, op.ID, op.ResourceID, op.Input)
		})
		opSvc.Start(appCtx)

		runtimeRegistry = runtimesvc.NewRegistry()

		var waSessionStore webauthn.SessionStore
		if redisClient != nil {
			waSessionStore = webauthn.NewRedisSessionStore(redisClient)
		} else {
			waSessionStore = newInMemoryWebAuthnSessionStore()
		}
		waSvc, err = webauthn.New(
			env("WEBAUTHN_RP_ID", "localhost"),
			env("WEBAUTHN_RP_DISPLAY_NAME", "GamePanel"),
			env("WEBAUTHN_RP_ORIGIN", "http://localhost:3000"),
			webauthn.NewPostgresCredentialStore(db.GetDB()),
			waSessionStore,
		)
		if err != nil {
			return fmt.Errorf("create webauthn service: %w", err)
		}

		autoSvc = autoscaler.New(db, cm, dockerRT, outboxPub)
		deploySvc = deployment.New(db, outboxPub)
		// Without this the deployment steps have no way to reach a node, and
		// every step that claims to change what is running fails closed.
		deployment.WireBeaconExecutor(deploySvc, db, daemonClient)
		// Resume any deployments that were in flight when the previous process
		// exited, so they are not orphaned until a manual /deployments/resume.
		if err := deploySvc.ResumeDeployments(appCtx); err != nil {
			slogLogger.Error("resume deployments at boot failed", slog.String("error", err.Error()))
		}
		lbSvc = loadbalancer.New(db, outboxPub)

		healthCheckRunner = healthchecksvc.New(db, healthchecksvc.DefaultConfig())
		var rollbackMu sync.Mutex
		healthCheckRunner.OnUnhealthy(func(ctx context.Context, serverID string, targetID string, consecutiveFailures int) {
			rollbackMu.Lock()
			defer rollbackMu.Unlock()

			deps, err := db.ListDeployments(ctx, serverID)
			if err != nil {
				slogLogger.Error("health check bridge: list deployments", slog.String("serverId", serverID), slog.String("error", err.Error()))
				return
			}
			for _, d := range deps {
				if !d.RollbackOnHealthFailure {
					continue
				}
				switch d.Status {
				case string(deployment.StatusInProgress), string(deployment.StatusPending), string(deployment.StatusProvisioning), string(deployment.StatusAwaitingHealth), string(deployment.StatusPromoting), string(deployment.StatusRollbackPending), string(deployment.StatusRollingBack):
					if err := db.UpdateDeploymentStatus(ctx, d.ID, string(deployment.StatusRollbackPending),
						fmt.Sprintf("auto-rollback triggered by runtime health degradation (target %s, %d failures)", targetID, consecutiveFailures)); err != nil {
						slogLogger.Error("health check bridge: update status", slog.String("deploymentId", d.ID), slog.String("error", err.Error()))
					}
					_, rollbackErr := deploySvc.RollbackToPrevious(ctx, d.ID)
					if rollbackErr != nil {
						slogLogger.Error("health check bridge: auto-rollback failed", slog.String("deploymentId", d.ID), slog.String("serverId", serverID), slog.String("error", rollbackErr.Error()))
						continue
					}
					if err := db.UpdateDeploymentStatus(ctx, d.ID, string(deployment.StatusRollingBack), ""); err != nil {
						slogLogger.Error("health check bridge: update status", slog.String("deploymentId", d.ID), slog.String("error", err.Error()))
					}
					if err := db.UpdateDeploymentStatus(ctx, d.ID, string(deployment.StatusRolledBack),
						fmt.Sprintf("auto-rollback due to runtime health degradation (target %s, %d failures)", targetID, consecutiveFailures)); err != nil {
						slogLogger.Error("health check bridge: update status", slog.String("deploymentId", d.ID), slog.String("error", err.Error()))
					}
				}
			}
		})
		healthCheckRunner.Start(appCtx)
		rec.SetHealthChecker(healthCheckRunner.ReconcilerAdapter())
		failSvc = failover.New(db, outboxPub)
		runFailoverAction := func(ctx context.Context, event *failover.Event) error {
			switch event.Action {
			case failover.FailoverActionEvacuate:
				node, err := db.GetNode(ctx, event.NodeID)
				if err != nil {
					return err
				}
				if node.ActualState == string(store.NodeActualStateOffline) {
					plan, err := rcv.CreatePlan(ctx, recoverysvc.CreatePlanRequest{NodeID: event.NodeID, Reason: event.Message})
					if err != nil {
						return err
					}
					_, err = rcv.ExecutePlan(ctx, plan.ID)
					return err
				}
				result, err := ep.CreatePlan(ctx, event.NodeID)
				if err != nil {
					return err
				}
				_, err = ep.ExecutePlan(ctx, result.Plan.ID)
				return err
			case failover.FailoverActionRestart:
				servers, err := db.ListServersForNode(ctx, event.NodeID)
				if err != nil {
					return err
				}
				for _, server := range servers {
					if _, err := cm.RestartServer(ctx, server.ID); err != nil {
						return fmt.Errorf("restart server %s: %w", server.ID, err)
					}
				}
			}
			return nil
		}
		failSvc.SetActionExecutor(func(_ context.Context, event *failover.Event) error {
			// Event subscribers have a short delivery deadline, while a game
			// migration or backup restore can legitimately take much longer.
			// Claiming the policy cooldown happens before this durable workflow
			// is launched, preventing relay retries from starting duplicates.
			eventCopy := *event
			go func() {
				defer func() {
					if r := recover(); r != nil {
						buf := make([]byte, 4096)
						n := runtime.Stack(buf, false)
						slogLogger.Error("failover action panic recovered",
							slog.String("node_id", eventCopy.NodeID),
							slog.String("action", string(eventCopy.Action)),
							slog.String("panic", fmt.Sprintf("%v", r)),
							slog.String("stack", string(buf[:n])))
					}
				}()
				ctx, cancel := context.WithTimeout(appCtx, 2*time.Hour)
				defer cancel()
				if err := runFailoverAction(ctx, &eventCopy); err != nil {
					slogLogger.Error("automatic failover action failed",
						slog.String("node_id", eventCopy.NodeID),
						slog.String("action", string(eventCopy.Action)),
						slog.String("error", err.Error()))
				}
			}()
			return nil
		})
		failSvc.SetWorkloadClassifier(func(ctx context.Context, nodeID string) (failover.FailoverAction, error) {
			servers, err := db.ListServersForNode(ctx, nodeID)
			if err != nil {
				return failover.FailoverActionNotify, err
			}
			allShared := len(servers) > 0
			for _, server := range servers {
				locality, _ := ep.StorageLocality(ctx, server.ID)
				if locality == evacuationplanner.StorageLocalOnly {
					return failover.FailoverActionNotify, nil
				}
				policy := ep.ReplacementPolicyForServer(ctx, server, locality)
				if policy == evacuationplanner.ReplacementPolicyProtect {
					return failover.FailoverActionNotify, nil
				}
			}
			if allShared {
				return failover.FailoverActionEvacuate, nil
			}
			return failover.FailoverActionNotify, nil
		})
		eventRegistry.Subscribe(events.EventNodeOffline, failSvc)
		crashDetector = crashdetector.New(crashdetector.DefaultConfig(), db)
		crashDetector.OnCrash(func(ctx context.Context, serverID string, crashCount int) {
			outboxPub.Publish(ctx, events.NewEnvelope(
				events.EventServerCrashed,
				eventRegistry.Source(),
				"server",
				serverID,
				map[string]any{
					"server_id":   serverID,
					"crash_count": crashCount,
				},
			))
		})
		crashDetector.OnSuspend(func(ctx context.Context, serverID string) {
			outboxPub.Publish(ctx, events.NewEnvelope(
				events.EventServerSuspended,
				eventRegistry.Source(),
				"server",
				serverID,
				map[string]any{
					"server_id": serverID,
				},
			))
		})
		bkSvc = backup.New(db)
		bkSvc.SetRetentionDays(envInt("BACKUP_RETENTION_DAYS", 30))
		backup.RegisterProvider("s3", backup.NewS3Factory)
		backup.RegisterProvider("gcs", backup.NewGCSFactory)
		backup.RegisterProvider("azure", backup.NewAzureFactory)
		backup.RegisterProvider("local", backup.NewLocalFactory)
		bkWorker = backup.NewWorker(db, bkSvc, daemonClient)
		// Give the worker a fully-wired job service so its out-of-band pickup of
		// pending/failed backup jobs actually runs (otherwise it short-circuits).
		bkAdmin := backup.NewMainService(db, backup.NewSlogLogger(slogLogger))
		bkAdmin.SetDaemonClient(daemonClient)
		bkWorker.SetJobService(bkAdmin.JobService())
		// Cron-scheduled admin backup configurations: give the worker the config
		// service so RunDueConfigs fires due schedules each tick.
		bkWorker.SetConfigService(bkAdmin.ConfigService())
		// Restic / Kopia backup engines: repository registration, scheduled
		// snapshots and restores. The service shells out to the restic/kopia CLI on
		// the control-plane host or on a bound beacon node through the daemon.
		backupEngineSvc = backupenginesvc.New(db, daemonClient, slogLogger)
		dnsSvc, err = dnssvc.New(db)
		if err != nil {
			return fmt.Errorf("create dns service: %w", err)
		}
		vaultSvc, err = vaultprovider.New(db)
		if err != nil {
			return fmt.Errorf("create vault provider service: %w", err)
		}
		// Install the Vault reference resolver into the environment-variable
		// resolution path so a `vault:<connection-id>/<path>#<field>` value is
		// fetched live from the named connection at deploy time. It is only
		// consulted for values carrying the reference prefix; everything else is
		// passed through exactly as before.
		db.SetVaultResolver(vaultSvc.ResolveIfReference)
		caddyProxy := trafficmanager.NewCaddyReverseProxy(env("CADDY_ADMIN_ADDR", "127.0.0.1:2019"))
		caddyTLS = trafficmanager.NewCaddyTLSManager(env("CADDY_ADMIN_ADDR", "127.0.0.1:2019"))
		acmeSvc = acmesvc.New(db, slogLogger)
		dnsSvc.RegisterWithAcme(func(name string, factory func(providerName string, credentials map[string]string) (challenge.Provider, error)) {
			acmeSvc.RegisterDNSProvider(name, factory)
		})
		// Route issued ACME certificates into the live Caddy gateway so HTTPS
		// actually serves them; previously issuance persisted a DB row only.
		acmeSvc.SetGateway(caddyGatewayCertInstaller{proxy: caddyProxy})
		discoverySvc = servicediscovery.New(db, servicediscovery.NewEndpointStore(db.GetDB()), outboxPub)
		crossNodeResolver = crossnode.NewResolver(resolutionStoreAdapter{db})
		crossNodeResolver.SetServiceDiscovery(discoverySvc)
		discoverySvc.Start(appCtx)

		healthFilter = crossnode.NewHealthFilter(2, 30*time.Second)
		healthFilter.StartReaper(appCtx, 5*time.Minute)
		ingressSync = crossnode.NewIngressSynchronizer(caddyProxy, crossNodeResolver, healthFilter, outboxPub)
		ingressSync.Start(appCtx, 30*time.Second)

		// NetBird mesh VPN control plane client (nil-safe when NETBIRD_API_URL /
		// NETBIRD_API_TOKEN are unset).
		netbirdSvc = netbirdsvc.New(db, slogLogger)

		domainNodeResolver := &domainNodeResolver{store: db}
		domainSvc = domains.New(store.NewDomainAdapter(db), caddyProxy, env("PANEL_IP", ""), outboxPub)
		domainSvc.SetNodeResolver(domainNodeResolver)
		buildSvc = buildsvc.NewService(db, daemonClient, slogLogger)
		tenancySvc = tenancy.New(db)
		procedureSvc = proceduresvc.New(db, outboxPub, slogLogger, db)
		apphostingSvc = apphostingsvc.New(db, tenancySvc, composeStackDeployer{lifecycle: composeLifecycle})
		onboardingSvc = onboardingsvc.NewService(db, gitSvc, apphostingSvc, slogLogger)
		endpointSvc = environments.New(db)
		if psvc, perr := pipelinesvc.New(pipelinesvc.Options{
			Store:          pipelinesvc.NewStore(db.GetDB()),
			SharedStore:    db,
			Daemon:         daemonClient,
			BuildService:   buildSvc,
			ComposeService: composeLifecycle,
			DeployService:  deploySvc,
			Logger:         slogLogger,
			DataDir:        env("PIPELINE_DATA_DIR", "./data/pipelines"),
		}); perr != nil {
			return fmt.Errorf("create pipeline service: %w", perr)
		} else {
			pipelineSvc = psvc
			pipelineSvc.Start(appCtx)
		}
		alertSvc = alerting.New(db, alerting.DefaultThresholds, slogLogger)
		obs.SetNodeMetricHook(func(m store.NodeMetric) {
			_ = alertSvc.CheckNodeThresholds(appCtx, m)
		})
		notifSvc = notification.New(db, slogLogger)
		if err := notifSvc.RefreshChannels(appCtx); err != nil {
			slogLogger.Warn("failed to refresh notification channels", slog.String("error", err.Error()))
		}
		// Notifications engine: replaces the per-event legacy subscriptions with
		// a single wildcard consumer that understands the catalog (canonical +
		// legacy event names) and performs concurrent fan-out with templating.
		notifRouter = notifs.NewRouter(db, db, slogLogger)
		notifRouter.RegisterWith(eventRegistry)
		// Control-plane events published to the durable webhook outbox (power
		// operations, node CRUD, compose gitops) reach the engine through the
		// same choke point, so channel notifications mirror outbound webhooks.
		db.SetWebhookEventHook(func(ctx context.Context, event string, payload map[string]any) {
			if err := notifRouter.DispatchEvent(ctx, event, payload); err != nil {
				slogLogger.Warn("notification dispatch failed", "event", event, "error", err.Error())
			}
		})
		membershipSvc = clustermembership.New(db, outboxPub)
		membershipSvc.SetEvacuationPlanner(ep)

		nodeAutoSvc = nodeautoscale.New(db, slogLogger).WithCloud(cloudMgr).WithMembership(membershipSvc)
		nodeAutoSvc.Start(appCtx)
		cleanupSvc = cleanupsvc.New(db, outboxPub)
		cleanupSvc.Start(appCtx)
		billingSvc = billingsvc.New(db)
		billingSvc.StartReaper(appCtx)
		drainLedger = drainsvc.New(db, slogLogger)
		placementSvc = envaffinitysvc.New(db, slogLogger).WithPredictiveScorer(predictiveScorer)
		// Mirror membership/evacuation drain events into the durable ledger so
		// drain progress survives restarts. The ledger records only; the
		// orchestration stays owned by clustermembership.
		drainLedgerSub := drainLedger.Subscriber()
		for _, et := range []events.EventType{
			events.EventNodeDrainingStarted,
			events.EventEvacuationPlanCreated,
			events.EventEvacuationPlanFailed,
			events.EventNodeDrainingCompleted,
		} {
			eventRegistry.Subscribe(et, drainLedgerSub)
		}
		dbContainerSvc = dbprovisioner.NewDBContainerService(db, daemonClient, env("BEACON_BASE_URL", "http://127.0.0.1:9090"), env("DAEMON_NODE_TOKEN", ""), env("DOCKER_HOST", "127.0.0.1"))
		// One-click service catalog (postgres/redis/rabbitmq/…). Depends on
		// dbContainerSvc and composeLifecycle being initialised above.
		catalogSvc, err = catalogsvc.New(catalogsvc.Options{
			Store:        db,
			DBProvider:   dbContainerSvc,
			ComposeStack: composeLifecycle,
			EnvSvc:       envvarsvc.New(db),
			Logger:       slogLogger,
		})
		if err != nil {
			return fmt.Errorf("create catalog service: %w", err)
		}
		// FORGEFILE_BASE_DOMAIN is the var the dashboard documents and the
		// forgefile package declares (forgefile.BaseDomainEnv); it was only
		// ever read by a route registrar whose forgefile routes were shadowed,
		// so setting it changed nothing about the links a manifest apply
		// produced. PANEL_BASE_DOMAIN stays the fallback for deployments that
		// set only that one.
		forgefileSvc = forgefile.NewService(db, apphostingSvc, slogLogger, env(forgefile.BaseDomainEnv, env("PANEL_BASE_DOMAIN", "")))
		dbSvcProv = services.NewDatabaseServiceProvisioner(db, daemonClient, env("BEACON_BASE_URL", "http://127.0.0.1:9090"), env("DAEMON_NODE_TOKEN", ""), env("DOCKER_HOST", "127.0.0.1"), masterKeyring)
		dbBackupSvc = dbbackupsvc.New(db, dbbackupsvc.NewLocalStorage(env("DB_BACKUP_STORAGE_DIR", ".dev-data/db-backups")))
		tmSvc = trafficmanager.NewWithPersistence(db, db, db, db, caddyProxy, outboxPub)
		eventRegistry.Subscribe(events.EventNodeOffline, tmSvc)
		eventRegistry.Subscribe(events.EventNodeRecovered, tmSvc)
		tmSvc.Start(appCtx)
		previewDeploySvc = previewenv.New(db, previewenv.Options{
			Publisher:   outboxPub,
			Logger:      slogLogger,
			PanelURL:    panelURL,
			GitService:  gitSvc,
			AcmeService: acmeSvc,
			TrafficMgr:  tmSvc,
			DomainSvc:   domainSvc,
		})
		previewDeploySvc.StartReaper(appCtx, 5*time.Minute)
		eventRegistry.Subscribe(events.EventNodeOffline, lbSvc)
		eventRegistry.Subscribe(events.EventNodeRecovered, lbSvc)
		eventRegistry.Subscribe(events.EventNodeOnline, events.HandlerFunc(func(ctx context.Context, _ events.Envelope) error {
			crossNodeResolver.ClearCache()
			if ingressSync != nil {
				if err := ingressSync.Sync(ctx); err != nil {
					slogLogger.Error("ingress sync failed", slog.String("event", "node.online"), slog.String("error", err.Error()))
				}
			}
			return nil
		}))
		eventRegistry.Subscribe(events.EventNodeOffline, events.HandlerFunc(func(ctx context.Context, _ events.Envelope) error {
			crossNodeResolver.ClearCache()
			if ingressSync != nil {
				if err := ingressSync.Sync(ctx); err != nil {
					slogLogger.Error("ingress sync failed", slog.String("event", "node.offline"), slog.String("error", err.Error()))
				}
			}
			return nil
		}))
		eventRegistry.Subscribe(events.EventNodeRecovered, events.HandlerFunc(func(ctx context.Context, _ events.Envelope) error {
			crossNodeResolver.ClearCache()
			if ingressSync != nil {
				if err := ingressSync.Sync(ctx); err != nil {
					slogLogger.Error("ingress sync failed", slog.String("event", "node.recovered"), slog.String("error", err.Error()))
				}
			}
			return nil
		}))
		eventRegistry.Subscribe(events.EventNodeReconciling, events.HandlerFunc(func(ctx context.Context, envelope events.Envelope) error {
			slog.Info("node reconciling", "nodeId", envelope.ResourceID, "payload", envelope.Payload)
			return nil
		}))
		lbSvc.Start(appCtx)

		// Wire observability as a catch-all event subscriber so every domain
		// event is persisted to the timeline.
		eventRegistry.Subscribe(events.WildcardEventType, obs)
		eventRegistry.Subscribe(events.WildcardEventType, whSvc)

		cronJobSvc, err = cronjobsvc.New(db, slogLogger)
		if err != nil {
			return fmt.Errorf("create cron job service: %w", err)
		}

		scheduledTaskSvc, err = scheduledtaskssvc.New(db, daemonClient, slogLogger, "api-"+uuid.NewString())
		if err != nil {
			return fmt.Errorf("create scheduled tasks service: %w", err)
		}

		zdSvc = zerodowntime.New(db)
		zdSvc.SetRollbackExecutor(func(ctx context.Context, serverID, imageTag string) error {
			if _, err := db.UpdateServer(ctx, serverID, store.UpdateServerRequest{DockerImage: &imageTag}, nil); err != nil {
				return err
			}
			return cm.SyncServerConfiguration(ctx, serverID)
		})

		processSvc = processsvc.New(db, &processDaemonAdapter{store: db, daemon: daemonClient}, slogLogger)
		buildpackSvc = buildpacksvc.NewService(db, buildSvc)
		if err := buildSvc.Start(appCtx); err != nil {
			return fmt.Errorf("start build recovery: %w", err)
		}
		if err := buildpackSvc.Start(appCtx); err != nil {
			return fmt.Errorf("recover app builds: %w", err)
		}

		certSvc = services.NewCertService(db, db, slogLogger)
		mtlsCfg = forgecfg.MTLSConfig()
		if mtlsCfg.AutoMigrate {
			mtlsMigrator = services.NewMTLSMigrator(certSvc, db, slogLogger)
			if err := mtlsMigrator.Run(appCtx); err != nil {
				slogLogger.Warn("mTLS auto-migration failed", slog.String("error", err.Error()))
			}
		} else {
			mtlsMigrator = services.NewMTLSMigrator(certSvc, db, slogLogger)
		}

		// Start background services.
		if err := cronJobSvc.Start(appCtx); err != nil {
			slogLogger.Error("cron job service startup failed", slog.String("error", err.Error()))
		}
		if err := scheduledTaskSvc.Start(appCtx); err != nil {
			slogLogger.Error("scheduled tasks service startup failed", slog.String("error", err.Error()))
		}
		if err := backupEngineSvc.Start(appCtx); err != nil {
			slogLogger.Error("backup engine service startup failed", slog.String("error", err.Error()))
		}
		resMgr.Start(appCtx)
		hbm.Start(appCtx)
		rec.Start(appCtx)
		mig.Start(appCtx)
		ep.Start(appCtx)
		mailWorker.Start(appCtx)
		whSvc.Start(appCtx)
		if err := failSvc.Start(appCtx); err != nil {
			slogLogger.Error("failover startup failed", slog.String("error", err.Error()))
		}
		bkWorker.Start(appCtx)
		eventRelay.Start(appCtx)
		domainSvc.StartReverify(appCtx)
		acmeSvc.StartAutoRenewal(appCtx)
		procedureSvc.Start(appCtx)
		replicaMgr.Start(appCtx)
		if err := autoSvc.Start(appCtx); err != nil {
			slogLogger.Error("autoscaler startup failed", slog.String("error", err.Error()))
		}

		sessionStore = auth.NewPostgresSessionStore(db.GetDB())
		go func() {
			defer func() {
				if r := recover(); r != nil {
					buf := make([]byte, 4096)
					n := runtime.Stack(buf, false)
					slogLogger.Error("session cleanup panic recovered", "panic", r, "stack", string(buf[:n]))
				}
			}()
			ticker := time.NewTicker(time.Hour)
			defer ticker.Stop()
			for {
				select {
				case <-appCtx.Done():
					return
				case <-ticker.C:
					if err := sessionStore.Cleanup(appCtx); err != nil {
						slogLogger.Error("session cleanup failed", slog.String("error", err.Error()))
					}
				}
			}
		}()
	}

	langsDir := env("LANGS_DIR", "lang")
	translator, err := i18n.New(i18n.Config{
		LangsDir: langsDir,
		Fallback: "en",
	})
	if err != nil {
		slogLogger.Warn("i18n service failed to load translations; continuing without translations", slog.String("langs_dir", langsDir), slog.String("error", err.Error()))
	}

	healthSvc := health.NewService(version.Version)
	if db != nil {
		healthSvc.AddCheck(health.NewDatabaseCheck(
			db.PingDatabase,
			func(ctx context.Context) (map[string]any, error) {
				details, err := db.DatabaseHealthDetails(ctx)
				if err != nil {
					return nil, err
				}
				return map[string]any{
					"role":              "panel metadata store",
					"engine":            "PostgreSQL",
					"version":           details.Version,
					"activeConnections": details.ActiveConnections,
					"migrationCount":    details.MigrationCount,
				}, nil
			},
		))
	}
	healthSvc.AddCheck(health.NewCacheCheck(
		func(ctx context.Context) error {
			if redisClient == nil {
				return nil
			}
			return redisClient.Ping(ctx).Err()
		},
		func(ctx context.Context) (map[string]any, error) {
			if redisClient == nil {
				return nil, nil
			}
			info, err := redisClient.Info(ctx, "memory", "clients").Result()
			if err != nil {
				return nil, err
			}
			details := make(map[string]any)
			for _, line := range strings.Split(info, "\n") {
				line = strings.TrimSpace(line)
				key, value, ok := strings.Cut(line, ":")
				if !ok {
					continue
				}
				switch key {
				case "used_memory_human", "connected_clients":
					details[key] = value
				}
			}
			return details, nil
		},
		redisEnabled && redisClient != nil,
	))
	healthSvc.AddCheck(health.NewDaemonCheck(func(ctx context.Context) (int, int, int, map[string]any, error) {
		if nr == nil {
			return 0, 0, 0, nil, nil
		}
		nodes, err := nr.ListNodes(ctx)
		if err != nil {
			return 0, 0, 0, nil, err
		}
		healthy := 0
		unhealthy := 0
		oldestHeartbeatAgeSeconds := int64(0)
		hasPersistedHeartbeat := false
		nodesWithoutHeartbeat := 0
		for _, node := range nodes {
			// HeartbeatState is persisted by the heartbeat monitor. Do not use the
			// legacy Status field here: it is not a live connectivity result.
			if node.HeartbeatState == string(store.NodeHeartbeatStateHealthy) {
				healthy++
			} else {
				unhealthy++
			}
			if node.LastSeenAt == nil {
				nodesWithoutHeartbeat++
				continue
			}
			hasPersistedHeartbeat = true
			age := time.Since(*node.LastSeenAt).Seconds()
			if int64(age) > oldestHeartbeatAgeSeconds {
				oldestHeartbeatAgeSeconds = int64(age)
			}
		}
		details := map[string]any{
			"healthyHeartbeatNodes":    healthy,
			"nonHealthyHeartbeatNodes": unhealthy,
			"nodesWithoutHeartbeat":    nodesWithoutHeartbeat,
		}
		if hasPersistedHeartbeat {
			details["oldestHeartbeatAgeSeconds"] = oldestHeartbeatAgeSeconds
		}
		return len(nodes), healthy, unhealthy, details, nil
	}))
	healthSvc.AddCheck(health.NewAPIRuntimeCheck(started))
	healthSvc.AddCheck(health.NewMemoryCheck(0))
	healthSvc.AddCheck(health.NewSystemCheck(started))

	cfg := config.Config{
		App: config.AppConfig{
			Env:            appEnv,
			Name:           env("APP_NAME", "GamePanel"),
			URL:            env("PANEL_URL", "http://localhost:3000"),
			Debug:          envBool("APP_DEBUG", false),
			Version:        env("APP_VERSION", "0.1.0"),
			Key:            env("APP_KEY", ""),
			Cipher:         env("APP_CIPHER", "AES-256-GCM"),
			Locale:         env("APP_LOCALE", "en"),
			FallbackLocale: env("APP_FALLBACK_LOCALE", "en"),
			MigrationsDir:  env("MIGRATIONS_DIR", "migrations"),
			PluginsDir:     env("PLUGINS_DIR", ""),
			LangsDir:       env("LANGS_DIR", "lang"),
		},
		Server: config.ServerConfig{
			Addr:        env("API_ADDR", ":8080"),
			ReadTimeout: 5 * time.Second,
			PanelURL:    env("PANEL_URL", "http://localhost:3000"),
		},
		DB: config.DBConfig{
			Driver: env("DB_CONNECTION", "postgres"),
			URL:    os.Getenv("DATABASE_URL"),
		},
		Redis: config.RedisConfig{
			Addr:     env("REDIS_ADDR", ""),
			Password: env("REDIS_PASSWORD", ""),
			DB:       envInt("REDIS_DB", 0),
			Enabled:  redisEnabled,
		},
		Auth: config.AuthConfig{
			Secret:   authSecret,
			TokenTTL: time.Duration(envInt("AUTH_TOKEN_TTL", 24)) * time.Hour,
		},
		Mail: config.MailConfig{
			Driver:      env("MAIL_MAILER", "log"),
			Host:        env("MAIL_HOST", "127.0.0.1"),
			Port:        envInt("MAIL_PORT", 587),
			Encryption:  env("MAIL_ENCRYPTION", "tls"),
			Username:    env("MAIL_USERNAME", ""),
			Password:    env("MAIL_PASSWORD", ""),
			FromAddress: env("MAIL_FROM_ADDRESS", "noreply@gamepanel.local"),
			FromName:    env("MAIL_FROM_NAME", "GamePanel"),
		},
		Daemon: config.DaemonConfig{
			NodeToken: env("DAEMON_NODE_TOKEN", ""),
		},
		Backup: config.BackupConfig{
			Driver:        env("BACKUP_DRIVER", "s3"),
			RetentionDays: envInt("BACKUP_RETENTION_DAYS", 30),
			MaxBackups:    envInt("BACKUP_MAX_BACKUPS", 10),
		},
		Log: config.LogConfig{
			Level:  env("LOG_LEVEL", "info"),
			Format: env("LOG_FORMAT", "text"),
			Output: env("LOG_OUTPUT", "stdout"),
		},
	}

	if cfg.App.Key == "" && strings.HasPrefix(cfg.App.Cipher, "AES-") {
		return errors.New("APP_KEY must be non-empty when APP_CIPHER is AES-based")
	}
	if err := validateConfig(&cfg, slogLogger); err != nil {
		return err
	}

	// Incus (containers/VMs) and Nomad (workload orchestration) runtime
	// integrations. Both are nil-safe and configured from the environment / node
	// registry, so they are always constructed and passed to the HTTP layer.
	incusSvc := incussvc.New(db, slogLogger)
	nomadSvc := nomadsvc.New(db, slogLogger)

	appCfg := http.Config{
		Logger:                     slogLogger,
		Addr:                       env("API_ADDR", ":8080"),
		ReadTimeout:                5 * time.Second,
		TokenTTL:                   cfg.Auth.TokenTTL,
		AppEnv:                     appEnv,
		AuthSecret:                 authSecret,
		LangsDir:                   langsDir,
		Store:                      db,
		Redis:                      redisClient,
		RedisEnabled:               redisEnabled,
		Daemon:                     daemonClient,
		BackgroundContext:          appCtx,
		PanelURL:                   env("PANEL_URL", "http://localhost:3000"),
		PluginsDir:                 env("PLUGINS_DIR", ""),
		PluginService:              pluginSvc,
		CORSConfig:                 http.DefaultCORSConfig(),
		NodeRegistry:               nr,
		NodeProbe:                  np,
		ClusterManager:             cm,
		EvacuationPlanner:          ep,
		MigrationService:           mig,
		ReservationManager:         resMgr,
		RecoveryCoordinator:        rcv,
		RecoveryTokenService:       rts,
		HeartbeatMonitor:           hbm,
		Observability:              obs,
		Reconciler:                 rec,
		EventRegistry:              eventRegistry,
		DBProvisioner:              dbProv,
		HealthService:              healthSvc,
		SessionStore:               sessionStore,
		MailTriggerService:         mailTriggerSvc,
		QueueService:               queueSvc,
		OperationService:           opSvc,
		RuntimeRegistry:            runtimeRegistry,
		WorkloadRuntime:            workloadRuntime,
		IncusService:               incusSvc,
		NomadService:               nomadSvc,
		WebAuthnService:            waSvc,
		ActivityService:            actSvc,
		AuditLogService:            auditLogSvc,
		EventRelay:                 eventRelay,
		AutoScaler:                 autoSvc,
		NodeAutoscaler:             nodeAutoSvc,
		CrashDetector:              crashDetector,
		DeploymentSvc:              deploySvc,
		PreviewDeploymentSvc:       previewDeploySvc,
		CloudManager:               cloudMgr,
		LoadBalancer:               lbSvc,
		FailoverSvc:                failSvc,
		TrafficManager:             tmSvc,
		CaddyTLS:                   caddyTLS,
		PredictiveScorer:           predictiveScorer,
		ConstraintScheduler:        constraintSched,
		BackupSvc:                  bkSvc,
		BackupEngineService:        backupEngineSvc,
		DNSService:                 dnsSvc,
		VaultService:               vaultSvc,
		AcmeService:                acmeSvc,
		DomainService:              domainSvc,
		BuildService:               buildSvc,
		Translator:                 translator,
		GitService:                 gitSvc,
		GitDeployService:           gitDeploySvc,
		GitProviderService:         gitProviderSvc,
		ComposeService:             composeLifecycle,
		ComposeTemplateService:     composeTemplateSvc,
		DBContainerService:         dbContainerSvc,
		DatabaseServiceProvisioner: dbSvcProv,
		DBBackupService:            dbBackupSvc,
		TenancyService:             tenancySvc,
		EnvVarService:              envvarsvc.New(db),
		ProcedureService:           procedureSvc,
		AppHostingService:          apphostingSvc,
		EndpointService:            endpointSvc,
		PipelineService:            pipelineSvc,
		AlertService:               alertSvc,
		NotificationService:        notifSvc,
		NotificationRouter:         notifRouter,
		InstallerService:           installerSvc,
		BillingService:             billingSvc,
		DrainLedger:                drainLedger,
		PlacementService:           placementSvc,
		HealthCheckRunner:          healthCheckRunner,
		FencingSvc:                 fenceSvc,
		UpgradeSvc:                 upgradeSvc,
		ClusterMembershipService:   membershipSvc,
		CleanupService:             cleanupSvc,
		ReplicaManager:             replicaMgr,
		GitDeployMgmtService:       gitDeployMgmtSvc,
		AppStoreService:            appStoreSvc,
		CatalogService:             catalogSvc,
		ForgefileSvc:               forgefileSvc,
		OnboardingService:          onboardingSvc,
		CronJobService:             cronJobSvc,
		ScheduledTaskService:       scheduledTaskSvc,
		BuildpackService:           buildpackSvc,
		ProcessService:             processSvc,
		ZeroDowntimeSvc:            zdSvc,
		ServiceDiscovery:           discoverySvc,
		CrossNodeResolver:          crossNodeResolver,
		IngressSynchronizer:        ingressSync,
		NetBirdService:             netbirdSvc,
		MTLSEnabled:                mtlsCfg.Enabled,
		MTLSCACertPath:             mtlsCfg.CACertPath,
		MTLSCertPath:               mtlsCfg.CertPath,
		MTLSKeyPath:                mtlsCfg.KeyPath,
		MTLSDevBypass:              mtlsCfg.DevBypass,
		CertService:                certSvc,
		MTLSMigrator:               mtlsMigrator,
	}

	app := http.NewServer(appCfg)
	listenErr := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 4096)
				n := runtime.Stack(buf, false)
				slogLogger.Error("http listener panic recovered", "panic", r, "stack", string(buf[:n]))
				listenErr <- fmt.Errorf("http listener panic: %v", r)
			}
		}()
		listenErr <- app.Listen(appCfg.Addr)
	}()
	slogLogger.Info("api listening", slog.String("addr", appCfg.Addr))
	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	select {
	case <-signalCtx.Done():
		shutdownServices(app, appCancel, nil, slogLogger, mailWorker, whSvc, queueSvc, opSvc, procedureSvc, gitOpsController, eventRelay, replicaMgr, discoverySvc, ingressSync, healthFilter, resMgr, hbm, rec, mig, ep, failSvc, bkWorker, healthCheckRunner, lbSvc, autoSvc, tmSvc, cleanupSvc, cronJobSvc, domainSvc, acmeSvc, backupEngineSvc)
	case err := <-listenErr:
		shutdownServices(app, appCancel, err, slogLogger, mailWorker, whSvc, queueSvc, opSvc, procedureSvc, gitOpsController, eventRelay, replicaMgr, discoverySvc, ingressSync, healthFilter, resMgr, hbm, rec, mig, ep, failSvc, bkWorker, healthCheckRunner, lbSvc, autoSvc, tmSvc, cleanupSvc, cronJobSvc, domainSvc, acmeSvc, backupEngineSvc)
	}
	return nil
}

func shutdownServices(app *fiber.App, cancelBackground context.CancelFunc, listenErr error, log *slog.Logger,
	mailWorker *mailservice.Worker,
	whSvc *webhook.Service,
	queueSvc *queue.Service,
	opSvc *operationsvc.Service,
	procedureSvc *proceduresvc.Service,
	gitOpsController *composesvc.GitOpsController,
	eventRelay *eventstore.Relay,
	replicaMgr *replicamanager.Manager,
	discoverySvc *servicediscovery.Service,
	ingressSync *crossnode.IngressSynchronizer,
	healthFilter *crossnode.HealthFilter,
	resMgr *reservations.Manager,
	hbm *heartbeatmonitor.Service,
	rec *reconciler.Service,
	mig *migration.Service,
	ep *evacuationplanner.Service,
	failSvc *failover.Service,
	bkWorker *backup.Worker,
	healthCheckRunner *healthchecksvc.Service,
	lbSvc *loadbalancer.Service,
	autoSvc *autoscaler.Service,
	tmSvc *trafficmanager.Service,
	cleanupSvc *cleanupsvc.Service,
	cronJobSvc *cronjobsvc.Service,
	domainSvc *domains.Service,
	acmeSvc *acmesvc.Service,
	backupEngineSvc *backupenginesvc.Service,
) {
	cancelBackground()
	if err := app.Shutdown(); err != nil {
		log.Warn("api shutdown error", slog.String("error", err.Error()))
	}
	if mailWorker != nil {
		mailWorker.Wait()
	}
	if whSvc != nil {
		whSvc.Wait()
	}
	if queueSvc != nil {
		queueSvc.Stop()
	}
	if opSvc != nil {
		opSvc.Stop()
	}
	if procedureSvc != nil {
		procedureSvc.Stop()
	}
	if gitOpsController != nil {
		gitOpsController.Stop()
	}
	if eventRelay != nil {
		eventRelay.Stop()
	}
	if replicaMgr != nil {
		replicaMgr.Stop()
	}
	if discoverySvc != nil {
		discoverySvc.Stop()
	}
	if ingressSync != nil {
		ingressSync.Stop()
	}
	if healthFilter != nil {
		healthFilter.StopReaper()
	}
	if resMgr != nil {
		resMgr.Stop()
	}
	if hbm != nil {
		hbm.Stop()
	}
	if rec != nil {
		rec.Stop()
	}
	if mig != nil {
		_ = mig.Shutdown(context.Background())
	}
	if ep != nil {
		ep.Stop()
	}
	if failSvc != nil {
		failSvc.Stop()
	}
	if bkWorker != nil {
		bkWorker.Stop()
	}
	if healthCheckRunner != nil {
		healthCheckRunner.Stop()
	}
	if lbSvc != nil {
		lbSvc.Shutdown()
	}
	if autoSvc != nil {
		autoSvc.Stop()
	}
	if tmSvc != nil {
		tmSvc.Stop()
	}
	if cleanupSvc != nil {
		cleanupSvc.Stop()
	}
	if cronJobSvc != nil {
		cronJobSvc.Stop()
	}
	if domainSvc != nil {
		domainSvc.StopReverify()
	}
	if acmeSvc != nil {
		acmeSvc.StopAutoRenewal()
	}
	if backupEngineSvc != nil {
		backupEngineSvc.Stop()
	}
	if listenErr != nil {
		log.Warn("api listener stopped", slog.String("error", listenErr.Error()))
	}
}

func healthcheckPort(addr string) string {
	if addr == "" || addr[0] == ':' {
		return addr
	}
	for index := len(addr) - 1; index >= 0; index-- {
		if addr[index] == ':' {
			return addr[index:]
		}
	}
	return ":8080"
}

func healthcheck(target string) error {
	client := nethttp.Client{Timeout: 3 * time.Second}
	res, err := client.Get(target)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, res.Body)
		res.Body.Close()
	}()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("unhealthy status %d", res.StatusCode)
	}
	return nil
}

// Job/operation payload shapes for the server install, uninstall, backup and
// transfer executors wired in run(). "serverId" is optional when the queue job
// (Job.ServerID) or operation (Operation.ResourceID) already carries it; the
// backup jobs additionally carry the parameters the HTTP endpoints take in their
// request body.
// caddyGatewayCertInstaller adapts the Caddy reverse proxy to the acme
// GatewayCertInstaller contract so issued/renewed certificates reach the live
// gateway (acme must not import trafficmanager directly).
type caddyGatewayCertInstaller struct {
	proxy *trafficmanager.CaddyReverseProxy
}

func (a caddyGatewayCertInstaller) InstallCertificate(ctx context.Context, certPEM, keyPEM string, domains []string) error {
	return a.proxy.SetCertificate(ctx, trafficmanager.CertConfig{Certificate: certPEM, PrivateKey: keyPEM, Domains: domains})
}

type serverInstallPayload struct {
	ServerID  string `json:"serverId,omitempty"`
	Reinstall bool   `json:"reinstall,omitempty"`
}

type serverUninstallPayload struct {
	ServerID string `json:"serverId,omitempty"`
	Force    bool   `json:"force,omitempty"`
}

type backupCreatePayload struct {
	ServerID     string   `json:"serverId,omitempty"`
	Name         string   `json:"name,omitempty"`
	IgnoredFiles []string `json:"ignored,omitempty"`
	ActorID      string   `json:"actorId,omitempty"`
}

type backupRestorePayload struct {
	ServerID string `json:"serverId,omitempty"`
	Name     string `json:"name"`
	Truncate bool   `json:"truncate,omitempty"`
	ActorID  string `json:"actorId,omitempty"`
}

type serverTransferPayload struct {
	ServerID     string `json:"serverId,omitempty"`
	SourceNodeID string `json:"sourceNodeId,omitempty"`
	TargetNodeID string `json:"targetNodeId,omitempty"`
}

// decodeJobPayload accepts an optional JSON payload. A missing payload is not an
// error (the job or operation may carry every parameter it needs), but a
// malformed one is reported instead of being silently ignored.
func decodeJobPayload(raw json.RawMessage, dst any) error {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("invalid job payload: %w", err)
	}
	return nil
}

// backupNameCandidates mirrors the HTTP handlers: backups are stored on the node
// as "<name>.zip" while legacy and pending records use the bare name, so both
// forms are tried in order of exactness.
func backupNameCandidates(identifier string) []string {
	if strings.HasSuffix(identifier, ".zip") {
		return []string{identifier, strings.TrimSuffix(identifier, ".zip")}
	}
	return []string{identifier, identifier + ".zip"}
}

// resolveJobBackupName resolves a payload-supplied backup identifier to the
// stored row, which is the source of truth forwarded to the node. It is the
// package main copy of the (unexported) HTTP resolveBackupName helper so async
// jobs restore exactly what the POST /servers/:id/backups/restore endpoint would.
func resolveJobBackupName(ctx context.Context, st *store.Store, serverID, identifier string) (store.Backup, error) {
	var lastErr error
	for _, name := range backupNameCandidates(strings.TrimSpace(identifier)) {
		if name == "" {
			continue
		}
		backup, err := st.GetBackupByName(ctx, serverID, name)
		if err == nil {
			return backup, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no backup name supplied")
	}
	return store.Backup{}, lastErr
}

func env(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
		log.Printf("WARNING: invalid boolean value for %s=%q, using fallback %t", key, val, fallback)
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
		log.Printf("WARNING: invalid integer value for %s=%q, using fallback %d", key, val, fallback)
	}
	return fallback
}

func masterKeyringFromEnvironment(production bool) (*secrets.Keyring, bool, error) {
	activeKey := strings.TrimSpace(os.Getenv("FORGE_MASTER_KEY"))
	if production && strings.Contains(activeKey, "CHANGE_ME") {
		return nil, false, errors.New("FORGE_MASTER_KEY must not contain a deployment placeholder")
	}
	ephemeral := false
	if activeKey == "" {
		allowEphemeral, err := strconv.ParseBool(env("FORGE_ALLOW_EPHEMERAL_MASTER_KEY", "false"))
		if err != nil {
			return nil, false, errors.New("FORGE_ALLOW_EPHEMERAL_MASTER_KEY must be a boolean")
		}
		if production || !allowEphemeral {
			return nil, false, errors.New("FORGE_MASTER_KEY is required before database startup")
		}
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return nil, false, errors.New("generate ephemeral master key")
		}
		activeKey = base64.StdEncoding.EncodeToString(raw)
		ephemeral = true
	}
	previous, err := parsePreviousMasterKeys(os.Getenv("FORGE_PREVIOUS_MASTER_KEYS"))
	if err != nil {
		return nil, false, err
	}
	keyring, err := secrets.New(env("FORGE_MASTER_KEY_ID", "primary"), activeKey, previous)
	if err != nil {
		return nil, false, err
	}
	return keyring, ephemeral, nil
}

func parsePreviousMasterKeys(raw string) (map[string]string, error) {
	keys := map[string]string{}
	for _, entry := range strings.Split(strings.TrimSpace(raw), ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, errors.New("FORGE_PREVIOUS_MASTER_KEYS must contain comma-separated key-id=encoded-key entries")
		}
		id := strings.TrimSpace(parts[0])
		if _, exists := keys[id]; exists {
			return nil, errors.New("FORGE_PREVIOUS_MASTER_KEYS contains a duplicate key ID")
		}
		keys[id] = strings.TrimSpace(parts[1])
	}
	return keys, nil
}

func validateConfig(cfg *config.Config, log *slog.Logger) error {
	if errs := cfg.ValidateAll(); len(errs) > 0 {
		for _, e := range errs {
			log.Error("config validation error", slog.String("field", e.Field), slog.String("message", e.Message))
		}
		return errors.New("invalid configuration; see errors above")
	}
	return nil
}

func randomEncodedSecret(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func demoSeedEnabled(appEnv, raw string) (bool, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return false, nil
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("API_SEED_DEMO must be a boolean: %w", err)
	}
	if !enabled {
		return false, nil
	}
	allowedEnvs := map[string]bool{"development": true, "local": true, "test": true}
	if !allowedEnvs[strings.ToLower(strings.TrimSpace(appEnv))] {
		return false, fmt.Errorf("API_SEED_DEMO is only allowed in development/local/test environments, got %q", appEnv)
	}
	return true, nil
}

func newInMemoryWebAuthnSessionStore() *inMemoryWebAuthnSessionStore {
	return &inMemoryWebAuthnSessionStore{data: make(map[string]inMemoryWebAuthnSessionEntry)}
}

type inMemoryWebAuthnSessionEntry struct {
	data   []byte
	expiry time.Time
}

type inMemoryWebAuthnSessionStore struct {
	mu   sync.Mutex
	data map[string]inMemoryWebAuthnSessionEntry
}

func (s *inMemoryWebAuthnSessionStore) Save(_ context.Context, key string, data []byte, expiry time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = inMemoryWebAuthnSessionEntry{data: data, expiry: time.Now().Add(expiry)}
	return nil
}

func (s *inMemoryWebAuthnSessionStore) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.data[key]
	if !ok || time.Now().After(entry.expiry) {
		if ok {
			delete(s.data, key)
		}
		return nil, fmt.Errorf("session not found")
	}
	return entry.data, nil
}

func (s *inMemoryWebAuthnSessionStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

// processDaemonAdapter adapts *daemon.Client to process.DaemonClient.
type processDaemonAdapter struct {
	store  *store.Store
	daemon *daemon.Client
}

func (a *processDaemonAdapter) StartContainer(ctx context.Context, serverID, processType string) error {
	target, err := a.store.ServerControlTarget(ctx, serverID)
	if err != nil {
		return err
	}
	_, err = a.daemon.SendPower(ctx, target.NodeURL, target.NodeToken, serverID, "start")
	return err
}

func (a *processDaemonAdapter) StopContainer(ctx context.Context, serverID, processType string) error {
	target, err := a.store.ServerControlTarget(ctx, serverID)
	if err != nil {
		return err
	}
	_, err = a.daemon.SendPower(ctx, target.NodeURL, target.NodeToken, serverID, "stop")
	return err
}

func (a *processDaemonAdapter) RunContainer(ctx context.Context, serverID, command string) (string, error) {
	target, err := a.store.ServerControlTarget(ctx, serverID)
	if err != nil {
		return "", err
	}
	return a.daemon.SendCommandWithOutput(ctx, target.NodeURL, target.NodeToken, serverID, command)
}

// predictiveStore adapts *store.Store to scheduler.predictiveStore.
type predictiveStore struct{ *store.Store }

func (s predictiveStore) ListServersByNode(ctx context.Context, nodeID string) ([]store.Server, error) {
	return s.ListServersForNode(ctx, nodeID)
}

// resolutionStoreAdapter adapts *store.Store to crossnode.ResolutionStore.
type resolutionStoreAdapter struct {
	*store.Store
}

func (a resolutionStoreAdapter) GetServerNodeID(ctx context.Context, id string) (string, error) {
	server, err := a.Store.GetServer(ctx, id)
	if err != nil {
		return "", err
	}
	return server.NodeID, nil
}

func (a resolutionStoreAdapter) GetNodeHost(ctx context.Context, id string) (string, string, error) {
	node, err := a.Store.GetNode(ctx, id)
	if err != nil {
		return "", "", err
	}
	return node.PublicHostname, node.FQDN, nil
}

// domainNodeResolver adapts *store.Store to domains.NodeResolver.
type domainNodeResolver struct {
	store *store.Store
}

func (r *domainNodeResolver) ResolveServerTarget(ctx context.Context, serverID string) (string, int, error) {
	nodeID, err := r.store.ServerNodeID(ctx, serverID)
	if err != nil {
		return "", 0, err
	}
	node, err := r.store.GetNode(ctx, nodeID)
	if err != nil {
		return "", 0, err
	}
	host := strings.TrimSpace(node.PublicHostname)
	if host == "" {
		host = strings.TrimSpace(node.FQDN)
	}
	port := node.DaemonListen
	if port <= 0 {
		port = 8080
	}
	return host, port, nil
}

// composeStackDeployer adapts the compose lifecycle service to app-hosting's
// StackDeployer contract: it performs the real release of a stack onto a node and
// reports the state that node observed. A release that did not happen returns an
// error, never a zero-value DeployedStack with a nil error.
type composeStackDeployer struct {
	lifecycle *composesvc.Service
}

func (d composeStackDeployer) DeployStack(ctx context.Context, req apphostingsvc.StackDeployRequest) (apphostingsvc.DeployedStack, error) {
	if d.lifecycle == nil {
		return apphostingsvc.DeployedStack{}, errors.New("compose lifecycle service is not configured")
	}
	stack, err := d.lifecycle.DeployComposeStack(ctx, composesvc.DeployComposeRequest{
		UserID:        req.UserID,
		Name:          req.Name,
		NodeID:        req.NodeID,
		ComposeYAML:   req.ComposeYAML,
		EnvVars:       req.EnvVars,
		MemoryMB:      req.MemoryMB,
		CPUShares:     req.CPUShares,
		DiskMB:        req.DiskMB,
		EnvironmentID: req.EnvironmentID,
	})
	if err != nil {
		return apphostingsvc.DeployedStack{}, err
	}
	if stack == nil {
		return apphostingsvc.DeployedStack{}, errors.New("compose deploy reported success without a stack")
	}
	return apphostingsvc.DeployedStack{ID: stack.ID, Status: string(stack.Status), Error: stack.Error}, nil
}

func (d composeStackDeployer) UpdateStack(ctx context.Context, stackID string, req apphostingsvc.StackUpdateRequest) (apphostingsvc.DeployedStack, error) {
	if d.lifecycle == nil {
		return apphostingsvc.DeployedStack{}, errors.New("compose lifecycle service is not configured")
	}
	stack, err := d.lifecycle.UpdateComposeStack(ctx, stackID, composesvc.UpdateComposeRequest{
		ComposeYAML: req.ComposeYAML,
		EnvVars:     req.EnvVars,
		MemoryMB:    req.MemoryMB,
		CPUShares:   req.CPUShares,
		DiskMB:      req.DiskMB,
	})
	if err != nil {
		return apphostingsvc.DeployedStack{}, err
	}
	if stack == nil {
		return apphostingsvc.DeployedStack{}, errors.New("compose update reported success without a stack")
	}
	return apphostingsvc.DeployedStack{ID: stack.ID, Status: string(stack.Status), Error: stack.Error}, nil
}
