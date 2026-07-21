package http

import (
	"bytes"
	"encoding/json"
	"strings"

	"gamepanel/forge/internal/daemon"

	"github.com/gofiber/fiber/v2"
)

func registerDockerRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {
	if cfg.Store == nil || cfg.Daemon == nil {
		return
	}

	docker := protected.Group("/docker", adminIPAccess, requireRole("admin"))

	// Containers
	docker.Get("/containers", requireAdminScope("servers.read"), dockerListContainers(cfg))
	docker.Post("/containers", mutationLimiter, requireAdminScope("servers.write"), dockerCreateContainer(cfg))
	docker.Get("/containers/:id", requireAdminScope("servers.read"), dockerGetContainer(cfg))
	docker.Post("/containers/:id/operate", mutationLimiter, requireAdminScope("servers.write"), dockerOperateContainer(cfg))
	docker.Delete("/containers/:id", mutationLimiter, requireAdminScope("servers.write"), dockerDeleteContainer(cfg))
	docker.Get("/containers/:id/logs", requireAdminScope("servers.read"), dockerContainerLogs(cfg))
	docker.Get("/containers/:id/stats", requireAdminScope("servers.read"), dockerContainerStats(cfg))
	docker.Get("/containers/:id/files", requireAdminScope("servers.read"), dockerContainerFilesList(cfg))
	docker.Post("/containers/:id/files/read", mutationLimiter, requireAdminScope("servers.read"), dockerContainerFilesRead(cfg))
	docker.Post("/containers/:id/files/upload", mutationLimiter, requireAdminScope("servers.write"), dockerContainerFilesUpload(cfg))
	docker.Post("/containers/:id/files/delete", mutationLimiter, requireAdminScope("servers.write"), dockerContainerFilesDelete(cfg))

	// Images
	docker.Get("/images", requireAdminScope("servers.read"), dockerListImages(cfg))
	docker.Post("/images/build", mutationLimiter, requireAdminScope("servers.write"), dockerBuildImage(cfg))
	docker.Post("/images/pull", mutationLimiter, requireAdminScope("servers.write"), dockerPullImage(cfg))
	docker.Post("/images/:id/push", mutationLimiter, requireAdminScope("servers.write"), dockerPushImage(cfg))
	docker.Post("/images/:id/tag", mutationLimiter, requireAdminScope("servers.write"), dockerTagImage(cfg))
	docker.Delete("/images/:id", mutationLimiter, requireAdminScope("servers.write"), dockerDeleteImage(cfg))
	docker.Get("/images/search", requireAdminScope("servers.read"), dockerSearchImages(cfg))

	// Networks
	docker.Get("/networks", requireAdminScope("servers.read"), dockerListNetworks(cfg))
	docker.Post("/networks", mutationLimiter, requireAdminScope("servers.write"), dockerCreateNetwork(cfg))
	docker.Delete("/networks/:id", mutationLimiter, requireAdminScope("servers.write"), dockerDeleteNetwork(cfg))

	// Volumes
	docker.Get("/volumes", requireAdminScope("servers.read"), dockerListVolumes(cfg))
	docker.Post("/volumes", mutationLimiter, requireAdminScope("servers.write"), dockerCreateVolume(cfg))
	docker.Delete("/volumes/:id", mutationLimiter, requireAdminScope("servers.write"), dockerDeleteVolume(cfg))
	docker.Post("/volumes/prune", mutationLimiter, requireRole("admin"), requireAdminScope("servers.write"), dockerPruneVolumes(cfg))
}

func resolveDockerNode(cfg Config, c *fiber.Ctx) (*nodeAdminRequest, error) {
	nodeID := c.Query("node")
	if nodeID != "" {
		target, err := resolveSingleNodeTarget(cfg, nodeID)
		if err != nil {
			return nil, err
		}
		return target, nil
	}
	targets, err := resolveAdminNodeTargets(cfg)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	if len(targets) == 0 {
		return nil, fiber.NewError(fiber.StatusNotFound, "no available nodes")
	}
	return &targets[0], nil
}

func nodeListOrFirst(cfg Config) ([]nodeAdminRequest, error) {
	targets, err := resolveAdminNodeTargets(cfg)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	if len(targets) == 0 {
		return nil, fiber.NewError(fiber.StatusNotFound, "no available nodes")
	}
	return targets, nil
}

// --- Containers ---

func dockerListContainers(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		all := c.Query("all") != "false"
		targets, err := nodeListOrFirst(cfg)
		if err != nil {
			return err
		}
		results := make([]fiber.Map, 0, len(targets))
		for _, t := range targets {
			data, err := cfg.Daemon.AdminContainerList(c.Context(), t.NodeURL, t.NodeToken, all)
			if err != nil {
				continue
			}
			results = append(results, fiber.Map{
				"nodeId":     t.NodeID,
				"nodeName":   t.NodeName,
				"containers": data,
			})
		}
		return c.JSON(results)
	}
}

func dockerGetContainer(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		containerID := c.Params("id")
		target, err := resolveDockerNode(cfg, c)
		if err != nil {
			return err
		}
		data, err := cfg.Daemon.AdminContainerInspect(c.Context(), target.NodeURL, target.NodeToken, containerID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(fiber.Map{"nodeId": target.NodeID, "nodeName": target.NodeName, "container": data})
	}
}

func dockerCreateContainer(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var body map[string]any
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		target, err := resolveDockerNode(cfg, c)
		if err != nil {
			return err
		}
		data, err := cfg.Daemon.AdminContainerCreate(c.Context(), target.NodeURL, target.NodeToken, body)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		recordAudit(cfg, c, "container:create", "container", nil, map[string]string{"nodeId": target.NodeID})
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"nodeId": target.NodeID, "result": data})
	}
}

func dockerOperateContainer(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		containerID := c.Params("id")
		var body struct {
			Action string `json:"action"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		target, err := resolveDockerNode(cfg, c)
		if err != nil {
			return err
		}
		var opErr error
		switch body.Action {
		case "start":
			opErr = cfg.Daemon.AdminContainerStart(c.Context(), target.NodeURL, target.NodeToken, containerID)
		case "stop":
			opErr = cfg.Daemon.AdminContainerStop(c.Context(), target.NodeURL, target.NodeToken, containerID)
		case "restart":
			opErr = cfg.Daemon.AdminContainerRestart(c.Context(), target.NodeURL, target.NodeToken, containerID)
		case "pause":
			opErr = cfg.Daemon.AdminContainerPause(c.Context(), target.NodeURL, target.NodeToken, containerID)
		case "unpause":
			opErr = cfg.Daemon.AdminContainerUnpause(c.Context(), target.NodeURL, target.NodeToken, containerID)
		default:
			return fiber.NewError(fiber.StatusBadRequest, "invalid action: must be start/stop/restart/pause/unpause")
		}
		if opErr != nil {
			return fiber.NewError(fiber.StatusBadGateway, opErr.Error())
		}
		recordAudit(cfg, c, "container:"+body.Action, "container", &containerID, map[string]string{"nodeId": target.NodeID})
		return c.JSON(fiber.Map{"id": containerID, "action": body.Action, "status": "ok"})
	}
}

func dockerDeleteContainer(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		containerID := c.Params("id")
		force := c.Query("force") == "true"
		target, err := resolveDockerNode(cfg, c)
		if err != nil {
			return err
		}
		if err := cfg.Daemon.AdminContainerDelete(c.Context(), target.NodeURL, target.NodeToken, containerID, force, false); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		recordAudit(cfg, c, "container:delete", "container", &containerID, map[string]string{"nodeId": target.NodeID})
		return c.JSON(fiber.Map{"id": containerID, "action": "delete", "status": "ok"})
	}
}

func dockerContainerLogs(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		containerID := c.Params("id")
		tail := c.Query("tail")
		target, err := resolveDockerNode(cfg, c)
		if err != nil {
			return err
		}
		logs, err := cfg.Daemon.AdminContainerLogs(c.Context(), target.NodeURL, target.NodeToken, containerID, tail)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.Type("text/plain").SendString(logs)
	}
}

func dockerContainerStats(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		containerID := c.Params("id")
		target, err := resolveDockerNode(cfg, c)
		if err != nil {
			return err
		}
		data, err := cfg.Daemon.AdminContainerStats(c.Context(), target.NodeURL, target.NodeToken, containerID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(fiber.Map{"nodeId": target.NodeID, "stats": data})
	}
}

// --- Images ---

func dockerListImages(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		targets, err := nodeListOrFirst(cfg)
		if err != nil {
			return err
		}
		results := make([]fiber.Map, 0, len(targets))
		for _, t := range targets {
			data, err := cfg.Daemon.AdminImageList(c.Context(), t.NodeURL, t.NodeToken)
			if err != nil {
				continue
			}
			var images []map[string]any
			if err := json.Unmarshal(data, &images); err != nil {
				continue
			}
			for _, img := range images {
				tags, _ := img["RepoTags"].([]any)
				tagStr := ""
				if len(tags) > 0 {
					var ts []string
					for _, tag := range tags {
						ts = append(ts, tag.(string))
					}
					tagStr = strings.Join(ts, ",")
				}
				results = append(results, fiber.Map{
					"nodeId":   t.NodeID,
					"nodeName": t.NodeName,
					"id":       img["Id"],
					"tags":     tagStr,
					"size":     img["Size"],
					"created":  img["Created"],
				})
			}
		}
		return c.JSON(results)
	}
}

func dockerPullImage(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var body struct {
			NodeID string `json:"nodeId"`
			Image  string `json:"image"`
			Tag    string `json:"tag"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		image := body.Image
		if body.Tag != "" && !strings.Contains(image, ":") {
			image = image + ":" + body.Tag
		}
		var target *nodeAdminRequest
		var err error
		if body.NodeID != "" {
			target, err = resolveSingleNodeTarget(cfg, body.NodeID)
		} else {
			targets, e := nodeListOrFirst(cfg)
			if e != nil {
				return e
			}
			target = &targets[0]
		}
		if err != nil {
			return err
		}
		if err := cfg.Daemon.AdminImagePull(c.Context(), target.NodeURL, target.NodeToken, image, ""); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		recordAudit(cfg, c, "image:pull", "image", &image, map[string]string{"nodeId": target.NodeID})
		return c.JSON(fiber.Map{"image": image, "nodeId": target.NodeID, "status": "pulled"})
	}
}

func dockerDeleteImage(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		imageID := c.Params("id")
		nodeID := c.Query("node")
		if nodeID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "node query parameter is required")
		}
		target, err := resolveSingleNodeTarget(cfg, nodeID)
		if err != nil {
			return err
		}
		if err := cfg.Daemon.AdminImageDelete(c.Context(), target.NodeURL, target.NodeToken, imageID, true); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		recordAudit(cfg, c, "image:delete", "image", &imageID, map[string]string{"nodeId": nodeID})
		return c.JSON(fiber.Map{"image": imageID, "status": "deleted"})
	}
}

// --- Networks ---

func dockerListNetworks(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		targets, err := nodeListOrFirst(cfg)
		if err != nil {
			return err
		}
		results := make([]fiber.Map, 0, len(targets))
		for _, t := range targets {
			data, err := cfg.Daemon.AdminNetworkList(c.Context(), t.NodeURL, t.NodeToken)
			if err != nil {
				continue
			}
			var networks []map[string]any
			if err := json.Unmarshal(data, &networks); err != nil {
				continue
			}
			for _, n := range networks {
				containers, _ := n["Containers"].(map[string]any)
				attached := 0
				if containers != nil {
					attached = len(containers)
				}
				results = append(results, fiber.Map{
					"nodeId":   t.NodeID,
					"nodeName": t.NodeName,
					"id":       n["Id"],
					"name":     n["Name"],
					"driver":   n["Driver"],
					"scope":    n["Scope"],
					"attached": attached,
				})
			}
		}
		return c.JSON(results)
	}
}

func dockerCreateNetwork(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var body map[string]any
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		target, err := resolveDockerNode(cfg, c)
		if err != nil {
			return err
		}
		data, err := cfg.Daemon.AdminNetworkCreate(c.Context(), target.NodeURL, target.NodeToken, body)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		recordAudit(cfg, c, "network:create", "network", nil, map[string]string{"nodeId": target.NodeID})
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"nodeId": target.NodeID, "result": data})
	}
}

func dockerDeleteNetwork(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		networkID := c.Params("id")
		nodeID := c.Query("node")
		if nodeID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "node query parameter is required")
		}
		target, err := resolveSingleNodeTarget(cfg, nodeID)
		if err != nil {
			return err
		}
		if err := cfg.Daemon.AdminNetworkDelete(c.Context(), target.NodeURL, target.NodeToken, networkID); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		recordAudit(cfg, c, "network:delete", "network", &networkID, map[string]string{"nodeId": nodeID})
		return c.JSON(fiber.Map{"id": networkID, "status": "deleted"})
	}
}

// --- Volumes ---

func dockerListVolumes(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		targets, err := nodeListOrFirst(cfg)
		if err != nil {
			return err
		}
		results := make([]fiber.Map, 0, len(targets))
		for _, t := range targets {
			data, err := cfg.Daemon.AdminVolumeList(c.Context(), t.NodeURL, t.NodeToken)
			if err != nil {
				continue
			}
			var resp struct {
				Volumes []map[string]any `json:"volumes"`
			}
			if err := json.Unmarshal(data, &resp); err != nil {
				continue
			}
			for _, v := range resp.Volumes {
				name, _ := v["Name"].(string)
				driver, _ := v["Driver"].(string)
				mountpoint, _ := v["Mountpoint"].(string)
				createdAt, _ := v["CreatedAt"].(string)
				results = append(results, fiber.Map{
					"nodeId":     t.NodeID,
					"nodeName":   t.NodeName,
					"name":       name,
					"driver":     driver,
					"mountpoint": mountpoint,
					"createdAt":  createdAt,
				})
			}
		}
		return c.JSON(results)
	}
}

func dockerCreateVolume(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var body map[string]any
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		target, err := resolveDockerNode(cfg, c)
		if err != nil {
			return err
		}
		data, err := cfg.Daemon.AdminVolumeCreate(c.Context(), target.NodeURL, target.NodeToken, body)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		recordAudit(cfg, c, "volume:create", "volume", nil, map[string]string{"nodeId": target.NodeID})
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"nodeId": target.NodeID, "result": data})
	}
}

func dockerDeleteVolume(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		volumeName := c.Params("id")
		nodeID := c.Query("node")
		if nodeID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "node query parameter is required")
		}
		target, err := resolveSingleNodeTarget(cfg, nodeID)
		if err != nil {
			return err
		}
		if err := cfg.Daemon.AdminVolumeDelete(c.Context(), target.NodeURL, target.NodeToken, volumeName); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		recordAudit(cfg, c, "volume:delete", "volume", &volumeName, map[string]string{"nodeId": nodeID})
		return c.JSON(fiber.Map{"name": volumeName, "status": "deleted"})
	}
}

// --- Container File Operations ---

func dockerContainerFilesList(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		containerID := c.Params("id")
		filePath := c.Query("path")
		target, err := resolveDockerNode(cfg, c)
		if err != nil {
			return err
		}
		data, err := cfg.Daemon.AdminContainerFilesList(c.Context(), target.NodeURL, target.NodeToken, containerID, filePath)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(data)
	}
}

func dockerContainerFilesRead(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		containerID := c.Params("id")
		var body struct {
			Path string `json:"path"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		target, err := resolveDockerNode(cfg, c)
		if err != nil {
			return err
		}
		data, err := cfg.Daemon.AdminContainerFilesRead(c.Context(), target.NodeURL, target.NodeToken, containerID, body.Path)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.Type("text/plain").SendString(data)
	}
}

func dockerContainerFilesUpload(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		containerID := c.Params("id")
		destPath := c.Query("path")
		target, err := resolveDockerNode(cfg, c)
		if err != nil {
			return err
		}
		body := c.Body()
		contentType := c.Get("Content-Type")
		if err := cfg.Daemon.AdminContainerFilesUpload(c.Context(), target.NodeURL, target.NodeToken, containerID, destPath, bytes.NewReader(body), contentType); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(fiber.Map{"status": "uploaded"})
	}
}

func dockerContainerFilesDelete(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		containerID := c.Params("id")
		var body struct {
			Path string `json:"path"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		target, err := resolveDockerNode(cfg, c)
		if err != nil {
			return err
		}
		if err := cfg.Daemon.AdminContainerFilesDelete(c.Context(), target.NodeURL, target.NodeToken, containerID, body.Path); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(fiber.Map{"status": "deleted"})
	}
}

// --- Image Build/Push/Tag/Search ---

func dockerBuildImage(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var body struct {
			Dockerfile string `json:"dockerfile"`
			Tag        string `json:"tag"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		target, err := resolveDockerNode(cfg, c)
		if err != nil {
			return err
		}
		result, err := cfg.Daemon.AdminImageBuild(c.Context(), target.NodeURL, target.NodeToken, body.Dockerfile, body.Tag)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		recordAudit(cfg, c, "image:build", "image", &body.Tag, map[string]string{"nodeId": target.NodeID})
		return c.JSON(fiber.Map{"nodeId": target.NodeID, "result": result})
	}
}

func dockerPushImage(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		imageRef := c.Params("id")
		var body struct {
			RegistryAuth *daemon.RegistryAuth `json:"registryAuth,omitempty"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		target, err := resolveDockerNode(cfg, c)
		if err != nil {
			return err
		}
		if err := cfg.Daemon.AdminImagePush(c.Context(), target.NodeURL, target.NodeToken, imageRef, body.RegistryAuth); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		recordAudit(cfg, c, "image:push", "image", &imageRef, map[string]string{"nodeId": target.NodeID})
		return c.JSON(fiber.Map{"image": imageRef, "status": "pushed"})
	}
}

func dockerTagImage(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		imageID := c.Params("id")
		var body struct {
			Repo string `json:"repo"`
			Tag  string `json:"tag"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		target, err := resolveDockerNode(cfg, c)
		if err != nil {
			return err
		}
		if err := cfg.Daemon.AdminImageTag(c.Context(), target.NodeURL, target.NodeToken, imageID, body.Repo, body.Tag); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		recordAudit(cfg, c, "image:tag", "image", &imageID, map[string]string{"nodeId": target.NodeID})
		return c.JSON(fiber.Map{"image": imageID, "tag": body.Repo + ":" + body.Tag, "status": "tagged"})
	}
}

func dockerSearchImages(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		term := c.Query("term")
		targets, err := nodeListOrFirst(cfg)
		if err != nil {
			return err
		}
		results := make([]fiber.Map, 0, len(targets))
		for _, t := range targets {
			data, err := cfg.Daemon.AdminImageSearch(c.Context(), t.NodeURL, t.NodeToken, term)
			if err != nil {
				continue
			}
			results = append(results, fiber.Map{
				"nodeId":   t.NodeID,
				"nodeName": t.NodeName,
				"results":  data,
			})
		}
		return c.JSON(results)
	}
}

func dockerPruneVolumes(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		targets, err := nodeListOrFirst(cfg)
		if err != nil {
			return err
		}
		results := make([]fiber.Map, 0, len(targets))
		for _, t := range targets {
			data, err := cfg.Daemon.AdminVolumePrune(c.Context(), t.NodeURL, t.NodeToken)
			if err != nil {
				continue
			}
			results = append(results, fiber.Map{"nodeId": t.NodeID, "nodeName": t.NodeName, "result": data})
			recordAudit(cfg, c, "volume:prune", "node", &t.NodeID, nil)
		}
		return c.JSON(results)
	}
}
