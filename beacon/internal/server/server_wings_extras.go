package server

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Daemon API extras: routes called by the panel (POST /api/update pushes
// config, POST /api/deauthorize-user kicks the user off SFTP/WS) or queried by
// monitoring tools (GET /api/system). All routes accept the same bearer
// token as the rest of the daemon (handled by `authenticate`).

// sessionRegistry tracks active WS/SFTP connections by user so
// deauthorize-user can close them.
type sessionRegistry struct {
	mu        sync.Mutex
	sessions  map[string]map[*websocket.Conn]struct{} // userID -> connections
	all       map[*websocket.Conn]struct{}
	wsServers map[*websocket.Conn]string
	external  map[string]map[io.Closer]string // userID -> closer -> serverID
}

func newSessionRegistry() *sessionRegistry {
	return &sessionRegistry{
		sessions:  make(map[string]map[*websocket.Conn]struct{}),
		all:       make(map[*websocket.Conn]struct{}),
		wsServers: make(map[*websocket.Conn]string),
		external:  make(map[string]map[io.Closer]string),
	}
}

func (r *sessionRegistry) track(userID, serverID string, conn *websocket.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[userID]; !ok {
		r.sessions[userID] = make(map[*websocket.Conn]struct{})
	}
	r.sessions[userID][conn] = struct{}{}
	r.all[conn] = struct{}{}
	r.wsServers[conn] = serverID
}

func (r *sessionRegistry) untrack(conn *websocket.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.all, conn)
	delete(r.wsServers, conn)
	for _, conns := range r.sessions {
		delete(conns, conn)
	}
}

func (r *sessionRegistry) trackExternal(userID, serverID string, closer io.Closer) func() {
	r.mu.Lock()
	if r.external[userID] == nil {
		r.external[userID] = make(map[io.Closer]string)
	}
	r.external[userID][closer] = serverID
	r.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { r.mu.Lock(); delete(r.external[userID], closer); r.mu.Unlock() }) }
}

func (r *sessionRegistry) closeUser(userID string, servers []string) int {
	r.mu.Lock()
	ws := make([]*websocket.Conn, 0, len(r.sessions[userID]))
	for conn := range r.sessions[userID] {
		if len(servers) == 0 {
			ws = append(ws, conn)
			continue
		}
		for _, serverID := range servers {
			if r.wsServers[conn] == serverID {
				ws = append(ws, conn)
				break
			}
		}
	}
	serverSet := make(map[string]bool, len(servers))
	for _, id := range servers {
		serverSet[id] = true
	}
	external := make([]io.Closer, 0)
	for closer, serverID := range r.external[userID] {
		if len(serverSet) == 0 || serverSet[serverID] {
			external = append(external, closer)
		}
	}
	r.mu.Unlock()
	closed := 0
	for _, c := range ws {
		_ = c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "deauthorized"), time.Now().Add(time.Second))
		_ = c.Close()
		closed++
	}
	for _, closer := range external {
		_ = closer.Close()
		closed++
	}
	return closed
}

func (r *sessionRegistry) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := len(r.all)
	for _, sessions := range r.external {
		count += len(sessions)
	}
	return count
}

// systemInfo is the JSON returned by GET /api/system (Wings parity).
type systemInfo struct {
	Version        string   `json:"version"`
	OS             string   `json:"os"`
	Architecture   string   `json:"architecture"`
	CPUThreads     int      `json:"cpu_threads"`
	MemoryMB       uint64   `json:"memoryMb"`
	DiskMB         uint64   `json:"diskMb"`
	GoVersion      string   `json:"go_version"`
	Goroutines     int      `json:"goroutines"`
	UptimeSeconds  int64    `json:"uptime_seconds"`
	DockerStatus   string   `json:"dockerStatus"`
	ActiveSessions int      `json:"activeSessions"`
	Capabilities   []string `json:"capabilities"`
}

var beaconStart = time.Now()

// postUpdate accepts a full or partial Wings config payload. To move closer to
// Wings' maturity model we persist the pushed payload to disk so the daemon has
// an auditable, recoverable copy of the latest panel-managed daemon
// configuration. Runtime hot-reload is still limited, but the update is no
// longer a no-op stub.
func (s *Server) postUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	body, err := readAllBounded(r.Body, 16*1024*1024)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	keys := make([]string, 0, len(payload))
	for k := range payload {
		keys = append(keys, k)
	}
	if err := persistDaemonConfigUpdate(payload); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	slogUpdateAccepted(keys)
	writeJSON(w, http.StatusOK, map[string]bool{"applied": true})
}

func persistDaemonConfigUpdate(payload map[string]any) error {
	path := daemonConfigPath()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o640)
}

func daemonConfigPath() string {
	if value := os.Getenv("DAEMON_CONFIG_PATH"); value != "" {
		return value
	}
	if value := os.Getenv("WINGS_CONFIG_FILE"); value != "" {
		return value
	}
	if dataDir := os.Getenv("DAEMON_DATA_DIR"); dataDir != "" {
		return filepath.Join(dataDir, ".config", "daemon.json")
	}
	return "/etc/gamepanel/beacon/config.json"
}

// postDeauthorizeUser terminates all active WS/SFTP sessions for a user.
// Wings: POST /api/deauthorize-user, body {"user": "uuid", "servers": [...]?}.
func (s *Server) postDeauthorizeUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		User    string   `json:"user"`
		Servers []string `json:"servers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.User == "" {
		writeError(w, http.StatusBadRequest, "user is required")
		return
	}
	registry := s.sessions()
	closed := registry.closeUser(body.User, body.Servers)
	writeJSON(w, http.StatusNoContent, map[string]int{"closed": closed})
}

// getSystem returns system info. Wings: GET /api/system? v=1|v=2.
// Without `?v=` we return the full info object (matching Wings v2 default).
func (s *Server) getSystem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var mem goruntime.MemStats
	goruntime.ReadMemStats(&mem)
	info := systemInfo{
		Version:        "beacon-dev",
		OS:             goruntime.GOOS,
		Architecture:   goruntime.GOARCH,
		CPUThreads:     goruntime.NumCPU(),
		MemoryMB:       mem.Alloc / (1024 * 1024),
		GoVersion:      goruntime.Version(),
		Goroutines:     goruntime.NumGoroutine(),
		UptimeSeconds:  int64(time.Since(beaconStart).Seconds()),
		DockerStatus:   s.dockerStatus(),
		ActiveSessions: s.sessions().count(),
		Capabilities:   []string{"docker", "sftp", "backups", "transfers", "stats", "console", "files"},
	}
	if r.URL.Query().Get("v") == "1" {
		// Trimmed legacy shape.
		writeJSON(w, http.StatusOK, map[string]any{
			"version":      info.Version,
			"os":           info.OS,
			"architecture": info.Architecture,
		})
		return
	}
	writeJSON(w, http.StatusOK, info)
}
