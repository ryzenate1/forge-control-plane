package http

import (
	"strconv"
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

func registerNodesAdminRoutes(protected fiber.Router, cfg Config, nodeRegistry *noderegistry.Service, clusterManager *clustermanager.Service, evacuationPlanner *evacuationplanner.Service, migrationService *migrationservice.Service, reservationManager *reservationsvc.Manager, recoveryCoordinator *recoverysvc.Coordinator, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {

	protected.Get("/nodes", requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		page := 1
		if p := c.Query("page"); p != "" {
			if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
				page = parsed
			}
		}
		perPage := 50
		if pp := c.Query("per_page"); pp != "" {
			if parsed, err := strconv.Atoi(pp); err == nil && parsed > 0 && parsed <= 100 {
				perPage = parsed
			}
		}
		offset := (page - 1) * perPage
		ctx, cancel := requestContext()
		defer cancel()
		nodes, err := cfg.Store.ListNodesPaginated(ctx, offset, perPage)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{
			"data": nodes,
			"meta": fiber.Map{
				"pagination": fiber.Map{
					"current":  page,
					"count":    len(nodes),
					"per_page": perPage,
				},
			},
		})
	})

	protected.Post("/nodes", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("nodes.write"), func(c *fiber.Ctx) error {
		var req CreateNodeRequest
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
		storeReq := store.CreateNodeRequest{
			Name:               req.Name,
			Region:             req.Region,
			RegionID:           req.RegionID,
			Description:        req.Description,
			LocationID:         req.LocationID,
			BaseURL:            req.BaseURL,
			FQDN:               req.FQDN,
			Scheme:             req.Scheme,
			BehindProxy:        req.BehindProxy,
			Public:             req.Public == nil || *req.Public,
			Maintenance:        req.MaintenanceMode != nil && *req.MaintenanceMode,
			MemoryMB:           req.MemoryMB,
			DiskMB:             req.DiskMB,
			UploadSizeMB:       req.UploadSizeMB,
			DaemonBase:         req.DaemonBase,
			DaemonListen:       req.DaemonListen,
			DaemonSFTP:         req.DaemonSFTP,
			MemoryOverallocate: derefInt(req.MemoryOverallocate),
			DiskOverallocate:   derefInt(req.DiskOverallocate),
			CPUCores:           derefInt(req.CPUCores),
			DisplayName:        req.DisplayName,
			PublicHostname:     req.PublicHostname,
			DaemonSFTPAlias:    req.DaemonSFTPAlias,
			DaemonConnect:      derefInt(req.DaemonConnect),
			CPUOverallocate:    derefInt(req.CPUOverallocate),
			Tags:               req.Tags,
		}
		node, token, err := nodeRegistry.RegisterNode(ctx, storeReq, actorID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return fiber.NewError(fiber.StatusNotFound, err.Error())
			}
			if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "duplicate") {
				return fiber.NewError(fiber.StatusConflict, err.Error())
			}
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		cfg.Store.DispatchWebhookEvent("node:created", map[string]any{
			"subject_type": "node",
			"subject_id":   node.ID,
			"name":         node.Name,
		})
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"node":  node,
			"token": token,
			"meta": fiber.Map{
				"resource": "/nodes/" + node.ID,
			},
		})
	})

	protected.Get("/nodes/:id", requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		node, err := nodeRegistry.GetNode(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "node not found")
		}
		return c.JSON(node)
	})

	protected.Get("/nodes/:id/configuration", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		config, err := cfg.Store.NodeConfiguration(ctx, c.Params("id"), strings.TrimRight(c.Protocol()+"://"+c.Hostname(), "/"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "node not found")
		}
		return c.JSON(config)
	})

	protected.Get("/nodes/:id/deployment", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		config, err := cfg.Store.NodeConfiguration(ctx, c.Params("id"), strings.TrimRight(c.Protocol()+"://"+c.Hostname(), "/"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "node not found")
		}
		node, err := cfg.Store.GetNode(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "node not found")
		}
		return c.JSON(fiber.Map{
			"node":         node,
			"config":       config,
			"deployMethod": "Registration tokens are issued at node creation or via POST /api/v1/nodes/:id/rotate-token",
		})
	})

	protected.Patch("/nodes/:id", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("nodes.write"), func(c *fiber.Ctx) error {
		var req UpdateNodeRequest
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
		node, err := nodeRegistry.PatchNode(ctx, c.Params("id"), store.NodePatch{
			Name: req.Name, Description: req.Description, LocationID: req.LocationID,
			BaseURL: req.BaseURL, FQDN: req.FQDN, Scheme: req.Scheme, BehindProxy: req.BehindProxy,
			Public: req.Public, DesiredState: req.DesiredState, Maintenance: req.Maintenance, Draining: req.Draining,
			MemoryMB: req.MemoryMB, DiskMB: req.DiskMB, UploadSizeMB: req.UploadSizeMB,
			DaemonBase: req.DaemonBase, DaemonListen: req.DaemonListen, DaemonSFTP: req.DaemonSFTP,
			Status:             req.Status,
			MemoryOverallocate: req.MemoryOverallocate,
			DiskOverallocate:   req.DiskOverallocate,
			CPUCores:           req.CPUCores,
			DisplayName:        req.DisplayName,
			PublicHostname:     req.PublicHostname,
			DaemonSFTPAlias:    req.DaemonSFTPAlias,
			DaemonConnect:      req.DaemonConnect,
			CPUOverallocate:    req.CPUOverallocate,
			Tags:               req.Tags,
		}, actorID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return fiber.NewError(fiber.StatusNotFound, err.Error())
			}
			if strings.Contains(err.Error(), "conflict") || strings.Contains(err.Error(), "invalid transition") {
				return fiber.NewError(fiber.StatusConflict, err.Error())
			}
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		cfg.Store.DispatchWebhookEvent("node:updated", map[string]any{
			"subject_type": "node",
			"subject_id":   node.ID,
			"name":         node.Name,
		})
		return c.JSON(node)
	})

	protected.Delete("/nodes/:id", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("nodes.delete"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := nodeRegistry.DeleteNode(ctx, c.Params("id"), actorID); err != nil {
			if strings.Contains(err.Error(), "not found") {
				return fiber.NewError(fiber.StatusNotFound, "node not found")
			}
			if strings.Contains(err.Error(), "evacuate or remove") {
				return fiber.NewError(fiber.StatusConflict, err.Error())
			}
			return fiber.NewError(fiber.StatusInternalServerError, "failed to delete node")
		}
		cfg.Store.DispatchWebhookEvent("node:deleted", map[string]any{
			"subject_type": "node",
			"subject_id":   c.Params("id"),
		})
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/nodes/:id/rotate-token", adminIPAccess, mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		token, err := nodeRegistry.RotateNodeToken(ctx, c.Params("id"), actorID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return fiber.NewError(fiber.StatusNotFound, err.Error())
			}
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"token": token})
	})

	// ---- Deployable node ----

	protected.Post("/nodes/deployable", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			MemoryMB    int      `json:"memoryMb"`
			DiskMB      int      `json:"diskMb"`
			CPUShares   int64    `json:"cpuShares"`
			LocationIDs []string `json:"locationIds"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid deployable request: "+err.Error())
		}
		ctx, cancel := requestContext()
		defer cancel()
		nodes, err := cfg.Store.ListNodes(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		// Filter by location, capacity, and health.
		var candidates []store.Node
		for _, node := range nodes {
			if node.Maintenance || node.Draining {
				continue
			}
			if len(req.LocationIDs) > 0 {
				match := false
				for _, loc := range req.LocationIDs {
					if node.LocationID != nil && *node.LocationID == loc {
						match = true
						break
					}
				}
				if !match {
					continue
				}
			}
			if node.NodeMemoryMB != nil && node.MemoryMB > 0 && *node.NodeMemoryMB+req.MemoryMB > node.MemoryMB {
				continue
			}
			if node.NodeDiskMB != nil && node.DiskMB > 0 && *node.NodeDiskMB+req.DiskMB > node.DiskMB {
				continue
			}
			candidates = append(candidates, node)
		}
		if len(candidates) == 0 {
			return fiber.NewError(fiber.StatusNotFound, "no suitable node available")
		}
		// Return the least-loaded node (lowest memory used/memory limit ratio).
		best := candidates[0]
		bestRatio := 1.0
		for _, node := range candidates {
			if node.NodeMemoryMB != nil && node.MemoryMB > 0 {
				ratio := float64(*node.NodeMemoryMB) / float64(node.MemoryMB)
				if ratio < bestRatio {
					bestRatio = ratio
					best = node
				}
			}
		}
		return c.JSON(fiber.Map{"node": best})
	})

	protected.Get("/nodes/:id/allocations", requireAdminScope("allocations.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		allocations, err := cfg.Store.ListAllocationsForNode(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(allocations)
	})

	protected.Get("/nodes/:id/servers", requireAdminScope("servers.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		servers, err := cfg.Store.ListServersForNode(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(servers)
	})

	protected.Get("/nodes/:id/health", requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		node, err := nodeRegistry.GetNode(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "node not found")
		}
		return c.JSON(nodeRegistry.Health(node))
	})

	protected.Get("/nodes/:id/lifecycle", requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		view, err := nodeRegistry.LifecycleView(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "node not found")
		}
		return c.JSON(view)
	})

	protected.Get("/nodes/:id/evacuation-preview", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plan, err := evacuationPlanner.PreviewPlan(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(plan)
	})

	protected.Post("/nodes/:id/evacuation-plan", adminIPAccess, mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plan, err := evacuationPlanner.CreatePlan(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(plan)
	})

	protected.Get("/nodes/:id/capacity", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		node, err := nodeRegistry.GetNode(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "node not found")
		}
		snapshot, err := clusterManager.NodeCapacity(ctx, node.ID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(snapshot)
	})
}
