package http

import (
	"net"
	"net/url"
	"strings"

	"gamepanel/forge/internal/services/appstore"

	"github.com/gofiber/fiber/v2"
)

func registerAppStoreRoutes(protected fiber.Router, cfg Config, svc *appstore.Service, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	// The app-store mutates the catalog and installs stacks, so the entire
	// surface is admin-only. Without this guard any authenticated user could
	// poison the catalog (sync) or install/uninstall/upgrade arbitrary apps.
	store := protected.Group("/app-store", requireRole("admin"), mutationLimiter)

	listAppsHandler := func(c *fiber.Ctx) error {
		category := c.Query("category")
		search := c.Query("search")
		apps, err := svc.ListApps(c.Context(), category, search)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": apps})
	}

	store.Get("/apps", listAppsHandler)
	// The frontend's GET /admin/app-templates is served by the default
	// template catalog in handlers_apphosting.go (registered earlier, so it
	// wins in Fiber). Expose the app-store catalog on a distinct admin path
	// instead of colliding with that route.
	protected.Get("/admin/app-store-templates", requireRole("admin"), listAppsHandler)

	store.Get("/apps/:key", func(c *fiber.Ctx) error {
		app, err := svc.GetApp(c.Context(), c.Params("key"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "app not found"})
		}
		return c.JSON(fiber.Map{"data": app})
	})

	store.Post("/install", func(c *fiber.Ctx) error {
		var req appstore.InstallRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request: " + err.Error()})
		}
		if strings.TrimSpace(req.NodeID) == "" || strings.TrimSpace(req.Name) == "" {
			return c.Status(400).JSON(fiber.Map{"error": "name and nodeId are required"})
		}
		if req.AppKey == "" {
			return c.Status(400).JSON(fiber.Map{"error": "appKey is required"})
		}
		// The actor is derived from the verified session/API-token claims and the
		// body-supplied userId is ignored, so a caller can never attribute an
		// install to another user or to "system".
		claims, cerr := currentClaims(c)
		if cerr != nil {
			return cerr
		}
		req.UserID = claims.Sub
		if req.MemoryMB <= 0 {
			req.MemoryMB = 256
		}
		if req.CPUShares <= 0 {
			req.CPUShares = 512
		}
		if req.DiskMB <= 0 {
			req.DiskMB = 1024
		}

		inst, err := svc.InstallApp(c.Context(), &req)
		if err != nil {
			if inst != nil {
				logInternalError(c, err)
				msg := "an internal error occurred"
				if !isProductionEnv(c) {
					msg = err.Error()
				}
				return c.Status(500).JSON(fiber.Map{"data": inst, "error": msg})
			}
			return respondInternalError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"data": inst})
	})

	store.Post("/:id/uninstall", func(c *fiber.Ctx) error {
		if err := svc.UninstallApp(c.Context(), c.Params("id")); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": "ok"})
	})

	store.Get("/installed", func(c *fiber.Ctx) error {
		installs, err := svc.ListInstalls(c.Context())
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": installs})
	})

	store.Post("/:id/upgrade", func(c *fiber.Ctx) error {
		inst, err := svc.UpgradeApp(c.Context(), c.Params("id"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": inst})
	})

	store.Post("/sync", func(c *fiber.Ctx) error {
		var req struct {
			RegistryURL string `json:"registryUrl"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := validateRegistryURL(req.RegistryURL); err != nil {
			return err
		}
		if err := svc.SyncFromRemote(c.Context(), req.RegistryURL); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": "sync initiated"})
	})

	// Re-seed the catalog from the embedded Coolify template library. Mounted on
	// an explicit admin path (matching the /admin/app-store-templates precedent
	// above) so the console route is unambiguous; guarded by the same admin role
	// and mutation limiter as the rest of the app-store surface.
	protected.Post("/admin/app-store/sync-bundled", requireRole("admin"), mutationLimiter, func(c *fiber.Ctx) error {
		res, err := svc.SeedBundledTemplates(c.Context())
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": res})
	})
}

// validateRegistryURL guards the app-store sync target against SSRF and
// catalog poisoning. It rejects non-HTTPS URLs and any host that resolves to a
// loopback, link-local or private address, so a caller cannot point the daemon
// at internal infrastructure (metadata endpoints, the control plane itself,
// other tenants' services) and have its response written into the catalog.
func validateRegistryURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fiber.NewError(fiber.StatusBadRequest, "registryUrl is required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fiber.NewError(fiber.StatusBadRequest, "invalid registry URL")
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return fiber.NewError(fiber.StatusBadRequest, "registry URL must use https")
	}
	if u.User != nil {
		return fiber.NewError(fiber.StatusBadRequest, "registry URL must not include credentials")
	}
	host := u.Hostname()
	if host == "" {
		return fiber.NewError(fiber.StatusBadRequest, "registry URL must include a host")
	}
	if ip := net.ParseIP(host); ip != nil {
		return rejectDisallowedRegistryIP(ip)
	}
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "registry host could not be resolved")
	}
	for _, ip := range ips {
		if err := rejectDisallowedRegistryIP(ip); err != nil {
			return err
		}
	}
	return nil
}

// rejectDisallowedRegistryIP reports whether an address falls into a range the
// registry fetch must never reach.
func rejectDisallowedRegistryIP(ip net.IP) error {
	if ip == nil {
		return fiber.NewError(fiber.StatusBadRequest, "registry host resolves to an invalid address")
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() {
		return fiber.NewError(fiber.StatusBadRequest, "registry host resolves to a disallowed address")
	}
	return nil
}
