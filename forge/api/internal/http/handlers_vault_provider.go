package http

import (
	"errors"
	"strings"

	"gamepanel/forge/internal/services/vaultprovider"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

// Admin HashiCorp Vault provider surface. Layered as handler ->
// vaultprovider.Service -> store (encrypted connection records) + Vault HTTP
// client. Every route is admin-gated (session role) and additionally requires
// the settings.* API scope, mirroring how private-registry credentials are
// exposed (see handlers_registries.go): reads use settings.read and every
// mutation, connection test and reference dry-run uses settings.write so a
// scoped key cannot register or probe a secret store.
//
// The service instance is the one wired in cmd/api (it is also installed as the
// environment-variable Vault resolver), so these routes never construct a
// competing service or a second HTTP client.

const vaultProviderRegistrarName = "vault-provider"

// vaultProviderPriority sits just after the recently added docker-events (230)
// and deployment-rollback (230) registrars.
const vaultProviderPriority = 235

func init() {
	RegisterPhaseRegistrar(vaultProviderRegistrarName, vaultProviderPriority, registerVaultProviderRoutes)
}

func registerVaultProviderRoutes(_ fiber.Router, protected fiber.Router, cfg *Config) error {
	if cfg.VaultService == nil {
		if cfg.Logger != nil {
			cfg.Logger.Warn(vaultProviderRegistrarName + ": vault service not configured, vault provider routes skipped")
		}
		return nil
	}
	svc := cfg.VaultService
	admin := protected.Group("/admin/vault", requireRole("admin"))
	limiter := RateLimiter(GetRateLimitForEndpoint("mutation", cfg.Redis, cfg.RedisEnabled && strings.EqualFold(strings.TrimSpace(cfg.AppEnv), "production")))
	registerVaultProviderRoutesOn(admin, *cfg, svc, limiter)
	return nil
}

func registerVaultProviderRoutesOn(admin fiber.Router, cfg Config, svc *vaultprovider.Service, mutationLimiter fiber.Handler) {
	// --- list (safe view: masked credential hints only) ---
	admin.Get("/", requireAdminScope("settings.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		// Connections are registered by admins as global (org_id IS NULL); a nil
		// org keeps parity with the DNS/registry provider surfaces.
		connections, err := svc.ListConnections(ctx, nil)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": connections})
	})

	// --- create ---
	admin.Post("/", mutationLimiter, requireAdminScope("settings.write"), func(c *fiber.Ctx) error {
		var req struct {
			Name          string  `json:"name"`
			BaseURL       string  `json:"baseUrl"`
			MountPath     string  `json:"mountPath"`
			Namespace     *string `json:"namespace"`
			EngineVersion int     `json:"engineVersion"`
			AuthMethod    string  `json:"authMethod"`
			Token         *string `json:"token"`
			RoleID        *string `json:"roleId"`
			SecretID      *string `json:"secretId"`
			Enabled       *bool   `json:"enabled"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		created, err := svc.CreateConnection(ctx, vaultprovider.ConnectionInput{
			Name:          req.Name,
			BaseURL:       req.BaseURL,
			MountPath:     req.MountPath,
			Namespace:     req.Namespace,
			EngineVersion: req.EngineVersion,
			AuthMethod:    req.AuthMethod,
			Token:         req.Token,
			RoleID:        req.RoleID,
			SecretID:      req.SecretID,
			Enabled:       req.Enabled,
		})
		if err != nil {
			return vaultProviderStoreError(c, err)
		}
		var actorID string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = claims.Sub
		}
		recordAudit(cfg, c, "vault:connection-create", "vault_connection", &created.ID, map[string]string{
			"name": created.Name, "actor": actorID,
		})
		return c.Status(fiber.StatusCreated).JSON(created)
	})

	// --- update ---
	admin.Patch("/:id", mutationLimiter, requireAdminScope("settings.write"), func(c *fiber.Ctx) error {
		var req struct {
			Name          *string `json:"name"`
			BaseURL       *string `json:"baseUrl"`
			MountPath     *string `json:"mountPath"`
			Namespace     *string `json:"namespace"`
			EngineVersion *int    `json:"engineVersion"`
			AuthMethod    *string `json:"authMethod"`
			Token         *string `json:"token"`
			RoleID        *string `json:"roleId"`
			SecretID      *string `json:"secretId"`
			Enabled       *bool   `json:"enabled"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		updated, err := svc.UpdateConnection(ctx, c.Params("id"), store.UpdateVaultConnectionRequest{
			Name:          req.Name,
			BaseURL:       req.BaseURL,
			MountPath:     req.MountPath,
			Namespace:     req.Namespace,
			EngineVersion: req.EngineVersion,
			AuthMethod:    req.AuthMethod,
			Token:         req.Token,
			RoleID:        req.RoleID,
			SecretID:      req.SecretID,
			Enabled:       req.Enabled,
		})
		if err != nil {
			return vaultProviderStoreError(c, err)
		}
		return c.JSON(updated)
	})

	// --- delete ---
	admin.Delete("/:id", mutationLimiter, requireAdminScope("settings.write"), func(c *fiber.Ctx) error {
		id := c.Params("id")
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.DeleteConnection(ctx, id); err != nil {
			return vaultProviderStoreError(c, err)
		}
		recordAudit(cfg, c, "vault:connection-delete", "vault_connection", &id, nil)
		return c.JSON(fiber.Map{"ok": true})
	})

	// --- test connection (real reachability + auth check) ---
	admin.Post("/:id/test", mutationLimiter, requireAdminScope("settings.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.TestConnection(ctx, c.Params("id")); err != nil {
			return vaultProviderRemoteError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// --- validate a reference (dry-run parse + resolve, never leaks the secret) ---
	admin.Post("/validate-reference", mutationLimiter, requireAdminScope("settings.write"), func(c *fiber.Ctx) error {
		var req struct {
			Value string `json:"value"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ref, isRef := vaultprovider.ParseVaultReference(req.Value)
		if !isRef {
			return c.JSON(fiber.Map{"reference": false})
		}
		ctx, cancel := requestContext()
		defer cancel()
		resolved, err := svc.Resolve(ctx, ref)
		if err != nil {
			return c.JSON(fiber.Map{"reference": true, "found": false, "reason": err.Error()})
		}
		return c.JSON(fiber.Map{"reference": true, "found": true, "length": len(resolved)})
	})
}

// vaultProviderStoreError maps store/service errors for the CRUD routes onto
// honest statuses: a missing row is 404, everything else is treated as input
// validation (400) exactly like the private-registry handlers.
func vaultProviderStoreError(c *fiber.Ctx, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return fiber.NewError(fiber.StatusNotFound, "vault connection not found")
	default:
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
}

// vaultProviderRemoteError maps connection-test / reference errors, which
// usually originate from the external Vault, onto honest statuses. An
// unreachable Vault is a 502, never a silent success.
func vaultProviderRemoteError(c *fiber.Ctx, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, vaultprovider.ErrSecretNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, vaultprovider.ErrInvalidBaseURL), errors.Is(err, vaultprovider.ErrUnknownAuthMethod):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	case errors.Is(err, vaultprovider.ErrUnreachable):
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	default:
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
}
