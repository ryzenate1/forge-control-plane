package http

import (
	"io"

	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerSchedulesServerRoutes(protected fiber.Router, cfg Config, runner *scheduleRunner, clusterManager *clustermanager.Service, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {

	protected.Get("/servers/:id/schedules", requireServerPermission(cfg, store.PermScheduleRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		schedules, err := cfg.Store.ListSchedules(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(schedules)
	})

	protected.Get("/servers/:id/schedules/:scheduleId", requireServerPermission(cfg, store.PermScheduleRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		schedule, err := cfg.Store.GetSchedule(ctx, c.Params("id"), c.Params("scheduleId"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "schedule not found")
		}
		return c.JSON(schedule)
	})

	protected.Post("/servers/:id/schedules", requireServerPermission(cfg, store.PermScheduleCreate), func(c *fiber.Ctx) error {
		var req CreateScheduleRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		// Enforce user-level schedule cap.
		if ownerID, ok := serverOwner(ctx, cfg, c.Params("id")); ok {
			if err := cfg.Store.CheckUserCanCreateSchedule(ctx, ownerID); err != nil {
				if store.IsUserLimitError(err) {
					return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
				}
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		schedule, err := cfg.Store.CreateSchedule(ctx, c.Params("id"), store.CreateScheduleRequest{
			Name:           req.Name,
			CronMinute:     req.CronMinute,
			CronHour:       req.CronHour,
			CronDayOfMonth: req.CronDayOfMonth,
			CronMonth:      req.CronMonth,
			CronDayOfWeek:  req.CronDayOfWeek,
			OnlyWhenOnline: req.OnlyWhenOnline,
			Enabled:        req.Enabled,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(schedule)
	})

	protected.Patch("/servers/:id/schedules/:scheduleId", requireServerPermission(cfg, store.PermScheduleUpdate), func(c *fiber.Ctx) error {
		var req PatchScheduleRequest
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
		schedule, err := cfg.Store.PatchSchedule(ctx, c.Params("id"), c.Params("scheduleId"), store.PatchScheduleRequest{
			Name:           req.Name,
			CronMinute:     req.CronMinute,
			CronHour:       req.CronHour,
			CronDayOfMonth: req.CronDayOfMonth,
			CronMonth:      req.CronMonth,
			CronDayOfWeek:  req.CronDayOfWeek,
			OnlyWhenOnline: req.OnlyWhenOnline,
			Enabled:        req.Enabled,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(schedule)
	})

	protected.Delete("/servers/:id/schedules/:scheduleId", requireServerPermission(cfg, store.PermScheduleDelete), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.DeleteSchedule(ctx, c.Params("id"), c.Params("scheduleId"), actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/schedules/:scheduleId/tasks", requireServerPermission(cfg, store.PermScheduleUpdate), func(c *fiber.Ctx) error {
		var req CreateScheduleTaskRequest
		if err := c.BodyParser(&req); err != nil {
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
		task, err := cfg.Store.CreateScheduleTask(ctx, c.Params("id"), c.Params("scheduleId"), store.CreateScheduleTaskRequest{
			Sequence:          req.Sequence,
			Action:            req.Action,
			Payload:           req.Payload,
			TimeOffsetSeconds: req.TimeOffsetSeconds,
			ContinueOnFailure: req.ContinueOnFailure,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(task)
	})

	protected.Post("/servers/:id/schedules/:scheduleId/run", requireServerPermission(cfg, store.PermScheduleUpdate), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := runner.RunNow(ctx, c.Params("id"), c.Params("scheduleId")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"ok": true})
	})

	protected.Get("/servers/:id/schedules/:scheduleId/runs", requireServerPermission(cfg, store.PermScheduleRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		runs, err := cfg.Store.ListScheduleRuns(ctx, c.Params("id"), c.Params("scheduleId"), 20)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(runs)
	})

	protected.Patch("/servers/:id/schedules/:scheduleId/tasks/:taskId", requireServerPermission(cfg, store.PermScheduleUpdate), func(c *fiber.Ctx) error {
		var req PatchScheduleTaskRequest
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
		task, err := cfg.Store.PatchScheduleTask(ctx, c.Params("id"), c.Params("scheduleId"), c.Params("taskId"), store.PatchScheduleTaskRequest{
			Sequence:          req.Sequence,
			Action:            req.Action,
			Payload:           req.Payload,
			TimeOffsetSeconds: req.TimeOffsetSeconds,
			ContinueOnFailure: req.ContinueOnFailure,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(task)
	})

	protected.Delete("/servers/:id/schedules/:scheduleId/tasks/:taskId", requireServerPermission(cfg, store.PermScheduleDelete), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.DeleteScheduleTask(ctx, c.Params("id"), c.Params("scheduleId"), c.Params("taskId"), actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Get("/servers/:id/schedules/:scheduleId/tasks", requireServerPermission(cfg, store.PermScheduleRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		tasks, err := cfg.Store.ListScheduleTasks(ctx, c.Params("id"), c.Params("scheduleId"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(tasks)
	})
}
