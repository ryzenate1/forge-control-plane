package http

import (
	"strconv"
	"time"

	cronjobsvc "gamepanel/forge/internal/services/cronjob"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerCronJobRoutes(protected fiber.Router, cfg Config, cronJobService *cronjobsvc.Service) {
	protected.Get("/cron-jobs", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		jobs, err := cfg.Store.ListCronJobs(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		type jobWithNextRun struct {
			store.CronJob
			NextRun *time.Time `json:"nextRun,omitempty"`
		}
		result := make([]jobWithNextRun, 0, len(jobs))
		for _, j := range jobs {
			next := cronJobService.NextRun(j)
			result = append(result, jobWithNextRun{CronJob: j, NextRun: next})
		}
		return c.JSON(result)
	})

	protected.Post("/cron-jobs", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Name            string `json:"name" validate:"required"`
			Description     string `json:"description"`
			Schedule        string `json:"schedule" validate:"required"`
			Command         string `json:"command" validate:"required"`
			Type            string `json:"type"`
			TargetType      string `json:"targetType"`
			TargetID        string `json:"targetId"`
			Enabled         bool   `json:"enabled"`
			RetryCount      int    `json:"retryCount"`
			TimeoutSeconds  int    `json:"timeoutSeconds"`
			NotifyOnFailure bool   `json:"notifyOnFailure"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.Name == "" || req.Schedule == "" || req.Command == "" {
			return fiber.NewError(fiber.StatusBadRequest, "name, schedule, and command are required")
		}
		if req.Type == "" {
			req.Type = "shell"
		}
		if req.TimeoutSeconds <= 0 {
			req.TimeoutSeconds = 300
		}
		ctx, cancel := requestContext()
		defer cancel()
		job, err := cfg.Store.CreateCronJob(ctx, store.CreateCronJobRequest{
			Name:            req.Name,
			Description:     req.Description,
			Schedule:        req.Schedule,
			Command:         req.Command,
			Type:            req.Type,
			TargetType:      req.TargetType,
			TargetID:        req.TargetID,
			Enabled:         req.Enabled,
			RetryCount:      req.RetryCount,
			TimeoutSeconds:  req.TimeoutSeconds,
			NotifyOnFailure: req.NotifyOnFailure,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if err := cronJobService.RescheduleJob(ctx, job); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(job)
	})

	protected.Get("/cron-jobs/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		job, err := cfg.Store.GetCronJob(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "cron job not found")
		}
		return c.JSON(job)
	})

	protected.Put("/cron-jobs/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Name            *string `json:"name"`
			Description     *string `json:"description"`
			Schedule        *string `json:"schedule"`
			Command         *string `json:"command"`
			Type            *string `json:"type"`
			TargetType      *string `json:"targetType"`
			TargetID        *string `json:"targetId"`
			Enabled         *bool   `json:"enabled"`
			RetryCount      *int    `json:"retryCount"`
			TimeoutSeconds  *int    `json:"timeoutSeconds"`
			NotifyOnFailure *bool   `json:"notifyOnFailure"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		job, err := cfg.Store.UpdateCronJob(ctx, c.Params("id"), store.UpdateCronJobRequest{
			Name:            req.Name,
			Description:     req.Description,
			Schedule:        req.Schedule,
			Command:         req.Command,
			Type:            req.Type,
			TargetType:      req.TargetType,
			TargetID:        req.TargetID,
			Enabled:         req.Enabled,
			RetryCount:      req.RetryCount,
			TimeoutSeconds:  req.TimeoutSeconds,
			NotifyOnFailure: req.NotifyOnFailure,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if err := cronJobService.RescheduleJob(ctx, job); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(job)
	})

	protected.Delete("/cron-jobs/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		cronJobService.RescheduleJob(ctx, store.CronJob{ID: c.Params("id"), Enabled: false})
		if err := cfg.Store.DeleteCronJob(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	protected.Post("/cron-jobs/:id/execute", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		execution, err := cronJobService.TriggerNow(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(execution)
	})

	protected.Post("/cron-jobs/:id/toggle", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		job, err := cfg.Store.ToggleCronJob(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if err := cronJobService.RescheduleJob(ctx, job); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(job)
	})

	protected.Get("/cron-jobs/:id/executions", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		limit, _ := strconv.Atoi(c.Query("limit", "50"))
		executions, err := cfg.Store.ListCronJobExecutions(ctx, c.Params("id"), limit)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(executions)
	})
}
