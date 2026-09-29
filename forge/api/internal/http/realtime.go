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

// wsSessionToken extracts the caller's session JWT from a WebSocket upgrade
// request. Bearer takes precedence; the cookie name honors
// LoadSessionCookieConfig so insecure (non-__Host-prefixed) deployments read
// the cookie they actually set instead of the production name.
func wsSessionToken(client *fiberws.Conn) string {
	auth := client.Headers("Authorization")
	if auth != "" && strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	cfg := LoadSessionCookieConfig()
	return client.Cookies(secureCookieName(sessionCookieName, cfg.Secure))
}

// validateWSOrigin is a defense-in-depth origin check for realtime upgrades.
// fiberws.Config.Origins already gates the handshake; this second check keeps
// the proxy fail-closed if the route config ever drifts. Empty origins belong
// to non-browser clients and are accepted, matching the upgrader semantics.
func validateWSOrigin(client *fiberws.Conn, cfg Config) error {
	origin := strings.TrimSpace(client.Headers("Origin"))
	if origin == "" {
		return nil
	}
	for _, allowed := range getWebSocketAllowedOrigins(cfg) {
		if origin == allowed {
			return nil
		}
	}
	return fiber.NewError(fiber.StatusForbidden, "websocket origin is not allowed")
}

// checkRealtimeTwoFactor enforces the panel 2FA policy on realtime upgrades.
// WS routes bypass the protected middleware chain (a browser WebSocket
// handshake cannot carry CSRF headers and tickets are minted separately), so
// the proxy must enforce the same policy requireTwoFactorAuthentication
// applies to HTTP: fail closed when settings are unreadable, exempt nothing.
func checkRealtimeTwoFactor(ctx context.Context, cfg Config, claims tokenClaims) error {
	if cfg.Store == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "realtime service requires postgres")
	}
	settings, err := cfg.Store.GetPanelSettings(ctx)
	if err != nil {
		settings = store.PanelSettings{Require2FA: "all"}
	}
	if settings.Require2FA == "" || settings.Require2FA == "none" {
		return nil
	}
	user, err := cfg.Store.GetUserByID(ctx, claims.Sub)
	if err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "user not found")
	}
	has2FA := userHasConfiguredTwoFactor(ctx, cfg, user)
	switch settings.Require2FA {
	case "admin":
		if user.Role == RoleAdmin && !has2FA {
			return fiber.NewError(fiber.StatusForbidden, "two-factor authentication is required for admin accounts")
		}
	case "all":
		if !has2FA {
			return fiber.NewError(fiber.StatusForbidden, "two-factor authentication is required")
		}
	}
	return nil
}

// sessionLookupCtx bounds the session/revocation DB lookup off the upgrade
// path. The websocket handler runs as func(*fiberws.Conn) with no request
// context to inherit, so an unbounded Background call could hang the upgrade
// forever; a 5s timeout fails closed instead. The proxy context below
// (ctx/cancel) is scoped to the connection lifetime: it is cancelled when
// either pump exits or the handler returns, so downstream dials and pumps
// observe conn close rather than a detached Background.
func sessionLookupCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func realtimeProxy(cfg Config, ticketStore *wsTicketStore, stream string) func(*fiberws.Conn) {
	return func(client *fiberws.Conn) {
		defer client.Close()

		if cfg.Store == nil || cfg.Daemon == nil {
			_ = client.WriteJSON(map[string]any{"error": "realtime service unavailable", "status": http.StatusServiceUnavailable})
			return
		}

		if err := validateWSOrigin(client, cfg); err != nil {
			_ = client.WriteJSON(map[string]any{"error": "websocket origin is not allowed"})
			return
		}

		// Two auth modes: a long-lived JWT (legacy) or a short-lived WS ticket.
		// Ticket takes precedence — we peek it to keep it single-use (consumed
		// at the moment of successful upgrade, before any data flows).
		var (
			userID          string
			userRole        string
			current         tokenClaims
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
			sessionToken := wsSessionToken(client)

			if sessionToken == "" {
				_ = client.WriteJSON(map[string]any{"error": "ticket requires session cookie or Authorization header"})
				return
			}
			claims, err := parseToken(cfg.AuthSecret, sessionToken)
			if err != nil {
				_ = client.WriteJSON(map[string]any{"error": "unauthorized"})
				return
			}
			lookupCtx, lookupCancel := sessionLookupCtx()
		validated, err := validateCurrentSession(lookupCtx, cfg.Store, claims)
		lookupCancel()
			if err != nil {
				_ = client.WriteJSON(map[string]any{"error": "invalid or revoked session"})
				return
			}
			if validated.Sub != wsTicket.UserID {
				_ = client.WriteJSON(map[string]any{"error": "ticket identity mismatch"})
				return
			}
			ticketToConsume = ticket
			current = validated
			userID, userRole, ok = validated.Sub, validated.Role, true
		} else {
			sessionToken := wsSessionToken(client)
			if sessionToken == "" {
				_ = client.WriteJSON(map[string]any{"error": "unauthorized"})
				return
			}
			claims, err := parseToken(cfg.AuthSecret, sessionToken)
			if err != nil {
				_ = client.WriteJSON(map[string]any{"error": "unauthorized"})
				return
			}
			lookupCtx, lookupCancel := sessionLookupCtx()
		validated, err := validateCurrentSession(lookupCtx, cfg.Store, claims)
		lookupCancel()
			if err != nil {
				_ = client.WriteJSON(map[string]any{"error": "unauthorized"})
				return
			}
			current = validated
			userID, userRole, ok = validated.Sub, validated.Role, true
		}
		if !ok {
			_ = client.WriteJSON(map[string]any{"error": "unauthorized"})
			return
		}

		// Realtime upgrades skip the protected chain (no CSRF header fits in a
		// browser WebSocket handshake), so the 2FA policy must be enforced
		// here for both ticket and legacy-JWT modes. Tickets are minted behind
		// requireTwoFactorAuthentication, but the session's 2FA posture can
		// change within the ticket's 60s window — recheck, fail closed.
		twoFactorCtx, twoFactorCancel := context.WithTimeout(context.Background(), 5*time.Second)
		twoFactorErr := checkRealtimeTwoFactor(twoFactorCtx, cfg, current)
		twoFactorCancel()
		if twoFactorErr != nil {
			_ = client.WriteJSON(map[string]any{"error": "two-factor authentication is required"})
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
		// Only interactive streams forward client input upstream. Console is
		// the interactive stream; install is a read-only progress tap on the
		// Beacon (see beacon installWS: it never reads a command), so it
		// forwards only when the client explicitly opts into interactive mode.
		// Every other stream (stats, logs, backup) is output-only: client
		// frames are still read — to answer pings and observe closes — but
		// discarded, never proxied to the daemon as commands.
		if realtimeAllowUpstream(stream, client) {
			// Rate-limit upstream-bound messages (from client) to 10/s to prevent
			// a compromised or malicious client from flooding the upstream daemon.
			clientLimiter := rate.NewLimiter(rate.Limit(10), 20)
			go pumpUpstreamToClient(ctx, upstream, client, errs)
			go pumpClientToUpstream(ctx, client, upstream, clientLimiter, errs)
		} else {
			go pumpUpstreamToClient(ctx, upstream, client, errs)
			go pumpClientDiscard(ctx, client, errs)
		}
		<-errs
		cancel()
		_ = client.Close()
		_ = upstream.Close()
		<-errs
	}
}

// realtimeAllowUpstream reports whether client frames on the given stream may
// be proxied to the upstream daemon. Console is interactive; install forwards
// only with an explicit interactive opt-in, because the Beacon's install
// socket never reads commands and anything forwarded would be dead bytes at
// best. All other streams are output-only.
func realtimeAllowUpstream(stream string, client *fiberws.Conn) bool {
	switch stream {
	case "console":
		return true
	case "install":
		return strings.EqualFold(strings.TrimSpace(client.Query("interactive")), "true")
	default:
		return false
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

// pumpClientDiscard drains inbound client frames on output-only streams
// without forwarding them. The reads still have to happen: they answer
// pings (extending the read deadline) and surface normal closes, so a quiet
// viewer is not torn down as dead. Anything the client sends is dropped —
// stats, logs, backup and non-interactive install sockets carry streams,
// never commands.
func pumpClientDiscard(ctx context.Context, client *fiberws.Conn, errs chan<- error) {
	for {
		if ctx.Err() != nil {
			errs <- ctx.Err()
			return
		}
		if _, _, err := client.ReadMessage(); err != nil {
			errs <- err
			return
		}
	}
}
