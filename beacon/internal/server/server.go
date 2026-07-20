package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gamepanel/beacon/internal/backup"
	"gamepanel/beacon/internal/events"
	"gamepanel/beacon/internal/remote"
	"gamepanel/beacon/internal/runtime"
	"gamepanel/beacon/internal/serverid"
	"gamepanel/beacon/internal/tokens"
	"gamepanel/beacon/internal/transfer"

	"github.com/gorilla/websocket"
)

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
	sessionsReg       *sessionRegistry
	dockerState       string
	panelClient       remote.Client
	eventBus          *events.Bus
	consoles          *consoleManager
	pullClientFactory func(context.Context, *url.URL) (*http.Client, error)
	transferProtocol  *transfer.Engine
	operations        *OperationQueue
	tokenGenerator    *tokens.Generator
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
	if rt == nil {
		server.dockerState = "error"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", server.health)
	mux.HandleFunc("GET /metrics", server.metrics)
	mux.HandleFunc("POST /servers", server.create)
	mux.HandleFunc("POST /applications/builds", server.buildApplication)
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
	mux.HandleFunc("HEAD /api/v1/transfers/{id}/destination/archive", server.destinationTransferOffset)
	mux.HandleFunc("PATCH /api/v1/transfers/{id}/destination/archive", server.receiveTransferChunk)
	mux.HandleFunc("POST /api/v1/transfers/{id}/destination/restore", server.restoreTransferDestination)
	mux.HandleFunc("POST /api/v1/transfers/{id}/destination/finalize", server.finalizeTransferDestination)
	mux.HandleFunc("DELETE /api/v1/transfers/{id}", server.cancelProtocolTransfer)
	// Global daemon endpoints (/api/*)
	mux.HandleFunc("GET /api/system", server.getSystem)
	mux.HandleFunc("POST /api/update", server.postUpdate)
	mux.HandleFunc("POST /api/deauthorize-user", server.postDeauthorizeUser)
	mux.HandleFunc("GET /download/backup", server.downloadBackupWithToken)
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

func requestTimeout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if websocket.IsWebSocketUpgrade(r) {
			next.ServeHTTP(w, r)
			return
		}
		http.TimeoutHandler(next, 15*time.Minute, "request timed out").ServeHTTP(w, r)
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

func (w *webSocketWriter) Ping() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	_ = w.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return w.conn.WriteMessage(websocket.PingMessage, nil)
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
