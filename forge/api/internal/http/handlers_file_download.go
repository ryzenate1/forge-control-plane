package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"mime"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

type fileDownloadTicket struct {
	serverID string
	filePath string
	expires  time.Time
}

type fileDownloadTicketStore struct {
	mu      sync.Mutex
	tickets map[string]fileDownloadTicket
}

func newFileDownloadTicketStore() *fileDownloadTicketStore {
	return &fileDownloadTicketStore{tickets: make(map[string]fileDownloadTicket)}
}

func (s *fileDownloadTicketStore) issue(ticket fileDownloadTicket) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	s.mu.Lock()
	s.tickets[token] = ticket
	s.mu.Unlock()
	time.AfterFunc(time.Until(ticket.expires)+time.Second, func() {
		s.mu.Lock()
		delete(s.tickets, token)
		s.mu.Unlock()
	})
	return token, nil
}

func (s *fileDownloadTicketStore) consume(token string) (fileDownloadTicket, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ticket, ok := s.tickets[token]
	delete(s.tickets, token)
	if !ok || time.Now().After(ticket.expires) {
		return fileDownloadTicket{}, false
	}
	return ticket, true
}

func issueFileDownloadTicket(cfg Config, tickets *fileDownloadTicketStore) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if err := checkServerPermission(c, cfg, store.PermFileReadContent); err != nil {
			return err
		}
		var req struct {
			Path string `json:"path"`
		}
		if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.Path) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "path is required")
		}
		filePath := strings.TrimSpace(req.Path)
		expires := time.Now().Add(60 * time.Second)
		token, err := tickets.issue(fileDownloadTicket{serverID: c.Params("id"), filePath: filePath, expires: expires})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not create download ticket")
		}
		if cfg.Store != nil {
			ctx, cancel := requestContext()
			defer cancel()
			var actorID *string
			if claims, ok := c.Locals("user").(tokenClaims); ok {
				actorID = &claims.Sub
			}
			serverID := c.Params("id")
			_ = cfg.Store.AppendAudit(ctx, actorID, "server:file.download", "server", &serverID, safeAuditMeta(map[string]string{"path": filePath}))
		}
		return c.JSON(fiber.Map{"token": token, "expiresAt": expires.UTC().Format(time.RFC3339)})
	}
}

func downloadFileWithTicket(cfg Config, tickets *fileDownloadTicketStore) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ticket, ok := tickets.consume(c.Query("token"))
		if !ok {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired download ticket")
		}
		if cfg.Store == nil || cfg.Daemon == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres and daemon are required")
		}
		lookupCtx, lookupCancel := requestContext()
		target, err := cfg.Store.ServerControlTarget(lookupCtx, ticket.serverID)
		lookupCancel()
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "server not found")
		}
		downloadCtx, downloadCancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer downloadCancel()

		if strings.HasPrefix(ticket.filePath, "backup://") {
			backupName := strings.TrimPrefix(ticket.filePath, "backup://")
			// Make sure backup exists in DB
			if _, err := cfg.Store.GetBackupByName(downloadCtx, target.ServerID, backupName); err != nil {
				return fiber.NewError(fiber.StatusNotFound, "backup not found")
			}
			body, err := cfg.Daemon.DownloadBackup(downloadCtx, target.NodeURL, target.NodeToken, target.ServerID, backupName)
			if err != nil {
				return fiber.NewError(fiber.StatusBadGateway, err.Error())
			}
			defer body.Close()
			disposition := mime.FormatMediaType("attachment", map[string]string{"filename": backupName})
			c.Set("Content-Type", "application/zip")
			c.Set("Content-Disposition", disposition)
			c.Set("X-Content-Type-Options", "nosniff")
			c.Set("Referrer-Policy", "no-referrer")
			c.Set("Cache-Control", "private, no-store")
			return c.SendStream(body)
		}

		download, err := cfg.Daemon.DownloadFile(downloadCtx, target.NodeURL, target.NodeToken, target.ServerID, ticket.filePath)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, err.Error())
		}
		defer download.Body.Close()
		disposition := mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(ticket.filePath)})
		c.Set("Content-Type", "application/octet-stream")
		c.Set("Content-Disposition", disposition)
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("Referrer-Policy", "no-referrer")
		c.Set("Cache-Control", "private, no-store")
		if download.Size >= 0 {
			c.Set("Content-Length", strconv.FormatInt(download.Size, 10))
		}
		return c.SendStream(download.Body)
	}
}

func issueBackupDownloadTicket(cfg Config, tickets *fileDownloadTicketStore) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if err := checkServerPermission(c, cfg, store.PermBackupDownload); err != nil {
			return err
		}
		var req struct {
			Name string `json:"name"`
		}
		if err := c.BodyParser(&req); err != nil || strings.TrimSpace(req.Name) == "" {
			return fiber.NewError(fiber.StatusBadRequest, "name is required")
		}
		backupName := strings.TrimSpace(req.Name)
		expires := time.Now().Add(60 * time.Second)
		token, err := tickets.issue(fileDownloadTicket{
			serverID: c.Params("id"),
			filePath: "backup://" + backupName,
			expires:  expires,
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not create download ticket")
		}
		return c.JSON(fiber.Map{"token": token, "expiresAt": expires.UTC().Format(time.RFC3339)})
	}
}
