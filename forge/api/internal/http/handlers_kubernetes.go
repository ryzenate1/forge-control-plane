package http

import (
	"strings"

	"github.com/gofiber/fiber/v2"
)

func registerKubernetesRoutes(protected fiber.Router, cfg Config, adminIPAccess fiber.Handler) {
	ks := protected.Group("/admin/kubernetes", adminIPAccess)
	ks.Get("/nodes", requireRole("admin"), func(c *fiber.Ctx) error { return listKubernetesNodes(c, cfg) })
	ks.Get("/pods", requireRole("admin"), func(c *fiber.Ctx) error { return kubernetesPods(c, cfg) })
	ks.Get("/deployments", requireRole("admin"), func(c *fiber.Ctx) error { return kubernetesDeployments(c, cfg) })
	ks.Get("/services", requireRole("admin"), func(c *fiber.Ctx) error { return kubernetesServices(c, cfg) })
	ks.Get("/events", requireRole("admin"), func(c *fiber.Ctx) error { return kubernetesEvents(c, cfg) })
	ks.Post("/deployments/:name/scale", requireRole("admin"), func(c *fiber.Ctx) error { return kubernetesScale(c, cfg) })
}

func listKubernetesNodes(c *fiber.Ctx, cfg Config) error {
	if cfg.Store == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "store unavailable")
	}
	ctx, cancel := requestContext()
	defer cancel()
	nodes, err := cfg.Store.ListNodes(ctx)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	var k8sNodes []fiber.Map
	for _, n := range nodes {
		if n.RuntimeProvider == "kubernetes" || n.RuntimeProvider == "k8s" {
			k8sNodes = append(k8sNodes, fiber.Map{
				"id": n.ID, "name": n.Name, "baseUrl": n.BaseURL, "runtimeProvider": n.RuntimeProvider,
				"runtimeStatus": n.RuntimeStatus, "status": n.Status, "regionId": n.RegionID,
			})
		}
	}
	if k8sNodes == nil {
		k8sNodes = []fiber.Map{}
	}
	return c.JSON(fiber.Map{"nodes": k8sNodes})
}

// resolveKubernetesDaemonTarget resolves the Beacon to proxy a Kubernetes
// request to. The node must be named explicitly: an empty nodeId is a bad
// request rather than an invitation to silently pick whichever node happens
// to be a Kubernetes node, which would read (or scale) a cluster the caller
// never asked about. Resolution is by node daemon credential, never by
// grabbing the first server that happens to sit on the node.
func resolveKubernetesDaemonTarget(c *fiber.Ctx, cfg Config) (string, string, error) {
	nodeID := strings.TrimSpace(c.Query("nodeId"))
	if nodeID == "" {
		nodeID = strings.TrimSpace(c.Query("node_id"))
	}
	if nodeID == "" {
		return "", "", fiber.NewError(fiber.StatusBadRequest, "nodeId is required: specify which kubernetes node this request targets")
	}
	if cfg.Store == nil {
		return "", "", fiber.NewError(fiber.StatusServiceUnavailable, "store unavailable")
	}
	ctx, cancel := requestContext()
	defer cancel()
	n, err := cfg.Store.GetNode(ctx, nodeID)
	if err != nil {
		return "", "", fiber.NewError(fiber.StatusNotFound, "node not found")
	}
	if n.RuntimeProvider != "kubernetes" && n.RuntimeProvider != "k8s" {
		return "", "", fiber.NewError(fiber.StatusBadRequest, "node is not a kubernetes node (runtime="+n.RuntimeProvider+")")
	}
	if strings.TrimSpace(n.BaseURL) == "" {
		return "", "", fiber.NewError(fiber.StatusBadGateway, "node has no daemon base URL")
	}
	token, err := cfg.Store.GetNodeDaemonCredential(ctx, n.ID)
	if err != nil {
		return "", "", fiber.NewError(fiber.StatusBadGateway, "cannot resolve node daemon credential: "+err.Error())
	}
	return n.BaseURL, token, nil
}

func kubernetesPods(c *fiber.Ctx, cfg Config) error {
	if cfg.Daemon == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client unavailable")
	}
	baseURL, token, err := resolveKubernetesDaemonTarget(c, cfg)
	if err != nil {
		return err
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	pods, err := cfg.Daemon.KubernetesPods(ctx, baseURL, token)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.JSON(fiber.Map{"pods": pods})
}

func kubernetesDeployments(c *fiber.Ctx, cfg Config) error {
	if cfg.Daemon == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client unavailable")
	}
	baseURL, token, err := resolveKubernetesDaemonTarget(c, cfg)
	if err != nil {
		return err
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	deps, err := cfg.Daemon.KubernetesDeployments(ctx, baseURL, token)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.JSON(fiber.Map{"deployments": deps})
}

func kubernetesServices(c *fiber.Ctx, cfg Config) error {
	if cfg.Daemon == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client unavailable")
	}
	baseURL, token, err := resolveKubernetesDaemonTarget(c, cfg)
	if err != nil {
		return err
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	svcs, err := cfg.Daemon.KubernetesServices(ctx, baseURL, token)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.JSON(fiber.Map{"services": svcs})
}

func kubernetesEvents(c *fiber.Ctx, cfg Config) error {
	if cfg.Daemon == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client unavailable")
	}
	baseURL, token, err := resolveKubernetesDaemonTarget(c, cfg)
	if err != nil {
		return err
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	events, err := cfg.Daemon.KubernetesEvents(ctx, baseURL, token)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.JSON(fiber.Map{"events": events})
}

func kubernetesScale(c *fiber.Ctx, cfg Config) error {
	if cfg.Daemon == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client unavailable")
	}
	name := c.Params("name")
	if name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "deployment name required")
	}
	var body struct {
		Replicas int32 `json:"replicas"`
	}
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	baseURL, token, err := resolveKubernetesDaemonTarget(c, cfg)
	if err != nil {
		return err
	}
	ctx, cancel := longRequestContext()
	defer cancel()
	if err := cfg.Daemon.KubernetesScale(ctx, baseURL, token, name, body.Replicas); err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	return c.JSON(fiber.Map{"ok": true, "deployment": name, "replicas": body.Replicas})
}
