package http

import (
	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/services/evacuationplanner"
	migrationservice "gamepanel/forge/internal/services/migration"
	"gamepanel/forge/internal/services/noderegistry"
	recoverysvc "gamepanel/forge/internal/services/recovery"
	reservationsvc "gamepanel/forge/internal/services/reservations"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerOperationsAdminRoutes(protected fiber.Router, cfg Config, nodeRegistry *noderegistry.Service, clusterManager *clustermanager.Service, evacuationPlanner *evacuationplanner.Service, migrationService *migrationservice.Service, reservationManager *reservationsvc.Manager, recoveryCoordinator *recoverysvc.Coordinator, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {

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
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
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
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
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

	// ---- Egg Export/Import ----

	// Test a prospective host before it is saved. This uses the same normalization,
	// validation, TLS configuration, and ping flow as provisioning without persistence.

	// This is an administrator-only diagnostic for an external provisioning host.
	// Preserve the connector's sanitized cause (timeout, TLS, authentication, etc.)
	// instead of collapsing every failure into an indistinguishable message.

	// TLS CA values are write-only. Treat a blank value as omitted so clients can
	// safely round-trip an edit without overwriting a redacted certificate.

	// Server <-> Mount attachment. The mount must be eligible for the server's
	// node and egg, just as it is for the server-scoped assignment route.

	// Register the static path before /allocations/:id so it is not interpreted as id="bulk".

	// ---- Plugins ----

	// ---- Roles ----

	// ---- OAuth2 admin routes ----

	// ---- Webhooks ----

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
