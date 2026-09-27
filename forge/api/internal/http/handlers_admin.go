package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/services/evacuationplanner"
	migrationservice "gamepanel/forge/internal/services/migration"
	"gamepanel/forge/internal/services/noderegistry"
	recoverysvc "gamepanel/forge/internal/services/recovery"
	reservationsvc "gamepanel/forge/internal/services/reservations"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

type importedVariable struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	EnvVariable  string `json:"envVariable"`
	DefaultValue string `json:"defaultValue"`
	UserViewable bool   `json:"userViewable"`
	UserEditable bool   `json:"userEditable"`
	Rules        string `json:"rules"`
	Sort         int    `json:"sort"`
}

func registerAdminRoutes(protected fiber.Router, cfg Config, nodeRegistry *noderegistry.Service, clusterManager *clustermanager.Service, evacuationPlanner *evacuationplanner.Service, migrationService *migrationservice.Service, reservationManager *reservationsvc.Manager, recoveryCoordinator *recoverysvc.Coordinator, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {
	// Recovery deliberately uses daemon backup restore rather than the live
	// migration executor: a recovery source is already classified offline.
	if recoveryCoordinator != nil && cfg.Store != nil && cfg.Daemon != nil {
		recoveryCoordinator.SetBackupRestoreExecutor(recoverysvc.NewDaemonBackupRestoreExecutor(cfg.Store, cfg.Daemon, clusterManager))
	}
	// ---- Permissions ----

	protected.Get("/permissions", requireRole("admin"), requireAdminScope("admin.read"), func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"permissions": store.PermissionDescriptions(),
		})
	})

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
		nodes, total, err := cfg.Store.ListNodesPaginated(ctx, offset, perPage)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		totalPages := (total + perPage - 1) / perPage
		if totalPages == 0 {
			totalPages = 1
		}
		return c.JSON(fiber.Map{
			"data": nodes,
			"meta": fiber.Map{
				"pagination": fiber.Map{
					"current":       page,
					"total":         totalPages,
					"count":         len(nodes),
					"per_page":      perPage,
					"total_records": total,
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
		schedulerType := strings.TrimSpace(req.SchedulerType)
		if schedulerType == "" {
			schedulerType = "docker"
		}
		storeReq := store.CreateNodeRequest{
			Name:                req.Name,
			Region:              req.Region,
			RegionID:            req.RegionID,
			Description:         req.Description,
			LocationID:          req.LocationID,
			BaseURL:             req.BaseURL,
			FQDN:                req.FQDN,
			Scheme:              req.Scheme,
			BehindProxy:         req.BehindProxy,
			Public:              req.Public == nil || *req.Public,
			Maintenance:         req.MaintenanceMode != nil && *req.MaintenanceMode,
			MemoryMB:            req.MemoryMB,
			DiskMB:              req.DiskMB,
			UploadSizeMB:        req.UploadSizeMB,
			DaemonBase:          req.DaemonBase,
			DaemonListen:        req.DaemonListen,
			DaemonSFTP:          req.DaemonSFTP,
			MemoryOverallocate:  derefInt(req.MemoryOverallocate),
			DiskOverallocate:    derefInt(req.DiskOverallocate),
			CPUCores:            derefInt(req.CPUCores),
			DisplayName:         req.DisplayName,
			PublicHostname:      req.PublicHostname,
			DaemonSFTPAlias:     req.DaemonSFTPAlias,
			DaemonConnect:       derefInt(req.DaemonConnect),
			CPUOverallocate:     derefInt(req.CPUOverallocate),
			Tags:                req.Tags,
			SchedulerType:       schedulerType,
			SchedulerConfig:     req.SchedulerConfig,
			AllowedIPs:          req.AllowedIPs,
			NetworkInterface:    req.NetworkInterface,
			ReservedMemoryMB:    derefInt(req.ReservedMemoryMB),
			ReservedDiskMB:      derefInt(req.ReservedDiskMB),
			DefaultAllocationIP: req.DefaultAllocationIP,
			AllocationPortMin:   derefInt(req.AllocationPortMin),
			AllocationPortMax:   derefInt(req.AllocationPortMax),
			AutoAllocate:        req.AutoAllocate != nil && *req.AutoAllocate,
			EnableHealthChecks:  req.EnableHealthChecks == nil || *req.EnableHealthChecks,
			EnableMetrics:       req.EnableMetrics == nil || *req.EnableMetrics,
			PrometheusEndpoint:  req.PrometheusEndpoint,
			AlertThresholdCPU:   derefInt(req.AlertThresholdCPU),
			AlertThresholdMem:   derefInt(req.AlertThresholdMem),
			AlertThresholdDisk:  derefInt(req.AlertThresholdDisk),
			MaintenanceMessage:  req.MaintenanceMessage,
			DrainBeforeMaint:    req.DrainBeforeMaint != nil && *req.DrainBeforeMaint,
			TokenRotationPolicy: req.TokenRotationPolicy,
			TLSSetting:          req.TLSSetting,
		}
		node, token, err := nodeRegistry.RegisterNode(ctx, storeReq, actorID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return fiber.NewError(fiber.StatusNotFound, err.Error())
			}
			if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "duplicate") {
				return fiber.NewError(fiber.StatusConflict, err.Error())
			}
			return respondInternalError(c, err)
		}
		_ = cfg.Store.DispatchWebhookEvent(c.Context(), "node:created", map[string]any{
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
			SchedulerType:      req.SchedulerType,
			SchedulerConfig:    req.SchedulerConfig,
		}, actorID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return fiber.NewError(fiber.StatusNotFound, err.Error())
			}
			if strings.Contains(err.Error(), "conflict") || strings.Contains(err.Error(), "invalid transition") {
				return fiber.NewError(fiber.StatusConflict, err.Error())
			}
			return respondInternalError(c, err)
		}
		_ = cfg.Store.DispatchWebhookEvent(c.Context(), "node:updated", map[string]any{
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
		_ = cfg.Store.DispatchWebhookEvent(c.Context(), "node:deleted", map[string]any{
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
			return respondInternalError(c, err)
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
		// Safe DTOs: never leak transferRunToken or secrets.
		return c.JSON(store.ServersToDTO(servers))
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

	protected.Get("/nodes/:id/capacity", requireRole("admin"), requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
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
			return respondInternalError(c, err)
		}
		return c.JSON(snapshot)
	})

	// ---- Regions CRUD (multi-node foundation) ----

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
			return respondInternalError(c, err)
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

	protected.Get("/evacuation-plans/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plan, err := evacuationPlanner.GetPlan(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "evacuation plan not found")
		}
		return c.JSON(plan)
	})

	protected.Post("/admin/migrations", adminIPAccess, mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req migrationservice.CreateMigrationRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := Validate(&req); err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(err)
		}
		ctx, cancel := requestContext()
		defer cancel()
		migration, err := migrationService.CreateMigration(ctx, req)
		if err != nil {
			return toMigrationError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(migration)
	})

	protected.Post("/migrations", adminIPAccess, mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req migrationservice.CreateMigrationRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := Validate(&req); err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(err)
		}
		ctx, cancel := requestContext()
		defer cancel()
		migration, err := migrationService.CreateMigration(ctx, req)
		if err != nil {
			return toMigrationError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(migration)
	})

	protected.Get("/migrations", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		migrations, err := migrationService.ListMigrations(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(migrations)
	})

	protected.Get("/admin/migrations", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		migrations, err := migrationService.ListMigrations(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(migrations)
	})

	protected.Get("/migrations/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		migration, err := migrationService.GetMigration(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "migration not found")
		}
		return c.JSON(migration)
	})

	protected.Patch("/migrations/:id/cancel", adminIPAccess, mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		migration, err := migrationService.CancelMigration(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(migration)
	})

	protected.Post("/admin/migrations/:id/cancel", adminIPAccess, mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		migration, err := migrationService.CancelMigration(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(migration)
	})

	protected.Get("/reservations", requireRole("admin"), requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		reservations, err := reservationManager.ListReservations(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(reservations)
	})

	protected.Get("/reservations/:id", requireRole("admin"), requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		reservation, err := reservationManager.GetReservation(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "reservation not found")
		}
		return c.JSON(reservation)
	})

	protected.Post("/reservations", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("nodes.write"), func(c *fiber.Ctx) error {
		if reservationManager == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "reservation manager service is not available")
		}
		var req store.CreatePlacementReservationRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.NodeID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "nodeId is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		reservation, err := reservationManager.CreateReservation(ctx, req)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(reservation)
	})

	protected.Post("/recovery-plans", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("nodes.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req recoverysvc.CreatePlanRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plan, err := recoveryCoordinator.CreatePlan(ctx, req)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(plan)
	})

	protected.Get("/recovery-plans", requireRole("admin"), requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plans, err := recoveryCoordinator.ListPlans(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(plans)
	})

	protected.Get("/recovery-plans/:id", requireRole("admin"), requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plan, err := recoveryCoordinator.GetPlan(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "recovery plan not found")
		}
		return c.JSON(plan)
	})

	// ---- Locations CRUD ----

	// ---- Nests CRUD ----

	// ---- Eggs CRUD ----

	protected.Get("/eggs/:id/variables", requireAdminScope("nests.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		variables, err := cfg.Store.ListEggVariables(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(variables)
	})

	protected.Post("/eggs/:id/variables", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("nests.write"), func(c *fiber.Ctx) error {
		var req EggVariableRequest
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
		variable, err := cfg.Store.CreateEggVariable(ctx, c.Params("id"), store.EggVariableRequest{
			Name: req.Name, Description: req.Description, EnvVariable: req.EnvVariable,
			DefaultValue: req.DefaultValue, UserViewable: req.UserViewable,
			UserEditable: req.UserEditable, Rules: req.Rules, Sort: req.Sort,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(variable)
	})

	protected.Patch("/eggs/:id/variables/:variableId", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("nests.write"), func(c *fiber.Ctx) error {
		var req EggVariableRequest
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
		variable, err := cfg.Store.UpdateEggVariable(ctx, c.Params("id"), c.Params("variableId"), store.EggVariableRequest{
			Name: req.Name, Description: req.Description, EnvVariable: req.EnvVariable,
			DefaultValue: req.DefaultValue, UserViewable: req.UserViewable,
			UserEditable: req.UserEditable, Rules: req.Rules, Sort: req.Sort,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(variable)
	})

	protected.Delete("/eggs/:id/variables/:variableId", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("nests.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.DeleteEggVariable(ctx, c.Params("id"), c.Params("variableId"), actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// ---- Egg Export/Import ----

	protected.Get("/eggs/:id/export", requireAdminScope("nests.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		egg, err := cfg.Store.GetEgg(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "egg not found")
		}
		variables, err := cfg.Store.ListEggVariables(ctx, c.Params("id"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{
			"egg":       egg,
			"variables": variables,
		})
	})

	protected.Post("/eggs/import", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("nests.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			NestID            string             `json:"nestId"`
			Name              string             `json:"name"`
			Description       string             `json:"description"`
			DockerImages      json.RawMessage    `json:"dockerImages"`
			Startup           string             `json:"startup"`
			Config            json.RawMessage    `json:"config"`
			DefaultMemoryMB   int                `json:"defaultMemoryMb"`
			InstallScript     string             `json:"installScript"`
			InstallContainer  string             `json:"installContainer"`
			InstallEntrypoint string             `json:"installEntrypoint"`
			FileDenylist      json.RawMessage    `json:"fileDenylist"`
			ConfigFrom        *string            `json:"configFrom,omitempty"`
			CopyScriptFrom    *string            `json:"copyScriptFrom,omitempty"`
			UpdateURL         string             `json:"updateUrl"`
			Author            string             `json:"author"`
			Features          json.RawMessage    `json:"features"`
			StartupCommands   json.RawMessage    `json:"startupCommands"`
			Variables         []importedVariable `json:"variables"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid import payload")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		egg, err := cfg.Store.CreateEgg(ctx, store.CreateEggRequest{
			NestID: req.NestID, Name: req.Name, Description: req.Description,
			DockerImages: req.DockerImages, Startup: req.Startup, Config: req.Config,
			DefaultMemoryMB: req.DefaultMemoryMB, InstallScript: req.InstallScript,
			InstallContainer: req.InstallContainer, InstallEntrypoint: req.InstallEntrypoint,
			FileDenylist: req.FileDenylist, ConfigFrom: req.ConfigFrom,
			CopyScriptFrom: req.CopyScriptFrom, UpdateURL: req.UpdateURL, Author: req.Author,
			Features: req.Features, StartupCommands: req.StartupCommands,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		for _, v := range req.Variables {
			if _, err := cfg.Store.CreateEggVariable(ctx, egg.ID, store.EggVariableRequest{
				Name: v.Name, Description: v.Description, EnvVariable: v.EnvVariable,
				DefaultValue: v.DefaultValue, UserViewable: v.UserViewable,
				UserEditable: v.UserEditable, Rules: v.Rules, Sort: v.Sort,
			}, actorID); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("failed to import variable %s: %v", v.EnvVariable, err))
			}
		}
		return c.Status(fiber.StatusCreated).JSON(egg)
	})

	protected.Get("/allocations/nodes", requireAdminScope("allocations.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		nodes, err := cfg.Store.ListAllocationNodes(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(nodes)
	})

	protected.Get("/allocations", requireAdminScope("allocations.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		allocations, err := cfg.Store.ListAllocations(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(allocations)
	})

	protected.Get("/database-hosts", requireRole("admin"), requireAdminScope("databases.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		hosts, err := cfg.Store.ListDatabaseHosts(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(hosts)
	})

	protected.Get("/database-hosts/:id", requireRole("admin"), requireAdminScope("databases.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		host, err := cfg.Store.GetDatabaseHost(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "database host not found")
		}
		return c.JSON(host)
	})

	// Test a prospective host before it is saved. This uses the same normalization,
	// validation, TLS configuration, and ping flow as provisioning without persistence.
	protected.Post("/database-hosts/test", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		var req CreateDatabaseHostRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if cfg.DBProvisioner == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "database provisioner is unavailable")
		}
		host, password, err := store.DatabaseHostForConnectionTest(store.CreateDatabaseHostRequest{
			NodeID:        req.NodeID,
			NodeIDs:       req.NodeIDs,
			Engine:        req.Engine,
			Name:          req.Name,
			Host:          req.Host,
			Port:          req.Port,
			Username:      req.Username,
			Password:      req.Password,
			TLSMode:       req.TLSMode,
			TLSCA:         req.TLSCA,
			TLSServerName: req.TLSServerName,
			MaxDatabases:  req.MaxDatabases,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.DBProvisioner.TestConnection(ctx, host, password); err != nil {
			// This is an administrator-only diagnostic for an external provisioning host.
			// Preserve the connector's sanitized cause (timeout, TLS, authentication, etc.)
			// instead of collapsing every failure into an indistinguishable message.
			return fiber.NewError(fiber.StatusBadGateway, fmt.Sprintf("database host connection test failed: %v", err))
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/database-hosts/:id/test", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		if cfg.DBProvisioner == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "database provisioner is unavailable")
		}
		ctx, cancel := requestContext()
		defer cancel()
		hostID := c.Params("id")
		host, password, err := cfg.Store.GetDatabaseHostForTest(ctx, hostID)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "database host not found")
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.DBProvisioner.TestConnection(ctx, host, password); err != nil {
			_ = cfg.Store.AppendAudit(ctx, actorID, "database host connection test failed", "database_host", &hostID, safeAuditMeta(map[string]string{"result": "failed"}))
			return fiber.NewError(fiber.StatusBadGateway, fmt.Sprintf("database host connection test failed: %v", err))
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "database host connection tested", "database_host", &hostID, safeAuditMeta(map[string]string{"result": "success"}))
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/database-hosts", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		var req CreateDatabaseHostRequest
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
		host, err := cfg.Store.CreateDatabaseHost(ctx, store.CreateDatabaseHostRequest{
			NodeID:        req.NodeID,
			NodeIDs:       req.NodeIDs,
			Engine:        req.Engine,
			Name:          req.Name,
			Host:          req.Host,
			Port:          req.Port,
			Username:      req.Username,
			Password:      req.Password,
			TLSMode:       req.TLSMode,
			TLSCA:         req.TLSCA,
			TLSServerName: req.TLSServerName,
			MaxDatabases:  req.MaxDatabases,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(host)
	})

	protected.Patch("/database-hosts/:id", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("databases.write"), func(c *fiber.Ctx) error {
		var req UpdateDatabaseHostRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		// TLS CA values are write-only. Treat a blank value as omitted so clients can
		// safely round-trip an edit without overwriting a redacted certificate.
		if req.TLSCA != nil && strings.TrimSpace(*req.TLSCA) == "" {
			req.TLSCA = nil
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
		host, err := cfg.Store.UpdateDatabaseHost(ctx, c.Params("id"), store.UpdateDatabaseHostRequest{
			NodeID:        req.NodeID,
			NodeIDs:       req.NodeIDs,
			Engine:        req.Engine,
			Name:          req.Name,
			Host:          req.Host,
			Port:          req.Port,
			Username:      req.Username,
			Password:      req.Password,
			TLSMode:       req.TLSMode,
			TLSCA:         req.TLSCA,
			TLSServerName: req.TLSServerName,
			MaxDatabases:  req.MaxDatabases,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(host)
	})

	protected.Delete("/database-hosts/:id", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("databases.delete"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.DeleteDatabaseHost(ctx, c.Params("id"), actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Get("/mounts", requireRole("admin"), requireAdminScope("mounts.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		mounts, err := cfg.Store.ListMounts(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(mounts)
	})

	protected.Get("/mounts/:id", requireRole("admin"), requireAdminScope("mounts.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		mount, err := cfg.Store.GetMount(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "mount not found")
		}
		return c.JSON(mount)
	})

	protected.Post("/mounts", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("mounts.write"), func(c *fiber.Ctx) error {
		var req CreateMountRequest
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
		mount, err := cfg.Store.CreateMount(ctx, store.CreateMountRequest{
			Name:          req.Name,
			Description:   req.Description,
			Source:        req.Source,
			Target:        req.Target,
			ReadOnly:      req.ReadOnly,
			UserMountable: req.UserMountable,
			NodeIDs:       req.NodeIDs,
			TemplateIDs:   req.TemplateIDs,
		}, actorID)
		if err != nil {
			msg := err.Error()
			// Surface the allowlist knob so admins can fix rejected sources.
			if strings.Contains(strings.ToLower(msg), "mount") && !strings.Contains(msg, "MOUNTS_ALLOWED_PREFIX") {
				msg = fmt.Sprintf("%s (allowed host prefix: set MOUNTS_ALLOWED_PREFIX on the panel to permit mount sources)", msg)
			}
			return fiber.NewError(fiber.StatusBadRequest, msg)
		}
		return c.Status(fiber.StatusCreated).JSON(mount)
	})

	protected.Delete("/mounts/:id", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("mounts.delete"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		mountID := c.Params("id")
		mount, err := cfg.Store.GetMount(ctx, mountID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		serverCount, err := cfg.Store.CountServersUsingMount(ctx, mountID)
		if err != nil {
			return respondInternalError(c, err)
		}
		if serverCount > 0 {
			return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("cannot delete mount: %d server(s) are still attached; detach them first", serverCount))
		}
		nodes, err := cfg.Store.ListNodesForMount(ctx, mountID)
		if err != nil {
			return respondInternalError(c, err)
		}
		if err := cfg.Store.DeleteMount(ctx, mountID, actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if cfg.Daemon != nil {
			for _, node := range nodes {
				nodeToken, tokenErr := cfg.Store.GetNodeDaemonToken(ctx, node.ID)
				if tokenErr != nil {
					slog.Warn("mount cleanup failed to get node token", "node", node.Name, "error", tokenErr)
					continue
				}
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
				resp, cleanupErr := cfg.Daemon.CleanupMount(cleanupCtx, node.BaseURL, nodeToken, mount.Source)
				cleanupCancel()
				if cleanupErr != nil {
					slog.Warn("mount cleanup failed for node", "node", node.Name, "source", mount.Source, "error", cleanupErr)
				} else if resp.OK && !resp.Removed {
					slog.Info("mount cleanup skipped", "node", node.Name, "source", mount.Source, "reason", resp.Reason)
				}
			}
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Patch("/mounts/:id", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("mounts.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var req struct {
			Name          *string `json:"name"`
			Description   *string `json:"description"`
			Source        *string `json:"source"`
			Target        *string `json:"target"`
			ReadOnly      *bool   `json:"readOnly"`
			UserMountable *bool   `json:"userMountable"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		mount, err := cfg.Store.UpdateMount(ctx, c.Params("id"), store.UpdateMountRequest{
			Name:          req.Name,
			Description:   req.Description,
			Source:        req.Source,
			Target:        req.Target,
			ReadOnly:      req.ReadOnly,
			UserMountable: req.UserMountable,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(mount)
	})

	protected.Get("/mounts/:id/eggs", requireRole("admin"), requireAdminScope("mounts.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		eggs, err := cfg.Store.ListEggsForMount(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(eggs)
	})

	protected.Get("/mounts/:id/nodes", requireRole("admin"), requireAdminScope("mounts.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		nodes, err := cfg.Store.ListNodesForMount(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(nodes)
	})

	protected.Post("/mounts/:id/eggs", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("mounts.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var req struct {
			EggIDs []string `json:"eggs"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		for _, eggID := range req.EggIDs {
			if err := cfg.Store.AttachEggToMount(ctx, c.Params("id"), eggID); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, err.Error())
			}
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Delete("/mounts/:id/eggs/:eggId", adminIPAccess, requireRole("admin"), requireAdminScope("mounts.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DetachEggFromMount(ctx, c.Params("id"), c.Params("eggId")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/mounts/:id/nodes", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("mounts.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var req struct {
			NodeIDs []string `json:"nodes"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		for _, nodeID := range req.NodeIDs {
			if err := cfg.Store.AttachNodeToMount(ctx, c.Params("id"), nodeID); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, err.Error())
			}
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Delete("/mounts/:id/nodes/:nodeId", adminIPAccess, requireRole("admin"), requireAdminScope("mounts.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DetachNodeFromMount(ctx, c.Params("id"), c.Params("nodeId")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// Server <-> Mount attachment. The mount must be eligible for the server's
	// node and egg, just as it is for the server-scoped assignment route.
	protected.Post("/mounts/:id/servers", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("mounts.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			ServerID string `json:"serverId"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.ServerID) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "serverId required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.AssignMountToServer(ctx, req.ServerID, c.Params("id"), actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if clusterManager == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "mount assignment persisted but runtime synchronization is pending")
		}
		if err := clusterManager.SyncServerConfiguration(ctx, req.ServerID); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "mount assignment persisted but runtime synchronization is pending: "+err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "runtimeSynchronized": true})
	})

	protected.Get("/mounts/:id/servers", requireRole("admin"), requireAdminScope("mounts.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		mounts, err := cfg.Store.ServerMountsForMount(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(mounts)
	})

	protected.Delete("/mounts/:id/servers/:serverId", adminIPAccess, requireRole("admin"), requireAdminScope("mounts.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DetachServerFromMount(ctx, c.Params("id"), c.Params("serverId")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if clusterManager == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "mount removal persisted but runtime synchronization is pending")
		}
		if err := clusterManager.SyncServerConfiguration(ctx, c.Params("serverId")); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "mount removal persisted but runtime synchronization is pending: "+err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "runtimeSynchronized": true})
	})

	protected.Post("/allocations", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("allocations.write"), func(c *fiber.Ctx) error {
		var req CreateAllocationRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := Validate(&req); err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(err)
		}
		ports := []int{}
		if req.Port > 0 {
			ports = append(ports, req.Port)
		}
		if strings.TrimSpace(req.Ports) != "" {
			parsed, err := parsePortRanges(req.Ports)
			if err != nil {
				return fiber.NewError(fiber.StatusBadRequest, err.Error())
			}
			ports = append(ports, parsed...)
		}
		if len(ports) == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "port or ports is required")
		}
		ports = uniquePorts(ports)
		if len(ports) > 2000 {
			return fiber.NewError(fiber.StatusBadRequest, "too many ports in one request")
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
		requests := make([]store.CreateAllocationRequest, 0, len(ports))
		protocol := strings.ToLower(strings.TrimSpace(req.Protocol))
		if protocol == "" {
			protocol = "tcp"
		}
		if protocol != "tcp" && protocol != "udp" {
			return fiber.NewError(fiber.StatusBadRequest, "protocol must be tcp or udp")
		}
		if req.ContainerPort != 0 && len(ports) != 1 {
			return fiber.NewError(fiber.StatusBadRequest, "containerPort can only be set for a single port")
		}
		for _, port := range ports {
			containerPort := req.ContainerPort
			if containerPort == 0 {
				containerPort = port
			}
			requests = append(requests, store.CreateAllocationRequest{
				NodeID:        req.NodeID,
				IP:            req.IP,
				Port:          port,
				ContainerPort: containerPort,
				Protocol:      protocol,
				Alias:         req.Alias,
				Notes:         req.Notes,
			})
		}
		created, err := cfg.Store.CreateAllocations(ctx, requests, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(created)
	})

	protected.Patch("/allocations/:id", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("allocations.write"), func(c *fiber.Ctx) error {
		var req UpdateAllocationRequest
		if err := c.BodyParser(&req); err != nil && err != io.EOF {
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
		allocation, err := cfg.Store.UpdateAllocation(ctx, c.Params("id"), store.UpdateAllocationRequest{
			Alias: req.Alias,
			Notes: req.Notes,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(allocation)
	})

	// Register the static path before /allocations/:id so it is not interpreted as id="bulk".
	protected.Delete("/allocations/bulk", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("allocations.delete"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			IDs []string `json:"ids"`
		}
		if err := c.BodyParser(&req); err != nil || len(req.IDs) == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "ids are required")
		}
		ids := make([]string, 0, len(req.IDs))
		seen := make(map[string]struct{}, len(req.IDs))
		for _, id := range req.IDs {
			id = strings.TrimSpace(id)
			if id == "" {
				return fiber.NewError(fiber.StatusBadRequest, "allocation ids must not be empty")
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
		if len(ids) > 2000 {
			return fiber.NewError(fiber.StatusBadRequest, "at most 2,000 allocation ids are allowed")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.DeleteAllocations(ctx, ids, actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Delete("/allocations/:id", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("allocations.delete"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.DeleteAllocation(ctx, c.Params("id"), actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/allocations/:id/alias", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("allocations.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Alias string `json:"alias"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.UpdateAllocationAlias(ctx, c.Params("id"), req.Alias); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Patch("/templates/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		var req CreateTemplateRequest
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
		template, err := cfg.Store.UpdateTemplate(ctx, c.Params("id"), store.CreateTemplateRequest{
			Name: req.Name, Image: req.Image, StartupCommand: req.StartupCommand,
			DefaultMemoryMB: req.DefaultMemoryMB,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(template)
	})

	// ---- Plugins ----
	protected.Get("/admin/plugins", requireRole("admin"), ListPlugins(cfg))
	protected.Get("/admin/plugins/:id", requireRole("admin"), GetPlugin(cfg))
	protected.Post("/admin/plugins/import/file", requireRole("admin"), ImportPluginFromFile(cfg))
	protected.Post("/admin/plugins/import/url", requireRole("admin"), ImportPluginFromURL(cfg))
	protected.Post("/admin/plugins/install", requireRole("admin"), InstallPlugin(cfg))
	protected.Post("/admin/plugins/:id/uninstall", requireRole("admin"), UninstallPlugin(cfg))
	protected.Post("/admin/plugins/:id/enable", requireRole("admin"), EnablePlugin(cfg))
	protected.Post("/admin/plugins/:id/disable", requireRole("admin"), DisablePlugin(cfg))
	protected.Patch("/admin/plugins/:id", requireRole("admin"), UpdatePlugin(cfg))
	protected.Delete("/admin/plugins/:id", requireRole("admin"), DeletePlugin(cfg))

	// ---- Roles ----
	protected.Get("/admin/roles", requireRole("admin"), ListRoles(cfg))
	protected.Get("/admin/roles/:id", requireRole("admin"), GetRole(cfg))
	protected.Post("/admin/roles", requireRole("admin"), CreateRole(cfg))
	protected.Patch("/admin/roles/:id", requireRole("admin"), UpdateRole(cfg))
	protected.Delete("/admin/roles/:id", requireRole("admin"), DeleteRole(cfg))
	protected.Get("/admin/users/:id/roles", requireRole("admin"), ListUserRoles(cfg))
	protected.Patch("/admin/users/:id/roles/assign", requireRole("admin"), AssignRolesToUser(cfg))
	protected.Patch("/admin/users/:id/roles/remove", requireRole("admin"), RemoveRolesFromUser(cfg))

	// ---- OAuth2 admin routes ----
	protected.Get("/admin/oauth-clients", requireRole("admin"), AdminListOAuthClients(cfg))
	protected.Post("/admin/oauth-clients", requireRole("admin"), AdminCreateOAuthClient(cfg))
	protected.Delete("/admin/oauth-clients/:id", requireRole("admin"), AdminDeleteOAuthClient(cfg))

	// ---- Webhooks ----

	protected.Get("/webhooks", requireRole("admin"), requireAdminScope("webhooks.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		webhooks, err := cfg.Store.ListWebhooks(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		for i := range webhooks {
			if webhooks[i].Secret != "" {
				webhooks[i].Secret = maskedSecret
			}
		}
		return c.JSON(webhooks)
	})

	protected.Post("/webhooks", requireRole("admin"), requireAdminScope("webhooks.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req store.CreateWebhookRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		wh, err := cfg.Store.CreateWebhook(ctx, req)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if wh.Secret != "" {
			wh.Secret = maskedSecret
		}
		return c.Status(fiber.StatusCreated).JSON(wh)
	})

	protected.Patch("/webhooks/:id", requireRole("admin"), requireAdminScope("webhooks.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req store.CreateWebhookRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if req.Secret == maskedSecret {
			req.Secret = ""
		}
		wh, err := cfg.Store.UpdateWebhook(ctx, c.Params("id"), req)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if wh.Secret != "" {
			wh.Secret = maskedSecret
		}
		return c.JSON(wh)
	})

	// GET /webhooks/:id — fetch a single webhook definition.
	protected.Get("/webhooks/:id", requireRole("admin"), requireAdminScope("webhooks.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		wh, err := cfg.Store.GetWebhook(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "webhook not found")
		}
		if wh.Secret != "" {
			wh.Secret = maskedSecret
		}
		return c.JSON(wh)
	})

	protected.Get("/webhooks/:id/deliveries", requireRole("admin"), requireAdminScope("webhooks.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		offset := 0
		if o := c.Query("offset"); o != "" {
			if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
				offset = parsed
			}
		}
		ctx, cancel := requestContext()
		defer cancel()
		deliveries, err := cfg.Store.ListWebhookDeliveries(ctx, c.Params("id"), queryLimit(c), offset)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(deliveries)
	})

	// GET /webhooks/:id/dead-letter — deliveries that exhausted all retries.
	protected.Get("/webhooks/:id/dead-letter", requireRole("admin"), requireAdminScope("webhooks.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		offset := 0
		if o := c.Query("offset"); o != "" {
			if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
				offset = parsed
			}
		}
		ctx, cancel := requestContext()
		defer cancel()
		deliveries, err := cfg.Store.ListDeadLetterDeliveries(ctx, c.Params("id"), queryLimit(c), offset)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(deliveries)
	})

	protected.Post("/webhooks/:id/deliveries/:deliveryId/retry", requireRole("admin"), requireAdminScope("webhooks.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.RetryWebhookDelivery(ctx, c.Params("deliveryId")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// POST /webhooks/:id/test — fire a test delivery so the admin can verify the
	// endpoint receives and ACKs events before enabling production traffic.
	protected.Post("/webhooks/:id/test", requireRole("admin"), requireAdminScope("webhooks.write"), mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		wh, err := cfg.Store.GetWebhookWithSecret(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "webhook not found")
		}
		payload := []byte(`{"event":"test","message":"Forge webhook test delivery"}`)
		delivery, err := cfg.Store.CreateTestDelivery(ctx, wh, "test", payload, payload)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.Status(201).JSON(delivery)
	})

	protected.Delete("/webhooks/deliveries/:deliveryId", requireRole("admin"), requireAdminScope("webhooks.delete"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteWebhookDelivery(ctx, c.Params("deliveryId")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Delete("/webhooks/:id", requireRole("admin"), requireAdminScope("webhooks.delete"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteWebhook(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// ---- Reconciler Orchestration ----

	protected.Get("/reconciler/metrics", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Reconciler == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "reconciler service is not available")
		}
		return c.JSON(cfg.Reconciler.Metrics())
	})

	protected.Post("/reconciler/run", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Reconciler == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "reconciler service is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Reconciler.RunOnce(ctx); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "reconciliation failed: "+err.Error())
		}
		return c.JSON(fiber.Map{"status": "success"})
	})

	// ---- Migration lifecycle ----

	protected.Get("/migrations/executor", requireRole("admin"), func(c *fiber.Ctx) error {
		available := migrationService != nil && migrationService.ExecutorAvailable()
		return c.JSON(fiber.Map{"available": available})
	})

	protected.Post("/migrations/:id/prepare", requireRole("admin"), prepareMigrationRoute(migrationService))
	protected.Post("/migrations/:id/execute", requireRole("admin"), executeMigrationRoute(migrationService))

	protected.Post("/migrations/:id/cancel", requireRole("admin"), func(c *fiber.Ctx) error {
		if migrationService == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "migration service is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		m, err := migrationService.CancelMigration(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(m)
	})

	// ---- Evacuation Orchestration ----

	protected.Get("/evacuations/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		if evacuationPlanner == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "evacuation planner service is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plan, err := evacuationPlanner.GetPlan(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "evacuation plan not found")
		}
		return c.JSON(plan)
	})

	protected.Post("/evacuations", mutationLimiter, requireRole("admin"), requireAdminScope("nodes.write"), executeEvacuationRoute(evacuationPlanner, migrationService))
	protected.Post("/evacuations/:id/cancel", mutationLimiter, requireRole("admin"), requireAdminScope("nodes.write"), func(c *fiber.Ctx) error {
		if evacuationPlanner == nil || migrationService == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "evacuation execution service is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plan, err := evacuationPlanner.CancelPlan(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(plan)
	})

	protected.Post("/evacuations/preview", requireRole("admin"), func(c *fiber.Ctx) error {
		if evacuationPlanner == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "evacuation planner service is not available")
		}
		var req struct {
			NodeID string `json:"nodeId"`
		}
		if err := c.BodyParser(&req); err != nil || req.NodeID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "nodeId is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		preview, err := evacuationPlanner.PreviewPlan(ctx, req.NodeID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(preview)
	})

	// ---- Reservation lifecycle ----

	protected.Post("/reservations/:id/cancel", requireRole("admin"), requireAdminScope("nodes.write"), func(c *fiber.Ctx) error {
		if reservationManager == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "reservation manager service is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		res, err := reservationManager.CancelReservation(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(res)
	})

	protected.Post("/reservations/:id/confirm", mutationLimiter, requireRole("admin"), requireAdminScope("nodes.write"), func(c *fiber.Ctx) error {
		if reservationManager == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "reservation manager service is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		res, err := reservationManager.ConfirmReservation(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(res)
	})

	// ---- Recovery Orchestration ----

	protected.Get("/recovery", requireRole("admin"), requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		if recoveryCoordinator == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "recovery coordinator service is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plans, err := recoveryCoordinator.ListPlans(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(plans)
	})

	protected.Get("/recovery/:id", requireRole("admin"), requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		if recoveryCoordinator == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "recovery coordinator service is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plan, err := recoveryCoordinator.GetPlan(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "recovery plan not found")
		}
		return c.JSON(plan)
	})

	// Start a saved recovery plan. This keeps the original route shape while
	// requiring an explicit plan ID so a plan is never executed implicitly.
	protected.Post("/recovery", requireRole("admin"), requireAdminScope("nodes.write"), executeRecoveryRoute(recoveryCoordinator))
	protected.Post("/recovery/:id/start", requireRole("admin"), requireAdminScope("nodes.write"), func(c *fiber.Ctx) error {
		if recoveryCoordinator == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "recovery coordinator service is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plan, err := recoveryCoordinator.ExecutePlan(ctx, c.Params("id"))
		if err != nil {
			return migrationRouteError(err)
		}
		return c.Status(fiber.StatusAccepted).JSON(plan)
	})

	protected.Post("/recovery/:id/cancel", requireRole("admin"), requireAdminScope("nodes.write"), func(c *fiber.Ctx) error {
		if recoveryCoordinator == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "recovery coordinator service is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plan, err := recoveryCoordinator.CancelPlan(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(plan)
	})
}

func executeRecoveryRoute(coordinator *recoverysvc.Coordinator) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if coordinator == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "recovery coordinator service is not available")
		}
		var req struct {
			PlanID string `json:"planId"`
		}
		if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.PlanID) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "planId is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plan, err := coordinator.ExecutePlan(ctx, strings.TrimSpace(req.PlanID))
		if err != nil {
			return migrationRouteError(err)
		}
		return c.Status(fiber.StatusAccepted).JSON(plan)
	}
}

func migrationRouteError(err error) error {
	var unavailable *migrationservice.ExecutorUnavailableError
	if errors.As(err, &unavailable) {
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
	}
	return fiber.NewError(fiber.StatusBadRequest, err.Error())
}

func executeEvacuationRoute(planner *evacuationplanner.Service, migrationService *migrationservice.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if planner == nil || migrationService == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "evacuation execution service is not available")
		}
		var req struct {
			PlanID string `json:"planId"`
		}
		if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.PlanID) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "planId is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plan, err := planner.ExecutePlan(ctx, strings.TrimSpace(req.PlanID))
		if err != nil {
			return migrationRouteError(err)
		}
		return c.Status(fiber.StatusAccepted).JSON(plan)
	}
}

func prepareMigrationRoute(service *migrationservice.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if service == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "migration service is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		migration, err := service.PrepareMigration(ctx, c.Params("id"))
		if err != nil {
			return migrationRouteError(err)
		}
		return c.JSON(migration)
	}
}

func executeMigrationRoute(service *migrationservice.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if service == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "migration service is not available")
		}
		ctx, cancel := requestContext()
		defer cancel()
		migration, err := service.ExecuteMigration(ctx, c.Params("id"))
		if err != nil {
			return migrationRouteError(err)
		}
		return c.JSON(migration)
	}
}

func toMigrationError(c *fiber.Ctx, err error) error {
	var targetErr *domain.TargetValidationError
	if errors.As(err, &targetErr) {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(targetErr)
	}
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"message": err.Error()})
}

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}
