package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	"gamepanel/forge/internal/services/resourcelimits"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
	"gopkg.in/yaml.v3"
)

// Resource limits, deploy-gating health checks and process scaling.
//
// Everything lives in this one file and one service package: the routes for all
// three sub-features share a body of access-control and error-translation
// helpers, and splitting them would duplicate the org-membership check three
// times for no gain.
//
// Registration happens through the phase-hook registry rather than by editing
// NewServer, so the feature is entirely additive.

// resourceLimitsPriority sits between phase8-env-as-code (800) and
// phase2-environment-engine (2000). The absolute number is arbitrary; the order
// is what Fiber uses when two routes could match the same path, and nothing
// here overlaps another phase's paths.
const resourceLimitsPriority = 900

func init() {
	RegisterPhaseRegistrar("resource-limits-health-checks", resourceLimitsPriority, registerResourceLimitsRoutes)
}

// registerResourceLimitsRoutes mounts the /apps/:appId/processes and
// /servers/:id/health surfaces.
//
// It degrades to "not mounted" (logged, non-fatal) without a Postgres pool:
// process configuration has no in-memory fallback, and a route that would 500
// on every request is worse than a 404 that tells the operator the feature is
// unavailable.
func registerResourceLimitsRoutes(v1 fiber.Router, protected fiber.Router, cfg *Config) error {
	_ = v1
	if cfg == nil || cfg.Store == nil || protected == nil {
		return nil
	}
	pool := cfg.Store.GetDB()
	if pool == nil {
		if cfg.Logger != nil {
			cfg.Logger.Warn("resource limits routes not registered: no postgres pool")
		}
		return nil
	}
	// requireServerPermission and the handler factories take Config by value,
	// which is how every route in this package receives it; the phase-hook
	// contract hands us a pointer, so it is dereferenced once here rather than
	// at eleven call sites.
	conf := *cfg
	svc := resourcelimits.NewFromPool(pool)

	// ---- Application process configuration -------------------------------
	//
	// Registered as full paths rather than a /apps/:appId/processes group: a
	// Group + Get("/") pair registers a trailing-slash route, which turns the
	// list endpoint into a 301 the browser has to follow.
	protected.Get("/apps/:appId/processes", resourceProcessList(conf, svc))
	protected.Get("/apps/:appId/processes/health-status", resourceHealthStatus(conf, svc))
	protected.Put("/apps/:appId/processes/:processType", resourceProcessUpsert(conf, svc))
	protected.Delete("/apps/:appId/processes/:processType", resourceProcessDelete(conf, svc))
	protected.Post("/apps/:appId/processes/:processType/scale", resourceProcessScale(conf, svc))
	protected.Post("/apps/:appId/processes/compose/preview", resourceComposeRender(conf, svc, true))
	protected.Post("/apps/:appId/processes/compose/apply", resourceComposeRender(conf, svc, false))

	// ---- Server-facing surfaces ------------------------------------------

	// The bound application is resolved server-side because GET /apps is an
	// admin-only listing: a per-server tab owned by a non-admin cannot find the
	// application id on its own.
	protected.Get("/servers/:id/resource-limits", requireServerPermission(conf, ""), serverResourceLimitsBundle(conf, svc))
	protected.Get("/servers/:id/health", requireServerPermission(conf, ""), serverHealthHistory(conf, svc))
	// health-report is the ingest seam ReportHealth was written for: without a
	// way to record a probe result, ShouldRollback could only ever say "nothing
	// was reported". Reads use the same gate as the existing
	// /servers/:id/health-check routes; writes require workload control.
	protected.Post("/servers/:id/health-report", requireServerPermission(conf, store.PermControlStart), serverHealthReport(conf, svc))

	return nil
}

// ---------------------------------------------------------------------------
// Access control
// ---------------------------------------------------------------------------

// authorizedApplicationForProcesses resolves :appId to an application the
// caller may edit, applying exactly the rule the app-hosting routes apply:
// global admins, or members of the owning organization. A foreign application
// answers 404 rather than 403 so organization membership is not enumerable
// through this API.
func authorizedApplicationForProcesses(c *fiber.Ctx, cfg Config) (*store.Application, error) {
	claims, ok := c.Locals("user").(tokenClaims)
	if !ok {
		return nil, fiber.NewError(fiber.StatusUnauthorized, "missing session")
	}
	appID := strings.TrimSpace(c.Params("appId"))
	if appID == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "application id is required")
	}
	ctx, cancel := requestContext()
	defer cancel()

	app, err := cfg.Store.GetApplication(ctx, appID)
	if err != nil {
		if cfg.Logger != nil {
			cfg.Logger.Error("load application for process config",
				slog.String("appId", appID), slog.String("error", err.Error()))
		}
		return nil, fiber.NewError(fiber.StatusNotFound, "application not found")
	}
	if claims.Role != "admin" {
		isMember, memberErr := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
		if memberErr != nil {
			return nil, respondInternalError(c, memberErr)
		}
		if !isMember {
			return nil, fiber.NewError(fiber.StatusNotFound, "application not found")
		}
	}
	return app, nil
}

// processError translates the service's sentinel and validation errors into HTTP
// statuses. errors.Is/errors.As are used rather than message matching, so a
// reworded error cannot silently move an endpoint from 400 to 500.
func processError(c *fiber.Ctx, err error) error {
	if err == nil {
		return nil
	}
	var already *fiber.Error
	if errors.As(err, &already) {
		return already
	}
	switch {
	case errors.Is(err, resourcelimits.ErrProcessConfigNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, resourcelimits.ErrApplicationNotFound):
		return fiber.NewError(fiber.StatusNotFound, "application not found")
	case errors.Is(err, resourcelimits.ErrNoBindableApplication):
		return fiber.NewError(fiber.StatusNotFound, "no application is bound to this server")
	case errors.Is(err, resourcelimits.ErrInvalidComposeDocument):
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	}
	var validation *resourcelimits.ValidationError
	if errors.As(err, &validation) {
		return fiber.NewError(fiber.StatusBadRequest, validation.Message)
	}
	return respondInternalError(c, err)
}

func processTypeParam(c *fiber.Ctx) (string, error) {
	raw := strings.TrimSpace(c.Params("processType"))
	if raw == "" {
		return "", fiber.NewError(fiber.StatusBadRequest, "process type is required")
	}
	normalized, err := resourcelimits.NormalizeProcessType(raw)
	if err != nil {
		return "", fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	// A path parameter must agree with itself after normalisation: silently
	// accepting /Processes/Web and writing "web" would let a client believe it
	// created a second process type.
	if normalized != raw {
		return "", fiber.NewError(fiber.StatusBadRequest, "process type must be lowercase: use "+normalized)
	}
	return normalized, nil
}

// ---------------------------------------------------------------------------
// /apps/:appId/processes
// ---------------------------------------------------------------------------

func resourceProcessList(cfg Config, svc *resourcelimits.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		app, err := authorizedApplicationForProcesses(c, cfg)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		configs, err := svc.ListProcessConfigs(ctx, app.ID)
		if err != nil {
			return processError(c, err)
		}
		return c.JSON(fiber.Map{
			"application": app,
			"processes":   configs,
			// The document the deploy path will ship, so the UI can say which
			// process types actually have a service to attach to.
			"compose": composeDocumentPresence(app.SourceConfig),
		})
	}
}

func resourceProcessUpsert(cfg Config, svc *resourcelimits.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		app, err := authorizedApplicationForProcesses(c, cfg)
		if err != nil {
			return err
		}
		processType, err := processTypeParam(c)
		if err != nil {
			return err
		}
		var input resourcelimits.ProcessConfigInput
		body := c.Body()
		if len(strings.TrimSpace(string(body))) == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "a process configuration body is required")
		}
		if err := c.BodyParser(&input); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body: "+err.Error())
		}
		ctx, cancel := requestContext()
		defer cancel()
		saved, err := svc.UpsertProcessConfig(ctx, app.ID, processType, input)
		if err != nil {
			return processError(c, err)
		}
		return c.JSON(fiber.Map{"process": saved})
	}
}

func resourceProcessDelete(cfg Config, svc *resourcelimits.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		app, err := authorizedApplicationForProcesses(c, cfg)
		if err != nil {
			return err
		}
		processType, err := processTypeParam(c)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		deleted, err := svc.DeleteProcessConfig(ctx, app.ID, processType)
		if err != nil {
			return processError(c, err)
		}
		if !deleted {
			// Reporting 204 here would claim a row was removed when nothing
			// matched the (application, process type) pair.
			return fiber.NewError(fiber.StatusNotFound, "no process configuration named "+processType)
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}

func resourceProcessScale(cfg Config, svc *resourcelimits.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		app, err := authorizedApplicationForProcesses(c, cfg)
		if err != nil {
			return err
		}
		processType, err := processTypeParam(c)
		if err != nil {
			return err
		}
		var req struct {
			Replicas *int `json:"replicas"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body: "+err.Error())
		}
		if req.Replicas == nil {
			return fiber.NewError(fiber.StatusBadRequest, "replicas is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		previous, err := svc.GetProcessConfig(ctx, app.ID, processType)
		if err != nil {
			return processError(c, err)
		}
		saved, err := svc.ScaleProcess(ctx, app.ID, processType, *req.Replicas)
		if err != nil {
			return processError(c, err)
		}
		return c.JSON(fiber.Map{
			"process":          saved,
			"previousReplicas": previous.Replicas,
			"changed":          previous.Replicas != saved.Replicas,
		})
	}
}

func resourceHealthStatus(cfg Config, svc *resourcelimits.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		app, err := authorizedApplicationForProcesses(c, cfg)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		statuses, err := svc.HealthStatus(ctx, app.ID)
		if err != nil {
			return processError(c, err)
		}
		decision, err := svc.RollbackDecision(ctx, app.ID)
		if err != nil {
			return processError(c, err)
		}
		return c.JSON(fiber.Map{
			"applicationId": app.ID,
			"statuses":      statuses,
			"rollback":      decision,
			// The deploy path asks this same question; surfacing the answer here
			// means the UI and the rollback never disagree about why.
			"shouldRollback": decision.Rollback,
		})
	}
}

// ---------------------------------------------------------------------------
// Compose rendering
// ---------------------------------------------------------------------------

// composeSourceKeys mirrors the two keys the app-hosting deploy path reads its
// document from, in the order composeDocument() prefers them. It is duplicated
// here deliberately: the alternative is mutating another service's private
// struct, which would couple this feature to app-hosting internals it has no
// business knowing.
const (
	composeKeyContent        = "content"
	composeKeyComposeContent = "composeContent"
)

// composeSourceConfig is the subset of applications.source_config this feature
// touches. Unknown keys are preserved by round-tripping through a map, so
// applying a limit cannot quietly drop autoDeploy or envVars.
type composeSourceConfig struct {
	Content        string `json:"content"`
	ComposeContent string `json:"composeContent"`
	StackID        string `json:"stackId"`
}

// readComposeSourceConfig extracts the document the deploy path would ship,
// which stored key it came from, and a copy of the source config to mutate.
func readComposeSourceConfig(raw json.RawMessage) (composeSourceConfig, map[string]any, string, string, error) {
	var spec composeSourceConfig
	if len(raw) == 0 {
		return spec, nil, "", "", errors.New("application has no compose document to render into")
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return spec, nil, "", "", errors.New("application source config is unreadable: " + err.Error())
	}
	restored := map[string]any{}
	if err := json.Unmarshal(raw, &restored); err != nil {
		return spec, nil, "", "", errors.New("application source config is not a JSON object: " + err.Error())
	}
	key, document := composeKeyContent, strings.TrimSpace(spec.Content)
	if document == "" {
		key, document = composeKeyComposeContent, strings.TrimSpace(spec.ComposeContent)
	}
	if document == "" {
		return spec, restored, "", "", errors.New("application has no compose document to render into")
	}
	return spec, restored, key, document, nil
}

// parseComposeDocument accepts YAML, and JSON too, since YAML is a superset and
// some applications store their document as the latter.
func parseComposeDocument(document string) (map[string]any, error) {
	doc := map[string]any{}
	if err := yaml.Unmarshal([]byte(document), &doc); err != nil {
		return nil, errors.New("the stored compose document does not parse: " + err.Error())
	}
	if len(doc) == 0 {
		return nil, errors.New("the stored compose document is empty")
	}
	return doc, nil
}

// composeDocumentPresence answers "is there a document, and what is it called"
// for the list endpoint, without shipping the whole YAML to a settings tab.
func composeDocumentPresence(raw json.RawMessage) fiber.Map {
	_, _, key, document, err := readComposeSourceConfig(raw)
	if err != nil {
		return fiber.Map{"present": false, "reason": err.Error()}
	}
	if _, parseErr := parseComposeDocument(document); parseErr != nil {
		return fiber.Map{"present": true, "sourceKey": key, "parsable": false, "reason": parseErr.Error()}
	}
	return fiber.Map{"present": true, "sourceKey": key, "parsable": true}
}

// resourceComposeRender builds both compose endpoints. When dryRun is true the
// document is rendered and returned but nothing is written; otherwise the
// rendered document replaces the stored one so the existing deploy path ships
// it, and — when the caller asked and the deploy engine is wired — a real
// deployment is triggered.
func resourceComposeRender(cfg Config, svc *resourcelimits.Service, dryRun bool) fiber.Handler {
	return func(c *fiber.Ctx) error {
		app, err := authorizedApplicationForProcesses(c, cfg)
		if err != nil {
			return err
		}
		var req struct {
			Deploy       *bool `json:"deploy"`
			AllowPartial *bool `json:"allowPartial"`
		}
		if len(strings.TrimSpace(string(c.Body()))) > 0 {
			if err := c.BodyParser(&req); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, "invalid request body: "+err.Error())
			}
		}

		spec, restored, sourceKey, document, err := readComposeSourceConfig(app.SourceConfig)
		if err != nil {
			return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
		}
		doc, err := parseComposeDocument(document)
		if err != nil {
			return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
		}

		ctx, cancel := requestContext()
		defer cancel()
		render, err := svc.RenderIntoCompose(ctx, app.ID, doc)
		if err != nil {
			return processError(c, err)
		}
		rendered, err := yaml.Marshal(doc)
		if err != nil {
			return respondInternalError(c, errors.New("marshal rendered compose document: "+err.Error()))
		}

		payload := fiber.Map{
			"applicationId": app.ID,
			"sourceKey":     sourceKey,
			"composeYaml":   string(rendered),
			"applied":       render.Applied,
			"skipped":       render.Skipped,
			"fullyApplied":  render.FullyApplied(),
			"persisted":     false,
			"deployed":      false,
		}
		if dryRun {
			return c.JSON(payload)
		}

		if !render.FullyApplied() && (req.AllowPartial == nil || !*req.AllowPartial) {
			// Storing a half-rendered document would make the next deploy ship
			// limits the operator believes are enforced. Say so, and let them
			// decide with allowPartial.
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
				"error":        "not every process configuration could be applied to the compose document; nothing was written",
				"applied":      render.Applied,
				"skipped":      render.Skipped,
				"fullyApplied": false,
				"persisted":    false,
				"deployed":     false,
			})
		}

		restored[sourceKey] = string(rendered)
		encoded, err := json.Marshal(restored)
		if err != nil {
			return respondInternalError(c, errors.New("encode source config: "+err.Error()))
		}
		if err := cfg.Store.UpdateApplication(ctx, app.ID, store.UpdateApplicationInput{SourceConfig: encoded}); err != nil {
			return respondInternalError(c, err)
		}
		payload["persisted"] = true
		payload["stackId"] = strings.TrimSpace(spec.StackID)

		if req.Deploy == nil || !*req.Deploy {
			return c.JSON(payload)
		}
		if cfg.AppHostingService == nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"error":        "the application-hosting deploy engine is not configured; the rendered document was stored but nothing was deployed",
				"applied":      render.Applied,
				"skipped":      render.Skipped,
				"fullyApplied": render.FullyApplied(),
				"persisted":    true,
				"deployed":     false,
			})
		}
		deployment, deployErr := cfg.AppHostingService.TriggerDeploy(ctx, app.ID, app.OrgID)
		if deployErr != nil {
			// The document was stored; a failed deploy is a real outcome and is
			// reported as one instead of being hidden behind a bare 500.
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
				"error":        deployErr.Error(),
				"applied":      render.Applied,
				"skipped":      render.Skipped,
				"fullyApplied": render.FullyApplied(),
				"persisted":    true,
				"deployed":     false,
			})
		}
		payload["deployed"] = true
		payload["deployment"] = deployment
		return c.JSON(payload)
	}
}

// ---------------------------------------------------------------------------
// /servers/:id/...
// ---------------------------------------------------------------------------

// serverHealthHistory is the raw probe log for one host. The application-level
// roll-up lives on /apps/:appId/processes/health-status; this endpoint exists
// because an operator debugging a node wants the un-aggregated timeline.
func serverHealthHistory(cfg Config, svc *resourcelimits.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		serverID := strings.TrimSpace(c.Params("id"))
		if serverID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "server id is required")
		}
		limit := 100
		if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed < 1 {
				return fiber.NewError(fiber.StatusBadRequest, "limit must be a positive integer")
			}
			limit = parsed
		}
		ctx, cancel := requestContext()
		defer cancel()
		observations, err := svc.ListServerHealth(ctx, serverID, limit)
		if err != nil {
			return processError(c, err)
		}
		return c.JSON(fiber.Map{"serverId": serverID, "observations": observations})
	}
}

func serverResourceLimitsBundle(cfg Config, svc *resourcelimits.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		serverID := strings.TrimSpace(c.Params("id"))
		if serverID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "server id is required")
		}
		ctx, cancel := requestContext()
		defer cancel()

		bundle := fiber.Map{
			"serverId":     serverID,
			"application":  nil,
			"processes":    []resourcelimits.ProcessConfig{},
			"health":       []resourcelimits.ProcessHealthState{},
			"observations": []resourcelimits.HealthObservation{},
			"rollback":     nil,
		}
		app, err := svc.GetApplicationByServer(ctx, serverID)
		if err != nil {
			if errors.Is(err, resourcelimits.ErrNoBindableApplication) {
				// A server with no application is a valid, explainable state for
				// a settings tab — not an error the UI should render as one.
				bundle["reason"] = err.Error()
				return c.JSON(bundle)
			}
			return processError(c, err)
		}
		configs, err := svc.ListProcessConfigs(ctx, app.ID)
		if err != nil {
			return processError(c, err)
		}
		statuses, err := svc.HealthStatus(ctx, app.ID)
		if err != nil {
			return processError(c, err)
		}
		decision, err := svc.RollbackDecision(ctx, app.ID)
		if err != nil {
			return processError(c, err)
		}
		observations, err := svc.ListServerHealth(ctx, serverID, 50)
		if err != nil {
			return processError(c, err)
		}
		bundle["application"] = app
		bundle["processes"] = configs
		bundle["health"] = statuses
		bundle["observations"] = observations
		bundle["rollback"] = decision
		return c.JSON(bundle)
	}
}

func serverHealthReport(cfg Config, svc *resourcelimits.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		serverID := strings.TrimSpace(c.Params("id"))
		if serverID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "server id is required")
		}
		var req struct {
			ProcessType *string `json:"processType"`
			Healthy     *bool   `json:"healthy"`
			Detail      string  `json:"detail"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body: "+err.Error())
		}
		// healthy is a pointer precisely because a missing "false" in JSON is
		// indistinguishable from an omitted field once parsed — and recording a
		// probe that never happened is the one thing this endpoint must not do.
		if req.ProcessType == nil || req.Healthy == nil {
			return fiber.NewError(fiber.StatusBadRequest, "processType and healthy are both required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		observation, err := svc.RecordHealth(ctx, resourcelimits.HealthObservation{
			ServerID:    serverID,
			ProcessType: *req.ProcessType,
			Healthy:     *req.Healthy,
			Detail:      req.Detail,
		})
		if err != nil {
			return processError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"observation": observation})
	}
}
