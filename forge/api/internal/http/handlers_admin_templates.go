package http

import (
	"encoding/json"
	"fmt"

	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/services/evacuationplanner"
	migrationservice "gamepanel/forge/internal/services/migration"
	"gamepanel/forge/internal/services/noderegistry"
	recoverysvc "gamepanel/forge/internal/services/recovery"
	reservationsvc "gamepanel/forge/internal/services/reservations"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerTemplatesAdminRoutes(protected fiber.Router, cfg Config, nodeRegistry *noderegistry.Service, clusterManager *clustermanager.Service, evacuationPlanner *evacuationplanner.Service, migrationService *migrationservice.Service, reservationManager *reservationsvc.Manager, recoveryCoordinator *recoverysvc.Coordinator, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {

	protected.Get("/nests", requireAdminScope("nests.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		nests, err := cfg.Store.ListNests(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(nests)
	})

	protected.Get("/nests/:id", requireAdminScope("nests.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		nest, err := cfg.Store.GetNest(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "nest not found")
		}
		return c.JSON(nest)
	})

	protected.Post("/nests", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("nests.write"), func(c *fiber.Ctx) error {
		var req CreateNestRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := Validate(&req); err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(err)
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		nest, err := cfg.Store.CreateNest(ctx, store.CreateNestRequest{
			Name:        req.Name,
			Description: req.Description,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(nest)
	})

	protected.Patch("/nests/:id", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("nests.write"), func(c *fiber.Ctx) error {
		var req UpdateNestRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		nest, err := cfg.Store.UpdateNest(ctx, c.Params("id"), store.UpdateNestRequest{
			Name:        req.Name,
			Description: req.Description,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(nest)
	})

	protected.Delete("/nests/:id", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("nests.delete"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.DeleteNest(ctx, c.Params("id"), actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// ---- Eggs CRUD ----

	protected.Get("/nests/:nestId/eggs", requireAdminScope("nests.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		eggs, err := cfg.Store.ListEggs(ctx, c.Params("nestId"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(eggs)
	})

	protected.Get("/eggs/:id", requireAdminScope("nests.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		egg, err := cfg.Store.GetEgg(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "egg not found")
		}
		return c.JSON(egg)
	})

	protected.Post("/eggs", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("nests.write"), func(c *fiber.Ctx) error {
		var req CreateEggRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := Validate(&req); err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(err)
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		egg, err := cfg.Store.CreateEgg(ctx, store.CreateEggRequest{
			NestID: req.NestID, Name: req.Name, Description: req.Description,
			DockerImages: req.DockerImages, Startup: req.Startup, Config: req.Config,
			DefaultMemoryMB: req.DefaultMemoryMB, InstallScript: req.InstallScript,
			InstallContainer: req.InstallContainer, InstallEntrypoint: req.InstallEntrypoint,
			FileDenylist: req.FileDenylist, ConfigFrom: req.ConfigFrom,
			CopyScriptFrom: req.CopyScriptFrom, UpdateURL: req.UpdateURL, Author: req.Author,
			Features: req.Features, StartupCommands: req.StartupCommands,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(egg)
	})

	protected.Patch("/eggs/:id", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("nests.write"), func(c *fiber.Ctx) error {
		var req UpdateEggRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		egg, err := cfg.Store.UpdateEgg(ctx, c.Params("id"), store.UpdateEggRequest{
			Name: req.Name, Description: req.Description, DockerImages: req.DockerImages,
			Startup: req.Startup, Config: req.Config, DefaultMemoryMB: req.DefaultMemoryMB,
			InstallScript: req.InstallScript, InstallContainer: req.InstallContainer,
			InstallEntrypoint: req.InstallEntrypoint, FileDenylist: req.FileDenylist,
			ConfigFrom: req.ConfigFrom, CopyScriptFrom: req.CopyScriptFrom,
			UpdateURL: req.UpdateURL, Author: req.Author,
			Features: req.Features, StartupCommands: req.StartupCommands,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(egg)
	})

	protected.Get("/eggs/:id/variables", requireAdminScope("nests.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		variables, err := cfg.Store.ListEggVariables(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(variables)
	})

	protected.Post("/eggs/:id/variables", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("nests.write"), func(c *fiber.Ctx) error {
		var req EggVariableRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := Validate(&req); err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(err)
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		variable, err := cfg.Store.CreateEggVariable(ctx, c.Params("id"), store.EggVariableRequest{
			Name: req.Name, Description: req.Description, EnvVariable: req.EnvVariable,
			DefaultValue: req.DefaultValue, UserViewable: req.UserViewable,
			UserEditable: req.UserEditable, Rules: req.Rules, Sort: req.Sort,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(variable)
	})

	protected.Patch("/eggs/:id/variables/:variableId", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("nests.write"), func(c *fiber.Ctx) error {
		var req EggVariableRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := Validate(&req); err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(err)
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		variable, err := cfg.Store.UpdateEggVariable(ctx, c.Params("id"), c.Params("variableId"), store.EggVariableRequest{
			Name: req.Name, Description: req.Description, EnvVariable: req.EnvVariable,
			DefaultValue: req.DefaultValue, UserViewable: req.UserViewable,
			UserEditable: req.UserEditable, Rules: req.Rules, Sort: req.Sort,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(variable)
	})

	protected.Delete("/eggs/:id/variables/:variableId", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("nests.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.DeleteEggVariable(ctx, c.Params("id"), c.Params("variableId"), actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Delete("/eggs/:id", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("nests.delete"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		if err := cfg.Store.DeleteEgg(ctx, c.Params("id"), actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// ---- Egg Export/Import ----

	protected.Get("/eggs/:id/export", requireAdminScope("nests.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		egg, err := cfg.Store.GetEgg(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "egg not found")
		}
		variables, err := cfg.Store.ListEggVariables(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{
			"egg":       egg,
			"variables": variables,
		})
	})

	protected.Post("/eggs/import", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("nests.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			NestID            string             `json:"nestId"`
			Name              string             `json:"name"`
			Description       string             `json:"description"`
			DockerImages      json.RawMessage    `json:"dockerImages"`
			Startup           string             `json:"startup"`
			Config            json.RawMessage    `json:"config"`
			DefaultMemoryMB   int                `json:"defaultMemoryMb"`
			InstallScript     string             `json:"installScript"`
			InstallContainer  string             `json:"installContainer"`
			InstallEntrypoint string             `json:"installEntrypoint"`
			FileDenylist      json.RawMessage    `json:"fileDenylist"`
			ConfigFrom        *string            `json:"configFrom,omitempty"`
			CopyScriptFrom    *string            `json:"copyScriptFrom,omitempty"`
			UpdateURL         string             `json:"updateUrl"`
			Author            string             `json:"author"`
			Features          json.RawMessage    `json:"features"`
			StartupCommands   json.RawMessage    `json:"startupCommands"`
			Variables         []importedVariable `json:"variables"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid import payload")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		egg, err := cfg.Store.CreateEgg(ctx, store.CreateEggRequest{
			NestID: req.NestID, Name: req.Name, Description: req.Description,
			DockerImages: req.DockerImages, Startup: req.Startup, Config: req.Config,
			DefaultMemoryMB: req.DefaultMemoryMB, InstallScript: req.InstallScript,
			InstallContainer: req.InstallContainer, InstallEntrypoint: req.InstallEntrypoint,
			FileDenylist: req.FileDenylist, ConfigFrom: req.ConfigFrom,
			CopyScriptFrom: req.CopyScriptFrom, UpdateURL: req.UpdateURL, Author: req.Author,
			Features: req.Features, StartupCommands: req.StartupCommands,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		for _, v := range req.Variables {
			if _, err := cfg.Store.CreateEggVariable(ctx, egg.ID, store.EggVariableRequest{
				Name: v.Name, Description: v.Description, EnvVariable: v.EnvVariable,
				DefaultValue: v.DefaultValue, UserViewable: v.UserViewable,
				UserEditable: v.UserEditable, Rules: v.Rules, Sort: v.Sort,
			}, actorID); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("failed to import variable %s: %v", v.EnvVariable, err))
			}
		}
		return c.Status(fiber.StatusCreated).JSON(egg)
	})

	// Test a prospective host before it is saved. This uses the same normalization,
	// validation, TLS configuration, and ping flow as provisioning without persistence.

	// This is an administrator-only diagnostic for an external provisioning host.
	// Preserve the connector's sanitized cause (timeout, TLS, authentication, etc.)
	// instead of collapsing every failure into an indistinguishable message.

	// TLS CA values are write-only. Treat a blank value as omitted so clients can
	// safely round-trip an edit without overwriting a redacted certificate.

	protected.Get("/mounts/:id/eggs", requireRole("admin"), requireAdminScope("mounts.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		eggs, err := cfg.Store.ListEggsForMount(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(eggs)
	})

	protected.Post("/mounts/:id/eggs", adminIPAccess, mutationLimiter, requireRole("admin"), requireAdminScope("mounts.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var req struct {
			EggIDs []string `json:"eggs"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		for _, eggID := range req.EggIDs {
			if err := cfg.Store.AttachEggToMount(ctx, c.Params("id"), eggID); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, err.Error())
			}
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Delete("/mounts/:id/eggs/:eggId", adminIPAccess, requireRole("admin"), requireAdminScope("mounts.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := cfg.Store.DetachEggFromMount(ctx, c.Params("id"), c.Params("eggId")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// Server <-> Mount attachment. The mount must be eligible for the server's
	// node and egg, just as it is for the server-scoped assignment route.

	// Register the static path before /allocations/:id so it is not interpreted as id="bulk".

	protected.Get("/templates", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		templates, err := cfg.Store.ListTemplates(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(templates)
	})

	protected.Get("/templates/:id", func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		template, err := cfg.Store.GetTemplate(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "template not found")
		}
		return c.JSON(template)
	})

	protected.Post("/templates", requireRole("admin"), func(c *fiber.Ctx) error {
		var req CreateTemplateRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := Validate(&req); err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(err)
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		template, err := cfg.Store.CreateTemplate(ctx, store.CreateTemplateRequest{
			Name:            req.Name,
			Image:           req.Image,
			StartupCommand:  req.StartupCommand,
			DefaultMemoryMB: req.DefaultMemoryMB,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(template)
	})

	protected.Patch("/templates/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		var req CreateTemplateRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		template, err := cfg.Store.UpdateTemplate(ctx, c.Params("id"), store.CreateTemplateRequest{
			Name: req.Name, Image: req.Image, StartupCommand: req.StartupCommand,
			DefaultMemoryMB: req.DefaultMemoryMB,
		}, actorID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(template)
	})
}
