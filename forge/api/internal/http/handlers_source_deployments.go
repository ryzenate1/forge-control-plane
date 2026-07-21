package http

import (
	"strings"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

type CreateSourceDeploymentBody struct {
	ServerID                string `json:"serverId"`
	GitProviderID           string `json:"gitProviderId"`
	Repository              string `json:"repository" validate:"required"`
	Branch                  string `json:"branch"`
	BuildType               string `json:"buildType"`
	BuildContext            string `json:"buildContext"`
	DockerfilePath          string `json:"dockerfilePath"`
	AutoDeploy              bool   `json:"autoDeploy"`
	Registry                string `json:"registry"`
	RegistryCredentialID    string `json:"registryCredentialId"`
	HealthCheckPath         string `json:"healthCheckPath"`
	HealthCheckPort         int    `json:"healthCheckPort"`
	RollbackOnHealthFailure bool   `json:"rollbackOnHealthFailure"`
}

type UpdateSourceDeploymentBody struct {
	ServerID                *string `json:"serverId"`
	GitProviderID           *string `json:"gitProviderId"`
	Repository              *string `json:"repository"`
	Branch                  *string `json:"branch"`
	BuildType               *string `json:"buildType"`
	BuildContext            *string `json:"buildContext"`
	DockerfilePath          *string `json:"dockerfilePath"`
	AutoDeploy              *bool   `json:"autoDeploy"`
	Registry                *string `json:"registry"`
	RegistryCredentialID    *string `json:"registryCredentialId"`
	HealthCheckPath         *string `json:"healthCheckPath"`
	HealthCheckPort         *int    `json:"healthCheckPort"`
	RollbackOnHealthFailure *bool   `json:"rollbackOnHealthFailure"`
}

func ListSourceDeployments(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		deps, err := cfg.Store.ListSourceDeployments(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(deps)
	}
}

func GetSourceDeployment(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		d, err := cfg.Store.GetSourceDeployment(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.JSON(d)
	}
}

func CreateSourceDeployment(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req CreateSourceDeploymentBody
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.Repository) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "repository is required")
		}
		if req.BuildType == "" {
			req.BuildType = "dockerfile"
		}
		validBuildTypes := map[string]bool{"dockerfile": true, "nixpacks": true, "heroku": true, "paketo": true, "static": true}
		if !validBuildTypes[req.BuildType] {
			return fiber.NewError(fiber.StatusBadRequest, "buildType must be one of: dockerfile, nixpacks, heroku, paketo, static")
		}

		ctx, cancel := requestContext()
		defer cancel()
		var createdBy *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			createdBy = &claims.Sub
		}

		var serverID, gitProviderID *string
		if req.ServerID != "" {
			serverID = &req.ServerID
		}
		if req.GitProviderID != "" {
			gitProviderID = &req.GitProviderID
		}

		d, err := cfg.Store.CreateSourceDeployment(ctx, store.CreateSourceDeploymentRequest{
			ServerID:                serverID,
			GitProviderID:           gitProviderID,
			Repository:              req.Repository,
			Branch:                  req.Branch,
			BuildType:               req.BuildType,
			BuildContext:            req.BuildContext,
			DockerfilePath:          req.DockerfilePath,
			AutoDeploy:              req.AutoDeploy,
			Registry:                req.Registry,
			RegistryCredentialID:    req.RegistryCredentialID,
			HealthCheckPath:         req.HealthCheckPath,
			HealthCheckPort:         req.HealthCheckPort,
			RollbackOnHealthFailure: req.RollbackOnHealthFailure,
			CreatedBy:               createdBy,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(d)
	}
}

func UpdateSourceDeployment(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var body UpdateSourceDeploymentBody
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()

		req := store.UpdateSourceDeploymentRequest{
			ServerID:                body.ServerID,
			GitProviderID:           body.GitProviderID,
			Repository:              body.Repository,
			Branch:                  body.Branch,
			BuildType:               body.BuildType,
			BuildContext:            body.BuildContext,
			DockerfilePath:          body.DockerfilePath,
			AutoDeploy:              body.AutoDeploy,
			Registry:                body.Registry,
			RegistryCredentialID:    body.RegistryCredentialID,
			HealthCheckPath:         body.HealthCheckPath,
			HealthCheckPort:         body.HealthCheckPort,
			RollbackOnHealthFailure: body.RollbackOnHealthFailure,
		}
		d, err := cfg.Store.UpdateSourceDeployment(ctx, c.Params("id"), req)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(d)
	}
}

func DeleteSourceDeployment(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DeleteSourceDeployment(ctx, c.Params("id")); err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	}
}

func DeploySourceDeployment(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		d, err := cfg.Store.GetSourceDeployment(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "deployment not found")
		}
		if err := cfg.Store.UpdateSourceDeploymentStatus(ctx, d.ID, "queued"); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		cfg.Store.CreateDeploymentBuildLog(ctx, d.ID, "queued", "Deployment queued")
		d.Status = "queued"
		return c.JSON(fiber.Map{"ok": true, "deployment": d})
	}
}

func CancelSourceDeployment(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		d, err := cfg.Store.GetSourceDeployment(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "deployment not found")
		}
		terminal := map[string]bool{"completed": true, "failed": true, "canceled": true}
		if terminal[d.Status] {
			return fiber.NewError(fiber.StatusConflict, "deployment already in terminal state")
		}
		if err := cfg.Store.UpdateSourceDeploymentStatus(ctx, d.ID, "canceled"); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		cfg.Store.CreateDeploymentBuildLog(ctx, d.ID, "canceled", "Deployment canceled by user")
		return c.JSON(fiber.Map{"ok": true})
	}
}

func GetDeploymentBuildLogs(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		logs, err := cfg.Store.ListDeploymentBuildLogs(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.JSON(logs)
	}
}

func registerSourceDeploymentRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler) {
	sdGroup := protected.Group("/source-deployments")
	sdGroup.Get("/", ListSourceDeployments(cfg))
	sdGroup.Post("/", mutationLimiter, CreateSourceDeployment(cfg))
	sdGroup.Get("/:id", GetSourceDeployment(cfg))
	sdGroup.Patch("/:id", mutationLimiter, UpdateSourceDeployment(cfg))
	sdGroup.Delete("/:id", mutationLimiter, DeleteSourceDeployment(cfg))
	sdGroup.Post("/:id/deploy", mutationLimiter, DeploySourceDeployment(cfg))
	sdGroup.Post("/:id/cancel", mutationLimiter, CancelSourceDeployment(cfg))
	sdGroup.Get("/:id/logs", GetDeploymentBuildLogs(cfg))
}
