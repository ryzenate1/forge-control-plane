package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	// userID and ip bind the ticket to the issuer so a leaked URL cannot be
	// replayed from another account or network.
	userID string
	ip     string
}

// fileDownloadTicketDTO is the Redis-serializable form of fileDownloadTicket
// (the in-memory struct keeps unexported fields).
type fileDownloadTicketDTO struct {
	ServerID string `json:"serverId"`
	FilePath string `json:"filePath"`
	Expires  int64  `json:"expires"`
	UserID   string `json:"userId"`
	IP       string `json:"ip"`
}

func fileDownloadTicketKey(token string) string {
	return "forge:file-download:" + token
}

type fileDownloadTicketStore struct {
	mu      sync.Mutex
	tickets map[string]fileDownloadTicket
	cfg     Config
}

func newFileDownloadTicketStore(cfg Config) *fileDownloadTicketStore {
	return &fileDownloadTicketStore{tickets: make(map[string]fileDownloadTicket), cfg: cfg}
}

func (s *fileDownloadTicketStore) useShared() bool {
	return s.cfg.RedisEnabled && s.cfg.Redis != nil
}

func (s *fileDownloadTicketStore) issue(ticket fileDownloadTicket) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	// Shared, single-use storage when Redis is available so a ticket minted
	// on one API instance redeems on any other behind the load balancer.
	// Without Redis the in-memory fallback keeps single-instance deployments
	// working; on multi-instance setups without Redis a ticket redeemed on a
	// different instance is rejected (fail closed — availability, not a
	// bypass, since redemption still requires the ticket bearer plus the IP
	// binding below).
	if s.useShared() {
		dto := fileDownloadTicketDTO{
			ServerID: ticket.serverID,
			FilePath: ticket.filePath,
			Expires:  ticket.expires.Unix(),
			UserID:   ticket.userID,
			IP:       ticket.ip,
		}
		data, err := json.Marshal(dto)
		if err != nil {
			return "", err
		}
		ttl := time.Until(ticket.expires)
		if ttl <= 0 {
			return "", errors.New("download ticket already expired")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := s.cfg.Redis.Set(ctx, fileDownloadTicketKey(token), data, ttl).Err(); err != nil {
			return "", err
		}
		return token, nil
	}
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
	// GETDEL makes redemption atomic and single-use across instances: the
	// first redeem wins, every later one finds nothing.
	if s.useShared() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		data, err := s.cfg.Redis.GetDel(ctx, fileDownloadTicketKey(token)).Bytes()
		if err != nil {
			return fileDownloadTicket{}, false
		}
		var dto fileDownloadTicketDTO
		if err := json.Unmarshal(data, &dto); err != nil {
			return fileDownloadTicket{}, false
		}
		ticket := fileDownloadTicket{
			serverID: dto.ServerID,
			filePath: dto.FilePath,
			expires:  time.Unix(dto.Expires, 0),
			userID:   dto.UserID,
			ip:       dto.IP,
		}
		if dto.ServerID == "" || dto.FilePath == "" || time.Now().After(ticket.expires) {
			return fileDownloadTicket{}, false
		}
		return ticket, true
	}
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
		var issuerID string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			issuerID = claims.Sub
		}
		token, err := tickets.issue(fileDownloadTicket{serverID: c.Params("id"), filePath: filePath, expires: expires, userID: issuerID, ip: ExtractClientIP(c)})
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
		// Bind to issuer: a ticket minted for one user/IP must not serve
		// another. The redeem route is public by design (the ticket bearer
		// IS the credential), so the binding is enforced manually here:
		// when the ticket carries a userID the redeeming request must
		// present the same session identity via cookie or Bearer JWT,
		// validated against current revocation state. Fail closed on
		// missing/mismatched identity — never downgrade to anonymous.
		if ticket.ip != "" && ticket.ip != ExtractClientIP(c) {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired download ticket")
		}
		if ticket.userID != "" {
			sessionToken := ""
			if tok, hasCookie := getSessionCookie(c); hasCookie && tok != "" {
				sessionToken = tok
			} else if header := c.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
				sessionToken = strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
			}
			if sessionToken == "" || cfg.Store == nil {
				return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired download ticket")
			}
			claims, err := parseToken(cfg.AuthSecret, sessionToken)
			if err != nil {
				return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired download ticket")
			}
			lookupCtx, lookupCancel := requestContext()
			validated, err := validateCurrentSession(lookupCtx, cfg.Store, claims)
			lookupCancel()
			if err != nil || validated.Sub == "" || validated.Sub != ticket.userID {
				return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired download ticket")
			}
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
		if backupName == "." || backupName == ".." || path.Base(backupName) != backupName || strings.ContainsRune(backupName, '\x00') {
			return fiber.NewError(fiber.StatusBadRequest, "invalid backup name")
		}
		expires := time.Now().Add(60 * time.Second)
		var issuerID string
		if claims, ok := c.Locals("user").(tokenClaims); ok {
			issuerID = claims.Sub
		}
		token, err := tickets.issue(fileDownloadTicket{
			serverID: c.Params("id"),
			filePath: "backup://" + backupName,
			expires:  expires,
			userID:   issuerID,
			ip:       ExtractClientIP(c),
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not create download ticket")
		}
		return c.JSON(fiber.Map{"token": token, "expiresAt": expires.UTC().Format(time.RFC3339)})
	}
}
