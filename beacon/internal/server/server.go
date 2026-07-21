package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	stdruntime "runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gamepanel/beacon/internal/backup"
	"gamepanel/beacon/internal/events"
	"gamepanel/beacon/internal/ignore"
	"gamepanel/beacon/internal/remote"
	"gamepanel/beacon/internal/rootfs"
	"gamepanel/beacon/internal/runtime"
	"gamepanel/beacon/internal/serverid"
	"gamepanel/beacon/internal/tokens"
	"gamepanel/beacon/internal/transfer"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/gorilla/websocket"
)

// Ensure UpgradePayload can be referenced from handlers defined in this file.
var _ = UpgradePayload{}

var (
	allowedWebSocketOrigins = loadAllowedWebSocketOrigins()
	errRuntimeUnavailable   = errors.New("runtime unavailable")
)

// ConsoleOutputEvent is the topic name used on the per-server event bus for
// console output lines. WebSocket clients subscribe to this topic.
const ConsoleOutputEvent = "console output"

const BackupProgressEvent = "backup progress"

type Server struct {
	runtime           runtime.Runtime
	manager           *ServerManager
	dataDir           string
	allowedMounts     []string
	allowedMountsMu   sync.RWMutex
	token             string
	started           time.Time
	backups           backup.BackupInterface
	backupMu          sync.Mutex
	sessionsReg       *sessionRegistry
	dockerState       string
	panelClient       remote.Client
	eventBus          *events.Bus
	consoles          *consoleManager
	pullClientFactory func(context.Context, *url.URL) (*http.Client, error)
	transferProtocol  *transfer.Engine
	operations        *OperationQueue
	tokenGenerator    *tokens.Generator
	composeStacks     *composeStack
	enrollmentMgr     *EnrollmentManager
	ctx               context.Context
	cancel            context.CancelFunc
}

// SetPanelClient wires the remote panel client so that install-status
// notifications can be sent after installation completes.
func (s *Server) SetPanelClient(c remote.Client) {
	s.panelClient = c
}

// SetTokenGenerator wires the JWT token generator for direct download tokens.
func (s *Server) SetTokenGenerator(g *tokens.Generator) {
	s.tokenGenerator = g
}

// SetAllowedMounts configures the host paths that panel-supplied mounts may
// use. An empty list denies all custom host mounts.
func (s *Server) SetAllowedMounts(mounts []string) {
	s.allowedMountsMu.Lock()
	defer s.allowedMountsMu.Unlock()
	s.allowedMounts = append(s.allowedMounts[:0], mounts...)
}

func (s *Server) allowedMountSources() []string {
	s.allowedMountsMu.RLock()
	defer s.allowedMountsMu.RUnlock()
	return append([]string(nil), s.allowedMounts...)
}

// EventBus returns the server-wide event bus used to publish and subscribe to
// events such as console output.
func (s *Server) EventBus() *events.Bus {
	return s.eventBus
}

var websocketUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin == "" {
			// Non-browser clients such as native tools generally do not send an
			// Origin header; allow them and rely on bearer/HMAC auth.
			return true
		}
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return false
		}
		for _, allowed := range allowedWebSocketOrigins {
			if strings.EqualFold(origin, allowed) {
				return true
			}
		}
		return false
	},
}

func loadAllowedWebSocketOrigins() []string {
	raw := os.Getenv("DAEMON_WS_ALLOWED_ORIGINS")
	if strings.TrimSpace(raw) == "" {
		origins := []string{}
		if panelURL := strings.TrimSpace(os.Getenv("PANEL_API_URL")); panelURL != "" {
			if origin := originFromURL(panelURL); origin != "" {
				origins = append(origins, origin)
			}
		}
		if panelURL := strings.TrimSpace(os.Getenv("WINGS_PANEL_URL")); panelURL != "" {
			if origin := originFromURL(panelURL); origin != "" {
				origins = append(origins, origin)
			}
		}
		return dedupeOrigins(origins)
	}
	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		if origin := originFromURL(value); origin != "" {
			origins = append(origins, origin)
		}
	}
	return dedupeOrigins(origins)
}

func originFromURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}

func dedupeOrigins(origins []string) []string {
	seen := make(map[string]struct{}, len(origins))
	result := make([]string, 0, len(origins))
	for _, origin := range origins {
		key := strings.ToLower(origin)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, origin)
	}
	return result
}

func NewServer(rt runtime.Runtime, dataDir string, nodeToken ...string) (*Server, http.Handler) {
	backupRoot := filepath.Join(filepath.Dir(dataDir), "backups")
	local, err := backup.NewLocalBackup(backupRoot, dataDir)
	if err != nil {
		log.Printf("[beacon] local backup adapter unavailable: %v", err)
	}
	return NewServerWithBackup(rt, dataDir, local, nodeToken...)
}

// NewServerWithBackup constructs the server with an explicit backup adapter.
// Pass an explicitly configured local or S3 adapter to choose where backups
// are stored. Driven by BACKUP_ADAPTER and DAEMON_BACKUP_DIR in main.go.
//
// Returns the *Server (for typed configuration like
// SetDetectCleanExitAsCrash) and the http.Handler to mount on an
// http.Server. Separating the two lets callers tweak the server before
// binding it.
func NewServerWithBackup(rt runtime.Runtime, dataDir string, backups backup.BackupInterface, nodeToken ...string) (*Server, http.Handler) {
	token := ""
	if len(nodeToken) > 0 {
		token = nodeToken[0]
	}
	manager := NewServerManager(rt)
	serverCtx, cancel := context.WithCancel(context.Background())
	protocol, protocolErr := transfer.NewProtocolEngine(dataDir)
	if protocolErr != nil {
		log.Printf("[beacon] transfer protocol unavailable: %v", protocolErr)
	}
	server := &Server{
		runtime:           rt,
		manager:           manager,
		dataDir:           dataDir,
		token:             token,
		started:           time.Now(),
		backups:           backups,
		sessionsReg:       newSessionRegistry(),
		dockerState:       "ok",
		eventBus:          events.NewBus(),
		pullClientFactory: securePullClient,
		transferProtocol:  protocol,
		composeStacks:     newComposeStackManager(dataDir),
		enrollmentMgr:     NewEnrollmentManager(filepath.Join(filepath.Dir(dataDir), "enrollment")),
		ctx:               serverCtx,
		cancel:            cancel,
	}
	server.consoles = newConsoleManager(serverCtx, rt)
	manager.SetConsoleCommand(server.consoles.Write)
	manager.SetConsoleLifecycle(func(serverID string) {
		if err := server.consoles.Ensure(serverID); err != nil {
			log.Printf("[beacon] failed to attach console for %s: %v", serverID, err)
		}
	}, server.consoles.Stop)
	manager.StartEventWatcher(serverCtx)
	operationHandler := func(ctx context.Context, op *Operation) error {
		if rt == nil {
			return errRuntimeUnavailable
		}
		return manager.HandlePower(ctx, op.ServerID, string(op.Type))
	}
	journalPath := filepath.Join(filepath.Dir(dataDir), "journal", "operations.db")
	operationQueue, journalErr := NewPersistentOperationQueue(journalPath, 2, operationHandler)
	if journalErr != nil {
		log.Printf("[beacon] persistent command journal unavailable, using memory queue: %v", journalErr)
		operationQueue = NewOperationQueue(2, operationHandler)
	}
	server.operations = operationQueue
	server.operations.Start(serverCtx)
	// Start automatic expiry cleanup for commands with TTL
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				server.operations.ExpireExpired()
			case <-serverCtx.Done():
				return
			}
		}
	}()
	if rt == nil {
		server.dockerState = "error"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", server.health)
	mux.HandleFunc("GET /metrics", server.metrics)
	mux.HandleFunc("POST /servers", server.create)
	mux.HandleFunc("DELETE /servers/{id}", server.delete)
	mux.HandleFunc("GET /servers/{id}/configuration", server.getConfiguration)
	mux.HandleFunc("PUT /servers/{id}/configuration", server.syncConfiguration)
	mux.HandleFunc("POST /servers/{id}/install", server.install)
	mux.HandleFunc("GET /servers/{id}/install/ws", server.installWS)
	mux.HandleFunc("POST /servers/{id}/reinstall", server.reinstall)
	mux.HandleFunc("POST /servers/{id}/power", server.power)
	mux.HandleFunc("GET /servers/{id}/operations", server.listOperations)
	mux.HandleFunc("GET /operations/{id}", server.getOperation)
	mux.HandleFunc("GET /servers/{id}/stats", server.stats)
	mux.HandleFunc("GET /servers/{id}/logs", server.logs)
	mux.HandleFunc("POST /servers/{id}/backups", server.createBackup)
	mux.HandleFunc("GET /servers/{id}/backups", server.listBackups)
	mux.HandleFunc("GET /servers/{id}/backups/download", server.downloadBackup)
	mux.HandleFunc("POST /servers/{id}/backups/restore", server.restoreBackup)
	mux.HandleFunc("DELETE /servers/{id}/backups/{backupId}", server.deleteBackup)
	mux.HandleFunc("DELETE /servers/{id}/backups", server.deleteBackup)
	mux.HandleFunc("GET /servers/{id}/ws/stats", server.statsWS)
	mux.HandleFunc("GET /servers/{id}/ws/logs", server.logsWS)
	mux.HandleFunc("GET /servers/{id}/ws/console", server.consoleWS)
	mux.HandleFunc("GET /servers/{id}/ws/backup", server.backupProgressWS)
	mux.HandleFunc("GET /servers/{id}/files", server.listFiles)
	mux.HandleFunc("DELETE /servers/{id}/files", server.deleteFile)
	mux.HandleFunc("POST /servers/{id}/files/mkdir", server.makeDir)
	mux.HandleFunc("PATCH /servers/{id}/files/rename", server.renameFile)
	mux.HandleFunc("POST /servers/{id}/files/archive", server.archiveFiles)
	mux.HandleFunc("POST /servers/{id}/files/decompress", server.decompressFile)
	mux.HandleFunc("POST /servers/{id}/files/delete-batch", server.batchDeleteFiles)
	mux.HandleFunc("POST /servers/{id}/files/rename-batch", server.batchRenameFiles)
	mux.HandleFunc("POST /servers/{id}/files/chmod", server.chmodFiles)
	mux.HandleFunc("POST /servers/{id}/files/copy", server.copyFile)
	mux.HandleFunc("POST /servers/{id}/files/pull", server.pullRemoteFile)
	mux.HandleFunc("GET /servers/{id}/files/download", server.downloadFile)
	mux.HandleFunc("GET /servers/{id}/files/content", server.readFile)
	mux.HandleFunc("PUT /servers/{id}/files/content", server.writeFile)
	mux.HandleFunc("PUT /servers/{id}/files/upload", server.uploadFileChunk)
	mux.HandleFunc("POST /servers/{id}/command", server.command)
	mux.HandleFunc("POST /servers/{id}/transfers", server.startTransfer)
	mux.HandleFunc("GET /servers/{id}/transfers/{transferId}", server.getTransferStatus)
	mux.HandleFunc("DELETE /servers/{id}/transfers/{transferId}", server.cancelTransfer)
	mux.HandleFunc("POST /api/transfers", server.receiveTransferArchive)
	mux.HandleFunc("POST /api/v1/transfers/credentials", server.registerTransferCredential)
	mux.HandleFunc("POST /api/v1/transfers/{id}/source/prepare", server.prepareTransferSource)
	mux.HandleFunc("POST /api/v1/transfers/{id}/source/push", server.pushTransferSource)
	mux.HandleFunc("GET /api/v1/transfers/{id}/source/status", server.sourceTransferStatus)
	mux.HandleFunc("POST /api/v1/transfers/{id}/source/cleanup", server.cleanupTransferSource)
	mux.HandleFunc("POST /mounts/cleanup", server.cleanupMount)
	mux.HandleFunc("HEAD /api/v1/transfers/{id}/destination/archive", server.destinationTransferOffset)
	mux.HandleFunc("PATCH /api/v1/transfers/{id}/destination/archive", server.receiveTransferChunk)
	mux.HandleFunc("POST /api/v1/transfers/{id}/destination/restore", server.restoreTransferDestination)
	mux.HandleFunc("POST /api/v1/transfers/{id}/destination/finalize", server.finalizeTransferDestination)
	mux.HandleFunc("DELETE /api/v1/transfers/{id}", server.cancelProtocolTransfer)
	// Global daemon endpoints (/api/*)
	mux.HandleFunc("GET /api/system", server.getSystem)
	mux.HandleFunc("GET /api/capabilities", server.handleGetCapabilities)
	mux.HandleFunc("GET /api/capabilities/delta", server.handleGetCapabilitiesDelta)
	mux.HandleFunc("POST /api/capabilities", server.handlePostCapabilitiesHeartbeat)
	mux.HandleFunc("POST /api/enroll", server.handleEnroll)
	mux.HandleFunc("GET /api/enroll/status", server.handleEnrollmentStatus)
	mux.HandleFunc("POST /api/update", server.postUpdate)
	mux.HandleFunc("POST /api/deauthorize-user", server.postDeauthorizeUser)
	mux.HandleFunc("GET /download/backup", server.downloadBackupWithToken)
	mux.HandleFunc("POST /compose/deploy", server.handleComposeDeploy)
	mux.HandleFunc("POST /compose/{stackId}/stop", server.handleComposeStop)
	mux.HandleFunc("POST /compose/{stackId}/start", server.handleComposeStart)
	mux.HandleFunc("POST /compose/{stackId}/restart", server.handleComposeRestart)
	mux.HandleFunc("DELETE /compose/{stackId}", server.handleComposeDelete)
	mux.HandleFunc("GET /compose/{stackId}/status", server.handleComposeStatus)
	mux.HandleFunc("GET /compose/{stackId}/logs", server.handleComposeLogs)
	mux.HandleFunc("POST /compose/{stackId}/pull", server.handleComposePull)
	// Git source deployment endpoints
	mux.HandleFunc("POST /git/clone", server.handleGitClone)
	mux.HandleFunc("POST /git/build", server.handleGitBuild)
	mux.HandleFunc("DELETE /git/cleanup", server.handleGitCleanup)
	// Database container provisioning endpoints
	mux.HandleFunc("POST /database/provision", server.handleDatabaseProvision)
	mux.HandleFunc("DELETE /database/provision", server.handleDatabaseDeProvision)
	mux.HandleFunc("POST /database/backup", server.handleDatabaseBackup)
	mux.HandleFunc("GET /database/status/{containerId}", server.handleDatabaseStatus)
	// Build endpoints
	mux.HandleFunc("POST /build/dockerfile", server.handleDockerfileBuild)
	mux.HandleFunc("POST /build/nixpacks", server.handleNixpacksBuild)
	mux.HandleFunc("GET /build/logs", server.handleBuildLogs)
	mux.HandleFunc("POST /build/cancel", server.handleBuildCancel)
	mux.HandleFunc("GET /build/status", server.handleBuildStatus)
	mux.HandleFunc("POST /build/cleanup", server.handleBuildCleanup)
	mux.HandleFunc("POST /image/push", server.handleImagePush)
	mux.HandleFunc("GET /image/inspect", server.handleImageInspect)
	mux.HandleFunc("POST /registry/login", server.handleRegistryLogin)
	// Edge agent and diagnostics endpoints
	mux.HandleFunc("GET /api/edge/status", server.handleEdgeStatus)
	mux.HandleFunc("GET /api/edge/stats", server.handleEdgeStats)
	mux.HandleFunc("POST /api/edge/connect", server.handleEdgeConnect)
	mux.HandleFunc("GET /api/diagnostics", server.handleDiagnostics)
	mux.HandleFunc("GET /api/diagnostics/connectivity", server.handleConnectivityDiagnostics)
	mux.HandleFunc("GET /api/version", server.handleVersionInventory)
	mux.HandleFunc("POST /api/upgrade", server.handleUpgradeBegin)
	mux.HandleFunc("GET /api/upgrade/status", server.handleUpgradeStatus)
	mux.HandleFunc("POST /api/upgrade/apply", server.handleUpgradeApply)
	mux.HandleFunc("POST /api/upgrade/rollback", server.handleUpgradeRollback)
	// Command acknowledgement and progress
	mux.HandleFunc("POST /api/commands/{id}/ack", server.handleCommandAck)
	mux.HandleFunc("POST /api/commands/{id}/progress", server.handleCommandProgress)
	mux.HandleFunc("POST /api/commands/{id}/result", server.handleCommandResult)
	// Command polling (Beacon pulls pending commands per server)
	mux.HandleFunc("GET /api/commands/pending", server.handlePendingCommands)
	// Portainer-inspired container/image/network/volume admin
	mux.HandleFunc("GET /api/admin/containers", server.handleContainerList)
	mux.HandleFunc("GET /api/admin/containers/{id}", server.handleContainerInspect)
	mux.HandleFunc("GET /api/admin/containers/{id}/logs", server.handleContainerLogs)
	mux.HandleFunc("POST /api/admin/containers/{id}/start", server.handleContainerStart)
	mux.HandleFunc("POST /api/admin/containers/{id}/stop", server.handleContainerStop)
	mux.HandleFunc("POST /api/admin/containers/{id}/restart", server.handleContainerRestart)
	mux.HandleFunc("DELETE /api/admin/containers/{id}", server.handleContainerDelete)
	mux.HandleFunc("POST /api/admin/containers/{id}/exec", server.handleContainerExec)
	mux.HandleFunc("GET /api/admin/containers/{id}/top", server.handleContainerTop)
	mux.HandleFunc("GET /api/admin/containers/{id}/changes", server.handleContainerChanges)
	mux.HandleFunc("GET /api/admin/containers/{id}/stats", server.handleContainerStats)
	mux.HandleFunc("GET /api/admin/containers/{id}/files", server.handleContainerFilesList)
	mux.HandleFunc("POST /api/admin/containers/{id}/files/read", server.handleContainerFilesRead)
	mux.HandleFunc("POST /api/admin/containers/{id}/files/upload", server.handleContainerFilesUpload)
	mux.HandleFunc("POST /api/admin/containers/{id}/files/delete", server.handleContainerFilesDelete)
	mux.HandleFunc("GET /api/admin/images", server.handleImageList)
	mux.HandleFunc("POST /api/admin/images/build", server.handleImageBuild)
	mux.HandleFunc("POST /api/admin/images/pull", server.handleImagePull)
	mux.HandleFunc("DELETE /api/admin/images/{id}", server.handleImageDelete)
	mux.HandleFunc("POST /api/admin/images/{id}/tag", server.handleImageTag)
	mux.HandleFunc("POST /api/admin/images/prune", server.handleImagePrune)
	mux.HandleFunc("GET /api/admin/images/search", server.handleImageSearch)
	mux.HandleFunc("GET /api/admin/networks", server.handleNetworkList)
	mux.HandleFunc("GET /api/admin/networks/{id}", server.handleNetworkInspect)
	mux.HandleFunc("GET /api/admin/volumes", server.handleVolumeList)
	mux.HandleFunc("GET /api/admin/volumes/{id}", server.handleVolumeInspect)
	mux.HandleFunc("GET /api/admin/volumes/usage", server.handleVolumeUsage)

	// Host system info endpoints (v1 API)
	mux.HandleFunc("GET /v1/host/info", server.handleHostInfo)
	mux.HandleFunc("GET /v1/host/disk", server.handleHostDisk)
	mux.HandleFunc("GET /v1/host/memory", server.handleHostMemory)
	mux.HandleFunc("GET /v1/host/network", server.handleHostNetwork)
	mux.HandleFunc("GET /v1/host/processes", server.handleHostProcesses)

	// Firewall management endpoints (v1 API)
	mux.HandleFunc("GET /v1/firewall/status", server.handleFirewallStatus)
	mux.HandleFunc("POST /v1/firewall/enable", server.handleFirewallEnable)
	mux.HandleFunc("POST /v1/firewall/disable", server.handleFirewallDisable)
	mux.HandleFunc("GET /v1/firewall/rules", server.handleFirewallListRules)
	mux.HandleFunc("POST /v1/firewall/rules", server.handleFirewallAddRule)
	mux.HandleFunc("DELETE /v1/firewall/rules/{id}", server.handleFirewallDeleteRule)
	mux.HandleFunc("PUT /v1/firewall/rules/{id}", server.handleFirewallUpdateRule)
	mux.HandleFunc("POST /v1/firewall/port", server.handleFirewallPort)
	mux.HandleFunc("GET /v1/firewall/forward", server.handleFirewallListForwards)
	mux.HandleFunc("POST /v1/firewall/forward", server.handleFirewallAddForward)
	mux.HandleFunc("DELETE /v1/firewall/forward/{id}", server.handleFirewallDeleteForward)

	return server, requestTimeout(server.authenticate(mux))
}

// ReconstructServer restores a panel-returned server into the in-memory
// manager without creating, starting, stopping, or deleting its container.
func (s *Server) ReconstructServer(ctx context.Context, reconstruction Reconstruction) error {
	if err := serverid.Validate(reconstruction.ServerID); err != nil {
		return err
	}
	if reconstruction.RootDir == "" {
		return errors.New("server root directory is required")
	}
	fsys, err := s.serverFilesystem(reconstruction.ServerID, true)
	if err != nil {
		return err
	}
	defer fsys.Close()
	if filepath.Clean(reconstruction.RootDir) != filepath.Clean(fsys.Root()) {
		return errors.New("server root does not match canonical server directory")
	}
	reconstruction.RootDir = fsys.Root()
	return s.manager.Reconcile(ctx, reconstruction)
}

// Shutdown cancels runtime event watchers and all per-server console producers.
func (s *Server) Shutdown() {
	if s == nil {
		return
	}
	s.cancel()
	if s.operations != nil {
		s.operations.Shutdown()
	}
	s.consoles.Close()
	s.eventBus.Destroy()
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "daemon", "runtime": s.runtime != nil})
}

func (s *Server) sessions() *sessionRegistry { return s.sessionsReg }

// TrackSession registers non-WebSocket transports (notably SFTP) in the same
// deauthorization registry used by daemon sessions.
func (s *Server) TrackSession(userID, serverID string, closer io.Closer) func() {
	return s.sessionsReg.trackExternal(userID, serverID, closer)
}

func (s *Server) trackWebSocket(r *http.Request, conn *websocket.Conn) func() {
	userID := strings.TrimSpace(r.Header.Get("X-Panel-User-ID"))
	if userID == "" {
		userID = strings.TrimSpace(r.URL.Query().Get("user"))
	}
	if userID == "" {
		return func() {}
	}
	s.sessionsReg.track(userID, r.PathValue("id"), conn)
	return func() { s.sessionsReg.untrack(conn) }
}

func (s *Server) dockerStatus() string {
	if s.runtime == nil {
		return "error"
	}
	return s.dockerState
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	var mem stdruntime.MemStats
	stdruntime.ReadMemStats(&mem)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte("# HELP game_panel_daemon_uptime_seconds Daemon process uptime.\n"))
	_, _ = w.Write([]byte("# TYPE game_panel_daemon_uptime_seconds gauge\n"))
	_, _ = w.Write([]byte("game_panel_daemon_uptime_seconds " + formatFloat(time.Since(s.started).Seconds()) + "\n"))
	_, _ = w.Write([]byte("# HELP game_panel_daemon_runtime_enabled Docker runtime availability, 1 when enabled.\n"))
	_, _ = w.Write([]byte("# TYPE game_panel_daemon_runtime_enabled gauge\n"))
	runtimeEnabled := "0"
	if s.runtime != nil {
		runtimeEnabled = "1"
	}
	_, _ = w.Write([]byte("game_panel_daemon_runtime_enabled " + runtimeEnabled + "\n"))
	_, _ = w.Write([]byte("# HELP game_panel_daemon_goroutines Current goroutine count.\n"))
	_, _ = w.Write([]byte("# TYPE game_panel_daemon_goroutines gauge\n"))
	_, _ = w.Write([]byte("game_panel_daemon_goroutines " + formatInt(stdruntime.NumGoroutine()) + "\n"))
	_, _ = w.Write([]byte("# HELP game_panel_daemon_memory_alloc_bytes Current Go heap allocation.\n"))
	_, _ = w.Write([]byte("# TYPE game_panel_daemon_memory_alloc_bytes gauge\n"))
	_, _ = w.Write([]byte("game_panel_daemon_memory_alloc_bytes " + formatUint(mem.Alloc) + "\n"))
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		http.Error(w, errRuntimeUnavailable.Error(), http.StatusServiceUnavailable)
		return
	}
	var body struct {
		ServerID string   `json:"serverId"`
		Image    string   `json:"image"`
		Command  []string `json:"command"`
		Env      []string `json:"env"`
		Ports    []struct {
			HostIP        string `json:"hostIp"`
			HostPort      int    `json:"hostPort"`
			ContainerPort int    `json:"containerPort"`
			Protocol      string `json:"protocol"`
		} `json:"ports"`
		Mounts          []mountConfiguration  `json:"mounts"`
		MemoryMB        int64                 `json:"memoryMb"`
		MemoryOverhead  float64               `json:"memoryOverhead"`
		SwapMB          int64                 `json:"swapMb"`
		CPUShares       int64                 `json:"cpuShares"`
		CPUPercent      int64                 `json:"cpuPercent"`
		CPUSet          string                `json:"cpuSet"`
		IOWeight        int64                 `json:"ioWeight"`
		OOMKillDisabled bool                  `json:"oomKillDisabled"`
		PIDLimit        int64                 `json:"pidLimit"`
		StopSignal      string                `json:"stopSignal"`
		StopTimeout     int64                 `json:"stopTimeoutSeconds"`
		UID             int                   `json:"uid"`
		GID             int                   `json:"gid"`
		DNS             []string              `json:"dns"`
		NetworkName     string                `json:"networkName"`
		NetworkSubnet   string                `json:"networkSubnet"`
		NetworkGateway  string                `json:"networkGateway"`
		NetworkIP       string                `json:"networkIp"`
		RegistryAuth    *runtime.RegistryAuth `json:"registryAuth"`
		DiskMB          int64                 `json:"diskMb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if body.ServerID == "" || body.Image == "" {
		http.Error(w, "serverId and image are required", http.StatusBadRequest)
		return
	}
	if err := serverid.Validate(body.ServerID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	rootDir, err := s.safePath(body.ServerID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(rootDir, 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ports := make([]runtime.PortBinding, 0, len(body.Ports))
	for _, port := range body.Ports {
		ports = append(ports, runtime.PortBinding{
			HostIP:        port.HostIP,
			HostPort:      port.HostPort,
			ContainerPort: port.ContainerPort,
			Protocol:      port.Protocol,
		})
	}
	mounts, err := s.runtimeMounts(body.Mounts)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	memoryOverhead := body.MemoryOverhead
	if memoryOverhead <= 0 {
		memoryOverhead = 10.0
	}
	createRequest := runtime.CreateRequest{
		ServerID:       body.ServerID,
		Image:          body.Image,
		Command:        body.Command,
		Env:            s.effectiveEnvList(body.ServerID, body.Env),
		Ports:          ports,
		Mounts:         mounts,
		MemoryMB:       body.MemoryMB,
		MemoryOverhead: memoryOverhead,
		SwapMB:         body.SwapMB, CPUShares: body.CPUShares, CPUPercent: body.CPUPercent,
		CPUSet: body.CPUSet, IOWeight: body.IOWeight, OOMKillDisabled: body.OOMKillDisabled, PIDLimit: body.PIDLimit,
		StopSignal: body.StopSignal, StopTimeout: time.Duration(body.StopTimeout) * time.Second, UID: body.UID, GID: body.GID,
		DNS: body.DNS, NetworkName: body.NetworkName, NetworkSubnet: body.NetworkSubnet, NetworkGateway: body.NetworkGateway,
		NetworkIP: body.NetworkIP, RegistryAuth: body.RegistryAuth, RootDir: rootDir,
	}
	err = s.runtime.Create(r.Context(), createRequest)
	if err != nil {
		http.Error(w, err.Error(), runtimeErrorStatus(err, http.StatusConflict))
		return
	}

	createRequest.RegistryAuth = nil
	if err := s.persistRuntimeRequest(body.ServerID, createRequest); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.manager.MarkCreated(body.ServerID, rootDir, body.DiskMB)
	writeJSON(w, http.StatusAccepted, map[string]any{"serverId": body.ServerID, "accepted": true, "mode": "docker"})
}

func (s *Server) syncConfiguration(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(r.Body, 2*1024*1024)).Decode(&payload); err != nil {
		http.Error(w, "invalid configuration", http.StatusBadRequest)
		return
	}
	configPath, err := s.safePath(serverID, filepath.Join(".config", "server.json"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.WriteFile(configPath, body, 0o640); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.applyConfigurationFiles(serverID, payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if s.runtime != nil {
		if desired, ok, err := s.runtimeRequestFromConfiguration(serverID, payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		} else if ok {
			if err := s.reconcileRuntimeConfiguration(r.Context(), desired); err != nil {
				http.Error(w, err.Error(), runtimeErrorStatus(err, http.StatusConflict))
				return
			}
			if err := s.persistRuntimeRequest(serverID, desired); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	s.manager.UpdateRuntimeConfig(serverID, memoryMBFromConfiguration(payload), allocationIPFromConfiguration(payload), allocationPortFromConfiguration(payload), stopTypeFromConfiguration(payload), stopValueFromConfiguration(payload), stopTimeoutFromConfiguration(payload))
	s.manager.MarkConfigurationSynced(serverID, diskLimitMBFromConfiguration(payload))
	writeJSON(w, http.StatusOK, map[string]any{"serverId": serverID, "synced": true})
}

func (s *Server) persistRuntimeRequest(serverID string, req runtime.CreateRequest) error {
	path, err := s.safePath(serverID, filepath.Join(".config", "runtime.json"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	req.RegistryAuth = nil
	body, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}

func (s *Server) runtimeRequestFromConfiguration(serverID string, payload map[string]any) (runtime.CreateRequest, bool, error) {
	path, err := s.safePath(serverID, filepath.Join(".config", "runtime.json"))
	if err != nil {
		return runtime.CreateRequest{}, false, err
	}
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return runtime.CreateRequest{}, false, nil
	}
	if err != nil {
		return runtime.CreateRequest{}, false, err
	}
	var req runtime.CreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return runtime.CreateRequest{}, false, err
	}
	if image, ok := payload["dockerImage"].(string); ok && strings.TrimSpace(image) != "" {
		req.Image = image
	}
	if invocation, ok := payload["invocation"].(string); ok {
		req.Command = []string{"/bin/sh", "-lc", invocation}
	}
	if environment, ok := payload["environment"].(map[string]any); ok {
		req.Env = req.Env[:0]
		keys := make([]string, 0, len(environment))
		for key := range environment {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			req.Env = append(req.Env, key+"="+fmt.Sprint(environment[key]))
		}
	}
	build, _ := payload["build"].(map[string]any)
	req.MemoryMB = int64Value(build, "memoryLimit", "memory_limit")
	req.SwapMB = int64Value(build, "swapMb", "swap")
	req.CPUShares = int64Value(build, "cpuShares", "cpu_shares")
	req.CPUPercent = int64Value(build, "cpuLimit", "cpu_limit")
	if threads, ok := firstMapValue(build, "threads").(string); ok {
		req.CPUSet = threads
	}
	if value := int64Value(build, "ioWeight", "io_weight"); value != 0 {
		req.IOWeight = value
	}
	if value, ok := firstMapValue(build, "oomDisabled", "oom_disabled").(bool); ok {
		req.OOMKillDisabled = value
	}
	allocations, _ := payload["allocations"].(map[string]any)
	ports := make([]runtime.PortBinding, 0)
	if detailed, ok := allocations["ports"].([]any); ok {
		for _, raw := range detailed {
			entry, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			hostPort := int(anyInt64(firstMapValue(entry, "port", "hostPort")))
			containerPort := int(anyInt64(firstMapValue(entry, "containerPort")))
			if containerPort == 0 {
				containerPort = hostPort
			}
			protocol, _ := firstMapValue(entry, "protocol").(string)
			if protocol == "" {
				protocol = "tcp"
			}
			hostIP, _ := firstMapValue(entry, "ip", "hostIP").(string)
			ports = append(ports, runtime.PortBinding{HostIP: hostIP, HostPort: hostPort, ContainerPort: containerPort, Protocol: protocol})
		}
	}
	if len(ports) == 0 {
		mappings, _ := allocations["mappings"].(map[string]any)
		for ip, raw := range mappings {
			if values, ok := raw.([]any); ok {
				for _, value := range values {
					port := int(anyInt64(value))
					ports = append(ports, runtime.PortBinding{HostIP: ip, HostPort: port, ContainerPort: port, Protocol: "tcp"})
				}
			}
		}
	}
	if len(ports) > 0 {
		req.Ports = ports
	}
	mounts, err := s.runtimeMountsFromConfiguration(payload)
	if err != nil {
		return runtime.CreateRequest{}, false, err
	}
	if mounts != nil {
		req.Mounts = mounts
	}
	return req, true, nil
}

func firstMapValue(values map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value
		}
	}
	return nil
}
func anyInt64(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	case json.Number:
		result, _ := typed.Int64()
		return result
	}
	return 0
}
func int64Value(values map[string]any, keys ...string) int64 {
	return anyInt64(firstMapValue(values, keys...))
}

func (s *Server) getConfiguration(w http.ResponseWriter, r *http.Request) {
	configPath, err := s.safePath(r.PathValue("id"), filepath.Join(".config", "server.json"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	body, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "configuration not synced", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

func (s *Server) install(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		http.Error(w, errRuntimeUnavailable.Error(), http.StatusServiceUnavailable)
		return
	}
	serverID := r.PathValue("id")
	var body struct {
		ServerID   string            `json:"serverId"`
		Image      string            `json:"image"`
		Entrypoint string            `json:"entrypoint"`
		Script     string            `json:"script"`
		Env        map[string]string `json:"env"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2*1024*1024)).Decode(&body); err != nil {
		http.Error(w, "invalid install request", http.StatusBadRequest)
		return
	}
	if body.ServerID != "" && body.ServerID != serverID {
		http.Error(w, "server id mismatch", http.StatusBadRequest)
		return
	}
	rootDir, err := s.safePath(serverID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(rootDir, 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	installDir, err := s.safePath(serverID, ".install")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(installDir, 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	scriptPath := filepath.Join(installDir, "install.sh")
	script := body.Script
	if strings.TrimSpace(script) == "" {
		script = "#!/bin/sh\nset -eu\necho \"No install script configured.\"\n"
	}
	if err := os.WriteFile(scriptPath, []byte(script), 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	env := s.effectiveEnvMapList(serverID, body.Env)

	// Mark as installing
	s.manager.MarkInstalling(serverID, true)

	// Execute installation
	result, err := s.runtime.Install(r.Context(), runtime.InstallRequest{
		ServerID:   serverID,
		Image:      body.Image,
		Entrypoint: body.Entrypoint,
		Script:     script,
		Env:        env,
		RootDir:    rootDir,
	})
	if err != nil {
		s.manager.MarkInstalling(serverID, false)
		s.notifyPanelInstallStatus(serverID, false, err.Error())
		http.Error(w, err.Error(), runtimeErrorStatus(err, http.StatusConflict))
		return
	}

	// Save logs
	logPath := filepath.Join(installDir, "install.log")
	_ = os.WriteFile(logPath, []byte(result.Logs), 0o640)

	// Mark installation complete
	s.manager.MarkInstalling(serverID, false)

	// Notify Panel of installation status
	success := result.ExitCode == 0
	errorMsg := ""
	if !success {
		errorMsg = "install script failed with exit code " + strconv.Itoa(result.ExitCode)
	}
	s.notifyPanelInstallStatus(serverID, success, errorMsg)

	if result.ExitCode != 0 {
		http.Error(w, "install script failed with exit code "+strconv.Itoa(result.ExitCode), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"serverId": serverID, "accepted": true, "mode": "docker", "exitCode": result.ExitCode, "logs": result.Logs})
}

func (s *Server) installWS(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		http.Error(w, errRuntimeUnavailable.Error(), http.StatusServiceUnavailable)
		return
	}

	// Authenticate WebSocket connection
	claims, err := s.authenticateWebSocket(w, r)
	if err != nil {
		http.Error(w, "authentication required: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Validate token scope for install operations
	if claims.Scope != tokens.ScopeWebsocket {
		http.Error(w, "invalid token scope for install websocket connection", http.StatusForbidden)
		return
	}

	conn, err := websocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	defer s.trackWebSocket(r, conn)()

	serverID := r.PathValue("id")

	// Validate that the token is for this specific server
	if claims.ServerID != serverID {
		conn.WriteJSON(map[string]interface{}{
			"type": "error",
			"data": "token not valid for this server",
		})
		return
	}

	// Send initial status
	conn.WriteJSON(map[string]interface{}{
		"type": "status",
		"data": "Starting installation...",
	})

	// Read installation request from WebSocket
	var body struct {
		Image      string            `json:"image"`
		Entrypoint string            `json:"entrypoint"`
		Script     string            `json:"script"`
		Env        map[string]string `json:"env"`
	}
	if err := conn.ReadJSON(&body); err != nil {
		conn.WriteJSON(map[string]interface{}{
			"type":  "error",
			"data":  "Invalid install request",
			"error": err.Error(),
		})
		return
	}

	rootDir, err := s.safePath(serverID, "")
	if err != nil {
		conn.WriteJSON(map[string]interface{}{
			"type": "error",
			"data": err.Error(),
		})
		return
	}

	installDir, err := s.safePath(serverID, ".install")
	if err != nil {
		conn.WriteJSON(map[string]interface{}{
			"type": "error",
			"data": err.Error(),
		})
		return
	}

	if err := os.MkdirAll(installDir, 0o750); err != nil {
		conn.WriteJSON(map[string]interface{}{
			"type": "error",
			"data": err.Error(),
		})
		return
	}

	scriptPath := filepath.Join(installDir, "install.sh")
	script := body.Script
	if strings.TrimSpace(script) == "" {
		script = "#!/bin/sh\nset -eu\necho \"No install script configured.\"\n"
	}

	if err := os.WriteFile(scriptPath, []byte(script), 0o750); err != nil {
		conn.WriteJSON(map[string]interface{}{
			"type": "error",
			"data": err.Error(),
		})
		return
	}

	env := s.effectiveEnvMapList(serverID, body.Env)

	s.manager.MarkInstalling(serverID, true)

	conn.WriteJSON(map[string]interface{}{
		"type": "status",
		"data": "Running install script...",
	})

	result, err := s.runtime.Install(r.Context(), runtime.InstallRequest{
		ServerID:   serverID,
		Image:      body.Image,
		Entrypoint: body.Entrypoint,
		Script:     script,
		Env:        env,
		RootDir:    rootDir,
	})
	if err != nil {
		s.manager.MarkInstalling(serverID, false)
		conn.WriteJSON(map[string]interface{}{
			"type": "error",
			"data": err.Error(),
		})
		s.notifyPanelInstallStatus(serverID, false, err.Error())
		return
	}

	// Stream logs
	for _, line := range strings.Split(result.Logs, "\n") {
		if line != "" {
			conn.WriteJSON(map[string]interface{}{
				"type": "log",
				"data": line,
			})
		}
	}

	// Save logs
	logPath := filepath.Join(installDir, "install.log")
	_ = os.WriteFile(logPath, []byte(result.Logs), 0o640)

	s.manager.MarkInstalling(serverID, false)

	success := result.ExitCode == 0
	errorMsg := ""
	if !success {
		errorMsg = "install script failed with exit code " + strconv.Itoa(result.ExitCode)
	}
	s.notifyPanelInstallStatus(serverID, success, errorMsg)

	conn.WriteJSON(map[string]interface{}{
		"type":     "complete",
		"success":  success,
		"exitCode": result.ExitCode,
		"error":    errorMsg,
	})
}

func (s *Server) reinstall(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")

	// Check if server is running
	state := s.manager.State(serverID)
	if state.PowerState == PowerStateRunning || state.PowerState == PowerStateStarting {
		http.Error(w, "server must be stopped before reinstalling", http.StatusConflict)
		return
	}

	// Forward to install handler (reinstall is just install with server stopped)
	s.install(w, r)
}

// notifyPanelInstallStatus notifies the Panel API of installation completion
func (s *Server) notifyPanelInstallStatus(serverID string, success bool, errorMsg string) {
	if s.panelClient == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.panelClient.SetInstallationStatus(ctx, serverID, success); err != nil {
		log.Printf("[beacon] failed to notify panel of install status for %s: %v", serverID, err)
	}
}

func (s *Server) power(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	var body struct {
		Signal string `json:"signal"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	switch body.Signal {
	case "start", "stop", "restart", "kill":
		if s.runtime == nil {
			http.Error(w, errRuntimeUnavailable.Error(), http.StatusServiceUnavailable)
			return
		}
		commandID := strings.TrimSpace(r.Header.Get("X-Forge-Command-ID"))
		if commandID == "" {
			commandID = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		}
		op, err := s.operations.EnqueueCommand(r.Context(), commandID, serverID, OperationType(body.Signal))
		if err != nil {
			// Return operation ID if available, even on error
			if op != nil {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error(), "operationId": op.ID})
			} else {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
			}
			return
		}
		// Preserve the existing error contract while execution continues on the
		// server context if the caller disconnects.
		// Add timeout to prevent indefinite blocking
		waitCtx, waitCancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer waitCancel()
		for {
			status, statusErr := s.operations.GetStatus(op.ID)
			if statusErr != nil {
				http.Error(w, statusErr.Error(), http.StatusInternalServerError)
				return
			}
			switch status.Status {
			case StatusCompleted:
				writeJSON(w, http.StatusAccepted, map[string]any{"serverId": serverID, "signal": body.Signal, "accepted": true, "mode": "docker", "operationId": op.ID})
				return
			case StatusFailed:
				err := errors.New(status.Error)
				writeJSON(w, runtimeErrorStatus(err, http.StatusConflict), map[string]any{"error": err.Error(), "operationId": op.ID})
				return
			}
			select {
			case <-waitCtx.Done():
				http.Error(w, "operation timeout", http.StatusGatewayTimeout)
				return
			case <-r.Context().Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
		}
	default:
		http.Error(w, "invalid power signal", http.StatusBadRequest)
	}
}

func (s *Server) getOperation(w http.ResponseWriter, r *http.Request) {
	op, err := s.operations.GetStatus(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, op)
}

func (s *Server) listOperations(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.operations.ListByServer(r.PathValue("id")))
}

func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		http.Error(w, errRuntimeUnavailable.Error(), http.StatusServiceUnavailable)
		return
	}
	serverID := r.PathValue("id")
	s.consoles.Stop(serverID)
	if err := s.runtime.Delete(r.Context(), serverID); err != nil {
		http.Error(w, err.Error(), runtimeErrorStatus(err, http.StatusConflict))
		return
	}
	s.manager.Delete(serverID)
	writeJSON(w, http.StatusAccepted, map[string]any{"serverId": serverID, "signal": "delete", "accepted": true, "mode": "docker"})
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		http.Error(w, errRuntimeUnavailable.Error(), http.StatusServiceUnavailable)
		return
	}
	stats, err := s.runtime.Stats(r.Context(), r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), runtimeErrorStatus(err, http.StatusConflict))
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) logs(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		http.Error(w, errRuntimeUnavailable.Error(), http.StatusServiceUnavailable)
		return
	}
	reader, err := s.runtime.Logs(r.Context(), r.PathValue("id"))
	if err != nil {
		if isContainerMissing(err) {
			http.Error(w, "container not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = stdcopy.StdCopy(w, w, io.LimitReader(reader, 256*1024))
}

func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		http.Error(w, "backup adapter unavailable", http.StatusServiceUnavailable)
		return
	}
	serverID := r.PathValue("id")
	root, err := s.safePath(serverID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := "backup-" + time.Now().UTC().Format("20060102T150405.000000000Z") + ".zip"

	// Read .pteroignore without following links outside the canonical server root.
	var ignored []string
	if fsys, fsErr := rootfs.New(root); fsErr == nil {
		if ignoreFile, openErr := fsys.Open(".pteroignore"); openErr == nil {
			if denylist, parseErr := ignore.LoadIgnoreReader(ignoreFile); parseErr == nil {
				ignored = denylist.Patterns()
			}
			_ = ignoreFile.Close()
		}
		_ = fsys.Close()
	}

	var reqBody struct {
		IgnoredFiles []string `json:"ignored_files"`
	}
	if r.Body != nil && r.Header.Get("Content-Type") == "application/json" {
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
	}
	if len(reqBody.IgnoredFiles) > 0 {
		ignored = append(ignored, reqBody.IgnoredFiles...)
	}

	s.backupMu.Lock()
	if s.eventBus != nil {
		s.backups.SetProgressCallback(func(p backup.BackupProgress) {
			s.eventBus.Publish(BackupProgressEvent+":"+serverID, p)
		})
	}

	info, err := s.backups.Create(r.Context(), root, serverID, name, ignored)
	s.backupMu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), backupErrorStatus(err))
		return
	}

	if s.panelClient != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = s.panelClient.SendBackupStatus(ctx, serverID, remote.BackupStatusRequest{
				BackupUUID: info.UUID,
				ServerUUID: serverID,
				Checksum:   info.Checksum,
				Size:       info.Size,
				Successful: true,
			})
		}()
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"uuid":         info.UUID,
		"name":         info.Name,
		"checksum":     info.Checksum,
		"size":         info.Size,
		"status":       info.Status,
		"created":      info.Created.Format(time.RFC3339),
		"completedAt":  info.CompletedAt.Format(time.RFC3339),
		"adapter":      info.Adapter,
		"ignoredFiles": info.IgnoredFiles,
	})
}

func (s *Server) listBackups(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		http.Error(w, "backup adapter unavailable", http.StatusServiceUnavailable)
		return
	}
	serverID := r.PathValue("id")
	if _, err := s.safePath(serverID, ""); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	backups, err := s.backups.List(serverID)
	if err != nil {
		http.Error(w, err.Error(), backupErrorStatus(err))
		return
	}
	writeJSON(w, http.StatusOK, backups)
}

func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		http.Error(w, "backup adapter unavailable", http.StatusServiceUnavailable)
		return
	}
	name := r.URL.Query().Get("name")
	if !safeBackupName(name) {
		http.Error(w, "invalid backup name", http.StatusBadRequest)
		return
	}
	serverID := r.PathValue("id")
	if _, err := s.safePath(serverID, ""); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	reader, err := s.backups.Download(serverID, name)
	if err != nil {
		http.Error(w, err.Error(), backupErrorStatus(err))
		return
	}
	defer reader.Close()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = io.Copy(w, reader)
}

func (s *Server) downloadBackupWithToken(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		http.Error(w, "backup adapter unavailable", http.StatusServiceUnavailable)
		return
	}
	if s.tokenGenerator == nil {
		http.Error(w, "token generator not configured", http.StatusInternalServerError)
		return
	}
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		http.Error(w, "token is required", http.StatusBadRequest)
		return
	}
	claims, err := s.tokenGenerator.Validate(tokenStr)
	if err != nil {
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}
	if claims.Scope != tokens.ScopeBackupDownload {
		http.Error(w, "invalid token scope", http.StatusForbidden)
		return
	}
	serverID := claims.ServerID
	backupUUID := claims.BackupID
	if backupUUID == "" {
		http.Error(w, "invalid token: missing backup id", http.StatusBadRequest)
		return
	}
	name := backupUUID + ".zip"
	if !safeBackupName(name) {
		http.Error(w, "invalid backup name", http.StatusBadRequest)
		return
	}
	if _, err := s.safePath(serverID, ""); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	reader, err := s.backups.Download(serverID, name)
	if err != nil {
		http.Error(w, err.Error(), backupErrorStatus(err))
		return
	}
	defer reader.Close()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = io.Copy(w, reader)
}

func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		http.Error(w, "backup adapter unavailable", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Name     string `json:"name"`
		Truncate bool   `json:"truncate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if !safeBackupName(body.Name) {
		http.Error(w, "invalid backup name", http.StatusBadRequest)
		return
	}
	serverID := r.PathValue("id")
	root, err := s.safePath(serverID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.backups.Restore(r.Context(), serverID, body.Name, root, body.Truncate); err != nil {
		if s.panelClient != nil {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_ = s.panelClient.SendRestoreStatus(ctx, serverID, remote.RestoreStatusRequest{
					BackupUUID: strings.TrimSuffix(body.Name, ".zip"),
					ServerUUID: serverID,
					Successful: false,
					Error:      err.Error(),
				})
			}()
		}
		http.Error(w, err.Error(), backupErrorStatus(err))
		return
	}

	if s.panelClient != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = s.panelClient.SendRestoreStatus(ctx, serverID, remote.RestoreStatusRequest{
				BackupUUID: strings.TrimSuffix(body.Name, ".zip"),
				ServerUUID: serverID,
				Successful: true,
			})
		}()
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "name": body.Name, "status": "restored"})
}

func (s *Server) deleteBackup(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		http.Error(w, "backup adapter unavailable", http.StatusServiceUnavailable)
		return
	}
	// Support both path-value (/backups/{backupId}) and legacy query-param (?name=).
	name := r.PathValue("backupId")
	if name == "" {
		name = r.URL.Query().Get("name")
	}
	if !safeBackupName(name) {
		http.Error(w, "invalid backup name", http.StatusBadRequest)
		return
	}
	serverID := r.PathValue("id")
	if _, err := s.safePath(serverID, ""); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.backups.Delete(serverID, name); err != nil {
		http.Error(w, err.Error(), backupErrorStatus(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) statsWS(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		http.Error(w, errRuntimeUnavailable.Error(), http.StatusServiceUnavailable)
		return
	}

	// Authenticate WebSocket connection
	claims, err := s.authenticateWebSocket(w, r)
	if err != nil {
		http.Error(w, "authentication required: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Validate token scope for websocket connections
	if claims.Scope != tokens.ScopeWebsocket {
		http.Error(w, "invalid token scope for websocket connection", http.StatusForbidden)
		return
	}

	conn, err := websocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	defer s.trackWebSocket(r, conn)()
	configureWebSocket(conn)
	writer := &webSocketWriter{conn: conn}
	done := make(chan struct{})
	defer close(done)
	go pingWebSocket(writer, done)

	serverID := r.PathValue("id")

	// Validate that the token is for this specific server
	if claims.ServerID != serverID {
		writeJSONError(writer, serverID, errors.New("token not valid for this server"))
		return
	}
	stream, err := s.runtime.StatsStream(r.Context(), serverID)
	if err != nil {
		writeJSONError(writer, serverID, err)
		return
	}
	defer stream.Close()

	go func() {
		<-r.Context().Done()
		_ = stream.Close()
	}()

	for {
		runtimeStats, err := runtime.DecodeDockerStats(stream)
		if err != nil {
			return
		}
		stats := map[string]any{
			"serverId":       serverID,
			"cpuPercent":     runtimeStats.CPUPercent,
			"memoryBytes":    runtimeStats.MemoryBytes,
			"memoryLimit":    runtimeStats.MemoryLimit,
			"networkRxBytes": runtimeStats.NetworkRxBytes,
			"networkTxBytes": runtimeStats.NetworkTxBytes,
		}
		if err := writer.WriteJSON(stats); err != nil {
			return
		}
	}
}

func (s *Server) logsWS(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		http.Error(w, errRuntimeUnavailable.Error(), http.StatusServiceUnavailable)
		return
	}

	// Authenticate WebSocket connection
	claims, err := s.authenticateWebSocket(w, r)
	if err != nil {
		http.Error(w, "authentication required: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Validate token scope for websocket connections
	if claims.Scope != tokens.ScopeWebsocket {
		http.Error(w, "invalid token scope for websocket connection", http.StatusForbidden)
		return
	}

	conn, err := websocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	defer s.trackWebSocket(r, conn)()
	configureWebSocket(conn)
	writer := &webSocketWriter{conn: conn}
	done := make(chan struct{})
	defer close(done)
	go pingWebSocket(writer, done)

	serverID := r.PathValue("id")

	// Validate that the token is for this specific server
	if claims.ServerID != serverID {
		writeJSONError(writer, serverID, errors.New("token not valid for this server"))
		return
	}
	stream, err := s.runtime.LogsStream(r.Context(), serverID, "100")
	if err != nil {
		writeJSONError(writer, serverID, err)
		return
	}
	defer stream.Close()

	go func() {
		<-r.Context().Done()
		_ = stream.Close()
	}()

	logWriter := &wsLogWriter{writer: writer, serverID: serverID}
	_, _ = stdcopy.StdCopy(logWriter, logWriter, stream)
}

type wsLogWriter struct {
	writer   *webSocketWriter
	serverID string
}

func (w *wsLogWriter) Write(p []byte) (n int, err error) {
	payload := map[string]any{
		"serverId": w.serverID,
		"logs":     string(p),
	}
	if err := w.writer.WriteJSON(payload); err != nil {
		return 0, err
	}
	return len(p), nil
}

func writeJSONError(writer *webSocketWriter, serverID string, err error) {
	_ = writer.WriteJSON(map[string]any{
		"serverId": serverID,
		"error":    err.Error(),
	})
}

func (s *Server) consoleWS(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		http.Error(w, errRuntimeUnavailable.Error(), http.StatusServiceUnavailable)
		return
	}

	// Authenticate WebSocket connection
	claims, err := s.authenticateWebSocket(w, r)
	if err != nil {
		http.Error(w, "authentication required: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Validate token scope for websocket connections
	if claims.Scope != tokens.ScopeWebsocket {
		http.Error(w, "invalid token scope for websocket connection", http.StatusForbidden)
		return
	}

	conn, err := websocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	defer s.trackWebSocket(r, conn)()
	configureWebSocket(conn)
	writer := &webSocketWriter{conn: conn}
	done := make(chan struct{})
	defer close(done)
	go pingWebSocket(writer, done)

	serverID := r.PathValue("id")

	// Validate that the token is for this specific server
	if claims.ServerID != serverID {
		_ = writer.WriteJSON(map[string]any{"type": "error", "data": "token not valid for this server"})
		return
	}

	// The manager owns one attach per running server. Every websocket receives
	// only this server's bounded replay and live output.
	if err := s.consoles.Ensure(serverID); err != nil {
		_ = writer.WriteJSON(map[string]any{"type": "error", "data": err.Error()})
		return
	}
	ch, unsubscribe, err := s.consoles.Subscribe(serverID)
	if err != nil {
		_ = writer.WriteJSON(map[string]any{"type": "error", "data": err.Error()})
		return
	}
	defer unsubscribe()

	errs := make(chan error, 2)

	go func() {
		for msg := range ch {
			if err := writer.WriteJSON(map[string]any{"type": "output", "data": string(msg)}); err != nil {
				errs <- err
				return
			}
		}
		errs <- nil
	}()

	// Read commands from the WebSocket and forward to Docker.
	go func() {
		for {
			select {
			case <-r.Context().Done():
				errs <- r.Context().Err()
				return
			default:
				conn.SetReadDeadline(time.Now().Add(30 * time.Second))
				messageType, payload, err := conn.ReadMessage()
				if err != nil {
					errs <- err
					return
				}
				if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
					continue
				}
				cmd := strings.TrimSpace(string(payload))
				if cmd == "" {
					continue
				}
				if err := s.consoles.Write(serverID, cmd); err != nil {
					_ = writer.WriteJSON(map[string]any{"type": "error", "data": err.Error()})
				}
			}
		}
	}()
	<-errs
}

func (s *Server) backupProgressWS(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		http.Error(w, "backup adapter unavailable", http.StatusServiceUnavailable)
		return
	}

	// Authenticate WebSocket connection
	claims, err := s.authenticateWebSocket(w, r)
	if err != nil {
		http.Error(w, "authentication required: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Validate token scope for backup operations
	if claims.Scope != tokens.ScopeWebsocket && claims.Scope != tokens.ScopeBackupDownload {
		http.Error(w, "invalid token scope for backup websocket connection", http.StatusForbidden)
		return
	}

	conn, err := websocketUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	defer s.trackWebSocket(r, conn)()
	configureWebSocket(conn)
	writer := &webSocketWriter{conn: conn}

	serverID := r.PathValue("id")
	// Validate that the token is for this specific server
	if claims.ServerID != serverID {
		_ = writer.WriteJSON(map[string]any{"type": "error", "data": "token not valid for this server"})
		return
	}
	done := make(chan struct{})
	defer close(done)
	go pingWebSocket(writer, done)
	ch := s.eventBus.Subscribe(BackupProgressEvent + ":" + serverID)
	defer s.eventBus.Unsubscribe(BackupProgressEvent+":"+serverID, ch)

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if _, err := writer.Write(msg); err != nil {
				return
			}
		}
	}
}

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	fsys, err := s.serverFilesystem(serverID, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	directory, err := rootfs.Clean(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	entries, err := fsys.ReadDir(directory)
	if err != nil {
		status := http.StatusNotFound
		if errors.Is(err, rootfs.ErrSymlink) {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	denylist := ignore.NewIgnoreList(nil)
	if ignoreFile, err := fsys.Open(".pteroignore"); err == nil {
		if parsed, parseErr := ignore.LoadIgnoreReader(ignoreFile); parseErr == nil {
			denylist = parsed
		}
		_ = ignoreFile.Close()
	}
	files := []map[string]any{}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 || denylist.IsIgnored(path.Join(directory, entry.Name())) {
			continue
		}
		mode := fmt.Sprintf("%04o", info.Mode().Perm())
		// Determine if file is editable (text file under 1MB)
		isEditable := !entry.IsDir() && info.Size() < 1_048_576 && isTextFile(path.Join(directory, entry.Name()))
		files = append(files, map[string]any{
			"name": entry.Name(), "path": path.Join(directory, entry.Name()),
			"directory": entry.IsDir(), "size": info.Size(),
			"modTime": info.ModTime().UTC().Format(time.RFC3339),
			"mode":    mode, "is_editable": isEditable,
		})
	}
	writeJSON(w, http.StatusOK, files)
}

// isTextFile determines if a file is likely a text file based on extension
func isTextFile(filePath string) bool {
	textExtensions := map[string]bool{
		".txt": true, ".md": true, ".json": true, ".xml": true, ".yaml": true, ".yml": true,
		".html": true, ".htm": true, ".css": true, ".js": true, ".ts": true, ".tsx": true,
		".jsx": true, ".vue": true, ".svelte": true, ".py": true, ".rb": true, ".php": true,
		".sh": true, ".bash": true, ".zsh": true, ".fish": true, ".ps1": true, ".bat": true,
		".cmd": true, ".ini": true, ".cfg": true, ".conf": true, ".config": true,
		".env": true, ".log": true, ".sql": true, ".db": true, ".sqlite": true,
		".go": true, ".rs": true, ".c": true, ".cpp": true, ".h": true, ".hpp": true,
		".java": true, ".kt": true, ".swift": true, ".dart": true, ".lua": true,
		".perl": true, ".pl": true, ".pm": true, ".tcl": true, ".r": true, ".R": true,
		".scala": true, ".groovy": true, ".kts": true, ".clj": true, ".cljs": true,
		".hs": true, ".lhs": true, ".erl": true, ".hrl": true, ".ex": true, ".exs": true,
		".ml": true, ".mli": true, ".fs": true, ".fsi": true, ".fsx": true,
		".v": true, ".sv": true, ".vhdl": true, ".verilog": true, ".nix": true,
		".toml": true, ".dockerfile": true, ".makefile": true, ".cmake": true,
		".gradle": true, ".properties": true, ".manifest": true, ".lock": true,
		".gitignore": true, ".gitattributes": true, ".gitmodules": true,
		".editorconfig": true, ".eslintrc": true, ".prettierrc": true,
		".babelrc": true, ".tsconfig": true, ".package": true, ".gemfile": true,
	}
	ext := strings.ToLower(path.Ext(filePath))
	if textExtensions[ext] {
		return true
	}
	// Check if file has no extension (common for scripts, configs, etc.)
	if ext == "" {
		return true
	}
	return false
}

func (s *Server) downloadFile(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	fsys, err := s.serverFilesystem(r.PathValue("id"), false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	file, err := fsys.Open(filePath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "path is not a regular file", http.StatusBadRequest)
		return
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(filePath)})
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, err := io.Copy(w, file); err != nil {
		return
	}
}

func (s *Server) readFile(w http.ResponseWriter, r *http.Request) {
	fsys, err := s.serverFilesystem(r.PathValue("id"), false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	file, err := fsys.Open(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "cannot read non-regular file", http.StatusBadRequest)
		return
	}
	if info.Size() > 1024*1024 {
		http.Error(w, "file exceeds read size limit", http.StatusRequestEntityTooLarge)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.Copy(w, file)
}

func (s *Server) writeFile(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	fsys, err := s.serverFilesystem(serverID, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	name, err := rootfs.Clean(r.URL.Query().Get("path"))
	if err != nil || name == "" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	if r.ContentLength > maxFileWriteBytes {
		http.Error(w, "file too large", http.StatusRequestEntityTooLarge)
		return
	}
	reservation := r.ContentLength
	if reservation < 0 {
		reservation = maxFileWriteBytes
	}
	if err := s.manager.HasSpaceForWriteFS(serverID, reservation, fsys); err != nil {
		http.Error(w, err.Error(), http.StatusInsufficientStorage)
		return
	}
	if err := fsys.AtomicWrite(name, r.Body, maxFileWriteBytes, 0o640); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, rootfs.ErrTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

const maxUploadChunkBytes = 8 * 1024 * 1024

func (s *Server) uploadFileChunk(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	uploadID := r.URL.Query().Get("uploadId")
	if !safeUploadID(uploadID) {
		http.Error(w, "invalid upload id", http.StatusBadRequest)
		return
	}
	unlock := lockUpload(serverID, uploadID)
	defer unlock()
	fsys, err := s.serverFilesystem(serverID, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	target, err := rootfs.Clean(r.URL.Query().Get("path"))
	if err != nil || target == "" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	offset, err := parseInt64Query(r, "offset")
	if err != nil || offset < 0 {
		http.Error(w, "invalid offset", http.StatusBadRequest)
		return
	}
	if r.ContentLength > maxUploadChunkBytes {
		http.Error(w, "upload chunk too large", http.StatusRequestEntityTooLarge)
		return
	}
	maxTotal := envBytes("DAEMON_UPLOAD_MAX_BYTES", defaultMaxUploadBytes)
	if offset > maxTotal || (r.ContentLength > 0 && r.ContentLength > maxTotal-offset) {
		http.Error(w, "upload too large", http.StatusRequestEntityTooLarge)
		return
	}
	cleanupExpiredUploads(fsys, time.Now())
	if err := fsys.MkdirAll(".uploads", 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	temp := path.Join(".uploads", uploadID+".part")
	current := int64(0)
	if info, statErr := fsys.Stat(temp); statErr == nil {
		current = info.Size()
	} else if offset != 0 {
		http.Error(w, "upload session not found", http.StatusConflict)
		return
	}
	if current != offset {
		http.Error(w, "upload offset mismatch", http.StatusConflict)
		return
	}
	reservation := r.ContentLength
	if reservation < 0 {
		reservation = maxUploadChunkBytes
	}
	if err := s.manager.HasSpaceForWriteFS(serverID, reservation, fsys); err != nil {
		http.Error(w, err.Error(), http.StatusInsufficientStorage)
		return
	}
	file, err := fsys.OpenFile(temp, os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		file.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	written, copyErr := io.Copy(file, io.LimitReader(r.Body, maxUploadChunkBytes+1))
	if copyErr != nil || written > maxUploadChunkBytes || written > maxTotal-offset {
		_ = file.Truncate(offset)
		_ = file.Close()
		if copyErr != nil {
			http.Error(w, copyErr.Error(), http.StatusInternalServerError)
		} else {
			http.Error(w, "upload chunk or total too large", http.StatusRequestEntityTooLarge)
		}
		return
	}
	if err := file.Sync(); err != nil {
		file.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := file.Close(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	nextOffset := offset + written
	final := r.URL.Query().Get("final") == "true"
	if final {
		if err := fsys.MkdirAll(path.Dir(target), 0o750); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := fsys.Rename(temp, target); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "offset": nextOffset, "final": final})
}

func (s *Server) makeDir(w http.ResponseWriter, r *http.Request) {
	fsys, err := s.serverFilesystem(r.PathValue("id"), true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	if err := fsys.MkdirAll(r.URL.Query().Get("path"), 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

func (s *Server) renameFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	fsys, err := s.serverFilesystem(r.PathValue("id"), false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	to, err := rootfs.Clean(body.To)
	if err != nil || to == "" {
		http.Error(w, "invalid destination", http.StatusBadRequest)
		return
	}
	if err := fsys.MkdirAll(path.Dir(to), 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := fsys.Rename(body.From, to); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) deleteFile(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("path") == "" {
		http.Error(w, "cannot delete server root", http.StatusBadRequest)
		return
	}
	fsys, err := s.serverFilesystem(r.PathValue("id"), false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	if err := fsys.RemoveAll(r.URL.Query().Get("path")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) archiveFiles(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	fsys, err := s.serverFilesystem(serverID, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	source, err := rootfs.Clean(r.URL.Query().Get("path"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	info, err := fsys.Stat(source)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	name := info.Name()
	if source == "" {
		name = serverID
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.tar.gz"`)
	gzipWriter := gzip.NewWriter(w)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := archiveTree(fsys, tarWriter, source, name); err != nil {
		return
	}
	if err := tarWriter.Close(); err != nil {
		return
	}
	_ = gzipWriter.Close()
}

func (s *Server) decompressFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	serverID := r.PathValue("id")
	fsys, err := s.serverFilesystem(serverID, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	archiveName, err := rootfs.Clean(body.Path)
	if err != nil || archiveName == "" {
		http.Error(w, "invalid archive path", http.StatusBadRequest)
		return
	}
	destination := path.Dir(archiveName)
	if destination == "." {
		destination = ""
	}
	if _, err := extractArchive(fsys, archiveName, destination, s.manager, serverID); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(strings.ToLower(err.Error()), "disk") {
			status = http.StatusInsufficientStorage
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true})
}

func (s *Server) batchDeleteFiles(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if len(body.Paths) == 0 || len(body.Paths) > 100 {
		http.Error(w, "between 1 and 100 paths are required", http.StatusBadRequest)
		return
	}
	cleaned := make([]string, len(body.Paths))
	seen := make(map[string]struct{}, len(body.Paths))
	for index, name := range body.Paths {
		value, err := rootfs.Clean(name)
		if err != nil || value == "" {
			http.Error(w, "invalid path in batch", http.StatusBadRequest)
			return
		}
		if _, duplicate := seen[value]; duplicate {
			http.Error(w, "duplicate path in batch", http.StatusBadRequest)
			return
		}
		seen[value] = struct{}{}
		cleaned[index] = value
	}
	fsys, err := s.serverFilesystem(r.PathValue("id"), false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	for _, name := range cleaned {
		if err := fsys.RemoveAll(name); err != nil {
			http.Error(w, "batch delete failed after partial execution: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": len(cleaned)})
}

func (s *Server) batchRenameFiles(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Files []struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if len(body.Files) == 0 || len(body.Files) > 100 {
		http.Error(w, "between 1 and 100 files are required", http.StatusBadRequest)
		return
	}
	type rename struct{ from, to string }
	cleaned := make([]rename, len(body.Files))
	targets := make(map[string]struct{}, len(body.Files))
	for index, file := range body.Files {
		from, fromErr := rootfs.Clean(file.From)
		to, toErr := rootfs.Clean(file.To)
		if fromErr != nil || toErr != nil || from == "" || to == "" || from == to {
			http.Error(w, "invalid rename in batch", http.StatusBadRequest)
			return
		}
		if _, duplicate := targets[to]; duplicate {
			http.Error(w, "duplicate destination in batch", http.StatusBadRequest)
			return
		}
		targets[to] = struct{}{}
		cleaned[index] = rename{from: from, to: to}
	}
	fsys, err := s.serverFilesystem(r.PathValue("id"), false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	for _, file := range cleaned {
		if _, err := fsys.Stat(file.from); err != nil {
			http.Error(w, "batch source is unavailable: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	for _, file := range cleaned {
		if err := fsys.MkdirAll(path.Dir(file.to), 0o750); err != nil {
			http.Error(w, "batch rename failed after partial execution: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := fsys.Rename(file.from, file.to); err != nil {
			http.Error(w, "batch rename failed after partial execution: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "renamed": len(cleaned)})
}

func validPermissionMode(mode string) bool {
	if len(mode) != 3 && len(mode) != 4 {
		return false
	}
	for _, character := range mode {
		if character < '0' || character > '7' {
			return false
		}
	}
	return true
}

func (s *Server) chmodFiles(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if body.Path == "" || body.Mode == "" {
		http.Error(w, "path and mode are required", http.StatusBadRequest)
		return
	}
	fsys, err := s.serverFilesystem(r.PathValue("id"), false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	if !validPermissionMode(body.Mode) {
		http.Error(w, "mode must contain three or four octal digits", http.StatusBadRequest)
		return
	}
	mode, err := strconv.ParseUint(body.Mode, 8, 32)
	if err != nil {
		http.Error(w, "invalid mode format", http.StatusBadRequest)
		return
	}
	if err := fsys.Chmod(body.Path, os.FileMode(mode)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) copyFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if body.From == "" || body.To == "" {
		http.Error(w, "from and to are required", http.StatusBadRequest)
		return
	}
	serverID := r.PathValue("id")
	fsys, err := s.serverFilesystem(serverID, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	info, err := fsys.Stat(body.From)
	if err != nil || !info.Mode().IsRegular() {
		http.Error(w, "source is not a regular file", http.StatusNotFound)
		return
	}
	if _, err := fsys.Stat(body.To); err == nil {
		http.Error(w, "destination already exists", http.StatusConflict)
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.manager.HasSpaceForWriteFS(serverID, info.Size(), fsys); err != nil {
		http.Error(w, err.Error(), http.StatusInsufficientStorage)
		return
	}
	if _, err := fsys.Copy(body.From, body.To, info.Mode().Perm(), info.Size()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

func (s *Server) pullRemoteFile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL      string `json:"url"`
		Target   string `json:"target"`
		FileName string `json:"fileName"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	parsed, err := url.Parse(body.URL)
	if err != nil {
		http.Error(w, "invalid URL", http.StatusBadRequest)
		return
	}
	client, err := s.pullClientFactory(r.Context(), parsed)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "private") {
			status = http.StatusForbidden
		}
		http.Error(w, err.Error(), status)
		return
	}
	fileName := body.FileName
	if fileName == "" {
		fileName = path.Base(parsed.EscapedPath())
		if fileName == "." || fileName == "/" {
			fileName = "download"
		}
	}
	fileName, err = safePullFilename(fileName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	target, err := rootfs.Clean(body.Target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	serverID := r.PathValue("id")
	fsys, err := s.serverFilesystem(serverID, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	if err := fsys.MkdirAll(target, 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, parsed.String(), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := client.Do(request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		http.Error(w, "remote server returned "+resp.Status, http.StatusBadGateway)
		return
	}
	maxBytes := envBytes("DAEMON_PULL_MAX_BYTES", defaultMaxPullBytes)
	if resp.ContentLength < 0 {
	} else if resp.ContentLength > maxBytes {
		http.Error(w, "remote file exceeds size limit", http.StatusRequestEntityTooLarge)
		return
	}
	reservation := resp.ContentLength
	if reservation < 0 {
		reservation = maxBytes
	}
	if err := s.manager.HasSpaceForWriteFS(serverID, reservation, fsys); err != nil {
		http.Error(w, err.Error(), http.StatusInsufficientStorage)
		return
	}
	finalName := path.Join(target, fileName)
	if resp.ContentLength >= 0 {
		err = fsys.AtomicWriteExact(finalName, resp.Body, maxBytes, resp.ContentLength, 0o640)
	} else {
		err = fsys.AtomicWrite(finalName, resp.Body, maxBytes, 0o640)
	}
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, rootfs.ErrTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "path": finalName})
}

func (s *Server) startTransfer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TargetNode string `json:"targetNode"`
		TargetURL  string `json:"targetUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if body.TargetNode == "" || body.TargetURL == "" {
		http.Error(w, "targetNode and targetUrl are required", http.StatusBadRequest)
		return
	}
	serverID := r.PathValue("id")
	serverRoot, err := s.safePath(serverID, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	transferMgr := getTransferManager()
	transfer, err := transferMgr.Start(r.Context(), serverID, "local", body.TargetNode, serverRoot, body.TargetURL, s.token, 0)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, transfer)
}

func (s *Server) getTransferStatus(w http.ResponseWriter, r *http.Request) {
	transferID := r.PathValue("transferId")
	transferMgr := getTransferManager()
	transfer, ok := transferMgr.Get(transferID)
	if !ok {
		http.Error(w, "transfer not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, transfer)
}

func (s *Server) cancelTransfer(w http.ResponseWriter, r *http.Request) {
	transferID := r.PathValue("transferId")
	transferMgr := getTransferManager()
	if err := transferMgr.Cancel(transferID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// receiveTransferArchive is the destination-side endpoint for incoming
// server-to-server transfers. The source daemon POSTs a multipart payload
// containing:
//   - "archive" part: tar.gz archive stream of the server files
//   - "checksum" part: hex-encoded SHA256 of the archive
//
// The handler:
//  1. Verifies the SHA256 checksum matches the archive stream
//  2. Extracts the archive to the destination server's root directory
//  3. Notifies the source daemon that the transfer is complete
func (s *Server) receiveTransferArchive(w http.ResponseWriter, r *http.Request) {
	serverID := r.Header.Get("X-Transfer-ServerID")
	if err := serverid.Validate(serverID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	transferID := r.Header.Get("X-Transfer-ID")
	if !safeUploadID(transferID) {
		http.Error(w, "invalid transfer id", http.StatusBadRequest)
		return
	}
	resumeOffset := strings.TrimSpace(r.Header.Get("X-Transfer-Resume-Offset"))
	if resumeOffset != "" {
		offset, err := strconv.ParseInt(resumeOffset, 10, 64)
		if err != nil || offset < 0 {
			http.Error(w, "invalid transfer resume offset", http.StatusBadRequest)
			return
		}
		if offset != 0 {
			http.Error(w, "transfer resume is not supported by the destination", http.StatusNotImplemented)
			return
		}
	}
	expectedChecksum := r.Header.Get("X-Checksum")
	if expectedChecksum == "" {
		http.Error(w, "X-Checksum header is required", http.StatusBadRequest)
		return
	}

	fsys, err := s.serverFilesystem(serverID, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer fsys.Close()
	if err := fsys.MkdirAll(".backups", 0o750); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tempName := path.Join(".backups", ".transfer-"+transferID+".tar.gz")
	out, err := fsys.OpenFile(tempName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	hasher := sha256.New()
	const transferLimit = int64(32 * 1024 * 1024 * 1024)
	written, copyErr := io.Copy(io.MultiWriter(out, hasher), io.LimitReader(r.Body, transferLimit+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil || written > transferLimit {
		_ = fsys.RemoveAll(tempName)
		if written > transferLimit {
			http.Error(w, "transfer archive too large", http.StatusRequestEntityTooLarge)
		} else if copyErr != nil {
			http.Error(w, copyErr.Error(), http.StatusBadRequest)
		} else {
			http.Error(w, closeErr.Error(), http.StatusInternalServerError)
		}
		return
	}
	actualChecksum := hex.EncodeToString(hasher.Sum(nil))
	if actualChecksum != expectedChecksum {
		_ = fsys.RemoveAll(tempName)
		http.Error(w, fmt.Sprintf("checksum mismatch (expected=%s actual=%s)", expectedChecksum, actualChecksum), http.StatusBadRequest)
		return
	}
	if _, err := extractArchive(fsys, tempName, "", s.manager, serverID); err != nil {
		_ = fsys.RemoveAll(tempName)
		http.Error(w, fmt.Sprintf("extract failed: %v", err), http.StatusBadRequest)
		return
	}
	_ = fsys.RemoveAll(tempName)

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"serverId":   serverID,
		"transferId": transferID,
		"bytes":      written,
		"checksum":   actualChecksum,
	})
}

// SetCrashHandler sets a callback that fires when a container exit is
// detected as a crash (exit code != 0 or OOM). The handler receives the
// server ID, exit code, and OOM flag. Typically used to report crashes to
// the panel API.
func (s *Server) SetCrashHandler(handler func(ctx context.Context, serverID string, exitCode int, oomKilled bool)) {
	if s == nil || s.manager == nil {
		return
	}
	s.manager.SetCrashHandler(handler)
}

// SetDetectCleanExitAsCrash forwards the configuration option to the
// underlying ServerManager so newly-created server states inherit it. See
// ServerManager.SetDetectCleanExitAsCrash for semantics.
func (s *Server) SetDetectCleanExitAsCrash(value bool) {
	if s == nil || s.manager == nil {
		return
	}
	s.manager.SetDetectCleanExitAsCrash(value)
}

func (s *Server) safePath(serverID, requested string) (string, error) {
	if err := serverid.Validate(serverID); err != nil {
		return "", err
	}
	if strings.ContainsRune(requested, 0) {
		return "", errors.New("invalid path")
	}
	cleaned := filepath.Clean(strings.TrimPrefix(requested, "/"))
	if cleaned == "." {
		cleaned = ""
	}
	if filepath.IsAbs(requested) || strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, string(filepath.Separator)+".."+string(filepath.Separator)) {
		return "", errors.New("path escapes server directory")
	}
	root, err := filepath.Abs(filepath.Join(s.dataDir, serverID))
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(filepath.Join(root, cleaned))
	if err != nil {
		return "", err
	}
	// Resolve root through symlinks so Rel comparison works on systems
	// where temp directories are symlinked (e.g., macOS /var → /private/var)
	if resolvedRoot, err := filepath.EvalSymlinks(root); err == nil {
		root = resolvedRoot
		target, err = filepath.Abs(filepath.Join(root, cleaned))
		if err != nil {
			return "", err
		}
	}
	rel, err := filepath.Rel(root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes server directory")
	}
	// If root doesn't exist yet, no symlinks to worry about — path is safe by Rel check alone
	if _, statErr := os.Stat(root); statErr != nil {
		return target, nil
	}
	if resolved, err := filepath.EvalSymlinks(target); err == nil {
		resolvedRel, err := filepath.Rel(root, resolved)
		if err != nil || resolvedRel == ".." || strings.HasPrefix(resolvedRel, ".."+string(filepath.Separator)) {
			return "", errors.New("path escapes server directory")
		}
	} else {
		parent := filepath.Dir(target)
		// Don't check parent if it is, or resolves to, the root
		if parent == root {
			return target, nil
		}
		if resolvedParent, parentErr := filepath.EvalSymlinks(parent); parentErr == nil {
			parentRel, err := filepath.Rel(root, resolvedParent)
			if err != nil || parentRel == ".." || strings.HasPrefix(parentRel, ".."+string(filepath.Separator)) {
				return "", errors.New("path escapes server directory")
			}
		}
	}
	return target, nil
}

func parseInt64Query(r *http.Request, key string) (int64, error) {
	var value int64
	text := r.URL.Query().Get(key)
	if text == "" {
		return 0, nil
	}
	for _, char := range text {
		if char < '0' || char > '9' {
			return 0, errors.New("invalid integer")
		}
		value = value*10 + int64(char-'0')
	}
	return value, nil
}

func safeUploadID(uploadID string) bool {
	if uploadID == "" || len(uploadID) > 96 {
		return false
	}
	for _, char := range uploadID {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return !strings.Contains(uploadID, "..")
}

func backupErrorStatus(err error) int {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return http.StatusNotFound
	case errors.Is(err, backup.ErrInvalidName), errors.Is(err, backup.ErrInvalidNamespace):
		return http.StatusBadRequest
	case errors.Is(err, backup.ErrChecksumMismatch):
		return http.StatusUnprocessableEntity
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return http.StatusRequestTimeout
	default:
		return http.StatusInternalServerError
	}
}

func safeBackupName(name string) bool {
	if name == "" || len(name) > 128 || !strings.HasSuffix(name, ".zip") || strings.Contains(name, "..") {
		return false
	}
	for _, char := range name {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func randomHex(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return time.Now().UTC().Format("20060102150405")
	}
	return hex.EncodeToString(buf)
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 3, 64)
}

func formatInt(value int) string {
	return strconv.Itoa(value)
}

func formatUint(value uint64) string {
	return strconv.FormatUint(value, 10)
}

func diskLimitMBFromConfiguration(payload map[string]any) int64 {
	build, ok := payload["build"].(map[string]any)
	if !ok {
		return -1
	}
	for _, key := range []string{"disk_space", "diskSpace", "diskMb"} {
		switch value := build[key].(type) {
		case float64:
			return int64(value)
		case int64:
			return value
		case int:
			return int64(value)
		case json.Number:
			parsed, err := value.Int64()
			if err == nil {
				return parsed
			}
		}
	}
	return -1
}

func memoryMBFromConfiguration(payload map[string]any) int64 {
	build, _ := payload["build"].(map[string]any)
	if len(build) == 0 {
		return 0
	}
	return int64(firstNumber(build, "memory_limit", "memoryLimit", "memoryMb", "memory_mb"))
}

func allocationIPFromConfiguration(payload map[string]any) string {
	allocations, _ := payload["allocations"].(map[string]any)
	if len(allocations) == 0 {
		return ""
	}
	defaultAlloc, _ := allocations["default"].(map[string]any)
	if len(defaultAlloc) == 0 {
		return ""
	}
	if value, ok := defaultAlloc["ip"].(string); ok {
		return value
	}
	return ""
}

func allocationPortFromConfiguration(payload map[string]any) int {
	allocations, _ := payload["allocations"].(map[string]any)
	if len(allocations) == 0 {
		return 0
	}
	defaultAlloc, _ := allocations["default"].(map[string]any)
	if len(defaultAlloc) == 0 {
		return 0
	}
	return firstNumber(defaultAlloc, "port")
}

func stopTypeFromConfiguration(payload map[string]any) string {
	processConfiguration, _ := payload["process_configuration"].(map[string]any)
	if len(processConfiguration) == 0 {
		return ""
	}
	stop, _ := processConfiguration["stop"].(map[string]any)
	if len(stop) == 0 {
		return ""
	}
	value, _ := stop["type"].(string)
	return value
}

func stopValueFromConfiguration(payload map[string]any) string {
	processConfiguration, _ := payload["process_configuration"].(map[string]any)
	if len(processConfiguration) == 0 {
		return ""
	}
	stop, _ := processConfiguration["stop"].(map[string]any)
	if len(stop) == 0 {
		return ""
	}
	value, _ := stop["value"].(string)
	return value
}

func stopTimeoutFromConfiguration(payload map[string]any) time.Duration {
	processConfiguration, _ := payload["process_configuration"].(map[string]any)
	stop, _ := processConfiguration["stop"].(map[string]any)
	for _, key := range []string{"timeout", "timeout_seconds"} {
		if value, ok := stop[key].(float64); ok && value > 0 {
			return time.Duration(value) * time.Second
		}
	}
	return 30 * time.Second
}

func firstNumber(values map[string]any, keys ...string) int {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int(typed)
		case float32:
			return int(typed)
		case int:
			return typed
		case int64:
			return int(typed)
		case int32:
			return int(typed)
		case json.Number:
			if parsed, err := typed.Int64(); err == nil {
				return int(parsed)
			}
		}
	}
	return 0
}

func (s *Server) applyConfigurationFiles(serverID string, payload map[string]any) error {
	config, _ := payload["config"].(map[string]any)
	if len(config) == 0 {
		return nil
	}
	files, _ := config["files"].([]any)
	if len(files) == 0 {
		return nil
	}
	env := map[string]string{}
	if rawEnv, ok := payload["environment"].(map[string]any); ok {
		for key, value := range rawEnv {
			env[key] = fmt.Sprint(value)
		}
	}
	for _, raw := range files {
		fileConfig, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		pathValue, _ := fileConfig["path"].(string)
		if pathValue == "" {
			pathValue, _ = fileConfig["file"].(string)
		}
		if pathValue == "" {
			continue
		}
		content := renderTemplate(fmt.Sprint(fileConfig["content"]), env)
		if properties, ok := fileConfig["properties"].(map[string]any); ok {
			lines := []string{}
			for key, value := range properties {
				lines = append(lines, key+"="+renderTemplate(fmt.Sprint(value), env))
			}
			content = strings.Join(lines, "\n") + "\n"
		}
		if jsonValue, ok := fileConfig["json"]; ok {
			body, err := json.MarshalIndent(jsonValue, "", "  ")
			if err != nil {
				return err
			}
			content = renderTemplate(string(body), env)
		}
		target, err := s.safePath(serverID, pathValue)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		if err := s.manager.HasSpaceForWrite(serverID, int64(len(content))); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(content), 0o640); err != nil {
			return err
		}
	}
	return nil
}

func renderTemplate(input string, env map[string]string) string {
	output := input
	for key, value := range env {
		output = strings.ReplaceAll(output, "{{"+key+"}}", value)
		output = strings.ReplaceAll(output, "{{ "+key+" }}", value)
		output = strings.ReplaceAll(output, "{{env."+key+"}}", value)
		output = strings.ReplaceAll(output, "{{ env."+key+" }}", value)
		output = strings.ReplaceAll(output, "{{"+key+"|default:''}}", value)
	}
	return output
}

func (s *Server) effectiveEnvList(serverID string, explicit []string) []string {
	merged := map[string]string{}
	state := s.manager.State(serverID)
	state.mu.Lock()
	if strings.TrimSpace(state.StartupCommand) != "" {
		merged["STARTUP"] = state.StartupCommand
	}
	if state.MemoryMB > 0 {
		merged["SERVER_MEMORY"] = strconv.FormatInt(state.MemoryMB, 10)
	}
	if strings.TrimSpace(state.AllocationIP) != "" {
		merged["SERVER_IP"] = state.AllocationIP
	}
	if state.AllocationPort > 0 {
		merged["SERVER_PORT"] = strconv.Itoa(state.AllocationPort)
	}
	for key, value := range state.EnvVars {
		if key != "" && !strings.Contains(key, "=") {
			merged[key] = value
		}
	}
	state.mu.Unlock()
	for _, entry := range explicit {
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			continue
		}
		merged[parts[0]] = parts[1]
	}
	out := make([]string, 0, len(merged))
	for key, value := range merged {
		out = append(out, key+"="+value)
	}
	return out
}

func (s *Server) effectiveEnvMapList(serverID string, explicit map[string]string) []string {
	merged := map[string]string{}
	state := s.manager.State(serverID)
	state.mu.Lock()
	if strings.TrimSpace(state.StartupCommand) != "" {
		merged["STARTUP"] = state.StartupCommand
	}
	if state.MemoryMB > 0 {
		merged["SERVER_MEMORY"] = strconv.FormatInt(state.MemoryMB, 10)
	}
	if strings.TrimSpace(state.AllocationIP) != "" {
		merged["SERVER_IP"] = state.AllocationIP
	}
	if state.AllocationPort > 0 {
		merged["SERVER_PORT"] = strconv.Itoa(state.AllocationPort)
	}
	for key, value := range state.EnvVars {
		if key != "" && !strings.Contains(key, "=") {
			merged[key] = value
		}
	}
	state.mu.Unlock()
	for key, value := range explicit {
		if key == "" || strings.Contains(key, "=") {
			continue
		}
		merged[key] = value
	}
	out := make([]string, 0, len(merged))
	for key, value := range merged {
		out = append(out, key+"="+value)
	}
	return out
}

func (s *Server) applyPower(r *http.Request, serverID, signal string) (string, error) {
	if s.runtime == nil {
		return "", errRuntimeUnavailable
	}
	if signal != "start" && signal != "stop" && signal != "restart" && signal != "kill" {
		return "", errors.New("invalid power signal")
	}
	err := s.manager.HandlePower(r.Context(), serverID, signal)
	if err == nil {
		return "docker", nil
	}
	return "", err
}

func isContainerMissing(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such container") || strings.Contains(message, "not found")
}

func isImageMissing(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such image") || strings.Contains(message, "pull access denied")
}

func isContainerExists(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "already in use") || strings.Contains(message, "already exists")
}

func runtimeErrorStatus(err error, fallback int) int {
	if errors.Is(err, errRuntimeUnavailable) {
		return http.StatusServiceUnavailable
	}
	if isContainerMissing(err) || isImageMissing(err) {
		return http.StatusNotFound
	}
	if isContainerExists(err) {
		return http.StatusConflict
	}
	return fallback
}

func requestTimeout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if websocket.IsWebSocketUpgrade(r) {
			next.ServeHTTP(w, r)
			return
		}
		http.TimeoutHandler(next, 15*time.Minute, "request timed out").ServeHTTP(w, r)
	})
}

func configureWebSocket(conn *websocket.Conn) {
	conn.SetReadLimit(1024)
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.token == "" || r.URL.Path == "/health" || r.URL.Path == "/metrics" || r.URL.Path == "/download/backup" || (strings.HasPrefix(r.URL.Path, "/api/v1/transfers/") && r.URL.Path != "/api/v1/transfers/credentials") {
			next.ServeHTTP(w, r)
			return
		}
		var body []byte
		var err error
		if isStreamingUpload(r) {
			body = nil
		} else {
			body, err = io.ReadAll(io.LimitReader(r.Body, 1024*1024))
		}
		if err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if !isStreamingUpload(r) {
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(body))
		}

		timestamp := r.Header.Get("X-Panel-Timestamp")
		signature := r.Header.Get("X-Panel-Signature")
		parsed, err := time.Parse(time.RFC3339, timestamp)
		if err != nil || time.Since(parsed) > 5*time.Minute || time.Until(parsed) > 5*time.Minute {
			http.Error(w, "invalid signature timestamp", http.StatusUnauthorized)
			return
		}
		expected := sign(s.token, r.Method, r.URL.RequestURI(), timestamp, body)
		if !hmac.Equal([]byte(signature), []byte(expected)) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isStreamingUpload(r *http.Request) bool {
	return r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/files/upload")
}

// authenticateWebSocket validates JWT token for WebSocket connections.
// Token can be provided via query parameter "token" or Authorization header.
func (s *Server) authenticateWebSocket(w http.ResponseWriter, r *http.Request) (*tokens.Claims, error) {
	if s.tokenGenerator == nil {
		return nil, errors.New("token generator not configured")
	}

	// Try to get token from query parameter first (common for WebSocket connections)
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		// Fall back to Authorization header
		auth := r.Header.Get("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			tokenStr = strings.TrimPrefix(auth, "Bearer ")
		}
	}

	if tokenStr == "" {
		return nil, errors.New("missing token")
	}

	claims, err := s.tokenGenerator.Validate(tokenStr)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	return claims, nil
}

type webSocketWriter struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *webSocketWriter) Write(payload []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	if err := w.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		return 0, err
	}
	return len(payload), nil
}

func (w *webSocketWriter) WriteJSON(value any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return w.conn.WriteJSON(value)
}

func (w *webSocketWriter) WriteBinary(data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return w.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (w *webSocketWriter) Ping() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return w.conn.WriteMessage(websocket.PingMessage, nil)
}

func pingWebSocket(writer *webSocketWriter, done <-chan struct{}) {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if err := writer.Ping(); err != nil {
				return
			}
		}
	}
}

func (s *Server) command(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	if strings.TrimSpace(body.Command) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "command is required"})
		return
	}
	if s.runtime == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runtime unavailable"})
		return
	}
	if err := s.consoles.Write(r.PathValue("id"), body.Command); err != nil {
		writeJSON(w, runtimeErrorStatus(err, http.StatusBadGateway), map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- Edge Agent Handlers ----

var edgeAgent *EdgeAgent
var edgeAgentMu sync.Mutex
var upgradeMgr *UpgradeManager
var upgradeMu sync.Mutex

func (s *Server) handleEdgeStatus(w http.ResponseWriter, r *http.Request) {
	edgeAgentMu.Lock()
	if edgeAgent == nil {
		edgeAgentMu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"state": "not-started"})
		return
	}
	state := edgeAgent.State()
	edgeAgentMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"state": string(state), "service": "edge-agent"})
}

func (s *Server) handleEdgeStats(w http.ResponseWriter, r *http.Request) {
	edgeAgentMu.Lock()
	if edgeAgent == nil {
		edgeAgentMu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"state": "not-started"})
		return
	}
	stats := edgeAgent.Stats()
	edgeAgentMu.Unlock()
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleEdgeConnect(w http.ResponseWriter, r *http.Request) {
	edgeAgentMu.Lock()
	if edgeAgent == nil {
		edgeAgentMu.Unlock()
		writeError(w, http.StatusServiceUnavailable, "edge agent not initialized")
		return
	}
	edgeAgent.ConnectNow()
	edgeAgentMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"connected": true})
}

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	diag := s.RunConnectivityDiagnostics(r.Context(), os.Getenv("PANEL_API_URL"))
	diag.EdgeState = string(EdgeStateDisconnected)
	edgeAgentMu.Lock()
	if edgeAgent != nil {
		diag.EdgeState = string(edgeAgent.State())
	}
	edgeAgentMu.Unlock()
	diag.AgentConnected = s.runtime != nil
	writeJSON(w, http.StatusOK, diag)
}

func (s *Server) handleConnectivityDiagnostics(w http.ResponseWriter, r *http.Request) {
	panelURL := r.URL.Query().Get("url")
	if panelURL == "" {
		panelURL = os.Getenv("PANEL_API_URL")
	}
	diag := s.RunConnectivityDiagnostics(r.Context(), panelURL)
	writeJSON(w, http.StatusOK, diag)
}

// Command acknowledgement: Beacon confirms it received a command.
func (s *Server) handleCommandAck(w http.ResponseWriter, r *http.Request) {
	commandID := r.PathValue("id")
	if commandID == "" {
		writeError(w, http.StatusBadRequest, "command id is required")
		return
	}
	if err := s.operations.AckCommand(commandID); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"acknowledged": true})
}

// Command progress: Beacon reports step-level progress for a command.
func (s *Server) handleCommandProgress(w http.ResponseWriter, r *http.Request) {
	commandID := r.PathValue("id")
	if commandID == "" {
		writeError(w, http.StatusBadRequest, "command id is required")
		return
	}
	var body struct {
		Progress    string `json:"progress"`
		ProgressPct int    `json:"progressPct"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid progress payload")
		return
	}
	if err := s.operations.SetProgress(commandID, body.Progress, body.ProgressPct); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": true})
}

// Command result: Beacon delivers terminal command output.
func (s *Server) handleCommandResult(w http.ResponseWriter, r *http.Request) {
	commandID := r.PathValue("id")
	if commandID == "" {
		writeError(w, http.StatusBadRequest, "command id is required")
		return
	}
	var body struct {
		ResultData string `json:"resultData"`
		Status     string `json:"status,omitempty"`
		Error      string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid result payload")
		return
	}
	if body.Status == "failed" || body.Error != "" {
		qErr := s.operations.SetError(commandID, body.Error)
		if qErr != nil {
			writeError(w, http.StatusNotFound, qErr.Error())
			return
		}
	}
	if err := s.operations.SetResult(commandID, body.ResultData); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"delivered": true})
}

// Pending commands: Beacon polls for commands that need to be processed.
// A serverID query parameter can be provided to filter by server.
func (s *Server) handlePendingCommands(w http.ResponseWriter, r *http.Request) {
	serverID := r.URL.Query().Get("serverId")
	if serverID != "" {
		writeJSON(w, http.StatusOK, s.operations.ListPendingByServer(serverID))
		return
	}
	all := s.operations.ServersWithPending()
	result := make(map[string][]Operation)
	for _, sid := range all {
		pending := s.operations.ListPendingByServer(sid)
		if len(pending) > 0 {
			result[sid] = pending
		}
	}
	writeJSON(w, http.StatusOK, result)
}

type VersionInventory struct {
	BeaconVersion string         `json:"beaconVersion"`
	APIVersion    string         `json:"apiVersion,omitempty"`
	GoVersion     string         `json:"goVersion"`
	OS            string         `json:"os"`
	Architecture  string         `json:"architecture"`
	Capabilities  []string       `json:"capabilities"`
	Upgrades      *UpgradeStatus `json:"upgrades,omitempty"`
	EdgeState     string         `json:"edgeState"`
	UptimeSeconds int64          `json:"uptimeSeconds"`
}

func (s *Server) handleVersionInventory(w http.ResponseWriter, r *http.Request) {
	inv := VersionInventory{
		BeaconVersion: "beacon-dev",
		GoVersion:     stdruntime.Version(),
		OS:            stdruntime.GOOS,
		Architecture:  stdruntime.GOARCH,
		Capabilities:  []string{"docker", "sftp", "backups", "transfers", "stats", "console", "files", "compose", "build", "edge-agent", "upgrade"},
		UptimeSeconds: int64(time.Since(beaconStart).Seconds()),
		EdgeState:     "unknown",
	}
	edgeAgentMu.Lock()
	if edgeAgent != nil {
		inv.EdgeState = string(edgeAgent.State())
	}
	edgeAgentMu.Unlock()
	upgradeMu.Lock()
	if upgradeMgr != nil {
		status := upgradeMgr.Status()
		inv.Upgrades = &status
	}
	upgradeMu.Unlock()
	writeJSON(w, http.StatusOK, inv)
}

func (s *Server) handleUpgradeBegin(w http.ResponseWriter, r *http.Request) {
	var payload UpgradePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid upgrade payload")
		return
	}
	if payload.Version == "" || payload.DownloadURL == "" {
		writeError(w, http.StatusBadRequest, "version and downloadUrl are required")
		return
	}
	upgradeMu.Lock()
	if upgradeMgr == nil {
		binPath, _ := os.Executable()
		upgradeMgr = NewUpgradeManager("beacon-dev", binPath, s.dataDir)
	}
	upgradeMu.Unlock()
	if err := upgradeMgr.Begin(r.Context(), payload); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, upgradeMgr.Status())
}

func (s *Server) handleUpgradeStatus(w http.ResponseWriter, r *http.Request) {
	upgradeMu.Lock()
	if upgradeMgr == nil {
		upgradeMu.Unlock()
		writeJSON(w, http.StatusOK, UpgradeStatus{State: UpgradeStateIdle})
		return
	}
	status := upgradeMgr.Status()
	upgradeMu.Unlock()
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleUpgradeApply(w http.ResponseWriter, r *http.Request) {
	upgradeMu.Lock()
	if upgradeMgr == nil {
		upgradeMu.Unlock()
		writeError(w, http.StatusBadRequest, "no upgrade in progress")
		return
	}
	upgradeMu.Unlock()
	if err := upgradeMgr.Apply(r.Context()); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, upgradeMgr.Status())
}

func (s *Server) handleUpgradeRollback(w http.ResponseWriter, r *http.Request) {
	upgradeMu.Lock()
	if upgradeMgr == nil {
		upgradeMu.Unlock()
		writeError(w, http.StatusBadRequest, "no upgrade to roll back")
		return
	}
	upgradeMu.Unlock()
	if err := upgradeMgr.Rollback(r.Context()); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, upgradeMgr.Status())
}

func sign(token, method, requestURI, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(method))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(requestURI))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
