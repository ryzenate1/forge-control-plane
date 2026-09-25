package http

import (
	"encoding/json"
	"strings"

	backupenginesvc "gamepanel/forge/internal/services/backupengine"

	"github.com/gofiber/fiber/v2"
)

// Admin backup-engine surface: Restic and Kopia repositories registered
// alongside the classic dbprovisioner / dbbackupsvc backup paths. Layered as
// handler -> backupengine.Service -> (beacon admin exec | control-plane host)
// -> restic/kopia CLI.
//
// Reads use the backups.read scope, every mutation (register, init, snapshot,
// verify, prune, restore) uses backups.write, so a scoped API key can inspect
// repositories without being able to prune or restore them. CLI invocations can
// run for minutes, so they use longRequestContext() rather than the 5-second
// request context.

const backupEnginePath = "/admin/backup-engines"

func registerBackupEngineRoutes(protected fiber.Router, cfg Config, svc *backupenginesvc.Service, mutationLimiter fiber.Handler) {
	if svc == nil {
		return
	}

	admin := protected.Group(backupEnginePath, requireRole("admin"))

	admin.Get("/repositories", requireAdminScope("backups.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		repos, err := svc.ListRepositories(ctx)
		if err != nil {
			return respondBackupEngineError(c, err)
		}
		for i := range repos {
			if repos[i].NextRunAt == nil {
				if next := svc.NextRun(repos[i]); next != nil {
					repos[i].NextRunAt = next
				}
			}
		}
		return c.JSON(fiber.Map{"data": repos})
	})

	admin.Post("/repositories", mutationLimiter, requireAdminScope("backups.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			Name        string          `json:"name"`
			Engine      string          `json:"engine"`
			Location    string          `json:"location"`
			Password    string          `json:"password"`
			Encryption  string          `json:"encryption"`
			PrunePolicy json.RawMessage `json:"prunePolicy"`
			NodeID      string          `json:"nodeId"`
			ServerID    string          `json:"serverId"`
			ArtifactID  string          `json:"artifactId"`
			Initialized bool            `json:"initialized"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		repo, err := svc.AddRepository(ctx, backupenginesvc.AddRepositoryRequest{
			Name:        req.Name,
			Engine:      req.Engine,
			Location:    req.Location,
			Password:    req.Password,
			Encryption:  req.Encryption,
			PrunePolicy: req.PrunePolicy,
			NodeID:      req.NodeID,
			ServerID:    req.ServerID,
			ArtifactID:  req.ArtifactID,
			Initialized: req.Initialized,
		})
		if err != nil {
			return respondBackupEngineError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(repo)
	})

	admin.Delete("/repositories/:id", mutationLimiter, requireAdminScope("backups.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		if err := svc.RemoveRepository(ctx, c.Params("id")); err != nil {
			return respondBackupEngineError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	// --- repository connectivity -------------------------------------------------

	testRepository := func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		id := strings.TrimSpace(c.Params("id"))
		if id == "" {
			var body struct {
				ID string `json:"id"`
			}
			if err := c.BodyParser(&body); err == nil {
				id = strings.TrimSpace(body.ID)
			}
		}
		if id == "" {
			return fiber.NewError(fiber.StatusBadRequest, "repository id is required")
		}
		ctx, cancel := longRequestContext()
		defer cancel()
		if err := svc.TestRepository(ctx, id); err != nil {
			return respondBackupEngineError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true, "id": id, "reachable": true})
	}
	admin.Post("/repositories/:id/test", mutationLimiter, requireAdminScope("backups.write"), testRepository)
	admin.Post("/test", mutationLimiter, requireAdminScope("backups.write"), testRepository)

	admin.Post("/repositories/:id/init", mutationLimiter, requireAdminScope("backups.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := longRequestContext()
		defer cancel()
		if err := svc.InitRepository(ctx, c.Params("id")); err != nil {
			return respondBackupEngineError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true, "initialized": true})
	})

	// --- snapshots -----------------------------------------------------------------

	listSnapshots := func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		repoID := strings.TrimSpace(c.Params("id"))
		if repoID == "" {
			repoID = strings.TrimSpace(c.Query("repoId"))
		}
		if repoID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "repoId is required")
		}
		ctx, cancel := longRequestContext()
		defer cancel()
		snapshots, err := svc.ListSnapshots(ctx, repoID)
		if err != nil {
			return respondBackupEngineError(c, err)
		}
		return c.JSON(fiber.Map{"data": snapshots, "repoId": repoID})
	}
	admin.Get("/repositories/:id/snapshots", requireAdminScope("backups.read"), listSnapshots)
	admin.Get("/snapshots", requireAdminScope("backups.read"), listSnapshots)

	admin.Post("/snapshots", mutationLimiter, requireAdminScope("backups.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			RepoID string   `json:"repoId"`
			Paths  []string `json:"paths"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.RepoID) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "repoId is required")
		}
		ctx, cancel := longRequestContext()
		defer cancel()
		snapshot, err := svc.CreateSnapshot(ctx, req.RepoID, req.Paths)
		if err != nil {
			return respondBackupEngineError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(snapshot)
	})

	admin.Post("/snapshots/verify", mutationLimiter, requireAdminScope("backups.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			RepoID     string `json:"repoId"`
			SnapshotID string `json:"snapshotId"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.RepoID) == "" || strings.TrimSpace(req.SnapshotID) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "repoId and snapshotId are required")
		}
		ctx, cancel := longRequestContext()
		defer cancel()
		if err := svc.VerifySnapshot(ctx, req.RepoID, req.SnapshotID); err != nil {
			return respondBackupEngineError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true, "repoId": req.RepoID, "snapshotId": req.SnapshotID, "verified": true})
	})

	// --- prune ---------------------------------------------------------------------

	admin.Post("/prune", mutationLimiter, requireAdminScope("backups.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			RepoID string `json:"repoId"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.RepoID) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "repoId is required")
		}
		ctx, cancel := longRequestContext()
		defer cancel()
		if err := svc.PruneRepository(ctx, req.RepoID); err != nil {
			return respondBackupEngineError(c, err)
		}
		return c.JSON(fiber.Map{"ok": true, "repoId": req.RepoID})
	})

	// --- restores ------------------------------------------------------------------

	admin.Post("/restore", mutationLimiter, requireAdminScope("backups.write"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req struct {
			RepoID     string `json:"repoId"`
			SnapshotID string `json:"snapshotId"`
			TargetPath string `json:"targetPath"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if strings.TrimSpace(req.RepoID) == "" || strings.TrimSpace(req.SnapshotID) == "" || strings.TrimSpace(req.TargetPath) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "repoId, snapshotId and targetPath are required")
		}
		ctx, cancel := longRequestContext()
		defer cancel()
		job, err := svc.RestoreSnapshot(ctx, req.RepoID, req.SnapshotID, req.TargetPath)
		if err != nil {
			return respondBackupEngineError(c, err)
		}
		return c.Status(fiber.StatusCreated).JSON(job)
	})

	admin.Get("/restore", requireAdminScope("backups.read"), func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		jobs, err := svc.ListRestoreJobs(ctx, c.Query("repoId"))
		if err != nil {
			return respondBackupEngineError(c, err)
		}
		return c.JSON(fiber.Map{"data": jobs})
	})
}

// respondBackupEngineError maps service errors to HTTP status codes. Input
// validation failures from the service are reported as 400 so the UI can show
// them inline; everything else (CLI unreachable, repository encrypted with a
// wrong password, node offline) goes out as a 500 carrying the CLI's message,
// which the service already redacts of secret material.
func respondBackupEngineError(c *fiber.Ctx, err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if isBackupEngineInputError(message) {
		return fiber.NewError(fiber.StatusBadRequest, message)
	}
	return fiber.NewError(fiber.StatusInternalServerError, message)
}

func isBackupEngineInputError(message string) bool {
	lowered := strings.ToLower(message)
	for _, marker := range []string{"is required", "unsupported", "invalid", "must be", "at least one", "not been initialised"} {
		if strings.Contains(lowered, marker) {
			return true
		}
	}
	return false
}
