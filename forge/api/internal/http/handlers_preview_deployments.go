package http

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"gamepanel/forge/internal/services/git"
	"gamepanel/forge/internal/services/previewenv"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// previewDeploymentService is the contract the preview-deployment admin API
// needs. It is satisfied by the real services/previewenv implementation (which
// derives per-PR URLs, provisions TLS/routing and reports commit status) so the
// endpoint is backed by a genuine lifecycle rather than a stub that only flips
// status and fabricates URLs.
type previewDeploymentService interface {
	ListAll(ctx context.Context) ([]store.PreviewDeployment, error)
	Get(ctx context.Context, id string) (*store.PreviewDeployment, error)
	Create(ctx context.Context, serverID string, req *store.PreviewDeployment) (*store.PreviewDeployment, error)
	Deploy(ctx context.Context, id string) error
	Cleanup(ctx context.Context, id string) error
	UpdateStatus(ctx context.Context, id string, status string) error
	List(ctx context.Context, serverID string) ([]store.PreviewDeployment, error)
}

func registerPreviewDeploymentRoutes(protected fiber.Router, cfg Config, svc previewDeploymentService, adminIPAccess, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	pd := protected.Group("/admin/preview-deployments", adminIPAccess)

	pd.Get("/", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		previews, err := svc.ListAll(c.Context())
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": previews})
	})

	pd.Get("/:id", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		p, err := svc.Get(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": p})
	})

	pd.Post("/", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		var req struct {
			ServerID  string `json:"serverId"`
			PRNumber  int    `json:"prNumber"`
			PRTitle   string `json:"prTitle"`
			PRURL     string `json:"prUrl"`
			Branch    string `json:"branch"`
			RepoOwner string `json:"repoOwner"`
			RepoName  string `json:"repoName"`
			CommitSHA string `json:"commitSha"`
			Source    string `json:"source"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if req.ServerID == "" {
			return c.Status(400).JSON(fiber.Map{"error": "serverId is required"})
		}
		if req.Source == "" {
			req.Source = "github"
		}
		p, err := svc.Create(c.Context(), req.ServerID, &store.PreviewDeployment{
			PRNumber:  req.PRNumber,
			PRTitle:   req.PRTitle,
			PRURL:     req.PRURL,
			Branch:    req.Branch,
			RepoOwner: req.RepoOwner,
			RepoName:  req.RepoName,
			CommitSHA: req.CommitSHA,
			Source:    req.Source,
		})
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"data": p})
	})

	pd.Post("/:id/deploy", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		if err := svc.Deploy(c.Context(), c.Params("id")); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": fiber.Map{"id": c.Params("id")}})
	})

	pd.Post("/:id/cleanup", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		if err := svc.Cleanup(c.Context(), c.Params("id")); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": fiber.Map{"id": c.Params("id")}})
	})

	pd.Post("/:id/status", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		var req struct {
			Status string `json:"status"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if err := svc.UpdateStatus(c.Context(), c.Params("id"), req.Status); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": fiber.Map{"id": c.Params("id"), "status": req.Status}})
	})

	pd.Get("/server/:serverId", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		previews, err := svc.List(c.Context(), c.Params("serverId"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": previews})
	})
}

// ---------------------------------------------------------------------------
// Project-scoped preview environments (preview_environments)
// ---------------------------------------------------------------------------

// previewEnvironmentRegistrarName mounts the project-scoped preview surface.
// Priority 160 puts it next to the pipeline phase: it depends on nothing but
// the store and the compose lifecycle already built in main.
const previewEnvironmentRegistrarName = "preview-environments"

// Configuration for the preview wildcard and its lifetime. The API binary owns
// process configuration, so these are read here (the same way the pipeline
// registrar reads its webhook secret) instead of reaching into a package that
// has no business reading the environment.
const (
	previewEnvDomainEnv        = "PREVIEW_ENV_DOMAIN"
	previewEnvTTLEnv           = "PREVIEW_ENV_TTL"
	previewEnvMaxPerProjectEnv = "PREVIEW_ENV_MAX_PER_PROJECT"
	// legacyPreviewDomainEnv is the per-PR base domain; reused as the default so
	// an existing wildcard configuration keeps working.
	legacyPreviewDomainEnv = "PREVIEW_DOMAIN"
)

func init() {
	RegisterPhaseRegistrar(previewEnvironmentRegistrarName, 160, registerPreviewEnvironmentRoutes)
}

// registerPreviewEnvironmentRoutes wires /api/v1/projects/:id/previews and the
// public /api/v1/webhooks/preview endpoint. It is fully additive: without a
// Postgres pool the routes are skipped (and therefore 404) rather than served
// from a fake store.
func registerPreviewEnvironmentRoutes(v1 fiber.Router, protected fiber.Router, cfg *Config) error {
	if cfg.Store == nil || cfg.Store.DB() == nil {
		logPhase(cfg, previewEnvironmentRegistrarName+": no postgres pool, project preview routes skipped")
		return nil
	}

	svc := previewenv.New(cfg.Store, previewenv.Options{
		PreviewDomain:         previewEnvironmentDomain(),
		PreviewTTL:            previewEnvironmentTTL(),
		MaxPreviewsPerProject: previewEnvironmentMaxPerProject(),
		Compose:               cfg.ComposeService,
		Logger:                cfg.Logger,
		GitService:            cfg.GitService,
		AcmeService:           cfg.AcmeService,
		TrafficMgr:            cfg.TrafficManager,
		DomainSvc:             cfg.DomainService,
		PanelURL:              cfg.PanelURL,
		BackgroundContext:     cfg.BackgroundContext,
	})
	if svc == nil {
		logPhase(cfg, previewEnvironmentRegistrarName+": preview service unavailable, routes skipped")
		return nil
	}
	if cfg.ComposeService == nil {
		// Honest, not fatal: the routes still answer, and every create reports
		// that previews are not configured on this control plane rather than
		// persisting a row that can never run.
		logPhase(cfg, previewEnvironmentRegistrarName+": compose lifecycle not configured, previews will report as unavailable")
	}
	if cfg.BackgroundContext != nil {
		svc.StartPreviewReaper(cfg.BackgroundContext, 5*time.Minute)
	} else {
		svc.StartPreviewReaper(context.Background(), 5*time.Minute)
	}

	previewEnvironmentRoutes(v1, protected, cfg, svc)
	return nil
}

func previewEnvironmentRoutes(v1 fiber.Router, protected fiber.Router, cfg *Config, svc *previewenv.Service) {
	mutationLimiter := RateLimiter(GetRateLimitForEndpoint("mutation", cfg.Redis, cfg.RedisEnabled && strings.EqualFold(strings.TrimSpace(cfg.AppEnv), "production")))

	pv := protected.Group("/projects/:id/previews", projectEnvAccess(*cfg))

	// Static paths are registered before the ":previewId" routes: Fiber matches
	// in registration order, so /previews/config must not be eaten by a param
	// route declared earlier.
	pv.Get("/config", func(c *fiber.Ctx) error {
		domain, ttl, maxPerProject := svc.PreviewConfig()
		return c.JSON(fiber.Map{"data": fiber.Map{
			"baseDomain":    domain,
			"ttlSeconds":    int64(ttl / time.Second),
			"maxPerProject": maxPerProject,
			"webhookUrl":    svc.WebhookURL(c.Params("id")),
		}})
	})
	// The signing secret is revealed only to organization members: it is the
	// single credential that lets a caller create and destroy previews.
	pv.Get("/webhook-secret", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		secret, err := svc.PreviewWebhookSecret(ctx, c.Params("id"), false)
		if err != nil {
			return mapPreviewEnvironmentErr(c, err)
		}
		return c.JSON(fiber.Map{"data": fiber.Map{
			"secret":     secret,
			"webhookUrl": svc.WebhookURL(c.Params("id")),
		}})
	})
	pv.Post("/webhook-secret/rotate", mutationLimiter, func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		secret, err := svc.PreviewWebhookSecret(ctx, c.Params("id"), true)
		if err != nil {
			return mapPreviewEnvironmentErr(c, err)
		}
		return c.JSON(fiber.Map{"data": fiber.Map{
			"secret":     secret,
			"webhookUrl": svc.WebhookURL(c.Params("id")),
		}})
	})

	pv.Get("/", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		items, err := svc.ListPreviews(ctx, c.Params("id"))
		if err != nil {
			return mapPreviewEnvironmentErr(c, err)
		}
		return c.JSON(fiber.Map{"data": items})
	})

	pv.Post("/", mutationLimiter, func(c *fiber.Ctx) error {
		var req struct {
			Branch         string `json:"branch"`
			PRNumber       int    `json:"prNumber"`
			CommitSHA      string `json:"commitSha"`
			Title          string `json:"title"`
			PRURL          string `json:"prUrl"`
			ComposeContent string `json:"composeContent"`
			BaseStackID    string `json:"baseStackId"`
			ServerID       string `json:"serverId"`
			EnvironmentID  string `json:"environmentId"`
			TTLSeconds     int    `json:"ttlSeconds"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.Branch) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "branch is required")
		}
		if req.TTLSeconds < 0 {
			return fiber.NewError(fiber.StatusBadRequest, "ttlSeconds cannot be negative")
		}
		ctx, cancel := longRequestContext()
		defer cancel()
		preview, err := svc.CreatePreview(ctx, previewenv.CreatePreviewRequest{
			ProjectID:      c.Params("id"),
			Branch:         strings.TrimSpace(req.Branch),
			PRNumber:       req.PRNumber,
			CommitSHA:      strings.TrimSpace(req.CommitSHA),
			Title:          strings.TrimSpace(req.Title),
			PRURL:          strings.TrimSpace(req.PRURL),
			ComposeContent: req.ComposeContent,
			BaseStackID:    strings.TrimSpace(req.BaseStackID),
			ServerID:       nilIfPresent(req.ServerID),
			EnvironmentID:  nilIfPresent(req.EnvironmentID),
			Source:         "manual",
			CreatedBy:      claimSub(c),
			TTL:            time.Duration(req.TTLSeconds) * time.Second,
		})
		if err != nil {
			return mapPreviewEnvironmentErr(c, err)
		}
		// 201 with status "pending": the row exists, the deployment is running
		// in the background. Callers poll the row (or the event stream) for the
		// outcome.
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": preview})
	})

	pv.Get("/:previewId", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		preview, err := svc.GetPreviewInProject(ctx, c.Params("id"), c.Params("previewId"))
		if err != nil {
			return mapPreviewEnvironmentErr(c, err)
		}
		return c.JSON(fiber.Map{"data": preview})
	})

	pv.Post("/:previewId/redeploy", mutationLimiter, func(c *fiber.Ctx) error {
		ctx, cancel := longRequestContext()
		defer cancel()
		preview, err := svc.RedeployPreviewInProject(ctx, c.Params("id"), c.Params("previewId"))
		if err != nil {
			return mapPreviewEnvironmentErr(c, err)
		}
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"data": preview})
	})

	// Close keeps the row as an audit trail (status "teardown"); DELETE removes
	// it after the same teardown. Both destroy the running stack first, and
	// neither reports success unless that destroy succeeded.
	pv.Post("/:previewId/close", mutationLimiter, func(c *fiber.Ctx) error {
		var req struct {
			Reason string `json:"reason"`
		}
		if err := c.BodyParser(&req); err != nil && len(c.Body()) > 0 {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := longRequestContext()
		defer cancel()
		if err := svc.ClosePreviewInProject(ctx, c.Params("id"), c.Params("previewId"), strings.TrimSpace(req.Reason)); err != nil {
			return mapPreviewEnvironmentErr(c, err)
		}
		return c.JSON(fiber.Map{"data": fiber.Map{"id": c.Params("previewId"), "status": previewenv.PreviewStatusTeardown}})
	})

	pv.Delete("/:previewId", mutationLimiter, func(c *fiber.Ctx) error {
		ctx, cancel := longRequestContext()
		defer cancel()
		if err := svc.DeletePreviewInProject(ctx, c.Params("id"), c.Params("previewId")); err != nil {
			return mapPreviewEnvironmentErr(c, err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// Public provider callbacks. Unauthenticated by necessity, so every request
	// is verified against the addressed project's own signing secret; a project
	// that has not generated one cannot receive events at all.
	hook := previewEnvironmentWebhookHandler(svc)
	v1.Post("/webhooks/preview", hook)
	v1.Post("/webhooks/preview/:projectId", hook)
}

func previewEnvironmentWebhookHandler(svc *previewenv.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		body := c.Body()
		provider := previewWebhookProvider(c)
		if provider == "" {
			return fiber.NewError(fiber.StatusBadRequest,
				"cannot identify the provider from the request headers; send X-GitHub-Event/X-Hub-Signature-256 or X-Gitlab-Token")
		}
		projectID := strings.TrimSpace(c.Params("projectId", c.Query("projectId")))
		if projectID == "" {
			projectID = strings.TrimSpace(c.Get("X-Forge-Project"))
		}
		if projectID == "" {
			projectID = previewWebhookBodyProject(body)
		}
		if projectID == "" {
			return fiber.NewError(fiber.StatusBadRequest,
				"a preview webhook must address a project: /webhooks/preview/<projectId>, ?projectId=, X-Forge-Project or a projectId field")
		}

		verifyCtx, cancelVerify := requestContext()
		err := svc.VerifyPreviewWebhook(verifyCtx, projectID, provider, body,
			c.Get("X-Hub-Signature-256"), c.Get("X-Gitlab-Token"))
		cancelVerify()
		if err != nil {
			return mapPreviewWebhookAuthErr(c, err)
		}

		payload, err := previewenv.ParsePreviewWebhook(body, provider, c.Get("X-GitHub-Event"))
		if err != nil {
			if errors.Is(err, previewenv.ErrPreviewEventIgnored) {
				return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"status": "ignored", "reason": err.Error()})
			}
			// A malformed payload is the sender's problem and the message
			// describes their body, not our internals.
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		payload.ProjectID = projectID

		ctx, cancel := longRequestContext()
		defer cancel()
		preview, err := svc.TriggerFromWebhook(ctx, payload)
		if err != nil {
			if errors.Is(err, previewenv.ErrPreviewEventIgnored) {
				return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"status": "ignored", "reason": err.Error()})
			}
			return mapPreviewEnvironmentErr(c, err)
		}
		// 202: accepted and (for a create) queued. The preview's own status
		// reports whether the environment ever came up.
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"status": "accepted", "data": preview})
	}
}

// previewWebhookProvider derives the provider from the headers only. It is
// probed before the secret is even looked up, so an unknown provider is a
// 400 rather than an oracle about which projects have secrets configured.
func previewWebhookProvider(c *fiber.Ctx) string {
	switch {
	case c.Get("X-GitHub-Event") != "" || c.Get("X-Hub-Signature-256") != "":
		return previewenv.PreviewProviderGitHub
	case c.Get("X-Gitlab-Event") != "" || c.Get("X-Gitlab-Token") != "":
		return previewenv.PreviewProviderGitLab
	default:
		return ""
	}
}

// previewWebhookBodyProject reads an optional project id out of the callback
// body. Providers do not send one; it exists for payloads Forge itself relays
// (a fan-out webhook, a pipeline step). The value is only ever acted on after
// the signature check against that project's secret, so it cannot escalate.
func previewWebhookBodyProject(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var probe struct {
		ProjectID    string `json:"projectId"`
		ProjectSnake string `json:"project_id"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return ""
	}
	if strings.TrimSpace(probe.ProjectID) != "" {
		return strings.TrimSpace(probe.ProjectID)
	}
	return strings.TrimSpace(probe.ProjectSnake)
}

// nilIfPresent turns an empty form field into NULL so the service can tell "not
// specified" from "explicitly empty".
func nilIfPresent(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func mapPreviewWebhookAuthErr(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, git.ErrWebhookSignatureMissing), errors.Is(err, git.ErrWebhookSignatureInvalid):
		return fiber.NewError(fiber.StatusUnauthorized, "invalid or missing webhook signature")
	case errors.Is(err, previewenv.ErrPreviewWebhookNotConfigured):
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	default:
		return mapPreviewEnvironmentErr(c, err)
	}
}

// mapPreviewEnvironmentErr translates the previewenv sentinels into HTTP
// status codes. Anything unrecognised is logged and answered as a 500 without
// echoing internals, so a genuine bug never looks like a client error.
func mapPreviewEnvironmentErr(c *fiber.Ctx, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, previewenv.ErrPreviewNotFound):
		return fiber.NewError(fiber.StatusNotFound, "preview deployment not found")
	case errors.Is(err, previewenv.ErrProjectNotFound):
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	case errors.Is(err, previewenv.ErrServerNotFound):
		return fiber.NewError(fiber.StatusBadRequest, "the selected server does not exist")
	case errors.Is(err, previewenv.ErrPreviewAlreadyExists):
		return fiber.NewError(fiber.StatusConflict, err.Error())
	case errors.Is(err, previewenv.ErrPreviewTornDown):
		return fiber.NewError(fiber.StatusConflict, err.Error())
	case errors.Is(err, previewenv.ErrPreviewLimitReached):
		return fiber.NewError(fiber.StatusConflict, err.Error())
	case errors.Is(err, previewenv.ErrNoComposeSource),
		errors.Is(err, previewenv.ErrAmbiguousComposeSource),
		errors.Is(err, previewenv.ErrComposeSourceOutsideProject),
		errors.Is(err, previewenv.ErrPreviewProjectRequired):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	case errors.Is(err, previewenv.ErrPreviewRuntimeUnavailable):
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
	default:
		return respondInternalError(c, err)
	}
}

func previewEnvironmentDomain() string {
	if d := strings.TrimSpace(os.Getenv(previewEnvDomainEnv)); d != "" {
		return d
	}
	return strings.TrimSpace(os.Getenv(legacyPreviewDomainEnv))
}

// previewEnvironmentTTL accepts a Go duration ("168h", "720m") or a bare number
// of seconds. Zero means "not configured", which lets previewenv.New apply its
// own 7-day default instead of this file duplicating it.
func previewEnvironmentTTL() time.Duration {
	raw := strings.TrimSpace(os.Getenv(previewEnvTTLEnv))
	if raw == "" {
		return 0
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return 0
}

// previewEnvironmentMaxPerProject returns 0 for "no explicit setting"; the
// service normalizes that to its own default, and an explicit negative lifts
// the cap entirely.
func previewEnvironmentMaxPerProject() int {
	raw := strings.TrimSpace(os.Getenv(previewEnvMaxPerProjectEnv))
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	if n == 0 {
		// 0 means unlimited to the service, so an operator who writes "0" in
		// the environment still gets the default rather than a surprise.
		return -1
	}
	return n
}
