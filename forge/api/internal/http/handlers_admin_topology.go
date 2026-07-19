package http

import (
	"strings"

	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/services/evacuationplanner"
	migrationservice "gamepanel/forge/internal/services/migration"
	"gamepanel/forge/internal/services/noderegistry"
	recoverysvc "gamepanel/forge/internal/services/recovery"
	reservationsvc "gamepanel/forge/internal/services/reservations"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerTopologyAdminRoutes(protected fiber.Router, cfg Config, nodeRegistry *noderegistry.Service, clusterManager *clustermanager.Service, evacuationPlanner *evacuationplanner.Service, migrationService *migrationservice.Service, reservationManager *reservationsvc.Manager, recoveryCoordinator *recoverysvc.Coordinator, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {

	protected.Get("/regions", adminIPAccess, requireRole("admin"), requireAdminScope("regions.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		regions, err := nodeRegistry.ListRegions(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(regions)
	})

	protected.Get("/regions/:id", adminIPAccess, requireRole("admin"), requireAdminScope("regions.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		region, err := nodeRegistry.GetRegion(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "region not found")
		}
		return c.JSON(region)
	})

	protected.Get("/regions/:id/capacity", adminIPAccess, requireRole("admin"), requireAdminScope("regions.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		snapshots, err := clusterManager.RegionCapacity(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(snapshots)
	})

	protected.Get("/regions/:id/cluster", adminIPAccess, requireRole("admin"), requireAdminScope("regions.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		capacity, err := clusterManager.RegionCapacity(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		nodes, err := nodeRegistry.ListNodes(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		lifecycle := []any{}
		for _, node := range nodes {
			if node.RegionID == nil || *node.RegionID != c.Params("id") {
				continue
			}
			view, err := nodeRegistry.LifecycleView(ctx, node.ID)
			if err != nil {
				continue
			}
			lifecycle = append(lifecycle, view)
		}
		return c.JSON(fiber.Map{
			"regionId": c.Params("id"),
			"capacity": capacity,
			"nodes":    lifecycle,
		})
	})

	protected.Post("/regions", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("regions.write"), func(c *fiber.Ctx) error {
		var req CreateRegionRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := Validate(&req); err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(err)
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		region, err := nodeRegistry.CreateRegion(ctx, store.CreateRegionRequest{
			Name:        req.Name,
			Slug:        req.Slug,
			Description: req.Description,
			Enabled:     req.Enabled,
		}, actorID)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "duplicate") {
				return fiber.NewError(fiber.StatusConflict, err.Error())
			}
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(region)
	})

	protected.Patch("/regions/:id", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("regions.write"), func(c *fiber.Ctx) error {
		var req UpdateRegionRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		region, err := nodeRegistry.UpdateRegion(ctx, c.Params("id"), store.UpdateRegionRequest{
			Name:        req.Name,
			Slug:        req.Slug,
			Description: req.Description,
			Enabled:     req.Enabled,
		}, actorID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return fiber.NewError(fiber.StatusNotFound, err.Error())
			}
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(region)
	})

	protected.Delete("/regions/:id", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("regions.delete"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := nodeRegistry.DeleteRegion(ctx, c.Params("id"), actorID); err != nil {
			if strings.Contains(err.Error(), "not found") {
				return fiber.NewError(fiber.StatusNotFound, err.Error())
			}
			if strings.Contains(err.Error(), "servers") || strings.Contains(err.Error(), "nodes") {
				return fiber.NewError(fiber.StatusConflict, err.Error())
			}
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// ---- Locations CRUD ----

	protected.Get("/locations", requireAdminScope("locations.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		locations, err := cfg.Store.ListLocations(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(locations)
	})

	protected.Get("/locations/:id", requireAdminScope("locations.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		loc, err := cfg.Store.GetLocation(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "location not found")
		}
		return c.JSON(loc)
	})

	protected.Post("/locations", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("locations.write"), func(c *fiber.Ctx) error {
		var req CreateLocationRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := Validate(&req); err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(err)
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		loc, err := cfg.Store.CreateLocation(ctx, store.CreateLocationRequest{
			Short: req.Short,
			Long:  req.Long,
		}, actorID)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "duplicate") {
				return fiber.NewError(fiber.StatusConflict, err.Error())
			}
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(loc)
	})

	protected.Patch("/locations/:id", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("locations.write"), func(c *fiber.Ctx) error {
		var req UpdateLocationRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		loc, err := cfg.Store.UpdateLocation(ctx, c.Params("id"), store.UpdateLocationRequest{
			Short: req.Short,
			Long:  req.Long,
		}, actorID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return fiber.NewError(fiber.StatusNotFound, err.Error())
			}
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(loc)
	})

	protected.Delete("/locations/:id", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("locations.delete"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.DeleteLocation(ctx, c.Params("id"), actorID); err != nil {
			if strings.Contains(err.Error(), "not found") {
				return fiber.NewError(fiber.StatusNotFound, err.Error())
			}
			if strings.Contains(err.Error(), "nodes") {
				return fiber.NewError(fiber.StatusConflict, err.Error())
			}
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})
}
