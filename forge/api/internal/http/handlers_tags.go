package http

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tagssvc "gamepanel/forge/internal/services/tags"

	"github.com/gofiber/fiber/v2"
)

// Tags API: a global catalog of color-coded labels plus per-resource assignment
// routes and tag-driven bulk operations. Layering is handler -> tags.Service ->
// store; the handler never queries the store directly. Wiring happens through a
// phase registrar so this feature does not need edits to server.go/main.go.

const tagsRegistrarName = "tags-and-assignments"

func init() {
	RegisterPhaseRegistrar(tagsRegistrarName, 200, registerTagRoutesPhase)
}

// resourceTypeFromSegment maps a URL resource segment (e.g. "servers") to the
// canonical tag resource type (e.g. "server"). Only these three kinds are
// taggable; anything else fails closed.
func resourceTypeFromSegment(segment string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(segment)) {
	case "applications", "app", "application":
		return string(tagssvc.ResourceApplication), true
	case "servers", "server":
		return string(tagssvc.ResourceServer), true
	case "environments", "environment":
		return string(tagssvc.ResourceEnvironment), true
	default:
		return "", false
	}
}

func registerTagRoutesPhase(v1 fiber.Router, protected fiber.Router, cfg *Config) error {
	if cfg.Store == nil {
		if cfg.Logger != nil {
			cfg.Logger.Warn(tagsRegistrarName + ": store not configured, tag routes skipped")
		}
		return nil
	}
	svc := tagssvc.New(cfg.Store)
	limiter := RateLimiter(GetRateLimitForEndpoint("mutation", cfg.Redis, cfg.RedisEnabled && strings.EqualFold(strings.TrimSpace(cfg.AppEnv), "production")))
	registerTagRoutes(protected, svc, limiter, *cfg)
	return nil
}

// registerTagRoutes wires the /tags catalog, per-resource assignment routes, and
// bulk operations onto the authenticated router.
func registerTagRoutes(protected fiber.Router, svc *tagssvc.Service, mutationLimiter fiber.Handler, cfg Config) {
	// ---- Catalog ----
	protected.Get("/tags", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		list, err := svc.ListTags(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": list})
	})

	protected.Get("/tags/:tagId", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		tag, err := svc.GetTag(ctx, c.Params("tagId"))
		if err != nil {
			return tagError(c, err)
		}
		return c.JSON(tag)
	})

	protected.Post("/tags", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		var req tagssvc.CreateRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		tag, err := svc.CreateTag(ctx, req)
		if err != nil {
			return tagError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(tag)
	})

	protected.Patch("/tags/:tagId", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		var req tagssvc.UpdateRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		tag, err := svc.UpdateTag(ctx, c.Params("tagId"), req)
		if err != nil {
			return tagError(c, err)
		}
		return c.JSON(tag)
	})

	protected.Delete("/tags/:tagId", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.DeleteTag(ctx, c.Params("tagId")); err != nil {
			return tagError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// Resources carrying a tag (mixed kinds, or ?resourceType= to narrow).
	protected.Get("/tags/:tagId/resources", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		resources, err := svc.GetResourcesByTag(ctx, c.Params("tagId"), c.Query("resourceType"))
		if err != nil {
			return tagError(c, err)
		}
		return c.JSON(fiber.Map{"data": resources})
	})

	// Bulk action over everything tagged. Admin-only: it dispatches real power
	// operations against many resources at once.
	protected.Post("/tags/:tagId/bulk", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		var req struct {
			ResourceType string `json:"resourceType"`
			Action       string `json:"action"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		action := strings.ToLower(strings.TrimSpace(req.Action))
		switch action {
		case "start", "stop", "restart", "deploy":
		default:
			return fiber.NewError(fiber.StatusBadRequest, "action must be one of: start, stop, restart, deploy")
		}
		ctx, cancel := longRequestContext()
		defer cancel()
		resources, err := svc.GetResourcesByTag(ctx, c.Params("tagId"), req.ResourceType)
		if err != nil {
			return tagError(c, err)
		}
		type bulkResult struct {
			ResourceType string `json:"resourceType"`
			ResourceID   string `json:"resourceId"`
			Name         string `json:"name"`
			OK           bool   `json:"ok"`
			Error        string `json:"error,omitempty"`
		}
		results := make([]bulkResult, 0, len(resources))
		for _, r := range resources {
			out := bulkResult{ResourceType: r.ResourceType, ResourceID: r.ResourceID, Name: r.Name}
			err := bulkDispatch(cfg, ctx, r, action)
			if err != nil {
				out.Error = err.Error()
			} else {
				out.OK = true
			}
			results = append(results, out)
		}
		return c.JSON(fiber.Map{"data": results, "total": len(results)})
	})

	// ---- Per-resource assignment ----
	// Registered per concrete kind so we never mount a root-level wildcard that
	// could shadow existing static routes.
	for _, seg := range []string{"applications", "servers", "environments"} {
		resourceType, ok := resourceTypeFromSegment(seg)
		if !ok {
			continue
		}
		prefix := "/" + seg + "/:resourceId/tags"
		protected.Get(prefix, func(c *fiber.Ctx) error {
			ctx, cancel := requestContext()
			defer cancel()
			list, err := svc.GetResourceTags(ctx, resourceType, c.Params("resourceId"))
			if err != nil {
				return tagError(c, err)
			}
			return c.JSON(fiber.Map{"data": list})
		})
		protected.Post(prefix, mutationLimiter, func(c *fiber.Ctx) error {
			var req struct {
				TagID string `json:"tagId"`
			}
			if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.TagID) == "" {
				return fiber.NewError(fiber.StatusBadRequest, "tagId is required")
			}
			ctx, cancel := requestContext()
			defer cancel()
			if err := svc.AssignTag(ctx, resourceType, c.Params("resourceId"), req.TagID); err != nil {
				return tagError(c, err)
			}
			return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true})
		})
		protected.Delete(prefix+"/:tagId", mutationLimiter, func(c *fiber.Ctx) error {
			ctx, cancel := requestContext()
			defer cancel()
			if err := svc.UnassignTag(ctx, resourceType, c.Params("resourceId"), c.Params("tagId")); err != nil {
				return tagError(c, err)
			}
			return c.JSON(fiber.Map{"ok": true})
		})
	}
}

// bulkDispatch resolves a single tagged resource to a real runtime action.
// Servers go through the durable operation -> Beacon -> Docker power path;
// applications reuse the app-hosting lifecycle helper (which drives the bound
// server). Environments have no runtime action and fail closed.
func bulkDispatch(cfg Config, ctx context.Context, r tagssvc.TaggedResource, action string) error {
	switch r.ResourceType {
	case string(tagssvc.ResourceServer):
		if cfg.OperationService == nil {
			return fmt.Errorf("operation service unavailable")
		}
		if _, err := cfg.OperationService.DispatchPower(ctx, r.ResourceID, action, ""); err != nil {
			return err
		}
		return nil
	case string(tagssvc.ResourceApplication):
		if cfg.Store == nil {
			return fmt.Errorf("postgres is required")
		}
		app, err := cfg.Store.GetApplication(ctx, r.ResourceID)
		if err != nil || app == nil {
			return fmt.Errorf("application not found")
		}
		return appLifecycleAction(cfg, ctx, *app, action)
	default:
		return fmt.Errorf("no bulk action is defined for %s resources", r.ResourceType)
	}
}

// tagError maps service sentinel errors onto honest HTTP statuses.
func tagError(c *fiber.Ctx, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, tagssvc.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, tagssvc.ErrDuplicateName):
		return fiber.NewError(fiber.StatusConflict, err.Error())
	case errors.Is(err, tagssvc.ErrInvalidResourceType),
		errors.Is(err, tagssvc.ErrEmptyName),
		errors.Is(err, tagssvc.ErrNameTooLong),
		errors.Is(err, tagssvc.ErrInvalidColor),
		errors.Is(err, tagssvc.ErrEmptyResourceID):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	default:
		return respondInternalError(c, err)
	}
}
