package http

import (
	"gamepanel/forge/internal/services/deployment"
	"github.com/gofiber/fiber/v2"
)

func registerDeploymentRoutes(protected fiber.Router, cfg Config, svc *deployment.Service, adminIPAccess, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	dep := protected.Group("/admin/deployments", adminIPAccess)

	dep.Post("/blue-green", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		var req struct {
			ServerID        string `json:"serverId"`
			Image           string `json:"image"`
			HealthCheckPath string `json:"healthCheckPath"`
			HealthCheckPort int    `json:"healthCheckPort"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if err := deployment.ValidateImageRef(req.Image); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		d, err := svc.StartBlueGreen(c.Context(), req.ServerID, req.Image, req.HealthCheckPath, req.HealthCheckPort)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"data": d})
	})

	// canary: start a canary deployment (provision new alongside old, health-gate,
	// promote or rollback). Useful for risky image changes where you want to verify
	// before cutting over.
	dep.Post("/canary", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		var req struct {
			ServerID        string `json:"serverId"`
			Image           string `json:"image"`
			HealthCheckPath string `json:"healthCheckPath"`
			HealthCheckPort int    `json:"healthCheckPort"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if err := deployment.ValidateImageRef(req.Image); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		d, err := svc.StartCanary(c.Context(), req.ServerID, req.Image, req.HealthCheckPath, req.HealthCheckPort)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"data": d})
	})

	// rolling: scale up new instances, optionally health-gate, then scale down old.
	// No named targets; works with the replica manager.
	dep.Post("/rolling", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		var req struct {
			ServerID        string `json:"serverId"`
			Image           string `json:"image"`
			HealthCheckPath string `json:"healthCheckPath"`
			HealthCheckPort int    `json:"healthCheckPort"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if err := deployment.ValidateImageRef(req.Image); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		d, err := svc.StartRolling(c.Context(), req.ServerID, req.Image, req.HealthCheckPath, req.HealthCheckPort)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"data": d})
	})

	// recreate: stop-then-start. For workloads that cannot run two copies at once
	// (game servers holding exclusive locks, databases).
	dep.Post("/recreate", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		var req struct {
			ServerID        string `json:"serverId"`
			Image           string `json:"image"`
			HealthCheckPath string `json:"healthCheckPath"`
			HealthCheckPort int    `json:"healthCheckPort"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if err := deployment.ValidateImageRef(req.Image); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		d, err := svc.StartRecreate(c.Context(), req.ServerID, req.Image, req.HealthCheckPath, req.HealthCheckPort)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"data": d})
	})

	dep.Post("/:id/rollback", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		d, err := svc.Rollback(c.Context(), c.Params("id"))
		if err != nil {
			return respondInternalError(c, err)
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
		d, err := svc.StartRollout(c.Context(), &req)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.Status(201).JSON(fiber.Map{"data": d})
	})

	dep.Post("/:id/complete", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		d, err := svc.CompleteDeployment(c.Context(), c.Params("id"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": d})
	})

	dep.Post("/:id/cancel", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		d, err := svc.CancelDeployment(c.Context(), c.Params("id"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": d})
	})

	dep.Post("/:id/execute", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		if err := svc.ExecuteDeployment(c.Context(), c.Params("id")); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": fiber.Map{"deploymentId": c.Params("id")}})
	})

	dep.Post("/:id/cleanup", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		if err := svc.CleanupDeployment(c.Context(), c.Params("id")); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": fiber.Map{"deploymentId": c.Params("id")}})
	})

	dep.Get("/:id/steps", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		steps, err := svc.ListSteps(c.Context(), c.Params("id"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": steps})
	})

	dep.Get("/:id/steps/:stepId", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		step, err := svc.GetStep(c.Context(), c.Params("stepId"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": step})
	})

	dep.Post("/resume", mutationLimiter, requireRole("admin"), requireAdminScope("deployments.write"), func(c *fiber.Ctx) error {
		if err := svc.ResumeDeployments(c.Context()); err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": "ok"})
	})

	dep.Get("/:id", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		d, err := svc.GetDeployment(c.Context(), c.Params("id"))
		if err != nil {
			return c.Status(404).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"data": d})
	})

	dep.Get("/server/:serverId", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		deployments, err := svc.ListDeployments(c.Context(), c.Params("serverId"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": deployments})
	})

	dep.Get("/", requireRole("admin"), requireAdminScope("deployments.read"), func(c *fiber.Ctx) error {
		all, err := svc.ListDeployments(c.Context(), "")
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": all})
	})
}
