package http

import (
	"encoding/json"
	"time"

	"gamepanel/forge/internal/services/reconciler"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerReconcileRoutes(protected fiber.Router, cfg Config, svc *reconciler.Service, adminIPAccess, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	rc := protected.Group("/admin/reconcile", adminIPAccess)

	rc.Get("/summary", requireRole("admin"), requireAdminScope("reconcile.read"), func(c *fiber.Ctx) error {
		summary, err := svc.ReconcileSummary(c.Context())
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": summary})
	})

	rc.Get("/plans", requireRole("admin"), requireAdminScope("reconcile.read"), func(c *fiber.Ctx) error {
		offset := c.QueryInt("offset", 0)
		limit := c.QueryInt("limit", 50)
		rows, total, err := svc.ListReconcilePlans(c.Context(), offset, limit)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		resp := make([]reconcilePlanRowResponse, len(rows))
		for i, r := range rows {
			resp[i] = toPlanRowResponse(&r)
		}
		return c.JSON(fiber.Map{"data": resp, "total": total})
	})

	rc.Get("/plans/:id", requireRole("admin"), requireAdminScope("reconcile.read"), func(c *fiber.Ctx) error {
		row, err := svc.GetReconcilePlan(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "plan not found"})
		}
		return c.JSON(fiber.Map{"data": toPlanRowResponse(row)})
	})

	rc.Post("/plans/:id/confirm", mutationLimiter, requireRole("admin"), requireAdminScope("reconcile.write"), func(c *fiber.Ctx) error {
		planID := c.Params("id")
		result, err := svc.ConfirmAndExecute(c.Context(), planID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": result})
	})

	rc.Post("/plans/:id/execute", mutationLimiter, requireRole("admin"), requireAdminScope("reconcile.write"), func(c *fiber.Ctx) error {
		planID := c.Params("id")
		result, err := svc.ExecuteReconcilePlan(c.Context(), planID)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": result})
	})

	rc.Post("/resources/:kind/:id", mutationLimiter, requireRole("admin"), requireAdminScope("reconcile.write"), func(c *fiber.Ctx) error {
		resourceKind := c.Params("kind")
		resourceID := c.Params("id")

		var rk reconciler.ResourceKind
		switch resourceKind {
		case "server":
			rk = reconciler.ResourceKindServer
		case "node":
			rk = reconciler.ResourceKindNode
		case "compose_stack":
			rk = reconciler.ResourceKindComposeStack
		default:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "unsupported resource kind: " + resourceKind})
		}

		result, err := svc.ReconcileResource(c.Context(), resourceID, rk)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": result})
	})

	rc.Post("/trigger-all", mutationLimiter, requireRole("admin"), requireAdminScope("reconcile.write"), func(c *fiber.Ctx) error {
		type triggerRequest struct {
			Kind string `json:"kind"`
		}
		var req triggerRequest
		if err := c.BodyParser(&req); err != nil {
			req.Kind = "all"
		}

		var results []*reconciler.ReconcileResult
		var err error

		switch req.Kind {
		case "servers", "server":
			results, err = svc.ReconcileAllResources(c.Context())
		case "nodes", "node":
			results, err = svc.ReconcileNodes(c.Context())
		case "compose_stacks", "compose_stack":
			results, err = svc.ReconcileAllComposeStacks(c.Context())
		default:
			results, err = svc.ReconcileAllResources(c.Context())
		}

		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": results, "count": len(results)})
	})

	rc.Post("/events", requireRole("admin"), requireAdminScope("reconcile.read"), func(c *fiber.Ctx) error {
		type eventsRequest struct {
			ResourceID string `json:"resourceId"`
			Limit      int    `json:"limit"`
		}
		var req eventsRequest
		if err := c.BodyParser(&req); err != nil {
			req.ResourceID = ""
			req.Limit = 50
		}
		if req.Limit <= 0 {
			req.Limit = 50
		}
		rows, err := svc.ListReconcileEvents(c.Context(), req.ResourceID, req.Limit)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		eventResp := make([]reconcileEventRowResponse, len(rows))
		for i, r := range rows {
			eventResp[i] = toEventRowResponse(&r)
		}
		return c.JSON(fiber.Map{"data": eventResp})
	})

	rc.Get("/metrics", requireRole("admin"), requireAdminScope("reconcile.read"), func(c *fiber.Ctx) error {
		metrics := svc.Metrics()
		return c.JSON(fiber.Map{"data": metrics})
	})
}

type reconcilePlanRowResponse struct {
	ID           string              `json:"id"`
	ResourceID   string              `json:"resourceId"`
	ResourceKind string              `json:"resourceKind"`
	State        string              `json:"state"`
	Destructive  bool                `json:"destructive"`
	Confirmed    bool                `json:"confirmed"`
	DiffCount    int                 `json:"diffCount"`
	DriftCount   int                 `json:"driftCount"`
	Diffs        []reconciler.ReconcileDiff `json:"diffs"`
	Drifts       []reconciler.DriftRecord   `json:"drifts"`
	Error        string              `json:"error,omitempty"`
	CreatedAt    time.Time           `json:"createdAt"`
	ExecutedAt   *time.Time          `json:"executedAt,omitempty"`
}

type reconcileEventRowResponse struct {
	ID           string    `json:"id"`
	PlanID       string    `json:"planId"`
	ResourceID   string    `json:"resourceId"`
	ResourceKind string    `json:"resourceKind"`
	EventType    string    `json:"eventType"`
	Summary      string    `json:"summary"`
	CreatedAt    time.Time `json:"createdAt"`
}

func toPlanRowResponse(row *store.ReconcilePlanRow) reconcilePlanRowResponse {
	if row == nil {
		return reconcilePlanRowResponse{}
	}
	var diffs []reconciler.ReconcileDiff
	var drifts []reconciler.DriftRecord
	if len(row.DiffData) > 0 {
		_ = json.Unmarshal(row.DiffData, &diffs)
	}
	if len(row.DriftData) > 0 {
		_ = json.Unmarshal(row.DriftData, &drifts)
	}
	return reconcilePlanRowResponse{
		ID:           row.ID,
		ResourceID:   row.ResourceID,
		ResourceKind: row.ResourceKind,
		State:        row.State,
		Destructive:  row.Destructive,
		Confirmed:    row.Confirmed,
		DiffCount:    row.DiffCount,
		DriftCount:   row.DriftCount,
		Diffs:        diffs,
		Drifts:       drifts,
		Error:        row.Error,
		CreatedAt:    row.CreatedAt,
		ExecutedAt:   row.ExecutedAt,
	}
}

func toEventRowResponse(row *store.ReconcileEventRow) reconcileEventRowResponse {
	if row == nil {
		return reconcileEventRowResponse{}
	}
	return reconcileEventRowResponse{
		ID:           row.ID,
		PlanID:       row.PlanID,
		ResourceID:   row.ResourceID,
		ResourceKind: row.ResourceKind,
		EventType:    row.EventType,
		Summary:      row.Summary,
		CreatedAt:    row.CreatedAt,
	}
}
