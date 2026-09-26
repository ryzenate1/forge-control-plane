package http

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	gpruntime "gamepanel/forge/internal/runtime"
	"gamepanel/forge/internal/services/nodeprobe"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerCapabilityRoutes(protected fiber.Router, cfg Config, nodeProbe *nodeprobe.Service) {
	// GET /workload-kinds — reports every runtime Forge models, derived from the
	// live wiring rather than a fixed list, so the create-workload form can tell
	// "not an engine we have" from "no adapter registered" from "registered but
	// no node runs it". Every unavailable answer carries a reason.
	protected.Get("/workload-kinds", func(c *fiber.Ctx) error {
		// Which engines a node can actually serve. A Beacon runs exactly one
		// runtime, so node reporting is the only honest placement signal.
		type nodeRef struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			State    string `json:"state"`
			Eligible bool   `json:"eligible"`
		}
		nodesByProvider := map[string][]nodeRef{}
		nodesRead := false
		if cfg.Store != nil {
			ctx, cancel := requestContext()
			defer cancel()
			if nodes, err := cfg.Store.ListNodes(ctx); err == nil {
				nodesRead = true
				for _, node := range nodes {
					key := gpruntime.NormalizeProvider(node.RuntimeProvider)
					if key == "" {
						key = gpruntime.DockerProvider
					}
					nodesByProvider[key] = append(nodesByProvider[key], nodeRef{
						ID:    node.ID,
						Name:  node.Name,
						State: node.ActualState,
						// Draining and maintenance hosts are excluded from
						// placement, so they must not make an engine look usable.
						Eligible: node.ActualState == "online" && !node.Draining && !node.Maintenance,
					})
				}
			}
		}

		// Which engines the dispatcher can actually route to.
		registered := map[string]gpruntime.RegisteredProvider{}
		if cfg.WorkloadRuntime != nil {
			for _, rp := range cfg.WorkloadRuntime.Registered() {
				registered[rp.Provider] = rp
			}
		}

		type kindView struct {
			Provider     string                 `json:"provider"`
			Description  string                 `json:"description"`
			Supported    bool                   `json:"supported"`
			Experimental bool                   `json:"experimental"`
			Registered   bool                   `json:"registered"`
			Available    bool                   `json:"available"`
			Reason       string                 `json:"reason,omitempty"`
			Capabilities gpruntime.Capabilities `json:"capabilities"`
			Nodes        []nodeRef              `json:"nodes"`
		}

		descriptions := map[string]string{
			gpruntime.DockerProvider:      "Containerized workloads on Docker Engine",
			gpruntime.ContainerdProvider:  "Containerized workloads on containerd",
			gpruntime.PodmanProvider:      "Containerized workloads on Podman",
			gpruntime.FirecrackerProvider: "MicroVM-isolated workloads via Firecracker",
			gpruntime.KubernetesProvider:  "Pod-managed workloads on a Kubernetes cluster",
			gpruntime.KVMProvider:         "Fully virtualized machines via QEMU/KVM",
			gpruntime.LXCProvider:         "System containers via LXC or Incus",
		}

		kinds := make([]kindView, 0, len(gpruntime.AllProviders()))
		for _, provider := range gpruntime.AllProviders() {
			view := kindView{
				Provider:     provider,
				Description:  descriptions[provider],
				Supported:    gpruntime.IsSupportedProvider(provider),
				Experimental: gpruntime.IsExperimentalProvider(provider),
				Nodes:        nodesByProvider[provider],
			}
			if view.Nodes == nil {
				view.Nodes = []nodeRef{}
			}
			if rp, ok := registered[provider]; ok {
				view.Registered = true
				view.Capabilities = rp.Capabilities
			}

			eligible := 0
			for _, node := range view.Nodes {
				if node.Eligible {
					eligible++
				}
			}

			switch {
			case !view.Supported:
				view.Reason = "refused by the provider allow-list; set ENABLE_EXPERIMENTAL_RUNTIMES to opt in"
			case !view.Registered:
				view.Reason = "no adapter is wired into the runtime dispatcher"
			case !nodesRead:
				// Unknown is not "none": say the node inventory could not be read
				// rather than reporting an engine as unavailable.
				view.Reason = "node inventory unavailable, so placement cannot be determined"
			case eligible == 0:
				if len(view.Nodes) > 0 {
					view.Reason = "only hosts running it are draining, in maintenance, or offline"
				} else {
					view.Reason = "no node reports this runtime"
				}
			default:
				view.Available = true
			}
			kinds = append(kinds, view)
		}
		return c.JSON(fiber.Map{"data": kinds})
	})

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
			return respondInternalError(c, err)
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

	// GET /capabilities/:nodeId/delta — drift between last two snapshots (amber-banner source)
	protected.Get("/capabilities/:nodeId/delta", requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		nodeID := c.Params("nodeId")
		ctx, cancel := requestContext()
		defer cancel()
		entries, err := cfg.Store.GetCapabilityHistory(ctx, nodeID, 2)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if len(entries) == 0 {
			return fiber.NewError(fiber.StatusNotFound, "capability not found")
		}
		// Single snapshot → everything is "added" (no baseline) — honest signal
		if len(entries) == 1 {
			var cur []map[string]any
			_ = json.Unmarshal(entries[0].Capabilities, &cur)
			if cur == nil {
				cur = []map[string]any{}
			}
			return c.JSON(fiber.Map{
				"nodeId":    nodeID,
				"fetchedAt": entries[0].ObservedAt.Format(time.RFC3339),
				"added":     cur,
				"removed":   []any{},
				"changed":   []any{},
				"unchanged": []any{},
			})
		}
		newest := entries[0]
		previous := entries[1]
		var curCaps []map[string]any
		var prevCaps []map[string]any
		_ = json.Unmarshal(newest.Capabilities, &curCaps)
		_ = json.Unmarshal(previous.Capabilities, &prevCaps)
		// Normalize nil to empty
		if curCaps == nil {
			curCaps = []map[string]any{}
		}
		if prevCaps == nil {
			prevCaps = []map[string]any{}
		}
		typeKey := func(m map[string]any) string {
			if t, ok := m["type"].(string); ok && t != "" {
				return t
			}
			b, _ := json.Marshal(m)
			return string(b)
		}
		prevMap := make(map[string]map[string]any, len(prevCaps))
		for _, m := range prevCaps {
			prevMap[typeKey(m)] = m
		}
		curMap := make(map[string]map[string]any, len(curCaps))
		for _, m := range curCaps {
			curMap[typeKey(m)] = m
		}
		var added, removed, changed, unchanged []any
		for _, m := range curCaps {
			k := typeKey(m)
			if old, ok := prevMap[k]; !ok {
				added = append(added, m)
			} else {
				a, _ := json.Marshal(old)
				b, _ := json.Marshal(m)
				if string(a) != string(b) {
					changed = append(changed, m)
				} else {
					unchanged = append(unchanged, m)
				}
			}
		}
		for _, m := range prevCaps {
			k := typeKey(m)
			if _, ok := curMap[k]; !ok {
				removed = append(removed, m)
			}
		}
		if added == nil {
			added = []any{}
		}
		if removed == nil {
			removed = []any{}
		}
		if changed == nil {
			changed = []any{}
		}
		if unchanged == nil {
			unchanged = []any{}
		}
		return c.JSON(fiber.Map{
			"nodeId":    nodeID,
			"fetchedAt": newest.ObservedAt.Format(time.RFC3339),
			"added":     added,
			"removed":   removed,
			"changed":   changed,
			"unchanged": unchanged,
		})
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
			NodeID:           nodeID,
			BeaconVersion:    info.Version,
			OS:               info.OS,
			Architecture:     info.Architecture,
			CPUThreads:       info.CPUThreads,
			MemoryMB:         int64(info.MemoryMB),
			UptimeSeconds:    info.DaemonUptimeSeconds,
			RuntimeAvailable: info.DockerAvailable,
			RuntimeStatus:    info.DockerStatus,
			RawReport:        capabilitiesJSON,
			FetchedAt:        time.Now().UTC(),
		}
		if err := cfg.Store.UpsertNodeCapability(ctx, nc); err != nil {
			return respondInternalError(c, err)
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
			Signature     string          `json:"signature"`
		}
		if err := c.BodyParser(&report); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid report")
		}
		if report.NodeID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "nodeId is required")
		}

		ctx, cancel := requestContext()
		defer cancel()

		token, err := cfg.Store.GetNodeDaemonToken(ctx, report.NodeID)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "node authentication failed")
		}

		mac := hmac.New(sha256.New, []byte(token))
		mac.Write([]byte(report.NodeID))
		expectedSig := hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(report.Signature), []byte(expectedSig)) {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid node signature")
		}

		fetchedAt := time.Now().UTC()
		if report.FetchedAt != "" {
			if t, err := time.Parse(time.RFC3339, report.FetchedAt); err == nil {
				fetchedAt = t
			}
		}

		nc := &store.NodeCapability{
			NodeID:        report.NodeID,
			BeaconVersion: report.BeaconVersion,
			OS:            report.OS,
			Architecture:  report.Architecture,
			CPUThreads:    report.CPUThreads,
			MemoryMB:      report.MemoryMB,
			DiskMB:        report.DiskMB,
			UptimeSeconds: report.UptimeSeconds,
			RawReport:     report.Capabilities,
			FetchedAt:     fetchedAt,
		}
		if err := cfg.Store.UpsertNodeCapability(ctx, nc); err != nil {
			return respondInternalError(c, err)
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
			NodeID   string `json:"nodeId"`
			TTLHours int    `json:"ttlHours"`
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
			"token":     token.PlainToken,
			"tokenId":   token.ID,
			"nodeId":    token.NodeID,
			"expiresAt": token.ExpiresAt.Format(time.RFC3339),
			"state":     token.State,
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
	return "system"
}
