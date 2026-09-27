package http

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"gamepanel/forge/internal/store"

	fiberws "github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	gorilla "github.com/gorilla/websocket"
	"golang.org/x/time/rate"
)

// getWebSocketAllowedOrigins returns the list of allowed WebSocket origins for CORS validation.
// This implements the security fix identified in the comprehensive audit to prevent
// WebSocket origin bypass attacks.
//
// The allowlist MERGES (in order):
//  1. explicit API_WS_ALLOWED_ORIGINS entries,
//  2. the configured panel URL (cfg.PanelURL / PANEL_URL / APP_URL env),
//  3. the HTTP CORS allow-list (cfg.CORSConfig.AllowedOrigins / API_CORS_ALLOWED_ORIGINS),
//  4. localhost dev defaults, only outside production.
//
// The wildcard "*" is stripped in production so a misconfigured env can never
// open a cross-origin WS hole (fail closed).
func getWebSocketAllowedOrigins(cfg Config) []string {
	isProd := strings.EqualFold(strings.TrimSpace(cfg.AppEnv), "production")
	origins := []string{}
	seen := make(map[string]bool)
	add := func(raw string) {
		origin := strings.TrimSpace(raw)
		if origin == "" {
			return
		}
		if origin == "*" && isProd {
			return
		}
		key := strings.ToLower(origin)
		if seen[key] {
			return
		}
		seen[key] = true
		origins = append(origins, origin)
	}

	// 1. Explicit WS origins from the environment.
	for _, origin := range strings.Split(os.Getenv("API_WS_ALLOWED_ORIGINS"), ",") {
		add(origin)
	}

	// 2. The panel URL is always an allowed (same-origin) WS caller.
	add(cfg.PanelURL)
	if panel := strings.TrimSpace(os.Getenv("PANEL_URL")); panel != "" {
		add(panel)
	} else if app := strings.TrimSpace(os.Getenv("APP_URL")); app != "" {
		add(app)
	}

	// 3. Everything in the CORS allow-list may also open websockets.
	for _, origin := range cfg.CORSConfig.AllowedOrigins {
		add(origin)
	}
	for _, origin := range strings.Split(os.Getenv("API_CORS_ALLOWED_ORIGINS"), ",") {
		add(origin)
	}

	// 4. Localhost dev defaults; in production the operator must configure
	// explicit origins (fail closed — no wildcard fallback).
	if !isProd {
		add("http://localhost:3000")
		add("http://127.0.0.1:3000")
		add("http://localhost:3002")
		add("http://127.0.0.1:3002")
	}

	// Returning an empty list is not "allow nothing" — it is "allow everything".
	// gofiber/contrib/websocket replaces an empty Config.Origins with []string{"*"}
	// and then short-circuits its origin check on Origins[0] == "*", so every
	// origin is accepted. That inverts this whole function: in production with no
	// PANEL_URL, no APP_URL and no CORS allow-list there is nothing to add, and
	// setting API_WS_ALLOWED_ORIGINS="*" is *stripped* above — so the branch
	// written to close the hole is the one that opens it widest.
	//
	// An unmatchable sentinel keeps the list non-empty and non-wildcard. It can
	// never equal a browser-sent Origin (those are scheme://host[:port], and
	// `null` — which sandboxed iframes and file:// pages really do send — is
	// deliberately not it), so an unconfigured production panel refuses every
	// cross-origin upgrade instead of accepting all of them.
	if len(origins) == 0 {
		return []string{wsOriginDenyAll}
	}

	return origins
}

// wsOriginDenyAll is the fail-closed sentinel for an empty websocket origin
// allow-list. It is compared for equality against the request's Origin header
// and cannot match any value a browser will send.
const wsOriginDenyAll = "forge:deny-all-websocket-origins"

func requireRealtimeServices(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "realtime service requires postgres and daemon")
		}
		return c.Next()
	}
}

func realtimeProxy(cfg Config, ticketStore *wsTicketStore, stream string) func(*fiberws.Conn) {
	return func(client *fiberws.Conn) {
		defer client.Close()

		if cfg.Store == nil || cfg.Daemon == nil {
			_ = client.WriteJSON(map[string]any{"error": "realtime service unavailable", "status": http.StatusServiceUnavailable})
			return
		}

		// Two auth modes: a long-lived JWT (legacy) or a short-lived WS ticket.
		// Ticket takes precedence — we peek it to keep it single-use (consumed
		// at the moment of successful upgrade, before any data flows).
		var (
			userID          string
			userRole        string
			ticketToConsume string
			ok              bool
		)
		if ticket := client.Query("token"); ticket != "" && ticketStore != nil {
			// Inspect without consuming first: invalid connections must not burn a
			// legitimate ticket. Consumption happens after identity binding below.
			wsTicket, ticketOK := inspectWSTicket(cfg, ticketStore, ticket)
			if !ticketOK || wsTicket.Stream != stream {
				_ = client.WriteJSON(map[string]any{"error": "invalid or expired ws ticket"})
				return
			}
			if ticketID := client.Params("id"); ticketID != wsTicket.ServerID {
				_ = client.WriteJSON(map[string]any{"error": "ticket server mismatch"})
				return
			}
			// A ticket is tied to the authenticated user that issued it. The
			// session cookie or bearer token provides current session/revocation validation.
			var sessionToken string
			auth := client.Headers("Authorization")
			if auth != "" && strings.HasPrefix(auth, "Bearer ") {
				sessionToken = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
			} else {
				sessionToken = client.Cookies(sessionCookieName)
			}

			if sessionToken == "" {
				_ = client.WriteJSON(map[string]any{"error": "ticket requires session cookie or Authorization header"})
				return
			}
			claims, err := parseToken(cfg.AuthSecret, sessionToken)
			if err != nil {
				_ = client.WriteJSON(map[string]any{"error": "unauthorized"})
				return
			}
			current, err := validateCurrentSession(context.Background(), cfg.Store, claims)
			if err != nil {
				_ = client.WriteJSON(map[string]any{"error": "invalid or revoked session"})
				return
			}
			if current.Sub != wsTicket.UserID {
				_ = client.WriteJSON(map[string]any{"error": "ticket identity mismatch"})
				return
			}
			ticketToConsume = ticket
			userID, userRole, ok = current.Sub, current.Role, true
		} else {
			var sessionToken string
			auth := client.Headers("Authorization")
			if auth != "" && strings.HasPrefix(auth, "Bearer ") {
				sessionToken = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
			} else {
				sessionToken = client.Cookies(sessionCookieName)
			}
			if sessionToken == "" {
				_ = client.WriteJSON(map[string]any{"error": "unauthorized"})
				return
			}
			claims, err := parseToken(cfg.AuthSecret, sessionToken)
			if err != nil {
				_ = client.WriteJSON(map[string]any{"error": "unauthorized"})
				return
			}
			current, err := validateCurrentSession(context.Background(), cfg.Store, claims)
			if err != nil {
				_ = client.WriteJSON(map[string]any{"error": "unauthorized"})
				return
			}
			userID, userRole, ok = current.Sub, current.Role, true
		}
		if !ok {
			_ = client.WriteJSON(map[string]any{"error": "unauthorized"})
			return
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		allowed, err := cfg.Store.UserCanAccessServer(ctx, client.Params("id"), userID, userRole, store.PermWebsocketConnect)
		if err != nil {
			_ = client.WriteJSON(map[string]any{"error": "server not found"})
			return
		}
		if !allowed {
			_ = client.WriteJSON(map[string]any{"error": "missing server permission: " + store.PermWebsocketConnect})
			return
		}

		// For interactive streams (console), additionally require the control.console
		// permission so that a user with only websocket.connect cannot send arbitrary
		// commands through the proxy to the upstream daemon.
		if stream == "console" {
			consoleAllowed, consoleErr := cfg.Store.UserCanAccessServer(ctx, client.Params("id"), userID, userRole, store.PermControlConsole)
			if consoleErr != nil {
				_ = client.WriteJSON(map[string]any{"error": "server not found"})
				return
			}
			if !consoleAllowed {
				_ = client.WriteJSON(map[string]any{"error": "missing server permission: " + store.PermControlConsole})
				return
			}
		}

		// Backup progress streaming is read-only but still gated by backup.read so
		// a caller without backup access cannot observe it.
		if stream == "backup" {
			backupAllowed, backupErr := cfg.Store.UserCanAccessServer(ctx, client.Params("id"), userID, userRole, store.PermBackupRead)
			if backupErr != nil {
				_ = client.WriteJSON(map[string]any{"error": "server not found"})
				return
			}
			if !backupAllowed {
				_ = client.WriteJSON(map[string]any{"error": "missing server permission: " + store.PermBackupRead})
				return
			}
		}

		if ticketToConsume != "" && !consumeWSTicket(cfg, ticketStore, ticketToConsume) {
			_ = client.WriteJSON(map[string]any{"error": "invalid or expired ws ticket"})
			return
		}

		target, err := cfg.Store.ServerControlTarget(ctx, client.Params("id"))
		if err != nil {
			_ = client.WriteJSON(map[string]any{"error": "server not found"})
			return
		}
		upstreamURL, requestURI := cfg.Daemon.WebSocketURL(target.NodeURL, target.ServerID, stream)
		headers, err := cfg.Daemon.SignedHeaders(target.NodeToken, http.MethodGet, requestURI, nil)
		if err != nil {
			_ = client.WriteJSON(map[string]any{"error": err.Error()})
			return
		}
		upstream, _, err := gorilla.DefaultDialer.DialContext(ctx, upstreamURL, headers)
		if err != nil {
			_ = client.WriteJSON(map[string]any{"error": err.Error()})
			return
		}
		defer upstream.Close()
		configureClientSocket(client)
		configureUpstreamSocket(upstream)

		// Start ping keepalive — periodically sends a ping in BOTH directions to
		// detect half-open connections and prevent silent disconnects.
		//
		// Both sockets enforce a read deadline that only an inbound pong extends
		// (see configureClientSocket / configureUpstreamSocket). A browser's
		// WebSocket answers pings but never initiates them, and a console or
		// stats viewer may legitimately send nothing for minutes, so without a
		// client-bound ping the browser side of every stream is torn down after
		// one read-deadline interval of user inactivity even though the workload
		// is still streaming output. Ping the client too.
		pingTicker := time.NewTicker(realtimePingInterval)
		defer pingTicker.Stop()
		go pumpKeepalive(ctx, pingTicker.C, upstream, client)

		errs := make(chan error, 2)
		// Rate-limit upstream-bound messages (from client) to 10/s to prevent
		// a compromised or malicious client from flooding the upstream daemon.
		clientLimiter := rate.NewLimiter(rate.Limit(10), 20)
		go pumpUpstreamToClient(ctx, upstream, client, errs)
		go pumpClientToUpstream(ctx, client, upstream, clientLimiter, errs)
		<-errs
		cancel()
		_ = client.Close()
		_ = upstream.Close()
		<-errs
	}
}

const (
	// realtimeReadLimit caps a single inbound frame on either side of the proxy.
	realtimeReadLimit = 1024 * 1024
	// realtimeReadTimeout is how long a socket may stay silent before it is
	// considered dead. Only an inbound pong extends it, so it must stay
	// comfortably above realtimePingInterval.
	realtimeReadTimeout = 60 * time.Second
	// realtimePingInterval is the keepalive cadence for both directions.
	realtimePingInterval = 30 * time.Second
	// realtimePingWriteTimeout bounds a blocked control-frame write.
	realtimePingWriteTimeout = 5 * time.Second
)

// controlPinger is the part of a WebSocket connection the keepalive loop uses.
// Both *gorilla.Conn and *fiberws.Conn satisfy it, so the loop can be driven
// with a stub in tests instead of a live socket pair.
type controlPinger interface {
	WriteControl(messageType int, data []byte, deadline time.Time) error
}

// pumpKeepalive pings every peer on each tick until the context is cancelled or
// a write fails. Every peer is pinged — dropping the client-bound ping is the
// regression this function exists to make testable, because the browser side of
// an idle stream is then torn down by its own read deadline.
func pumpKeepalive(ctx context.Context, tick <-chan time.Time, peers ...controlPinger) {
	for {
		select {
		case <-tick:
			deadline := time.Now().Add(realtimePingWriteTimeout)
			for _, peer := range peers {
				if err := peer.WriteControl(gorilla.PingMessage, []byte("keepalive"), deadline); err != nil {
					return
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

func configureClientSocket(conn *fiberws.Conn) {
	conn.SetReadLimit(realtimeReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(realtimeReadTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(realtimeReadTimeout))
	})
}

func configureUpstreamSocket(conn *gorilla.Conn) {
	conn.SetReadLimit(realtimeReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(realtimeReadTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(realtimeReadTimeout))
	})
}

func pumpUpstreamToClient(ctx context.Context, upstream *gorilla.Conn, client *fiberws.Conn, errs chan<- error) {
	for {
		if ctx.Err() != nil {
			errs <- ctx.Err()
			return
		}
		messageType, payload, err := upstream.ReadMessage()
		if err != nil {
			errs <- err
			return
		}
		_ = client.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := client.WriteMessage(messageType, payload); err != nil {
			errs <- err
			return
		}
	}
}

func pumpClientToUpstream(ctx context.Context, client *fiberws.Conn, upstream *gorilla.Conn, limiter *rate.Limiter, errs chan<- error) {
	for {
		if ctx.Err() != nil {
			errs <- ctx.Err()
			return
		}
		if limiter != nil {
			if err := limiter.Wait(ctx); err != nil {
				errs <- err
				return
			}
		}
		messageType, payload, err := client.ReadMessage()
		if err != nil {
			errs <- err
			return
		}
		_ = upstream.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := upstream.WriteMessage(messageType, payload); err != nil {
			errs <- err
			return
		}
	}
}
