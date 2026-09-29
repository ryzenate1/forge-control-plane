package http

import (
	"errors"

	"github.com/gofiber/fiber/v2"

	nomadsvc "gamepanel/forge/internal/services/nomad"
)

func registerNomadRoutes(protected fiber.Router, cfg Config, adminIPAccess fiber.Handler) {
	nm := protected.Group("/admin/nomad", adminIPAccess)
	nm.Get("/jobs", requireRole("admin"), requireAdminScope("nomad.read"), func(c *fiber.Ctx) error { return nomadListJobs(c, cfg) })
	nm.Post("/jobs", requireRole("admin"), requireAdminScope("nomad.write"), func(c *fiber.Ctx) error { return nomadSubmitJob(c, cfg) })
	nm.Get("/jobs/:id", requireRole("admin"), requireAdminScope("nomad.read"), func(c *fiber.Ctx) error { return nomadGetJob(c, cfg) })
	nm.Post("/jobs/:id/stop", requireRole("admin"), requireAdminScope("nomad.write"), func(c *fiber.Ctx) error { return nomadStopJob(c, cfg) })
	nm.Get("/allocations", requireRole("admin"), requireAdminScope("nomad.read"), func(c *fiber.Ctx) error { return nomadListAllocations(c, cfg) })
	nm.Post("/allocations/:id/promote", requireRole("admin"), requireAdminScope("nomad.write"), func(c *fiber.Ctx) error { return nomadPromoteAllocation(c, cfg) })
	nm.Get("/nodes", requireRole("admin"), requireAdminScope("nomad.read"), func(c *fiber.Ctx) error { return nomadListNodes(c, cfg) })
	nm.Get("/nodes/:id", requireRole("admin"), requireAdminScope("nomad.read"), func(c *fiber.Ctx) error { return nomadGetNode(c, cfg) })
	nm.Post("/nodes/:id/drain", requireRole("admin"), requireAdminScope("nomad.write"), func(c *fiber.Ctx) error { return nomadDrainNode(c, cfg) })
	nm.Get("/deployments", requireRole("admin"), requireAdminScope("nomad.read"), func(c *fiber.Ctx) error { return nomadListDeployments(c, cfg) })
	nm.Get("/deployments/:id", requireRole("admin"), requireAdminScope("nomad.read"), func(c *fiber.Ctx) error { return nomadGetDeployment(c, cfg) })
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
	// Absent body defaults to starting a drain; pass {"drain": false} to reverse.
	drain := true
	if err := c.BodyParser(&body); err == nil && body.Drain != nil {
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
