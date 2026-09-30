package http

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"gamepanel/forge/internal/services/nodeprobe"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// registerAdminExtras registers admin AJAX endpoints that don't fit the
// standard CRUD or server-detail buckets. They live under the existing
// `protected` group so the auth middleware applies.
//
// Every route here is an admin-only node or user accessor, so each one carries
// the same explicit scope its canonical sibling in registerAdminRoutes carries:
// reads nodes.read, writes nodes.write, the user directory users.read. The
// credential-minting endpoint also takes the admin IP allowlist and the
// mutation limiter, which the /nodes/:id/rotate-token route it mirrors has
// always required.
func registerAdminExtras(protected fiber.Router, cfg Config, probe *nodeprobe.Service, adminIPAccess, mutationLimiter fiber.Handler) {
	// GET /admin/users/accounts.json?filter[email]=&page=
	// Used by select2 user search when creating a server or assigning subusers.
	// Returns: { data: [ { id, name_first, name_last, email, username, md5 } ] }
	//
	// A failed user search is an error, not an empty directory: this used to
	// answer 200 with data: [] and total: 0 whenever the store was missing or the
	// query failed, so the picker rendered "no such user" for a database outage
	// and operators created duplicate accounts. Unknown is not zero.
	protected.Get("/admin/users/accounts.json", requireRole("admin"), requireAdminScope("users.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		filter := strings.TrimSpace(c.Query("filter[email]", c.Query("filter", "")))
		page, _ := strconv.Atoi(c.Query("page", "1"))
		if page < 1 {
			page = 1
		}
		perPage, _ := strconv.Atoi(c.Query("per_page", "50"))
		if perPage < 1 || perPage > 200 {
			perPage = 50
		}
		users, total, err := cfg.Store.SearchUsers(ctx, filter, page, perPage)
		if err != nil {
			return respondInternalError(c, err)
		}
		data := make([]fiber.Map, 0, len(users))
		for _, u := range users {
			md5sum := md5OfEmail(u.Email)
			data = append(data, fiber.Map{
				"id":         u.ID,
				"email":      u.Email,
				"username":   u.Username,
				"name_first": u.NameFirst,
				"name_last":  u.NameLast,
				"md5":        md5sum,
			})
		}
		totalPages := (total + perPage - 1) / perPage
		return c.JSON(fiber.Map{
			"data": data,
			"meta": fiber.Map{
				"pagination": fiber.Map{
					"total":         total,
					"count":         len(data),
					"per_page":      perPage,
					"current":       page,
					"total_records": total,
					"current_page":  page,
					"total_pages":   totalPages,
					"links":         fiber.Map{},
				},
			},
		})
	})

	// GET /admin/nodes/view/{id}/system-information
	// Server-side proxy that pings the daemon (HMAC) and returns its info.
	//
	// Both spellings of this route answer the same probe. The error from a probe
	// is reported as a failure of the probe, not as the node's own answer, and the
	// raw store/daemon error text is logged rather than echoed to the client.
	protected.Get("/nodes/:id/system", requireRole("admin"), requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		return nodeSystemInformation(c, probe)
	})

	protected.Get("/nodes/:id/system-information", requireRole("admin"), requireAdminScope("nodes.read"), func(c *fiber.Ctx) error {
		return nodeSystemInformation(c, probe)
	})

	// POST /admin/nodes/view/{id}/configuration/token
	// Generates a one-shot auto-deploy token for the beacon configure command.
	//
	// This mints a live node credential, so it is gated exactly like its sibling
	// /nodes/:id/rotate-token: admin role, nodes.write scope, admin IP allowlist
	// and the mutation limiter.
	protected.Post("/nodes/:id/configuration/token", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("nodes.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		node, err := cfg.Store.GetNode(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "node not found")
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		// Compatibility endpoint: mint by rotating rather than re-disclosing an
		// existing secret. Consumers receive the complete credential exactly once.
		token, err := cfg.Store.RotateNodeToken(ctx, node.ID, actorID)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{
			"token":  token,
			"node":   node.ID,
			"fqdn":   node.FQDN,
			"scheme": node.Scheme,
		})
	})

	// POST /admin/nodes/view/{id}/allocation/alias
	// Body: { allocation_id, alias }
	protected.Post("/nodes/:id/allocations/alias", mutationLimiter, requireRole("admin"), requireAdminScope("allocations.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var body struct {
			AllocationID string `json:"allocation_id"`
			Alias        string `json:"alias"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if body.AllocationID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "allocation_id is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.UpdateAllocationAlias(ctx, body.AllocationID, body.Alias); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	// DELETE /admin/nodes/view/{id}/allocations
	// Body: { allocations: [{id: N}, ...] }
	protected.Delete("/nodes/:id/allocations/bulk", mutationLimiter, requireRole("admin"), requireAdminScope("allocations.delete"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var body struct {
			Allocations []struct {
				ID string `json:"id"`
			} `json:"allocations"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if len(body.Allocations) == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "allocations list is required")
		}
		// An entry without an id is a malformed request, not an entry with
		// nothing to do. Skipping it used to answer 204 for a batch that never
		// touched those allocations, so the caller believed they were gone.
		ids := make([]string, 0, len(body.Allocations))
		for _, a := range body.Allocations {
			if strings.TrimSpace(a.ID) == "" {
				return fiber.NewError(fiber.StatusBadRequest, "every allocation entry must carry an id")
			}
			ids = append(ids, a.ID)
		}
		ctx, cancel := requestContext()
		defer cancel()
		nodeID := c.Params("id")
		// Deleting stops at the first failure and says which allocation failed:
		// a partially applied batch is reported as a failure, never as 204.
		for _, id := range ids {
			if err := cfg.Store.DeleteNodeAllocation(ctx, nodeID, id); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("allocation %s: %s", id, err.Error()))
			}
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	// DELETE /admin/nodes/view/{id}/allocation/remove/{allocationId}
	// Single allocation delete (separate endpoint from bulk).
	protected.Delete("/nodes/:id/allocations/:allocationId", mutationLimiter, requireRole("admin"), requireAdminScope("allocations.delete"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteNodeAllocation(ctx, c.Params("id"), c.Params("allocationId")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
}

// nodeSystemInformation answers both /nodes/:id/system routes. A missing probe
// service is 503 (nothing could ask the node), and a failed probe is reported
// as a failed probe with online: false; the daemon's raw error text is logged
// rather than echoed to the client.
func nodeSystemInformation(c *fiber.Ctx, probe *nodeprobe.Service) error {
	if probe == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"error":  "node probe unavailable",
			"online": false,
		})
	}
	ctx, cancel := requestContext()
	defer cancel()
	info, err := probe.ProbeNode(ctx, c.Params("id"))
	if err != nil {
		// Keep the documented {error, online:false} shape for the node view, but
		// log the real cause server-side and echo only a generic reason.
		logInternalError(c, err)
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":  "node probe failed",
			"online": false,
		})
	}
	return c.JSON(info)
}

// md5OfEmail returns a 32-char hex md5 of the email for the Gravatar URL hint
// on the admin UI. Implementation matches `hash('md5', strtolower(trim($email)))`.
func md5OfEmail(email string) string {
	e := strings.ToLower(strings.TrimSpace(email))
	if e == "" {
		return ""
	}
	// We avoid importing crypto/md5 at the package level to keep the
	// dependency surface small; use a small manual md5 (RFC 1321). Standard
	// library has crypto/md5 — use it.
	return md5Hex(e)
}

// generateRandomTokenHex returns n random bytes as hex. Entropy failure is
// returned, never masked with a deterministic fallback: callers must fail the
// request rather than mint a guessable token.
func generateRandomTokenHex(n int) (string, error) {
	if n <= 0 {
		return "", fmt.Errorf("token length must be positive")
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// silenceUnusedGenerate keeps the random helper available for future use
// (e.g. signed-URL nonces in Phase 4) without an import-cycle warning.
var _ = generateRandomTokenHex

// silenceUnusedProbeImport ensures the probe package is referenced even if
// `registerAdminExtras` is called with a nil probe in unit tests.
var _ = func() *nodeprobe.Service { return nil }

// ensureStoreImport keeps the store package imported for future use.
var _ = store.PermissionDescriptions
