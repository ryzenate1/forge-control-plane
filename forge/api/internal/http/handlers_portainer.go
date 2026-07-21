package http

import (
	"context"
	"encoding/json"
	"strings"

	"gamepanel/forge/internal/daemon"

	"github.com/gofiber/fiber/v2"
)

func registerPortainerRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {
	if cfg.Store == nil || cfg.Daemon == nil {
		return
	}

	admin := protected.Group("/admin/containers", adminIPAccess, requireRole("admin"))
	admin.Get("/", requireAdminScope("servers.read"), listContainers(cfg))
	admin.Get("/inventory", requireAdminScope("servers.read"), containerInventory(cfg))
	admin.Get("/:id", requireAdminScope("servers.read"), inspectContainer(cfg))
	admin.Get("/:id/logs", requireAdminScope("servers.read"), containerLogs(cfg))
	admin.Post("/:id/start", mutationLimiter, requireAdminScope("servers.write"), startContainer(cfg))
	admin.Post("/:id/stop", mutationLimiter, requireAdminScope("servers.write"), stopContainer(cfg))
	admin.Post("/:id/restart", mutationLimiter, requireAdminScope("servers.write"), restartContainer(cfg))
	admin.Delete("/:id", mutationLimiter, requireAdminScope("servers.write"), deleteContainer(cfg))

	// Interactive exec requires explicit admin role (safe exec, not raw shell)
	admin.Post("/:id/exec", mutationLimiter, requireRole("admin"), requireAdminScope("servers.write"), execContainer(cfg))
	admin.Get("/:id/top", requireAdminScope("servers.read"), containerTop(cfg))
	admin.Get("/:id/changes", requireAdminScope("servers.read"), containerChanges(cfg))
	admin.Get("/:id/stats", requireAdminScope("servers.read"), containerStats(cfg))

	// Image management
	images := protected.Group("/admin/images", adminIPAccess, requireRole("admin"))
	images.Get("/", requireAdminScope("servers.read"), listImages(cfg))
	images.Get("/inventory", requireAdminScope("servers.read"), imageInventory(cfg))
	images.Post("/pull", mutationLimiter, requireAdminScope("servers.write"), pullImage(cfg))
	images.Delete("/:id", mutationLimiter, requireAdminScope("servers.write"), deleteImage(cfg))
	images.Post("/prune", mutationLimiter, requireRole("admin"), requireAdminScope("servers.write"), pruneImages(cfg))

	// Network inventory (read-only)
	networks := protected.Group("/admin/networks", adminIPAccess, requireRole("admin"))
	networks.Get("/", requireAdminScope("servers.read"), listNetworks(cfg))
	networks.Get("/inventory", requireAdminScope("servers.read"), networkInventory(cfg))
	networks.Get("/:id", requireAdminScope("servers.read"), inspectNetwork(cfg))

	// Volume inventory (read-only, with usage)
	volumes := protected.Group("/admin/volumes", adminIPAccess, requireRole("admin"))
	volumes.Get("/", requireAdminScope("servers.read"), listVolumes(cfg))
	volumes.Get("/inventory", requireAdminScope("servers.read"), volumeInventory(cfg))
	volumes.Get("/usage", requireAdminScope("servers.read"), volumeUsage(cfg))
	volumes.Get("/:id", requireAdminScope("servers.read"), inspectVolume(cfg))
}

type nodeAdminRequest struct {
	NodeURL   string
	NodeToken string
	NodeID    string
	NodeName  string
}

func resolveAdminNodeTargets(cfg Config) ([]nodeAdminRequest, error) {
	ctx, cancel := requestContext()
	defer cancel()
	nodes, err := cfg.Store.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	targets := make([]nodeAdminRequest, 0, len(nodes))
	for _, n := range nodes {
		if n.BaseURL == "" {
			continue
		}
		token, err := cfg.Store.GetNodeDaemonCredential(ctx, n.ID)
		if err != nil {
			continue
		}
		targets = append(targets, nodeAdminRequest{
			NodeURL: n.BaseURL, NodeToken: token,
			NodeID: n.ID, NodeName: n.Name,
		})
	}
	return targets, nil
}

func resolveSingleNodeTarget(cfg Config, nodeID string) (*nodeAdminRequest, error) {
	ctx, cancel := requestContext()
	defer cancel()
	node, err := cfg.Store.GetNode(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if node.BaseURL == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "node has no base URL")
	}
	token, err := cfg.Store.GetNodeDaemonCredential(ctx, node.ID)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadGateway, "node credential unavailable")
	}
	return &nodeAdminRequest{
		NodeURL: node.BaseURL, NodeToken: token,
		NodeID: node.ID, NodeName: node.Name,
	}, nil
}

func recordAudit(cfg Config, c *fiber.Ctx, action string, targetType string, targetID *string, meta map[string]string) {
	if cfg.Store == nil {
		return
	}
	ctx, cancel := requestContext()
	defer cancel()
	var actorID *string
	if claims, ok := c.Locals("user").(tokenClaims); ok {
		actorID = &claims.Sub
	}
	_ = cfg.Store.AppendAudit(ctx, actorID, action, targetType, targetID, safeAuditMeta(meta))
}

// --- Container handlers ---

func listContainers(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		targets, err := resolveAdminNodeTargets(cfg)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		type nodeContainerList struct {
			NodeID      string          `json:"nodeId"`
			NodeName    string          `json:"nodeName"`
			Containers  json.RawMessage `json:"containers"`
		}
		results := make([]nodeContainerList, 0, len(targets))
		for _, t := range targets {
			data, err := cfg.Daemon.AdminContainerList(c.Context(), t.NodeURL, t.NodeToken, true)
			if err != nil {
				continue
			}
			results = append(results, nodeContainerList{
				NodeID: t.NodeID, NodeName: t.NodeName, Containers: data,
			})
		}
		return c.JSON(results)
	}
}

func containerInventory(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		type containerEntry struct {
			NodeID     string `json:"nodeId"`
			NodeName   string `json:"nodeName"`
			ContainerID string `json:"containerId"`
			Name       string `json:"name"`
			Image      string `json:"image"`
			State      string `json:"state"`
			Status     string `json:"status"`
			Managed    bool   `json:"managed"`
		}
		targets, _ := resolveAdminNodeTargets(cfg)
		var all []containerEntry
		for _, t := range targets {
			data, err := cfg.Daemon.AdminContainerList(c.Context(), t.NodeURL, t.NodeToken, true)
			if err != nil {
				continue
			}
			var containers []struct {
				ID     string            `json:"id"`
				Names  []string          `json:"names"`
				Image  string            `json:"image"`
				State  string            `json:"state"`
				Status string            `json:"status"`
				Labels map[string]string `json:"labels"`
			}
			if err := json.Unmarshal(data, &containers); err != nil {
				continue
			}
			for _, ct := range containers {
				name := ""
				if len(ct.Names) > 0 {
					name = strings.TrimPrefix(ct.Names[0], "/")
				}
				managed := ct.Labels["modern-game-panel.server_id"] != ""
				all = append(all, containerEntry{
					NodeID: t.NodeID, NodeName: t.NodeName,
					ContainerID: ct.ID, Name: name, Image: ct.Image,
					State: ct.State, Status: ct.Status, Managed: managed,
				})
			}
		}
		return c.JSON(all)
	}
}

func inspectContainer(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		nodeID := c.Query("node")
		id := c.Params("id")
		if nodeID != "" {
			target, err := resolveSingleNodeTarget(cfg, nodeID)
			if err != nil {
				return err
			}
			data, err := cfg.Daemon.AdminContainerInspect(c.Context(), target.NodeURL, target.NodeToken, id)
			if err != nil {
				return fiber.NewError(fiber.StatusBadGateway, err.Error())
			}
			recordAudit(cfg, c, "container:inspect", "container", &id, map[string]string{"nodeId": nodeID})
			return c.JSON(fiber.Map{"nodeId": nodeID, "nodeName": target.NodeName, "inspect": data})
		}
		targets, _ := resolveAdminNodeTargets(cfg)
		for _, t := range targets {
			data, err := cfg.Daemon.AdminContainerInspect(c.Context(), t.NodeURL, t.NodeToken, id)
			if err != nil {
				continue
			}
			recordAudit(cfg, c, "container:inspect", "container", &id, map[string]string{"nodeId": t.NodeID})
			return c.JSON(fiber.Map{"nodeId": t.NodeID, "nodeName": t.NodeName, "inspect": data})
		}
		return fiber.NewError(fiber.StatusNotFound, "container not found on any node")
	}
}

func containerLogs(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		nodeID := c.Query("node")
		id := c.Params("id")
		tail := c.Query("tail")
		if nodeID != "" {
			target, err := resolveSingleNodeTarget(cfg, nodeID)
			if err != nil {
				return err
			}
			logs, err := cfg.Daemon.AdminContainerLogs(c.Context(), target.NodeURL, target.NodeToken, id, tail)
			if err != nil {
				return fiber.NewError(fiber.StatusBadGateway, err.Error())
			}
			return c.Type("text/plain").SendString(logs)
		}
		targets, _ := resolveAdminNodeTargets(cfg)
		for _, t := range targets {
			logs, err := cfg.Daemon.AdminContainerLogs(c.Context(), t.NodeURL, t.NodeToken, id, tail)
			if err != nil {
				continue
			}
			return c.Type("text/plain").SendString(logs)
		}
		return fiber.NewError(fiber.StatusNotFound, "container not found on any node")
	}
}

func startContainer(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		return containerPowerOp(cfg, c, "start")
	}
}

func stopContainer(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		return containerPowerOp(cfg, c, "stop")
	}
}

func restartContainer(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		return containerPowerOp(cfg, c, "restart")
	}
}

func containerPowerOp(cfg Config, c *fiber.Ctx, action string) error {
	nodeID := c.Query("node")
	containerID := c.Params("id")
	var target *nodeAdminRequest
	var err error
	if nodeID != "" {
		target, err = resolveSingleNodeTarget(cfg, nodeID)
		if err != nil {
			return err
		}
	} else {
		targets, _ := resolveAdminNodeTargets(cfg)
		for _, t := range targets {
			data, inspectErr := cfg.Daemon.AdminContainerInspect(c.Context(), t.NodeURL, t.NodeToken, containerID)
			if inspectErr == nil && data != nil {
				target = &t
				break
			}
		}
		if target == nil {
			return fiber.NewError(fiber.StatusNotFound, "container not found on any node")
		}
	}
	var opErr error
	switch action {
	case "start":
		opErr = cfg.Daemon.AdminContainerStart(c.Context(), target.NodeURL, target.NodeToken, containerID)
	case "stop":
		opErr = cfg.Daemon.AdminContainerStop(c.Context(), target.NodeURL, target.NodeToken, containerID)
	case "restart":
		opErr = cfg.Daemon.AdminContainerRestart(c.Context(), target.NodeURL, target.NodeToken, containerID)
	}
	if opErr != nil {
		return fiber.NewError(fiber.StatusBadGateway, opErr.Error())
	}
	recordAudit(cfg, c, "container:"+action, "container", &containerID, map[string]string{"nodeId": target.NodeID})
	return c.JSON(fiber.Map{"id": containerID, "action": action, "status": "ok", "nodeId": target.NodeID})
}

func deleteContainer(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		nodeID := c.Query("node")
		containerID := c.Params("id")
		force := c.Query("force") == "true"
		removeVolumes := c.Query("v") == "true"
		var target *nodeAdminRequest
		var err error
		if nodeID != "" {
			target, err = resolveSingleNodeTarget(cfg, nodeID)
			if err != nil {
				return err
			}
		} else {
			targets, _ := resolveAdminNodeTargets(cfg)
			for _, t := range targets {
				data, inspectErr := cfg.Daemon.AdminContainerInspect(c.Context(), t.NodeURL, t.NodeToken, containerID)
				if inspectErr == nil && data != nil {
					target = &t
					break
				}
			}
			if target == nil {
				return fiber.NewError(fiber.StatusNotFound, "container not found on any node")
			}
		}
		if err := cfg.Daemon.AdminContainerDelete(c.Context(), target.NodeURL, target.NodeToken, containerID, force, removeVolumes); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		recordAudit(cfg, c, "container:delete", "container", &containerID, map[string]string{"nodeId": target.NodeID, "force": c.Query("force"), "v": c.Query("v")})
		return c.JSON(fiber.Map{"id": containerID, "action": "delete", "status": "ok"})
	}
}

func execContainer(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		nodeID := c.Query("node")
		containerID := c.Params("id")
		var body struct {
			Cmd []string `json:"cmd"`
			Tty bool     `json:"tty"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if len(body.Cmd) == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "cmd is required")
		}
		var target *nodeAdminRequest
		var err error
		if nodeID != "" {
			target, err = resolveSingleNodeTarget(cfg, nodeID)
			if err != nil {
				return err
			}
		} else {
			targets, _ := resolveAdminNodeTargets(cfg)
			for _, t := range targets {
				data, inspectErr := cfg.Daemon.AdminContainerInspect(c.Context(), t.NodeURL, t.NodeToken, containerID)
				if inspectErr == nil && data != nil {
					target = &t
					break
				}
			}
			if target == nil {
				return fiber.NewError(fiber.StatusNotFound, "container not found on any node")
			}
		}
		data, err := cfg.Daemon.AdminContainerExec(c.Context(), target.NodeURL, target.NodeToken, containerID, body.Cmd, body.Tty)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		recordAudit(cfg, c, "container:exec", "container", &containerID, map[string]string{"nodeId": target.NodeID, "cmd": strings.Join(body.Cmd, " ")})
		return c.JSON(fiber.Map{"nodeId": target.NodeID, "result": data})
	}
}

func containerTop(cfg Config) fiber.Handler {
	return containerInspectOnNode(cfg, "top", func(dc *daemon.Client, ctx context.Context, url, token, id string) (json.RawMessage, error) {
		return dc.AdminContainerTop(ctx, url, token, id)
	})
}

func containerChanges(cfg Config) fiber.Handler {
	return containerInspectOnNode(cfg, "changes", func(dc *daemon.Client, ctx context.Context, url, token, id string) (json.RawMessage, error) {
		return dc.AdminContainerInspect(ctx, url, token, id)
	})
}

func containerStats(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		nodeID := c.Query("node")
		containerID := c.Params("id")
		if nodeID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "node query parameter is required for stats")
		}
		target, err := resolveSingleNodeTarget(cfg, nodeID)
		if err != nil {
			return err
		}
		data, err := cfg.Daemon.AdminContainerInspect(c.Context(), target.NodeURL, target.NodeToken, containerID)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "container not found on node")
		}
		_ = data
		return c.JSON(fiber.Map{"nodeId": target.NodeID, "nodeName": target.NodeName})
	}
}

type containerInspectFunc func(*daemon.Client, context.Context, string, string, string) (json.RawMessage, error)

func containerInspectOnNode(cfg Config, action string, fn containerInspectFunc) fiber.Handler {
	return func(c *fiber.Ctx) error {
		nodeID := c.Query("node")
		containerID := c.Params("id")
		if nodeID != "" {
			target, err := resolveSingleNodeTarget(cfg, nodeID)
			if err != nil {
				return err
			}
			data, err := fn(cfg.Daemon, c.Context(), target.NodeURL, target.NodeToken, containerID)
			if err != nil {
				return fiber.NewError(fiber.StatusBadGateway, err.Error())
			}
			return c.JSON(fiber.Map{"nodeId": target.NodeID, "nodeName": target.NodeName, action: data})
		}
		targets, _ := resolveAdminNodeTargets(cfg)
		for _, t := range targets {
			data, err := fn(cfg.Daemon, c.Context(), t.NodeURL, t.NodeToken, containerID)
			if err != nil {
				continue
			}
			return c.JSON(fiber.Map{"nodeId": t.NodeID, "nodeName": t.NodeName, action: data})
		}
		return fiber.NewError(fiber.StatusNotFound, "container not found on any node")
	}
}

// --- Image handlers ---

func listImages(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		targets, err := resolveAdminNodeTargets(cfg)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		type nodeImageList struct {
			NodeID   string          `json:"nodeId"`
			NodeName string          `json:"nodeName"`
			Images   json.RawMessage `json:"images"`
		}
		results := make([]nodeImageList, 0, len(targets))
		for _, t := range targets {
			data, err := cfg.Daemon.AdminImageList(c.Context(), t.NodeURL, t.NodeToken)
			if err != nil {
				continue
			}
			results = append(results, nodeImageList{NodeID: t.NodeID, NodeName: t.NodeName, Images: data})
		}
		return c.JSON(results)
	}
}

func imageInventory(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		type imageEntry struct {
			NodeID   string `json:"nodeId"`
			NodeName string `json:"nodeName"`
			ID       string `json:"id"`
			Tags     string `json:"tags"`
			Size     int64  `json:"size"`
			Created  int64  `json:"created"`
		}
		targets, _ := resolveAdminNodeTargets(cfg)
		var all []imageEntry
		for _, t := range targets {
			data, err := cfg.Daemon.AdminImageList(c.Context(), t.NodeURL, t.NodeToken)
			if err != nil {
				continue
			}
			var images []struct {
				ID       string   `json:"id"`
				RepoTags []string `json:"repoTags"`
				Size     int64    `json:"size"`
				Created  int64    `json:"created"`
			}
			if err := json.Unmarshal(data, &images); err != nil {
				continue
			}
			for _, img := range images {
				tags := strings.Join(img.RepoTags, ",")
				all = append(all, imageEntry{
					NodeID: t.NodeID, NodeName: t.NodeName,
					ID: img.ID, Tags: tags, Size: img.Size, Created: img.Created,
				})
			}
		}
		return c.JSON(all)
	}
}

func pullImage(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var body struct {
			NodeID       string `json:"nodeId"`
			Image        string `json:"image"`
			RegistryAuth string `json:"registryAuth,omitempty"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if body.Image == "" {
			return fiber.NewError(fiber.StatusBadRequest, "image is required")
		}
		if body.NodeID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "nodeId is required")
		}
		target, err := resolveSingleNodeTarget(cfg, body.NodeID)
		if err != nil {
			return err
		}
		if err := cfg.Daemon.AdminImagePull(c.Context(), target.NodeURL, target.NodeToken, body.Image, body.RegistryAuth); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		recordAudit(cfg, c, "image:pull", "image", &body.Image, map[string]string{"nodeId": body.NodeID})
		return c.JSON(fiber.Map{"image": body.Image, "nodeId": body.NodeID, "status": "pulled"})
	}
}

func deleteImage(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		nodeID := c.Query("node")
		imageID := c.Params("id")
		if nodeID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "node query parameter is required")
		}
		target, err := resolveSingleNodeTarget(cfg, nodeID)
		if err != nil {
			return err
		}
		force := c.Query("force") == "true"
		if err := cfg.Daemon.AdminImageDelete(c.Context(), target.NodeURL, target.NodeToken, imageID, force); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		recordAudit(cfg, c, "image:delete", "image", &imageID, map[string]string{"nodeId": nodeID, "force": c.Query("force")})
		return c.JSON(fiber.Map{"image": imageID, "status": "removed"})
	}
}

func pruneImages(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		targets, err := resolveAdminNodeTargets(cfg)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		type pruneResult struct {
			NodeID   string          `json:"nodeId"`
			NodeName string          `json:"nodeName"`
			Result   json.RawMessage `json:"result"`
		}
		results := make([]pruneResult, 0, len(targets))
		for _, t := range targets {
			data, err := cfg.Daemon.AdminImagePrune(c.Context(), t.NodeURL, t.NodeToken)
			if err != nil {
				continue
			}
			results = append(results, pruneResult{NodeID: t.NodeID, NodeName: t.NodeName, Result: data})
			recordAudit(cfg, c, "image:prune", "node", &t.NodeID, nil)
		}
		return c.JSON(results)
	}
}

// --- Network handlers ---

func listNetworks(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		targets, err := resolveAdminNodeTargets(cfg)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		type nodeNetworkList struct {
			NodeID   string          `json:"nodeId"`
			NodeName string          `json:"nodeName"`
			Networks json.RawMessage `json:"networks"`
		}
		results := make([]nodeNetworkList, 0, len(targets))
		for _, t := range targets {
			data, err := cfg.Daemon.AdminNetworkList(c.Context(), t.NodeURL, t.NodeToken)
			if err != nil {
				continue
			}
			results = append(results, nodeNetworkList{NodeID: t.NodeID, NodeName: t.NodeName, Networks: data})
		}
		return c.JSON(results)
	}
}

func networkInventory(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		type netEntry struct {
			NodeID   string `json:"nodeId"`
			NodeName string `json:"nodeName"`
			ID       string `json:"id"`
			Name     string `json:"name"`
			Driver   string `json:"driver"`
			Scope    string `json:"scope"`
			Attached int    `json:"attached"`
		}
		targets, _ := resolveAdminNodeTargets(cfg)
		var all []netEntry
		for _, t := range targets {
			data, err := cfg.Daemon.AdminNetworkList(c.Context(), t.NodeURL, t.NodeToken)
			if err != nil {
				continue
			}
			var networks []struct {
				ID      string `json:"Id"`
				Name    string `json:"Name"`
				Driver  string `json:"Driver"`
				Scope   string `json:"Scope"`
				Containers map[string]any `json:"Containers"`
			}
			if err := json.Unmarshal(data, &networks); err != nil {
				continue
			}
			for _, n := range networks {
				all = append(all, netEntry{
					NodeID: t.NodeID, NodeName: t.NodeName,
					ID: n.ID, Name: n.Name, Driver: n.Driver,
					Scope: n.Scope, Attached: len(n.Containers),
				})
			}
		}
		return c.JSON(all)
	}
}

func inspectNetwork(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		nodeID := c.Query("node")
		netID := c.Params("id")
		if nodeID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "node query parameter is required")
		}
		target, err := resolveSingleNodeTarget(cfg, nodeID)
		if err != nil {
			return err
		}
		data, err := cfg.Daemon.AdminNetworkInspect(c.Context(), target.NodeURL, target.NodeToken, netID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(fiber.Map{"nodeId": target.NodeID, "nodeName": target.NodeName, "network": data})
	}
}

// --- Volume handlers ---

func listVolumes(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		targets, err := resolveAdminNodeTargets(cfg)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		type nodeVolumeList struct {
			NodeID   string          `json:"nodeId"`
			NodeName string          `json:"nodeName"`
			Volumes  json.RawMessage `json:"volumes"`
		}
		results := make([]nodeVolumeList, 0, len(targets))
		for _, t := range targets {
			data, err := cfg.Daemon.AdminVolumeList(c.Context(), t.NodeURL, t.NodeToken)
			if err != nil {
				continue
			}
			results = append(results, nodeVolumeList{NodeID: t.NodeID, NodeName: t.NodeName, Volumes: data})
		}
		return c.JSON(results)
	}
}

func volumeInventory(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		type volEntry struct {
			NodeID     string `json:"nodeId"`
			NodeName   string `json:"nodeName"`
			Name       string `json:"name"`
			Driver     string `json:"driver"`
			Mountpoint string `json:"mountpoint"`
			Scope      string `json:"scope"`
		}
		targets, _ := resolveAdminNodeTargets(cfg)
		var all []volEntry
		for _, t := range targets {
			data, err := cfg.Daemon.AdminVolumeList(c.Context(), t.NodeURL, t.NodeToken)
			if err != nil {
				continue
			}
			var resp struct {
				Volumes []struct {
					Name       string `json:"name"`
					Driver     string `json:"driver"`
					Mountpoint string `json:"mountpoint"`
					Scope      string `json:"scope"`
				} `json:"volumes"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				continue
			}
			for _, v := range resp.Volumes {
				all = append(all, volEntry{
					NodeID: t.NodeID, NodeName: t.NodeName,
					Name: v.Name, Driver: v.Driver,
					Mountpoint: v.Mountpoint, Scope: v.Scope,
				})
			}
		}
		return c.JSON(all)
	}
}

func inspectVolume(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		nodeID := c.Query("node")
		volName := c.Params("id")
		if nodeID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "node query parameter is required")
		}
		target, err := resolveSingleNodeTarget(cfg, nodeID)
		if err != nil {
			return err
		}
		data, err := cfg.Daemon.AdminVolumeInspect(c.Context(), target.NodeURL, target.NodeToken, volName)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(fiber.Map{"nodeId": target.NodeID, "nodeName": target.NodeName, "volume": data})
	}
}

func volumeUsage(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		targets, err := resolveAdminNodeTargets(cfg)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		type nodeVolumeUsage struct {
			NodeID   string          `json:"nodeId"`
			NodeName string          `json:"nodeName"`
			Usage    json.RawMessage `json:"usage"`
		}
		results := make([]nodeVolumeUsage, 0, len(targets))
		for _, t := range targets {
			data, err := cfg.Daemon.AdminVolumeUsage(c.Context(), t.NodeURL, t.NodeToken)
			if err != nil {
				continue
			}
			results = append(results, nodeVolumeUsage{NodeID: t.NodeID, NodeName: t.NodeName, Usage: data})
		}
		return c.JSON(results)
	}
}
