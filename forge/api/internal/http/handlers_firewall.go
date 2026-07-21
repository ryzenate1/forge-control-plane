package http

import (
	"github.com/gofiber/fiber/v2"
)

func registerFirewallRoutes(protected fiber.Router, cfg Config) {
	fw := protected.Group("/host/firewall")

	fw.Get("/status", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Daemon == nil || cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client and store are required")
		}
		target, err := resolveNodeHostTarget(cfg, c.Query("nodeId"))
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		status, err := cfg.Daemon.GetFirewallStatus(ctx, target.NodeURL, target.NodeToken)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(status)
	})

	fw.Post("/enable", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Daemon == nil || cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client and store are required")
		}
		target, err := resolveNodeHostTarget(cfg, c.Query("nodeId"))
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		result, err := cfg.Daemon.EnableFirewall(ctx, target.NodeURL, target.NodeToken)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(result)
	})

	fw.Post("/disable", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Daemon == nil || cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client and store are required")
		}
		target, err := resolveNodeHostTarget(cfg, c.Query("nodeId"))
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		result, err := cfg.Daemon.DisableFirewall(ctx, target.NodeURL, target.NodeToken)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(result)
	})

	fw.Get("/rules", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Daemon == nil || cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client and store are required")
		}
		target, err := resolveNodeHostTarget(cfg, c.Query("nodeId"))
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		rules, err := cfg.Daemon.ListFirewallRules(ctx, target.NodeURL, target.NodeToken)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(rules)
	})

	fw.Post("/rules", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Daemon == nil || cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client and store are required")
		}
		target, err := resolveNodeHostTarget(cfg, c.Query("nodeId"))
		if err != nil {
			return err
		}
		var body map[string]any
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		rule, err := cfg.Daemon.AddFirewallRule(ctx, target.NodeURL, target.NodeToken, body)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(rule)
	})

	fw.Delete("/rules/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Daemon == nil || cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client and store are required")
		}
		target, err := resolveNodeHostTarget(cfg, c.Query("nodeId"))
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Daemon.DeleteFirewallRule(ctx, target.NodeURL, target.NodeToken, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	fw.Put("/rules/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Daemon == nil || cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client and store are required")
		}
		target, err := resolveNodeHostTarget(cfg, c.Query("nodeId"))
		if err != nil {
			return err
		}
		var body map[string]any
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		rule, err := cfg.Daemon.UpdateFirewallRule(ctx, target.NodeURL, target.NodeToken, c.Params("id"), body)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(rule)
	})

	fw.Post("/port", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Daemon == nil || cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client and store are required")
		}
		target, err := resolveNodeHostTarget(cfg, c.Query("nodeId"))
		if err != nil {
			return err
		}
		var body map[string]any
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		result, err := cfg.Daemon.OpenFirewallPort(ctx, target.NodeURL, target.NodeToken, body)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(result)
	})

	fw.Get("/forward", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Daemon == nil || cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client and store are required")
		}
		target, err := resolveNodeHostTarget(cfg, c.Query("nodeId"))
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		forwards, err := cfg.Daemon.ListPortForwards(ctx, target.NodeURL, target.NodeToken)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(forwards)
	})

	fw.Post("/forward", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Daemon == nil || cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client and store are required")
		}
		target, err := resolveNodeHostTarget(cfg, c.Query("nodeId"))
		if err != nil {
			return err
		}
		var body map[string]any
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		pf, err := cfg.Daemon.AddPortForward(ctx, target.NodeURL, target.NodeToken, body)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(pf)
	})

	fw.Delete("/forward/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Daemon == nil || cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client and store are required")
		}
		target, err := resolveNodeHostTarget(cfg, c.Query("nodeId"))
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Daemon.DeletePortForward(ctx, target.NodeURL, target.NodeToken, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
}
