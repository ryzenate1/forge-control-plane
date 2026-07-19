package http

import (
	"errors"
	"strings"

	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerDatabasesServerRoutes(protected fiber.Router, cfg Config, runner *scheduleRunner, clusterManager *clustermanager.Service, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {

	protected.Get("/servers/:id/databases", requireServerPermission(cfg, store.PermDatabaseRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		databases, err := cfg.Store.ListServerDatabases(ctx, c.Params("id"), false)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(databases)
	})

	protected.Get("/servers/:id/databases/:databaseId", requireServerPermission(cfg, store.PermDatabaseRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		database, err := cfg.Store.GetServerDatabase(ctx, c.Params("id"), c.Params("databaseId"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "database not found")
		}
		return c.JSON(database)
	})

	protected.Post("/servers/:id/databases", requireServerPermission(cfg, store.PermDatabaseCreate), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req CreateServerDatabaseRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		// Enforce user-level database cap.
		if ownerID, ok := serverOwner(ctx, cfg, c.Params("id")); ok {
			if err := cfg.Store.CheckUserCanCreateDatabase(ctx, ownerID); err != nil {
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
		database, err := cfg.Store.CreateServerDatabase(ctx, c.Params("id"), store.CreateServerDatabaseRequest{
			Database:       req.Database,
			Remote:         req.Remote,
			MaxConnections: req.MaxConnections,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if cfg.DBProvisioner == nil {
			_ = cfg.Store.SetServerDatabaseProvisioningState(ctx, c.Params("id"), database.ID, store.DatabaseStateFailed, "database provisioner is unavailable")
			return fiber.NewError(fiber.StatusServiceUnavailable, "database record created in failed state: database provisioner is unavailable")
		}
		if err := cfg.DBProvisioner.Provision(ctx, c.Params("id"), database.ID); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "database provisioning failed; record retained in failed state: "+err.Error())
		}
		database, err = cfg.Store.GetServerDatabaseForProvisioning(ctx, c.Params("id"), database.ID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "database is ready but its creation response could not be loaded")
		}
		if database.ProvisioningState != store.DatabaseStateReady {
			database.Password = nil
			return fiber.NewError(fiber.StatusInternalServerError, "database did not reach ready state")
		}
		return c.Status(fiber.StatusCreated).JSON(database)
	})

	protected.Post("/servers/:id/databases/:databaseId/rotate-password", requireServerPermission(cfg, store.PermDatabaseUpdate), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if cfg.DBProvisioner == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "database provisioner is unavailable")
		}
		database, err := cfg.DBProvisioner.RotatePassword(ctx, c.Params("id"), c.Params("databaseId"), actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "failed to rotate database password: "+err.Error())
		}
		return c.JSON(database)
	})

	protected.Delete("/servers/:id/databases/:databaseId", requireServerPermission(cfg, store.PermDatabaseDelete), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		force := strings.EqualFold(c.Query("force"), "true")
		var deprovisionErr error
		if cfg.DBProvisioner == nil {
			deprovisionErr = errors.New("database provisioner is unavailable")
		} else {
			deprovisionErr = cfg.DBProvisioner.Deprovision(ctx, c.Params("id"), c.Params("databaseId"))
		}
		if deprovisionErr != nil {
			if !force {
				return fiber.NewError(fiber.StatusBadGateway, "failed to deprovision database; panel record retained: "+deprovisionErr.Error())
			}
			if err := cfg.Store.ForceDeleteServerDatabase(ctx, c.Params("id"), c.Params("databaseId"), deprovisionErr.Error(), actorID); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "failed to record orphan remediation: "+err.Error())
			}
			return c.JSON(fiber.Map{"ok": true, "orphanRemediation": true})
		}
		if err := cfg.Store.DeleteServerDatabase(ctx, c.Params("id"), c.Params("databaseId"), actorID); err != nil {
			detail := "remote database resources were removed, but panel record deletion failed: " + err.Error()
			_ = cfg.Store.SetServerDatabaseProvisioningState(ctx, c.Params("id"), c.Params("databaseId"), store.DatabaseStateFailed, detail)
			return fiber.NewError(fiber.StatusInternalServerError, detail)
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// Route aliases:
	//   DELETE /admin/servers/view/{serverId}/database/{databaseId}/delete
	//   PATCH  /admin/servers/view/{serverId}/database  body: { database: <id> }
	// (note: no id in URL for the reset-password endpoint). We register both
	// shapes so admin UIs that use either path work.
	protected.Delete("/servers/:id/databases/:databaseId/delete", requireServerPermission(cfg, store.PermDatabaseDelete), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		force := strings.EqualFold(c.Query("force"), "true")
		var deprovisionErr error
		if cfg.DBProvisioner == nil {
			deprovisionErr = errors.New("database provisioner is unavailable")
		} else {
			deprovisionErr = cfg.DBProvisioner.Deprovision(ctx, c.Params("id"), c.Params("databaseId"))
		}
		if deprovisionErr != nil {
			if !force {
				return fiber.NewError(fiber.StatusBadGateway, "failed to deprovision database; panel record retained: "+deprovisionErr.Error())
			}
			if err := cfg.Store.ForceDeleteServerDatabase(ctx, c.Params("id"), c.Params("databaseId"), deprovisionErr.Error(), actorID); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "failed to record orphan remediation: "+err.Error())
			}
			return c.SendStatus(fiber.StatusNoContent)
		}
		if err := cfg.Store.DeleteServerDatabase(ctx, c.Params("id"), c.Params("databaseId"), actorID); err != nil {
			detail := "remote database resources were removed, but panel record deletion failed: " + err.Error()
			_ = cfg.Store.SetServerDatabaseProvisioningState(ctx, c.Params("id"), c.Params("databaseId"), store.DatabaseStateFailed, detail)
			return fiber.NewError(fiber.StatusInternalServerError, detail)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
	protected.Patch("/servers/:id/databases/reset-password", requireServerPermission(cfg, store.PermDatabaseUpdate), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var body struct {
			Database string `json:"database"`
		}
		if err := c.BodyParser(&body); err != nil || body.Database == "" {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body; expected { database: <id> }")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if cfg.DBProvisioner == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "database provisioner is unavailable")
		}
		if _, err := cfg.DBProvisioner.RotatePassword(ctx, c.Params("id"), body.Database, actorID); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "failed to rotate database password: "+err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
}
