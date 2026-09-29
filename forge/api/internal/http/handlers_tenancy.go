package http

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"gamepanel/forge/internal/services/envvars"
	"gamepanel/forge/internal/services/tenancy"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// tenancyAdminGate is the role test every tenancy write handler applies:
// organization owners and admins, plus a platform admin regardless of their
// organization role.
func tenancyAdminGate(role string, claims tokenClaims) bool {
	return role == "owner" || role == "admin" || claims.Role == "admin"
}

// requireProjectAdmin resolves a project's owning organization and reports
// whether the caller may administer it. It returns a ready-to-return
// *fiber.Error, or nil when the caller is permitted.
//
// This replaces the GetProject → ResolvePermissions → role-check preamble that
// was repeated verbatim in each project and environment write handler.
func requireProjectAdmin(ctx context.Context, tenancySvc *tenancy.Service, projectID string, claims tokenClaims) error {
	project, err := tenancySvc.GetProject(ctx, projectID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "project not found")
	}
	if !tenancyAdminGate(tenancySvc.ResolvePermissions(ctx, project.OrgID, claims.Sub, claims.Role), claims) {
		return fiber.NewError(fiber.StatusForbidden, "insufficient permissions")
	}
	return nil
}

// requireEnvironmentAdmin is requireProjectAdmin for a handler addressed by
// environment ID.
//
// The environment → project → organization walk goes through
// Store.ResolveEnvContext, which already existed for exactly this purpose. The
// handlers previously ran `SELECT project_id FROM environments` inline and
// then made a second call to load the project, so this drops a raw query out
// of the HTTP layer and a round trip with it.
func requireEnvironmentAdmin(ctx context.Context, st *store.Store, tenancySvc *tenancy.Service, envID string, claims tokenClaims) error {
	envCtx, err := st.ResolveEnvContext(ctx, envID)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "environment not found")
	}
	if !tenancyAdminGate(tenancySvc.ResolvePermissions(ctx, envCtx.Project.OrgID, claims.Sub, claims.Role), claims) {
		return fiber.NewError(fiber.StatusForbidden, "insufficient permissions")
	}
	return nil
}

func registerTenancyRoutes(protected fiber.Router, cfg Config, tenancySvc *tenancy.Service, envvarSvc *envvars.Service) {
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
		if err := rejectScopedTenancyWrites(c); err != nil {
			return err
		}
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
		if err := rejectScopedTenancyWrites(c); err != nil {
			return err
		}
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
		if err := rejectScopedTenancyWrites(c); err != nil {
			return err
		}
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
		if err := rejectScopedTenancyWrites(c); err != nil {
			return err
		}
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
		if err := requireProjectAdmin(ctx, tenancySvc, c.Params("id"), claims); err != nil {
			return err
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
		if err := rejectScopedTenancyWrites(c); err != nil {
			return err
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := requireProjectAdmin(ctx, tenancySvc, c.Params("id"), claims); err != nil {
			return err
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
		isMember, err := tenancySvc.UserIsOrgMember(ctx, project.OrgID, claims.Sub)
		if err != nil && claims.Role != "admin" {
			// Still denies, as before — but a failed membership lookup used to
			// be indistinguishable from a definite "not a member", which sends
			// the operator looking at permissions during a database outage.
			return respondInternalError(c, err)
		}
		if !isMember && claims.Role != "admin" {
			return fiber.NewError(fiber.StatusForbidden, "not a member of this organization")
		}
		envs, err := tenancySvc.ListEnvironments(ctx, c.Params("id"))
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(envs)
	})

	protected.Post("/projects/:id/envs", func(c *fiber.Ctx) error {
		if err := rejectScopedTenancyWrites(c); err != nil {
			return err
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := requireProjectAdmin(ctx, tenancySvc, c.Params("id"), claims); err != nil {
			return err
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
		if err := rejectScopedTenancyWrites(c); err != nil {
			return err
		}
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
		if err := requireEnvironmentAdmin(ctx, cfg.Store, tenancySvc, c.Params("id"), claims); err != nil {
			return err
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
		if err := rejectScopedTenancyWrites(c); err != nil {
			return err
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := requireEnvironmentAdmin(ctx, cfg.Store, tenancySvc, c.Params("id"), claims); err != nil {
			return err
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
		if err := rejectScopedTenancyWrites(c); err != nil {
			return err
		}
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
		if err := rejectScopedTenancyWrites(c); err != nil {
			return err
		}
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
		if err := rejectScopedTenancyWrites(c); err != nil {
			return err
		}
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

	// ---- Organizations PATCH ----

	protected.Patch("/organizations/:id", func(c *fiber.Ctx) error {
		if err := rejectScopedTenancyWrites(c); err != nil {
			return err
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := tenancySvc.CanManageOrg(ctx, c.Params("id"), claims.Sub, claims.Role); err != nil {
			return fiber.NewError(fiber.StatusForbidden, err.Error())
		}
		var req struct {
			Name *string `json:"name"`
			Slug *string `json:"slug"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		org, err := tenancySvc.GetOrganization(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "organization not found")
		}
		name := org.Name
		if req.Name != nil {
			name = *req.Name
		}
		slug := org.Slug
		if req.Slug != nil {
			slug = *req.Slug
		}
		_, err = cfg.Store.UpdateOrganization(ctx, c.Params("id"), name, slug)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// ---- Invitations ----

	protected.Post("/organizations/:id/invitations", tenancyOrgAccess(tenancySvc), func(c *fiber.Ctx) error {
		if err := rejectScopedTenancyWrites(c); err != nil {
			return err
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		role := tenancySvc.ResolvePermissions(ctx, c.Params("id"), claims.Sub, claims.Role)
		if role != "owner" && role != "admin" && claims.Role != "admin" {
			return fiber.NewError(fiber.StatusForbidden, "only owners and admins can invite members")
		}
		var req struct {
			Email string `json:"email"`
			Role  string `json:"role"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.Role == "" {
			req.Role = "member"
		}
		inv, err := tenancySvc.CreateInvitation(ctx, tenancy.CreateInvitationInput{
			OrgID:     c.Params("id"),
			Email:     req.Email,
			Role:      req.Role,
			InvitedBy: claims.Sub,
			TTL:       7 * 24 * time.Hour,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(inv)
	})

	protected.Get("/organizations/:id/invitations", tenancyOrgAccess(tenancySvc), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		invites, err := tenancySvc.ListInvitations(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(invites)
	})

	acceptTenancyInvitation := func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		var req struct {
			Token string `json:"token"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := tenancySvc.AcceptInvitation(ctx, req.Token, claims.Sub); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	}
	// Only /tenancy/invitations/accept is registered here. A second
	// registration on /invitations/accept used to sit alongside it, but
	// registerAuthRoutes claims that path first for *subuser* invitations, and
	// Fiber resolves overlapping paths in registration order — so this copy
	// could never run, and a caller posting an organization invitation token
	// there had it looked up as a subuser token and rejected. The two
	// invitation kinds live in different tables and must keep different paths.
	protected.Post("/tenancy/invitations/accept", acceptTenancyInvitation)

	protected.Delete("/organizations/:id/invitations/:invId", tenancyOrgAccess(tenancySvc), func(c *fiber.Ctx) error {
		if err := rejectScopedTenancyWrites(c); err != nil {
			return err
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		role := tenancySvc.ResolvePermissions(ctx, c.Params("id"), claims.Sub, claims.Role)
		if role != "owner" && role != "admin" && claims.Role != "admin" {
			return fiber.NewError(fiber.StatusForbidden, "only owners and admins can manage invitations")
		}
		if err := tenancySvc.RevokeInvitation(ctx, c.Params("id"), c.Params("invId")); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// ---- Granular Permissions ----

	protected.Get("/organizations/:id/members/:userId/permissions", tenancyOrgAccess(tenancySvc), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		perms, err := tenancySvc.GetMemberPermissions(ctx, c.Params("id"), c.Params("userId"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, err.Error())
		}
		return c.JSON(perms)
	})

	protected.Put("/organizations/:id/members/:userId/permissions", tenancyOrgAccess(tenancySvc), func(c *fiber.Ctx) error {
		if err := rejectScopedTenancyWrites(c); err != nil {
			return err
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		ctx, cancel := requestContext()
		defer cancel()
		role := tenancySvc.ResolvePermissions(ctx, c.Params("id"), claims.Sub, claims.Role)
		if role != "owner" && role != "admin" && claims.Role != "admin" {
			return fiber.NewError(fiber.StatusForbidden, "only owners and admins can manage permissions")
		}
		var req store.GranularPermissions
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if err := tenancySvc.SetMemberPermissions(ctx, c.Params("id"), c.Params("userId"), req, claims.Sub); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// ---- Environment Variables ----

	protected.Get("/environments/:id/env-vars", func(c *fiber.Ctx) error {
		if _, err := phase2EnvAccess(c, &cfg); err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		vars, err := envvarSvc.List(ctx, "environment", c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(vars)
	})

	protected.Post("/environments/:id/env-vars", func(c *fiber.Ctx) error {
		if err := rejectScopedTenancyWrites(c); err != nil {
			return err
		}
		if _, err := phase2EnvAccess(c, &cfg); err != nil {
			return err
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		var req struct {
			Key         string `json:"key"`
			Value       string `json:"value"`
			IsSensitive bool   `json:"isSensitive"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		actorID := claims.Sub
		envID := c.Params("id")
		v, err := envvarSvc.Create(c.Context(), envvars.CreateEnvVarInput{
			EnvironmentID: &envID,
			Scope:         "environment",
			Key:           req.Key,
			Value:         req.Value,
			IsSensitive:   req.IsSensitive,
			Actor:         &actorID,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(v)
	})

	protected.Put("/env-vars/:id", envVarAccess(cfg, envvarSvc), func(c *fiber.Ctx) error {
		if err := rejectScopedTenancyWrites(c); err != nil {
			return err
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		var req struct {
			Value       string `json:"value"`
			IsSensitive bool   `json:"isSensitive"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		actorID := claims.Sub
		v, err := envvarSvc.Update(c.Context(), c.Params("id"), envvars.UpdateEnvVarInput{
			Value:       req.Value,
			IsSensitive: req.IsSensitive,
			Actor:       &actorID,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(v)
	})

	protected.Delete("/env-vars/:id", envVarAccess(cfg, envvarSvc), func(c *fiber.Ctx) error {
		if err := rejectScopedTenancyWrites(c); err != nil {
			return err
		}
		actorID := ""
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = claims.Sub
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := envvarSvc.Delete(ctx, c.Params("id"), &actorID); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Get("/environments/:id/env-vars/resolved", func(c *fiber.Ctx) error {
		envCtx, err := phase2EnvAccess(c, &cfg)
		if err != nil {
			return err
		}
		ctx, cancel := requestContext()
		defer cancel()
		var serviceID string
		resolved, err := envvarSvc.Resolve(ctx, envCtx.Org.ID, envCtx.Project.ID, c.Params("id"), serviceID)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(resolved)
	})

	protected.Get("/env-vars/:id/revisions", envVarAccess(cfg, envvarSvc), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		revisions, err := envvarSvc.Revisions(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(revisions)
	})

	protected.Get("/projects/:id/env-vars", projectEnvAccess(cfg), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		vars, err := envvarSvc.List(ctx, "project", c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(vars)
	})

	protected.Post("/projects/:id/env-vars", projectEnvAccess(cfg), func(c *fiber.Ctx) error {
		if err := rejectScopedTenancyWrites(c); err != nil {
			return err
		}
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		var req struct {
			Key         string `json:"key"`
			Value       string `json:"value"`
			IsSensitive bool   `json:"isSensitive"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		actorID := claims.Sub
		projectID := c.Params("id")
		v, err := envvarSvc.Create(c.Context(), envvars.CreateEnvVarInput{
			ProjectID:   &projectID,
			Scope:       "project",
			Key:         req.Key,
			Value:       req.Value,
			IsSensitive: req.IsSensitive,
			Actor:       &actorID,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		return c.Status(fiber.StatusCreated).JSON(v)
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
			// Safe DTOs: never leak transferRunToken or secrets into console payloads.
			"data": store.ServersToDTO(servers),
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
		// Scoped credentials (API keys / OAuth) carry only servers.* scopes;
		// tenancy has no delegated scope family, so they are rejected here
		// rather than silently inheriting the caller's org membership.
		if scoped, _ := c.Locals("scopedAuth").(bool); scoped {
			return fiber.NewError(fiber.StatusForbidden, "scoped credentials cannot access organization resources")
		}
		if claims.Role == "admin" {
			// Admins with scoped credentials were already rejected above;
			// full sessions pass through.
			return c.Next()
		}
		ctx, cancel := requestContext()
		defer cancel()
		// :slug routes (GET /organizations/:slug) carry the slug, not the id.
		// Reading Params("id") there yields "" and would deny every member.
		orgParam := c.Params("id")
		if orgParam == "" {
			orgParam = c.Params("slug")
		}
		if orgParam == "" {
			return fiber.NewError(fiber.StatusBadRequest, "organization identifier is required")
		}
		// When the identifier is a slug, resolve it to the org ID for the
		// membership check; fail closed when it cannot be resolved.
		orgID := orgParam
		if org, err := tenancySvc.GetOrganization(ctx, orgParam); err == nil && org.ID != "" {
			orgID = org.ID
		}
		isMember, err := tenancySvc.UserIsOrgMember(ctx, orgID, claims.Sub)
		if err != nil || !isMember {
			// Fall back to checking the raw param (covers ID-vs-slug stores
			// where GetOrganization resolves differently than membership).
			if orgID != orgParam {
				if retry, rerr := tenancySvc.UserIsOrgMember(ctx, orgParam, claims.Sub); rerr == nil && retry {
					return c.Next()
				}
			}
			return fiber.NewError(fiber.StatusForbidden, "not a member of this organization")
		}
		return c.Next()
	}
}

// rejectScopedTenancyWrites blocks API-key / OAuth credentials on mutating
// tenancy routes. Tenancy mutations (create org/project/env, invite, role
// changes) have no delegated scope family, so scoped creds must not drive
// them even when the owner also holds a valid key.
func rejectScopedTenancyWrites(c *fiber.Ctx) error {
	if scoped, _ := c.Locals("scopedAuth").(bool); scoped {
		return fiber.NewError(fiber.StatusForbidden, "scoped credentials cannot modify organization resources")
	}
	return nil
}

// projectEnvAccess guards project-scoped env-var routes: the :id identifies a
// project, so the owning org is resolved and the caller's membership verified.
// Scopes whose owning org cannot be resolved are denied (fail closed); admins
// short-circuit. Mirrors the existing envVarAccess / phase2EnvAccess pattern.
func projectEnvAccess(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		if scoped, _ := c.Locals("scopedAuth").(bool); scoped {
			return fiber.NewError(fiber.StatusForbidden, "scoped credentials cannot access project resources")
		}
		if claims.Role == "admin" {
			return c.Next()
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		orgID, err := cfg.Store.GetProjectOrgID(ctx, c.Params("id"))
		switch {
		case errors.Is(err, store.ErrProjectNotFound):
			return fiber.NewError(fiber.StatusNotFound, "project not found")
		case err != nil:
			// Fails closed either way, but a database outage should not be
			// reported to the caller as "this project does not exist".
			return fiber.NewError(fiber.StatusForbidden, "cannot resolve organization for this project")
		}
		member, err := cfg.Store.UserIsOrgMember(ctx, orgID, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if !member {
			return fiber.NewError(fiber.StatusForbidden, "not a member of this organization")
		}
		return c.Next()
	}
}

// envVarAccess guards routes keyed by an environment-variable id. Unlike the
// env-scoped routes, the :id here identifies the variable rather than its
// environment, so the owning organization is resolved from the variable's
// environment/project/org scope and the caller's membership is verified — the
// same env→project→org check every sibling route performs. Scopes whose owning
// org cannot be resolved are denied (fail closed); admins short-circuit.
func envVarAccess(cfg Config, envvarSvc *envvars.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		claims, ok := c.Locals("user").(tokenClaims)
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "missing session")
		}
		if scoped, _ := c.Locals("scopedAuth").(bool); scoped {
			return fiber.NewError(fiber.StatusForbidden, "scoped credentials cannot access env var resources")
		}
		if claims.Role == "admin" {
			return c.Next()
		}
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		v, err := envvarSvc.Get(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "env var not found")
		}
		var orgID string
		switch {
		case v.OrgID != nil && *v.OrgID != "":
			orgID = *v.OrgID
		case v.EnvironmentID != nil && *v.EnvironmentID != "":
			envCtx, err := cfg.Store.ResolveEnvContext(ctx, *v.EnvironmentID)
			if err != nil {
				return fiber.NewError(fiber.StatusNotFound, "environment not found")
			}
			orgID = envCtx.Org.ID
		case v.ProjectID != nil && *v.ProjectID != "":
			resolved, err := cfg.Store.GetProjectOrgID(ctx, *v.ProjectID)
			if err != nil {
				return fiber.NewError(fiber.StatusNotFound, "project not found")
			}
			orgID = resolved
		}
		if orgID == "" {
			return fiber.NewError(fiber.StatusForbidden, "cannot resolve organization for this env var")
		}
		member, err := cfg.Store.UserIsOrgMember(ctx, orgID, claims.Sub)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if !member {
			return fiber.NewError(fiber.StatusForbidden, "not a member of this organization")
		}
		return c.Next()
	}
}
