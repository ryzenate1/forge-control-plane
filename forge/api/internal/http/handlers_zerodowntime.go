package http

import (
	"log/slog"

	"gamepanel/forge/internal/services/zerodowntime"
	"github.com/gofiber/fiber/v2"
)

func registerZeroDowntimeRoutes(protected fiber.Router, cfg Config, svc *zerodowntime.Service, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	protected.Post("/servers/:id/deployments", mutationLimiter, requireServerPermission(cfg, ""), func(c *fiber.Ctx) error {
		var req struct {
			ImageTag string `json:"imageTag"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		if req.ImageTag == "" {
			return c.Status(400).JSON(fiber.Map{"error": "imageTag is required"})
		}

		release, err := svc.CreateRelease(c.Context(), c.Params("id"), req.ImageTag)
		if err != nil {
			return respondInternalError(c, err)
		}

		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("zero-downtime deploy panicked", "panic", r)
				}
			}()
			_, _ = svc.DeployRelease(cfg.BackgroundContext, release.ID)
			ok, _ := svc.RunHealthChecks(cfg.BackgroundContext, release.ID)
			if ok {
				_, _ = svc.PromoteRelease(cfg.BackgroundContext, release.ID)
			}
		}()

		return c.Status(201).JSON(fiber.Map{"data": release})
	})

	protected.Get("/servers/:id/deployments", requireServerPermission(cfg, ""), func(c *fiber.Ctx) error {
		releases, err := svc.ListReleases(c.Context(), c.Params("id"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": releases})
	})

	protected.Get("/servers/:id/deployments/:releaseId", requireServerPermission(cfg, ""), func(c *fiber.Ctx) error {
		release, err := svc.GetRelease(c.Context(), c.Params("releaseId"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": release})
	})

	protected.Post("/servers/:id/deployments/:releaseId/rollback", mutationLimiter, requireServerPermission(cfg, ""), func(c *fiber.Ctx) error {
		release, err := svc.RollbackRelease(c.Context(), c.Params("releaseId"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": release})
	})

	protected.Put("/servers/:id/health-check", mutationLimiter, requireServerPermission(cfg, ""), func(c *fiber.Ctx) error {
		var req zerodowntime.HealthCheckConfig
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		config, err := svc.UpsertHealthCheckConfig(c.Context(), c.Params("id"), &req)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": config})
	})

	protected.Get("/servers/:id/health-check", requireServerPermission(cfg, ""), func(c *fiber.Ctx) error {
		config, err := svc.GetHealthCheckConfig(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "health check config not found"})
		}
		return c.JSON(fiber.Map{"data": config})
	})

	protected.Get("/servers/:id/deployments/:releaseId/health", requireServerPermission(cfg, ""), func(c *fiber.Ctx) error {
		results, err := svc.GetHealthCheckResults(c.Context(), c.Params("releaseId"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": results})
	})

	protected.Post("/servers/:id/deployments/:releaseId/promote", mutationLimiter, requireServerPermission(cfg, ""), func(c *fiber.Ctx) error {
		release, err := svc.PromoteRelease(c.Context(), c.Params("releaseId"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": release})
	})

	protected.Get("/servers/:id/deployments/:releaseId/events", requireServerPermission(cfg, ""), func(c *fiber.Ctx) error {
		events, err := svc.GetDeploymentEvents(c.Context(), c.Params("releaseId"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": events})
	})
}
