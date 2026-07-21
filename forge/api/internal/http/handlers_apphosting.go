package http

import (
	"encoding/json"

	"gamepanel/forge/internal/services/apphosting"
	"gamepanel/forge/internal/services/tenancy"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerAppHostingRoutes(protected fiber.Router, cfg Config, appSvc *apphosting.Service, mutationLimiter fiber.Handler) {
	if appSvc == nil || cfg.Store == nil {
		return
	}

	tenantAccess := func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		if claims.Role == "admin" {
			return c.Next()
		}
		orgID := c.Params("orgId")
		if orgID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "orgId is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		isMember, err := cfg.Store.UserIsOrgMember(ctx, orgID, claims.Sub)
		if err != nil || !isMember {
			return fiber.NewError(fiber.StatusForbidden, "not a member of this organization")
		}
		return c.Next()
	}

	resolveOrg := func(c *fiber.Ctx) (tenancy.OrgContext, error) {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return tenancy.OrgContext{}, fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		oc := tenancy.OrgContext{OrgID: c.Params("orgId"), Role: claims.Role}
		if claims.Role != "admin" && oc.OrgID != "" {
			ctx, cancel := requestContext()
			defer cancel()
			role := cfg.Store.ResolveEffectiveOrgRole(ctx, oc.OrgID, claims.Sub, claims.Role)
			if role != "" {
				oc.Role = role
			}
		}
		return oc, nil
	}

	// ---- Applications ----

	protected.Get("/organizations/:orgId/apps", tenantAccess, func(c *fiber.Ctx) error {
		oc, err := resolveOrg(c)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		apps, err := appSvc.ListApps(ctx, oc.OrgID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"data": apps})
	})

	protected.Post("/organizations/:orgId/apps", tenantAccess, mutationLimiter, func(c *fiber.Ctx) error {
		oc, err := resolveOrg(c)
		if err != nil {
			return err
		}
		var req apphosting.CreateAppRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		req.OrgID = oc.OrgID
		ctx, cancel := requestContext()
		defer cancel()
		app, err := appSvc.CreateApp(ctx, oc, req)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(app)
	})

	// ---- Global /apps routes (no org prefix) ----
	// These resolve the user's first org as fallback when no orgId is provided.

	protected.Get("/apps", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if claims.Role == "admin" {
			apps, err := cfg.Store.ListApplications(ctx, "")
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, err.Error())
			}
			return c.JSON(fiber.Map{"data": apps})
		}
		orgs, err := cfg.Store.ListOrganizationsForUser(ctx, claims.Sub)
		if err != nil || len(orgs) == 0 {
			return c.JSON(fiber.Map{"data": []store.Application{}})
		}
		var allApps []store.Application
		seen := map[string]bool{}
		for _, org := range orgs {
			apps, err := cfg.Store.ListApplications(ctx, org.ID)
			if err != nil {
				continue
			}
			for _, app := range apps {
				if !seen[app.ID] {
					seen[app.ID] = true
					allApps = append(allApps, app)
				}
			}
		}
		return c.JSON(fiber.Map{"data": allApps})
	})

	protected.Post("/apps", mutationLimiter, func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		var req apphosting.CreateAppRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.OrgID == "" {
			ctx, cancel := requestContext()
			defer cancel()
			if claims.Role == "admin" {
				req.OrgID = "default"
			} else {
				orgs, err := cfg.Store.ListOrganizationsForUser(ctx, claims.Sub)
				if err != nil || len(orgs) == 0 {
					return fiber.NewError(fiber.StatusBadRequest, "no organizations available for this user")
				}
				req.OrgID = orgs[0].ID
			}
		} else {
			ctx, cancel := requestContext()
			defer cancel()
			isMember, err := cfg.Store.UserIsOrgMember(ctx, req.OrgID, claims.Sub)
			if err != nil || (!isMember && claims.Role != "admin") {
				return fiber.NewError(fiber.StatusForbidden, "not a member of this organization")
			}
		}
		oc := tenancy.OrgContext{OrgID: req.OrgID, Role: claims.Role}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := appSvc.CreateApp(ctx, oc, req)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(app)
	})

	protected.Get("/apps/:id", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		return c.JSON(app)
	})

	protected.Put("/apps/:id", mutationLimiter, func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		var req apphosting.UpdateAppRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		updated, err := appSvc.UpdateApp(ctx, c.Params("id"), app.OrgID, req)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(updated)
	})

	protected.Delete("/apps/:id", mutationLimiter, func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		if err := appSvc.DeleteApp(ctx, c.Params("id"), app.OrgID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/apps/:id/deploy", mutationLimiter, func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		depl, err := appSvc.TriggerDeploy(ctx, c.Params("id"), app.OrgID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(depl)
	})

	// ---- Services ----

	protected.Get("/apps/:id/services", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		services, err := appSvc.ListServices(ctx, c.Params("id"), app.OrgID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"data": services})
	})

	protected.Post("/apps/:id/services", mutationLimiter, func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		var req apphosting.CreateServiceRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		svc, err := appSvc.CreateService(ctx, c.Params("id"), app.OrgID, req)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(svc)
	})

	protected.Delete("/apps/:appId/services/:serviceId", mutationLimiter, func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("appId"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		if err := appSvc.DeleteService(ctx, c.Params("serviceId"), c.Params("appId"), app.OrgID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// ---- App Lifecycle (start / stop / restart) ----

	protected.Post("/apps/:id/start", mutationLimiter, func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		running := "running"
		_, err = appSvc.UpdateApp(ctx, c.Params("id"), app.OrgID, apphosting.UpdateAppRequest{DesiredState: &running})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/apps/:id/stop", mutationLimiter, func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		stopped := "stopped"
		_, err = appSvc.UpdateApp(ctx, c.Params("id"), app.OrgID, apphosting.UpdateAppRequest{DesiredState: &stopped})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/apps/:id/restart", mutationLimiter, func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		depl, err := appSvc.TriggerDeploy(ctx, c.Params("id"), app.OrgID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(depl)
	})

	// ---- Deployments ----

	protected.Get("/apps/:id/deployments", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		return c.JSON(fiber.Map{"data": []store.Deployment{}})
	})

	// ---- Logs ----

	protected.Get("/apps/:id/logs", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		return c.JSON(fiber.Map{"data": []any{}})
	})

	// ---- Domains ----

	protected.Get("/apps/:id/domains", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		return c.JSON(fiber.Map{"data": []any{}})
	})

	protected.Post("/apps/:id/domains", mutationLimiter, func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		var req struct {
			Domain string `json:"domain"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.Domain == "" {
			return fiber.NewError(fiber.StatusBadRequest, "domain is required")
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true, "domain": req.Domain})
	})

	protected.Delete("/apps/:id/domains/:domainId", mutationLimiter, func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// ---- Backups ----

	protected.Get("/apps/:id/backups", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		return c.JSON(fiber.Map{"data": []any{}})
	})

	protected.Post("/apps/:id/backups", mutationLimiter, func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true})
	})

	protected.Post("/apps/:id/backups/:backupId/restore", mutationLimiter, func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Delete("/apps/:id/backups/:backupId", mutationLimiter, func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// ---- Compose ----

	protected.Get("/apps/:id/compose", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		return c.JSON(fiber.Map{"sourceType": app.SourceType, "sourceConfig": app.SourceConfig})
	})

	protected.Put("/apps/:id/compose", mutationLimiter, func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		var req struct {
			SourceConfig json.RawMessage `json:"sourceConfig"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := cfg.Store.UpdateApplication(ctx, c.Params("id"), store.UpdateApplicationInput{
			SourceConfig: req.SourceConfig,
		}); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		updated, _ := cfg.Store.GetApplication(ctx, c.Params("id"))
		return c.JSON(fiber.Map{"sourceType": "COMPOSE", "sourceConfig": updated.SourceConfig})
	})

	protected.Post("/apps/:id/compose/redeploy", mutationLimiter, func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		depl, err := appSvc.TriggerDeploy(ctx, c.Params("id"), app.OrgID)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(depl)
	})

	// ---- Git ----

	protected.Get("/apps/:id/git", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		return c.JSON(fiber.Map{"sourceType": app.SourceType, "sourceConfig": app.SourceConfig})
	})

	protected.Patch("/apps/:id/git", mutationLimiter, func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		var req struct {
			SourceConfig json.RawMessage `json:"sourceConfig"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := cfg.Store.UpdateApplication(ctx, c.Params("id"), store.UpdateApplicationInput{
			SourceConfig: req.SourceConfig,
		}); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		updated, _ := cfg.Store.GetApplication(ctx, c.Params("id"))
		return c.JSON(fiber.Map{"sourceType": "GIT", "sourceConfig": updated.SourceConfig})
	})

	protected.Patch("/apps/:id/git/auto-deploy", mutationLimiter, func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := cfg.Store.GetApplication(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "application not found")
		}
		if claims.Role != "admin" {
			isMember, _ := cfg.Store.UserIsOrgMember(ctx, app.OrgID, claims.Sub)
			if !isMember {
				return fiber.NewError(fiber.StatusNotFound, "application not found")
			}
		}
		var req struct {
			AutoDeploy *bool `json:"autoDeploy"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.AutoDeploy == nil {
			return fiber.NewError(fiber.StatusBadRequest, "autoDeploy is required")
		}
		var sc map[string]any
		if err := json.Unmarshal(app.SourceConfig, &sc); err != nil {
			sc = map[string]any{}
		}
		sc["autoDeploy"] = *req.AutoDeploy
		updatedConfig, _ := json.Marshal(sc)
		if err := cfg.Store.UpdateApplication(ctx, c.Params("id"), store.UpdateApplicationInput{
			SourceConfig: updatedConfig,
		}); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true, "autoDeploy": *req.AutoDeploy})
	})
}
