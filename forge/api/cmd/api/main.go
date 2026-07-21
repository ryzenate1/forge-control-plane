package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"log/slog"
	nethttp "net/http"
	"os"
	"os/signal"
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

	"gamepanel/forge/internal/services"
	acmesvc "gamepanel/forge/internal/services/acme"
	"gamepanel/forge/internal/services/activity"
	alerting "gamepanel/forge/internal/services/alerting"
	apphostingsvc "gamepanel/forge/internal/services/apphosting"
	appstoresvc "gamepanel/forge/internal/services/appstore"
	auditlogsvc "gamepanel/forge/internal/services/auditlog"
	"gamepanel/forge/internal/services/autoscaler"
	"gamepanel/forge/internal/services/backup"
	buildsvc "gamepanel/forge/internal/services/build"
	buildpacksvc "gamepanel/forge/internal/services/buildpack"
	cleanupsvc "gamepanel/forge/internal/services/cleanup"
	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/services/clustermembership"
	composesvc "gamepanel/forge/internal/services/compose"
	"gamepanel/forge/internal/services/configvalidator"
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
	fencing "gamepanel/forge/internal/services/fencing"
	gitsvc "gamepanel/forge/internal/services/git"
	gitprovidersvc "gamepanel/forge/internal/services/gitprovider"
	"gamepanel/forge/internal/services/health"
	healthchecksvc "gamepanel/forge/internal/services/healthcheckrunner"
	"gamepanel/forge/internal/services/heartbeatmonitor"
	"gamepanel/forge/internal/services/i18n"
	"gamepanel/forge/internal/services/loadbalancer"
	"gamepanel/forge/internal/services/logger"
	mailservice "gamepanel/forge/internal/services/mail"
	"gamepanel/forge/internal/services/migration"
	"gamepanel/forge/internal/services/nodeprobe"
	"gamepanel/forge/internal/services/noderegistry"
	notification "gamepanel/forge/internal/services/notification"
	"gamepanel/forge/internal/services/observability"
	operationsvc "gamepanel/forge/internal/services/operation"
	"gamepanel/forge/internal/services/plugins"
	previewsvc "gamepanel/forge/internal/services/preview"
	proceduresvc "gamepanel/forge/internal/services/procedure"
	processsvc "gamepanel/forge/internal/services/process"
	"gamepanel/forge/internal/services/queue"
	"gamepanel/forge/internal/services/reconciler"
	recoverysvc "gamepanel/forge/internal/services/recovery"
	"gamepanel/forge/internal/services/replicamanager"
	"gamepanel/forge/internal/services/reservations"
	runtimesvc "gamepanel/forge/internal/services/runtime"
	"gamepanel/forge/internal/services/scheduler"
	"gamepanel/forge/internal/services/servicediscovery"
	"gamepanel/forge/internal/services/tenancy"
	"gamepanel/forge/internal/services/trafficmanager"
	"gamepanel/forge/internal/services/webauthn"
	"gamepanel/forge/internal/services/webhook"
	"gamepanel/forge/internal/services/zerodowntime"
	"gamepanel/forge/internal/store"
	"gamepanel/forge/internal/version"

	"github.com/redis/go-redis/v9"
)

const readinessHealthPath = "/api/v1/health/ready"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--healthcheck" {
		healthcheck("http://127.0.0.1" + healthcheckPort(env("API_ADDR", ":8080")) + readinessHealthPath)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	appEnv := env("APP_ENV", "development")
	production := strings.EqualFold(strings.TrimSpace(appEnv), "production")
	seedDemo, err := demoSeedEnabled(appEnv, os.Getenv("API_SEED_DEMO"))
	if err != nil {
		log.Fatal(err)
	}
	authSecret := env("API_AUTH_SECRET", "dev-api-secret")
	if production && (authSecret == "" || authSecret == "dev-api-secret") {
		log.Fatal("API_AUTH_SECRET must be set to a production secret")
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
		masterKeyring = kr
		keyring := kr
		if err != nil {
			log.Fatal(err)
		}
		if ephemeral {
			slogLogger.Warn("FORGE_ALLOW_EPHEMERAL_MASTER_KEY is enabled; encrypted data will be unrecoverable after this process exits")
		}
		connected, err := store.ConnectWithKeyring(ctx, databaseURL, keyring)
		if err != nil {
			log.Fatal(err)
		}
		defer connected.Close()
		if err := connected.RunMigrations(ctx, env("MIGRATIONS_DIR", "migrations")); err != nil {
			log.Fatal(err)
		}
		if err := connected.RunSelectedMigrations(ctx, env("BATCH2_MIGRATIONS_DIR", "internal/store/migrations"), []string{
			"024_a_sftp_config.sql",
			"025_a_install_workflows.sql",
			"026_a_external_ids.sql",
			"033_node_onboarding.sql",
			"034_build_pipeline.sql",
			"035_compose_gitops.sql",
			"035_a_infra_endpoints.sql",
			"035_b_observability_monitoring.sql",
			"036_compose_concurrency.sql",
			"037_build_extended_fields.sql",
			"038_traffic_routing.sql",
			"040_git_deployment.sql",
			"041_a_placement_intents.sql",
			"041_buildpack_support.sql",
			"042_service_discovery_endpoints.sql",
			"043_webhook_idempotency.sql",
			"114_e_zero_downtime_deploy.sql",
		}); err != nil {
			log.Fatal(err)
		}
		if err := eventstore.Migrate(connected.GetDB()); err != nil {
			log.Fatal(err)
		}
		if err := connected.MigrateOperationalSecrets(ctx); err != nil {
			log.Fatal(err)
		}
		if len(os.Args) > 1 && os.Args[1] == "rotate-master-key" {
			slogLogger.Info("secret rotation completed", slog.String("active_key", keyring.ActiveKeyID()))
			return
		}
		if len(os.Args) > 1 && os.Args[1] == "restore-plaintext-secrets" {
			if err := connected.RestoreOperationalSecrets(ctx); err != nil {
				log.Fatal(err)
			}
			slogLogger.Info("legacy plaintext secret columns restored; ciphertext retained")
			return
		}
		if seedDemo {
			if err := connected.Seed(ctx); err != nil {
				log.Fatal(err)
			}
		}
		db = connected
	}

	var redisClient *redis.Client
	redisEnabled := false
	if redisAddr := os.Getenv("REDIS_ADDR"); redisAddr != "" {
		redisEnabled = true
		redisClient = redis.NewClient(&redis.Options{Addr: redisAddr})
		defer redisClient.Close()
		if err := redisClient.Ping(ctx).Err(); err != nil {
			slogLogger.Warn("redis ping failed at startup", slog.String("error", err.Error()))
		}
	}

	daemonClient := daemon.NewClient()
	if !production {
		// Backward-compatible Phase 0 fallback for development only. Normal
		// outbound requests always pass the current target node credential.
		daemonClient = daemon.NewClientWithDevelopmentFallback(env("DAEMON_NODE_TOKEN", "dev-node-token"))
	}

	// Build the service graph. All services are nil-safe when db == nil;
	// handler nil-guards already handle the "no database" dev-mode case.
	var (
		nr                *noderegistry.Service
		np                *nodeprobe.Service
		cm                *clustermanager.Service
		ep                *evacuationplanner.Service
		mig               *migration.Service
		resMgr            *reservations.Manager
		rcv               *recoverysvc.Coordinator
		rts               *recoverysvc.TokenService
		hbm               *heartbeatmonitor.Service
		obs               *observability.Service
		rec               *reconciler.Service
		dbProv            *dbprovisioner.Service
		whSvc             *webhook.Service
		mailWorker        *mailservice.Worker
		mailTriggerSvc    *mailservice.TriggerService
		actSvc            *activity.Service
		auditLogSvc       auditlogsvc.AuditLogger
		pluginSvc         *plugins.Service
		queueSvc          *queue.Service
		opSvc             *operationsvc.Service
		runtimeRegistry   *runtimesvc.Registry
		waSvc             *webauthn.Service
		autoSvc           *autoscaler.Service
		bkSvc             *backup.Service
		bkWorker          *backup.Worker
		dnsSvc            *dnssvc.Service
		acmeSvc           *acmesvc.Service
		domainSvc         *domains.Service
		buildSvc          *buildsvc.Service
		deploySvc         *deployment.Service
		previewDeploySvc  *previewsvc.Service
		cloudMgr          *cloud.Manager
		lbSvc             *loadbalancer.Service
		failSvc           *failover.Service
		crashDetector     *crashdetector.Detector
		tmSvc             *trafficmanager.Service
		predictiveScorer  *scheduler.PredictiveScorer
		constraintSched   *scheduler.ConstraintScheduler
		healthCheckRunner *healthchecksvc.Service
		tenancySvc        *tenancy.Service
		dbContainerSvc    *dbprovisioner.DBContainerService
		composeLifecycle  *composesvc.Service
		procedureSvc      *proceduresvc.Service
		apphostingSvc     *apphostingsvc.Service
		endpointSvc       *environments.Service
		alertSvc          *alerting.Service
		notifSvc          *notification.Service
		fenceSvc          *fencing.Service
		membershipSvc     *clustermembership.Service
		cleanupSvc        *cleanupsvc.Service
		gitSvc            *gitsvc.Service
		gitDeploySvc      *gitsvc.DeployService
		gitProviderSvc    *gitprovidersvc.Service
		gitOpsController  *composesvc.GitOpsController
		sessionStore      *auth.PostgresSessionStore
		replicaMgr        *replicamanager.Manager
		discoverySvc      *servicediscovery.Service
		crossNodeResolver *crossnode.Resolver
		ingressSync       *crossnode.IngressSynchronizer
		healthFilter      *crossnode.HealthFilter
		appStoreSvc       *appstoresvc.Service
		cronJobSvc        *cronjobsvc.Service
		gitDeployMgmtSvc  *gitsvc.DeploymentManagementService
		zdSvc             *zerodowntime.Service
		dbSvcProv         *services.DatabaseServiceProvisioner
		dbBackupSvc       *dbbackupsvc.Service
		buildpackSvc      *buildpacksvc.Service
		processSvc        *processsvc.Service
		certSvc           *services.CertService
		mtlsMigrator      *services.MTLSMigrator
		mtlsCfg           forgecfg.MTLS
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
		eventRegistry := events.NewRegistry("forge-api")

		es := eventstore.New(db.GetDB())
		eventRelay = eventstore.NewRelay(es, 5*time.Second)
		outboxPub := eventstore.NewOutboxPublisher(es, eventRegistry)

		placeEngine = placement.NewEngine(placement.NewScorer(placement.StrategyLeastLoaded), placement.NewConstraintChecker())

		predictiveScorer = scheduler.NewPredictiveScorer(predictiveStore{db})
		constraintSched = scheduler.NewConstraintScheduler(db)

		resMgr = reservations.New(db, outboxPub)
		sched := scheduler.New(db, placeEngine, outboxPub).
			WithPredictiveScorer(predictiveScorer).
			WithConstraintScheduler(constraintSched).
			WithReservations(resMgr)
		dockerRT := gpruntime.NewDockerAdapter(daemonClient)
		cm = clustermanager.New(db, dockerRT, sched, resMgr, outboxPub)

		// Initialize Beacon HTTP client for replicamanager
		beaconBaseURL := env("BEACON_BASE_URL", "http://127.0.0.1:9090")
		beaconHTTPClient := replicamanager.NewBeaconHTTPClient(db, daemonClient, beaconBaseURL, slogLogger)

		// Initialize replicamanager with all required dependencies
		replicaMgr = replicamanager.New(db, placeEngine, sched, resMgr, nil, beaconHTTPClient, slogLogger, outboxPub)
		hbm = heartbeatmonitor.New(db, outboxPub)
		rec = reconciler.New(db, cm, 0, outboxPub)
		ep = evacuationplanner.New(db, sched, outboxPub)
		mig = migration.New(db, sched, ep, resMgr, dockerRT, outboxPub)
		ep.SetMigrationExecutor(mig)
		ep.SetServerMountStore(db)
		fenceSvc = fencing.New(db, outboxPub)
		eventRegistry.Subscribe(events.EventNodeRecovered, fenceSvc)
		rcv = recoverysvc.NewWithMigrationExecutor(db, sched, resMgr, mig, outboxPub)
		recTokenStore := recoverysvc.NewStore(db.GetDB())
		rts = recoverysvc.NewTokenService(recTokenStore)
		obs = observability.New(db)
		obs.StartMetricsCollection(appCtx, 30*time.Second)
		nr = noderegistry.New(db)
		np = nodeprobe.NewService(db)
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
						db.DispatchWebhookEvent(event, map[string]any{"subject_type": "server", "subject_id": job.ServerID, "signal": signal, "operation_id": job.ID})
					}
				}
				return err
			})
		}
		registerPowerJob(queue.JobServerStart, "start")
		registerPowerJob(queue.JobServerStop, "stop")
		registerPowerJob(queue.JobServerRestart, "restart")
		registerPowerJob(queue.JobServerKill, "kill")
		composeLifecycle, err = composesvc.New(db, outboxPub)
		if err != nil {
			log.Fatalf("failed to create compose service: %v", err)
		}
		composeLifecycle.WithReservationManager(resMgr).WithScheduler(sched)
		composeQH, err := composesvc.NewQueueHandler(composeLifecycle)
		if err != nil {
			log.Fatalf("failed to create compose queue handler: %v", err)
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

		gitSvc = gitsvc.NewService(db, slogLogger)
		gitDeploySvc = gitsvc.NewDeployService(gitSvc, db, slogLogger, "", "")
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
			log.Fatalf("failed to create gitops controller: %v", err)
		}
		gitOpsController.Start(appCtx)

		appStoreSvc, err = appstoresvc.New(db, composeLifecycle)
		if err != nil {
			log.Fatalf("failed to create app store service: %v", err)
		}

		queueSvc.Start(appCtx)

		opStore := operationsvc.NewPostgresStore(db.GetDB())
		opSvc = operationsvc.New(opStore)
		registerPowerOp := func(opType operationsvc.OperationType, signal string) {
			opSvc.RegisterHandler(opType, func(ctx context.Context, op *operationsvc.Operation) error {
				commandCtx := daemon.ContextWithCommandID(ctx, op.ID)
				_, _, err := cm.RequestServerPower(commandCtx, op.ResourceID, signal)
				if err == nil {
					event := map[string]string{"start": "server:started", "stop": "server:stopped", "restart": "server:restarted", "kill": "server:stopped"}[signal]
					if event != "" {
						db.DispatchWebhookEvent(event, map[string]any{"subject_type": "server", "subject_id": op.ResourceID, "signal": signal, "operation_id": op.ID})
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
		opSvc.Start(appCtx)

		runtimeRegistry = runtimesvc.NewRegistry()

		waSvc, err = webauthn.New(
			env("WEBAUTHN_RP_ID", "localhost"),
			env("WEBAUTHN_RP_DISPLAY_NAME", "GamePanel"),
			env("WEBAUTHN_RP_ORIGIN", "http://localhost:3000"),
			webauthn.NewPostgresCredentialStore(db.GetDB()),
			webauthn.NewRedisSessionStore(redisClient),
		)
		if err != nil {
			log.Fatalf("failed to create webauthn service: %v", err)
		}

		autoSvc = autoscaler.New(db, cm, dockerRT, outboxPub)
		deploySvc = deployment.New(db, outboxPub)
		previewDeploySvc = previewsvc.New(db, outboxPub)
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
					db.UpdateDeploymentStatus(ctx, d.ID, string(deployment.StatusRollbackPending),
						fmt.Sprintf("auto-rollback triggered by runtime health degradation (target %s, %d failures)", targetID, consecutiveFailures))
					_, rollbackErr := deploySvc.RollbackToPrevious(ctx, d.ID)
					if rollbackErr != nil {
						slogLogger.Error("health check bridge: auto-rollback failed", slog.String("deploymentId", d.ID), slog.String("serverId", serverID), slog.String("error", rollbackErr.Error()))
						continue
					}
					db.UpdateDeploymentStatus(ctx, d.ID, string(deployment.StatusRollingBack), "")
					db.UpdateDeploymentStatus(ctx, d.ID, string(deployment.StatusRolledBack),
						fmt.Sprintf("auto-rollback due to runtime health degradation (target %s, %d failures)", targetID, consecutiveFailures))
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
		dnsSvc, err = dnssvc.New(db)
		if err != nil {
			log.Fatalf("failed to create dns service: %v", err)
		}
		caddyProxy := trafficmanager.NewCaddyReverseProxy(env("CADDY_ADMIN_ADDR", "127.0.0.1:2019"))
		acmeSvc = acmesvc.New(db, slogLogger)
		dnsSvc.RegisterWithAcme(func(name string, factory func(providerName string, credentials map[string]string) (challenge.Provider, error)) {
			acmeSvc.RegisterDNSProvider(name, factory)
		})
		discoverySvc = servicediscovery.New(db, servicediscovery.NewEndpointStore(db.GetDB()), outboxPub)
		crossNodeResolver = crossnode.NewResolver(resolutionStoreAdapter{db})
		crossNodeResolver.SetServiceDiscovery(discoverySvc)
		discoverySvc.Start(appCtx)

		healthFilter = crossnode.NewHealthFilter(2, 30*time.Second)
		healthFilter.StartReaper(appCtx, 5*time.Minute)
		ingressSync = crossnode.NewIngressSynchronizer(caddyProxy, crossNodeResolver, healthFilter, outboxPub)
		ingressSync.Start(appCtx, 30*time.Second)

		domainNodeResolver := &domainNodeResolver{store: db}
		domainSvc = domains.New(store.NewDomainAdapter(db), caddyProxy, env("PANEL_IP", ""), outboxPub)
		domainSvc.SetNodeResolver(domainNodeResolver)
		buildSvc = buildsvc.NewService(db, slogLogger)
		tenancySvc = tenancy.New(db)
		procedureSvc = proceduresvc.New(db, outboxPub, slogLogger, db)
		apphostingSvc = apphostingsvc.New(db, tenancySvc)
		endpointSvc = environments.New(db)
		alertSvc = alerting.New(db, alerting.DefaultThresholds, slogLogger)
		notifSvc = notification.New(db, slogLogger)
		if err := notifSvc.RefreshChannels(appCtx); err != nil {
			slogLogger.Warn("failed to refresh notification channels", slog.String("error", err.Error()))
		}
		eventRegistry.Subscribe(events.EventServerCrashed, notifSvc)
		eventRegistry.Subscribe(events.EventServerInstallCompleted, notifSvc)
		eventRegistry.Subscribe(events.EventServerBackupCreated, notifSvc)
		eventRegistry.Subscribe(events.EventServerBackupFailed, notifSvc)
		eventRegistry.Subscribe(events.EventDeploymentCompleted, notifSvc)
		eventRegistry.Subscribe(events.EventDeploymentFailed, notifSvc)
		eventRegistry.Subscribe(events.EventNodeOffline, notifSvc)
		eventRegistry.Subscribe(events.EventNodeOnline, notifSvc)
		membershipSvc = clustermembership.New(db, outboxPub)
		membershipSvc.SetEvacuationPlanner(ep)
		cleanupSvc = cleanupsvc.New(db, outboxPub)
		cleanupSvc.Start(appCtx)
		dbContainerSvc = dbprovisioner.NewDBContainerService(db, daemonClient, env("BEACON_BASE_URL", "http://127.0.0.1:9090"), env("DAEMON_NODE_TOKEN", ""), env("DOCKER_HOST", "127.0.0.1"))
		dbSvcProv = services.NewDatabaseServiceProvisioner(db, daemonClient, env("BEACON_BASE_URL", "http://127.0.0.1:9090"), env("DAEMON_NODE_TOKEN", ""), env("DOCKER_HOST", "127.0.0.1"), masterKeyring)
		dbBackupSvc = dbbackupsvc.New(db, dbbackupsvc.NewNoopStorage())
		tmSvc = trafficmanager.NewWithPersistence(db, db, db, db, caddyProxy, outboxPub)
		eventRegistry.Subscribe(events.EventNodeOffline, tmSvc)
		eventRegistry.Subscribe(events.EventNodeRecovered, tmSvc)
		tmSvc.Start(appCtx)
		eventRegistry.Subscribe(events.EventNodeOffline, lbSvc)
		eventRegistry.Subscribe(events.EventNodeRecovered, lbSvc)
		eventRegistry.Subscribe(events.EventNodeOnline, events.HandlerFunc(func(ctx context.Context, _ events.Envelope) error {
			crossNodeResolver.ClearCache()
			if ingressSync != nil {
				_ = ingressSync.Sync(ctx)
			}
			return nil
		}))
		eventRegistry.Subscribe(events.EventNodeOffline, events.HandlerFunc(func(ctx context.Context, _ events.Envelope) error {
			crossNodeResolver.ClearCache()
			if ingressSync != nil {
				_ = ingressSync.Sync(ctx)
			}
			return nil
		}))
		eventRegistry.Subscribe(events.EventNodeRecovered, events.HandlerFunc(func(ctx context.Context, _ events.Envelope) error {
			crossNodeResolver.ClearCache()
			if ingressSync != nil {
				_ = ingressSync.Sync(ctx)
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
			log.Fatalf("failed to create cron job service: %v", err)
		}

		zdSvc = zerodowntime.New(db)

		processSvc = processsvc.New(db, &processDaemonAdapter{store: db, daemon: daemonClient}, slogLogger)
		buildpackSvc = buildpacksvc.NewService(db)

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
		resMgr.Start(appCtx)
		hbm.Start(appCtx)
		rec.Start(appCtx)
		mig.Start(appCtx)
		ep.Start(appCtx)
		mailWorker.Start(appCtx)
		whSvc.Start(appCtx)
		_ = failSvc.Start(appCtx)
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
	} else {
		healthSvc.AddCheck(health.NewDatabaseCheck(nil, nil))
	}
	healthSvc.AddCheck(health.NewCacheCheck(
		func(ctx context.Context) error {
			return redisClient.Ping(ctx).Err()
		},
		func(ctx context.Context) (map[string]any, error) {
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
	healthSvc.AddCheck(health.NewDockerCheck())
	healthSvc.AddCheck(health.NewSystemCheck(started))

	cfg := config.Config{
		App: config.AppConfig{
			Env:            appEnv,
			Name:           env("APP_NAME", "GamePanel"),
			URL:            env("PANEL_URL", "http://localhost:3000"),
			Debug:          env("APP_ENV", "development") != "production",
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
			Driver:          env("DB_CONNECTION", "postgres"),
			URL:             os.Getenv("DATABASE_URL"),
			MaxOpenConns:    envInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    envInt("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: envInt("DB_CONN_MAX_LIFETIME", 3600),
		},
		Redis: config.RedisConfig{
			Addr:     env("REDIS_ADDR", ""),
			Password: env("REDIS_PASSWORD", ""),
			DB:       envInt("REDIS_DB", 0),
			Enabled:  redisEnabled,
		},
		Auth: config.AuthConfig{
			Secret:   authSecret,
			TokenTTL: 24 * time.Hour,
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

	validateConfig(&cfg, db != nil)

	appCfg := http.Config{
		Logger:                     slogLogger,
		Addr:                       env("API_ADDR", ":8080"),
		ReadTimeout:                5 * time.Second,
		AuthSecret:                 authSecret,
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
		DBProvisioner:              dbProv,
		HealthService:              healthSvc,
		SessionStore:               sessionStore,
		MailTriggerService:         mailTriggerSvc,
		QueueService:               queueSvc,
		OperationService:           opSvc,
		RuntimeRegistry:            runtimeRegistry,
		WebAuthnService:            waSvc,
		ActivityService:            actSvc,
		AuditLogService:            auditLogSvc,
		EventRelay:                 eventRelay,
		AutoScaler:                 autoSvc,
		CrashDetector:              crashDetector,
		DeploymentSvc:              deploySvc,
		PreviewDeploymentSvc:       previewDeploySvc,
		CloudManager:               cloudMgr,
		LoadBalancer:               lbSvc,
		FailoverSvc:                failSvc,
		TrafficManager:             tmSvc,
		PredictiveScorer:           predictiveScorer,
		ConstraintScheduler:        constraintSched,
		BackupSvc:                  bkSvc,
		DNSService:                 dnsSvc,
		AcmeService:                acmeSvc,
		DomainService:              domainSvc,
		BuildService:               buildSvc,
		Translator:                 translator,
		GitService:                 gitSvc,
		GitDeployService:           gitDeploySvc,
		GitProviderService:         gitProviderSvc,
		ComposeService:             composeLifecycle,
		DBContainerService:         dbContainerSvc,
		DatabaseServiceProvisioner: dbSvcProv,
		DBBackupService:            dbBackupSvc,
		TenancyService:             tenancySvc,
		EnvVarService:              envvarsvc.New(db),
		ProcedureService:           procedureSvc,
		AppHostingService:          apphostingSvc,
		EndpointService:            endpointSvc,
		AlertService:               alertSvc,
		NotificationService:        notifSvc,
		ClusterMembershipService:   membershipSvc,
		CleanupService:             cleanupSvc,
		ReplicaManager:             replicaMgr,
		GitDeployMgmtService:       gitDeployMgmtSvc,
		AppStoreService:            appStoreSvc,
		CronJobService:             cronJobSvc,
		BuildpackService:           buildpackSvc,
		ProcessService:             processSvc,
		ZeroDowntimeSvc:            zdSvc,
		ServiceDiscovery:           discoverySvc,
		CrossNodeResolver:          crossNodeResolver,
		IngressSynchronizer:        ingressSync,
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
	go func() { listenErr <- app.Listen(appCfg.Addr) }()
	slogLogger.Info("api listening", slog.String("addr", appCfg.Addr))
	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	select {
	case <-signalCtx.Done():
		appCancel()
		if err := app.Shutdown(); err != nil {
			slogLogger.Warn("api shutdown error", slog.String("error", err.Error()))
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
	case err := <-listenErr:
		appCancel()
		_ = app.Shutdown()
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
		if gitOpsController != nil {
			gitOpsController.Stop()
		}
		if procedureSvc != nil {
			procedureSvc.Stop()
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
		if err != nil {
			slogLogger.Warn("api listener stopped", slog.String("error", err.Error()))
		}
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

func healthcheck(target string) {
	client := nethttp.Client{Timeout: 3 * time.Second}
	res, err := client.Get(target)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		fmt.Fprintf(os.Stderr, "unhealthy status %d\n", res.StatusCode)
		os.Exit(1)
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}

func masterKeyringFromEnvironment(production bool) (*secrets.Keyring, bool, error) {
	activeKey := strings.TrimSpace(os.Getenv("FORGE_MASTER_KEY"))
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

func validateConfig(cfg *config.Config, _ bool) {
	if errs := configvalidator.Validate(cfg); len(errs) > 0 {
		for _, e := range errs {
			log.Printf("CONFIG ERROR: %s - %s", e.Field, e.Message)
		}
		log.Fatal("invalid configuration; see errors above")
	}
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
	err = a.daemon.SendCommand(ctx, target.NodeURL, target.NodeToken, serverID, command)
	return "", err
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
