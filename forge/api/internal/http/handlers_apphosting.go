package http

import (
	"encoding/json"
	"fmt"
	"time"

	"gamepanel/forge/internal/services/apphosting"
	"gamepanel/forge/internal/services/backup"
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
	appBackupSvc := backup.NewMainService(cfg.Store, backup.NewSlogLogger(nil))
	appBackupSvc.SetDaemonClient(cfg.Daemon)

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
		var deployments []store.Deployment
		if app.ServerID != nil && *app.ServerID != "" {
			deployments, err = cfg.Store.ListDeployments(ctx, *app.ServerID)
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "failed to list deployments")
			}
		} else if app.CurrentDeploymentID != nil && *app.CurrentDeploymentID != "" {
			depl, derr := cfg.Store.GetDeployment(ctx, *app.CurrentDeploymentID)
			if derr == nil {
				deployments = append(deployments, depl)
			}
		}
		if deployments == nil {
			deployments = []store.Deployment{}
		}
		return c.JSON(fiber.Map{"data": deployments})
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
		logs := []store.BuildLog{}
		if app.CurrentDeploymentID != nil && *app.CurrentDeploymentID != "" {
			entries, lerr := cfg.Store.ListDeploymentBuildLogs(ctx, *app.CurrentDeploymentID)
			if lerr != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "failed to list logs")
			}
			logs = entries
		}
		return c.JSON(fiber.Map{"data": logs})
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
		appID := c.Params("id")
		svcType := "app"
		domains, derr := cfg.Store.ListProxyDomains(ctx, store.ProxyDomainFilter{ServiceID: &appID, ServiceType: &svcType})
		if derr != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to list domains")
		}
		if domains == nil {
			domains = []store.ProxyDomain{}
		}
		return c.JSON(fiber.Map{"data": domains})
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
		if existing, _ := cfg.Store.GetProxyDomainByHostname(ctx, req.Domain); existing != nil {
			return fiber.NewError(fiber.StatusConflict, "domain already in use")
		}
		created, cerr := cfg.Store.CreateProxyDomain(ctx, store.ProxyDomain{
			Hostname:    req.Domain,
			ServiceID:   c.Params("id"),
			ServiceType: "app",
			HTTPS:       true,
			AutoRenew:   true,
			Path:        "/",
		})
		if cerr != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to create domain")
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true, "domain": created})
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
		domain, derr := cfg.Store.GetProxyDomain(ctx, c.Params("domainId"))
		if derr != nil || domain == nil || domain.ServiceID != c.Params("id") || domain.ServiceType != "app" {
			return fiber.NewError(fiber.StatusNotFound, "domain not found")
		}
		if err := cfg.Store.DeleteProxyDomain(ctx, domain.ID); err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to delete domain")
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
		appID := c.Params("id")
		artifacts, total, err := appBackupSvc.ListBackupArtifacts(ctx, backup.ArtifactFilter{
			SourceAppID: &appID, Page: c.QueryInt("page", 1), PerPage: c.QueryInt("perPage", 50),
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to list application backups")
		}
		return c.JSON(fiber.Map{"data": artifacts, "total": total})
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
		var body struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		}
		if len(c.Body()) > 0 {
			if err := c.BodyParser(&body); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
			}
		}
		if body.Name == "" {
			body.Name = "app-" + app.ID + "-" + time.Now().UTC().Format("20060102-150405")
		}
		job, err := appBackupSvc.CreateBackupJob(ctx, backup.CreateBackupJobRequest{
			JobType: backup.BackupTypeApp, AppID: &app.ID, Name: body.Name,
			Description: body.Description, TriggeredBy: "manual",
		}, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
		}
		if err := appBackupSvc.ExecuteBackupJob(ctx, job.ID, claims.Sub); err != nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, fmt.Sprintf("application backup failed: %v", err))
		}
		completed, err := appBackupSvc.GetBackupJob(ctx, job.ID)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(completed)
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
		artifact, err := appBackupSvc.GetBackupArtifact(ctx, c.Params("backupId"))
		if err != nil || artifact.SourceAppID == nil || *artifact.SourceAppID != app.ID {
			return fiber.NewError(fiber.StatusNotFound, "application backup not found")
		}
		restore, err := appBackupSvc.CreateRestore(ctx, backup.CreateRestoreRequest{
			ArtifactID: artifact.ID, RestoreType: backup.BackupTypeApp,
			TargetAppID: &app.ID, TriggeredBy: "manual",
		}, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
		}
		if err := appBackupSvc.ExecuteRestore(ctx, restore.ID, claims.Sub); err != nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
		}
		return c.JSON(restore)
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
		artifact, err := appBackupSvc.GetBackupArtifact(ctx, c.Params("backupId"))
		if err != nil || artifact.SourceAppID == nil || *artifact.SourceAppID != app.ID {
			return fiber.NewError(fiber.StatusNotFound, "application backup not found")
		}
		if err := appBackupSvc.DeleteBackupArtifact(ctx, artifact.ID, claims.Sub); err != nil {
			return fiber.NewError(fiber.StatusConflict, err.Error())
		}
		return c.SendStatus(fiber.StatusNoContent)
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

	// ---- App templates ----
	// Serves the curated application template catalog. This backs the
	// frontend's GET /admin/app-templates call, which previously 404'd and
	// silently fell back to bundled data (audit P2 remediation).
	protected.Get("/admin/app-templates", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		if claims.Role != "admin" {
			return fiber.NewError(fiber.StatusForbidden, "admin access required")
		}
		return c.JSON(fiber.Map{"data": defaultAppTemplates()})
	})
}

type appTemplatePort struct {
	HostPort      int    `json:"hostPort"`
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
	Name          string `json:"name"`
}

type appTemplateResources struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
	Disk   string `json:"disk"`
}

type appTemplate struct {
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	Description      string               `json:"description"`
	Type             string               `json:"type"`
	Image            string               `json:"image,omitempty"`
	ComposeContent   string               `json:"composeContent,omitempty"`
	DefaultPorts     []appTemplatePort    `json:"defaultPorts"`
	DefaultEnvVars   map[string]string    `json:"defaultEnvVars"`
	DefaultResources appTemplateResources `json:"defaultResources"`
}

// defaultAppTemplates is the server-side source of truth for the app
// template catalog, mirroring forge/web/lib/app-templates-data.ts.
func defaultAppTemplates() []appTemplate {
	return []appTemplate{
		{
			ID: "nginx", Name: "Nginx",
			Description: "A lightweight production web server and reverse proxy.",
			Type:        "image", Image: "nginx:1.27-alpine",
			DefaultPorts:     []appTemplatePort{{HostPort: 8080, ContainerPort: 80, Protocol: "tcp", Name: "http"}},
			DefaultEnvVars:   map[string]string{},
			DefaultResources: appTemplateResources{CPU: "0.5", Memory: "256", Disk: "1024"},
		},
		{
			ID: "node", Name: "Node.js",
			Description:      "Build and run a Node.js application from a Git repository.",
			Type:             "git",
			DefaultPorts:     []appTemplatePort{{HostPort: 3000, ContainerPort: 3000, Protocol: "tcp", Name: "http"}},
			DefaultEnvVars:   map[string]string{"NODE_ENV": "production"},
			DefaultResources: appTemplateResources{CPU: "1", Memory: "512", Disk: "2048"},
		},
		{
			ID: "python", Name: "Python Web",
			Description:      "Deploy a Python web service from a Git repository.",
			Type:             "git",
			DefaultPorts:     []appTemplatePort{{HostPort: 8000, ContainerPort: 8000, Protocol: "tcp", Name: "http"}},
			DefaultEnvVars:   map[string]string{"PYTHONUNBUFFERED": "1"},
			DefaultResources: appTemplateResources{CPU: "1", Memory: "512", Disk: "2048"},
		},
		{
			ID: "postgres-compose", Name: "PostgreSQL",
			Description:      "A PostgreSQL database stack with persistent storage.",
			Type:             "compose",
			ComposeContent:   "services:\n  postgres:\n    image: postgres:16-alpine\n    environment:\n      POSTGRES_DB: app\n      POSTGRES_USER: app\n      POSTGRES_PASSWORD: change-me\n    ports:\n      - \"5432:5432\"\n    volumes:\n      - postgres-data:/var/lib/postgresql/data\nvolumes:\n  postgres-data:\n",
			DefaultPorts:     []appTemplatePort{{HostPort: 5432, ContainerPort: 5432, Protocol: "tcp", Name: "postgres"}},
			DefaultEnvVars:   map[string]string{"POSTGRES_DB": "app", "POSTGRES_USER": "app", "POSTGRES_PASSWORD": ""},
			DefaultResources: appTemplateResources{CPU: "1", Memory: "1024", Disk: "10240"},
		},
		{
			ID: "redis", Name: "Redis",
			Description: "An in-memory cache and queue service.",
			Type:        "image", Image: "redis:7-alpine",
			DefaultPorts:     []appTemplatePort{{HostPort: 6379, ContainerPort: 6379, Protocol: "tcp", Name: "redis"}},
			DefaultEnvVars:   map[string]string{},
			DefaultResources: appTemplateResources{CPU: "0.5", Memory: "256", Disk: "1024"},
		},
	}
}
