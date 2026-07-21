package http

import (
	"gamepanel/forge/internal/services/procedure"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerProcedureRoutes(protected fiber.Router, cfg Config, svc *procedure.Service, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	proc := protected.Group("/procedures")

	proc.Get("/", func(c *fiber.Ctx) error {
		tenantID := c.Query("tenantId")
		var tenantIDPtr *string
		if tenantID != "" {
			tenantIDPtr = &tenantID
		}
		procedures, err := svc.ListProcedures(c.Context(), tenantIDPtr)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": procedures})
	})

	proc.Get("/:id", func(c *fiber.Ctx) error {
		p, err := svc.GetProcedure(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": p})
	})

	proc.Post("/", mutationLimiter, func(c *fiber.Ctx) error {
		var req createProcedureRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request: " + err.Error()})
		}
		if req.Name == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
		}
		storeReq := req.toStoreRequest()
		p, err := svc.CreateProcedure(c.Context(), storeReq)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": p})
	})

	proc.Put("/:id", mutationLimiter, func(c *fiber.Ctx) error {
		var req createProcedureRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request: " + err.Error()})
		}
		if req.Name == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
		}
		p, err := svc.UpdateProcedure(c.Context(), c.Params("id"), req.toStoreRequest())
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": p})
	})

	proc.Delete("/:id", mutationLimiter, func(c *fiber.Ctx) error {
		if err := svc.DeleteProcedure(c.Context(), c.Params("id")); err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	proc.Post("/:id/execute", mutationLimiter, func(c *fiber.Ctx) error {
		userID := getUserID(c)
		var userIDPtr *string
		if userID != "" {
			userIDPtr = &userID
		}
		execution, err := svc.ExecuteProcedure(c.Context(), c.Params("id"), "manual", nil, userIDPtr)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"data": execution})
	})

	exec := proc.Group("/executions")

	exec.Get("/:id", func(c *fiber.Ctx) error {
		e, err := svc.GetExecution(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": e})
	})

	exec.Post("/:id/cancel", mutationLimiter, func(c *fiber.Ctx) error {
		if err := svc.CancelExecution(c.Context(), c.Params("id")); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"status": "cancelled"})
	})

	exec.Post("/steps/:stepId/approve", mutationLimiter, func(c *fiber.Ctx) error {
		userID := getUserID(c)
		if err := svc.ApproveStep(c.Context(), c.Params("stepId"), &userID); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"status": "approved"})
	})

	exec.Post("/steps/:stepId/reject", mutationLimiter, func(c *fiber.Ctx) error {
		userID := getUserID(c)
		if err := svc.RejectStep(c.Context(), c.Params("stepId"), &userID); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"status": "rejected"})
	})

	exec.Get("/steps/:stepId/logs", func(c *fiber.Ctx) error {
		logs, err := svc.ListStepLogs(c.Context(), c.Params("stepId"))
		if err != nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": logs})
	})

	proc.Get("/:id/executions", func(c *fiber.Ctx) error {
		limit := c.QueryInt("limit", 20)
		executions, err := svc.ListExecutions(c.Context(), c.Params("id"), limit)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": executions})
	})
}

type createProcedureStepRequest struct {
	Position          int            `json:"position"`
	Name              string         `json:"name"`
	Action            string         `json:"action"`
	Config            map[string]any `json:"config"`
	MaxRetries        int            `json:"maxRetries"`
	TimeoutSeconds    int            `json:"timeoutSeconds"`
	RequiresApproval  bool           `json:"requiresApproval"`
	ContinueOnFailure bool           `json:"continueOnFailure"`
	RollbackEnabled   bool           `json:"rollbackEnabled"`
}

type createProcedureScheduleRequest struct {
	CronExpression string `json:"cronExpression"`
	Timezone       string `json:"timezone"`
	Enabled        bool   `json:"enabled"`
}

type createProcedureRequest struct {
	Name        string                          `json:"name"`
	Description string                          `json:"description"`
	TenantID    *string                         `json:"tenantId,omitempty"`
	Enabled     bool                            `json:"enabled"`
	Steps       []createProcedureStepRequest    `json:"steps"`
	Schedule    *createProcedureScheduleRequest `json:"schedule,omitempty"`
}

func (r createProcedureRequest) toStoreRequest() store.CreateProcedureRequest {
	steps := make([]store.CreateProcedureStepRequest, len(r.Steps))
	for i, s := range r.Steps {
		if s.MaxRetries <= 0 {
			s.MaxRetries = 3
		}
		if s.TimeoutSeconds <= 0 {
			s.TimeoutSeconds = 300
		}
		if s.Action == "" {
			s.Action = "run_command"
		}
		steps[i] = store.CreateProcedureStepRequest{
			Position:          s.Position,
			Name:              s.Name,
			Action:            s.Action,
			Config:            s.Config,
			MaxRetries:        s.MaxRetries,
			TimeoutSeconds:    s.TimeoutSeconds,
			RequiresApproval:  s.RequiresApproval,
			ContinueOnFailure: s.ContinueOnFailure,
			RollbackEnabled:   s.RollbackEnabled,
		}
	}
	var schedule *store.CreateProcedureScheduleRequest
	if r.Schedule != nil && r.Schedule.CronExpression != "" {
		schedule = &store.CreateProcedureScheduleRequest{
			CronExpression: r.Schedule.CronExpression,
			Timezone:       r.Schedule.Timezone,
			Enabled:        r.Schedule.Enabled,
		}
	}
	return store.CreateProcedureRequest{
		Name:        r.Name,
		Description: r.Description,
		TenantID:    r.TenantID,
		Enabled:     r.Enabled,
		Steps:       steps,
		Schedule:    schedule,
	}
}
