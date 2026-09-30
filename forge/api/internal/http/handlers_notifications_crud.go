package http

import (
	"context"
	"errors"
	"time"

	notifengine "gamepanel/forge/internal/services/notifications"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// registerNotificationCrudRoutes exposes the notifications engine to users:
// per-user channel management, event subscriptions and the event catalog.
// Admins additionally see and manage global (user_id NULL) and org-wide
// channels; regular users only manage channels they own.
//
// It must be registered BEFORE registerEnhancedNotificationRoutes so the
// user-scoped /notifications/channels list wins over the dormant admin-only
// enhanced registrar.
func registerNotificationCrudRoutes(protected fiber.Router, router *notifengine.Router, mutationLimiter fiber.Handler) {
	if router == nil {
		return
	}
	engine := protected.Group("/notifications")
	engine.Get("/events", handleNotificationEventCatalog(router))

	channels := engine.Group("/channels")
	channels.Get("/", handleListEngineChannels(router))
	channels.Post("/", mutationLimiter, handleCreateEngineChannel(router))
	channels.Post("/test", mutationLimiter, handleTestAllEngineChannels(router))
	channels.Patch("/:id", mutationLimiter, handleUpdateEngineChannel(router))
	channels.Delete("/:id", mutationLimiter, handleDeleteEngineChannel(router))
	channels.Post("/:id/test", mutationLimiter, handleTestEngineChannel(router))

	subs := engine.Group("/subscriptions")
	subs.Get("/", handleListEngineSubscriptions(router))
	subs.Post("/", mutationLimiter, handleCreateEngineSubscription(router))
	subs.Delete("/:id", mutationLimiter, handleDeleteEngineSubscription(router))
}

// engineViewer builds the ownership context from the session claims.
func engineViewer(c *fiber.Ctx) (notifengine.Viewer, error) {
	claims, ok := c.Locals("user").(tokenClaims)
	if !ok || claims.Sub == "" {
		return notifengine.Viewer{}, fiber.NewError(fiber.StatusUnauthorized, "missing session")
	}
	return notifengine.Viewer{UserID: claims.Sub, IsAdmin: claims.Role == RoleAdmin}, nil
}

// engineError maps engine sentinel errors onto HTTP statuses; validation
// failures (bad URLs, missing fields) are the caller's fault and answer 400.
// Anything else is treated as an internal failure and answered honestly via
// respondInternalError.
func engineError(c *fiber.Ctx, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, notifengine.ErrChannelNotFound):
		return fiber.NewError(fiber.StatusNotFound, "notification channel not found")
	case errors.Is(err, notifengine.ErrSubscriptionNotFound):
		return fiber.NewError(fiber.StatusNotFound, "notification subscription not found")
	case errors.Is(err, notifengine.ErrForbidden):
		return fiber.NewError(fiber.StatusForbidden, err.Error())
	default:
		return respondStoreError(c, err)
	}
}

func handleNotificationEventCatalog(router *notifengine.Router) fiber.Handler {
	return func(c *fiber.Ctx) error {
		catalog := router.Catalog()
		return c.JSON(fiber.Map{"events": catalog, "total": len(catalog)})
	}
}

func handleListEngineChannels(router *notifengine.Router) fiber.Handler {
	return func(c *fiber.Ctx) error {
		viewer, err := engineViewer(c)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		channels, err := router.ListChannels(ctx, viewer)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"channels": channels})
	}
}

func handleCreateEngineChannel(router *notifengine.Router) fiber.Handler {
	return func(c *fiber.Ctx) error {
		viewer, err := engineViewer(c)
		if err != nil {
			return err
		}
		var req struct {
			Type    string         `json:"type"`
			Name    string         `json:"name"`
			Config  map[string]any `json:"config"`
			Enabled *bool          `json:"enabled"`
			OrgID   *string        `json:"orgId"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.Type == "" || req.Name == "" {
			return fiber.NewError(fiber.StatusBadRequest, "type and name are required")
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		create := store.CreateNotificationChannelRequest{
			Type:    store.NotificationChannelType(req.Type),
			Name:    req.Name,
			Config:  req.Config,
			Enabled: enabled,
		}
		// Org-wide channels are an admin privilege; regular users always own
		// their channels personally (stamped by the router).
		if viewer.IsAdmin && req.OrgID != nil && *req.OrgID != "" {
			create.OrgID = req.OrgID
		}
		ctx, cancel := requestContext()
		defer cancel()
		ch, err := router.CreateChannel(ctx, viewer, create)
		if err != nil {
			return engineError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(ch)
	}
}

func handleUpdateEngineChannel(router *notifengine.Router) fiber.Handler {
	return func(c *fiber.Ctx) error {
		viewer, err := engineViewer(c)
		if err != nil {
			return err
		}
		var req struct {
			Name    *string         `json:"name"`
			Config  *map[string]any `json:"config"`
			Enabled *bool           `json:"enabled"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		ch, err := router.UpdateChannel(ctx, viewer, c.Params("id"), store.UpdateNotificationChannelRequest{
			Name:    req.Name,
			Config:  req.Config,
			Enabled: req.Enabled,
		})
		if err != nil {
			return engineError(c, err)
		}
		return c.JSON(ch)
	}
}

func handleDeleteEngineChannel(router *notifengine.Router) fiber.Handler {
	return func(c *fiber.Ctx) error {
		viewer, err := engineViewer(c)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := router.DeleteChannel(ctx, viewer, c.Params("id")); err != nil {
			return engineError(c, err)
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}

func handleTestEngineChannel(router *notifengine.Router) fiber.Handler {
	return func(c *fiber.Ctx) error {
		viewer, err := engineViewer(c)
		if err != nil {
			return err
		}
		// Outbound delivery to Discord/Telegram/Slack can legitimately take a
		// few seconds; requestContext()'s 5s budget would flake the test.
		ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
		defer cancel()
		if err := router.TestChannel(ctx, viewer, c.Params("id")); err != nil {
			return engineError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true, "message": "test notification delivered"})
	}
}

// handleTestAllEngineChannels is a convenience for the UI's "test everything"
// affordance: sends a test notification through every enabled channel the
// viewer can see and reports per-channel outcomes without failing the request.
func handleTestAllEngineChannels(router *notifengine.Router) fiber.Handler {
	return func(c *fiber.Ctx) error {
		viewer, err := engineViewer(c)
		if err != nil {
			return err
		}
		listCtx, cancel := requestContext()
		channels, err := router.ListChannels(listCtx, viewer)
		cancel()
		if err != nil {
			return respondInternalError(c, err)
		}
		type result struct {
			ChannelID string `json:"channelId"`
			Name      string `json:"name"`
			OK        bool   `json:"ok"`
			Error     string `json:"error,omitempty"`
		}
		results := []result{}
		for _, ch := range channels {
			if !ch.Enabled {
				continue
			}
			ctx, testCancel := context.WithTimeout(c.Context(), 30*time.Second)
			testErr := router.TestChannel(ctx, viewer, ch.ID)
			testCancel()
			item := result{ChannelID: ch.ID, Name: ch.Name, OK: testErr == nil}
			if testErr != nil {
				item.Error = testErr.Error()
			}
			results = append(results, item)
		}
		return c.JSON(fiber.Map{"results": results})
	}
}

func handleListEngineSubscriptions(router *notifengine.Router) fiber.Handler {
	return func(c *fiber.Ctx) error {
		viewer, err := engineViewer(c)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		subs, err := router.ListSubscriptions(ctx, viewer)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"subscriptions": subs})
	}
}

func handleCreateEngineSubscription(router *notifengine.Router) fiber.Handler {
	return func(c *fiber.Ctx) error {
		viewer, err := engineViewer(c)
		if err != nil {
			return err
		}
		var req struct {
			ChannelID string `json:"channelId"`
			EventType string `json:"eventType"`
			Template  string `json:"template"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.ChannelID == "" || req.EventType == "" {
			return fiber.NewError(fiber.StatusBadRequest, "channelId and eventType are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		sub, err := router.Subscribe(ctx, viewer, req.ChannelID, req.EventType, req.Template)
		if err != nil {
			return engineError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(sub)
	}
}

func handleDeleteEngineSubscription(router *notifengine.Router) fiber.Handler {
	return func(c *fiber.Ctx) error {
		viewer, err := engineViewer(c)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := router.Unsubscribe(ctx, viewer, c.Params("id")); err != nil {
			return engineError(c, err)
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}
