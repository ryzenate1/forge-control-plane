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
// Self-upgrade replaces the control plane itself, so it is gated like the
// panel-settings surface it mutates: reads require settings.read and every
// mutation settings.write, in addition to the admin role. A key scoped only to,
// say, servers.read must not be able to roll the API binary.
func registerUpgradeRoutes(protected fiber.Router, cfg Config, svc *upgradesvc.Service, mutationLimiter fiber.Handler) {
	if svc == nil || cfg.Store == nil {
		return
	}

	up := protected.Group("/upgrade", requireRole("admin"))

	// ---- Version check -------------------------------------------------------

	up.Get("/versions", requireAdminScope("settings.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		infos, err := svc.CheckForUpgrades(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": infos})
	})

	up.Get("/system-status", requireAdminScope("settings.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		status, err := svc.GetSystemStatus(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": status})
	})

	// ---- Plans CRUD ----------------------------------------------------------

	up.Get("/plans", requireAdminScope("settings.read"), func(c *fiber.Ctx) error {
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

	up.Get("/plans/:id", requireAdminScope("settings.read"), func(c *fiber.Ctx) error {
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

	up.Post("/plans", mutationLimiter, requireAdminScope("settings.write"), func(c *fiber.Ctx) error {
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

	up.Post("/plans/:id/execute", mutationLimiter, requireAdminScope("settings.write"), func(c *fiber.Ctx) error {
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

	up.Post("/plans/:id/cancel", mutationLimiter, requireAdminScope("settings.write"), func(c *fiber.Ctx) error {
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

	// up.Post("/plans/:id/verify", ...) deliberately does NOT certify health.
	// upgrade.Service.VerifyUpgrade loops over the plan's components calling
	// verifyComponentHealth, which is a stub that returns nil unconditionally
	// ("For now, just return success"). Answering {ok:true, healthy:true} from
	// that is a green light produced by nothing at all: an operator would read
	// "healthy" after a half-applied upgrade and stop watching. The plan is
	// still resolved (so a bad id is a real 404), and the endpoint reports 501
	// until the service performs an actual probe — the same fail-loud shape used
	// by POST /install-workflows/:id/execute when its executor is not wired.
	up.Post("/plans/:id/verify", mutationLimiter, requireAdminScope("settings.write"), func(c *fiber.Ctx) error {
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
		return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
			"verified":     false,
			"healthy":      false,
			"healthProbed": false,
			"planId":       id,
			"planState":    plan.Status,
			"error":        "component health verification is not implemented in the upgrade service",
			"hint":         "VerifyUpgrade calls verifyComponentHealth, which returns success without probing. Inspect GET /upgrade/system-status and the per-component health endpoints, or implement the probe, before treating an upgrade as verified.",
		})
	})

	up.Delete("/plans/:id", mutationLimiter, requireAdminScope("settings.write"), func(c *fiber.Ctx) error {
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
