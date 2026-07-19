package http

import (
	"time"

	"gamepanel/forge/internal/services/webauthn"

	"github.com/gofiber/fiber/v2"
)

func registerWebAuthnRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler, wa *webauthn.Service) {
	if wa == nil {
		return
	}

	protected.Post("/auth/webauthn/register/begin", mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()

		user, err := cfg.Store.GetUserByID(ctx, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "user not found")
		}

		creation, sessionID, err := wa.BeginRegistration(ctx, user.ID, user.Email, user.Email)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		return c.JSON(fiber.Map{
			"creation":  creation,
			"sessionId": sessionID,
		})
	})

	protected.Post("/auth/webauthn/register/finish", mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}

		var req struct {
			SessionID string `json:"sessionId"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.SessionID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "sessionId is required")
		}

		ctx, cancel := requestContext()
		defer cancel()

		_, err := wa.FinishRegistration(ctx, req.SessionID, claims.Sub, c.Body())
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}

		return c.JSON(fiber.Map{"status": "ok"})
	})

	protected.Post("/auth/webauthn/login/begin", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()

		assertion, sessionID, err := wa.BeginLogin(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		return c.JSON(fiber.Map{
			"assertion": assertion,
			"sessionId": sessionID,
		})
	})

	protected.Post("/auth/webauthn/login/finish", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}

		var req struct {
			SessionID string `json:"sessionId"`
			UserID    string `json:"userId"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.SessionID == "" || req.UserID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "sessionId and userId are required")
		}

		ctx, cancel := requestContext()
		defer cancel()

		_, err := wa.FinishLogin(ctx, req.SessionID, req.UserID, c.Body())
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, err.Error())
		}

		user, err := cfg.Store.GetUserByID(ctx, req.UserID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "user not found")
		}

		token, err := issueToken(cfg.AuthSecret, user)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not issue token")
		}

		csrfToken, _ := generateCSRFToken()
		expires := time.Now().Add(tokenTTL)
		setSessionCookies(c, token, csrfToken, expires)

		return c.JSON(fiber.Map{
			"complete": true,
			"token":    token,
			"user":     user,
		})
	})

	protected.Get("/account/webauthn/credentials", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()

		creds, err := wa.ListCredentials(ctx, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}

		type credResp struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			CreatedAt string `json:"createdAt"`
			LastUsed  string `json:"lastUsed"`
		}
		resp := make([]credResp, 0, len(creds))
		for _, c := range creds {
			resp = append(resp, credResp{
				ID:        c.ID,
				Name:      c.Name,
				CreatedAt: c.CreatedAt.Format("2006-01-02T15:04:05Z"),
				LastUsed:  c.LastUsedAt.Format("2006-01-02T15:04:05Z"),
			})
		}
		return c.JSON(resp)
	})

	protected.Delete("/account/webauthn/credentials/:id", mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()

		if err := wa.RemoveCredential(ctx, claims.Sub, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}

		return c.SendStatus(fiber.StatusNoContent)
	})
}
