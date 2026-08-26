package http

import (
	"encoding/json"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// mergeJSONConfig overlays the keys present in body onto current (marshalled to
// JSON first) and decodes the result into out. PUT handlers use it so a partial
// body updates only the fields it names: decoding straight into the store struct
// would upsert zero values for every omitted field and silently wipe settings
// such as listen ports and connection limits.
func mergeJSONConfig(current any, body []byte, out any) error {
	raw, err := json.Marshal(current)
	if err != nil {
		return err
	}
	merged := map[string]any{}
	if err := json.Unmarshal(raw, &merged); err != nil {
		return err
	}
	if len(body) > 0 {
		patch := map[string]any{}
		if err := json.Unmarshal(body, &patch); err != nil {
			return err
		}
		for k, v := range patch {
			merged[k] = v
		}
	}
	round, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	return json.Unmarshal(round, out)
}

func registerSFTPRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	admin := protected.Group("/admin", requireRole("admin"))

	admin.Get("/sftp/settings", requireAdminScope("settings.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		settings, err := cfg.Store.GetSFTPGlobalConfig(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "SFTP settings not found")
		}
		return c.JSON(settings)
	})

	admin.Put("/sftp/settings", mutationLimiter, requireAdminScope("settings.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		current, err := cfg.Store.GetSFTPGlobalConfig(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		var req store.SFTPGlobalConfig
		if err := mergeJSONConfig(current, c.Body(), &req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := cfg.Store.UpdateSFTPGlobalConfig(ctx, req); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	admin.Get("/nodes/:nodeId/sftp", requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		config, err := cfg.Store.GetSFTPNodeConfig(ctx, c.Params("nodeId"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "node SFTP config not found")
		}
		return c.JSON(config)
	})

	admin.Put("/nodes/:nodeId/sftp", mutationLimiter, requireAdminScope("nodes.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		current, err := cfg.Store.GetSFTPNodeConfig(ctx, c.Params("nodeId"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		var req store.SFTPNodeConfig
		if err := mergeJSONConfig(current, c.Body(), &req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		req.NodeID = c.Params("nodeId")
		if err := cfg.Store.UpdateSFTPNodeConfig(ctx, req); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	admin.Get("/sftp/nodes", requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		configs, err := cfg.Store.ListSFTPNodeConfigs(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(configs)
	})
}
