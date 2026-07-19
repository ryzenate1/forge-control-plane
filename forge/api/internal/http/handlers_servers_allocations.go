package http

import (
	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerAllocationsServerRoutes(protected fiber.Router, cfg Config, runner *scheduleRunner, clusterManager *clustermanager.Service, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {

	protected.Get("/servers/:id/allocations", requireServerPermission(cfg, store.PermAllocationRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		allocations, err := cfg.Store.ListServerAllocations(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(allocations)
	})

	protected.Patch("/servers/:id/allocations/:allocationId", mutationLimiter, requireServerPermission(cfg, store.PermAllocationUpdate), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req UpdateAllocationRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		allocation, err := cfg.Store.UpdateServerAllocation(ctx, c.Params("id"), c.Params("allocationId"), store.UpdateAllocationRequest{Alias: req.Alias, Notes: req.Notes}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(allocation)
	})

	// Enforce user-level subuser cap (for the owner of the server).

	protected.Post("/servers/:id/allocations", mutationLimiter, requireServerPermission(cfg, store.PermAllocationCreate), func(c *fiber.Ctx) error {
		var body struct {
			AllocationID string `json:"allocationId"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if body.AllocationID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "allocationId is required")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		// Enforce user-level allocation cap.
		ctx, cancel := requestContext()
		defer cancel()
		if ownerID, ok := serverOwner(ctx, cfg, c.Params("id")); ok {
			if err := cfg.Store.CheckUserCanCreateAllocation(ctx, ownerID); err != nil {
				if store.IsUserLimitError(err) {
					return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
				}
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.AssignAllocationToServer(ctx, c.Params("id"), body.AllocationID, actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true})
	})

	protected.Delete("/servers/:id/allocations/:allocationId", mutationLimiter, requireServerPermission(cfg, store.PermAllocationDelete), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.UnassignAllocationFromServer(ctx, c.Params("id"), c.Params("allocationId"), actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/allocations/:allocationId/primary", mutationLimiter, requireServerPermission(cfg, store.PermAllocationUpdate), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.SetPrimaryAllocation(ctx, c.Params("id"), c.Params("allocationId"), actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})
}
