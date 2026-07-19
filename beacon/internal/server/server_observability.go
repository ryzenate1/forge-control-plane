package server

import (
	"encoding/json"
	"io"
	"net/http"
	stdruntime "runtime"
	"strings"
	"sync"
	"time"

	"gamepanel/beacon/internal/runtime"

	"github.com/docker/docker/pkg/stdcopy"
	"github.com/gorilla/websocket"
)

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

func (s *Server) statsWS(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		http.Error(w, errRuntimeUnavailable.Error(), http.StatusServiceUnavailable)
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
	}()
	<-errs
}

func configureWebSocket(conn *websocket.Conn) {
	conn.SetReadLimit(1024)
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})
}

type webSocketWriter struct {
	conn *websocket.Conn
	mu   sync.Mutex
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
