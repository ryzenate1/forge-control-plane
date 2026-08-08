package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"gamepanel/beacon/config"
	"gamepanel/beacon/internal/backup"
	"gamepanel/beacon/internal/cron"
	"gamepanel/beacon/internal/logrotate"

	"gamepanel/beacon/internal/logging"
	"gamepanel/beacon/internal/pprof"
	"gamepanel/beacon/internal/ratelimit"
	"gamepanel/beacon/internal/remote"
	"gamepanel/beacon/internal/runtime"
	daemonhttp "gamepanel/beacon/internal/server"
	"gamepanel/beacon/internal/serverid"
	"gamepanel/beacon/internal/sftpserver"
	"gamepanel/beacon/internal/shutdown"
	"gamepanel/beacon/internal/system"
	"gamepanel/beacon/internal/tls"
	"gamepanel/beacon/internal/tokens"
)

func main() {
	if err := run(); err != nil {
		log.Printf("beacon: %v", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 {
		var command *cobra.Command
		switch os.Args[1] {
		case "configure":
			command = configureCmd
		case "diagnostics":
			command = diagnosticsCmd
		case "update":
			command = newUpdateCommand()
		}
		if command != nil {
			command.SetArgs(os.Args[2:])
			return command.Execute()
		}
	}
	logger, logErr := logging.NewZapLogger()
	if logErr != nil {
		return fmt.Errorf("initialize logger: %w", logErr)
	}
	defer func() { _ = logger.Sync() }()
	logger.Info("starting beacon daemon")

	if len(os.Args) > 1 && os.Args[1] == "--healthcheck" {
		return healthcheck("http://127.0.0.1" + healthcheckPort(env("DAEMON_ADDR", ":9090")) + "/health")
	}
	appEnv := env("APP_ENV", "development")
	addr := env("DAEMON_ADDR", ":9090")
	dataDir := env("DAEMON_DATA_DIR", "/srv/game-panel/servers")
	beaconConfig, configErr := config.LoadWithOptions(config.LoadOptions{Path: os.Getenv("DAEMON_CONFIG_FILE")})
	if configErr != nil {
		logger.Error("failed to load beacon configuration", logging.Field{Key: "error", Value: configErr})
		return fmt.Errorf("load Beacon configuration: %w", configErr)
	}

	nodeToken := os.Getenv("DAEMON_NODE_TOKEN")
	if nodeToken == "" {
		nodeToken = strings.TrimSpace(beaconConfig.Token)
	}
	if nodeToken == "" {
		nodeToken = os.Getenv("WINGS_TOKEN")
		if tokenID := os.Getenv("WINGS_TOKEN_ID"); tokenID != "" && nodeToken != "" && !strings.Contains(nodeToken, ".") {
			nodeToken = tokenID + "." + nodeToken
		}
	}
	nodeID := strings.TrimSpace(os.Getenv("DAEMON_NODE_ID"))
	if nodeID == "" {
		nodeID = strings.TrimSpace(beaconConfig.UUID)
	}
	if nodeID == "" {
		nodeID = strings.TrimSpace(os.Getenv("WINGS_NODE_ID"))
	}
	panelAPIURL := strings.TrimSpace(os.Getenv("PANEL_API_URL"))
	if panelAPIURL == "" {
		panelAPIURL = strings.TrimSpace(beaconConfig.PanelURL)
	}
	if panelAPIURL == "" {
		panelAPIURL = strings.TrimSpace(os.Getenv("WINGS_PANEL_URL"))
	}
	if err := validatePanelOnboarding(nodeID, panelAPIURL, nodeToken); err != nil {
		return err
	}
	panelOnboardingEnabled := nodeID != ""
	if panelOnboardingEnabled {
		log.Print("panel onboarding enabled; server sync and node heartbeats will start")
	} else {
		log.Print("panel onboarding is not configured; running without panel sync (set DAEMON_NODE_ID, PANEL_API_URL, and DAEMON_NODE_TOKEN to enable it)")
	}

	allowInsecureNoAuth := env("DAEMON_ALLOW_INSECURE_NO_AUTH", "false") == "true"
	if appEnv == "production" && (strings.Contains(nodeToken, "CHANGE_ME") || strings.Contains(nodeID, "CHANGE_ME") || strings.Contains(panelAPIURL, "panel.example.com")) {
		return errors.New("production Beacon configuration still contains placeholder values")
	}
	if appEnv == "production" && (nodeToken == "" || nodeToken == "dev-node-token" || nodeToken == "devnodetoken0001.dev-node-token" || allowInsecureNoAuth) {
		return errors.New("DAEMON_NODE_TOKEN must be set to a production secret and unauthenticated mode must be disabled")
	}
	if nodeToken == "" && !allowInsecureNoAuth {
		return errors.New("DAEMON_NODE_TOKEN is required; set DAEMON_ALLOW_INSECURE_NO_AUTH=true only for isolated development tests")
	}
	if allowInsecureNoAuth {
		if !isLoopbackListenAddress(addr) {
			return errors.New("DAEMON_ALLOW_INSECURE_NO_AUTH requires DAEMON_ADDR to bind to loopback")
		}
		log.Print("==================================================================")
		log.Print("WARNING: daemon API authentication is DISABLED (DAEMON_ALLOW_INSECURE_NO_AUTH=true).")
		log.Print("This mode is for isolated development and tests ONLY.")
		log.Print("The daemon API is reachable without any token; it must never be used in production.")
		log.Print("==================================================================")
	}

	// Runtime provider selection
	runtimeProvider := env("DAEMON_RUNTIME_PROVIDER", "docker")
	log.Printf("Using runtime provider: %s", runtimeProvider)

	runtimeConfig := runtime.RuntimeConfig{
		Provider: runtimeProvider,
		Kubernetes: runtime.KubernetesConfig{
			KubeconfigPath: os.Getenv("KUBECONFIG"),
			Namespace:      env("DAEMON_KUBERNETES_NAMESPACE", "forge"),
			InCluster:      env("DAEMON_KUBERNETES_IN_CLUSTER", "false") == "true",
		},
		Podman: runtime.PodmanConfig{
			URI:      strings.TrimSpace(os.Getenv("DAEMON_PODMAN_URI")),
			Identity: strings.TrimSpace(os.Getenv("DAEMON_PODMAN_IDENTITY")),
		},
		Containerd: runtime.ContainerdConfig{
			Address:   strings.TrimSpace(os.Getenv("DAEMON_CONTAINERD_ADDRESS")),
			Namespace: strings.TrimSpace(os.Getenv("DAEMON_CONTAINERD_NAMESPACE")),
		},
		Firecracker: runtime.FirecrackerConfig{
			SocketPath:     strings.TrimSpace(os.Getenv("DAEMON_FIRECRACKER_SOCKET_PATH")),
			KernelImage:    strings.TrimSpace(os.Getenv("DAEMON_FIRECRACKER_KERNEL_IMAGE")),
			RootfsImage:    strings.TrimSpace(os.Getenv("DAEMON_FIRECRACKER_ROOTFS_IMAGE")),
			CPUTemplate:    strings.TrimSpace(os.Getenv("DAEMON_FIRECRACKER_CPU_TEMPLATE")),
			JailerPath:     strings.TrimSpace(os.Getenv("DAEMON_FIRECRACKER_JAILER_PATH")),
			FirecrackerBin: strings.TrimSpace(os.Getenv("DAEMON_FIRECRACKER_BINARY")),
		},
	}
	runtimeInitCtx, cancelRuntimeInit := context.WithTimeout(context.Background(), 30*time.Second)
	rt, err := runtime.NewFactory(runtimeConfig).CreateRuntime(runtimeInitCtx)
	cancelRuntimeInit()

	if err != nil {
		if appEnv == "production" {
			return fmt.Errorf("runtime unavailable in production mode: %w", err)
		}
		if env("DAEMON_ALLOW_MOCK_RUNTIME", "false") != "true" {
			return fmt.Errorf("runtime unavailable and DAEMON_ALLOW_MOCK_RUNTIME is not true: %w", err)
		}
		log.Printf("runtime unavailable, using explicit mock mode: %v", err)
		rt = runtime.NewUnavailableRuntime(err)
	}
	if err := ensurePrivateDataDirectory(dataDir); err != nil {
		return fmt.Errorf("prepare daemon data directory: %w", err)
	}
	daemonCtx, cancelDaemon := context.WithCancel(context.Background())
	defer cancelDaemon()

	backupRoot := env("DAEMON_BACKUP_DIR", filepath.Join(filepath.Dir(dataDir), "backups"))
	backupAdapter, err := buildBackupAdapter(backupRoot, dataDir)
	if err != nil {
		return fmt.Errorf("initialize backup adapter: %w", err)
	}
	if err := backup.RecoverRestoreJournals(dataDir); err != nil {
		return fmt.Errorf("recover interrupted backup restore: %w", err)
	}

	writeLimit := beaconConfig.BackupConfig().WriteLimit
	if writeLimit > 0 {
		if bw, ok := backupAdapter.(interface{ SetWriteLimit(int64) }); ok {
			bw.SetWriteLimit(writeLimit)
			log.Printf("backup I/O write limit set to %d bytes/sec", writeLimit)
		}
	}

	logDirectory := beaconConfig.System.LogDirectory
	if logDirectory == "" {
		logDirectory = "/var/log/beacon"
	}
	if err := logrotate.WriteSystemConfig(logDirectory, "beacon"); err != nil {
		log.Printf("logrotate config generation skipped (non-fatal): %v", err)
	} else {
		log.Printf("logrotate configuration written for %s", logDirectory)
	}

	server, handler := daemonhttp.NewServerWithBackup(rt, dataDir, backupAdapter, nodeToken)
	server.SetVersion(Version)
	server.SetAllowedMounts(beaconConfig.AllowedMountsList())
	metricsToken := strings.TrimSpace(os.Getenv("METRICS_TOKEN"))
	if metricsToken == "" && strings.TrimSpace(os.Getenv("METRICS_TOKEN_FILE")) != "" {
		var err error
		metricsToken, err = readSingleLineSecret(os.Getenv("METRICS_TOKEN_FILE"))
		if err != nil {
			return fmt.Errorf("read METRICS_TOKEN_FILE: %w", err)
		}
	}
	if appEnv == "production" && len(metricsToken) < 32 {
		return errors.New("METRICS_TOKEN must contain at least 32 characters in production")
	}
	server.SetMetricsToken(metricsToken)

	if pprof.IsEnabled() {
		pprofMux := http.NewServeMux()
		pprof.RegisterRoutes(pprofMux)
		origHandler := handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/debug/pprof/") {
				pprof.RequireBearer(pprofMux, nodeToken).ServeHTTP(w, r)
				return
			}
			origHandler.ServeHTTP(w, r)
		})
		log.Printf("pprof profiling enabled at /debug/pprof/")
	}

	rateTiers := ratelimit.DefaultTiers()
	for index, key := range []string{"DAEMON_RATE_POWER_RPM", "DAEMON_RATE_WEBSOCKET_RPM", "DAEMON_RATE_FILES_RPM", "DAEMON_RATE_DEFAULT_RPM"} {
		rateTiers[index].RequestsPerMinute = envInt(key, rateTiers[index].RequestsPerMinute)
	}
	rateLimiter := ratelimit.NewTieredLimiter(rateTiers)
	handler = rateLimiter.Middleware()(handler)
	log.Printf("HTTP rate limiting enabled (tiers: power=30, ws=60, files=120, default=240 req/min)")

	tokenGen := tokens.NewGenerator([]byte(nodeToken))
	if tokenGen == nil {
		return errors.New("token generator initialization failed")
	}
	server.SetTokenGenerator(tokenGen)

	cronScheduler, cronErr := cron.NewScheduler("UTC")
	if cronErr != nil {
		log.Printf("WARNING: cron scheduler failed: %v", cronErr)
	} else {
		cleanupCron := cron.NewCleanupCron(dataDir)
		cronScheduler.AddJob("cleanup", 24*time.Hour, cleanupCron.Run)
		if err := cronScheduler.Start(daemonCtx); err != nil {
			log.Printf("WARNING: cron start failed: %v", err)
		} else {
			log.Printf("cron scheduler started (cleanup job: 24h interval)")
			defer cronScheduler.Stop()
		}
	}

	// Wire the remote panel client so install-status notifications work.
	if panelOnboardingEnabled {
		panelClient := remote.NewClient(panelAPIURL, nodeToken)
		server.SetPanelClient(panelClient)

		// Start the edge agent for panel-side edge connectivity/heartbeat
		// (separate from the node heartbeat loop below).
		edgeAgent := daemonhttp.NewEdgeAgent(panelAPIURL, nodeToken, nodeID, Version)
		server.SetEdgeAgent(edgeAgent)
		go edgeAgent.Start(daemonCtx)

		// Report crashes to the panel API.
		server.SetCrashHandler(func(ctx context.Context, serverID string, exitCode int, oomKilled bool) {
			if err := panelClient.SendCrashEvent(ctx, serverID, exitCode, oomKilled, true); err != nil {
				log.Printf("failed to report crash for server %s: %v", serverID, err)
			}
		})
	}

	// Apply crash detection settings from env (matches Wings'
	// config.system.crash_detection.detect_clean_exit_as_crash).
	server.SetDetectCleanExitAsCrash(env("DAEMON_DETECT_CLEAN_EXIT_AS_CRASH", "false") == "true")

	// SFTP activity is deduplicated before remote delivery. Session/client
	// metadata is bounded and contains no credentials.
	var activity *system.ActivityDedup
	if panelOnboardingEnabled {
		activityClient := remote.NewClient(panelAPIURL, nodeToken)
		activity = system.NewActivityDedup(2*time.Second, 100, func(serverID string, entries []system.ActivityEntry) {
			batch := make([]remote.Activity, 0, len(entries))
			for _, entry := range entries {
				batch = append(batch, remote.Activity{Event: entry.Action, User: entry.User, Server: serverID, IP: entry.IP, Timestamp: entry.Timestamp.UTC().Format(time.RFC3339Nano), Metadata: map[string]interface{}{"path": entry.Path, "client": entry.Client, "session": entry.SessionID}})
			}
			flushCtx, cancel := context.WithTimeout(daemonCtx, 10*time.Second)
			defer cancel()
			if err := activityClient.SendActivityLogs(flushCtx, batch); err != nil && daemonCtx.Err() == nil {
				log.Printf("sftp activity delivery failed: %v", err)
			}
		})
		activity.Start()
		defer activity.Stop()
	}

	// SFTP server
	sftpErr := make(chan error, 1)
	go func() {
		// DAEMON_SFTP_ADDR is the advertised endpoint; the bind-specific value
		// controls the listener when supplied by containerized deployments.
		sftpAddr := env("DAEMON_SFTP_BIND_ADDR", env("DAEMON_SFTP_ADDR", ":2022"))
		sftpPassphrase, passErr := sftpHostKeyPassphrase(dataDir, os.Getenv("DAEMON_SFTP_HOST_KEY_PASSPHRASE"))
		if passErr != nil {
			log.Printf("native sftp disabled: %v", passErr)
			return
		}
		sftpSrv := &sftpserver.Server{
			Addr: sftpAddr, DataDir: dataDir, PanelAPIURL: panelAPIURL, NodeToken: nodeToken,
			ReadOnly: env("DAEMON_SFTP_READ_ONLY", "false") == "true", IdleTimeout: envDuration("DAEMON_SFTP_IDLE_TIMEOUT", 15*time.Minute),
			MaxConnections: envInt("DAEMON_SFTP_MAX_CONNECTIONS", 128), MaxSessionsPerUser: envInt("DAEMON_SFTP_MAX_SESSIONS_PER_USER", 8),
			MaxSessionLifetime: envDuration("DAEMON_SFTP_MAX_SESSION_LIFETIME", 24*time.Hour),
			HostKeyPassphrase:  sftpPassphrase,
			Activity:           activity, Sessions: server,
		}
		if err := sftpSrv.Run(daemonCtx); err != nil {
			sftpErr <- err
		}
	}()

	// Sync servers from panel and start heartbeat.
	if panelOnboardingEnabled {
		if err := syncServersFromPanel(daemonCtx, panelAPIURL, nodeToken, dataDir, server); err != nil {
			log.Printf("panel server sync failed: %v", err)
			log.Printf("attempting server recovery from local disk cache...")
			if recErr := recoverServersFromDisk(daemonCtx, dataDir, server); recErr != nil {
				log.Printf("local server recovery failed: %v", recErr)
			}
		}
		var pinger runtime.Pinger
		if rt != nil {
			if p, ok := rt.(runtime.Pinger); ok {
				pinger = p
			}
		}
		go heartbeatLoop(daemonCtx, panelAPIURL, nodeID, nodeToken, dataDir, pinger, runtimeProvider, Version)
	} else {
		if err := recoverServersFromDisk(daemonCtx, dataDir, server); err != nil {
			log.Printf("local server recovery failed: %v", err)
		}
	}

	tlsCfg := &tls.Config{Mode: tls.ModeNone}
	if cert := os.Getenv("DAEMON_TLS_CERT_FILE"); cert != "" && os.Getenv("DAEMON_TLS_KEY_FILE") != "" {
		tlsCfg.Mode = tls.ModeManual
		tlsCfg.CertFile = cert
		tlsCfg.KeyFile = os.Getenv("DAEMON_TLS_KEY_FILE")
	} else if hostname := os.Getenv("DAEMON_AUTO_TLS_HOSTNAME"); hostname != "" {
		tlsCfg.Mode = tls.ModeAutoTLS
		tlsCfg.Hostname = hostname
		tlsCfg.CacheDir = filepath.Join(dataDir, ".tls-cache")
	}

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    envInt("DAEMON_MAX_HEADER_BYTES", 64<<10),
	}

	if err := tlsCfg.Apply(httpServer); err != nil {
		return fmt.Errorf("TLS configuration failed: %w", err)
	}

	// Initialize shutdown manager
	shutdownManager := shutdown.NewShutdownManager(30 * time.Second)
	go shutdownManager.Wait()

	// Run server in background and wait for SIGINT/SIGTERM for graceful
	// shutdown so in-flight transfers/installs can complete cleanly.
	serverErr := make(chan error, 1)
	go func() {
		switch tlsCfg.Mode {
		case tls.ModeAutoTLS:
			log.Printf("daemon listening on %s (Auto-TLS: %s)", addr, tlsCfg.Hostname)
			if err := httpServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				serverErr <- err
			}
		case tls.ModeManual:
			log.Printf("daemon listening on %s (TLS: %s)", addr, tlsCfg.CertFile)
			if err := httpServer.ListenAndServeTLS(tlsCfg.CertFile, tlsCfg.KeyFile); err != nil && err != http.ErrServerClosed {
				serverErr <- err
			}
		default:
			log.Printf("daemon listening on %s", addr)
			if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				serverErr <- err
			}
		}
	}()

	stop := make(chan os.Signal, 2)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	var serveErr error
waitForShutdown:
	for {
		select {
		case serveErr = <-serverErr:
			log.Printf("http server stopped: %v", serveErr)
			break waitForShutdown
		case err := <-sftpErr:
			log.Printf("native sftp stopped: %v", err)
			// The SFTP subsystem is auxiliary to the daemon's primary runtime
			// API. A failure (bind address in use, host-key initialization
			// problem, unexpected shutdown) must not take the whole control
			// plane down; keep serving HTTP and let the operator investigate.
		case sig := <-stop:
			if sig == syscall.SIGHUP {
				reloaded, err := config.LoadWithOptions(config.LoadOptions{Path: os.Getenv("DAEMON_CONFIG_FILE")})
				if err != nil {
					log.Printf("configuration reload rejected: %v", err)
					continue
				}
				server.SetAllowedMounts(reloaded.AllowedMountsList())
				if bw, ok := backupAdapter.(interface{ SetWriteLimit(int64) }); ok {
					bw.SetWriteLimit(reloaded.BackupConfig().WriteLimit)
				}
				log.Print("reloaded dynamic configuration (allowed mounts and backup write limit); listener/TLS changes require restart")
				continue
			}
			log.Printf("received %s, shutting down gracefully", sig)
			break waitForShutdown
		}
	}
	signal.Stop(stop)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
	cancelDaemon()
	server.Shutdown()
	tlsCfg.StopChallengeServer()
	shutdownManager.Shutdown()
	if serveErr != nil {
		return serveErr
	}
	return nil
}

func isLoopbackListenAddress(address string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func readSingleLineSecret(path string) (string, error) {
	info, err := os.Lstat(strings.TrimSpace(path))
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 || info.Size() > 4096 {
		return "", errors.New("secret file must be a non-empty regular file no larger than 4 KiB")
	}
	if info.Mode().Perm() != 0o600 {
		return "", errors.New("secret file permissions must be 0600")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	value := strings.TrimSuffix(string(body), "\n")
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return "", errors.New("secret file must contain exactly one non-empty line")
	}
	return value, nil
}

// sftpHostKeyPassphrase resolves the passphrase used to encrypt the SFTP host
// key at rest. An explicit DAEMON_SFTP_HOST_KEY_PASSPHRASE (16+ bytes) wins;
// otherwise a stable per-install secret is loaded from (or generated into)
// <dataDir>/.sftp/host-key-passphrase so first-run bootstrap completes without
// extra configuration and the host key survives daemon restarts and
// node-token rotation. The stored secret is created with 0600 permissions and
// is never logged.
func sftpHostKeyPassphrase(dataDir, envValue string) (string, error) {
	if passphrase := strings.TrimSpace(envValue); passphrase != "" {
		if len(passphrase) < 16 {
			return "", errors.New("DAEMON_SFTP_HOST_KEY_PASSPHRASE must contain at least 16 bytes")
		}
		return passphrase, nil
	}
	path := filepath.Join(dataDir, ".sftp", "host-key-passphrase")
	if body, err := os.ReadFile(path); err == nil {
		value := strings.TrimSuffix(string(body), "\n")
		if value == "" || len(value) < 16 || strings.ContainsAny(value, "\r\n") {
			return "", fmt.Errorf("stored SFTP host-key passphrase is invalid; remove %s to regenerate it", path)
		}
		return value, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	secret := make([]byte, 24)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("generate SFTP host-key passphrase: %w", err)
	}
	value := hex.EncodeToString(secret)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("create SFTP secret directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(value+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("persist SFTP host-key passphrase: %w", err)
	}
	return value, nil
}

func syncServersFromPanel(ctx context.Context, panelAPIURL, token, dataDir string, daemon *daemonhttp.Server) error {
	client := remote.NewClient(panelAPIURL, token)
	servers, err := client.GetServers(ctx, 50)
	if err != nil {
		return err
	}
	var syncErrors []error
	for _, srv := range servers {
		if err := serverid.Validate(srv.Uuid); err != nil {
			return fmt.Errorf("panel returned invalid server id %q: %w", srv.Uuid, err)
		}
		serverRoot, err := filepath.Abs(filepath.Join(dataDir, srv.Uuid))
		if err != nil {
			syncErrors = append(syncErrors, fmt.Errorf("canonicalize server %s root: %w", srv.Uuid, err))
			continue
		}
		root := filepath.Join(serverRoot, ".config")
		if err := os.MkdirAll(root, 0o750); err != nil {
			syncErrors = append(syncErrors, fmt.Errorf("prepare server %s: %w", srv.Uuid, err))
			continue
		}
		body, err := json.MarshalIndent(map[string]any{
			"settings":              srv.Settings,
			"process_configuration": srv.ProcessConfiguration,
			"suspended":             srv.Suspended,
			"is_installing":         srv.Installing,
			"installed":             srv.Installed,
			"status":                srv.Status,
		}, "", "  ")
		if err != nil {
			return err
		}
		if err := writePrivateFileAtomic(filepath.Join(root, "server.json"), body); err != nil {
			syncErrors = append(syncErrors, fmt.Errorf("persist server %s: %w", srv.Uuid, err))
			continue
		}
		diskLimitMB, suspended, installationState := panelServerState(srv.Settings)
		if srv.Suspended != nil {
			suspended = *srv.Suspended
		}
		switch {
		case srv.Installing != nil && *srv.Installing, strings.EqualFold(srv.Status, "installing"):
			installationState = "installing"
		case srv.Installed != nil && !*srv.Installed:
			installationState = "uninstalled"
		}
		if err := daemon.ReconstructServer(ctx, daemonhttp.Reconstruction{
			ServerID:            srv.Uuid,
			RootDir:             serverRoot,
			DiskLimitMB:         diskLimitMB,
			ConfigurationSynced: true,
			InstallationState:   installationState,
			Suspended:           suspended,
		}); err != nil {
			syncErrors = append(syncErrors, fmt.Errorf("reconstruct server %s: %w", srv.Uuid, err))
		}
	}
	if err := client.ResetServersState(ctx); err != nil {
		syncErrors = append(syncErrors, err)
	}
	log.Printf("synced %d server configurations from panel", len(servers))
	return errors.Join(syncErrors...)
}

func recoverServersFromDisk(ctx context.Context, dataDir string, daemon *daemonhttp.Server) error {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return err
	}
	var recoveryErrors []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		serverID := entry.Name()
		if err := serverid.Validate(serverID); err != nil {
			continue
		}
		serverRoot := filepath.Join(dataDir, serverID)
		configFile := filepath.Join(serverRoot, ".config", "server.json")
		data, err := os.ReadFile(configFile)
		if err != nil {
			continue
		}
		var srv struct {
			Settings             json.RawMessage `json:"settings"`
			ProcessConfiguration json.RawMessage `json:"process_configuration"`
			Suspended            *bool           `json:"suspended"`
			Installing           *bool           `json:"is_installing"`
			Installed            *bool           `json:"installed"`
			Status               string          `json:"status"`
		}
		if err := json.Unmarshal(data, &srv); err != nil {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("unmarshal cached config for %s: %w", serverID, err))
			continue
		}
		diskLimitMB, suspended, installationState := panelServerState(srv.Settings)
		if srv.Suspended != nil {
			suspended = *srv.Suspended
		}
		switch {
		case srv.Installing != nil && *srv.Installing, strings.EqualFold(srv.Status, "installing"):
			installationState = "installing"
		case srv.Installed != nil && !*srv.Installed:
			installationState = "uninstalled"
		}
		if err := daemon.ReconstructServer(ctx, daemonhttp.Reconstruction{
			ServerID:            serverID,
			RootDir:             serverRoot,
			DiskLimitMB:         diskLimitMB,
			ConfigurationSynced: true,
			InstallationState:   installationState,
			Suspended:           suspended,
		}); err != nil {
			recoveryErrors = append(recoveryErrors, fmt.Errorf("recover server %s: %w", serverID, err))
		}
	}
	return errors.Join(recoveryErrors...)
}

func panelServerState(raw json.RawMessage) (diskLimitMB int64, suspended bool, installationState string) {
	installationState = "installed"
	var settings struct {
		Suspended  bool   `json:"suspended"`
		Installing bool   `json:"is_installing"`
		Status     string `json:"status"`
		Installed  *bool  `json:"installed"`
		Disk       int64  `json:"disk"`
		DiskMB     int64  `json:"disk_mb"`
		DiskSpace  int64  `json:"disk_space"`
		Build      struct {
			DiskSpace int64 `json:"disk_space"`
			Disk      int64 `json:"disk"`
			DiskMB    int64 `json:"disk_mb"`
		} `json:"build"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		log.Printf("panel server settings could not be decoded: %v", err)
		return 0, false, installationState
	}
	suspended = settings.Suspended
	switch {
	case settings.Installing || strings.EqualFold(settings.Status, "installing"):
		installationState = "installing"
	case settings.Installed != nil && !*settings.Installed:
		installationState = "uninstalled"
	}
	for _, value := range []int64{settings.Build.DiskSpace, settings.Build.Disk, settings.Build.DiskMB, settings.DiskSpace, settings.Disk, settings.DiskMB} {
		if value > 0 {
			diskLimitMB = value
			break
		}
	}
	return diskLimitMB, suspended, installationState
}

func heartbeatLoop(ctx context.Context, panelAPIURL, nodeID, token, dataDir string, pinger runtime.Pinger, runtimeProvider, version string) {
	client := remote.NewClient(panelAPIURL, token)
	startTime := time.Now()

	send := func() {
		runtimeStatus, errText := runtimeHeartbeatStatus(pinger, runtimeProvider)
		loadAvg := systemLoadAverage()

		heartbeat := remote.NodeHeartbeat{
			Version:         version,
			OS:              goruntime.GOOS,
			Architecture:    goruntime.GOARCH,
			CPUThreads:      goruntime.NumCPU(),
			MemoryMB:        readMemoryMB(),
			DiskMB:          readDiskMB(dataDir),
			RuntimeStatus:   runtimeStatus,
			RuntimeProvider: runtimeProvider,
			Error:           errText,
			Uptime:          int64(time.Since(startTime).Seconds()),
			LoadAverage:     loadAvg,
		}

		if err := client.SendNodeHeartbeat(ctx, nodeID, heartbeat); err != nil && ctx.Err() == nil {
			log.Printf("heartbeat failed: %v", err)
		}
	}

	send()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			send()
		}
	}
}

func runtimeHeartbeatStatus(pinger runtime.Pinger, runtimeProvider string) (string, string) {
	if pinger == nil {
		return "error", runtimeProvider + " runtime unavailable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pinger.Ping(ctx); err != nil {
		return "error", runtimeProvider + " ping failed: " + err.Error()
	}
	return "ok", ""
}

// validatePanelOnboarding enforces the panel-specific startup contract. A node
// token can be set for local daemon API authentication without enabling panel
// connectivity, but a panel connection requires all three values.
func validatePanelOnboarding(nodeID, panelAPIURL, nodeToken string) error {
	if (strings.TrimSpace(nodeID) == "") != (strings.TrimSpace(panelAPIURL) == "") {
		return errors.New("panel onboarding requires DAEMON_NODE_ID and PANEL_API_URL together (legacy WINGS_NODE_ID and WINGS_PANEL_URL are also supported)")
	}
	if strings.TrimSpace(nodeID) == "" {
		return nil
	}
	if strings.TrimSpace(nodeToken) == "" {
		return errors.New("panel onboarding requires DAEMON_NODE_TOKEN (or WINGS_TOKEN) with DAEMON_NODE_ID and PANEL_API_URL")
	}

	parsed, err := url.Parse(strings.TrimSpace(panelAPIURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("PANEL_API_URL (or WINGS_PANEL_URL) must be an absolute http(s) URL, got %q", panelAPIURL)
	}
	return nil
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
	return ":9090"
}

func healthcheck(target string) error {
	client := http.Client{Timeout: 3 * time.Second}
	res, err := client.Get(target)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("unhealthy status %d", res.StatusCode)
	}
	return nil
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func ensurePrivateDataDirectory(dataDir string) error {
	if strings.TrimSpace(dataDir) == "" {
		return errors.New("data directory is required")
	}
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return err
	}
	info, err := os.Stat(dataDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("data directory path is not a directory")
	}
	probe, err := os.CreateTemp(dataDir, ".beacon-write-test-*")
	if err != nil {
		return fmt.Errorf("data directory is not writable: %w", err)
	}
	probePath := probe.Name()
	if err := probe.Close(); err != nil {
		_ = os.Remove(probePath)
		return err
	}
	return os.Remove(probePath)
}

// buildBackupAdapter returns the configured backup adapter based on
// BACKUP_ADAPTER env var. "s3" enables the S3 adapter; anything else (or
// unset) uses the local filesystem adapter.
//
// Required S3 env vars:
//   - BACKUP_ADAPTER=s3
//   - S3_BUCKET  (required)
//   - S3_REGION  (required)
//   - S3_ACCESS_KEY_ID (required)
//   - S3_SECRET_ACCESS_KEY (required)
//   - S3_ENDPOINT  (optional, for S3-compatible storage)
//   - S3_PREFIX    (optional, default empty)
func buildBackupAdapter(backupRoot string, legacyDataRoot ...string) (backup.BackupInterface, error) {
	adapter := strings.ToLower(strings.TrimSpace(env("BACKUP_ADAPTER", "local")))
	switch adapter {
	case "", "local":
		local, err := backup.NewLocalBackup(backupRoot, legacyDataRoot...)
		if err != nil {
			return nil, err
		}
		log.Printf("backup adapter: local (root=%s)", backupRoot)
		return local, nil
	case "s3":
		cfg := &backup.S3Config{
			Endpoint:        env("S3_ENDPOINT", ""),
			Region:          os.Getenv("S3_REGION"),
			Bucket:          os.Getenv("S3_BUCKET"),
			AccessKeyID:     os.Getenv("S3_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("S3_SECRET_ACCESS_KEY"),
			Prefix:          env("S3_PREFIX", ""),
			UsePathStyle:    strings.EqualFold(env("S3_USE_PATH_STYLE", "true"), "true"),
			BackupRoot:      backupRoot,
		}
		s3Adapter, err := backup.NewS3Backup(cfg)
		if err != nil {
			return nil, fmt.Errorf("BACKUP_ADAPTER=s3: %w", err)
		}
		log.Printf("backup adapter: s3 (bucket=%s region=%s prefix=%s staging=%s path_style=%t)", cfg.Bucket, cfg.Region, cfg.Prefix, backupRoot, cfg.UsePathStyle)
		return s3Adapter, nil
	default:
		return nil, fmt.Errorf("unsupported BACKUP_ADAPTER %q", adapter)
	}
}

// readMemoryMB is defined in mem_linux.go (Linux) and mem_other.go (other OS).

func readDiskMB(dataDir string) int64 {
	if dataDir == "" {
		return 0
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(dataDir, &stat); err != nil {
		log.Printf("read disk capacity for %s failed: %v", dataDir, err)
		return 0
	}
	bytes := uint64(stat.Bavail) * uint64(stat.Bsize)
	return int64(bytes / (1024 * 1024))
}
