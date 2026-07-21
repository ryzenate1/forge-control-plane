package http

import (
	buildpacksvc "gamepanel/forge/internal/services/buildpack"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerBuildpackRoutes(protected fiber.Router, cfg Config, buildpackSvc *buildpacksvc.Service, mutationLimiter fiber.Handler) {
	if buildpackSvc == nil || cfg.Store == nil {
		return
	}

	protected.Get("/admin/buildpacks", requireRole("admin"), func(c *fiber.Ctx) error {
		bps, err := cfg.Store.ListBuildpacks(c.Context())
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		if bps == nil {
			bps = []store.Buildpack{}
		}
		return c.JSON(fiber.Map{"data": bps})
	})
	protected.Post("/admin/buildpacks", requireRole("admin"), mutationLimiter, func(c *fiber.Ctx) error {
		var req store.CreateBuildpackRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		if req.Name == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
		}
		bp, err := cfg.Store.CreateBuildpack(c.Context(), req)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": bp})
	})

	protected.Get("/servers/:id/buildpacks", func(c *fiber.Ctx) error {
		sb, err := cfg.Store.ListServerBuildpacks(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		if sb == nil {
			sb = []store.ServerBuildpack{}
		}
		return c.JSON(fiber.Map{"data": sb})
	})
	protected.Post("/servers/:id/buildpacks", mutationLimiter, func(c *fiber.Ctx) error {
		var req struct {
			BuildpackID string `json:"buildpackId"`
			Priority    int    `json:"priority"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		if req.BuildpackID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "buildpackId is required"})
		}
		sb, err := cfg.Store.AssignBuildpackToServer(c.Context(), c.Params("id"), req.BuildpackID, req.Priority)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": sb})
	})
	protected.Delete("/servers/:id/buildpacks/:buildpackId", mutationLimiter, func(c *fiber.Ctx) error {
		if err := cfg.Store.RemoveServerBuildpack(c.Context(), c.Params("id"), c.Params("buildpackId")); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	protected.Post("/builds/language/detect", func(c *fiber.Ctx) error {
		var req struct {
			Files []string `json:"files"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		if len(req.Files) == 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "files list is required"})
		}
		langInfo, err := buildpackSvc.DetectBuildpack(c.Context(), req.Files)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": langInfo})
	})

	builds := protected.Group("/servers/:id/builds")
	builds.Post("/", mutationLimiter, func(c *fiber.Ctx) error {
		var req struct {
			BuildpackID *string `json:"buildpackId"`
		}
		_ = c.BodyParser(&req)
		build, err := buildpackSvc.TriggerBuild(c.Context(), c.Params("id"), req.BuildpackID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"data": build})
	})
	builds.Get("/", func(c *fiber.Ctx) error {
		list, err := cfg.Store.ListAppBuilds(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		if list == nil {
			list = []store.AppBuild{}
		}
		return c.JSON(fiber.Map{"data": list})
	})
	builds.Get("/:buildId", func(c *fiber.Ctx) error {
		build, err := buildpackSvc.GetBuildStatus(c.Context(), c.Params("buildId"))
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": build})
	})
}
