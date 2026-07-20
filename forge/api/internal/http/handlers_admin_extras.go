package http

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"

	"gamepanel/forge/internal/services/nodeprobe"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// registerAdminExtras registers admin AJAX endpoints that don't fit the
// standard CRUD or server-detail buckets. They live under the existing
// `protected` group so the auth middleware applies.
func registerAdminExtras(protected fiber.Router, cfg Config, probe *nodeprobe.Service) {
	// GET /admin/users/accounts.json?filter[email]=&page=
	// Used by select2 user search when creating a server or assigning subusers.
	// Returns: { data: [ { id, name_first, name_last, email, username, md5 } ] }
	protected.Get("/admin/users/accounts.json", requireRole("admin"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return c.JSON(fiber.Map{"data": []fiber.Map{}, "meta": fiber.Map{"pagination": fiber.Map{"total": 0, "count": 0, "per_page": 50, "current_page": 1, "total_pages": 1, "links": fiber.Map{}}}})
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
			return c.JSON(fiber.Map{"data": []fiber.Map{}, "meta": fiber.Map{"pagination": fiber.Map{"total": 0, "count": 0}}})
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
					"total":        total,
					"count":        len(data),
					"per_page":     perPage,
					"current_page": page,
					"total_pages":  totalPages,
					"links":        fiber.Map{},
				},
			},
		})
	})

	// GET /admin/nodes/view/{id}/system-information
	// Server-side proxy that pings the daemon (HMAC) and returns its info.
	protected.Get("/nodes/:id/system", requireRole("admin"), func(c *fiber.Ctx) error {
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
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":  err.Error(),
				"online": false,
			})
		}
		return c.JSON(info)
	})

	protected.Get("/nodes/:id/system-information", requireRole("admin"), func(c *fiber.Ctx) error {
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
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error":  err.Error(),
				"online": false,
			})
		}
		return c.JSON(info)
	})

	// POST /admin/nodes/view/{id}/configuration/token
	// Generates a one-shot auto-deploy token for the beacon configure command.
	protected.Post("/nodes/:id/configuration/token", requireRole("admin"), func(c *fiber.Ctx) error {
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
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
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
	protected.Post("/nodes/:id/allocations/alias", requireRole("admin"), func(c *fiber.Ctx) error {
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
	protected.Delete("/nodes/:id/allocations/bulk", requireRole("admin"), func(c *fiber.Ctx) error {
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
		ctx, cancel := requestContext()
		defer cancel()
		nodeID := c.Params("id")
		for _, a := range body.Allocations {
			if a.ID == "" {
				continue
			}
			if err := cfg.Store.DeleteNodeAllocation(ctx, nodeID, a.ID); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, err.Error())
			}
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	// DELETE /admin/nodes/view/{id}/allocation/remove/{allocationId}
	// Single allocation delete (separate endpoint from bulk).
	protected.Delete("/nodes/:id/allocations/:allocationId", requireRole("admin"), func(c *fiber.Ctx) error {
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

func generateRandomTokenHex(n int) string {
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// silenceUnusedGenerate keeps the random helper available for future use
// (e.g. signed-URL nonces in Phase 4) without an import-cycle warning.
var _ = generateRandomTokenHex

// silenceUnusedProbeImport ensures the probe package is referenced even if
// `registerAdminExtras` is called with a nil probe in unit tests.
var _ = func() *nodeprobe.Service { return nil }

// ensureStoreImport keeps the store package imported for future use.
var _ = store.PermissionDescriptions
