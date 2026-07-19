package http

import (
	"fmt"
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

func registerDatabasesAdminRoutes(protected fiber.Router, cfg Config, nodeRegistry *noderegistry.Service, clusterManager *clustermanager.Service, evacuationPlanner *evacuationplanner.Service, migrationService *migrationservice.Service, reservationManager *reservationsvc.Manager, recoveryCoordinator *recoverysvc.Coordinator, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {

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
}
