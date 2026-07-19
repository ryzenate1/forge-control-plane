package http

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path"
	"strconv"
	"strings"
	"time"

	"gamepanel/forge/internal/daemon"
	"gamepanel/forge/internal/services/clustermanager"
	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func registerFilesServerRoutes(protected fiber.Router, cfg Config, runner *scheduleRunner, clusterManager *clustermanager.Service, mutationLimiter fiber.Handler, adminIPAccess fiber.Handler) {

	protected.Get("/servers/:id/files", requireServerPermission(cfg, store.PermFileRead), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		files, err := cfg.Daemon.ListFiles(ctx, target.NodeURL, target.NodeToken, target.ServerID, c.Query("path"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.JSON(files)
	})

	protected.Post("/servers/:id/files/archive", requireServerPermission(cfg, store.PermFileArchive), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		archivePath := c.Query("path")
		if archivePath == "" {
			var req struct {
				Path string `json:"path"`
			}
			if err := c.BodyParser(&req); err == nil && req.Path != "" {
				archivePath = req.Path
			}
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		archiveCtx, archiveCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer archiveCancel()
		body, err := cfg.Daemon.ArchiveFiles(archiveCtx, target.NodeURL, target.NodeToken, target.ServerID, archivePath)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		defer body.Close()
		c.Set("Content-Type", "application/gzip")
		c.Set("Content-Disposition", `attachment; filename="archive.tar.gz"`)
		return c.SendStream(body)
	})

	protected.Post("/servers/:id/files/decompress", requireServerPermission(cfg, store.PermFileArchive), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		var body struct {
			Path string `json:"path"`
		}
		if err := c.BodyParser(&body); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		decompressCtx, decompressCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer decompressCancel()
		if err := cfg.Daemon.DecompressFile(decompressCtx, target.NodeURL, target.NodeToken, target.ServerID, body.Path); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"ok": true})
	})

	protected.Get("/servers/:id/files/content", requireServerPermission(cfg, store.PermFileReadContent), func(c *fiber.Ctx) error {
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		body, err := cfg.Daemon.ReadFile(ctx, target.NodeURL, target.NodeToken, target.ServerID, c.Query("path"))
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.read", "server", &target.ServerID, safeAuditMeta(map[string]string{"file": c.Query("path")}))
		c.Set("Content-Type", "text/plain; charset=utf-8")
		return c.SendString(body)
	})

	protected.Put("/servers/:id/files/content", requireServerPermission(cfg, store.PermFileUpdate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}

		var fileBytes []byte
		contentType := c.Get("Content-Type")
		if strings.HasPrefix(contentType, "multipart/form-data") {
			form, err := c.MultipartForm()
			if err != nil {
				return fiber.NewError(fiber.StatusBadRequest, "failed to parse multipart form")
			}
			// Look for "file" or "files" or "content" fields
			files := form.File["file"]
			if len(files) == 0 {
				files = form.File["content"]
			}
			if len(files) > 0 {
				fileHeader := files[0]
				file, err := fileHeader.Open()
				if err != nil {
					return fiber.NewError(fiber.StatusBadRequest, "failed to open form file")
				}
				defer file.Close()
				fileBytes, err = io.ReadAll(file)
				if err != nil {
					return fiber.NewError(fiber.StatusInternalServerError, "failed to read form file")
				}
			} else {
				// Check for text values in case they sent text in "content" form field
				if vals := form.Value["content"]; len(vals) > 0 {
					fileBytes = []byte(vals[0])
				} else {
					return fiber.NewError(fiber.StatusBadRequest, "no file or content field found in multipart form")
				}
			}
		} else {
			fileBytes = c.Body()
		}

		if err := cfg.Daemon.WriteFile(ctx, target.NodeURL, target.NodeToken, target.ServerID, c.Query("path"), fileBytes); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.write", "server", &target.ServerID, safeAuditMeta(map[string]string{"file": c.Query("path")}))
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Put("/servers/:id/files/upload", requireServerPermission(cfg, store.PermFileCreate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		offset, err := strconv.ParseInt(c.Query("offset", "0"), 10, 64)
		if err != nil || offset < 0 {
			return fiber.NewError(fiber.StatusBadRequest, "invalid offset")
		}
		uploadID := c.Query("uploadId")
		if uploadID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "uploadId is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		body := c.Context().RequestBodyStream()
		if body == nil {
			body = bytes.NewReader(c.Body())
		} else {
			defer c.Context().Request.CloseBodyStream()
		}
		uploadCtx, uploadCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer uploadCancel()
		if err := cfg.Daemon.UploadFileChunk(uploadCtx, target.NodeURL, target.NodeToken, target.ServerID, c.Query("path"), uploadID, offset, c.Query("final") == "true", body); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		if c.Query("final") == "true" {
			var actorID *string
			if claims, ok := c.Locals("user").(tokenClaims); ok {
				actorID = &claims.Sub
			}
			_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.upload", "server", &target.ServerID, safeAuditMeta(map[string]string{"file": c.Query("path")}))
		}
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Delete("/servers/:id/files", requireServerPermission(cfg, store.PermFileDelete), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if err := cfg.Daemon.DeleteFile(ctx, target.NodeURL, target.NodeToken, target.ServerID, c.Query("path")); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.delete", "server", &target.ServerID, safeAuditMeta(map[string]string{"file": c.Query("path")}))
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/files/mkdir", requireServerPermission(cfg, store.PermFileCreate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		mkdirPath := c.Query("path")
		if mkdirPath == "" {
			var req struct {
				Path string `json:"path"`
			}
			if err := c.BodyParser(&req); err == nil && req.Path != "" {
				mkdirPath = req.Path
			}
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if err := cfg.Daemon.MakeDir(ctx, target.NodeURL, target.NodeToken, target.ServerID, mkdirPath); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.create-directory", "server", &target.ServerID, safeAuditMeta(map[string]string{"directory": mkdirPath}))
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true})
	})

	protected.Patch("/servers/:id/files/rename", requireServerPermission(cfg, store.PermFileUpdate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		var req RenameFileRequest
		if err := c.BodyParser(&req); err != nil && err != io.EOF {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if err := cfg.Daemon.RenameFile(ctx, target.NodeURL, target.NodeToken, target.ServerID, req.From, req.To); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.rename", "server", &target.ServerID, safeAuditMeta(map[string]string{"from": req.From, "to": req.To}))
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/files/delete-batch", mutationLimiter, requireServerPermission(cfg, store.PermFileDelete), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		var req struct {
			Paths []string `json:"paths"`
		}
		if err := c.BodyParser(&req); err != nil || len(req.Paths) == 0 || len(req.Paths) > 100 {
			return fiber.NewError(fiber.StatusBadRequest, "between 1 and 100 paths are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		opCtx, opCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer opCancel()
		if err := cfg.Daemon.DeleteFiles(opCtx, target.NodeURL, target.NodeToken, target.ServerID, req.Paths); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.delete-batch", "server", &target.ServerID, safeAuditMeta(map[string]string{"paths": strings.Join(req.Paths, ",")}))
		return c.JSON(fiber.Map{"ok": true, "deleted": len(req.Paths)})
	})

	protected.Post("/servers/:id/files/rename-batch", mutationLimiter, requireServerPermission(cfg, store.PermFileUpdate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		var req struct {
			Files []struct {
				From string `json:"from"`
				To   string `json:"to"`
			} `json:"files"`
		}
		if err := c.BodyParser(&req); err != nil || len(req.Files) == 0 || len(req.Files) > 100 {
			return fiber.NewError(fiber.StatusBadRequest, "between 1 and 100 files are required")
		}
		files := make([]map[string]string, len(req.Files))
		for index, file := range req.Files {
			if strings.TrimSpace(file.From) == "" || strings.TrimSpace(file.To) == "" {
				return fiber.NewError(fiber.StatusBadRequest, "every rename requires from and to")
			}
			files[index] = map[string]string{"from": file.From, "to": file.To}
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		opCtx, opCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer opCancel()
		if err := cfg.Daemon.RenameFiles(opCtx, target.NodeURL, target.NodeToken, target.ServerID, files); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.rename-batch", "server", &target.ServerID, safeAuditMeta(map[string]string{"count": strconv.Itoa(len(files))}))
		return c.JSON(fiber.Map{"ok": true, "renamed": len(files)})
	})

	protected.Post("/servers/:id/files/copy", mutationLimiter, requireServerPermission(cfg, store.PermFileCreate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		var req struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.From) == "" || strings.TrimSpace(req.To) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "from and to are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		copyCtx, copyCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer copyCancel()
		if err := cfg.Daemon.CopyFile(copyCtx, target.NodeURL, target.NodeToken, target.ServerID, req.From, req.To); err != nil {
			status := fiber.StatusBadGateway
			var daemonErr *daemon.ResponseError
			if errors.As(err, &daemonErr) && daemonErr.StatusCode == fiber.StatusConflict {
				status = fiber.StatusConflict
			}
			return fiber.NewError(status, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.copy", "server", &target.ServerID, safeAuditMeta(map[string]string{"from": req.From, "to": req.To}))
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/files/chmod", mutationLimiter, requireServerPermission(cfg, store.PermFileUpdate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		var req struct {
			Path string `json:"path"`
			Mode string `json:"mode"`
		}
		if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.Path) == "" || !validFileMode(req.Mode) {
			return fiber.NewError(fiber.StatusBadRequest, "path and a three or four digit octal mode are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if err := cfg.Daemon.ChmodFile(ctx, target.NodeURL, target.NodeToken, target.ServerID, req.Path, req.Mode); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.chmod", "server", &target.ServerID, safeAuditMeta(map[string]string{"path": req.Path, "mode": req.Mode}))
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Delete("/servers/:id/files/delete", requireServerPermission(cfg, store.PermFileDelete), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if err := cfg.Daemon.DeleteFile(ctx, target.NodeURL, target.NodeToken, target.ServerID, c.Query("path")); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.delete", "server", &target.ServerID, safeAuditMeta(map[string]string{"file": c.Query("path")}))
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/files/rename", requireServerPermission(cfg, store.PermFileUpdate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		var req RenameFileRequest
		if err := c.BodyParser(&req); err != nil && err != io.EOF {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if err := cfg.Daemon.RenameFile(ctx, target.NodeURL, target.NodeToken, target.ServerID, req.From, req.To); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.rename", "server", &target.ServerID, safeAuditMeta(map[string]string{"from": req.From, "to": req.To}))
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/files/chmod-batch", mutationLimiter, requireServerPermission(cfg, store.PermFileUpdate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		var req struct {
			Files []struct {
				Path string `json:"path"`
				Mode string `json:"mode"`
			} `json:"files"`
		}
		if err := c.BodyParser(&req); err != nil || len(req.Files) == 0 || len(req.Files) > 100 {
			return fiber.NewError(fiber.StatusBadRequest, "between 1 and 100 files are required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		for _, f := range req.Files {
			if !validFileMode(f.Mode) {
				return fiber.NewError(fiber.StatusBadRequest, "invalid mode: "+f.Mode)
			}
			if err := cfg.Daemon.ChmodFile(ctx, target.NodeURL, target.NodeToken, target.ServerID, f.Path, f.Mode); err != nil {
				return fiber.NewError(fiber.StatusBadGateway, err.Error())
			}
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.chmod-batch", "server", &target.ServerID, safeAuditMeta(map[string]string{"count": strconv.Itoa(len(req.Files))}))
		return c.JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/files/create-directory", requireServerPermission(cfg, store.PermFileCreate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		var req struct {
			Path string `json:"path"`
		}
		if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.Path) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "path is required")
		}
		ctx, cancel := requestContext()
		defer cancel()
		target, err := cfg.Store.ServerControlTarget(ctx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		if err := cfg.Daemon.MakeDir(ctx, target.NodeURL, target.NodeToken, target.ServerID, req.Path); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.create-directory", "server", &target.ServerID, safeAuditMeta(map[string]string{"directory": req.Path}))
		return c.Status(fiber.StatusCreated).JSON(fiber.Map{"ok": true})
	})

	protected.Post("/servers/:id/files/pull", mutationLimiter, requireServerPermission(cfg, store.PermFileCreate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		var req struct {
			URL  string `json:"url"`
			Path string `json:"path"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		req.URL = strings.TrimSpace(req.URL)
		if req.URL == "" {
			return fiber.NewError(fiber.StatusBadRequest, "url is required")
		}
		destination := strings.TrimSpace(req.Path)
		targetDirectory, fileName := "", ""
		if destination != "" {
			cleaned := path.Clean(destination)
			if cleaned == "." || cleaned == "/" || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "\\") {
				return fiber.NewError(fiber.StatusBadRequest, "invalid destination path")
			}
			targetDirectory = path.Dir(cleaned)
			if targetDirectory == "." {
				targetDirectory = ""
			}
			fileName = path.Base(cleaned)
		}
		lookupCtx, lookupCancel := requestContext()
		defer lookupCancel()
		target, err := cfg.Store.ServerControlTarget(lookupCtx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		pullCtx, pullCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer pullCancel()
		if err := cfg.Daemon.PullRemoteFile(pullCtx, target.NodeURL, target.NodeToken, target.ServerID, req.URL, targetDirectory, fileName); err != nil {
			status := fiber.StatusBadGateway
			var daemonErr *daemon.ResponseError
			if errors.As(err, &daemonErr) {
				switch daemonErr.StatusCode {
				case fiber.StatusBadRequest, fiber.StatusForbidden, fiber.StatusRequestEntityTooLarge, fiber.StatusInsufficientStorage:
					status = daemonErr.StatusCode
				}
			}
			return fiber.NewError(status, "remote file pull failed: "+err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(lookupCtx, actorID, "server:file.pull", "server", &target.ServerID, safeAuditMeta(map[string]string{"url": req.URL, "path": destination}))
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"ok": true, "path": destination})
	})

	// ---- PufferPanel-inspired operations pipeline ----

	protected.Post("/servers/:id/files/download", mutationLimiter, requireServerPermission(cfg, store.PermFileCreate), func(c *fiber.Ctx) error {
		if err := ensureTransferIdle(c, cfg, c.Params("id")); err != nil {
			return err
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		var req struct {
			URL  string `json:"url"`
			Path string `json:"path"`
		}
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		req.URL = strings.TrimSpace(req.URL)
		if req.URL == "" {
			return fiber.NewError(fiber.StatusBadRequest, "url is required")
		}
		destination := strings.TrimSpace(req.Path)
		targetDirectory, fileName := "", ""
		if destination != "" {
			cleaned := path.Clean(destination)
			if cleaned == "." || cleaned == "/" || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "\\") {
				return fiber.NewError(fiber.StatusBadRequest, "invalid destination path")
			}
			targetDirectory = path.Dir(cleaned)
			if targetDirectory == "." {
				targetDirectory = ""
			}
			fileName = path.Base(cleaned)
		}
		lookupCtx, lookupCancel := requestContext()
		defer lookupCancel()
		target, err := cfg.Store.ServerControlTarget(lookupCtx, c.Params("id"))
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		pullCtx, pullCancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer pullCancel()
		if err := cfg.Daemon.PullRemoteFile(pullCtx, target.NodeURL, target.NodeToken, target.ServerID, req.URL, targetDirectory, fileName); err != nil {
			status := fiber.StatusBadGateway
			var daemonErr *daemon.ResponseError
			if errors.As(err, &daemonErr) {
				switch daemonErr.StatusCode {
				case fiber.StatusBadRequest, fiber.StatusForbidden, fiber.StatusRequestEntityTooLarge, fiber.StatusInsufficientStorage:
					status = daemonErr.StatusCode
				}
			}
			return fiber.NewError(status, "remote file pull failed: "+err.Error())
		}
		var actorID *string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			actorID = &claims.Sub
		}
		_ = cfg.Store.AppendAudit(lookupCtx, actorID, "server:file.pull", "server", &target.ServerID, safeAuditMeta(map[string]string{"url": req.URL, "path": destination}))
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"ok": true, "path": destination})
	})
}
