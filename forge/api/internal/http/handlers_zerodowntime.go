package http

import (
	"context"
	"log/slog"
	"time"

	"gamepanel/forge/internal/services/zerodowntime"
	"gamepanel/forge/internal/store"
	"github.com/gofiber/fiber/v2"
)

func registerZeroDowntimeRoutes(protected fiber.Router, cfg Config, svc *zerodowntime.Service, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	protected.Post("/servers/:id/deployments", mutationLimiter, requireServerPermission(cfg, store.PermControlRestart), func(c *fiber.Ctx) error {
		var req struct {
			ImageTag string `json:"imageTag"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		if req.ImageTag == "" {
			return c.Status(400).JSON(fiber.Map{"error": "imageTag is required"})
		}

		ctx, cancel := requestContext()
		defer cancel()
		release, err := svc.CreateRelease(ctx, c.Params("id"), req.ImageTag)
		if err != nil {
			return respondInternalError(c, err)
		}

		// Deploy the release synchronously so a 201 means the deploy step was
		// accepted, not merely that a row was inserted. Health checks and
		// promotion still run in the background, but their outcome is surfaced
		// through the release status, health results and deployment events —
		// never discarded. A failed deploy returns 500 instead of a 201 that
		// later silently fails.
		// The deploy outlives the request, so it must not inherit the short
		// request timeout; it keeps the request's values only for correlation.
		bgCtx := cfg.BackgroundContext
		if bgCtx == nil {
			bgCtx = context.WithoutCancel(c.Context())
		}
		deployed, err := svc.DeployRelease(bgCtx, release.ID)
		if err != nil {
			return respondInternalError(c, err)
		}

		go func(releaseID string, base context.Context) {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("zero-downtime deploy panicked", "panic", r, "releaseId", releaseID)
				}
			}()
			bg, cancel := context.WithTimeout(context.WithoutCancel(base), 15*time.Minute)
			defer cancel()
			ok, err := svc.RunHealthChecks(bg, releaseID)
			if err != nil {
				slog.Error("zero-downtime health checks failed", "releaseId", releaseID, "error", err)
			}
			if ok {
				if _, err := svc.PromoteRelease(bg, releaseID); err != nil {
					slog.Error("zero-downtime promote failed", "releaseId", releaseID, "error", err)
				}
			}
		}(release.ID, bgCtx)

		return c.Status(201).JSON(fiber.Map{"data": deployed})
	})

	protected.Get("/servers/:id/deployments", requireServerPermission(cfg, store.PermServerView), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		releases, err := svc.ListReleases(ctx, c.Params("id"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": releases})
	})

	protected.Get("/servers/:id/deployments/:releaseId", requireServerPermission(cfg, store.PermServerView), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		release, err := svc.GetRelease(ctx, c.Params("releaseId"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": release})
	})

	protected.Post("/servers/:id/deployments/:releaseId/rollback", mutationLimiter, requireServerPermission(cfg, store.PermControlRestart), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		release, err := svc.RollbackRelease(ctx, c.Params("releaseId"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": release})
	})

	protected.Put("/servers/:id/health-check", mutationLimiter, requireServerPermission(cfg, store.PermServerSettings), func(c *fiber.Ctx) error {
		var req zerodowntime.HealthCheckConfig
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		ctx, cancel := requestContext()
		defer cancel()
		config, err := svc.UpsertHealthCheckConfig(ctx, c.Params("id"), &req)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": config})
	})

	protected.Get("/servers/:id/health-check", requireServerPermission(cfg, store.PermServerView), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		config, err := svc.GetHealthCheckConfig(ctx, c.Params("id"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "health check config not found"})
		}
		return c.JSON(fiber.Map{"data": config})
	})

	protected.Get("/servers/:id/deployments/:releaseId/health", requireServerPermission(cfg, store.PermServerView), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		results, err := svc.GetHealthCheckResults(ctx, c.Params("releaseId"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": results})
	})

	protected.Post("/servers/:id/deployments/:releaseId/promote", mutationLimiter, requireServerPermission(cfg, store.PermControlRestart), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		release, err := svc.PromoteRelease(ctx, c.Params("releaseId"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": release})
	})

	protected.Get("/servers/:id/deployments/:releaseId/events", requireServerPermission(cfg, store.PermActivityRead), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		events, err := svc.GetDeploymentEvents(ctx, c.Params("releaseId"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": events})
	})
}
