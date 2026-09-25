package http

import (
	"context"
	"errors"
	"strconv"
	"time"

	scheduledtaskssvc "gamepanel/forge/internal/services/scheduledtasks"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// registerScheduledTaskRoutes exposes per-server "Scheduled Tasks" (per-app
// cron) under /api/v1/servers/:id/tasks. The path parameter is named :id so
// the shared requireServerPermission middleware (which resolves the server
// from c.Params("id"), see auth.go) guards every route; the public URL shape
// is unchanged (/servers/<serverId>/tasks). Handlers only talk to the
// scheduledtasks service — no direct store access.
func registerScheduledTaskRoutes(protected fiber.Router, cfg Config, svc *scheduledtaskssvc.Service, mutationLimiter fiber.Handler) {
	serverTasks := "/servers/:id/tasks"

	// ---- collection ----

	protected.Get(serverTasks, requireServerPermission(cfg, store.PermScheduleRead), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		tasks, err := svc.ListTasks(ctx, c.Params("id"))
		if err != nil {
			return scheduledTasksError(c, err)
		}
		return c.JSON(tasks)
	})

	protected.Post(serverTasks, mutationLimiter, requireServerPermission(cfg, store.PermScheduleCreate), func(c *fiber.Ctx) error {
		var req struct {
			Name     string `json:"name"`
			Command  string `json:"command"`
			Schedule string `json:"schedule"`
			Enabled  *bool  `json:"enabled"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		var createdBy string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			createdBy = claims.Sub
		}
		ctx, cancel := requestContext()
		defer cancel()
		task, err := svc.CreateTask(ctx, scheduledtaskssvc.CreateInput{
			ServerID:  c.Params("id"),
			Name:      req.Name,
			Command:   req.Command,
			Schedule:  req.Schedule,
			Enabled:   req.Enabled,
			CreatedBy: createdBy,
		})
		if err != nil {
			return scheduledTasksError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(task)
	})

	// ---- single task ----

	protected.Get(serverTasks+"/:taskId", requireServerPermission(cfg, store.PermScheduleRead), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		task, err := svc.GetTaskForServer(ctx, c.Params("id"), c.Params("taskId"))
		if err != nil {
			return scheduledTasksError(c, err)
		}
		return c.JSON(task)
	})

	protected.Patch(serverTasks+"/:taskId", mutationLimiter, requireServerPermission(cfg, store.PermScheduleUpdate), func(c *fiber.Ctx) error {
		// Pointer fields keep PATCH semantics absent => unchanged.
		var req struct {
			Name     *string `json:"name"`
			Command  *string `json:"command"`
			Schedule *string `json:"schedule"`
			Enabled  *bool   `json:"enabled"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		task, err := svc.UpdateTask(ctx, c.Params("id"), c.Params("taskId"), scheduledtaskssvc.UpdateInput{
			Name:     req.Name,
			Command:  req.Command,
			Schedule: req.Schedule,
			Enabled:  req.Enabled,
		})
		if err != nil {
			return scheduledTasksError(c, err)
		}
		return c.JSON(task)
	})

	protected.Delete(serverTasks+"/:taskId", mutationLimiter, requireServerPermission(cfg, store.PermScheduleDelete), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.DeleteTask(ctx, c.Params("id"), c.Params("taskId")); err != nil {
			return scheduledTasksError(c, err)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	// ---- execution ----

	protected.Post(serverTasks+"/:taskId/run", mutationLimiter, requireServerPermission(cfg, store.PermScheduleUpdate), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		task, err := svc.GetTaskForServer(ctx, c.Params("id"), c.Params("taskId"))
		cancel()
		if err != nil {
			return scheduledTasksError(c, err)
		}
		// Manual trigger runs synchronously so the caller sees the real
		// outcome; it must outlive the 5s request context, so it gets its own
		// bounded background context (the service caps the Beacon round-trip).
		runCtx, runCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer runCancel()
		run, err := svc.RunTask(runCtx, task.ID)
		if err != nil {
			return scheduledTasksError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(run)
	})

	protected.Get(serverTasks+"/:taskId/runs", requireServerPermission(cfg, store.PermScheduleRead), func(c *fiber.Ctx) error {
		limit, err := strconv.Atoi(c.Query("limit", "50"))
		if err != nil || limit <= 0 {
			limit = 50
		}
		ctx, cancel := requestContext()
		defer cancel()
		runs, err := svc.ListRuns(ctx, c.Params("id"), c.Params("taskId"), limit)
		if err != nil {
			return scheduledTasksError(c, err)
		}
		return c.JSON(runs)
	})
}

// scheduledTasksError maps service sentinel errors onto HTTP statuses.
// Anything unrecognized becomes a logged 500 with a generic client message
// (never echo raw internal errors to callers).
func scheduledTasksError(c *fiber.Ctx, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, scheduledtaskssvc.ErrValidation):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	case errors.Is(err, scheduledtaskssvc.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, scheduledtaskssvc.ErrDispatch):
		// The run record was persisted as failed; surface it as a bad gateway
		// so the UI can distinguish "beacon unreachable" from a 500.
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	default:
		return respondInternalError(c, err)
	}
}
