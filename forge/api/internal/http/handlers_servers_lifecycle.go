package http

import (
	"context"
	"strconv"
	"strings"
	"time"

	"gamepanel/forge/internal/domain"
	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/services/queue"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerLifecycleServerRoutes(protected fiber.Router, cfg Config, runner *scheduleRunner, clusterManager *clustermanager.Service, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {

	protected.Get("/servers", requireAdminScope("servers.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}

		// Parse pagination parameters
		page := 1
		if p := c.Query("page"); p != "" {
			if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
				page = parsed
			}
		}

		perPage := 15
		if pp := c.Query("per_page"); pp != "" {
			if parsed, err := strconv.Atoi(pp); err == nil && parsed > 0 && parsed <= 100 {
				perPage = parsed
			}
		}

		search := c.Query("search", "")

		ctx, cancel := requestContext()
		defer cancel()
		servers, total, err := cfg.Store.ListServersForUser(ctx, claims.Sub, claims.Role, page, perPage, search)
		if err != nil {
			return err
		}

		totalPages := (total + perPage - 1) / perPage
		if totalPages == 0 {
			totalPages = 1
		}

		return c.JSON(fiber.Map{
			"data": servers,
			"meta": fiber.Map{
				"pagination": fiber.Map{
					"current":       page,
					"total":         totalPages,
					"count":         len(servers),
					"per_page":      perPage,
					"total_records": total,
				},
			},
		})
	})

	protected.Get("/servers/:id", requireServerAccess(cfg), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		server, err := cfg.Store.GetServer(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		if claims.Role == "admin" || claims.Sub == server.OwnerID {
			server.Permissions = []string{"*"}
		} else {
			subuser, err := cfg.Store.GetServerSubuser(ctx, server.ID, claims.Sub)
			if err != nil {
				return fiber.NewError(fiber.StatusForbidden, "server access is not assigned to this user")
			}
			server.Permissions = subuser.Permissions
		}
		return c.JSON(server)
	})

	protected.Patch("/servers/:id", mutationLimiter, func(c *fiber.Ctx) error {
		var req UpdateServerRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		detailsChanged := req.Name != nil || req.Description != nil
		startupChanged := req.StartupCommand != nil
		imageChanged := req.DockerImage != nil
		allocationChanged := req.PrimaryAlloc != nil
		adminChanged := req.OwnerID != nil || req.MemoryMB != nil || req.CPUShares != nil || req.CPULimit != nil || req.DiskMB != nil || req.DatabaseLimit != nil || req.BackupLimit != nil || req.AllocationLimit != nil || req.IOWeight != nil || req.SwapMB != nil || req.Threads != nil || req.OOMDisabled != nil
		if !detailsChanged && !startupChanged && !imageChanged && !allocationChanged && !adminChanged {
			return fiber.NewError(fiber.StatusBadRequest, "at least one supported field is required")
		}
		if detailsChanged {
			if err := checkServerPermission(c, cfg, store.PermSettingsRename); err != nil {
				return err
			}
		}
		if startupChanged {
			if err := checkServerPermission(c, cfg, store.PermStartupUpdate); err != nil {
				return err
			}
		}
		if imageChanged {
			if err := checkServerPermission(c, cfg, store.PermStartupDockerImage); err != nil {
				return err
			}
		}
		if allocationChanged {
			if err := checkServerPermission(c, cfg, store.PermAllocationUpdate); err != nil {
				return err
			}
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if adminChanged && (!ok || claims.Role != "admin") {
			return fiber.NewError(fiber.StatusForbidden, "admin role is required to update owner or build limits")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if ok {
			actorID = &claims.Sub
		}
		server, err := cfg.Store.UpdateServer(ctx, c.Params("id"), store.UpdateServerRequest{
			Name: req.Name, Description: req.Description, OwnerID: req.OwnerID,
			MemoryMB: req.MemoryMB, CPUShares: req.CPUShares, CPULimit: req.CPULimit, DiskMB: req.DiskMB,
			DatabaseLimit: req.DatabaseLimit, BackupLimit: req.BackupLimit, AllocationLimit: req.AllocationLimit,
			IOWeight: req.IOWeight, SwapMB: req.SwapMB, Threads: req.Threads, OOMDisabled: req.OOMDisabled,
			DockerImage: req.DockerImage, StartupCommand: req.StartupCommand, PrimaryAllocationID: req.PrimaryAlloc,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if server.ConfigSyncPending {
			if clusterManager == nil {
				return fiber.NewError(fiber.StatusServiceUnavailable, "runtime synchronization is unavailable; update is pending sync")
			}
			if err := clusterManager.SyncServerConfiguration(ctx, server.ID); err != nil {
				return fiber.NewError(fiber.StatusBadGateway, "update persisted but runtime synchronization is pending: "+err.Error())
			}
			server, err = cfg.Store.GetServer(ctx, server.ID)
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
		}
		return c.JSON(server)
	})

	// Dedicated description endpoint.
	protected.Post("/servers/:id/description", mutationLimiter, requireServerPermission(cfg, store.PermSettingsRename), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Description string `json:"description"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		server, err := cfg.Store.UpdateServer(ctx, c.Params("id"), store.UpdateServerRequest{
			Description: &req.Description,
		}, nil)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(server)
	})

	// Server reload: re-reads the server definition from disk on the daemon
	// (PufferPanel-style `server.reload`). Useful for picking up manually
	// edited config files without a full reinstall.
	protected.Post("/servers/:id/reload", mutationLimiter, requireServerPermission(cfg, store.PermSettingsReinstall), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerProvisionTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if err := cfg.Daemon.SyncServerConfiguration(ctx, target.NodeURL, target.NodeToken, target.ServerID, buildDaemonServerConfiguration(target.ToDTO())); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// Enforce user-level subuser cap (for the owner of the server).

	// Enforce user-level allocation cap.

	protected.Post("/servers", mutationLimiter, requireRole("admin"), requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		var req CreateServerRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := Validate(&req); err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(err)
		}
		if req.TemplateID == "" || (req.RegionID == "" && req.Region == "" && req.NodeID == "" && req.RequiredNode == "") {
			return fiber.NewError(fiber.StatusBadRequest, "templateId, and regionId or nodeId are required")
		}
		if req.OwnerID == "" {
			if claims, ok := c.Locals("user").(tokenClaims); ok {
				req.OwnerID = claims.Sub
			}
		}
		if cfg.Store == nil || clusterManager == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and runtime lifecycle service are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		defaultMemoryMB := 2048
		if req.MemoryMB == nil {
			egg, err := cfg.Store.GetEgg(ctx, req.TemplateID)
			if err != nil {
				return fiber.NewError(fiber.StatusBadRequest, "egg not found")
			}
			defaultMemoryMB = egg.DefaultMemoryMB
		}
		memoryMB := createResourceValue(req.MemoryMB, defaultMemoryMB)
		cpuShares := createResourceValue(req.CPUShares, 1024)
		cpuLimit := createResourceValue(req.CPU, 0)
		diskMB := createResourceValue(req.DiskMB, 10240)
		databaseLimit := createResourceValue(req.DatabaseLimit, 0)
		backupLimit := createResourceValue(req.BackupLimit, 0)
		allocationLimit := createResourceValue(req.AllocationLimit, 0)
		ioWeight := createResourceValue(req.IOWeight, 500)
		swapMB := createResourceValue(req.SwapMB, 0)
		if memoryMB <= 0 || cpuShares <= 0 || diskMB <= 0 || cpuLimit < 0 || databaseLimit < 0 || backupLimit < 0 || allocationLimit < 0 || swapMB < -1 || ioWeight < 10 || ioWeight > 1000 {
			return fiber.NewError(fiber.StatusBadRequest, "invalid server resource limits")
		}
		// Zero is an explicit unlimited value for CPU, database, backup, and
		// allocation limits. Required build resources receive defaults only when omitted.
		if err := cfg.Store.CheckUserCanCreateServer(ctx, req.OwnerID, memoryMB, diskMB, cpuLimit); err != nil {
			if store.IsUserLimitError(err) {
				return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
			}
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		server, _, err := clusterManager.CreateServer(ctx, store.CreateServerRequest{
			Name:                    req.Name,
			NodeID:                  req.NodeID,
			OwnerID:                 req.OwnerID,
			TemplateID:              req.TemplateID,
			AllocationID:            req.AllocationID,
			AdditionalAllocationIDs: req.AdditionalAllocationIDs,
			MemoryMB:                memoryMB,
			CPUShares:               cpuShares,
			CPULimit:                cpuLimit,
			DiskMB:                  diskMB,
			DatabaseLimit:           databaseLimit,
			BackupLimit:             backupLimit,
			AllocationLimit:         allocationLimit,
			IOWeight:                ioWeight,
			SwapMB:                  swapMB,
			Threads:                 req.Threads,
			OOMDisabled:             req.OOMDisabled,
			DockerImage:             req.DockerImage,
			StartupCommand:          req.StartupCommand,
			StartupVariables:        req.StartupVariables,
		}, domain.PlacementRequest{
			RegionID:      req.RegionID,
			Region:        req.Region,
			NodeID:        req.NodeID,
			PreferredNode: req.PreferredNode,
			RequiredNode:  req.RequiredNode,
			AllocationID:  req.AllocationID,
			MemoryMB:      memoryMB,
			CPUShares:     cpuShares,
			CPU:           cpuLimit,
			DiskMB:        diskMB,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		if cfg.Store != nil {
			cfg.Store.DispatchWebhookEvent("server:created", map[string]any{
				"subject_type": "server",
				"subject_id":   server.ID,
				"name":         server.Name,
				"owner_id":     server.Owner,
				"node_id":      server.Node,
			})
		}
		return c.Status(fiber.StatusCreated).JSON(server)
	})

	protected.Post("/servers/:id/power", mutationLimiter, func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		var req PowerRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		switch req.Signal {
		case "start", "stop", "restart", "kill":
			requiredPermission := store.PermControlStart
			switch req.Signal {
			case "stop", "kill":
				requiredPermission = store.PermControlStop
			case "restart":
				requiredPermission = store.PermControlRestart
			}
			if err := checkServerPermission(c, cfg, requiredPermission); err != nil {
				return err
			}
			if cfg.QueueService == nil {
				return fiber.NewError(fiber.StatusServiceUnavailable, "durable operation service is required")
			}
			jobType := queue.JobServerStart
			switch req.Signal {
			case "stop":
				jobType = queue.JobServerStop
			case "restart":
				jobType = queue.JobServerRestart
			case "kill":
				jobType = queue.JobServerKill
			}
			idempotencyKey := strings.TrimSpace(c.Get("Idempotency-Key"))
			if idempotencyKey != "" {
				idempotencyKey = "power:" + c.Params("id") + ":" + req.Signal + ":" + idempotencyKey
			}
			ctx, cancel := requestContext()
			defer cancel()
			job, err := cfg.QueueService.DispatchIdempotent(ctx, idempotencyKey, jobType, c.Params("id"), "", map[string]any{"signal": req.Signal}, 100)
			if err != nil {
				return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
			}
			return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
				"serverId":    c.Params("id"),
				"signal":      req.Signal,
				"accepted":    true,
				"mode":        "queued",
				"operationId": job.ID,
			})
		default:
			return fiber.NewError(fiber.StatusBadRequest, "invalid power signal")
		}
	})

	protected.Get("/operations/:id", func(c *fiber.Ctx) error {
		if cfg.QueueService == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "durable operation service is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		job, err := cfg.QueueService.Get(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if job == nil {
			return fiber.NewError(fiber.StatusNotFound, "operation not found")
		}
		return c.JSON(job)
	})

	protected.Post("/servers/:id/install", mutationLimiter, requireRole("admin"), requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if clusterManager == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "runtime lifecycle service is required")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		response, err := clusterManager.InstallServer(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		if cfg.Store != nil {
			cfg.Store.DispatchWebhookEvent("server:installed", map[string]any{"subject_type": "server", "subject_id": c.Params("id")})
		}
		return c.Status(fiber.StatusAccepted).JSON(response)
	})

	protected.Get("/servers/:id/configuration", requireRole("admin"), requireAdminScope("servers.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerProvisionTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		return c.JSON(buildDaemonServerConfiguration(target.ToDTO()))
	})

	protected.Post("/servers/:id/reinstall", mutationLimiter, requireServerPermission(cfg, store.PermSettingsReinstall), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if clusterManager == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "runtime lifecycle service is required")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		response, err := clusterManager.ReinstallServer(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.Status(fiber.StatusAccepted).JSON(response)
	})

	// Enforce user-level schedule cap.

	protected.Post("/servers/:id/toggle-install", requireRole("admin"), requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		next, err := cfg.Store.ToggleServerInstallStatus(ctx, c.Params("id"), actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"status": next})
	})

	protected.Post("/servers/:id/suspension", requireRole("admin"), requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		var body struct {
			Action string `json:"action"`
		}
		_ = c.BodyParser(&body)
		switch body.Action {
		case "suspend", "unsuspend":
		default:
			return fiber.NewError(fiber.StatusBadRequest, "action must be suspend or unsuspend")
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
		suspended := body.Action == "suspend"
		if suspended && cfg.Daemon != nil {
			// Best-effort: stop server processes when suspending.
			if target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id")); err == nil {
				_, _ = cfg.Daemon.SendPower(ctx, target.NodeURL, target.NodeToken, target.ServerID, "stop")
				_ = cfg.Store.SetServerPowerState(ctx, target.ServerID, "stop")
			}
		}
		if err := cfg.Store.SetServerSuspended(ctx, c.Params("id"), suspended, actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "suspended": suspended})
	})

	protected.Post("/servers/:id/suspend", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.SetServerSuspension(ctx, c.Params("id"), true); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/unsuspend", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.SetServerSuspension(ctx, c.Params("id"), false); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Delete("/servers/:id", requireRole("admin"), requireAdminScope("servers.delete"), func(c *fiber.Ctx) error {
		forceDelete := c.Query("force") == "1" || c.Query("force") == "true"
		if cfg.Store == nil || clusterManager == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and runtime lifecycle service are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		response, err := clusterManager.DeleteServer(ctx, c.Params("id"), forceDelete)
		if err != nil && !forceDelete {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		if err != nil && forceDelete {
			return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
				"serverId": c.Params("id"),
				"accepted": true,
				"mode":     "force",
				"warning":  err.Error(),
			})
		}
		return c.Status(fiber.StatusAccepted).JSON(response)
	})

	protected.Get("/servers/:id/stats", requireServerPermission(cfg, store.PermWebsocketConnect), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		stats, err := cfg.Daemon.Stats(ctx, target.NodeURL, target.NodeToken, target.ServerID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(stats)
	})

	protected.Get("/servers/:id/logs", requireServerPermission(cfg, store.PermWebsocketConnect), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		logs, err := cfg.Daemon.Logs(ctx, target.NodeURL, target.NodeToken, target.ServerID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		c.Set("Content-Type", "text/plain; charset=utf-8")
		return c.SendString(logs)
	})

	// Enforce user-level database cap.

	// Route aliases:
	//   DELETE /admin/servers/view/{serverId}/database/{databaseId}/delete
	//   PATCH  /admin/servers/view/{serverId}/database  body: { database: <id> }
	// (note: no id in URL for the reset-password endpoint). We register both
	// shapes so admin UIs that use either path work.

	// Parse pagination parameters

	// Enforce user-level backup cap (in addition to per-server cap below).

	// Enforce backup rate limit per server

	// Store a pending record immediately so the client can track progress
	// even if the daemon operation is asynchronous.

	// Initiate the backup on the daemon. The daemon will call back via
	// /api/remote/backups/:backup when the operation completes.

	// Mark the pending record as failed so it does not hang indefinitely.

	// Update the pending record with the daemon result.

	// The backup was created on the daemon but we failed to persist.
	// The daemon callback will reconcile this on retry.

	// Backup lock/unlock endpoints

	// Backup rename endpoint

	// Backup cleanup endpoint

	// Get panel settings for retention policy

	// Get server-specific backup limit

	// Perform cleanup

	// Subuser invitation endpoints

	// Create invitation with 7-day expiration

	// Look for "file" or "files" or "content" fields

	// Check for text values in case they sent text in "content" form field

	protected.Get("/admin/audit", requireRole("admin"), requireAdminScope("audit.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		events, err := cfg.Store.ListAudit(ctx)
		if err != nil {
			return err
		}
		return c.JSON(events)
	})

	protected.Get("/audit", requireRole("admin"), requireAdminScope("audit.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		events, err := cfg.Store.ListAudit(ctx)
		if err != nil {
			return err
		}
		return c.JSON(events)
	})

	// ---- PufferPanel-inspired operations pipeline ----

	// ---- API parity endpoints ----

	protected.Get("/servers/:id/status", requireServerAccess(cfg), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		server, err := cfg.Store.GetServer(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		running := server.Status == "running" || server.ActualState == store.ServerActualStateRunning
		installing := server.Status == "installing" || server.Status == "install_failed"
		return c.JSON(fiber.Map{
			"running":    running,
			"installing": installing,
		})
	})

	protected.Patch("/servers/:id/details", mutationLimiter, requireServerPermission(cfg, store.PermSettingsRename), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		server, err := cfg.Store.UpdateServer(ctx, c.Params("id"), store.UpdateServerRequest{
			Name: req.Name, Description: req.Description,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(server)
	})

	protected.Patch("/servers/:id/build", mutationLimiter, requireRole("admin"), requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			MemoryMB      *int    `json:"memoryMb"`
			CPUShares     *int    `json:"cpuShares"`
			CPULimit      *int    `json:"cpuLimit"`
			DiskMB        *int    `json:"diskMb"`
			DatabaseLimit *int    `json:"databaseLimit"`
			BackupLimit   *int    `json:"backupLimit"`
			IOWeight      *int    `json:"ioWeight"`
			SwapMB        *int    `json:"swapMb"`
			Threads       *string `json:"threads"`
			OOMDisabled   *bool   `json:"oomDisabled"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		server, err := cfg.Store.UpdateServer(ctx, c.Params("id"), store.UpdateServerRequest{
			MemoryMB: req.MemoryMB, CPUShares: req.CPUShares, CPULimit: req.CPULimit,
			DiskMB: req.DiskMB, DatabaseLimit: req.DatabaseLimit, BackupLimit: req.BackupLimit,
			IOWeight: req.IOWeight, SwapMB: req.SwapMB, Threads: req.Threads, OOMDisabled: req.OOMDisabled,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(server)
	})

	protected.Post("/servers/:id/settings/rename", mutationLimiter, requireServerPermission(cfg, store.PermSettingsRename), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Name string `json:"name"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.Name) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "name is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		server, err := cfg.Store.UpdateServer(ctx, c.Params("id"), store.UpdateServerRequest{
			Name: &req.Name,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(server)
	})

	protected.Get("/servers/:id/flags", requireServerAccess(cfg), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		flags, err := cfg.Store.GetServerFlags(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		return c.JSON(flags)
	})

	protected.Post("/servers/:id/flags", mutationLimiter, requireServerPermission(cfg, store.PermSettingsRename), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			AutoStart   *bool `json:"auto_start"`
			AutoRestart *bool `json:"auto_restart"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		current, err := cfg.Store.GetServerFlags(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if req.AutoStart != nil {
			current.AutoStart = *req.AutoStart
		}
		if req.AutoRestart != nil {
			current.AutoRestart = *req.AutoRestart
		}
		if err := cfg.Store.SetServerFlags(ctx, c.Params("id"), current); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(current)
	})

	protected.Post("/servers/:id/operations/run", mutationLimiter, requireServerPermission(cfg, store.PermSettingsReinstall), func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotImplemented, "operation pipelines are disabled until durable execution, per-step authorization, and failure reporting are implemented")
	})
}
