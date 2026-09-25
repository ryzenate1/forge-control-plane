package http

import (
	"errors"
	"log/slog"
	"strings"

	"gamepanel/forge/internal/services/redirects"
	"gamepanel/forge/internal/services/trafficmanager"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// Per-application domain redirects and forwards.
//
// The Dokploy counterpart is the `redirects` tRPC router plus
// `utils/traefik/redirect.ts`; here the routes are this file and the behaviour
// lives entirely in internal/services/redirects. Nothing is added to NewServer:
// the feature mounts itself through the phase registrar, so it is additive and
// disappears cleanly if the database is not there.
//
// Two things deliberately do NOT live here:
//
//   - The admin proxy-domain redirects (/domains/:domainId/redirects in
//     handlers_proxy_domains.go, backed by the `redirect_rules` table). Those
//     are keyed to a domain and edited by an administrator; these are keyed to
//     an application and edited by whoever owns the application. Same word,
//     different feature, so the table, the route tree and the service package
//     are all separate.
//   - Writing the rendered config to Traefik or Caddy. The traffic manager owns
//     the gateway adapter, and its adapter interface accepts routing rules,
//     not redirect rules. POST /apply therefore recomputes the config, triggers
//     the traffic sync it can legitimately trigger, and reports exactly what
//     happened instead of implying the rules were published.

const (
	domainRedirectsRegistrarName = "domain-redirects"
	// domainRedirectsPriority sits between app-mount-management (210) and
	// container-files (220) — another per-application settings surface. The
	// absolute number only decides order among phases, and no other phase claims
	// /apps/:appId/redirects.
	domainRedirectsPriority = 218
)

func init() {
	RegisterPhaseRegistrar(domainRedirectsRegistrarName, domainRedirectsPriority, registerDomainRedirectsPhase)
}

// registerDomainRedirectsPhase mounts /apps/:appId/redirects.
//
// Without a Postgres pool the routes are skipped (logged, non-fatal): redirect
// rules have no in-memory fallback, and a route that fails on every request is
// worse than a 404 that tells the operator the feature is unavailable.
func registerDomainRedirectsPhase(v1 fiber.Router, protected fiber.Router, cfg *Config) error {
	_ = v1
	if cfg == nil || cfg.Store == nil || protected == nil {
		return nil
	}
	pool := cfg.Store.GetDB()
	if pool == nil {
		if cfg.Logger != nil {
			cfg.Logger.Warn(domainRedirectsRegistrarName + ": no postgres pool, domain redirect routes skipped")
		}
		return nil
	}
	// Handler factories take Config by value, which is how every route in this
	// package receives it; the registrar contract hands over a pointer, so it is
	// dereferenced once here instead of at every call site.
	conf := *cfg
	svc := redirects.NewFromPool(pool)
	mutationLimiter := RateLimiter(GetRateLimitForEndpoint("mutation", cfg.Redis,
		cfg.RedisEnabled && strings.EqualFold(strings.TrimSpace(cfg.AppEnv), "production")))

	// Registered as full paths rather than a /apps/:appId/redirects group: a
	// Group + Get("/") pair registers a trailing-slash route, which turns the
	// list endpoint into a 301 the browser has to follow.
	//
	// The static /presets and /apply paths are declared before the :redirectId
	// routes because Fiber matches in registration order — declared later,
	// PATCH /redirects/apply would be a plausible :redirectId.
	protected.Get("/apps/:appId/redirects", domainRedirectList(conf, svc))
	protected.Post("/apps/:appId/redirects", mutationLimiter, domainRedirectCreate(conf, svc))
	protected.Post("/apps/:appId/redirects/presets", mutationLimiter, domainRedirectPresets(conf, svc))
	protected.Post("/apps/:appId/redirects/apply", mutationLimiter, domainRedirectApply(conf, svc, cfg.TrafficManager))
	protected.Patch("/apps/:appId/redirects/:redirectId", mutationLimiter, domainRedirectUpdate(conf, svc))
	protected.Delete("/apps/:appId/redirects/:redirectId", mutationLimiter, domainRedirectDelete(conf, svc))
	return nil
}

// ---------------------------------------------------------------------------
// Access control and error translation
// ---------------------------------------------------------------------------

// authorizedApplicationForRedirects resolves :appId to an application the
// caller may edit. It deliberately delegates: the rule is exactly the one the
// process-configuration routes apply (global admin, or a member of the owning
// organization; a foreign application answers 404 so membership is not
// enumerable through this API), and a second implementation would be a second
// thing to keep correct.
func authorizedApplicationForRedirects(c *fiber.Ctx, cfg Config) (*store.Application, error) {
	return authorizedApplicationForProcesses(c, cfg)
}

// redirectParam reads :redirectId. An id that reaches the service empty would
// otherwise be reported as "not found" for a request that never named a rule.
func redirectParam(c *fiber.Ctx) (string, error) {
	raw := strings.TrimSpace(c.Params("redirectId"))
	if raw == "" {
		return "", fiber.NewError(fiber.StatusBadRequest, "redirect id is required")
	}
	return raw, nil
}

// redirectError translates the service's sentinels into HTTP statuses.
// errors.Is/errors.As are used rather than message matching, so rewording an
// error cannot silently move an endpoint from 400 to 500.
func redirectError(c *fiber.Ctx, err error) error {
	if err == nil {
		return nil
	}
	var already *fiber.Error
	if errors.As(err, &already) {
		return already
	}
	switch {
	case errors.Is(err, redirects.ErrRedirectNotFound), errors.Is(err, redirects.ErrApplicationNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, redirects.ErrEmptyAppID):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	case errors.Is(err, redirects.ErrDuplicateRedirect),
		errors.Is(err, redirects.ErrCircularRedirect),
		errors.Is(err, redirects.ErrSelfRedirect):
		// All three describe a rule that conflicts with the rest of this
		// application's rule set rather than a malformed request, which is what
		// 409 means. The panel shows the message verbatim, so the operator sees
		// *which* rule it collided with.
		return fiber.NewError(fiber.StatusConflict, err.Error())
	case errors.Is(err, redirects.ErrTooManyRules):
		// 409 would suggest a name clash; 400 would suggest a typo. The request
		// is well-formed and unprocessable only because of the existing state
		// limit, which is 422.
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	}
	var validation *redirects.ValidationError
	if errors.As(err, &validation) {
		return fiber.NewError(fiber.StatusBadRequest, validation.Message)
	}
	return respondInternalError(c, err)
}

// ---------------------------------------------------------------------------
// /apps/:appId/redirects
// ---------------------------------------------------------------------------

// domainRedirectList returns every rule for the application. The response is
// the panel's source of truth: an empty list is `[]`, never null, so the UI can
// distinguish "no rules yet" from a request that failed.
func domainRedirectList(cfg Config, svc *redirects.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		app, err := authorizedApplicationForRedirects(c, cfg)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		rules, err := svc.List(ctx, app.ID)
		if err != nil {
			return redirectError(c, err)
		}
		return c.JSON(fiber.Map{
			"data":          rules,
			"applicationId": app.ID,
			"limit":         redirects.MaxRulesPerApplication,
		})
	}
}

func domainRedirectCreate(cfg Config, svc *redirects.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		app, err := authorizedApplicationForRedirects(c, cfg)
		if err != nil {
			return err
		}
		if len(strings.TrimSpace(string(c.Body()))) == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "a redirect body is required")
		}
		var req redirects.CreateRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body: "+err.Error())
		}
		ctx, cancel := requestContext()
		defer cancel()
		created, err := svc.Create(ctx, app.ID, req)
		if err != nil {
			return redirectError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": created})
	}
}

func domainRedirectUpdate(cfg Config, svc *redirects.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		app, err := authorizedApplicationForRedirects(c, cfg)
		if err != nil {
			return err
		}
		redirectID, err := redirectParam(c)
		if err != nil {
			return err
		}
		if len(strings.TrimSpace(string(c.Body()))) == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "a redirect body is required")
		}
		var req redirects.UpdateRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body: "+err.Error())
		}
		ctx, cancel := requestContext()
		defer cancel()
		updated, err := svc.Update(ctx, app.ID, redirectID, req)
		if err != nil {
			return redirectError(c, err)
		}
		return c.JSON(fiber.Map{"data": updated})
	}
}

// domainRedirectDelete answers 204 only when a row was actually removed. The
// service already reports "no such rule for this application" as an error, so a
// successful 404-vs-204 confusion is not possible here.
func domainRedirectDelete(cfg Config, svc *redirects.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		app, err := authorizedApplicationForRedirects(c, cfg)
		if err != nil {
			return err
		}
		redirectID, err := redirectParam(c)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.Delete(ctx, app.ID, redirectID); err != nil {
			return redirectError(c, err)
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}

// ---------------------------------------------------------------------------
// /apps/:appId/redirects/presets
// ---------------------------------------------------------------------------

// domainRedirectPresets bulk-creates the common redirect patterns from the
// hosts the application is already bound to.
//
// The response is always 200 when the request itself was sound: a preset that
// created nothing because the host is not bound is an answer about the
// application's domains, not a failed request, and it is reported in
// `skipped[]` with the reason. A client that needs to know whether anything
// changed can compare `created.length`.
func domainRedirectPresets(cfg Config, svc *redirects.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		app, err := authorizedApplicationForRedirects(c, cfg)
		if err != nil {
			return err
		}
		var req struct {
			Presets []string `json:"presets"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body: "+err.Error())
		}
		presets := make([]redirects.PresetType, 0, len(req.Presets))
		for _, raw := range req.Presets {
			preset, err := redirects.ParsePresetType(raw)
			if err != nil {
				var validation *redirects.ValidationError
				if errors.As(err, &validation) {
					return fiber.NewError(fiber.StatusBadRequest, validation.Message)
				}
				return redirectError(c, err)
			}
			if preset == "" {
				return fiber.NewError(fiber.StatusBadRequest, "a preset name must not be empty")
			}
			presets = append(presets, preset)
		}
		ctx, cancel := requestContext()
		defer cancel()
		result, err := svc.ApplyPresets(ctx, app.ID, presets)
		if err != nil {
			return redirectError(c, err)
		}
		return c.JSON(fiber.Map{
			"applicationId": result.ApplicationID,
			"created":       result.Created,
			"skipped":       result.Skipped,
		})
	}
}

// ---------------------------------------------------------------------------
// /apps/:appId/redirects/apply
// ---------------------------------------------------------------------------

// domainRedirectApply recomputes the gateway configuration for this
// application's enabled rules and triggers the traffic manager's sync.
//
// What it does *not* do is claim the redirects are live. GenerateGatewayConfig
// renders Traefik middleware blocks and Caddy route fragments for the stored
// rules; publishing them is the traffic manager's job, and its adapter accepts
// routing rules rather than redirect rules. So the response separates the two
// facts — `rules` were generated, `gatewaySynced` says whether the gateway
// reload succeeded — and explains a missing traffic manager instead of
// reporting success. A sync failure is a 200 with the reason attached: the
// requested recomputation did happen and the stored data is intact, so 5xx
// would tell the operator to go delete-and-retry something that is fine.
func domainRedirectApply(cfg Config, svc *redirects.Service, tm *trafficmanager.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		app, err := authorizedApplicationForRedirects(c, cfg)
		if err != nil {
			return err
		}
		// A gateway reload can legitimately outlive the 5s request budget every
		// other route in this file uses.
		ctx, cancel := longRequestContext()
		defer cancel()

		rules, err := svc.GenerateGatewayConfig(ctx, app.ID)
		if err != nil {
			return redirectError(c, err)
		}
		payload := fiber.Map{
			"applicationId": app.ID,
			"rules":         rules,
			"ruleCount":     len(rules),
			"gatewaySynced": false,
		}
		if tm == nil {
			payload["syncDetail"] = "the traffic manager is not configured on this panel, so no gateway was reloaded; the generated rules are returned unchanged"
			return c.JSON(payload)
		}
		payload["adapter"] = string(tm.AdapterKind(ctx))
		if err := tm.SyncRoutes(ctx); err != nil {
			if cfg.Logger != nil {
				cfg.Logger.Error("traffic sync after redirect apply failed",
					slog.String("appId", app.ID), slog.String("error", err.Error()))
			}
			payload["syncDetail"] = "the rules were regenerated but the gateway sync failed: " + err.Error()
			return c.JSON(payload)
		}
		payload["gatewaySynced"] = true
		payload["syncDetail"] = "the gateway routing table was resynced; the generated redirect rules are part of this application's deploy-time config and are returned above for the caller that publishes them"
		return c.JSON(payload)
	}
}
