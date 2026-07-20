package http

import (
	"encoding/json"

	apphostingpostgres "gamepanel/forge/internal/modules/apphosting/adapters/postgres"
	apphostingapplication "gamepanel/forge/internal/modules/apphosting/application"
	apphostingdomain "gamepanel/forge/internal/modules/apphosting/domain"
	platformoperations "gamepanel/forge/internal/platform/operations"
	"gamepanel/forge/internal/platform/tenancy"
	"gamepanel/forge/internal/platform/workloads"
	"gamepanel/forge/internal/store"
	"github.com/ryzenate1/forge-control-plane/packages/agent-protocol"

	"github.com/gofiber/fiber/v2"
)

// registerPlatformRoutes exposes the stable control-plane primitives. These
// are provider-admin routes until organization RBAC is introduced; individual
// capability modules add their own project-scoped workflows on top.
func registerPlatformRoutes(protected fiber.Router, cfg Config, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {
	platform := protected.Group("/platform", requireRole("admin"))
	platform.Get("/scope/default", func(c *fiber.Ctx) error { return c.JSON(tenancy.DefaultScope()) })
	platform.Post("/organizations", adminIPAccess, mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var request struct {
			Name string `json:"name"`
			Slug string `json:"slug"`
		}
		if err := c.BodyParser(&request); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		organization, err := cfg.Store.CreateOrganization(ctx, store.CreateOrganizationRequest{Name: request.Name, Slug: request.Slug})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(organization)
	})
	platform.Post("/projects", adminIPAccess, mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var request struct {
			OrganizationID string `json:"organizationId"`
			Name           string `json:"name"`
			Slug           string `json:"slug"`
		}
		if err := c.BodyParser(&request); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		project, err := cfg.Store.CreateProject(ctx, store.CreateProjectRequest{OrganizationID: request.OrganizationID, Name: request.Name, Slug: request.Slug})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(project)
	})
	platform.Post("/environments", adminIPAccess, mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var request struct {
			ProjectID  string `json:"projectId"`
			Name       string `json:"name"`
			Slug       string `json:"slug"`
			Production bool   `json:"production"`
		}
		if err := c.BodyParser(&request); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		environment, err := cfg.Store.CreateEnvironment(ctx, store.CreateEnvironmentRequest{ProjectID: request.ProjectID, Name: request.Name, Slug: request.Slug, Production: request.Production})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(environment)
	})
	platform.Get("/workloads", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		values, err := cfg.Store.ListWorkloads(ctx, c.Query("environmentId"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(values)
	})
	platform.Post("/workloads", adminIPAccess, mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var request struct {
			EnvironmentID string                 `json:"environmentId"`
			Kind          workloads.Kind         `json:"kind"`
			Name          string                 `json:"name"`
			DesiredState  workloads.DesiredState `json:"desiredState"`
			Spec          json.RawMessage        `json:"spec"`
		}
		if err := c.BodyParser(&request); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		createdBy := ""
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			createdBy = claims.Sub
		}
		workload, revision, err := cfg.Store.CreateWorkload(ctx, store.CreateWorkloadRequest{EnvironmentID: request.EnvironmentID, Kind: request.Kind, Name: request.Name, DesiredState: request.DesiredState, Spec: request.Spec, CreatedBy: createdBy})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"workload": workload, "revision": revision})
	})
	platform.Post("/applications", adminIPAccess, mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var request struct {
			EnvironmentID   string                      `json:"environmentId"`
			Name            string                      `json:"name"`
			Source          apphostingdomain.SourceKind `json:"source"`
			Image           string                      `json:"image"`
			RepositoryURL   string                      `json:"repositoryUrl"`
			ComposeFile     string                      `json:"composeFile"`
			Deployment      apphostingdomain.Strategy   `json:"deployment"`
			HealthCheckPath string                      `json:"healthCheckPath"`
			HealthCheckPort int                         `json:"healthCheckPort"`
		}
		if err := c.BodyParser(&request); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		dispatcher, err := platformoperations.NewService(store.NewOperationRepository(cfg.Store))
		if err != nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
		}
		service, err := apphostingapplication.New(apphostingpostgres.NewWorkloads(cfg.Store), dispatcher)
		if err != nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
		}
		ctx, cancel := requestContext()
		defer cancel()
		workload, operation, err := service.Create(ctx, apphostingdomain.Application{EnvironmentID: request.EnvironmentID, Name: request.Name, Source: request.Source, Image: request.Image, RepositoryURL: request.RepositoryURL, ComposeFile: request.ComposeFile, Deployment: request.Deployment, HealthCheckPath: request.HealthCheckPath, HealthCheckPort: request.HealthCheckPort})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"workload": workload, "operation": operation})
	})
	platform.Get("/operations/:id", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		operation, err := cfg.Store.GetOperation(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "operation not found")
		}
		return c.JSON(operation)
	})
	platform.Get("/operations", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		operations, err := cfg.Store.ListOperations(ctx, c.Query("resourceId"), c.QueryInt("limit", 50))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(operations)
	})
	platform.Post("/workloads/:id/observations", adminIPAccess, mutationLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var observation protocol.Observation
		if err := c.BodyParser(&observation); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if observation.WorkloadID != c.Params("id") {
			return fiber.NewError(fiber.StatusBadRequest, "workload id does not match route")
		}
		if err := observation.Validate(); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.RecordWorkloadObservation(ctx, observation.WorkloadID, observation.Generation, workloads.ObservedState(observation.State), observation.Details); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
}

// registerPlatformAgentRoutes uses the existing remote node middleware. These
// endpoints are deliberately separate from provider-admin routes: a Beacon can
// only fetch or acknowledge commands assigned to its own node.
func registerPlatformAgentRoutes(remote fiber.Router, cfg Config) {
	remote.Get("/platform/commands", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}
		ctx, cancel := requestContext()
		defer cancel()
		commands, err := cfg.Store.PendingAgentCommands(ctx, node.ID, c.QueryInt("limit", 100))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(commands)
	})
	remote.Post("/platform/commands/:id/acknowledgements", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		node, ok := c.Locals("remoteNode").(store.Node)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing node")
		}
		var request protocol.Acknowledgement
		if err := c.BodyParser(&request); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := request.Validate(); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if request.CommandID != c.Params("id") {
			return fiber.NewError(fiber.StatusBadRequest, "command id does not match route")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.AcknowledgeAgentCommandForNode(ctx, request.CommandID, node.ID, request.Status, request.Error, request.Result); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
	})
}
