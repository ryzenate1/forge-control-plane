package http

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	dockerleanupsvc "gamepanel/forge/internal/services/dockerleanup"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// Admin Docker disk-usage & automated cleanup surface. Layered as
// handler -> dockerleanup.Service -> (Beacon over signed channel | policy
// store). All routes are admin-gated (session role + API scope) and read-only
// queries use the servers.read scope while every prune / policy mutation uses
// servers.write so a scoped API key cannot trigger destructive pruning.

const dockerCleanupRegistrarName = "docker-cleanup"

// dockerCleanupStartOnce guards the background scheduler so repeated route
// registration (e.g. in tests) never launches more than one leader loop.
var dockerCleanupStartOnce sync.Once

func init() {
	RegisterPhaseRegistrar(dockerCleanupRegistrarName, 215, registerDockerCleanupRoutes)
}

// nodeResolver adapts the node store to dockerleanup.NodeResolver. The API is
// the only party that ever sees the node daemon credential; the service is
// handed an opaque NodeTarget and forwards it to the Beacon signer.
type nodeResolver struct {
	st *store.Store
}

func (r nodeResolver) All(ctx context.Context) ([]dockerleanupsvc.NodeTarget, error) {
	nodes, err := r.st.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	targets := make([]dockerleanupsvc.NodeTarget, 0, len(nodes))
	for _, n := range nodes {
		if strings.TrimSpace(n.BaseURL) == "" {
			continue
		}
		token, err := r.st.GetNodeDaemonCredential(ctx, n.ID)
		if err != nil {
			// A node without a usable credential is skipped rather than
			// aborting the whole fleet sweep.
			continue
		}
		targets = append(targets, dockerleanupsvc.NodeTarget{ID: n.ID, Name: n.Name, URL: n.BaseURL, Token: token})
	}
	return targets, nil
}

func (r nodeResolver) Resolve(ctx context.Context, nodeID string) (dockerleanupsvc.NodeTarget, error) {
	node, err := r.st.GetNode(ctx, nodeID)
	if err != nil {
		return dockerleanupsvc.NodeTarget{}, err
	}
	token, err := r.st.GetNodeDaemonCredential(ctx, node.ID)
	if err != nil {
		return dockerleanupsvc.NodeTarget{}, fmt.Errorf("node credential unavailable: %w", err)
	}
	return dockerleanupsvc.NodeTarget{ID: node.ID, Name: node.Name, URL: node.BaseURL, Token: token}, nil
}

func registerDockerCleanupRoutes(_ fiber.Router, protected fiber.Router, cfg *Config) error {
	if cfg.Store == nil || cfg.Daemon == nil {
		return fmt.Errorf("%w: store or daemon not configured, docker cleanup routes skipped", ErrPhaseSkipped)
	}

	svc, err := dockerleanupsvc.New(cfg.Store, cfg.Daemon, nodeResolver{st: cfg.Store}, cfg.Logger, dockerCleanupInstanceID())
	if err != nil {
		return err
	}

	// Launch the leader-elected scheduler loop exactly once for the process.
	dockerCleanupStartOnce.Do(func() {
		if err := svc.Start(context.Background()); err != nil && cfg.Logger != nil {
			cfg.Logger.Error("failed to start docker cleanup scheduler", "error", err.Error())
		}
	})

	admin := protected.Group("/admin/docker-cleanup", requireRole("admin"))
	limiter := RateLimiter(GetRateLimitForEndpoint("mutation", cfg.Redis, cfg.RedisEnabled && strings.EqualFold(strings.TrimSpace(cfg.AppEnv), "production")))
	registerDockerCleanupRoutesOn(admin, *cfg, svc, limiter)
	return nil
}

func registerDockerCleanupRoutesOn(admin fiber.Router, cfg Config, svc *dockerleanupsvc.Service, mutationLimiter fiber.Handler) {
	// --- disk usage & unused images (read) ---
	admin.Get("/disk-usage/:nodeId", requireAdminScope("servers.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		usage, err := svc.GetDiskUsage(ctx, c.Params("nodeId"))
		if err != nil {
			return dockerCleanupError(c, err)
		}
		return c.JSON(usage)
	})

	admin.Get("/unused-images/:nodeId", requireAdminScope("servers.read"), func(c *fiber.Ctx) error {
		limit := 1
		if raw := c.Query("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 {
				return fiber.NewError(fiber.StatusBadRequest, "limit must be a non-negative integer")
			}
			limit = n
		}
		ctx, cancel := requestContext()
		defer cancel()
		images, err := svc.ListUnusedImages(ctx, c.Params("nodeId"), limit)
		if err != nil {
			return dockerCleanupError(c, err)
		}
		return c.JSON(fiber.Map{"data": images})
	})

	// --- manual prune actions (write) ---
	admin.Post("/prune/images", mutationLimiter, requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		var req struct {
			NodeID   string   `json:"nodeId"`
			ImageIDs []string `json:"imageIds"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		res, err := svc.PruneImages(ctx, req.NodeID, req.ImageIDs)
		if err != nil {
			return dockerCleanupError(c, err)
		}
		recordAudit(cfg, c, "docker:prune-images", "node", &req.NodeID, nil)
		return c.JSON(res)
	})

	admin.Post("/prune/build-cache/:nodeId", mutationLimiter, requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		nodeID := c.Params("nodeId")
		ctx, cancel := requestContext()
		defer cancel()
		res, err := svc.PruneBuildCache(ctx, nodeID)
		if err != nil {
			return dockerCleanupError(c, err)
		}
		recordAudit(cfg, c, "docker:prune-build-cache", "node", &nodeID, nil)
		return c.JSON(res)
	})

	admin.Post("/prune/volumes/:nodeId", mutationLimiter, requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		nodeID := c.Params("nodeId")
		ctx, cancel := requestContext()
		defer cancel()
		res, err := svc.PruneVolumes(ctx, nodeID)
		if err != nil {
			return dockerCleanupError(c, err)
		}
		recordAudit(cfg, c, "docker:prune-volumes", "node", &nodeID, nil)
		return c.JSON(res)
	})

	// --- policies CRUD ---
	admin.Get("/policies", requireAdminScope("servers.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		policies, err := svc.ListPolicies(ctx)
		if err != nil {
			return dockerCleanupError(c, err)
		}
		return c.JSON(fiber.Map{"data": policies})
	})

	admin.Post("/policies", mutationLimiter, requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		var req struct {
			NodeID          string `json:"nodeId"`
			Schedule        string `json:"schedule"`
			MostRecentLimit int    `json:"mostRecentLimit"`
			Enabled         *bool  `json:"enabled"`
			PruneBuildCache *bool  `json:"pruneBuildCache"`
			PruneVolumes    *bool  `json:"pruneVolumes"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		var createdBy string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			createdBy = claims.Sub
		}
		ctx, cancel := requestContext()
		defer cancel()
		created, err := svc.CreatePolicy(ctx, dockerleanupsvc.CreatePolicyInput{
			NodeID:          req.NodeID,
			Schedule:        req.Schedule,
			MostRecentLimit: req.MostRecentLimit,
			Enabled:         req.Enabled,
			PruneBuildCache: req.PruneBuildCache,
			PruneVolumes:    req.PruneVolumes,
			CreatedBy:       createdBy,
		})
		if err != nil {
			return dockerCleanupError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(created)
	})

	admin.Patch("/policies/:id", mutationLimiter, requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		var req struct {
			NodeID          *string `json:"nodeId"`
			Schedule        *string `json:"schedule"`
			MostRecentLimit *int    `json:"mostRecentLimit"`
			Enabled         *bool   `json:"enabled"`
			PruneBuildCache *bool   `json:"pruneBuildCache"`
			PruneVolumes    *bool   `json:"pruneVolumes"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		updated, err := svc.UpdatePolicy(ctx, c.Params("id"), dockerleanupsvc.UpdatePolicyInput{
			NodeID:          req.NodeID,
			Schedule:        req.Schedule,
			MostRecentLimit: req.MostRecentLimit,
			Enabled:         req.Enabled,
			PruneBuildCache: req.PruneBuildCache,
			PruneVolumes:    req.PruneVolumes,
		})
		if err != nil {
			return dockerCleanupError(c, err)
		}
		return c.JSON(updated)
	})

	admin.Delete("/policies/:id", mutationLimiter, requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.DeletePolicy(ctx, c.Params("id")); err != nil {
			return dockerCleanupError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	admin.Post("/policies/:id/run-now", mutationLimiter, requireAdminScope("servers.write"), func(c *fiber.Ctx) error {
		id := c.Params("id")
		ctx, cancel := requestContext()
		defer cancel()
		policy, err := svc.RunPolicyNow(ctx, id)
		if policy != nil {
			recordAudit(cfg, c, "docker:cleanup-policy-run", "policy", &id, nil)
			return c.JSON(policy)
		}
		if err != nil {
			return dockerCleanupError(c, err)
		}
		return respondInternalError(c, errors.New("docker cleanup: policy run produced no result"))
	})
}

// dockerCleanupError maps dockerleanup sentinel errors onto honest HTTP
// statuses: validation -> 400, not found -> 404, Beacon dispatch -> 502,
// anything else -> 500.
func dockerCleanupError(c *fiber.Ctx, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, dockerleanupsvc.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, dockerleanupsvc.ErrValidation):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	case errors.Is(err, dockerleanupsvc.ErrDispatch):
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	default:
		return respondInternalError(c, err)
	}
}

// dockerCleanupInstanceID identifies this API replica for the scheduler lease.
func dockerCleanupInstanceID() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "api"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}
