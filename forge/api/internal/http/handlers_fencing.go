package http

import (
	"strings"

	fencingsvc "gamepanel/forge/internal/services/fencing"

	"github.com/gofiber/fiber/v2"
)

// registerFencingRoutes exposes the fencing service's operator surface. The
// fencing service already runs as an event subscriber (subscribed to
// EventNodeRecovered in main.go), automatically bumping workload generations
// when a node returns from offline so stale leases are invalidated. This
// handler lets admins trigger the same fence operation manually — e.g. after
// a maintenance window or before a planned failover test — and view which
// servers were affected.
//
// Route prefix /fencing is distinct from /nodes/:id/* so no shadowing occurs.
func registerFencingRoutes(protected fiber.Router, cfg Config, svc *fencingsvc.Service, mutationLimiter fiber.Handler) {
	if svc == nil || cfg.Store == nil {
		return
	}

	fence := protected.Group("/fencing", requireRole("admin"))

	// Manual fence: bumps every server on the node's generation and publishes
	// EventNodeFenced. Returns the affected server IDs.
	fence.Post("/nodes/:id", mutationLimiter, func(c *fiber.Ctx) error {
		nodeID := strings.TrimSpace(c.Params("id"))
		if nodeID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "node id is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		servers, err := cfg.Store.ListServersForNode(ctx, nodeID)
		if err != nil {
			return respondInternalError(c, err)
		}
		if len(servers) == 0 {
			return c.JSON(fiber.Map{"data": fiber.Map{
				"nodeId":       nodeID,
				"fenced":       false,
				"serverCount":  0,
				"reason":       "no servers on this node",
			}})
		}
		if err := svc.FenceNode(ctx, nodeID); err != nil {
			return respondInternalError(c, err)
		}
		ids := make([]string, 0, len(servers))
		for _, s := range servers {
			ids = append(ids, s.ID)
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": fiber.Map{
			"nodeId":      nodeID,
			"fenced":      true,
			"serverCount": len(servers),
			"serverIds":   ids,
		}})
	})

	// Preview what would be fenced without mutating state. Useful before a
	// planned maintenance window.
	fence.Get("/nodes/:id/preview", func(c *fiber.Ctx) error {
		nodeID := strings.TrimSpace(c.Params("id"))
		if nodeID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "node id is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		servers, err := cfg.Store.ListServersForNode(ctx, nodeID)
		if err != nil {
			return respondInternalError(c, err)
		}
		type previewRow struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Status     string `json:"status"`
			Generation int    `json:"generation"`
		}
		out := make([]previewRow, 0, len(servers))
		for _, s := range servers {
			out = append(out, previewRow{ID: s.ID, Name: s.Name, Status: s.Status, Generation: s.Generation})
		}
		return c.JSON(fiber.Map{"data": out})
	})
}
