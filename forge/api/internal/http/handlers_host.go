package http

import (
	"context"
	"errors"
	"net/url"

	gpruntime "gamepanel/forge/internal/runtime"

	"github.com/gofiber/fiber/v2"
)

func mapDaemonError(err error) error {
	if err == nil {
		return nil
	}
	// Asking for a runtime Forge cannot run is a bad request, not a node
	// failure, and must not be reported as one.
	if errors.Is(err, gpruntime.ErrUnsupportedProvider) {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return fiber.NewError(fiber.StatusGatewayTimeout, "Node unreachable — request timed out")
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return fiber.NewError(fiber.StatusGatewayTimeout, "Node unreachable — connection timed out")
		}
		return fiber.NewError(fiber.StatusBadGateway, "Node offline — Beacon daemon unreachable")
	}
	return fiber.NewError(fiber.StatusBadGateway, err.Error())
}

type nodeHostTarget struct {
	NodeURL   string
	NodeToken string
	NodeID    string
	NodeName  string
}

// resolveNodeHostTarget resolves the Beacon a /host or /firewall request is
// aimed at. The node must be named explicitly.
//
// It used to fall back to the first node that happened to have a base URL and
// a credential whenever nodeId was absent. That silently answered host
// inspection for a machine the caller never asked about, and applied firewall
// changes to an arbitrary node. Reading the wrong machine's metrics is
// misleading; writing the wrong machine's firewall is dangerous. A request
// that does not say which node it means is incomplete, so it is rejected.
func resolveNodeHostTarget(cfg Config, nodeID string) (*nodeHostTarget, error) {
	ctx, cancel := requestContext()
	defer cancel()
	if nodeID == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "nodeId is required: specify which Beacon this request targets")
	}
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

func registerHostRoutes(protected fiber.Router, cfg Config) {
	protected.Get("/host/info", requireRole("admin"), requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
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
			return mapDaemonError(err)
		}
		return c.JSON(info)
	})

	protected.Get("/host/disk", requireRole("admin"), requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
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
			return mapDaemonError(err)
		}
		return c.JSON(disk)
	})

	protected.Get("/host/memory", requireRole("admin"), requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
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
			return mapDaemonError(err)
		}
		return c.JSON(mem)
	})

	protected.Get("/host/network", requireRole("admin"), requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
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
			return mapDaemonError(err)
		}
		return c.JSON(netIfaces)
	})

	protected.Get("/host/processes", requireRole("admin"), requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
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
			return mapDaemonError(err)
		}
		return c.JSON(procs)
	})
}
