package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"gamepanel/beacon/internal/tokens"
)

// This file implements the *streaming* half of server installation. Commands
// never enter here: an installation is started exclusively by the
// panel-authenticated POST /servers/{id}/install (and its reinstall alias),
// which publishes progress into the hub below. The install websocket is a
// read-only tap on that progress, which is what keeps the "WebSockets carry
// streams, never commands" invariant intact.

// installReplayLimit bounds how many already-published progress payloads are
// kept for a socket that attaches to a running install, so an install with very
// chatty output cannot grow the buffer without limit.
const installReplayLimit = 512

// installSubscriberBuffer is the per-socket queue depth. A socket that falls
// behind loses the oldest frames instead of stalling the installing request.
const installSubscriberBuffer = 64

// installSession tracks one server's install progress. It is written to only by
// the request that owns the install and read from only by attaching sockets.
type installSession struct {
	mu     sync.Mutex
	active bool
	replay [][]byte
	subs   map[chan []byte]struct{}
}

func newInstallSession() *installSession {
	return &installSession{subs: make(map[chan []byte]struct{})}
}

// publish encodes payload, buffers it for late attachers, and forwards it to
// every attached socket. It never blocks on a slow client.
func (ss *installSession) publish(payload map[string]any) {
	if ss == nil {
		return
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}

	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.replay = append(ss.replay, encoded)
	if overflow := len(ss.replay) - installReplayLimit; overflow > 0 {
		ss.replay = append(ss.replay[:0:0], ss.replay[overflow:]...)
	}
	for ch := range ss.subs {
		select {
		case ch <- encoded:
		default:
			// Drop the oldest queued frame to make room, matching the
			// backpressure policy used by the event bus.
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- encoded:
			default:
			}
		}
	}
}

// attach snapshots the buffered progress and registers a live channel in the
// same critical section, so an attacher sees every frame exactly once. Attaching
// is only possible while an install is running.
func (ss *installSession) attach() ([][]byte, chan []byte, bool) {
	if ss == nil {
		return nil, nil, false
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if !ss.active {
		return nil, nil, false
	}
	live := make(chan []byte, installSubscriberBuffer)
	if ss.subs == nil {
		ss.subs = make(map[chan []byte]struct{})
	}
	ss.subs[live] = struct{}{}
	return append([][]byte(nil), ss.replay...), live, true
}

func (ss *installSession) detach(live chan []byte) {
	if ss == nil || live == nil {
		return
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	delete(ss.subs, live)
}

// end marks the install finished and closes every attached socket's channel so
// readers return after draining the final event. It is idempotent.
func (ss *installSession) end() {
	if ss == nil {
		return
	}
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if !ss.active {
		return
	}
	ss.active = false
	for ch := range ss.subs {
		close(ch)
		delete(ss.subs, ch)
	}
}

// installHub keeps the current (or most recent) install session per server.
type installHub struct {
	mu       sync.Mutex
	sessions map[string]*installSession
}

func newInstallHub() *installHub {
	return &installHub{sessions: make(map[string]*installSession)}
}

func (h *installHub) begin(serverID string) *installSession {
	session := newInstallSession()
	session.active = true
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sessions == nil {
		h.sessions = make(map[string]*installSession)
	}
	h.sessions[serverID] = session
	return session
}

func (h *installHub) session(serverID string) *installSession {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sessions[serverID]
}

// installSessions returns the hub, creating it on first use so a Server built
// without going through NewServer still behaves predictably.
func (s *Server) installSessions() *installHub {
	if s == nil {
		return nil
	}
	s.installMu.Lock()
	defer s.installMu.Unlock()
	if s.installs == nil {
		s.installs = newInstallHub()
	}
	return s.installs
}

// beginInstall records that an install for serverID is running and discards the
// progress buffered by the previous install for the same server.
func (s *Server) beginInstall(serverID string) *installSession {
	hub := s.installSessions()
	if hub == nil {
		return nil
	}
	return hub.begin(serverID)
}

// publishInstall emits a progress payload (status, log line, error, or
// completion) for the install currently running for serverID.
func (s *Server) publishInstall(serverID string, payload map[string]any) {
	hub := s.installSessions()
	if hub == nil {
		return
	}
	hub.session(serverID).publish(payload)
}

// finishInstall marks the install for serverID as no longer running and releases
// every attached socket.
func (s *Server) finishInstall(serverID string) {
	hub := s.installSessions()
	if hub == nil {
		return
	}
	hub.session(serverID).end()
}

// attachInstall joins the install currently running for serverID. It returns
// ok=false when no install is running, in which case the caller must not stream
// anything: there is nothing to attach to.
func (s *Server) attachInstall(serverID string) ([][]byte, chan []byte, bool) {
	hub := s.installSessions()
	if hub == nil {
		return nil, nil, false
	}
	return hub.session(serverID).attach()
}

func (s *Server) detachInstall(serverID string, live chan []byte) {
	hub := s.installSessions()
	if hub == nil {
		return
	}
	hub.session(serverID).detach(live)
}

// installStreamClaims validates the credential presented for the install
// progress stream. The route is deliberately not a scoped-token route, so the
// panel's node HMAC signature (verified by authenticate) is the normal path and
// no bearer token is presented at all. When a bearer token *is* presented it
// must be a node-admin token bound to this server: a tenant websocket, file or
// backup token must never reach installation output.
func (s *Server) installStreamClaims(r *http.Request, serverID string) (int, string) {
	tokenStr := strings.TrimSpace(r.URL.Query().Get("token"))
	if tokenStr == "" {
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			tokenStr = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		}
	}
	if tokenStr == "" {
		return http.StatusOK, ""
	}
	if s.tokenGenerator == nil {
		return http.StatusUnauthorized, "token generator not configured"
	}
	claims, err := s.tokenGenerator.Validate(tokenStr)
	if err != nil {
		return http.StatusUnauthorized, "invalid install stream token: " + err.Error()
	}
	if claims.Scope != tokens.ScopeAdmin {
		return http.StatusForbidden, "install progress requires an admin-scoped token"
	}
	if strings.TrimSpace(claims.ServerID) != "" && claims.ServerID != serverID {
		return http.StatusForbidden, "token not valid for this server"
	}
	return http.StatusOK, ""
}

// installWS streams the progress and output of the installation currently
// running for a server. It never reads a command from the socket: image,
// entrypoint, script, and environment are supplied only to
// POST /servers/{id}/install. A client that connects while no install is
// running is told so and the connection closes.
func (s *Server) installWS(w http.ResponseWriter, r *http.Request) {
	if s.runtime == nil {
		http.Error(w, errRuntimeUnavailable.Error(), http.StatusServiceUnavailable)
		return
	}

	serverID := r.PathValue("id")
	if status, reason := s.installStreamClaims(r, serverID); status != http.StatusOK {
		http.Error(w, reason, status)
		return
	}

	replay, live, running := s.attachInstall(serverID)
	if !running {
		http.Error(w, "no installation is running for this server; start one with POST /servers/"+serverID+"/install", http.StatusConflict)
		return
	}
	defer s.detachInstall(serverID, live)

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

	for _, payload := range replay {
		if _, err := writer.Write(payload); err != nil {
			return
		}
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case payload, open := <-live:
			if !open {
				// The install finished; its final event was already streamed.
				return
			}
			if _, err := writer.Write(payload); err != nil {
				return
			}
		}
	}
}
