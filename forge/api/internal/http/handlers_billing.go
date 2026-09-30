package http

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	billingsvc "gamepanel/forge/internal/services/billing"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// registerBillingRoutes wires the Phase 7 commerce v1 surface that the admin
// UI (forge/web/lib/api/billing.ts) already calls: plans CRUD, per-org quota
// and usage views, settings, and the org-plan assignment mutation. Plan/quota
// CRUD goes straight through cfg.Store (matching the codebase convention),
// while usage metering, quota enforcement, and webhook verification flow
// through cfg.BillingService so the same invariants apply as internal callers.
//
// The public webhook receiver is registered on the v1 group via
// registerBillingWebhookRoute so it does not sit behind the auth middleware.
func registerBillingRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	svc := cfg.BillingService
	if svc == nil || cfg.Store == nil {
		return
	}

	billing := protected.Group("/billing", requireRole("admin"))

	billing.Get("/plans", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		plans, err := cfg.Store.ListBillingPlans(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		if plans == nil {
			plans = []store.BillingPlan{}
		}
		return c.JSON(fiber.Map{"data": plans})
	})

	billing.Get("/plans/:code", func(c *fiber.Ctx) error {
		code := strings.TrimSpace(c.Params("code"))
		if code == "" {
			return fiber.NewError(fiber.StatusBadRequest, "plan code is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		plan, err := cfg.Store.GetBillingPlanByCode(ctx, code)
		if err != nil {
			return mapBillingErr(err)
		}
		if plan == nil {
			return fiber.NewError(fiber.StatusNotFound, "plan not found")
		}
		return c.JSON(fiber.Map{"data": plan})
	})

	billing.Post("/plans", mutationLimiter, func(c *fiber.Ctx) error {
		var req struct {
			Code          string          `json:"code"`
			Name          string          `json:"name"`
			CentsPerMonth int64           `json:"centsPerMonth"`
			Entitlements  json.RawMessage `json:"entitlements"`
			TrialDays     int             `json:"trialDays"`
			Active        *bool           `json:"active"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		req.Code = strings.TrimSpace(req.Code)
		req.Name = strings.TrimSpace(req.Name)
		if req.Code == "" || req.Name == "" {
			return fiber.NewError(fiber.StatusBadRequest, "code and name are required")
		}
		if req.CentsPerMonth < 0 || req.TrialDays < 0 {
			return fiber.NewError(fiber.StatusBadRequest, "centsPerMonth and trialDays must be >= 0")
		}
		if len(req.Entitlements) == 0 {
			req.Entitlements = json.RawMessage("{}")
		}
		active := true
		if req.Active != nil {
			active = *req.Active
		}
		ctx, cancel := requestContext()
		defer cancel()
		plan, err := cfg.Store.CreateBillingPlan(ctx, store.BillingPlan{
			Code:          req.Code,
			Name:          req.Name,
			CentsPerMonth: req.CentsPerMonth,
			Entitlements:  req.Entitlements,
			TrialDays:     req.TrialDays,
			Active:        active,
		})
		if err != nil {
			return mapBillingErr(err)
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": plan})
	})

	billing.Put("/plans/:id", mutationLimiter, func(c *fiber.Ctx) error {
		id := strings.TrimSpace(c.Params("id"))
		if id == "" {
			return fiber.NewError(fiber.StatusBadRequest, "plan id is required")
		}
		var req struct {
			Name          *string          `json:"name"`
			CentsPerMonth *int64           `json:"centsPerMonth"`
			Entitlements  *json.RawMessage `json:"entitlements"`
			TrialDays     *int             `json:"trialDays"`
			Active        *bool            `json:"active"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.Name != nil {
			name := strings.TrimSpace(*req.Name)
			req.Name = &name
		}
		var raw json.RawMessage
		if req.Entitlements != nil {
			raw = *req.Entitlements
		}
		ctx, cancel := requestContext()
		defer cancel()
		plan, err := cfg.Store.UpdateBillingPlan(ctx, id, derefString(req.Name), req.CentsPerMonth, raw, req.TrialDays, req.Active)
		if err != nil {
			return mapBillingErr(err)
		}
		return c.JSON(fiber.Map{"data": plan})
	})

	billing.Delete("/plans/:id", mutationLimiter, func(c *fiber.Ctx) error {
		id := strings.TrimSpace(c.Params("id"))
		if id == "" {
			return fiber.NewError(fiber.StatusBadRequest, "plan id is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteBillingPlan(ctx, id); err != nil {
			return mapBillingErr(err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	billing.Get("/org/:orgId/quota", func(c *fiber.Ctx) error {
		orgID := strings.TrimSpace(c.Params("orgId"))
		if orgID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "orgId is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		quota, err := cfg.Store.EnsureOrgQuota(ctx, orgID)
		if err != nil {
			return mapBillingErr(err)
		}
		return c.JSON(fiber.Map{"data": quota})
	})

	billing.Post("/org/:orgId/plan", mutationLimiter, func(c *fiber.Ctx) error {
		orgID := strings.TrimSpace(c.Params("orgId"))
		if orgID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "orgId is required")
		}
		var req struct {
			PlanCode string `json:"planCode"`
			Trial    bool   `json:"trial"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		req.PlanCode = strings.TrimSpace(req.PlanCode)
		if req.PlanCode == "" {
			return fiber.NewError(fiber.StatusBadRequest, "planCode is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if _, err := svc.SetOrgPlan(ctx, orgID, req.PlanCode, req.Trial); err != nil {
			return mapBillingErr(err)
		}
		quota, err := cfg.Store.GetOrgQuota(ctx, orgID)
		if err != nil {
			return mapBillingErr(err)
		}
		return c.JSON(fiber.Map{"data": quota})
	})

	billing.Get("/org/:orgId/usage", func(c *fiber.Ctx) error {
		orgID := strings.TrimSpace(c.Params("orgId"))
		if orgID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "orgId is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		usage, err := svc.GetOrgUsage(ctx, orgID)
		if err != nil {
			return mapBillingErr(err)
		}
		return c.JSON(fiber.Map{"data": usage})
	})

	billing.Get("/org/:orgId/usage-events", func(c *fiber.Ctx) error {
		orgID := strings.TrimSpace(c.Params("orgId"))
		if orgID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "orgId is required")
		}
		since := time.Now().Add(-24 * time.Hour)
		if raw := strings.TrimSpace(c.Query("since")); raw != "" {
			parsed, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				return fiber.NewError(fiber.StatusBadRequest, "since must be RFC3339")
			}
			since = parsed
		}
		limit := 100
		if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 || n > 5000 {
				return fiber.NewError(fiber.StatusBadRequest, "limit must be 0..5000")
			}
			limit = n
		}
		ctx, cancel := requestContext()
		defer cancel()
		events, err := cfg.Store.ListUsageEvents(ctx, orgID, since, limit)
		if err != nil {
			return mapBillingErr(err)
		}
		if events == nil {
			events = []store.UsageEvent{}
		}
		return c.JSON(fiber.Map{"data": events})
	})

	billing.Post("/usage", mutationLimiter, func(c *fiber.Ctx) error {
		var req struct {
			OrgID    string `json:"orgId"`
			Resource string `json:"resource"`
			Quantity int64  `json:"quantity"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		req.OrgID = strings.TrimSpace(req.OrgID)
		req.Resource = strings.TrimSpace(req.Resource)
		if req.OrgID == "" || req.Resource == "" {
			return fiber.NewError(fiber.StatusBadRequest, "orgId and resource are required")
		}
		if req.Quantity <= 0 {
			return fiber.NewError(fiber.StatusBadRequest, "quantity must be > 0")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.RecordUsage(ctx, req.OrgID, req.Resource, req.Quantity); err != nil {
			return mapBillingErr(err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	billing.Get("/settings", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		settings, err := cfg.Store.GetBillingSettings(ctx)
		if err != nil {
			return mapBillingErr(err)
		}
		return c.JSON(fiber.Map{"data": settings})
	})

	billing.Put("/settings", mutationLimiter, func(c *fiber.Ctx) error {
		var req struct {
			WebhookSecret      *string `json:"webhookSecret"`
			ExternalProcessor  *string `json:"externalProcessor"`
			ClearWebhookSecret bool    `json:"clearWebhookSecret"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		secret := ""
		if req.WebhookSecret != nil {
			secret = *req.WebhookSecret
		}
		if req.ClearWebhookSecret {
			secret = ""
		}
		processor := ""
		if req.ExternalProcessor != nil {
			processor = strings.TrimSpace(*req.ExternalProcessor)
		}
		ctx, cancel := requestContext()
		defer cancel()
		settings, err := cfg.Store.UpdateBillingSettings(ctx, secret, processor)
		if err != nil {
			return mapBillingErr(err)
		}
		return c.JSON(fiber.Map{"data": settings})
	})
}

// registerBillingWebhookRoute attaches the public HMAC-verified webhook
// receiver on the v1 group (outside the auth middleware) so external payment
// processors can call it without a session cookie.
func registerBillingWebhookRoute(v1 fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	svc := cfg.BillingService
	if svc == nil {
		return
	}
	v1.Post("/billing/webhook", mutationLimiter, func(c *fiber.Ctx) error {
		body := c.Body()
		if len(body) == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "empty body")
		}
		sig := c.Get("X-Billing-Signature")
		if sig == "" {
			sig = c.Get("X-Signature")
		}
		if sig == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "missing signature header")
		}
		ctx, cancel := requestContext()
		defer cancel()
		ok, err := svc.VerifyWebhookSignature(ctx, body, sig)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid signature")
		}
		eventID := strings.TrimSpace(c.Get("X-Billing-Event-Id"))
		if eventID == "" {
			eventID = strings.TrimSpace(c.Get("X-Event-Id"))
		}
		if eventID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "missing event id header")
		}
		provider := strings.TrimSpace(c.Get("X-Billing-Provider"))
		if provider == "" {
			provider = "generic"
		}
		eventType := strings.TrimSpace(c.Get("X-Billing-Event-Type"))
		if eventType == "" {
			eventType = "unknown"
		}
		receipt, perr := svc.ProcessWebhook(ctx, provider, eventID, eventType, body)
		if perr != nil {
			return fiber.NewError(fiber.StatusInternalServerError, perr.Error())
		}
		return c.JSON(fiber.Map{"data": receipt})
	})
}

// mapBillingErr translates billing/store sentinel errors into Fiber errors with
// sensible HTTP status codes. Quota rejections map to 422 (parity with
// per-user-limit errors); unclassified store errors fall through by substring.
func mapBillingErr(err error) error {
	if err == nil {
		return nil
	}
	if billingsvc.IsQuotaExceeded(err) {
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "does not exist"), strings.Contains(low, "not found"):
		return fiber.NewError(fiber.StatusNotFound, msg)
	case strings.Contains(low, "already exists"), strings.Contains(low, "duplicate"):
		return fiber.NewError(fiber.StatusConflict, msg)
	case strings.Contains(low, "required"), strings.Contains(low, "invalid"):
		return fiber.NewError(fiber.StatusBadRequest, msg)
	}
	return fiber.NewError(fiber.StatusInternalServerError, msg)
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
