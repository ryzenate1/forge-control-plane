package http

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	incus "gamepanel/forge/internal/services/incus"
)

func registerIncusRoutes(protected fiber.Router, cfg Config, adminIPAccess fiber.Handler) {
	ic := protected.Group("/admin/incus", adminIPAccess)
	ic.Get("/nodes", requireRole("admin"), requireAdminScope("incus.read"), func(c *fiber.Ctx) error { return incusListNodes(c, cfg) })
	ic.Get("/instances", requireRole("admin"), requireAdminScope("incus.read"), func(c *fiber.Ctx) error { return incusListInstances(c, cfg) })
	ic.Get("/instances/:name", requireRole("admin"), requireAdminScope("incus.read"), func(c *fiber.Ctx) error { return incusGetInstance(c, cfg) })
	ic.Post("/instances", requireRole("admin"), requireAdminScope("incus.write"), func(c *fiber.Ctx) error { return incusCreateInstance(c, cfg) })
	ic.Post("/instances/:name/start", requireRole("admin"), requireAdminScope("incus.write"), func(c *fiber.Ctx) error { return incusStartInstance(c, cfg) })
	ic.Post("/instances/:name/stop", requireRole("admin"), requireAdminScope("incus.write"), func(c *fiber.Ctx) error { return incusStopInstance(c, cfg) })
	ic.Post("/instances/:name/restart", requireRole("admin"), requireAdminScope("incus.write"), func(c *fiber.Ctx) error { return incusRestartInstance(c, cfg) })
	ic.Delete("/instances/:name", requireRole("admin"), requireAdminScope("incus.write"), func(c *fiber.Ctx) error { return incusDeleteInstance(c, cfg) })
	ic.Get("/images", requireRole("admin"), requireAdminScope("incus.read"), func(c *fiber.Ctx) error { return incusListImages(c, cfg) })
	ic.Get("/profiles", requireRole("admin"), requireAdminScope("incus.read"), func(c *fiber.Ctx) error { return incusListProfiles(c, cfg) })
	ic.Get("/storage-pools", requireRole("admin"), requireAdminScope("incus.read"), func(c *fiber.Ctx) error { return incusListStoragePools(c, cfg) })
	ic.Get("/cluster", requireRole("admin"), requireAdminScope("incus.read"), func(c *fiber.Ctx) error { return incusListClusterMembers(c, cfg) })
	ic.Get("/metrics", requireRole("admin"), requireAdminScope("incus.read"), func(c *fiber.Ctx) error { return incusServerMetrics(c, cfg) })
}

// incusError maps a service failure onto HTTP: an unconfigured backend is 503
// (unavailable), an upstream Incus fault stays 502 (bad gateway).
func incusError(err error) error {
	if errors.Is(err, incus.ErrNotConfigured) {
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
	}
	return fiber.NewError(fiber.StatusBadGateway, err.Error())
}

func incusListNodes(c *fiber.Ctx, cfg Config) error {
	if cfg.Store == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "store unavailable")
	}
	ctx, cancel := requestContext()
	defer cancel()
	nodes, err := cfg.Store.ListNodes(ctx)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	var out []fiber.Map
	for _, n := range nodes {
		if n.RuntimeProvider == incus.RuntimeProvider {
			out = append(out, fiber.Map{
				"id": n.ID, "name": n.Name, "baseUrl": n.BaseURL, "runtimeProvider": n.RuntimeProvider,
				"runtimeStatus": n.RuntimeStatus, "status": n.Status, "regionId": n.RegionID,
			})
		}
	}
	if out == nil {
		out = []fiber.Map{}
	}
	return c.JSON(fiber.Map{"nodes": out})
}

// resolveIncusNode returns the target node id from the query. With no node
// given it only auto-selects when at most one Forge Virtualization node exists:
// an unqualified request against several incus nodes is rejected rather than
// silently picking the first one. Zero nodes leaves the id empty so the service
// falls back to its environment-configured endpoint.
func resolveIncusNode(c *fiber.Ctx, cfg Config) (string, error) {
	nodeID := c.Query("nodeId")
	if nodeID == "" {
		nodeID = c.Query("node_id")
	}
	if nodeID != "" || cfg.Store == nil {
		return nodeID, nil
	}
	ctx, cancel := requestContext()
	defer cancel()
	nodes, err := cfg.Store.ListNodes(ctx)
	if err != nil {
		return "", fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	selected := ""
	count := 0
	for _, n := range nodes {
		if n.RuntimeProvider == incus.RuntimeProvider {
			selected = n.ID
			count++
		}
	}
	if count > 1 {
		return "", fiber.NewError(fiber.StatusBadRequest, "nodeId is required: multiple Forge Virtualization nodes are registered")
	}
	if count == 1 {
		return selected, nil
	}
	return "", nil
}

func incusListInstances(c *fiber.Ctx, cfg Config) error {
	svc := cfg.IncusService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Virtualization service unavailable")
	}
	nodeID, err := resolveIncusNode(c, cfg)
	if err != nil {
		return err
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	instances, err := svc.ListInstances(ctx, nodeID)
	if err != nil {
		return incusError(err)
	}
	return c.JSON(fiber.Map{"instances": instances})
}

func incusGetInstance(c *fiber.Ctx, cfg Config) error {
	svc := cfg.IncusService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Virtualization service unavailable")
	}
	name := c.Params("name")
	if name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "instance name required")
	}
	nodeID, err := resolveIncusNode(c, cfg)
	if err != nil {
		return err
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	instance, err := svc.GetInstance(ctx, nodeID, name)
	if err != nil {
		return incusError(err)
	}
	return c.JSON(instance)
}

func incusCreateInstance(c *fiber.Ctx, cfg Config) error {
	svc := cfg.IncusService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Virtualization service unavailable")
	}
	var spec map[string]any
	if err := c.BodyParser(&spec); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if len(spec) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "instance specification required")
	}
	nodeID, err := resolveIncusNode(c, cfg)
	if err != nil {
		return err
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	if err := svc.CreateInstance(ctx, nodeID, spec); err != nil {
		return incusError(err)
	}
	return c.JSON(fiber.Map{"ok": true})
}

func incusStartInstance(c *fiber.Ctx, cfg Config) error {
	svc := cfg.IncusService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Virtualization service unavailable")
	}
	name := c.Params("name")
	if name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "instance name required")
	}
	nodeID, err := resolveIncusNode(c, cfg)
	if err != nil {
		return err
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	if err := svc.StartInstance(ctx, nodeID, name); err != nil {
		return incusError(err)
	}
	return c.JSON(fiber.Map{"ok": true, "instance": name, "action": "start"})
}

func incusStopInstance(c *fiber.Ctx, cfg Config) error {
	svc := cfg.IncusService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Virtualization service unavailable")
	}
	name := c.Params("name")
	if name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "instance name required")
	}
	force := isTrueQuery(c)
	nodeID, err := resolveIncusNode(c, cfg)
	if err != nil {
		return err
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	if err := svc.StopInstance(ctx, nodeID, name, force); err != nil {
		return incusError(err)
	}
	return c.JSON(fiber.Map{"ok": true, "instance": name, "action": "stop"})
}

func incusRestartInstance(c *fiber.Ctx, cfg Config) error {
	svc := cfg.IncusService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Virtualization service unavailable")
	}
	name := c.Params("name")
	if name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "instance name required")
	}
	nodeID, err := resolveIncusNode(c, cfg)
	if err != nil {
		return err
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	if err := svc.RestartInstance(ctx, nodeID, name); err != nil {
		return incusError(err)
	}
	return c.JSON(fiber.Map{"ok": true, "instance": name, "action": "restart"})
}

func incusDeleteInstance(c *fiber.Ctx, cfg Config) error {
	svc := cfg.IncusService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Virtualization service unavailable")
	}
	name := c.Params("name")
	if name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "instance name required")
	}
	force := isTrueQuery(c)
	nodeID, err := resolveIncusNode(c, cfg)
	if err != nil {
		return err
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	if err := svc.DeleteInstance(ctx, nodeID, name, force); err != nil {
		return incusError(err)
	}
	return c.JSON(fiber.Map{"ok": true, "instance": name})
}

func incusListImages(c *fiber.Ctx, cfg Config) error {
	svc := cfg.IncusService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Virtualization service unavailable")
	}
	nodeID, err := resolveIncusNode(c, cfg)
	if err != nil {
		return err
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	images, err := svc.ListImages(ctx, nodeID)
	if err != nil {
		return incusError(err)
	}
	return c.JSON(fiber.Map{"images": images})
}

func incusListProfiles(c *fiber.Ctx, cfg Config) error {
	svc := cfg.IncusService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Virtualization service unavailable")
	}
	nodeID, err := resolveIncusNode(c, cfg)
	if err != nil {
		return err
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	profiles, err := svc.ListProfiles(ctx, nodeID)
	if err != nil {
		return incusError(err)
	}
	return c.JSON(fiber.Map{"profiles": profiles})
}

func incusListStoragePools(c *fiber.Ctx, cfg Config) error {
	svc := cfg.IncusService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Virtualization service unavailable")
	}
	nodeID, err := resolveIncusNode(c, cfg)
	if err != nil {
		return err
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	pools, err := svc.ListStoragePools(ctx, nodeID)
	if err != nil {
		return incusError(err)
	}
	return c.JSON(fiber.Map{"storagePools": pools})
}

func incusListClusterMembers(c *fiber.Ctx, cfg Config) error {
	svc := cfg.IncusService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Virtualization service unavailable")
	}
	nodeID, err := resolveIncusNode(c, cfg)
	if err != nil {
		return err
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	members, err := svc.ListClusterMembers(ctx, nodeID)
	if err != nil {
		return incusError(err)
	}
	return c.JSON(fiber.Map{"clusterMembers": members})
}

func incusServerMetrics(c *fiber.Ctx, cfg Config) error {
	svc := cfg.IncusService
	if svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "Forge Virtualization service unavailable")
	}
	nodeID, err := resolveIncusNode(c, cfg)
	if err != nil {
		return err
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	metrics, err := svc.GetServerMetrics(ctx, nodeID)
	if err != nil {
		return incusError(err)
	}
	return c.JSON(metrics)
}

// isTrueQuery reports whether the "force" query flag is set to a truthy value.
func isTrueQuery(c *fiber.Ctx) bool {
	return isTrueParam(c, "force")
}

// isTrueParam reports whether the named query parameter is set to a truthy
// value (1/true/yes/on).
func isTrueParam(c *fiber.Ctx, key string) bool {
	switch strings.ToLower(strings.TrimSpace(c.Query(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
