package http

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gamepanel/forge/internal/services/pipeline"

	"github.com/gofiber/fiber/v2"
)

const phase5RegistrarName = "phase5-pipelines"

// pipelineWebhookSecretEnv gates the public webhook trigger. When unset the
// webhook route is not mounted at all (rather than accepting unsigned calls),
// matching the "unknown is not authorized" convention.
const pipelineWebhookSecretEnv = "PIPELINE_WEBHOOK_SECRET"

func init() {
	RegisterPhaseRegistrar(phase5RegistrarName, 150, registerPhase5PipelineRoutes)
}

// registerPhase5PipelineRoutes mounts the CI/CD pipeline surface the admin
// Pipelines page (forge/web/app/admin/pipelines/page.tsx) calls: definitions,
// triggered runs, streamed logs, artifacts, approvals and the webhook trigger.
//
// The pipeline service already ships complete (services/pipeline, migrations
// 185-189) and has its own pgxpool-backed Store; this registrar is the missing
// wiring. It is fully additive: without a Postgres pool or the dependent
// build/compose/deployment services it logs and skips instead of crashing, and
// every /pipelines path then 404s intentionally.
func registerPhase5PipelineRoutes(v1 fiber.Router, protected fiber.Router, cfg *Config) error {
	if cfg.Store == nil {
		return fmt.Errorf("%w: store not configured, pipeline routes not mounted", ErrPhaseSkipped)
	}
	// Single validated surface: prefer the injected service main wires so the
	// DI container stays the authority; only build inline for dev/test when
	// the field is unset.
	if cfg.PipelineService != nil {
		pipelineRoutes(v1, protected, cfg, cfg.PipelineService)
		return nil
	}
	pool := cfg.Store.DB()
	if pool == nil {
		return fmt.Errorf("%w: no postgres pool, pipeline routes not mounted", ErrPhaseSkipped)
	}

	svc, err := pipeline.New(pipeline.Options{
		Store:          pipeline.NewStore(pool),
		SharedStore:    cfg.Store,
		Daemon:         cfg.Daemon,
		BuildService:   cfg.BuildService,
		ComposeService: cfg.ComposeService,
		DeployService:  cfg.DeploymentSvc,
		Logger:         cfg.Logger,
		DataDir:        pipelineDataDir(),
	})
	if err != nil {
		return err
	}

	if cfg.BackgroundContext != nil {
		svc.Start(cfg.BackgroundContext)
	} else {
		svc.Start(context.Background())
	}

	pipelineRoutes(v1, protected, cfg, svc)
	return nil
}

func pipelineRoutes(v1 fiber.Router, protected fiber.Router, cfg *Config, svc *pipeline.Service) {
	mutationLimiter := RateLimiter(GetRateLimitForEndpoint("mutation", cfg.Redis, cfg.RedisEnabled && strings.EqualFold(strings.TrimSpace(cfg.AppEnv), "production")))

	defs := protected.Group("/pipelines", requireRole("admin"))
	defs.Get("", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		items, err := svc.ListDefinitions(ctx)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": items})
	})
	defs.Get("/:id", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		def, err := svc.GetDefinition(ctx, c.Params("id"))
		if err != nil {
			return mapPipelineErr(err)
		}
		return c.JSON(fiber.Map{"data": def})
	})
	defs.Post("", mutationLimiter, func(c *fiber.Ctx) error {
		var def pipeline.Definition
		if err := c.BodyParser(&def); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		def.Name = strings.TrimSpace(def.Name)
		if def.Name == "" {
			return fiber.NewError(fiber.StatusBadRequest, "name is required")
		}
		if len(def.Stages) == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "at least one stage is required")
		}
		def.CreatedBy = claimSub(c)
		ctx, cancel := requestContext()
		defer cancel()
		created, err := svc.CreateDefinition(ctx, &def)
		if err != nil {
			return mapPipelineErr(err)
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": created})
	})
	defs.Put("/:id", mutationLimiter, func(c *fiber.Ctx) error {
		var def pipeline.Definition
		if err := c.BodyParser(&def); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		def.ID = c.Params("id")
		ctx, cancel := requestContext()
		defer cancel()
		updated, err := svc.UpdateDefinition(ctx, def.ID, &def)
		if err != nil {
			return mapPipelineErr(err)
		}
		return c.JSON(fiber.Map{"data": updated})
	})
	defs.Delete("/:id", mutationLimiter, func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.DeleteDefinition(ctx, c.Params("id")); err != nil {
			return mapPipelineErr(err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})
	// Trigger a manual run of a definition.
	defs.Post("/:id/runs", mutationLimiter, func(c *fiber.Ctx) error {
		var req struct {
			Trigger string `json:"trigger"`
		}
		_ = c.BodyParser(&req)
		trigger := strings.TrimSpace(req.Trigger)
		if trigger == "" {
			trigger = "manual"
		}
		ctx, cancel := requestContext()
		defer cancel()
		run, err := svc.TriggerRun(ctx, c.Params("id"), trigger, claimSub(c))
		if err != nil {
			return mapPipelineErr(err)
		}
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": run})
	})

	runs := protected.Group("/pipeline-runs", requireRole("admin"))
	runs.Get("", func(c *fiber.Ctx) error {
		limit := 50
		if n, err := strconv.Atoi(c.Query("limit", "")); err == nil && n > 0 && n <= 500 {
			limit = n
		}
		offset := 0
		if n, err := strconv.Atoi(c.Query("offset", "")); err == nil && n > 0 {
			offset = n
		}
		ctx, cancel := requestContext()
		defer cancel()
		items, err := svc.ListRuns(ctx, c.Query("pipelineId"), c.Query("status"), limit, offset)
		if err != nil {
			return respondInternalError(c, err)
		}
		return c.JSON(fiber.Map{"data": items})
	})
	runs.Get("/:id", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		run, err := svc.GetRun(ctx, c.Params("id"))
		if err != nil {
			return mapPipelineErr(err)
		}
		return c.JSON(fiber.Map{"data": run})
	})
	runs.Get("/:id/logs", func(c *fiber.Ctx) error {
		var after int64
		if raw := strings.TrimSpace(c.Query("after")); raw != "" {
			if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
				after = n
			}
		}
		ctx, cancel := requestContext()
		defer cancel()
		logs, err := svc.ListLogs(ctx, c.Params("id"), after)
		if err != nil {
			return mapPipelineErr(err)
		}
		if logs == nil {
			logs = []pipeline.LogEntry{}
		}
		return c.JSON(fiber.Map{"data": logs})
	})
	runs.Get("/:id/artifacts", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		arts, err := svc.ListArtifacts(ctx, c.Params("id"))
		if err != nil {
			return mapPipelineErr(err)
		}
		return c.JSON(fiber.Map{"data": arts})
	})
	runs.Post("/:id/cancel", mutationLimiter, func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.CancelRun(ctx, c.Params("id")); err != nil {
			return mapPipelineErr(err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})
	runs.Post("/:id/retry", mutationLimiter, func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		run, err := svc.RetryRun(ctx, c.Params("id"), claimSub(c))
		if err != nil {
			return mapPipelineErr(err)
		}
		return c.JSON(fiber.Map{"data": run})
	})
	runs.Post("/:id/stages/:stageId/approve", mutationLimiter, func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.ApproveStage(ctx, c.Params("id"), c.Params("stageId"), claimSub(c)); err != nil {
			return mapPipelineErr(err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})
	runs.Post("/:id/stages/:stageId/reject", mutationLimiter, func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.RejectStage(ctx, c.Params("id"), c.Params("stageId"), claimSub(c)); err != nil {
			return mapPipelineErr(err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// Artifact download by id (binary payload).
	protected.Get("/pipeline-artifacts/:id", requireRole("admin"), func(c *fiber.Ctx) error {
		ctx, cancel := longRequestContext()
		defer cancel()
		art, data, err := svc.ReadArtifact(ctx, c.Params("id"))
		if err != nil {
			return mapPipelineErr(err)
		}
		if art.ContentType != "" {
			c.Set("Content-Type", art.ContentType)
		}
		c.Set("Content-Disposition", "attachment; filename=\""+filepath.Base(art.Name)+"\"")
		return c.Send(data)
	})

	// Public webhook trigger, mounted only when a signing secret is configured.
	if secret := strings.TrimSpace(os.Getenv(pipelineWebhookSecretEnv)); secret != "" {
		v1.Post("/pipelines/webhook/:id", func(c *fiber.Ctx) error {
			body := c.Body()
			sig := c.Get("X-Pipeline-Signature", c.Get("X-Hub-Signature-256"))
			if !verifyPipelineSignature(secret, body, sig) {
				return fiber.NewError(fiber.StatusUnauthorized, "invalid webhook signature")
			}
			ctx, cancel := requestContext()
			defer cancel()
			run, err := svc.TriggerRun(ctx, c.Params("id"), "webhook", "webhook")
			if err != nil {
				return mapPipelineErr(err)
			}
			return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"data": run})
		})
	}
}

// verifyPipelineSignature accepts either a bare hex HMAC-SHA256 of the body or
// the GitHub-style "sha256=<hex>" prefix. Constant-time compare throughout.
func verifyPipelineSignature(secret string, body []byte, provided string) bool {
	provided = strings.TrimSpace(provided)
	if provided == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	provided = strings.TrimPrefix(provided, "sha256=")
	return hmac.Equal([]byte(strings.ToLower(provided)), []byte(expected))
}

func pipelineDataDir() string {
	if dir := strings.TrimSpace(os.Getenv("PIPELINE_DATA_DIR")); dir != "" {
		return dir
	}
	if root := strings.TrimSpace(os.Getenv("DATA_DIR")); root != "" {
		return filepath.Join(root, "pipelines")
	}
	return filepath.Join(os.TempDir(), "forge-pipelines")
}

// claimSub returns the authenticated user's subject id for actor attribution.
func claimSub(c *fiber.Ctx) string {
	if claims, ok := c.Locals("user").(tokenClaims); ok {
		return claims.Sub
	}
	return ""
}

func logPhase(cfg *Config, msg string) {
	if cfg.Logger != nil {
		cfg.Logger.Warn(msg)
	}
}

func mapPipelineErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pipeline.ErrNotFound) {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "not found"):
		return fiber.NewError(fiber.StatusNotFound, msg)
	case strings.Contains(low, "already"), strings.Contains(low, "conflict"):
		return fiber.NewError(fiber.StatusConflict, msg)
	case strings.Contains(low, "required"), strings.Contains(low, "invalid"), strings.Contains(low, "cannot"), strings.Contains(low, "unknown action"):
		return fiber.NewError(fiber.StatusBadRequest, msg)
	}
	return fiber.NewError(fiber.StatusInternalServerError, msg)
}
