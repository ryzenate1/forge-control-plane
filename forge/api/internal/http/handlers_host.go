package http

import (
	"github.com/gofiber/fiber/v2"
)

type nodeHostTarget struct {
	NodeURL   string
	NodeToken string
	NodeID    string
	NodeName  string
}

func resolveNodeHostTarget(cfg Config, nodeID string) (*nodeHostTarget, error) {
	ctx, cancel := requestContext()
	defer cancel()
	if nodeID != "" {
		node, err := cfg.Store.GetNode(ctx, nodeID)
		if err != nil {
			return nil, fiber.NewError(fiber.StatusNotFound, "node not found")
		}
		if node.BaseURL == "" {
			return nil, fiber.NewError(fiber.StatusBadRequest, "node has no base URL")
		}
		token, err := cfg.Store.GetNodeDaemonCredential(ctx, node.ID)
		if err != nil {
			return nil, fiber.NewError(fiber.StatusBadGateway, "node credential unavailable")
		}
		return &nodeHostTarget{NodeURL: node.BaseURL, NodeToken: token, NodeID: node.ID, NodeName: node.Name}, nil
	}
	nodes, err := cfg.Store.ListNodes(ctx)
	if err != nil || len(nodes) == 0 {
		return nil, fiber.NewError(fiber.StatusNotFound, "no nodes available")
	}
	for _, n := range nodes {
		if n.BaseURL == "" {
			continue
		}
		token, err := cfg.Store.GetNodeDaemonCredential(ctx, n.ID)
		if err != nil {
			continue
		}
		return &nodeHostTarget{NodeURL: n.BaseURL, NodeToken: token, NodeID: n.ID, NodeName: n.Name}, nil
	}
	return nil, fiber.NewError(fiber.StatusNotFound, "no reachable nodes")
}

func registerHostRoutes(protected fiber.Router, cfg Config) {
	protected.Get("/host/info", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Daemon == nil || cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client and store are required")
		}
		target, err := resolveNodeHostTarget(cfg, c.Query("nodeId"))
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		info, err := cfg.Daemon.GetHostInfo(ctx, target.NodeURL, target.NodeToken)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(info)
	})

	protected.Get("/host/disk", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Daemon == nil || cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client and store are required")
		}
		target, err := resolveNodeHostTarget(cfg, c.Query("nodeId"))
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		disk, err := cfg.Daemon.GetHostDisk(ctx, target.NodeURL, target.NodeToken)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(disk)
	})

	protected.Get("/host/memory", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Daemon == nil || cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client and store are required")
		}
		target, err := resolveNodeHostTarget(cfg, c.Query("nodeId"))
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		mem, err := cfg.Daemon.GetHostMemory(ctx, target.NodeURL, target.NodeToken)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(mem)
	})

	protected.Get("/host/network", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Daemon == nil || cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client and store are required")
		}
		target, err := resolveNodeHostTarget(cfg, c.Query("nodeId"))
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		netIfaces, err := cfg.Daemon.GetHostNetwork(ctx, target.NodeURL, target.NodeToken)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(netIfaces)
	})

	protected.Get("/host/processes", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Daemon == nil || cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "daemon client and store are required")
		}
		target, err := resolveNodeHostTarget(cfg, c.Query("nodeId"))
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		procs, err := cfg.Daemon.GetHostProcesses(ctx, target.NodeURL, target.NodeToken)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(procs)
	})
}
