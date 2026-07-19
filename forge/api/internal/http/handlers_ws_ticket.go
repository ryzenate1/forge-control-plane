package http

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
)

var consumeScript = redis.NewScript(`
	local val = redis.call('get', KEYS[1])
	if val then
		redis.call('del', KEYS[1])
	end
	return val
`)

// wsTicketStore holds the in-memory ticket registry. Tickets are short-lived
// (60 seconds) and single-use; after a beacon WS upgrade consumes a ticket
// it's removed.
type wsTicketStore struct {
	mu      sync.RWMutex
	tickets map[string]wsTicket
	cfg     Config
}

type wsTicket struct {
	Subject   string    `json:"subject"`
	UserID    string    `json:"userId"`
	ServerID  string    `json:"serverId"`
	Stream    string    `json:"stream"`
	ExpiresAt time.Time `json:"expiresAt"`
	Consumed  bool      `json:"consumed"`
}

func newWSTicketStore(cfg Config) *wsTicketStore {
	return &wsTicketStore{
		tickets: make(map[string]wsTicket),
		cfg:     cfg,
	}
}

func (s *wsTicketStore) put(t wsTicket) {
	if s.cfg.RedisEnabled && s.cfg.Redis != nil {
		data, err := json.Marshal(t)
		if err == nil {
			s.cfg.Redis.Set(context.Background(), "ws_ticket:"+t.Subject, data, time.Until(t.ExpiresAt)).Err()
		}
		return
	}
	s.mu.Lock()
	s.tickets[t.Subject] = t
	s.mu.Unlock()
	go func() {
		time.Sleep(time.Until(t.ExpiresAt) + 30*time.Second)
		s.mu.Lock()
		delete(s.tickets, t.Subject)
		s.mu.Unlock()
	}()
}

func (s *wsTicketStore) consume(subject string) (wsTicket, bool) {
	if s.cfg.RedisEnabled && s.cfg.Redis != nil {
		val, err := consumeScript.Run(context.Background(), s.cfg.Redis, []string{"ws_ticket:" + subject}).Result()
		if err != nil || val == nil {
			return wsTicket{}, false
		}
		strVal, ok := val.(string)
		if !ok {
			return wsTicket{}, false
		}
		var t wsTicket
		if err := json.Unmarshal([]byte(strVal), &t); err != nil {
			return wsTicket{}, false
		}
		return t, true
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tickets[subject]
	if !ok || t.Consumed || time.Now().After(t.ExpiresAt) {
		return wsTicket{}, false
	}
	t.Consumed = true
	s.tickets[subject] = t
	return t, true
}

func (s *wsTicketStore) peek(subject string) (wsTicket, bool) {
	if s.cfg.RedisEnabled && s.cfg.Redis != nil {
		val, err := s.cfg.Redis.Get(context.Background(), "ws_ticket:"+subject).Result()
		if err != nil {
			return wsTicket{}, false
		}
		var t wsTicket
		if err := json.Unmarshal([]byte(val), &t); err != nil {
			return wsTicket{}, false
		}
		if t.Consumed || time.Now().After(t.ExpiresAt) {
			return wsTicket{}, false
		}
		return t, true
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tickets[subject]
	if !ok || t.Consumed || time.Now().After(t.ExpiresAt) {
		return wsTicket{}, false
	}
	return t, true
}

// IssueWSTicket issues a short-lived (60s) single-use ticket for a server WS
// connection. The ticket is signed with the API auth secret. The frontend
// should pass it in `?token=<ticket>` (replacing the JWT in the query string).
// On WS upgrade the daemon / realtime proxy validate the ticket signature
// and check that the subject matches.
func IssueWSTicket(cfg Config, store *wsTicketStore) fiber.Handler {
	return func(c *fiber.Ctx) error {
		serverID := c.Params("id")
		if serverID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "server id required")
		}
		stream := c.Query("stream", "console")
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok || claims.Sub == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "missing user")
		}
		// Apply the same OAuth server binding, scope, and current-user permission
		// checks as every other server endpoint.
		if err := checkServerPermission(c, cfg, "websocket.connect"); err != nil {
			return err
		}
		// Generate ticket.
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not generate websocket ticket")
		}
		ticketID := hex.EncodeToString(raw)
		expiresAt := time.Now().Add(60 * time.Second)
		subject := ticketID
		token := signTicket(cfg.AuthSecret, subject)
		store.put(wsTicket{
			Subject:   subject,
			UserID:    claims.Sub,
			ServerID:  serverID,
			Stream:    stream,
			ExpiresAt: expiresAt,
		})
		return c.JSON(fiber.Map{
			"token":     token,
			"expiresAt": expiresAt.UTC().Format(time.RFC3339),
			"server":    serverID,
			"stream":    stream,
		})
	}
}

// VerifyWSTicket validates a WS ticket and returns the server+stream it was
// issued for. Returns false if the ticket is missing, malformed, expired,
// or has already been consumed.
func verifyWSTicketSignature(cfg Config, token string) (string, bool) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return "", false
	}
	subject, sig := parts[0], parts[1]
	mac := hmac.New(sha256.New, []byte(cfg.AuthSecret))
	mac.Write([]byte(subject))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return subject, hmac.Equal([]byte(sig), []byte(expected))
}

// inspectWSTicket validates the signature and returns an unconsumed ticket.
// Callers must consume only after every connection check has succeeded.
func inspectWSTicket(cfg Config, store *wsTicketStore, token string) (wsTicket, bool) {
	subject, ok := verifyWSTicketSignature(cfg, token)
	if !ok {
		return wsTicket{}, false
	}
	return store.peek(subject)
}

func consumeWSTicket(cfg Config, store *wsTicketStore, token string) bool {
	subject, ok := verifyWSTicketSignature(cfg, token)
	if !ok {
		return false
	}
	_, ok = store.consume(subject)
	return ok
}

// VerifyWSTicket is retained for callers that need an atomic validate-and-consume operation.
func VerifyWSTicket(cfg Config, store *wsTicketStore, token string) (serverID, stream string, ok bool) {
	t, ok := inspectWSTicket(cfg, store, token)
	if !ok || !consumeWSTicket(cfg, store, token) {
		return "", "", false
	}
	return t.ServerID, t.Stream, true
}

func (s *wsTicketStore) Peek(token string) (wsTicket, bool) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return wsTicket{}, false
	}
	return s.peek(parts[0])
}

// signTicket returns the signed ticket token for a given subject. The
// signature is the URL-safe base64 of HMAC-SHA256(secret, subject).
func signTicket(secret, subject string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(subject))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return subject + "." + sig
}
