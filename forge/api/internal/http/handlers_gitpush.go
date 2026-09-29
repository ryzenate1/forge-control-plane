package http

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	gitpushsvc "gamepanel/forge/internal/services/gitpush"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// CreateGitPushAppRequest is the body for POST /git-push/apps.
type CreateGitPushAppRequest struct {
	Name   string `json:"name"`
	NodeID string `json:"nodeId"`
	// Builder is herokuish|dockerfile|nixpacks|null; empty defaults to dockerfile.
	Builder string `json:"builder"`
	// Branch is the deploy branch (Dokku's `git:set-deploy-branch`); empty defaults to main.
	Branch string `json:"branch"`
}

// UpdateGitPushAppRequest is the body for PATCH /git-push/apps/:id. Pointer
// fields distinguish "not sent" from "set to false/empty".
type UpdateGitPushAppRequest struct {
	Builder    *string `json:"builder"`
	Branch     *string `json:"branch"`
	AutoDeploy *bool   `json:"autoDeploy"`
}

// GitPushEnvRequest is the body for PUT /git-push/apps/:id/env. Keys may be
// sent as `set`/`vars`; `delete` names variables to remove in the same call.
type GitPushEnvRequest struct {
	Set    map[string]string `json:"set"`
	Vars   map[string]string `json:"vars"`
	Delete []string          `json:"delete"`
}

// gitPushResponse decorates an app with the derived fields the admin UI shows
// and the store does not keep: the pushable remote and whether the bare
// repository is actually provisioned yet.
type gitPushResponse struct {
	*gitpushsvc.App
	RemoteURL      string `json:"remoteUrl,omitempty"`
	RemoteResolved bool   `json:"remoteResolved"`
	ProvisionHint  string `json:"provisionHint,omitempty"`
}

func (r *gitPushResponse) decorate(ctx context.Context, svc *gitpushsvc.Service, app *gitpushsvc.App) {
	r.App = app
	remote, err := svc.RemoteURL(ctx, app)
	if err == nil && remote != "" {
		r.RemoteURL = remote
		r.RemoteResolved = true
		return
	}
	// The remote cannot be built until the node advertises an address, and the
	// repository still needs `git init --bare` plus the post-receive hook. Say
	// so instead of handing back a URL that looks finished.
	r.ProvisionHint = "Repository path " + app.RepoPath + " is reserved on the node; the bare repository and its post-receive hook still need to be created there (Beacon exposes host file operations but no host exec)."
}

// mergeGitPushEnvSets merges the `set`/`vars` aliases for PUT env. Both keys
// exist for backward compatibility; when both carry values they are unioned,
// and a key present in both with different values is rejected instead of
// silently dropping one alias.
func mergeGitPushEnvSets(set, vars map[string]string) (map[string]string, error) {
	if len(set) == 0 {
		return vars, nil
	}
	if len(vars) == 0 {
		return set, nil
	}
	merged := make(map[string]string, len(set)+len(vars))
	for k, v := range set {
		merged[k] = v
	}
	for k, v := range vars {
		if existing, ok := merged[k]; ok && existing != v {
			return nil, errors.New("conflicting values for environment variable " + k + ": set and vars disagree")
		}
		merged[k] = v
	}
	return merged, nil
}

// gitPushService resolves the configured service, falling back to one built
// inline so the routes work even before main wires the deployer. The fallback
// deliberately passes a nil deployer: pushes are then recorded and marked
// skipped rather than reported as deployed.
func gitPushService(cfg Config) *gitpushsvc.Service {
	if cfg.Store == nil {
		return nil
	}
	if cfg.GitPushService != nil {
		return cfg.GitPushService
	}
	svc := gitpushsvc.New(cfg.Store, cfg.Daemon, nil, cfg.Logger)
	if host := panelHostname(cfg.PanelURL); host != "" {
		svc.WithPanelEndpoint(host, 22)
	}
	return svc
}

func panelHostname(panelURL string) string {
	panelURL = strings.TrimSpace(panelURL)
	if panelURL == "" {
		return ""
	}
	if !strings.Contains(panelURL, "://") {
		panelURL = "https://" + panelURL
	}
	parsed, err := url.Parse(panelURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func gitPushAppResponse(c *fiber.Ctx, svc *gitpushsvc.Service, app *gitpushsvc.App) error {
	ctx, cancel := gitPushContext(c)
	defer cancel()
	out := gitPushResponse{}
	out.decorate(ctx, svc, app)
	return c.JSON(out)
}

// gitPushContext widens requestContext's short budget for the derive-a-remote
// lookup, which may round-trip to the node store.
func gitPushContext(c *fiber.Ctx) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(c.Context()), 10*time.Second)
}

func writeGitPushError(c *fiber.Ctx, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gitpushsvc.ErrAppNotFound), errors.Is(err, store.ErrGitPushAppNotFound):
		return fiber.NewError(fiber.StatusNotFound, "git-push app not found")
	case errors.Is(err, gitpushsvc.ErrSignature):
		return fiber.NewError(fiber.StatusUnauthorized, "invalid signature")
	case errors.Is(err, gitpushsvc.ErrInvalidName),
		errors.Is(err, gitpushsvc.ErrInvalidBuilder),
		errors.Is(err, gitpushsvc.ErrInvalidBranch),
		errors.Is(err, gitpushsvc.ErrInvalidEnvKey),
		errors.Is(err, gitpushsvc.ErrNotProvisioned):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	default:
		return respondInternalError(c, err)
	}
}

// registerGitPushRoutes mounts the admin surface. Everything under
// /git-push/apps is session-authenticated (it rides the `protected` router) and
// additionally requires the admin role, because registering an app reserves a
// path on a node and mints a callback secret.
func registerGitPushRoutes(protected fiber.Router, cfg Config, mutationLimiter, adminIPAccess fiber.Handler) {
	svc := gitPushService(cfg)
	if svc == nil {
		return
	}

	group := protected.Group("/git-push")
	if adminIPAccess != nil {
		group.Use(adminIPAccess)
	}

	group.Get("/apps", requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		apps, err := svc.ListApps(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		out := make([]gitPushResponse, 0, len(apps))
		deriveCtx, deriveCancel := gitPushContext(c)
		defer deriveCancel()
		for i := range apps {
			row := gitPushResponse{}
			row.decorate(deriveCtx, svc, &apps[i])
			out = append(out, row)
		}
		return c.JSON(out)
	})

	group.Post("/apps", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		var req CreateGitPushAppRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.NodeID) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "name and nodeId are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := svc.CreateApp(ctx, req.Name, req.NodeID, req.Builder, req.Branch)
		if err != nil {
			return writeGitPushError(c, err)
		}
		return gitPushAppResponse(c, svc, app)
	})

	group.Get("/apps/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		app, err := svc.GetApp(ctx, c.Params("id"))
		if err != nil {
			return writeGitPushError(c, err)
		}
		return gitPushAppResponse(c, svc, app)
	})

	group.Patch("/apps/:id", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		var req UpdateGitPushAppRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := svc.UpdateApp(ctx, c.Params("id"), req.Builder, req.Branch, req.AutoDeploy)
		if err != nil {
			return writeGitPushError(c, err)
		}
		return gitPushAppResponse(c, svc, app)
	})

	group.Delete("/apps/:id", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.DeleteApp(ctx, c.Params("id")); err != nil {
			return writeGitPushError(c, err)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	group.Get("/apps/:id/events", requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if _, err := svc.GetApp(ctx, c.Params("id")); err != nil {
			return writeGitPushError(c, err)
		}
		events, err := svc.ListEvents(ctx, c.Params("id"), 50)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(events)
	})

	// The remote is derived, not stored, so it gets its own read for the
	// "copy this into `git remote add`" affordance.
	group.Get("/apps/:id/remote", requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := gitPushContext(c)
		defer cancel()
		app, err := svc.GetApp(ctx, c.Params("id"))
		if err != nil {
			return writeGitPushError(c, err)
		}
		remote, err := svc.RemoteURL(ctx, app)
		if err != nil {
			return fiber.NewError(fiber.StatusConflict, err.Error())
		}
		return c.JSON(map[string]string{"remoteUrl": remote, "repoPath": app.RepoPath})
	})

	group.Put("/apps/:id/env", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		var req GitPushEnvRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := svc.GetApp(ctx, c.Params("id"))
		if err != nil {
			return writeGitPushError(c, err)
		}
		if len(req.Delete) > 0 {
			if app, err = svc.DeleteEnv(ctx, app.ID, req.Delete); err != nil {
				return writeGitPushError(c, err)
			}
		}
		set, err := mergeGitPushEnvSets(req.Set, req.Vars)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		if len(set) > 0 {
			if app, err = svc.SetEnv(ctx, app.ID, set); err != nil {
				return writeGitPushError(c, err)
			}
		}
		return gitPushAppResponse(c, svc, app)
	})

	group.Delete("/apps/:id/env", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		var req GitPushEnvRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		keys := req.Delete
		if len(keys) == 0 {
			for k := range req.Set {
				keys = append(keys, k)
			}
			for k := range req.Vars {
				keys = append(keys, k)
			}
		}
		if len(keys) == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "no environment variable names supplied")
		}
		ctx, cancel := requestContext()
		defer cancel()
		app, err := svc.DeleteEnv(ctx, c.Params("id"), keys)
		if err != nil {
			return writeGitPushError(c, err)
		}
		return gitPushAppResponse(c, svc, app)
	})

	// Rotating invalidates the hook on the node, so it is confirm-gated in the
	// UI and limited here as well.
	group.Post("/apps/:id/rotate-secret", mutationLimiter, requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		app, err := svc.RotateSecret(ctx, c.Params("id"))
		if err != nil {
			return writeGitPushError(c, err)
		}
		return gitPushAppResponse(c, svc, app)
	})
}

// registerGitPushWebhookRoutes mounts the node-facing half of the flow on the
// public v1 router, exactly like registerGitWebhookRoutes does for provider
// webhooks. There is no session to authenticate here: the push came from a
// beacon's post-receive hook, so the HMAC-SHA256 of the raw body against the
// app's shared secret *is* the credential.
func registerGitPushWebhookRoutes(v1 fiber.Router, cfg Config) {
	svc := gitPushService(cfg)
	if svc == nil {
		return
	}

	v1.Post("/git-push/receive/:slug", func(c *fiber.Ctx) error {
		signature := c.Get("X-Git-Push-Signature")
		if signature == "" {
			signature = c.Get("X-Hub-Signature-256")
		}
		body := c.Body()

		// A deploy runs on the build machine and can outlive the request, so it
		// must not inherit a short UI timeout; it keeps the request's values
		// only for logging correlation.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Context()), 10*time.Minute)
		defer cancel()

		result, err := svc.HandleReceive(ctx, c.Params("slug"), signature, body)
		if err != nil {
			if cfg.Logger != nil {
				cfg.Logger.Warn("git-push receive rejected", "slug", c.Params("slug"), "error", err)
			}
			return writeGitPushError(c, err)
		}
		if cfg.Logger != nil {
			cfg.Logger.Debug("git-push receive processed", "slug", c.Params("slug"), "refs", len(result.Events), "deployed", result.Deployed)
		}
		return c.JSON(result)
	})
}
