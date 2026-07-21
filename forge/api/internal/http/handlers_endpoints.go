package http

import (
	"strconv"
	"strings"

	"gamepanel/forge/internal/domain"
	envsvc "gamepanel/forge/internal/services/environments"

	"github.com/gofiber/fiber/v2"
)

func registerEndpointRoutes(protected fiber.Router, cfg Config, svc *envsvc.Service) {
	if svc == nil {
		return
	}

	// ---- Infrastructure Endpoints (Portainer-style Environment abstraction) ----
	// These are logical groupings over one or more Nodes, NOT project environments.

	protected.Get("/endpoints", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		endpoints, err := svc.List(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"data": endpoints})
	})

	protected.Post("/endpoints", requireRole("admin"), func(c *fiber.Ctx) error {
		var req domain.CreateEndpointRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.Name) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "name is required")
		}

		claims := c.Locals("user").(tokenClaims)
		actorID := claims.Sub
		ctx, cancel := requestContext()
		defer cancel()
		ep, err := svc.Create(ctx, req, &actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(ep)
	})

	protected.Get("/endpoints/:id", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		ep, err := svc.Get(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "endpoint not found")
		}
		return c.JSON(ep)
	})

	protected.Put("/endpoints/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		var req domain.UpdateEndpointRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		claims := c.Locals("user").(tokenClaims)
		actorID := claims.Sub
		ctx, cancel := requestContext()
		defer cancel()
		ep, err := svc.Update(ctx, c.Params("id"), req, &actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(ep)
	})

	protected.Delete("/endpoints/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		claims := c.Locals("user").(tokenClaims)
		actorID := claims.Sub
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.Delete(ctx, c.Params("id"), &actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// ---- Node membership ----

	protected.Get("/endpoints/:id/nodes", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		nodes, err := cfg.Store.ListInfraEndpointNodes(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		result := make([]fiber.Map, 0, len(nodes))
		for _, en := range nodes {
			node, err := cfg.Store.GetNode(ctx, en.NodeID)
			if err != nil {
				continue
			}
			result = append(result, fiber.Map{
				"id":         en.ID,
				"nodeId":     en.NodeID,
				"nodeName":   node.Name,
				"nodeStatus": node.Status,
				"createdAt":  en.CreatedAt,
			})
		}
		return c.JSON(fiber.Map{"data": result})
	})

	protected.Post("/endpoints/:id/nodes", requireRole("admin"), func(c *fiber.Ctx) error {
		var req struct {
			NodeID string `json:"nodeId"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.AddNode(ctx, c.Params("id"), req.NodeID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true})
	})

	protected.Delete("/endpoints/:id/nodes/:nodeId", requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.RemoveNode(ctx, c.Params("id"), c.Params("nodeId")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// ---- Access Policies ----

	protected.Get("/endpoints/:id/policies", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		policies, err := svc.ListAccessPolicies(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"data": policies})
	})

	protected.Post("/endpoints/:id/policies", requireRole("admin"), func(c *fiber.Ctx) error {
		var req struct {
			PrincipalType string `json:"principalType"`
			PrincipalID   string `json:"principalId"`
			Role          string `json:"role"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		policy, err := svc.SetAccessPolicy(ctx, c.Params("id"), req.PrincipalType, req.PrincipalID, req.Role)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(policy)
	})

	protected.Delete("/endpoints/:id/policies/:principalType/:principalId", requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.RemoveAccessPolicy(ctx, c.Params("id"), c.Params("principalType"), c.Params("principalId")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// ---- Diagnostics ----

	protected.Get("/endpoints/:id/diagnostics", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		diag, err := svc.Diagnostics(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(diag)
	})

	protected.Get("/endpoints/:id/inventory", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		summary, err := svc.Inventory(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(summary)
	})

	// ---- Health History ----

	protected.Get("/endpoints/:id/health", func(c *fiber.Ctx) error {
		limit := 50
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 200 {
				limit = parsed
			}
		}
		ctx, cancel := requestContext()
		defer cancel()
		records, err := svc.HealthHistory(ctx, c.Params("id"), limit)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"data": records})
	})
}
