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
)

// getWebSocketAllowedOrigins returns the list of allowed WebSocket origins for CORS validation.
// This implements the security fix identified in the comprehensive audit to prevent
// WebSocket origin bypass attacks.
func getWebSocketAllowedOrigins(cfg Config) []string {
	// Check for explicit environment variable configuration
	if raw := os.Getenv("API_WS_ALLOWED_ORIGINS"); strings.TrimSpace(raw) != "" {
		origins := []string{}
		for _, origin := range strings.Split(raw, ",") {
			if trimmed := strings.TrimSpace(origin); trimmed != "" {
				origins = append(origins, trimmed)
			}
		}
		if len(origins) > 0 {
			return origins
		}
	}

	// Default to localhost origins for development
	// In production, API_WS_ALLOWED_ORIGINS MUST be set explicitly
	return []string{
		"http://localhost:3000",
		"http://127.0.0.1:3000",
		"http://localhost:3002",
		"http://127.0.0.1:3002",
	}
}

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

		errs := make(chan error, 2)
		go pumpUpstreamToClient(ctx, upstream, client, errs)
		go pumpClientToUpstream(ctx, client, upstream, errs)
		<-errs
		cancel()
		_ = client.Close()
		_ = upstream.Close()
		<-errs
	}
}

func configureClientSocket(conn *fiberws.Conn) {
	conn.SetReadLimit(1024 * 1024)
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})
}

func configureUpstreamSocket(conn *gorilla.Conn) {
	conn.SetReadLimit(1024 * 1024)
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
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

func pumpClientToUpstream(ctx context.Context, client *fiberws.Conn, upstream *gorilla.Conn, errs chan<- error) {
	for {
		if ctx.Err() != nil {
			errs <- ctx.Err()
			return
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
