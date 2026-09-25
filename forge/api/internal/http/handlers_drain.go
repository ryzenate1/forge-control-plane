package http

import (
	"strings"

	drainsvc "gamepanel/forge/internal/services/drain"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// registerDrainRoutes exposes the durable drain LEDGER (services/drain), which
// passively mirrors clustermembership's drain/evacuation events into the
// drain_states table (migration 191) so progress survives restarts.
//
// Mounted on its own /drain-ledger group rather than under /nodes: the admin
// router registers GET /nodes/:id, which greedily matches /nodes/drain and would
// otherwise shadow the aggregate listing as "node not found". clustermembership
// keeps the per-node begin/cancel/status routes (/nodes/:id/drain, .../cancel);
// the ledger only records and reads, so there is no mutation or collision here.
func registerDrainRoutes(protected fiber.Router, cfg Config, svc *drainsvc.Service) {
	if svc == nil {
		return
	}

	ledger := protected.Group("/drain-ledger", requireAdminScope("nodes.read"))

	// Fleet-wide drain ledger listing.
	ledger.Get("", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		states, err := svc.List(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		if states == nil {
			states = []store.DrainState{}
		}
		return c.JSON(fiber.Map{"data": states})
	})

	// Per-node durable progress.
	ledger.Get("/:nodeId", func(c *fiber.Ctx) error {
		nodeID := strings.TrimSpace(c.Params("nodeId"))
		if nodeID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "node id is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		state, err := svc.Status(ctx, nodeID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		// Status returns a zero DrainState with nil error when nothing recorded;
		// surface that as JSON null rather than an empty object so the UI can
		// distinguish "never drained" from "drained".
		if state.NodeID == "" {
			return c.JSON(fiber.Map{"data": nil})
		}
		return c.JSON(fiber.Map{"data": state})
	})
}
