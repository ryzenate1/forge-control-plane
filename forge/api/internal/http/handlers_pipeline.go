package http

import (
	"strconv"

	"gamepanel/forge/internal/services/pipeline"

	"github.com/gofiber/fiber/v2"
)

// registerPipelineRoutes exposes the pipeline engine (definitions + runs + logs
// + artifacts + manual stage approvals). The service, store and run queue were
// fully implemented but never routed, so the /admin/pipelines dashboard had no
// backend. All routes are admin-only.
func registerPipelineRoutes(protected fiber.Router, svc *pipeline.Service, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}
	actor := func(c *fiber.Ctx) string {
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			return claims.Sub
		}
		return ""
	}

	api := protected.Group("/pipelines", requireRole("admin"))

	api.Get("/", func(c *fiber.Ctx) error {
		defs, err := svc.ListDefinitions(c.Context())
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": defs})
	})

	api.Post("/", mutationLimiter, func(c *fiber.Ctx) error {
		var def pipeline.Definition
		if err := c.BodyParser(&def); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		created, err := svc.CreateDefinition(c.Context(), &def)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(created)
	})

	api.Get("/:id", func(c *fiber.Ctx) error {
		def, err := svc.GetDefinition(c.Context(), c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.JSON(def)
	})

	api.Put("/:id", mutationLimiter, func(c *fiber.Ctx) error {
		var def pipeline.Definition
		if err := c.BodyParser(&def); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		updated, err := svc.UpdateDefinition(c.Context(), c.Params("id"), &def)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(updated)
	})

	api.Delete("/:id", mutationLimiter, func(c *fiber.Ctx) error {
		if err := svc.DeleteDefinition(c.Context(), c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	api.Post("/:id/run", mutationLimiter, func(c *fiber.Ctx) error {
		run, err := svc.TriggerRun(c.Context(), c.Params("id"), "manual", actor(c))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.Status(fiber.StatusAccepted).JSON(run)
	})

	api.Get("/:id/runs", func(c *fiber.Ctx) error {
		limit := 50
		if raw := c.Query("limit"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				limit = n
			}
		}
		runs, err := svc.ListRuns(c.Context(), c.Params("id"), c.Query("status"), limit, 0)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": runs})
	})

	runs := protected.Group("/pipeline-runs", requireRole("admin"))

	runs.Get("/:runId", func(c *fiber.Ctx) error {
		run, err := svc.GetRun(c.Context(), c.Params("runId"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.JSON(run)
	})

	runs.Post("/:runId/retry", mutationLimiter, func(c *fiber.Ctx) error {
		run, err := svc.RetryRun(c.Context(), c.Params("runId"), actor(c))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(run)
	})

	runs.Post("/:runId/cancel", mutationLimiter, func(c *fiber.Ctx) error {
		if err := svc.CancelRun(c.Context(), c.Params("runId")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	runs.Get("/:runId/logs", func(c *fiber.Ctx) error {
		var after int64
		if a := c.Query("after"); a != "" {
			after, _ = strconv.ParseInt(a, 10, 64)
		}
		logs, err := svc.ListLogs(c.Context(), c.Params("runId"), after)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": logs})
	})

	runs.Get("/:runId/artifacts", func(c *fiber.Ctx) error {
		arts, err := svc.ListArtifacts(c.Context(), c.Params("runId"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": arts})
	})

	runs.Post("/:runId/stages/:stageId/approve", mutationLimiter, func(c *fiber.Ctx) error {
		if err := svc.ApproveStage(c.Context(), c.Params("runId"), c.Params("stageId"), actor(c)); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	runs.Post("/:runId/stages/:stageId/reject", mutationLimiter, func(c *fiber.Ctx) error {
		if err := svc.RejectStage(c.Context(), c.Params("runId"), c.Params("stageId"), actor(c)); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})
}
