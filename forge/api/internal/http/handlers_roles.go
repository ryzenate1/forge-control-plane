package http

import (
	"strings"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// ---- Roles ----

// Role CRUD with assign/remove endpoints.

func ListRoles(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		roles, err := cfg.Store.ListRoles(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(roles)
	}
}

func GetRole(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		role, err := cfg.Store.GetRole(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.JSON(role)
	}
}

func CreateRole(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Key     string `json:"key"`
			Name    string `json:"name"`
			IsAdmin bool   `json:"isAdmin"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		role, err := cfg.Store.CreateRole(ctx, strings.TrimSpace(req.Key), strings.TrimSpace(req.Name), req.IsAdmin)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(role)
	}
}

func UpdateRole(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Key     string `json:"key"`
			Name    string `json:"name"`
			IsAdmin bool   `json:"isAdmin"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		role, err := cfg.Store.UpdateRole(ctx, c.Params("id"), strings.TrimSpace(req.Key), strings.TrimSpace(req.Name), req.IsAdmin)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(role)
	}
}

func DeleteRole(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteRole(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	}
}

func AssignRolesToUser(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		// Defense in depth: even though the route is registered behind
		// requireRole("admin"), enforce the same check here so this handler
		// remains safe if it is ever reused behind different middleware.
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		if claims.Role != "admin" {
			return fiber.NewError(fiber.StatusForbidden, "insufficient role")
		}
		targetID := c.Params("id")
		// Prevent privilege self-escalation: a caller must not be able to
		// change their own role assignments through this endpoint.
		if targetID == claims.Sub {
			return fiber.NewError(fiber.StatusForbidden, "cannot modify your own roles")
		}
		var req struct {
			Roles []string `json:"roles"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		// Prevent granting privileges the caller does not themselves hold:
		// only a full admin (already enforced above) may assign roles that
		// are flagged as IsAdmin, so re-verify against the current role set.
		allRoles, err := cfg.Store.ListRoles(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		isAdminRole := make(map[string]bool, len(allRoles))
		for _, role := range allRoles {
			isAdminRole[role.Key] = role.IsAdmin
		}
		for _, r := range req.Roles {
			if isAdminRole[r] && claims.Role != "admin" {
				return fiber.NewError(fiber.StatusForbidden, "cannot assign an admin role")
			}
			if err := cfg.Store.AssignRole(ctx, targetID, r); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, err.Error())
			}
		}
		meta := map[string]string{"roles": strings.Join(req.Roles, ",")}
		_ = cfg.Store.AppendAudit(ctx, &claims.Sub, "admin.roles.assigned", "user", &targetID, safeAuditMeta(meta))
		return c.JSON(fiber.Map{"ok": true, "roles": req.Roles})
	}
}

func RemoveRolesFromUser(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		// Defense in depth: mirror the admin-only, no-self-modification checks
		// applied in AssignRolesToUser so this endpoint cannot be used to
		// strip roles without proper authorization.
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		if claims.Role != "admin" {
			return fiber.NewError(fiber.StatusForbidden, "insufficient role")
		}
		targetID := c.Params("id")
		if targetID == claims.Sub {
			return fiber.NewError(fiber.StatusForbidden, "cannot modify your own roles")
		}
		var req struct {
			Roles []string `json:"roles"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		for _, r := range req.Roles {
			if err := cfg.Store.RemoveRole(ctx, targetID, r); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, err.Error())
			}
		}
		meta := map[string]string{"roles": strings.Join(req.Roles, ",")}
		_ = cfg.Store.AppendAudit(ctx, &claims.Sub, "admin.roles.removed", "user", &targetID, safeAuditMeta(meta))
		return c.JSON(fiber.Map{"ok": true, "roles": req.Roles})
	}
}

func ListUserRoles(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		roles, err := cfg.Store.UserRoles(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(roles)
	}
}

// _ silences unused-import warnings on minimal builds.
var _ = store.AdminScopes
