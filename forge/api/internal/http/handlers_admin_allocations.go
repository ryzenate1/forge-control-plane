package http

import (
	"io"
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

func registerAllocationsAdminRoutes(protected fiber.Router, cfg Config, nodeRegistry *noderegistry.Service, clusterManager *clustermanager.Service, evacuationPlanner *evacuationplanner.Service, migrationService *migrationservice.Service, reservationManager *reservationsvc.Manager, recoveryCoordinator *recoverysvc.Coordinator, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {

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

	// Test a prospective host before it is saved. This uses the same normalization,
	// validation, TLS configuration, and ping flow as provisioning without persistence.

	// This is an administrator-only diagnostic for an external provisioning host.
	// Preserve the connector's sanitized cause (timeout, TLS, authentication, etc.)
	// instead of collapsing every failure into an indistinguishable message.

	// TLS CA values are write-only. Treat a blank value as omitted so clients can
	// safely round-trip an edit without overwriting a redacted certificate.

	// Server <-> Mount attachment. The mount must be eligible for the server's
	// node and egg, just as it is for the server-scoped assignment route.

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
}
