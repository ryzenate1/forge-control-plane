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
	"syscall"
	"time"

	"gamepanel/forge/internal/bootstrap"
	"gamepanel/forge/internal/cloud"
	"gamepanel/forge/internal/config"
	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/eventstore"
	"gamepanel/forge/internal/http"
	"gamepanel/forge/internal/placement"
	gpruntime "gamepanel/forge/internal/runtime"
	"gamepanel/forge/internal/secrets"
	"gamepanel/forge/internal/services/activity"
	auditlogsvc "gamepanel/forge/internal/services/auditlog"
	"gamepanel/forge/internal/services/autoscaler"
	"gamepanel/forge/internal/services/backup"
	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/services/configvalidator"
	"gamepanel/forge/internal/services/crashdetector"
	"gamepanel/forge/internal/services/dbprovisioner"
	"gamepanel/forge/internal/services/deployment"
	"gamepanel/forge/internal/services/evacuationplanner"
	"gamepanel/forge/internal/services/failover"
	"gamepanel/forge/internal/services/health"
	"gamepanel/forge/internal/services/heartbeatmonitor"
	"gamepanel/forge/internal/services/i18n"
	"gamepanel/forge/internal/services/loadbalancer"
	"gamepanel/forge/internal/services/logger"
	mailservice "gamepanel/forge/internal/services/mail"
	"gamepanel/forge/internal/services/migration"
	"gamepanel/forge/internal/services/nodeprobe"
	"gamepanel/forge/internal/services/noderegistry"
	"gamepanel/forge/internal/services/observability"
	"gamepanel/forge/internal/services/plugins"
	"gamepanel/forge/internal/services/queue"
	"gamepanel/forge/internal/services/reconciler"
	recoverysvc "gamepanel/forge/internal/services/recovery"
	"gamepanel/forge/internal/services/reservations"
	runtimesvc "gamepanel/forge/internal/services/runtime"
	"gamepanel/forge/internal/services/scheduler"
	"gamepanel/forge/internal/services/trafficmanager"
	"gamepanel/forge/internal/services/webauthn"
	"gamepanel/forge/internal/services/webhook"
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
	moduleRegistry, err := bootstrap.ModuleRegistry()
	if err != nil {
		log.Fatalf("register platform modules: %v", err)
	}
	if err := moduleRegistry.Start(ctx); err != nil {
		log.Fatalf("start platform modules: %v", err)
	}
	registeredModules := moduleRegistry.Modules()
	moduleNames := make([]string, 0, len(registeredModules))
	for _, module := range registeredModules {
		moduleNames = append(moduleNames, module.Name())
	}
	slogLogger.Info("platform modules registered", slog.Any("modules", moduleNames))

	var db *store.Store
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		keyring, ephemeral, err := masterKeyringFromEnvironment(production)
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
		nr               *noderegistry.Service
		np               *nodeprobe.Service
		cm               *clustermanager.Service
		ep               *evacuationplanner.Service
		mig              *migration.Service
		resMgr           *reservations.Manager
		rcv              *recoverysvc.Coordinator
		rts              *recoverysvc.TokenService
		hbm              *heartbeatmonitor.Service
		obs              *observability.Service
		rec              *reconciler.Service
		dbProv           *dbprovisioner.Service
		whSvc            *webhook.Service
		mailWorker       *mailservice.Worker
		mailTriggerSvc   *mailservice.TriggerService
		actSvc           *activity.Service
		auditLogSvc      auditlogsvc.AuditLogger
		pluginSvc        *plugins.Service
		queueSvc         *queue.Service
		runtimeRegistry  *runtimesvc.Registry
		waSvc            *webauthn.Service
		autoSvc          *autoscaler.Service
		bkSvc            *backup.Service
		deploySvc        *deployment.Service
		cloudMgr         *cloud.Manager
		lbSvc            *loadbalancer.Service
		failSvc          *failover.Service
		crashDetector    *crashdetector.Detector
		tmSvc            *trafficmanager.Service
		predictiveScorer *scheduler.PredictiveScorer
		constraintSched  *scheduler.ConstraintScheduler
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

		sched := scheduler.New(db, placeEngine, outboxPub)
		resMgr = reservations.New(db, outboxPub)
		dockerRT := gpruntime.NewDockerAdapter(daemonClient)
		cm = clustermanager.New(db, dockerRT, sched, resMgr, outboxPub)
		hbm = heartbeatmonitor.New(db, outboxPub)
		rec = reconciler.New(db, cm, 0, outboxPub)
		ep = evacuationplanner.New(db, sched, outboxPub)
		mig = migration.New(db, sched, ep, resMgr, dockerRT, outboxPub)
		ep.SetMigrationExecutor(mig)
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
		queueSvc.Start(appCtx)

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
		lbSvc = loadbalancer.New(db, outboxPub)
		lbSvc.Start(appCtx)
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
		eventRegistry.Subscribe(events.EventNodeSuspected, failSvc)
		eventRegistry.Subscribe(events.EventNodeUnreachable, failSvc)
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
		tmSvc = trafficmanager.New(db, nil, outboxPub)
		predictiveScorer = scheduler.NewPredictiveScorer(predictiveStore{db})
		constraintSched = scheduler.NewConstraintScheduler(db)

		// Wire observability as a catch-all event subscriber so every domain
		// event is persisted to the timeline.
		eventRegistry.Subscribe(events.WildcardEventType, obs)
		eventRegistry.Subscribe(events.WildcardEventType, whSvc)

		// Start background services.
		resMgr.Start(appCtx)
		hbm.Start(appCtx)
		rec.Start(appCtx)
		mig.Start(appCtx)
		ep.Start(appCtx)
		mailWorker.Start(appCtx)
		whSvc.Start(appCtx)
		_ = failSvc.Start(appCtx)
		eventRelay.Start(appCtx)
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
		Logger:               slogLogger,
		Addr:                 env("API_ADDR", ":8080"),
		ReadTimeout:          5 * time.Second,
		AuthSecret:           authSecret,
		Store:                db,
		Redis:                redisClient,
		RedisEnabled:         redisEnabled,
		Daemon:               daemonClient,
		BackgroundContext:    appCtx,
		PanelURL:             env("PANEL_URL", "http://localhost:3000"),
		PluginsDir:           env("PLUGINS_DIR", ""),
		PluginService:        pluginSvc,
		NodeRegistry:         nr,
		NodeProbe:            np,
		ClusterManager:       cm,
		EvacuationPlanner:    ep,
		MigrationService:     mig,
		ReservationManager:   resMgr,
		RecoveryCoordinator:  rcv,
		RecoveryTokenService: rts,
		HeartbeatMonitor:     hbm,
		Observability:        obs,
		Reconciler:           rec,
		DBProvisioner:        dbProv,
		HealthService:        healthSvc,
		MailTriggerService:   mailTriggerSvc,
		QueueService:         queueSvc,
		RuntimeRegistry:      runtimeRegistry,
		WebAuthnService:      waSvc,
		ActivityService:      actSvc,
		AuditLogService:      auditLogSvc,
		EventRelay:           eventRelay,
		AutoScaler:           autoSvc,
		CrashDetector:        crashDetector,
		DeploymentSvc:        deploySvc,
		CloudManager:         cloudMgr,
		LoadBalancer:         lbSvc,
		FailoverSvc:          failSvc,
		TrafficManager:       tmSvc,
		PredictiveScorer:     predictiveScorer,
		ConstraintScheduler:  constraintSched,
		BackupSvc:            bkSvc,
		Translator:           translator,
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
		if eventRelay != nil {
			eventRelay.Stop()
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
	if enabled && strings.EqualFold(strings.TrimSpace(appEnv), "production") {
		return false, fmt.Errorf("API_SEED_DEMO cannot be enabled in production")
	}
	return enabled, nil
}

// predictiveStore adapts *store.Store to scheduler.predictiveStore.
type predictiveStore struct{ *store.Store }

func (s predictiveStore) ListServersByNode(ctx context.Context, nodeID string) ([]store.Server, error) {
	return s.ListServersForNode(ctx, nodeID)
}
