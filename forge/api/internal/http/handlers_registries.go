package http

import (
	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// registerRegistryRoutes exposes private Docker registry credentials so Forge
// can pull/push images from authenticated registries. Credentials are stored
// encrypted; list/get return them masked, and only deploy-time resolution reads
// plaintext server-side. POST /registries/:id/verify performs a real login on a
// node using the existing daemon LoginRegistry path.
func registerRegistryRoutes(protected fiber.Router, cfg Config, adminIPAccess, mutationLimiter fiber.Handler) {
	if cfg.Store == nil {
		return
	}
	reg := protected.Group("/registries", adminIPAccess, requireRole("admin"))

	reg.Get("/", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		items, err := cfg.Store.ListDockerRegistries(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": items})
	})

	reg.Post("/", mutationLimiter, requireAdminScope("settings.write"), func(c *fiber.Ctx) error {
		var req struct {
			Name          string `json:"name"`
			ServerAddress string `json:"serverAddress"`
			Username      string `json:"username"`
			Credential    string `json:"credential"`
			Email         string `json:"email"`
			IsGlobal      bool   `json:"isGlobal"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		claims, _ := c.Locals("user").(tokenClaims)
		ctx, cancel := requestContext()
		defer cancel()
		created, err := cfg.Store.CreateDockerRegistry(ctx, store.CreateDockerRegistryRequest{
			UserID:        claims.Sub,
			Name:          req.Name,
			ServerAddress: req.ServerAddress,
			Username:      req.Username,
			Credential:    req.Credential,
			Email:         req.Email,
			IsGlobal:      req.IsGlobal,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(created)
	})

	reg.Get("/:id", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		item, err := cfg.Store.GetDockerRegistry(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.JSON(item)
	})

	reg.Put("/:id", mutationLimiter, requireAdminScope("settings.write"), func(c *fiber.Ctx) error {
		var req struct {
			Name          string `json:"name"`
			ServerAddress string `json:"serverAddress"`
			Username      string `json:"username"`
			Credential    string `json:"credential"`
			Email         string `json:"email"`
			IsGlobal      bool   `json:"isGlobal"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		updated, err := cfg.Store.UpdateDockerRegistry(ctx, c.Params("id"), store.CreateDockerRegistryRequest{
			Name:          req.Name,
			ServerAddress: req.ServerAddress,
			Username:      req.Username,
			Credential:    req.Credential,
			Email:         req.Email,
			IsGlobal:      req.IsGlobal,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(updated)
	})

	reg.Delete("/:id", mutationLimiter, requireAdminScope("settings.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteDockerRegistry(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// Verify performs a real `docker login` on the first available node using the
	// stored credential, so an operator knows the secret is valid before relying
	// on it for pulls.
	reg.Post("/:id/verify", mutationLimiter, requireAdminScope("settings.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		cred, err := cfg.Store.GetDockerRegistryUnmasked(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		if cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client unavailable")
		}
		nodes, nerr := cfg.Store.ListNodes(ctx)
		if nerr != nil || len(nodes) == 0 {
			return fiber.NewError(fiber.StatusServiceUnavailable, "no nodes available to verify against")
		}
		nodeToken, terr := cfg.Store.GetNodeDaemonCredential(ctx, nodes[0].ID)
		if terr != nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, terr.Error())
		}
		auth := daemon.RegistryAuth{
			Username:      cred.Username,
			Password:      cred.Credential,
			ServerAddress: cred.ServerAddress,
		}
		if err := cfg.Daemon.LoginRegistry(ctx, nodes[0].BaseURL, nodeToken, auth); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"ok": false, "error": err.Error()})
		}
		return c.JSON(fiber.Map{"ok": true, "verified": true})
	})
}
