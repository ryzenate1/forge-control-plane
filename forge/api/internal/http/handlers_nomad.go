package http

import (
	"bytes"
	"errors"

	"github.com/gofiber/fiber/v2"

	nomadsvc "gamepanel/forge/internal/services/nomad"
)

// registerNomadRoutes wires the Forge Orchestration (Nomad) admin surface.
//
// It is gated by the scheduler.* scope family rather than a nomad.* family:
// there is no nomad.read/nomad.write in store.AdminScopes, and requireAdminScope
// only accepts a scope that an API key can actually be issued. ValidateApiKeyScopes
// rejects unregistered names and the scope catalogue served at
// GET /api-keys is store.AdminScopes itself, so gating on nomad.* made every
// route here permanently 403 for every API key — reachable only by
// session-cookie admins, who bypass scopes entirely via "*". scheduler.* means
// "view and manage workload placement/scheduling", which is exactly what these
// routes do, so reusing it keeps the gate real instead of minting a dead scope.
func registerNomadRoutes(protected fiber.Router, cfg Config, adminIPAccess fiber.Handler) {
	nm := protected.Group("/admin/nomad", adminIPAccess)
	nm.Get("/jobs", requireRole("admin"), requireAdminScope("scheduler.read"), func(c *fiber.Ctx) error { return nomadListJobs(c, cfg) })
	nm.Post("/jobs", requireRole("admin"), requireAdminScope("scheduler.write"), func(c *fiber.Ctx) error { return nomadSubmitJob(c, cfg) })
	nm.Get("/jobs/:id", requireRole("admin"), requireAdminScope("scheduler.read"), func(c *fiber.Ctx) error { return nomadGetJob(c, cfg) })
	nm.Post("/jobs/:id/stop", requireRole("admin"), requireAdminScope("scheduler.write"), func(c *fiber.Ctx) error { return nomadStopJob(c, cfg) })
	nm.Get("/allocations", requireRole("admin"), requireAdminScope("scheduler.read"), func(c *fiber.Ctx) error { return nomadListAllocations(c, cfg) })
	nm.Post("/allocations/:id/promote", requireRole("admin"), requireAdminScope("scheduler.write"), func(c *fiber.Ctx) error { return nomadPromoteAllocation(c, cfg) })
	nm.Get("/nodes", requireRole("admin"), requireAdminScope("scheduler.read"), func(c *fiber.Ctx) error { return nomadListNodes(c, cfg) })
	nm.Get("/nodes/:id", requireRole("admin"), requireAdminScope("scheduler.read"), func(c *fiber.Ctx) error { return nomadGetNode(c, cfg) })
	nm.Post("/nodes/:id/drain", requireRole("admin"), requireAdminScope("scheduler.write"), func(c *fiber.Ctx) error { return nomadDrainNode(c, cfg) })
	nm.Get("/deployments", requireRole("admin"), requireAdminScope("scheduler.read"), func(c *fiber.Ctx) error { return nomadListDeployments(c, cfg) })
	nm.Get("/deployments/:id", requireRole("admin"), requireAdminScope("scheduler.read"), func(c *fiber.Ctx) error { return nomadGetDeployment(c, cfg) })
}

// nomadError answers 503 when Forge Orchestration is not configured and 502 for
// upstream Nomad control-plane faults.
func nomadError(err error) error {
	if errors.Is(err, nomadsvc.ErrNotConfigured) {
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
	}
	return fiber.NewError(fiber.StatusBadGateway, err.Error())
}

func nomadListJobs(c *fiber.Ctx, cfg Config) error {
	svc := cfg.NomadService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Orchestration service unavailable")
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	jobs, err := svc.ListJobs(ctx, c.Query("namespace"))
	if err != nil {
		return nomadError(err)
	}
	return c.JSON(fiber.Map{"jobs": jobs})
}

func nomadSubmitJob(c *fiber.Ctx, cfg Config) error {
	svc := cfg.NomadService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Orchestration service unavailable")
	}
	var spec map[string]any
	if err := c.BodyParser(&spec); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if len(spec) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "job specification required")
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	job, err := svc.SubmitJob(ctx, spec)
	if err != nil {
		return nomadError(err)
	}
	return c.JSON(fiber.Map{"ok": true, "job": job})
}

func nomadGetJob(c *fiber.Ctx, cfg Config) error {
	svc := cfg.NomadService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Orchestration service unavailable")
	}
	id := c.Params("id")
	if id == "" {
		return fiber.NewError(fiber.StatusBadRequest, "job id required")
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	job, err := svc.GetJobStatus(ctx, id)
	if err != nil {
		return nomadError(err)
	}
	return c.JSON(job)
}

func nomadStopJob(c *fiber.Ctx, cfg Config) error {
	svc := cfg.NomadService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Orchestration service unavailable")
	}
	id := c.Params("id")
	if id == "" {
		return fiber.NewError(fiber.StatusBadRequest, "job id required")
	}
	purge := isTrueParam(c, "purge")
	ctx, cancel := longRequestContext()
	defer cancel()
	evalID, err := svc.StopJob(ctx, id, purge)
	if err != nil {
		return nomadError(err)
	}
	return c.JSON(fiber.Map{"ok": true, "job": id, "evalId": evalID})
}

func nomadListAllocations(c *fiber.Ctx, cfg Config) error {
	svc := cfg.NomadService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Orchestration service unavailable")
	}
	jobID := c.Query("jobId")
	if jobID == "" {
		jobID = c.Query("job_id")
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	allocs, err := svc.ListAllocations(ctx, jobID)
	if err != nil {
		return nomadError(err)
	}
	return c.JSON(fiber.Map{"allocations": allocs})
}

func nomadPromoteAllocation(c *fiber.Ctx, cfg Config) error {
	svc := cfg.NomadService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Orchestration service unavailable")
	}
	id := c.Params("id")
	if id == "" {
		return fiber.NewError(fiber.StatusBadRequest, "allocation id required")
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	deployID, err := svc.PromoteAllocation(ctx, id)
	if err != nil {
		return nomadError(err)
	}
	return c.JSON(fiber.Map{"ok": true, "allocation": id, "deployment": deployID})
}

func nomadListNodes(c *fiber.Ctx, cfg Config) error {
	svc := cfg.NomadService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Orchestration service unavailable")
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	nodes, err := svc.ListNodes(ctx)
	if err != nil {
		return nomadError(err)
	}
	return c.JSON(fiber.Map{"nodes": nodes})
}

func nomadGetNode(c *fiber.Ctx, cfg Config) error {
	svc := cfg.NomadService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Orchestration service unavailable")
	}
	id := c.Params("id")
	if id == "" {
		return fiber.NewError(fiber.StatusBadRequest, "node id required")
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	node, err := svc.GetNode(ctx, id)
	if err != nil {
		return nomadError(err)
	}
	return c.JSON(node)
}

func nomadDrainNode(c *fiber.Ctx, cfg Config) error {
	svc := cfg.NomadService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Orchestration service unavailable")
	}
	id := c.Params("id")
	if id == "" {
		return fiber.NewError(fiber.StatusBadRequest, "node id required")
	}
	var body struct {
		Drain *bool `json:"drain"`
	}
	// A truly absent body defaults to starting a drain. A body that is present
	// but unparseable must not: silently treating corrupt JSON as "drain the
	// node" turns a typo into a fleet evacuation, so it is rejected instead of
	// resolved to the destructive default.
	drain := true
	if raw := bytes.TrimSpace(c.Body()); len(raw) > 0 && string(raw) != "null" {
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if body.Drain == nil {
			return fiber.NewError(fiber.StatusBadRequest, "drain must be a boolean")
		}
		drain = *body.Drain
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	if err := svc.DrainNode(ctx, id, drain); err != nil {
		return nomadError(err)
	}
	return c.JSON(fiber.Map{"ok": true, "node": id, "drain": drain})
}

func nomadListDeployments(c *fiber.Ctx, cfg Config) error {
	svc := cfg.NomadService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Orchestration service unavailable")
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	deployments, err := svc.ListDeployments(ctx)
	if err != nil {
		return nomadError(err)
	}
	return c.JSON(fiber.Map{"deployments": deployments})
}

func nomadGetDeployment(c *fiber.Ctx, cfg Config) error {
	svc := cfg.NomadService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Orchestration service unavailable")
	}
	id := c.Params("id")
	if id == "" {
		return fiber.NewError(fiber.StatusBadRequest, "deployment id required")
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	deployment, err := svc.GetDeployment(ctx, id)
	if err != nil {
		return nomadError(err)
	}
	return c.JSON(deployment)
}
