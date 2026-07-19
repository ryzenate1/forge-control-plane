package http

import (
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
)

type SetupStatus struct {
	Required   bool   `json:"required"`
	HasAdmin   bool   `json:"hasAdmin"`
	AppVersion string `json:"appVersion"`
}

type SetupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

func registerSetupRoutes(public fiber.Router, cfg Config, authLimiter fiber.Handler) {
	public.Get("/setup/status", func(c *fiber.Ctx) error {
		status := SetupStatus{Required: false, HasAdmin: false, AppVersion: "0.1.0"}
		if cfg.Store == nil {
			return c.JSON(status)
		}
		ctx, cancel := requestContext()
		defer cancel()
		has, err := cfg.Store.HasAnyAdmin(ctx)
		if err != nil {
			return c.JSON(status)
		}
		status.HasAdmin = has
		status.Required = !has
		return c.JSON(status)
	})

	public.Post("/setup", authLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required for setup")
		}
		var req SetupRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.Email == "" || req.Password == "" {
			return fiber.NewError(fiber.StatusBadRequest, "email and password are required")
		}
		if err := store.ValidatePassword(req.Password); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		ctx, cancel := requestContext()
		defer cancel()
		has, err := cfg.Store.HasAnyAdmin(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if has {
			return fiber.NewError(fiber.StatusForbidden, "setup already completed")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		user, err := cfg.Store.CreateSetupAdmin(ctx, req.Email, string(hash))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "userId": user.ID, "email": user.Email})
	})
}
