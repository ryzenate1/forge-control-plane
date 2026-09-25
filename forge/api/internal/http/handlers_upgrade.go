package http

import (
	"errors"
	"strconv"
	"strings"

	upgradesvc "gamepanel/forge/internal/services/upgrade"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// registerUpgradeRoutes exposes the control-plane self-upgrade service. The
// upgrade.Service handles version checks, plan creation, execution with
// automatic backup + rollback, and health verification. These routes make it
// operable from the admin UI (/admin/upgrade).
//
// All endpoints require admin. Execution is gated behind a confirm step in
// the UI — the API itself trusts the authenticated admin caller.
func registerUpgradeRoutes(protected fiber.Router, cfg Config, svc *upgradesvc.Service, mutationLimiter fiber.Handler) {
	if svc == nil || cfg.Store == nil {
		return
	}

	up := protected.Group("/upgrade", requireRole("admin"))

	// ---- Version check -------------------------------------------------------

	up.Get("/versions", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		infos, err := svc.CheckForUpgrades(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": infos})
	})

	up.Get("/system-status", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		status, err := svc.GetSystemStatus(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": status})
	})

	// ---- Plans CRUD ----------------------------------------------------------

	up.Get("/plans", func(c *fiber.Ctx) error {
		limit := 50
		if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
			n, err := strconv.Atoi(raw)
			if err == nil && n > 0 && n <= 500 {
				limit = n
			}
		}
		ctx, cancel := requestContext()
		defer cancel()
		plans, err := svc.ListUpgradeHistory(ctx, limit)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": plans})
	})

	up.Get("/plans/:id", func(c *fiber.Ctx) error {
		id := strings.TrimSpace(c.Params("id"))
		if id == "" {
			return fiber.NewError(fiber.StatusBadRequest, "plan id is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plan, err := svc.GetUpgradeStatus(ctx, id)
		if err != nil {
			return mapUpgradeErr(err)
		}
		return c.JSON(fiber.Map{"data": plan})
	})

	up.Post("/plans", mutationLimiter, func(c *fiber.Ctx) error {
		var req struct {
			Type       string   `json:"type"`
			Components []string `json:"components"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		req.Type = strings.TrimSpace(req.Type)
		if req.Type == "" {
			req.Type = "full"
		}
		validTypes := map[string]bool{"api": true, "web": true, "beacon": true, "database": true, "full": true}
		if !validTypes[req.Type] {
			return fiber.NewError(fiber.StatusBadRequest, "type must be one of: api, web, beacon, database, full")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plan, err := svc.CreateUpgradePlan(ctx, upgradesvc.UpgradeType(req.Type), req.Components)
		if err != nil {
			return mapUpgradeErr(err)
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": plan})
	})

	// ---- Execute / cancel / verify -------------------------------------------

	up.Post("/plans/:id/execute", mutationLimiter, func(c *fiber.Ctx) error {
		id := strings.TrimSpace(c.Params("id"))
		if id == "" {
			return fiber.NewError(fiber.StatusBadRequest, "plan id is required")
		}
		ctx, cancel := longRequestContext()
		defer cancel()
		result, err := svc.ExecuteUpgradePlan(ctx, id)
		if err != nil {
			return mapUpgradeErr(err)
		}
		return c.JSON(fiber.Map{"data": result})
	})

	up.Post("/plans/:id/cancel", mutationLimiter, func(c *fiber.Ctx) error {
		id := strings.TrimSpace(c.Params("id"))
		if id == "" {
			return fiber.NewError(fiber.StatusBadRequest, "plan id is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.CancelUpgrade(ctx, id); err != nil {
			return mapUpgradeErr(err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	up.Post("/plans/:id/verify", func(c *fiber.Ctx) error {
		id := strings.TrimSpace(c.Params("id"))
		if id == "" {
			return fiber.NewError(fiber.StatusBadRequest, "plan id is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plan, err := svc.GetUpgradeStatus(ctx, id)
		if err != nil {
			return mapUpgradeErr(err)
		}
		if err := svc.VerifyUpgrade(ctx, plan); err != nil {
			return mapUpgradeErr(err)
		}
		return c.JSON(fiber.Map{"ok": true, "healthy": true})
	})

	up.Delete("/plans/:id", mutationLimiter, func(c *fiber.Ctx) error {
		id := strings.TrimSpace(c.Params("id"))
		if id == "" {
			return fiber.NewError(fiber.StatusBadRequest, "plan id is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteUpgradePlan(ctx, id); err != nil {
			return mapUpgradeErr(err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})
}

func mapUpgradeErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, store.ErrUpgradePlanNotFound) {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "not found"), strings.Contains(low, "does not exist"):
		return fiber.NewError(fiber.StatusNotFound, msg)
	case strings.Contains(low, "already"), strings.Contains(low, "in progress"), strings.Contains(low, "cannot"):
		return fiber.NewError(fiber.StatusConflict, msg)
	case strings.Contains(low, "required"), strings.Contains(low, "invalid"), strings.Contains(low, "unknown"):
		return fiber.NewError(fiber.StatusBadRequest, msg)
	}
	return fiber.NewError(fiber.StatusInternalServerError, msg)
}
