package http

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	gpruntime "gamepanel/forge/internal/runtime"
	"gamepanel/forge/internal/services/nodeprobe"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// capabilityStore is the surface these handlers need from the layer beneath
// them. Both registrations.Service (the intended handler -> service -> store
// path, which counts the metrics Config already exposes a field for) and
// *store.Store satisfy it, so the routes use the service as soon as the
// container builds one and keep working against a bare store in dev trees.
type capabilityStore interface {
	CreateOnboardingToken(ctx context.Context, nodeID string, expiresAt time.Time) (*store.OnboardingToken, error)
	GetOnboardingToken(ctx context.Context, tokenID string) (*store.OnboardingToken, error)
	ApproveOnboardingToken(ctx context.Context, tokenID, approvedBy string) error
	RejectOnboardingToken(ctx context.Context, tokenID, reason string) error
	RevokeOnboardingToken(ctx context.Context, tokenID, reason string) error
	ListOnboardingTokens(ctx context.Context, nodeID string) ([]store.OnboardingToken, error)
	UpsertNodeCapability(ctx context.Context, nc *store.NodeCapability) error
	GetNodeCapability(ctx context.Context, nodeID string) (*store.NodeCapability, error)
	ListCapabilities(ctx context.Context, filter store.CapabilityInventoryFilter) ([]store.NodeCapability, error)
	GetCapabilityHistory(ctx context.Context, nodeID string, limit int) ([]store.NodeCapabilityHistoryEntry, error)
}

// capabilityBackend resolves the backing layer for a capability or onboarding
// route. ok is false only when neither the service nor a store exists, which
// every caller turns into a 503 rather than an empty answer.
func capabilityBackend(cfg Config) (capabilityStore, bool) {
	if cfg.RegistrationService != nil {
		return cfg.RegistrationService, true
	}
	if cfg.Store != nil {
		return cfg.Store, true
	}
	return nil, false
}

func registerCapabilityRoutes(protected fiber.Router, cfg Config, nodeProbe *nodeprobe.Service) {
	// GET /workload-kinds — reports every runtime Forge models, derived from the
	// live wiring rather than a fixed list, so the create-workload form can tell
	// "not an engine we have" from "no adapter registered" from "registered but
	// no node runs it". Every unavailable answer carries a reason.
	//
	// The response embeds the fleet inventory (node id, name, actual state per
	// runtime), so it is a node read and gated as one: the only consumer is the
	// admin servers page, and an unauthenticated-to-nodes caller must not be able
	// to enumerate which machines exist and what state they are in.
	protected.Get("/workload-kinds", requireRole("admin"), requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
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
		backend, ok := capabilityBackend(cfg)
		if !ok {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capability and onboarding backend is not wired")
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
		caps, err := backend.ListCapabilities(ctx, store.CapabilityInventoryFilter{Offset: offset, Limit: limit})
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": caps})
	})

	// GET /capabilities/:nodeId — single node capability detail
	protected.Get("/capabilities/:nodeId", requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		backend, ok := capabilityBackend(cfg)
		if !ok {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capability and onboarding backend is not wired")
		}
		ctx, cancel := requestContext()
		defer cancel()
		nc, err := backend.GetNodeCapability(ctx, c.Params("nodeId"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "capability not found")
		}
		return c.JSON(nc)
	})

	// GET /capabilities/:nodeId/history — capability change history
	protected.Get("/capabilities/:nodeId/history", requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		backend, ok := capabilityBackend(cfg)
		if !ok {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capability and onboarding backend is not wired")
		}
		limit := 20
		if l := c.Query("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
				limit = v
			}
		}
		ctx, cancel := requestContext()
		defer cancel()
		entries, err := backend.GetCapabilityHistory(ctx, c.Params("nodeId"), limit)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"data": entries})
	})

	// GET /capabilities/:nodeId/delta — drift between last two snapshots (amber-banner source)
	protected.Get("/capabilities/:nodeId/delta", requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		backend, ok := capabilityBackend(cfg)
		if !ok {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capability and onboarding backend is not wired")
		}
		nodeID := c.Params("nodeId")
		ctx, cancel := requestContext()
		defer cancel()
		entries, err := backend.GetCapabilityHistory(ctx, nodeID, 2)
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
	protected.Post("/capabilities/:nodeId/probe", requireRole("admin"), requireAdminScope("nodes.write"), func(c *fiber.Ctx) error {
		backend, ok := capabilityBackend(cfg)
		if !ok {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capability and onboarding backend is not wired")
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
		if err := backend.UpsertNodeCapability(ctx, nc); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{
			"online":       true,
			"capabilities": nc,
		})
	})

	// POST /capabilities/ingest is NOT registered, deliberately.
	//
	// It was a Beacon-facing webhook that no Beacon sends: nothing in
	// beacon/internal or internal/daemon posts to /capabilities/ingest, and it sat
	// on the user-session `protected` group, which node credentials cannot
	// authenticate against (nodes talk back over /api/remote). Its signature was
	// also unsound in two ways: it keyed on report.NodeID alone, so the HMAC
	// covered no byte of the CPU/memory/OS payload and one captured signature
	// could be replayed with invented capacity to poison placement; and it derived
	// that HMAC from GetNodeDaemonToken, while the platform's node credential is
	// the composite tokenID.token from GetNodeDaemonCredential, so a correctly
	// signed real report could never have verified either. The panel-initiated
	// POST /capabilities/:nodeId/probe below is the live capability path and writes
	// the same row. Re-adding ingest means: mount it on /api/remote behind the
	// node guard, sign method+URI+timestamp+body with the composite credential,
	// and reject a report whose body is not covered by the signature.

	// ---- Onboarding Token Management ----

	// POST /onboarding-tokens — generate an onboarding token for a node
	protected.Post("/onboarding-tokens", requireRole("admin"), requireAdminScope("nodes.write"), func(c *fiber.Ctx) error {
		backend, ok := capabilityBackend(cfg)
		if !ok {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capability and onboarding backend is not wired")
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

		token, err := backend.CreateOnboardingToken(ctx, req.NodeID, time.Now().UTC().Add(ttl))
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
		backend, ok := capabilityBackend(cfg)
		if !ok {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capability and onboarding backend is not wired")
		}
		ctx, cancel := requestContext()
		defer cancel()
		token, err := backend.GetOnboardingToken(ctx, c.Params("tokenId"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "token not found")
		}
		return c.JSON(token)
	})

	// POST /onboarding-tokens/:tokenId/approve — approve a pending token
	protected.Post("/onboarding-tokens/:tokenId/approve", requireRole("admin"), requireAdminScope("nodes.write"), func(c *fiber.Ctx) error {
		backend, ok := capabilityBackend(cfg)
		if !ok {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capability and onboarding backend is not wired")
		}
		ctx, cancel := requestContext()
		defer cancel()

		actorID := actorIDFromCtx(c)
		if err := backend.ApproveOnboardingToken(ctx, c.Params("tokenId"), actorID); err != nil {
			return fiber.NewError(fiber.StatusConflict, err.Error())
		}
		return c.JSON(fiber.Map{"approved": true})
	})

	// POST /onboarding-tokens/:tokenId/reject
	protected.Post("/onboarding-tokens/:tokenId/reject", requireRole("admin"), requireAdminScope("nodes.write"), func(c *fiber.Ctx) error {
		backend, ok := capabilityBackend(cfg)
		if !ok {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capability and onboarding backend is not wired")
		}
		var req struct {
			Reason string `json:"reason"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := backend.RejectOnboardingToken(ctx, c.Params("tokenId"), req.Reason); err != nil {
			return fiber.NewError(fiber.StatusConflict, err.Error())
		}
		return c.JSON(fiber.Map{"rejected": true})
	})

	// POST /onboarding-tokens/:tokenId/revoke
	protected.Post("/onboarding-tokens/:tokenId/revoke", requireRole("admin"), requireAdminScope("nodes.write"), func(c *fiber.Ctx) error {
		backend, ok := capabilityBackend(cfg)
		if !ok {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capability and onboarding backend is not wired")
		}
		var req struct {
			Reason string `json:"reason"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := backend.RevokeOnboardingToken(ctx, c.Params("tokenId"), req.Reason); err != nil {
			return fiber.NewError(fiber.StatusConflict, err.Error())
		}
		return c.JSON(fiber.Map{"revoked": true})
	})

	// GET /onboarding-tokens — list tokens for a node
	protected.Get("/onboarding-tokens", requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		backend, ok := capabilityBackend(cfg)
		if !ok {
			return fiber.NewError(fiber.StatusServiceUnavailable, "capability and onboarding backend is not wired")
		}
		nodeID := c.Query("nodeId")
		if nodeID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "nodeId query parameter is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		tokens, err := backend.ListOnboardingTokens(ctx, nodeID)
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
