package http

import (
	"gamepanel/forge/internal/services/process"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerProcessRoutes(protected fiber.Router, cfg Config, processSvc *process.Service) {
	protected.Get("/servers/:id/processes", requireServerPermission(cfg, store.PermControlStart), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		processes, err := processSvc.ListProcesses(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(processes)
	})

	protected.Post("/servers/:id/processes", requireServerPermission(cfg, store.PermControlStart), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Processes []process.ProcfileEntry `json:"processes" validate:"required"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		results, err := processSvc.SetProcesses(ctx, c.Params("id"), req.Processes)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(results)
	})

	protected.Put("/servers/:id/processes/:type/scale", requireServerPermission(cfg, store.PermControlStart), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Quantity int `json:"quantity" validate:"required"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		var triggeredBy string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			triggeredBy = claims.Sub
		}
		ctx, cancel := requestContext()
		defer cancel()
		pt, err := processSvc.ScaleProcess(ctx, c.Params("id"), c.Params("type"), req.Quantity, triggeredBy)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(pt)
	})

	protected.Post("/servers/:id/processes/run", requireServerPermission(cfg, store.PermControlConsole), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Command string `json:"command" validate:"required"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.Command == "" {
			return fiber.NewError(fiber.StatusBadRequest, "command is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		task, err := processSvc.RunOneOffTask(ctx, c.Params("id"), req.Command)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(task)
	})

	protected.Get("/servers/:id/processes/tasks", requireServerPermission(cfg, store.PermControlConsole), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		tasks, err := processSvc.ListTasks(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(tasks)
	})

	protected.Get("/servers/:id/processes/history", requireServerPermission(cfg, store.PermControlStart), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		events, err := processSvc.GetScalingHistory(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(events)
	})

	protected.Post("/servers/:id/processes/parse-procfile", requireServerPermission(cfg, store.PermControlStart), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Content string `json:"content" validate:"required"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		entries, err := process.ParseProcfile(req.Content)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(entries)
	})
}
