package http

import (
	"errors"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// registerOrphanRemediationRoutes exposes the cleanup work retained after a
// forced deletion removed the panel record before its remote resource.
func registerOrphanRemediationRoutes(protected fiber.Router, cfg Config, mutationLimiter, adminIPAccess fiber.Handler) {
	admin := protected.Group("/admin/orphan-remediations", adminIPAccess, requireRole("admin"))

	admin.Get("/", requireAdminScope("servers.read"), requireAdminScope("databases.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		status, err := orphanRemediationStatusFromRequest(c)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()

		serverRemediations, err := cfg.Store.ListServerOrphanRemediations(ctx, status)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to list server orphan remediations")
		}
		databaseRemediations, err := cfg.Store.ListDatabaseOrphanRemediations(ctx, status)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to list database orphan remediations")
		}
		return c.JSON(fiber.Map{
			"serverRemediations":   serverRemediations,
			"databaseRemediations": databaseRemediations,
		})
	})

	admin.Post("/servers/:id/resolve", mutationLimiter, requireAdminScope("servers.delete"), func(c *fiber.Ctx) error {
		return resolveServerOrphanRemediation(c, cfg)
	})
	admin.Post("/databases/:id/resolve", mutationLimiter, requireAdminScope("databases.delete"), func(c *fiber.Ctx) error {
		return resolveDatabaseOrphanRemediation(c, cfg)
	})
}

func orphanRemediationStatusFromRequest(c *fiber.Ctx) (store.OrphanRemediationStatus, error) {
	status := store.OrphanRemediationStatus(c.Query("status"))
	switch status {
	case "", store.OrphanRemediationStatusPending, store.OrphanRemediationStatusResolved:
		return status, nil
	default:
		return "", fiber.NewError(fiber.StatusBadRequest, "status must be pending or resolved")
	}
}

func resolveServerOrphanRemediation(c *fiber.Ctx, cfg Config) error {
	if cfg.Store == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
	}
	ctx, cancel := requestContext()
	defer cancel()
	actorID := remediationActorID(c)
	remediation, err := cfg.Store.ResolveServerOrphanRemediation(ctx, c.Params("id"), actorID)
	if err != nil {
		return orphanRemediationResolutionHTTPError(err)
	}
	return c.JSON(remediation)
}

func resolveDatabaseOrphanRemediation(c *fiber.Ctx, cfg Config) error {
	if cfg.Store == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
	}
	ctx, cancel := requestContext()
	defer cancel()
	actorID := remediationActorID(c)
	remediation, err := cfg.Store.ResolveDatabaseOrphanRemediation(ctx, c.Params("id"), actorID)
	if err != nil {
		return orphanRemediationResolutionHTTPError(err)
	}
	return c.JSON(remediation)
}

func remediationActorID(c *fiber.Ctx) *string {
	if claims, ok := c.Locals("user").(tokenClaims); ok {
		return &claims.Sub
	}
	return nil
}

func orphanRemediationResolutionHTTPError(err error) error {
	switch {
	case errors.Is(err, store.ErrOrphanRemediationNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, store.ErrOrphanRemediationResolved):
		return fiber.NewError(fiber.StatusConflict, err.Error())
	default:
		return fiber.NewError(fiber.StatusInternalServerError, "failed to resolve orphan remediation")
	}
}
