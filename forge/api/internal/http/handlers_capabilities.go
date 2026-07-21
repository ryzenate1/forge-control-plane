package http

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"gamepanel/forge/internal/services/nodeprobe"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func registerCapabilityRoutes(protected fiber.Router, cfg Config, nodeProbe *nodeprobe.Service) {
	// GET /capabilities — list all node capabilities (inventory view)
	protected.Get("/capabilities", requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		offset := 0
		if p := c.Query("offset"); p != "" {
			if v, err := strconv.Atoi(p); err == nil && v >= 0 {
				offset = v
			}
		}
		limit := 50
		if l := c.Query("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
				limit = v
			}
		}
		ctx, cancel := requestContext()
		defer cancel()
		caps, err := cfg.Store.ListCapabilities(ctx, store.CapabilityInventoryFilter{Offset: offset, Limit: limit})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"data": caps})
	})

	// GET /capabilities/:nodeId — single node capability detail
	protected.Get("/capabilities/:nodeId", requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		nc, err := cfg.Store.GetNodeCapability(ctx, c.Params("nodeId"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "capability not found")
		}
		return c.JSON(nc)
	})

	// GET /capabilities/:nodeId/history — capability change history
	protected.Get("/capabilities/:nodeId/history", requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		limit := 20
		if l := c.Query("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
				limit = v
			}
		}
		ctx, cancel := requestContext()
		defer cancel()
		entries, err := cfg.Store.GetCapabilityHistory(ctx, c.Params("nodeId"), limit)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"data": entries})
	})

	// POST /capabilities/:nodeId/probe — live-probe a node's beacon for capabilities
	protected.Post("/capabilities/:nodeId/probe", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		if nodeProbe == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "node probe not available")
		}
		nodeID := c.Params("nodeId")
		ctx, cancel := requestContext()
		defer cancel()

		info, err := nodeProbe.ProbeNode(ctx, nodeID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if !info.Online {
			return c.Status(fiber.StatusOK).JSON(fiber.Map{
				"online": false,
				"error":  info.Error,
			})
		}

		capEntries := []map[string]any{
			{"type": "runtime", "dockerStatus": info.DockerStatus, "dockerAvailable": info.DockerAvailable},
		}
		for _, cp := range info.Capabilities {
			capEntries = append(capEntries, map[string]any{"type": cp})
		}
		capabilitiesJSON, _ := json.Marshal(capEntries)

		nc := &store.NodeCapability{
			NodeID:          nodeID,
			BeaconVersion:   info.Version,
			OS:              info.OS,
			Architecture:    info.Architecture,
			CPUThreads:      info.CPUThreads,
			MemoryMB:        int64(info.MemoryMB),
			UptimeSeconds:   info.UptimeSeconds,
			RuntimeAvailable: info.DockerAvailable,
			RuntimeStatus:   info.DockerStatus,
			RawReport:       capabilitiesJSON,
			FetchedAt:       time.Now().UTC(),
		}
		if err := cfg.Store.UpsertNodeCapability(ctx, nc); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{
			"online":       true,
			"capabilities": nc,
		})
	})

	// POST /capabilities/ingest — Beacon capability report webhook (HMAC-authenticated)
	protected.Post("/capabilities/ingest", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var report struct {
			NodeID        string          `json:"nodeId"`
			BeaconVersion string          `json:"beaconVersion"`
			OS            string          `json:"os"`
			Architecture  string          `json:"architecture"`
			CPUThreads    int             `json:"cpuThreads"`
			MemoryMB      int64           `json:"memoryMb"`
			DiskMB        int64           `json:"diskMb"`
			UptimeSeconds int64           `json:"uptimeSeconds"`
			Capabilities  json.RawMessage `json:"capabilities"`
			FetchedAt     string          `json:"fetchedAt"`
		}
		if err := c.BodyParser(&report); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid report")
		}
		if report.NodeID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "nodeId is required")
		}

		ctx, cancel := requestContext()
		defer cancel()

		fetchedAt := time.Now().UTC()
		if report.FetchedAt != "" {
			if t, err := time.Parse(time.RFC3339, report.FetchedAt); err == nil {
				fetchedAt = t
			}
		}

		nc := &store.NodeCapability{
			NodeID:           report.NodeID,
			BeaconVersion:    report.BeaconVersion,
			OS:               report.OS,
			Architecture:     report.Architecture,
			CPUThreads:       report.CPUThreads,
			MemoryMB:         report.MemoryMB,
			DiskMB:           report.DiskMB,
			UptimeSeconds:    report.UptimeSeconds,
			RawReport:        report.Capabilities,
			FetchedAt:        fetchedAt,
		}
		if err := cfg.Store.UpsertNodeCapability(ctx, nc); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"accepted": true})
	})

	// ---- Onboarding Token Management ----

	// POST /onboarding-tokens — generate an onboarding token for a node
	protected.Post("/onboarding-tokens", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			NodeID    string `json:"nodeId"`
			TTLHours  int    `json:"ttlHours"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request")
		}
		if strings.TrimSpace(req.NodeID) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "nodeId is required")
		}
		ttl := 24 * time.Hour
		if req.TTLHours > 0 {
			ttl = time.Duration(req.TTLHours) * time.Hour
		}
		if ttl > 72*time.Hour {
			return fiber.NewError(fiber.StatusBadRequest, "ttlHours must not exceed 72")
		}

		ctx, cancel := requestContext()
		defer cancel()

		token, err := cfg.Store.CreateOnboardingToken(ctx, req.NodeID, time.Now().UTC().Add(ttl))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"token":       token.TokenHash,
			"tokenId":     token.ID,
			"nodeId":      token.NodeID,
			"expiresAt":   token.ExpiresAt.Format(time.RFC3339),
			"state":       token.State,
		})
	})

	// GET /onboarding-tokens/:tokenId — view token status
	protected.Get("/onboarding-tokens/:tokenId", requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		token, err := cfg.Store.GetOnboardingToken(ctx, c.Params("tokenId"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "token not found")
		}
		return c.JSON(token)
	})

	// POST /onboarding-tokens/:tokenId/approve — approve a pending token
	protected.Post("/onboarding-tokens/:tokenId/approve", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()

		actorID := actorIDFromCtx(c)
		if err := cfg.Store.ApproveOnboardingToken(ctx, c.Params("tokenId"), actorID); err != nil {
			return fiber.NewError(fiber.StatusConflict, err.Error())
		}
		return c.JSON(fiber.Map{"approved": true})
	})

	// POST /onboarding-tokens/:tokenId/reject
	protected.Post("/onboarding-tokens/:tokenId/reject", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Reason string `json:"reason"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.RejectOnboardingToken(ctx, c.Params("tokenId"), req.Reason); err != nil {
			return fiber.NewError(fiber.StatusConflict, err.Error())
		}
		return c.JSON(fiber.Map{"rejected": true})
	})

	// POST /onboarding-tokens/:tokenId/revoke
	protected.Post("/onboarding-tokens/:tokenId/revoke", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Reason string `json:"reason"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.RevokeOnboardingToken(ctx, c.Params("tokenId"), req.Reason); err != nil {
			return fiber.NewError(fiber.StatusConflict, err.Error())
		}
		return c.JSON(fiber.Map{"revoked": true})
	})

	// GET /onboarding-tokens — list tokens for a node
	protected.Get("/onboarding-tokens", requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		nodeID := c.Query("nodeId")
		if nodeID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "nodeId query parameter is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		tokens, err := cfg.Store.ListOnboardingTokens(ctx, nodeID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"data": tokens})
	})
}

func actorIDFromCtx(c *fiber.Ctx) string {
	if claims, ok := c.Locals("user").(tokenClaims); ok {
		return claims.Sub
	}
	return uuid.NewString()
}
