package http

import (
	"strconv"

	notificationsvc "gamepanel/forge/internal/services/notification"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerNotificationRoutes(protected fiber.Router, notificationService *notificationsvc.Service) {
	channels := protected.Group("/notification-channels", requireRole("admin"))
	channels.Get("/", handleListNotificationChannels(notificationService))
	channels.Post("/", handleCreateNotificationChannel(notificationService))
	channels.Get("/:id", handleGetNotificationChannel(notificationService))
	channels.Patch("/:id", handleUpdateNotificationChannel(notificationService))
	channels.Delete("/:id", handleDeleteNotificationChannel(notificationService))
	channels.Post("/:id/test", handleTestNotificationChannel(notificationService))

	subs := protected.Group("/notification-channels/:id/subscribe", requireRole("admin"))
	subs.Get("/", handleListSubscriptions(notificationService))
	subs.Post("/", handleCreateSubscription(notificationService))
	subs.Delete("/:subId", handleDeleteSubscription(notificationService))

	protected.Get("/notification-logs", requireRole("admin"), handleListNotificationLogs(notificationService))
}

func handleListNotificationChannels(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		channels, err := svc.ListChannels(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"channels": channels})
	}
}

func handleCreateNotificationChannel(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		var req struct {
			Type    string         `json:"type"`
			Name    string         `json:"name"`
			Config  map[string]any `json:"config"`
			Enabled bool           `json:"enabled"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.Type == "" || req.Name == "" {
			return fiber.NewError(fiber.StatusBadRequest, "type and name are required")
		}
		ch, err := svc.CreateChannel(ctx, store.CreateNotificationChannelRequest{
			Type:    store.NotificationChannelType(req.Type),
			Name:    req.Name,
			Config:  req.Config,
			Enabled: req.Enabled,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(ch)
	}
}

func handleGetNotificationChannel(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		ch, err := svc.GetChannel(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "channel not found")
		}
		return c.JSON(ch)
	}
}

func handleUpdateNotificationChannel(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		var req struct {
			Name    *string         `json:"name"`
			Config  *map[string]any `json:"config"`
			Enabled *bool           `json:"enabled"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ch, err := svc.UpdateChannel(ctx, c.Params("id"), store.UpdateNotificationChannelRequest{
			Name:    req.Name,
			Config:  req.Config,
			Enabled: req.Enabled,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(ch)
	}
}

func handleDeleteNotificationChannel(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		if err := svc.DeleteChannel(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}

func handleTestNotificationChannel(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		if err := svc.TestChannel(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"status": "test message sent"})
	}
}

func handleListSubscriptions(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		subs, err := svc.ListSubscriptions(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"subscriptions": subs})
	}
}

func handleCreateSubscription(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		var req struct {
			EventType string `json:"eventType"`
			Template  string `json:"template"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.EventType == "" {
			return fiber.NewError(fiber.StatusBadRequest, "eventType is required")
		}
		sub, err := svc.CreateSubscription(ctx, c.Params("id"), req.EventType, req.Template)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(sub)
	}
}

func handleDeleteSubscription(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		if err := svc.DeleteSubscription(ctx, c.Params("subId")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}

func handleListNotificationLogs(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		channelID := c.Query("channelId")
		limit, _ := strconv.Atoi(c.Query("limit", "100"))
		offset, _ := strconv.Atoi(c.Query("offset", "0"))

		logs, err := svc.ListLogs(ctx, channelID, limit, offset)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"logs": logs})
	}
}
