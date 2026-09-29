package http

import (
	"strings"

	"gamepanel/forge/internal/services/deployment"
	"github.com/gofiber/fiber/v2"
)

// deploymentStartRequest is the body every per-strategy creation endpoint
// accepts, validated before it reaches the service.
type deploymentStartRequest struct {
	ServerID        string
	Image           string
	HealthCheckPath string
	HealthCheckPort int
}

// validateDeploymentHealthCheckTarget applies the executor's own bounds at the
// edge: the legacy per-strategy path never runs the rollout validation, so an
// out-of-range port would otherwise reach the health checker and a bad probe
// target would read as a 500 rather than a 400.
func validateDeploymentHealthCheckTarget(path string, port int) error {
	if port < 0 || port > 65535 {
		return fiber.NewError(fiber.StatusBadRequest, "healthCheckPort must be between 0 and 65535")
	}
	if path != "" && !strings.HasPrefix(path, "/") {
		return fiber.NewError(fiber.StatusBadRequest, "healthCheckPath must be a local absolute path")
	}
	return nil
}

func parseDeploymentStartRequest(c *fiber.Ctx) (deploymentStartRequest, error) {
	var req struct {
		ServerID        string `json:"serverId"`
		Image           string `json:"image"`
		HealthCheckPath string `json:"healthCheckPath"`
		HealthCheckPort int    `json:"healthCheckPort"`
	}
	if err := c.BodyParser(&req); err != nil {
		return deploymentStartRequest{}, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	// An unnamed workload is refused here rather than reaching the store as "",
	// where it would read back as whichever row the query happens to match.
	serverID := strings.TrimSpace(req.ServerID)
	if serverID == "" {
		return deploymentStartRequest{}, fiber.NewError(fiber.StatusBadRequest, "serverId is required")
	}
	if err := deployment.ValidateImageRef(req.Image); err != nil {
		return deploymentStartRequest{}, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	path := strings.TrimSpace(req.HealthCheckPath)
	if err := validateDeploymentHealthCheckTarget(path, req.HealthCheckPort); err != nil {
		return deploymentStartRequest{}, err
	}
	return deploymentStartRequest{
		ServerID:        serverID,
		Image:           req.Image,
		HealthCheckPath: path,
		HealthCheckPort: req.HealthCheckPort,
	}, nil
}

// deploymentTargetID returns a path identifier or a 400: a blank id must not be
// handed to a service that keys off it directly.
func deploymentTargetID(c *fiber.Ctx, name string) (string, error) {
	id := strings.TrimSpace(c.Params(name))
	if id == "" {
		return "", fiber.NewError(fiber.StatusBadRequest, name+" is required")
	}
	return id, nil
}

func registerDeploymentRoutes(protected fiber.Router, cfg Config, svc *deployment.Service, adminIPAccess, mutationLimiter fiber.Handler) {
	if svc == nil {
		if cfg.Logger != nil {
			cfg.Logger.Warn("deployment routes not registered: deployment service is not configured")
		}
		return
	}

	dep := protected.Group("/admin/deployments", adminIPAccess)

	dep.Post("/blue-green", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		req, err := parseDeploymentStartRequest(c)
		if err != nil {
			return err
		}
		ctx, cancel := longRequestContext()
		defer cancel()
		d, err := svc.StartBlueGreen(ctx, req.ServerID, req.Image, req.HealthCheckPath, req.HealthCheckPort)
		if err != nil {
			return deploymentRollbackError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"data": d})
	})

	// canary: start a canary deployment (provision new alongside old, health-gate,
	// promote or rollback). Useful for risky image changes where you want to verify
	// before cutting over.
	dep.Post("/canary", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		req, err := parseDeploymentStartRequest(c)
		if err != nil {
			return err
		}
		ctx, cancel := longRequestContext()
		defer cancel()
		d, err := svc.StartCanary(ctx, req.ServerID, req.Image, req.HealthCheckPath, req.HealthCheckPort)
		if err != nil {
			return deploymentRollbackError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"data": d})
	})

	// rolling: scale up new instances, optionally health-gate, then scale down old.
	// No named targets; works with the replica manager.
	dep.Post("/rolling", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		req, err := parseDeploymentStartRequest(c)
		if err != nil {
			return err
		}
		ctx, cancel := longRequestContext()
		defer cancel()
		d, err := svc.StartRolling(ctx, req.ServerID, req.Image, req.HealthCheckPath, req.HealthCheckPort)
		if err != nil {
			return deploymentRollbackError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"data": d})
	})

	// recreate: stop-then-start. For workloads that cannot run two copies at once
	// (game servers holding exclusive locks, databases).
	dep.Post("/recreate", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		req, err := parseDeploymentStartRequest(c)
		if err != nil {
			return err
		}
		ctx, cancel := longRequestContext()
		defer cancel()
		d, err := svc.StartRecreate(ctx, req.ServerID, req.Image, req.HealthCheckPath, req.HealthCheckPort)
		if err != nil {
			return deploymentRollbackError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"data": d})
	})

	dep.Post("/:id/rollback", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		deploymentID, err := deploymentTargetID(c, "id")
		if err != nil {
			return err
		}
		ctx, cancel := longRequestContext()
		defer cancel()
		d, err := svc.Rollback(ctx, deploymentID)
		if err != nil {
			return deploymentRollbackError(c, err)
		}
		return c.JSON(fiber.Map{"data": d})
	})

	// rollout: unified endpoint the web UI uses to start a deployment with any
	// strategy in one call. Replaces four separate POST endpoints for new code;
	// the per-strategy endpoints above remain for backward-compatibility.
	dep.Post("/:serverId/rollout", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		var req deployment.RolloutRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		// The server ID comes from the path; override anything in the body so a
		// mismatched URL/body pair cannot target the wrong workload.
		req.ServerID = c.Params("serverId")
		ctx, cancel := longRequestContext()
		defer cancel()
		d, err := svc.StartRollout(ctx, &req)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"data": d})
	})

	dep.Post("/:id/complete", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		d, err := svc.CompleteDeployment(ctx, c.Params("id"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": d})
	})

	dep.Post("/:id/cancel", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		d, err := svc.CancelDeployment(ctx, c.Params("id"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": d})
	})

	dep.Post("/:id/execute", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		ctx, cancel := longRequestContext()
		defer cancel()
		if err := svc.ExecuteDeployment(ctx, c.Params("id")); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": fiber.Map{"deploymentId": c.Params("id")}})
	})

	dep.Post("/:id/cleanup", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		ctx, cancel := longRequestContext()
		defer cancel()
		if err := svc.CleanupDeployment(ctx, c.Params("id")); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": fiber.Map{"deploymentId": c.Params("id")}})
	})

	dep.Get("/:id/steps", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		steps, err := svc.ListSteps(ctx, c.Params("id"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": steps})
	})

	dep.Get("/:id/steps/:stepId", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		step, err := svc.GetStep(ctx, c.Params("stepId"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": step})
	})

	dep.Post("/resume", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		ctx, cancel := longRequestContext()
		defer cancel()
		if err := svc.ResumeDeployments(ctx); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": "ok"})
	})

	dep.Get("/:id", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		d, err := svc.GetDeployment(ctx, c.Params("id"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": d})
	})

	dep.Get("/server/:serverId", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		deployments, err := svc.ListDeployments(ctx, c.Params("serverId"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": deployments})
	})

	dep.Get("/", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		all, err := svc.ListDeployments(ctx, "")
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": all})
	})
}
