package http

import (
	"strconv"
	"strings"

	"gamepanel/forge/internal/services/tenancy"

	"github.com/gofiber/fiber/v2"
)

func registerTenancyRoutes(protected fiber.Router, cfg Config, tenancySvc *tenancy.Service) {
	if cfg.Store == nil {
		return
	}

	// ---- Organizations ----

	protected.Get("/organizations", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		orgs, err := tenancySvc.ListOrganizations(ctx, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(orgs)
	})

	protected.Post("/organizations", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		var req struct {
			Name string `json:"name"`
			Slug string `json:"slug"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		actorID := claims.Sub
		org, err := tenancySvc.CreateOrganization(ctx, tenancy.CreateOrganizationInput{
			Name:   strings.TrimSpace(req.Name),
			Slug:   strings.TrimSpace(req.Slug),
			UserID: claims.Sub,
			Actor:  &actorID,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(org)
	})

	protected.Get("/organizations/:slug", tenancyOrgAccess(tenancySvc), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		org, err := tenancySvc.GetOrganization(ctx, c.Params("slug"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "organization not found")
		}
		return c.JSON(org)
	})

	protected.Delete("/organizations/:id", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		isOwner, err := cfg.Store.OrganizationOwnedByUser(ctx, c.Params("id"), claims.Sub)
		if err != nil || !isOwner {
			if claims.Role != "admin" {
				return fiber.NewError(fiber.StatusForbidden, "only the organization owner or an admin can delete it")
			}
		}
		actorID := claims.Sub
		if err := tenancySvc.DeleteOrganization(ctx, c.Params("id"), &actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// ---- Projects ----

	protected.Get("/organizations/:id/projects", tenancyOrgAccess(tenancySvc), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		projects, err := tenancySvc.ListProjects(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(projects)
	})

	protected.Post("/organizations/:id/projects", tenancyOrgAccess(tenancySvc), func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		// Only owners and admins of the org can create projects
		ctx, cancel := requestContext()
		defer cancel()
		role := tenancySvc.ResolvePermissions(ctx, c.Params("id"), claims.Sub, claims.Role)
		if role != "owner" && role != "admin" && claims.Role != "admin" {
			return fiber.NewError(fiber.StatusForbidden, "insufficient permissions")
		}

		var req struct {
			Name        string `json:"name"`
			Slug        string `json:"slug"`
			Description string `json:"description"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		actorID := claims.Sub
		project, err := tenancySvc.CreateProject(ctx, c.Params("id"), tenancy.CreateProjectInput{
			Name:        strings.TrimSpace(req.Name),
			Slug:        strings.TrimSpace(req.Slug),
			Description: strings.TrimSpace(req.Description),
			Actor:       &actorID,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(project)
	})

	protected.Put("/projects/:id", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		var req struct {
			Name        string `json:"name"`
			Slug        string `json:"slug"`
			Description string `json:"description"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		project, err := tenancySvc.GetProject(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "project not found")
		}
		role := tenancySvc.ResolvePermissions(ctx, project.OrgID, claims.Sub, claims.Role)
		if role != "owner" && role != "admin" && claims.Role != "admin" {
			return fiber.NewError(fiber.StatusForbidden, "insufficient permissions")
		}
		updated, err := tenancySvc.UpdateProject(ctx, c.Params("id"), tenancy.CreateProjectInput{
			Name:        strings.TrimSpace(req.Name),
			Slug:        strings.TrimSpace(req.Slug),
			Description: strings.TrimSpace(req.Description),
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(updated)
	})

	protected.Delete("/projects/:id", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		project, err := tenancySvc.GetProject(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "project not found")
		}
		role := tenancySvc.ResolvePermissions(ctx, project.OrgID, claims.Sub, claims.Role)
		if role != "owner" && role != "admin" && claims.Role != "admin" {
			return fiber.NewError(fiber.StatusForbidden, "insufficient permissions")
		}
		actorID := claims.Sub
		if err := tenancySvc.DeleteProject(ctx, c.Params("id"), &actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// ---- Environments ----

	protected.Get("/projects/:id/envs", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		project, err := tenancySvc.GetProject(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "project not found")
		}
		isMember, _ := tenancySvc.UserIsOrgMember(ctx, project.OrgID, claims.Sub)
		if !isMember && claims.Role != "admin" {
			return fiber.NewError(fiber.StatusForbidden, "not a member of this organization")
		}
		envs, err := tenancySvc.ListEnvironments(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(envs)
	})

	protected.Post("/projects/:id/envs", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		project, err := tenancySvc.GetProject(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "project not found")
		}
		role := tenancySvc.ResolvePermissions(ctx, project.OrgID, claims.Sub, claims.Role)
		if role != "owner" && role != "admin" && claims.Role != "admin" {
			return fiber.NewError(fiber.StatusForbidden, "insufficient permissions")
		}

		var req struct {
			Name      string `json:"name"`
			Color     string `json:"color"`
			Protected bool   `json:"protected"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		actorID := claims.Sub
		env, err := tenancySvc.CreateEnvironment(ctx, c.Params("id"), tenancy.CreateEnvironmentInput{
			Name:      strings.TrimSpace(req.Name),
			Color:     strings.TrimSpace(req.Color),
			Protected: req.Protected,
			Actor:     &actorID,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(env)
	})

	protected.Put("/envs/:id", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		var req struct {
			Name      string `json:"name"`
			Color     string `json:"color"`
			Protected bool   `json:"protected"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		// Resolve project through environment
		var projectID string
		if err := cfg.Store.DB().QueryRow(ctx, `SELECT project_id::text FROM environments WHERE id = $1`, c.Params("id")).Scan(&projectID); err != nil {
			return fiber.NewError(fiber.StatusNotFound, "environment not found")
		}
		project, err := tenancySvc.GetProject(ctx, projectID)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "project not found")
		}
		role := tenancySvc.ResolvePermissions(ctx, project.OrgID, claims.Sub, claims.Role)
		if role != "owner" && role != "admin" && claims.Role != "admin" {
			return fiber.NewError(fiber.StatusForbidden, "insufficient permissions")
		}
		env, err := tenancySvc.UpdateEnvironment(ctx, c.Params("id"), tenancy.CreateEnvironmentInput{
			Name:      strings.TrimSpace(req.Name),
			Color:     strings.TrimSpace(req.Color),
			Protected: req.Protected,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(env)
	})

	protected.Delete("/envs/:id", func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		var projectID string
		if err := cfg.Store.DB().QueryRow(ctx, `SELECT project_id::text FROM environments WHERE id = $1`, c.Params("id")).Scan(&projectID); err != nil {
			return fiber.NewError(fiber.StatusNotFound, "environment not found")
		}
		project, err := tenancySvc.GetProject(ctx, projectID)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "project not found")
		}
		role := tenancySvc.ResolvePermissions(ctx, project.OrgID, claims.Sub, claims.Role)
		if role != "owner" && role != "admin" && claims.Role != "admin" {
			return fiber.NewError(fiber.StatusForbidden, "insufficient permissions")
		}
		actorID := claims.Sub
		if err := tenancySvc.DeleteEnvironment(ctx, c.Params("id"), &actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// ---- Team Members ----

	protected.Get("/organizations/:id/members", tenancyOrgAccess(tenancySvc), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		members, err := tenancySvc.ListTeamMembers(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(members)
	})

	protected.Post("/organizations/:id/members", tenancyOrgAccess(tenancySvc), func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		role := tenancySvc.ResolvePermissions(ctx, c.Params("id"), claims.Sub, claims.Role)
		if role != "owner" && role != "admin" && claims.Role != "admin" {
			return fiber.NewError(fiber.StatusForbidden, "only owners and admins can manage members")
		}

		var req struct {
			UserID string `json:"userId"`
			Role   string `json:"role"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.Role == "" {
			req.Role = "member"
		}
		member, err := tenancySvc.AddTeamMember(ctx, tenancy.AddMemberInput{
			OrgID:   c.Params("id"),
			UserID:  req.UserID,
			Role:    req.Role,
			ActorID: claims.Sub,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(member)
	})

	protected.Put("/organizations/:id/members/:userId", tenancyOrgAccess(tenancySvc), func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		role := tenancySvc.ResolvePermissions(ctx, c.Params("id"), claims.Sub, claims.Role)
		if role != "owner" && role != "admin" && claims.Role != "admin" {
			return fiber.NewError(fiber.StatusForbidden, "only owners and admins can manage members")
		}

		var req struct {
			Role string `json:"role"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := tenancySvc.UpdateMemberRole(ctx, tenancy.UpdateMemberInput{
			OrgID:   c.Params("id"),
			UserID:  c.Params("userId"),
			Role:    req.Role,
			ActorID: claims.Sub,
		}); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Delete("/organizations/:id/members/:userId", tenancyOrgAccess(tenancySvc), func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		role := tenancySvc.ResolvePermissions(ctx, c.Params("id"), claims.Sub, claims.Role)
		if role != "owner" && role != "admin" && claims.Role != "admin" {
			return fiber.NewError(fiber.StatusForbidden, "only owners and admins can manage members")
		}

		if err := tenancySvc.RemoveTeamMember(ctx, tenancy.RemoveMemberInput{
			OrgID:   c.Params("id"),
			UserID:  c.Params("userId"),
			ActorID: claims.Sub,
		}); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// ---- Org-scoped server listing ----

	protected.Get("/organizations/:id/servers", tenancyOrgAccess(tenancySvc), func(c *fiber.Ctx) error {
		page := 1
		if p := c.Query("page"); p != "" {
			if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
				page = parsed
			}
		}
		perPage := 50
		if pp := c.Query("per_page"); pp != "" {
			if parsed, err := strconv.Atoi(pp); err == nil && parsed > 0 && parsed <= 100 {
				perPage = parsed
			}
		}
		search := c.Query("search")
		ctx, cancel := requestContext()
		defer cancel()
		servers, total, err := tenancySvc.ScopeServersByOrg(ctx, c.Params("id"), page, perPage, search)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{
			"data": servers,
			"meta": fiber.Map{
				"pagination": fiber.Map{
					"current":  page,
					"count":    len(servers),
					"total":    total,
					"per_page": perPage,
				},
			},
		})
	})
}

func tenancyOrgAccess(tenancySvc *tenancy.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		if claims.Role == "admin" {
			return c.Next()
		}
		ctx, cancel := requestContext()
		defer cancel()
		isMember, err := tenancySvc.UserIsOrgMember(ctx, c.Params("id"), claims.Sub)
		if err != nil || !isMember {
			return fiber.NewError(fiber.StatusForbidden, "not a member of this organization")
		}
		return c.Next()
	}
}
