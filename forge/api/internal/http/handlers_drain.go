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
// Deliberately non-conflicting: clustermembership already owns the per-node
// begin/cancel/status routes (/nodes/:id/drain POST/GET, /nodes/:id/drain/cancel).
// Those keep orchestrating drains. The ledger adds only a fleet-wide listing and
// a per-node progress view on distinct paths, so both can coexist with no
// shadowing. Reads are node-scoped admin reads; the ledger records, never drives,
// so there are no mutations here.
func registerDrainRoutes(protected fiber.Router, cfg Config, svc *drainsvc.Service) {
	if svc == nil {
		return
	}

	// Fleet-wide drain ledger listing. No existing route owns /nodes/drain.
	protected.Get("/nodes/drain", requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
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

	// Per-node durable progress. Uses /drain/progress to avoid clobbering the
	// clustermembership GET /nodes/:id/drain status endpoint.
	protected.Get("/nodes/:id/drain/progress", requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		nodeID := strings.TrimSpace(c.Params("id"))
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
