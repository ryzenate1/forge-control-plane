package http

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/services/domainsenv"
	"gamepanel/forge/internal/services/envgroups"
	"gamepanel/forge/internal/services/envmanifest"
	"gamepanel/forge/internal/services/envvars"

	"gamepanel/forge/internal/store"

	fiberws "github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
)

// envLogTicket is a short-lived, single-use credential for the per-service
// log WebSocket. It binds the issuing user to one environment + service so a
// leaked URL cannot be replayed for another env/service or by another user.
type envLogTicket struct {
	Subject   string    `json:"subject"`
	UserID    string    `json:"userId"`
	EnvID     string    `json:"envId"`
	Service   string    `json:"service"`
	ExpiresAt time.Time `json:"expiresAt"`
	Consumed  bool      `json:"consumed"`
}

type envLogTicketStore struct {
	mu      sync.RWMutex
	tickets map[string]envLogTicket
	cfg     *Config
}

func newEnvLogTicketStore(cfg *Config) *envLogTicketStore {
	return &envLogTicketStore{tickets: make(map[string]envLogTicket), cfg: cfg}
}

func (s *envLogTicketStore) useShared() bool {
	return s != nil && s.cfg != nil && s.cfg.RedisEnabled && s.cfg.Redis != nil
}

func signEnvLogTicket(secret, subject string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("forge:env-log-ticket:v1\x00"))
	mac.Write([]byte(subject))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return subject + "." + sig
}

func verifyEnvLogTicketSignature(secret, token string) (string, bool) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return "", false
	}
	subject, sig := parts[0], parts[1]
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("forge:env-log-ticket:v1\x00"))
	mac.Write([]byte(subject))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return subject, hmac.Equal([]byte(sig), []byte(expected))
}

func (s *envLogTicketStore) put(t envLogTicket) {
	if s.useShared() {
		data, err := json.Marshal(t)
		if err == nil {
			ttl := time.Until(t.ExpiresAt)
			if ttl > 0 {
				_ = s.cfg.Redis.Set(context.Background(), "env_log_ticket:"+t.Subject, data, ttl).Err()
			}
		}
		return
	}
	s.mu.Lock()
	s.tickets[t.Subject] = t
	s.mu.Unlock()
	time.AfterFunc(time.Until(t.ExpiresAt)+30*time.Second, func() {
		s.mu.Lock()
		delete(s.tickets, t.Subject)
		s.mu.Unlock()
	})
}

func (s *envLogTicketStore) peek(subject string) (envLogTicket, bool) {
	if s.useShared() {
		val, err := s.cfg.Redis.Get(context.Background(), "env_log_ticket:"+subject).Result()
		if err != nil {
			return envLogTicket{}, false
		}
		var t envLogTicket
		if err := json.Unmarshal([]byte(val), &t); err != nil {
			return envLogTicket{}, false
		}
		if t.Consumed || time.Now().After(t.ExpiresAt) {
			return envLogTicket{}, false
		}
		return t, true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tickets[subject]
	if !ok || t.Consumed || time.Now().After(t.ExpiresAt) {
		return envLogTicket{}, false
	}
	return t, true
}

func (s *envLogTicketStore) consume(subject string) (envLogTicket, bool) {
	if s.useShared() {
		val, err := consumeScript.Run(context.Background(), s.cfg.Redis, []string{"env_log_ticket:" + subject}).Result()
		if err != nil || val == nil {
			return envLogTicket{}, false
		}
		strVal, ok := val.(string)
		if !ok {
			return envLogTicket{}, false
		}
		var t envLogTicket
		if err := json.Unmarshal([]byte(strVal), &t); err != nil {
			return envLogTicket{}, false
		}
		if t.Consumed || time.Now().After(t.ExpiresAt) {
			return envLogTicket{}, false
		}
		return t, true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tickets[subject]
	if !ok || t.Consumed || time.Now().After(t.ExpiresAt) {
		return envLogTicket{}, false
	}
	t.Consumed = true
	s.tickets[subject] = t
	return t, true
}

func inspectEnvLogTicket(secret string, st *envLogTicketStore, token string) (envLogTicket, bool) {
	subject, ok := verifyEnvLogTicketSignature(secret, token)
	if !ok {
		return envLogTicket{}, false
	}
	return st.peek(subject)
}

// validateEnvLogWSUpgrade validates the ticket + session + org binding for an
// env-log WebSocket upgrade. It inspects (not consumes) the ticket; callers
// consume only after every check succeeds so a failed upgrade never burns a
// legitimate ticket.
func validateEnvLogWSUpgrade(conn *fiberws.Conn, cfg *Config, st *envLogTicketStore) (envLogTicket, tokenClaims, error) {
	if cfg == nil || cfg.Store == nil || st == nil {
		return envLogTicket{}, tokenClaims{}, fiber.NewError(fiber.StatusServiceUnavailable, "store unavailable")
	}
	raw := conn.Query("token")
	if strings.TrimSpace(raw) == "" {
		return envLogTicket{}, tokenClaims{}, fiber.NewError(fiber.StatusUnauthorized, "log stream ticket is required")
	}
	ticket, ok := inspectEnvLogTicket(cfg.AuthSecret, st, raw)
	if !ok {
		return envLogTicket{}, tokenClaims{}, fiber.NewError(fiber.StatusUnauthorized, "invalid or expired log stream ticket")
	}
	if ticket.EnvID != conn.Params("id") || ticket.Service != conn.Params("service") {
		return envLogTicket{}, tokenClaims{}, fiber.NewError(fiber.StatusForbidden, "ticket does not match this log stream")
	}
	sessionToken := wsSessionToken(conn)
	if sessionToken == "" {
		return envLogTicket{}, tokenClaims{}, fiber.NewError(fiber.StatusUnauthorized, "ticket requires session cookie or Authorization header")
	}
	claims, err := parseToken(cfg.AuthSecret, sessionToken)
	if err != nil {
		return envLogTicket{}, tokenClaims{}, fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}
	lookupCtx, lookupCancel := sessionLookupCtx()
	validated, err := validateCurrentSession(lookupCtx, cfg.Store, claims)
	lookupCancel()
	if err != nil {
		return envLogTicket{}, tokenClaims{}, fiber.NewError(fiber.StatusUnauthorized, "invalid or revoked session")
	}
	if validated.Sub != ticket.UserID {
		return envLogTicket{}, tokenClaims{}, fiber.NewError(fiber.StatusUnauthorized, "ticket identity mismatch")
	}
	// Org authorization mirrors phase2EnvAccess: admins short-circuit,
	// others must be members of the environment's org. Fail closed on error.
	orgCtx, orgCancel := sessionLookupCtx()
	envCtx, err := cfg.Store.ResolveEnvContext(orgCtx, ticket.EnvID)
	orgCancel()
	if err != nil {
		return envLogTicket{}, tokenClaims{}, fiber.NewError(fiber.StatusNotFound, "environment not found")
	}
	if envCtx.Environment.ID != ticket.EnvID {
		return envLogTicket{}, tokenClaims{}, fiber.NewError(fiber.StatusForbidden, "ticket does not match this log stream")
	}
	if validated.Role != "admin" {
		memberCtx, memberCancel := sessionLookupCtx()
		member, err := cfg.Store.UserIsOrgMember(memberCtx, envCtx.Org.ID, validated.Sub)
		memberCancel()
		if err != nil || !member {
			return envLogTicket{}, tokenClaims{}, fiber.NewError(fiber.StatusForbidden, "not a member of the environment's organization")
		}
	}
	// Re-check 2FA posture: the session's 2FA state can change within the
	// ticket's window, so enforce the panel policy on upgrade as well.
	twoFactorCtx, twoFactorCancel := context.WithTimeout(context.Background(), 5*time.Second)
	twoFactorErr := checkRealtimeTwoFactor(twoFactorCtx, *cfg, validated)
	twoFactorCancel()
	if twoFactorErr != nil {
		return envLogTicket{}, tokenClaims{}, fiber.NewError(fiber.StatusForbidden, "two-factor authentication is required")
	}
	return ticket, validated, nil
}

// Phase 2 — Environment engine.
//
// Registers the environment-manifest API surface (apply/render/ports-urls),
// env-var group CRUD, per-environment service log streaming and the domain
// provisioning trigger. Route wiring is opt-in per middleware convention:
// everything lives on the protected router behind session auth.

func init() {
	RegisterPhaseRegistrar("phase2-environment-engine", 2000, registerPhase2EnvironmentEngine)
}

func registerPhase2EnvironmentEngine(v1 fiber.Router, protected fiber.Router, cfg *Config) error {
	if cfg == nil || cfg.Store == nil {
		return fmt.Errorf("%w: store not configured, environment engine routes not mounted", ErrPhaseSkipped)
	}

	// Injected via Config (handlers -> services -> store); inline construction
	// is the dev/test fallback when main has not populated the fields.
	manifestSvc := cfg.EnvManifestService
	if manifestSvc == nil {
		manifestSvc = envmanifest.New(cfg.Store)
		manifestSvc.WithEnvVars(cfg.Store)
	}

	groupsSvc := cfg.EnvGroupsService
	if groupsSvc == nil {
		groupsSvc = envgroups.New(cfg.Store)
	}

	domainSvc := cfg.DomainsEnvService
	if domainSvc == nil {
		domainOpts := domainsenv.OptionsFromEnv(nil)
		domainSvc = domainsenv.New(cfg.Store, cfg.AcmeService, domainOpts, cfg.Logger)
	}

	// Reconciler: ensures wildcard CNAME + TLS intents for every environment.
	if cfg.BackgroundContext != nil {
		domainSvc.Start(cfg.BackgroundContext)
	}

	envLogTickets := newEnvLogTicketStore(cfg)

	// ---- Environment manifest ----

	protected.Get("/envs/:id/manifest", func(c *fiber.Ctx) error {
		envCtx, err := phase2EnvAccess(c, cfg)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		manifest, yaml, err := manifestSvc.Render(ctx, envCtx.Environment.ID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if manifest == nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "no manifest applied yet"})
		}
		return c.JSON(fiber.Map{"manifest": manifest, "yaml": yaml, "schema": envmanifest.Schema()})
	})

	protected.Put("/envs/:id/manifest", func(c *fiber.Ctx) error {
		envCtx, err := phase2EnvAccess(c, cfg)
		if err != nil {
			return err
		}
		body := c.Body()

		// Accept either a raw manifest document (YAML or JSON) as the body,
		// or {"manifest": "...", "override": {"K":"V"}} to pass overrides.
		doc := body
		override := map[string]string{}
		if len(body) > 0 && (body[0] == '{' || body[0] == '[') {
			var wrapped struct {
				Manifest string            `json:"manifest"`
				Override map[string]string `json:"override,omitempty"`
			}
			if err := c.BodyParser(&wrapped); err == nil && wrapped.Manifest != "" {
				doc = []byte(wrapped.Manifest)
				override = wrapped.Override
			}
		}

		parsed, err := envmanifest.Parse(doc)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}

		ctx, cancel := requestContext()
		defer cancel()
		actorID := phase2ActorID(c)
		result, err := manifestSvc.Apply(ctx, envCtx.Environment.ID, parsed, override, string(doc), &actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(result)
	})

	protected.Get("/envs/:id/ports-urls", func(c *fiber.Ctx) error {
		envCtx, err := phase2EnvAccess(c, cfg)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		view, err := manifestSvc.PortsURLs(ctx, envCtx.Environment.ID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(view)
	})

	// ---- Env-var groups ----

	protected.Get("/envs/:id/groups", func(c *fiber.Ctx) error {
		envCtx, err := phase2EnvAccess(c, cfg)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		groups, err := groupsSvc.List(ctx, envCtx.Environment.ID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(groups)
	})

	protected.Put("/envs/:id/groups/:name", func(c *fiber.Ctx) error {
		envCtx, err := phase2EnvAccess(c, cfg)
		if err != nil {
			return err
		}
		var req struct {
			Description string            `json:"description"`
			Variables   map[string]string `json:"variables"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		actorID := phase2ActorID(c)
		ctx, cancel := requestContext()
		defer cancel()
		group, err := groupsSvc.Save(ctx, envCtx.Environment.ID, c.Params("name"), req.Description, req.Variables, &actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(group)
	})

	protected.Delete("/envs/:id/groups/:name", func(c *fiber.Ctx) error {
		envCtx, err := phase2EnvAccess(c, cfg)
		if err != nil {
			return err
		}
		actorID := phase2ActorID(c)
		ctx, cancel := requestContext()
		defer cancel()
		if err := groupsSvc.Delete(ctx, envCtx.Environment.ID, c.Params("name"), &actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// Apply a group bundle onto the environment's resolved variables.
	protected.Post("/envs/:id/groups/:name/apply", func(c *fiber.Ctx) error {
		envCtx, err := phase2EnvAccess(c, cfg)
		if err != nil {
			return err
		}
		actorID := phase2ActorID(c)
		ctx, cancel := requestContext()
		defer cancel()
		group, err := groupsSvc.Get(ctx, envCtx.Environment.ID, c.Params("name"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		var applier envVarApplicator
		if cfg.EnvVarService != nil {
			applier = envVarGroupApplier{svc: cfg.EnvVarService}
		}
		applied, err := applyGroupEnvVars(ctx, applier, envCtx.Environment.ID, group.Variables, &actorID)
		if err != nil {
			var fe *fiber.Error
			if errors.As(err, &fe) {
				return fe
			}
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "applied": applied})
	})

	// ---- Per-environment service logs ----

	protected.Get("/environments/:id/services/:service/logs", func(c *fiber.Ctx) error {
		envCtx, err := phase2EnvAccess(c, cfg)
		if err != nil {
			return err
		}
		since, err := parseSinceQuery(c)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		ctx, cancel := requestContext()
		defer cancel()
		entries, err := cfg.Store.ListEnvServiceLogs(ctx, envCtx.Environment.ID, c.Params("service"), since, 1000)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(entries)
	})

	// Short-lived ticket for the service-log WebSocket, minted behind the
	// same org authorization as every other env route (like IssueWSTicket).
	protected.Post("/envs/:id/services/:service/logs/ticket", func(c *fiber.Ctx) error {
		envCtx, err := phase2EnvAccess(c, cfg)
		if err != nil {
			return err
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok || claims.Sub == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		service := strings.TrimSpace(c.Params("service"))
		if service == "" {
			return fiber.NewError(fiber.StatusBadRequest, "service is required")
		}
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not generate log stream ticket")
		}
		subject := hex.EncodeToString(raw)
		expiresAt := time.Now().Add(60 * time.Second)
		token := signEnvLogTicket(cfg.AuthSecret, subject)
		envLogTickets.put(envLogTicket{
			Subject:   subject,
			UserID:    claims.Sub,
			EnvID:     envCtx.Environment.ID,
			Service:   service,
			ExpiresAt: expiresAt,
		})
		return c.JSON(fiber.Map{
			"token":     token,
			"expiresAt": expiresAt.UTC().Format(time.RFC3339),
			"env":       envCtx.Environment.ID,
			"service":   service,
		})
	})

	protected.Get("/envs/:id/services/:service/logs/ws", fiberws.New(func(conn *fiberws.Conn) {
		defer conn.Close()
		if cfg.Store == nil {
			_ = conn.WriteJSON(fiber.Map{"error": "store unavailable"})
			return
		}
		// Ticket + session + org are validated on upgrade, before any log
		// data flows. A failed upgrade never consumes the ticket.
		ticket, _, err := validateEnvLogWSUpgrade(conn, cfg, envLogTickets)
		if err != nil {
			_ = conn.WriteJSON(fiber.Map{"error": "unauthorized"})
			return
		}
		if _, ok := envLogTickets.consume(ticket.Subject); !ok {
			_ = conn.WriteJSON(fiber.Map{"error": "invalid or expired log stream ticket"})
			return
		}
		envID := ticket.EnvID
		service := ticket.Service

		var since *time.Time
		if raw := conn.Query("since"); raw != "" {
			if t, perr := time.Parse(time.RFC3339, raw); perr == nil {
				since = &t
			}
		}

		streamCtx, stopStream := context.WithCancel(context.Background())
		defer stopStream()
		// The websocket read loop is the only reliable disconnect signal, so a
		// failed read cancels the log tail.
		go func() {
			defer stopStream()
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		last := since
		for {
			select {
			case <-streamCtx.Done():
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				entries, err := cfg.Store.ListEnvServiceLogs(ctx, envID, service, last, 500)
				cancel()
				if err != nil {
					continue
				}
				for i := range entries {
					if last == nil || entries[i].CreatedAt.After(*last) {
						last = &entries[i].CreatedAt
					}
					if werr := conn.WriteJSON(entries[i]); werr != nil {
						return
					}
				}
			}
		}
	}))

	// ---- Domain provisioning trigger ----

	protected.Post("/envs/:id/domains/provision", func(c *fiber.Ctx) error {
		envCtx, err := phase2EnvAccess(c, cfg)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		info, err := domainSvc.Provision(ctx, envCtx.Environment.ID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "info": info})
	})

	return nil
}

// phase2EnvAccess resolves the target environment and verifies the caller
// belongs to the owning org (admin short-circuits).
func phase2EnvAccess(c *fiber.Ctx, cfg *Config) (store.EnvContext, error) {
	claims, ok := c.Locals("user").(tokenClaims)
	if !ok {
		return store.EnvContext{}, fiber.NewError(fiber.StatusUnauthorized, "missing session")
	}
	if scoped, _ := c.Locals("scopedAuth").(bool); scoped {
		return store.EnvContext{}, fiber.NewError(fiber.StatusForbidden, "scoped credentials cannot access environment resources")
	}
	ctx, cancel := requestContext()
	defer cancel()
	envCtx, err := cfg.Store.ResolveEnvContext(ctx, c.Params("id"))
	if err != nil {
		return store.EnvContext{}, fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	if claims.Role == "admin" {
		return envCtx, nil
	}
	member, err := cfg.Store.UserIsOrgMember(ctx, envCtx.Org.ID, claims.Sub)
	if err != nil {
		return store.EnvContext{}, fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	if !member {
		return store.EnvContext{}, fiber.NewError(fiber.StatusForbidden, "not a member of the environment's organization")
	}
	return envCtx, nil
}

// phase2ActorID returns the calling user id (empty when unauthenticated).
func phase2ActorID(c *fiber.Ctx) string {
	if claims, ok := c.Locals("user").(tokenClaims); ok {
		return claims.Sub
	}
	return ""
}

// parseSinceQuery parses the optional ?since=RFC3339 query parameter.
func parseSinceQuery(c *fiber.Ctx) (*time.Time, error) {
	raw := c.Query("since")
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// applyGroupEnvVars upserts each group variable at environment-scope using
// the env-var service. A nil store is a 503 (unknown is not zero): answering
// ok:true with applied:0 would claim success that never happened. Upsert
// failures propagate so a partial apply is never reported as fully applied.
func applyGroupEnvVars(ctx context.Context, envVarStore envVarApplicator, envID string, variables map[string]string, actorID *string) (int, error) {
	if envVarStore == nil {
		return 0, fiber.NewError(fiber.StatusServiceUnavailable, "environment variable service is not available")
	}
	existing, err := envVarStore.List(ctx, "environment", envID)
	if err != nil {
		return 0, err
	}
	applied := 0
	for key, value := range variables {
		if key == "" || value == "" {
			continue
		}
		if _, err := envVarStore.Upsert(ctx, envID, key, value, actorID, existing); err != nil {
			return applied, err
		}
		applied++
	}
	return applied, nil
}

// envVarApplicator is the env-var surface needed to apply group bundles.
type envVarApplicator interface {
	List(ctx context.Context, scopeType, scopeID string) ([]store.EnvironmentVariable, error)
	Upsert(ctx context.Context, envID, key, value string, actorID *string, existing []store.EnvironmentVariable) (store.EnvironmentVariable, error)
}

// envVarGroupApplier adapts *envvars.Service to envVarApplicator.
type envVarGroupApplier struct {
	svc *envvars.Service
}

func (a envVarGroupApplier) List(ctx context.Context, scopeType, scopeID string) ([]store.EnvironmentVariable, error) {
	return a.svc.List(ctx, scopeType, scopeID)
}

func (a envVarGroupApplier) Upsert(ctx context.Context, envID, key, value string, actorID *string, existing []store.EnvironmentVariable) (store.EnvironmentVariable, error) {
	for _, v := range existing {
		if v.Key == key {
			return a.svc.Update(ctx, v.ID, envvars.UpdateEnvVarInput{Value: value, Actor: actorID})
		}
	}
	return a.svc.Create(ctx, envvars.CreateEnvVarInput{
		EnvironmentID: &envID,
		Scope:         "environment",
		Key:           key,
		Value:         value,
		Actor:         actorID,
	})
}
