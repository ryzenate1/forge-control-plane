package http

import (
	"strconv"

	notificationsvc "gamepanel/forge/internal/services/notifications"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// registerEnhancedNotificationRoutes registers routes for the enhanced notification system
func registerEnhancedNotificationRoutes(protected fiber.Router, notificationService *notificationsvc.Service) {
	// Alert Rules endpoints
	alerts := protected.Group("/notifications/alerts", requireRole("admin"))
	alerts.Get("/", handleListAlertRules(notificationService))
	alerts.Post("/", handleCreateAlertRule(notificationService))
	alerts.Get("/:id", handleGetAlertRule(notificationService))
	alerts.Patch("/:id", handleUpdateAlertRule(notificationService))
	alerts.Delete("/:id", handleDeleteAlertRule(notificationService))

	// Alert States endpoints
	alerts.Get("/:id/states", handleListAlertStates(notificationService))

	// Notification Preferences endpoints
	prefs := protected.Group("/notifications/preferences", requireAuth())
	prefs.Get("/", handleListNotificationPreferences(notificationService))
	prefs.Post("/", handleCreateNotificationPreference(notificationService))
	prefs.Get("/:id", handleGetNotificationPreference(notificationService))
	prefs.Patch("/:id", handleUpdateNotificationPreference(notificationService))
	prefs.Delete("/:id", handleDeleteNotificationPreference(notificationService))

	// Test notification endpoint
	protected.Post("/notifications/test", requireRole("admin"), handleTestNotification(notificationService))

	// WebSocket notification endpoint
	protected.Get("/notifications/ws", handleNotificationWebSocket(notificationService))

	// Enhanced channels endpoints with tenant support
	channels := protected.Group("/notifications/channels", requireRole("admin"))
	channels.Get("/", handleListNotificationChannelsEnhanced(notificationService))
	channels.Post("/", handleCreateNotificationChannelEnhanced(notificationService))
	channels.Get("/:id", handleGetNotificationChannelEnhanced(notificationService))
	channels.Patch("/:id", handleUpdateNotificationChannelEnhanced(notificationService))
	channels.Delete("/:id", handleDeleteNotificationChannelEnhanced(notificationService))
	channels.Post("/:id/test", handleTestNotificationChannelEnhanced(notificationService))

	// Enhanced logs endpoint
	protected.Get("/notifications/logs", requireRole("admin"), handleListNotificationLogsEnhanced(notificationService))
}

// Alert Rule Handlers

func handleListAlertRules(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		tenantID := c.Query("tenantId")
		userID := c.Query("userId")
		entityType := c.Query("entityType")

		var tenantPtr, userPtr *string
		var entityTypePtr *notificationsvc.EntityType

		if tenantID != "" {
			tenantPtr = &tenantID
		}
		if userID != "" {
			userPtr = &userID
		}
		if entityType != "" {
			et := notificationsvc.EntityType(entityType)
			entityTypePtr = &et
		}

		rules, err := svc.ListAlertRules(ctx, tenantPtr, userPtr, entityTypePtr)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"alertRules": rules})
	}
}

func handleCreateAlertRule(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		var req notificationsvc.CreateAlertRuleRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}

		// Validate required fields
		if req.Name == "" {
			return fiber.NewError(fiber.StatusBadRequest, "name is required")
		}
		if req.RuleType == "" {
			return fiber.NewError(fiber.StatusBadRequest, "ruleType is required")
		}
		if req.EntityType == "" {
			return fiber.NewError(fiber.StatusBadRequest, "entityType is required")
		}

		// Validate threshold rules
		if req.RuleType == notificationsvc.AlertRuleTypeThreshold {
			if req.MetricName == nil || *req.MetricName == "" {
				return fiber.NewError(fiber.StatusBadRequest, "metricName is required for threshold alerts")
			}
			if req.ThresholdValue == nil {
				return fiber.NewError(fiber.StatusBadRequest, "thresholdValue is required for threshold alerts")
			}
			if req.ComparisonOperator == nil || *req.ComparisonOperator == "" {
				return fiber.NewError(fiber.StatusBadRequest, "comparisonOperator is required for threshold alerts")
			}
		}

		// Validate state rules
		if req.RuleType == notificationsvc.AlertRuleTypeState {
			if req.StateValue == nil || *req.StateValue == "" {
				return fiber.NewError(fiber.StatusBadRequest, "stateValue is required for state alerts")
			}
		}

		rule, err := svc.CreateAlertRule(ctx, req)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(rule)
	}
}

func handleGetAlertRule(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		rule, err := svc.GetAlertRule(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "alert rule not found")
		}
		return c.JSON(rule)
	}
}

func handleUpdateAlertRule(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		var req notificationsvc.UpdateAlertRuleRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}

		// Validate threshold rules
		if req.RuleType != nil && *req.RuleType == notificationsvc.AlertRuleTypeThreshold {
			if req.MetricName == nil || *req.MetricName == "" {
				return fiber.NewError(fiber.StatusBadRequest, "metricName is required for threshold alerts")
			}
			if req.ThresholdValue == nil {
				return fiber.NewError(fiber.StatusBadRequest, "thresholdValue is required for threshold alerts")
			}
			if req.ComparisonOperator == nil || *req.ComparisonOperator == "" {
				return fiber.NewError(fiber.StatusBadRequest, "comparisonOperator is required for threshold alerts")
			}
		}

		// Validate state rules
		if req.RuleType != nil && *req.RuleType == notificationsvc.AlertRuleTypeState {
			if req.StateValue == nil || *req.StateValue == "" {
				return fiber.NewError(fiber.StatusBadRequest, "stateValue is required for state alerts")
			}
		}

		rule, err := svc.UpdateAlertRule(ctx, c.Params("id"), req)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(rule)
	}
}

func handleDeleteAlertRule(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		if err := svc.DeleteAlertRule(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}

func handleListAlertStates(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		tenantID := c.Query("tenantId")
		var tenantPtr *string
		if tenantID != "" {
			tenantPtr = &tenantID
		}

		states, err := svc.ListActiveAlertStates(ctx, tenantPtr)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"alertStates": states})
	}
}

// Notification Preference Handlers

func handleListNotificationPreferences(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		userID := getUserIDFromContext(c)
		if userID == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "user not authenticated")
		}

		tenantID := c.Query("tenantId")
		var tenantPtr *string
		if tenantID != "" {
			tenantPtr = &tenantID
		}

		prefs, err := svc.ListNotificationPreferences(ctx, userID, tenantPtr)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"preferences": prefs})
	}
}

func handleCreateNotificationPreference(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		userID := getUserIDFromContext(c)
		if userID == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "user not authenticated")
		}

		var req notificationsvc.CreateNotificationPreferenceRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}

		// Set user ID from context
		req.UserID = userID

		// Validate required fields
		if req.ChannelType == "" {
			return fiber.NewError(fiber.StatusBadRequest, "channelType is required")
		}

		pref, err := svc.CreateNotificationPreference(ctx, req)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(pref)
	}
}

func handleGetNotificationPreference(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		userID := getUserIDFromContext(c)
		if userID == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "user not authenticated")
		}

		tenantID := c.Query("tenantId")
		if tenantID == "" {
			tenantID = "global"
		}

		channelType := c.Query("channelType")
		if channelType == "" {
			return fiber.NewError(fiber.StatusBadRequest, "channelType is required")
		}

		pref, err := svc.GetNotificationPreference(ctx, userID, channelType, tenantID)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "preference not found")
		}
		return c.JSON(pref)
	}
}

func handleUpdateNotificationPreference(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		var req notificationsvc.UpdateNotificationPreferenceRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}

		pref, err := svc.UpdateNotificationPreference(ctx, c.Params("id"), req)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(pref)
	}
}

func handleDeleteNotificationPreference(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		if err := svc.DeleteNotificationPreference(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}

// Test Notification Handler

func handleTestNotification(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		var req struct {
			ChannelID string `json:"channelId"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}

		if req.ChannelID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "channelId is required")
		}

		// Get the channel
		ch, err := svc.GetChannel(ctx, req.ChannelID)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "channel not found")
		}

		// Convert to store.NotificationChannel
		storeChannel := store.NotificationChannel{
			ID:        ch.ID,
			Type:      store.NotificationChannelType(ch.Type),
			Name:      ch.Name,
			Config:    ch.Config,
			Enabled:   ch.Enabled,
			CreatedAt: ch.CreatedAt,
			UpdatedAt: ch.UpdatedAt,
		}

		if err := svc.SendTest(ctx, storeChannel); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		return c.JSON(fiber.Map{"status": "test message sent"})
	}
}

// WebSocket Notification Handler

func handleNotificationWebSocket(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// WebSocket upgrade logic would go here
		// For now, return not implemented as WebSocket support depends on the framework
		return fiber.NewError(fiber.StatusNotImplemented, "WebSocket notifications not yet implemented")
	}
}

// Enhanced Channel Handlers with Tenant Support

func handleListNotificationChannelsEnhanced(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		tenantID := c.Query("tenantId")
		userID := c.Query("userId")

		// The current notification store is not tenant-scoped. Preserve the
		// query parameters in the public contract until tenant filtering is
		// supported end-to-end, rather than constructing unused pointers.
		_ = tenantID
		_ = userID

		// Fall back to the existing method.
		channels, err := svc.ListChannels(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"channels": channels})
	}
}

func handleCreateNotificationChannelEnhanced(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		var req struct {
			TenantID string         `json:"tenantId"`
			UserID   *string        `json:"userId,omitempty"`
			Type     string         `json:"type"`
			Name     string         `json:"name"`
			Config   map[string]any `json:"config"`
			IsActive bool           `json:"isActive"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.Type == "" || req.Name == "" {
			return fiber.NewError(fiber.StatusBadRequest, "type and name are required")
		}

		// Convert to store request
		storeReq := store.CreateNotificationChannelRequest{
			Type:    store.NotificationChannelType(req.Type),
			Name:    req.Name,
			Config:  req.Config,
			Enabled: req.IsActive,
		}

		ch, err := svc.CreateChannel(ctx, storeReq)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(ch)
	}
}

func handleGetNotificationChannelEnhanced(svc *notificationsvc.Service) fiber.Handler {
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

func handleUpdateNotificationChannelEnhanced(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		var req struct {
			Name     *string         `json:"name,omitempty"`
			Config   *map[string]any `json:"config,omitempty"`
			IsActive *bool           `json:"isActive,omitempty"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}

		// Convert to store request
		storeReq := store.UpdateNotificationChannelRequest{
			Name:    req.Name,
			Config:  req.Config,
			Enabled: req.IsActive,
		}

		ch, err := svc.UpdateChannel(ctx, c.Params("id"), storeReq)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(ch)
	}
}

func handleDeleteNotificationChannelEnhanced(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		if err := svc.DeleteChannel(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}

func handleTestNotificationChannelEnhanced(svc *notificationsvc.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		if err := svc.TestChannel(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"status": "test message sent"})
	}
}

func handleListNotificationLogsEnhanced(svc *notificationsvc.Service) fiber.Handler {
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

// Helper function to get user ID from context
func getUserIDFromContext(c *fiber.Ctx) string {
	// This would be implemented based on your authentication middleware
	// For now, return a placeholder
	if user := c.Locals("user"); user != nil {
		if userMap, ok := user.(map[string]interface{}); ok {
			if id, ok := userMap["id"].(string); ok {
				return id
			}
		}
	}
	return ""
}
