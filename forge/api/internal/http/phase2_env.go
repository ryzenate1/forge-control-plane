package http

import (
	"context"
	"time"

	"gamepanel/forge/internal/services/domainsenv"
	"gamepanel/forge/internal/services/envgroups"
	"gamepanel/forge/internal/services/envmanifest"
	"gamepanel/forge/internal/services/envvars"

	"gamepanel/forge/internal/store"

	fiberws "github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
)

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
		return nil
	}

	manifestSvc := envmanifest.New(cfg.Store)
	manifestSvc.WithEnvVars(cfg.Store)

	groupsSvc := envgroups.New(cfg.Store)

	domainOpts := domainsenv.OptionsFromEnv(nil)
	domainSvc := domainsenv.New(cfg.Store, cfg.AcmeService, domainOpts, cfg.Logger)

	// Reconciler: ensures wildcard CNAME + TLS intents for every environment.
	if cfg.BackgroundContext != nil {
		domainSvc.Start(cfg.BackgroundContext)
	}

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

	protected.Get("/envs/:id/services/:service/logs/ws", fiberws.New(func(conn *fiberws.Conn) {
		defer conn.Close()
		if cfg.Store == nil {
			_ = conn.WriteJSON(fiber.Map{"error": "store unavailable"})
			return
		}
		envID := conn.Params("id")
		service := conn.Params("service")

		var since *time.Time
		if raw := conn.Query("since"); raw != "" {
			if t, perr := time.Parse(time.RFC3339, raw); perr == nil {
				since = &t
			}
		}

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		last := since
		for {
			select {
			case <-conn.Context().Done():
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
// the env-var service.
func applyGroupEnvVars(ctx context.Context, envVarStore envVarApplicator, envID string, variables map[string]string, actorID *string) (int, error) {
	if envVarStore == nil {
		return 0, nil
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
			continue
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
