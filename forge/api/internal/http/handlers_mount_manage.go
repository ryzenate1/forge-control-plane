package http

import (
	"fmt"

	"errors"
	"strings"

	mountssvc "gamepanel/forge/internal/services/mounts"

	"github.com/gofiber/fiber/v2"
)

// Declarative per-application mount management.
//
// This is deliberately distinct from handlers_files.go, which operates on the
// *host* filesystem through Beacon. These routes manage an application's
// persistent-mount DECLARATIONS (named volumes, bind mounts, tmpfs, seed files)
// that survive redeploys and are injected into the compose document at deploy
// time by mounts.Service.ApplyToCompose.
//
// Layering: handler -> mounts.Service -> store. Authorization (org membership)
// is resolved inside the service so the handler never touches the store.

const appMountsRegistrarName = "app-mount-management"

func init() {
	RegisterPhaseRegistrar(appMountsRegistrarName, 210, registerAppMountRoutesPhase)
}

func registerAppMountRoutesPhase(v1 fiber.Router, protected fiber.Router, cfg *Config) error {
	if cfg.Store == nil {
		return fmt.Errorf("%w: store not configured, app mount routes skipped", ErrPhaseSkipped)
	}
	// Injected via Config (handlers -> mounts.Service -> store); inline New is
	// the dev/test fallback when main has not populated the field.
	svc := cfg.MountService
	if svc == nil {
		svc = mountssvc.New(cfg.Store)
	}
	limiter := RateLimiter(GetRateLimitForEndpoint("mutation", cfg.Redis, cfg.RedisEnabled && strings.EqualFold(strings.TrimSpace(cfg.AppEnv), "production")))
	registerAppMountRoutes(protected, svc, limiter)
	return nil
}

func registerAppMountRoutes(protected fiber.Router, svc *mountssvc.Service, mutationLimiter fiber.Handler) {
	// requireAppAccess enforces that the caller may manage the application's
	// mounts: admins always can, everyone else must belong to the app's org.
	requireAppAccess := func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		allowed, err := svc.CanManageApp(ctx, c.Params("appId"), claims.Sub, claims.Role == "admin")
		if err != nil {
			return respondInternalError(c, err)
		}
		if !allowed {
			return fiber.NewError(fiber.StatusForbidden, "you do not have access to this application")
		}
		return c.Next()
	}

	protected.Get("/apps/:appId/mounts", requireAppAccess, func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		list, err := svc.List(ctx, c.Params("appId"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": list})
	})

	// Validation feedback for the UI (no persistence). Returns 200 with the
	// concrete rule violation so the form can show path/type errors inline.
	protected.Post("/apps/:appId/mounts/validate", requireAppAccess, func(c *fiber.Ctx) error {
		var req mountssvc.CreateRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		req.Type = strings.ToLower(strings.TrimSpace(req.Type))
		if err := mountssvc.Validate(req); err != nil {
			return c.JSON(fiber.Map{"valid": false, "error": err.Error()})
		}
		return c.JSON(fiber.Map{"valid": true})
	})

	protected.Post("/apps/:appId/mounts", mutationLimiter, requireAppAccess, func(c *fiber.Ctx) error {
		var req mountssvc.CreateRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		req.Type = strings.ToLower(strings.TrimSpace(req.Type))
		ctx, cancel := requestContext()
		defer cancel()
		created, err := svc.Create(ctx, c.Params("appId"), req)
		if err != nil {
			return mountError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(created)
	})

	protected.Patch("/apps/:appId/mounts/:mountId", mutationLimiter, requireAppAccess, func(c *fiber.Ctx) error {
		var req mountssvc.UpdateRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		owning, err := svc.Get(ctx, c.Params("mountId"))
		if err != nil {
			return mountError(c, err)
		}
		if owning.ApplicationID != strings.TrimSpace(c.Params("appId")) {
			return fiber.NewError(fiber.StatusNotFound, "mount not found")
		}
		updated, err := svc.Update(ctx, c.Params("mountId"), req)
		if err != nil {
			return mountError(c, err)
		}
		return c.JSON(updated)
	})

	protected.Delete("/apps/:appId/mounts/:mountId", mutationLimiter, requireAppAccess, func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		owning, err := svc.Get(ctx, c.Params("mountId"))
		if err != nil {
			return mountError(c, err)
		}
		if owning.ApplicationID != strings.TrimSpace(c.Params("appId")) {
			return fiber.NewError(fiber.StatusNotFound, "mount not found")
		}
		if err := svc.Delete(ctx, c.Params("mountId")); err != nil {
			return mountError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})
}

// mountError maps mount service sentinel errors onto honest HTTP statuses.
func mountError(c *fiber.Ctx, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, mountssvc.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, mountssvc.ErrInvalidType),
		errors.Is(err, mountssvc.ErrMissingName),
		errors.Is(err, mountssvc.ErrMissingTarget),
		errors.Is(err, mountssvc.ErrBadPath),
		errors.Is(err, mountssvc.ErrDangerousTarget),
		errors.Is(err, mountssvc.ErrBindSource),
		errors.Is(err, mountssvc.ErrVolumeSource),
		errors.Is(err, mountssvc.ErrSeedContent),
		errors.Is(err, mountssvc.ErrSeedTooLarge),
		errors.Is(err, mountssvc.ErrEmptyAppID):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	default:
		// Unique-target violations and other store messages are user-fixable
		// input problems, so surface them as 400 rather than 500.
		if isMountConflict(err) {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return respondInternalError(c, err)
	}
}

func isMountConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already targets") || strings.Contains(msg, "unique")
}
