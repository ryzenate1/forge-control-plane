package http

import (
	"errors"
	"strings"

	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/services/evacuationplanner"
	migrationservice "gamepanel/forge/internal/services/migration"
	"gamepanel/forge/internal/services/noderegistry"
	recoverysvc "gamepanel/forge/internal/services/recovery"
	reservationsvc "gamepanel/forge/internal/services/reservations"

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

	if recoveryCoordinator != nil && cfg.Store != nil && cfg.Daemon != nil {
		recoveryCoordinator.SetBackupRestoreExecutor(recoverysvc.NewDaemonBackupRestoreExecutor(cfg.Store, cfg.Daemon, clusterManager))
	}
	registerAdministrationAdminRoutes(protected, cfg, nodeRegistry, clusterManager, evacuationPlanner, migrationService, reservationManager, recoveryCoordinator, mutationLimiter, adminIPAccess)
	registerAllocationsAdminRoutes(protected, cfg, nodeRegistry, clusterManager, evacuationPlanner, migrationService, reservationManager, recoveryCoordinator, mutationLimiter, adminIPAccess)
	registerDatabasesAdminRoutes(protected, cfg, nodeRegistry, clusterManager, evacuationPlanner, migrationService, reservationManager, recoveryCoordinator, mutationLimiter, adminIPAccess)
	registerMountsAdminRoutes(protected, cfg, nodeRegistry, clusterManager, evacuationPlanner, migrationService, reservationManager, recoveryCoordinator, mutationLimiter, adminIPAccess)
	registerNodesAdminRoutes(protected, cfg, nodeRegistry, clusterManager, evacuationPlanner, migrationService, reservationManager, recoveryCoordinator, mutationLimiter, adminIPAccess)
	registerOperationsAdminRoutes(protected, cfg, nodeRegistry, clusterManager, evacuationPlanner, migrationService, reservationManager, recoveryCoordinator, mutationLimiter, adminIPAccess)
	registerTemplatesAdminRoutes(protected, cfg, nodeRegistry, clusterManager, evacuationPlanner, migrationService, reservationManager, recoveryCoordinator, mutationLimiter, adminIPAccess)
	registerTopologyAdminRoutes(protected, cfg, nodeRegistry, clusterManager, evacuationPlanner, migrationService, reservationManager, recoveryCoordinator, mutationLimiter, adminIPAccess)

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

// workloadExecutionNotImplemented remains available for routes whose runtime
// has no executor. Recovery and evacuation no longer use this handler.
func workloadExecutionNotImplemented(operation string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusNotImplemented, operation+" is not implemented; no workload executor is available")
	}
}

func migrationRouteError(err error) error {
	var notImplemented *migrationservice.NotImplementedError
	if errors.As(err, &notImplemented) {
		return fiber.NewError(fiber.StatusNotImplemented, err.Error())
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

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}
