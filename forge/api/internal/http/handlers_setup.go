package http

import (
	"strconv"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
)

type SetupStatus struct {
	Required   bool   `json:"required"`
	HasAdmin   bool   `json:"hasAdmin"`
	AppVersion string `json:"appVersion"`
}

type SetupRequest struct {
	Email           string `json:"email"`
	Password        string `json:"password"`
	Name            string `json:"name"`
	OrgName         string `json:"orgName"`
	NodeName        string `json:"nodeName"`
	NodeFqdn        string `json:"nodeFqdn"`
	SmtpHost        string `json:"smtpHost"`
	SmtpPort        string `json:"smtpPort"`
	SmtpUser        string `json:"smtpUser"`
	SmtpPass        string `json:"smtpPass"`
	SmtpFrom        string `json:"smtpFrom"`
	SmtpEncryption  string `json:"smtpEncryption"`
	BackupDriver    string `json:"backupDriver"`
	S3Bucket        string `json:"s3Bucket"`
	S3Region        string `json:"s3Region"`
	S3Endpoint      string `json:"s3Endpoint"`
	DomainName      string `json:"domainName"`
	TlsEmail        string `json:"tlsEmail"`
}

func registerSetupRoutes(public fiber.Router, cfg Config, authLimiter fiber.Handler) {
	public.Get("/setup/status", func(c *fiber.Ctx) error {
		status := SetupStatus{Required: false, HasAdmin: false, AppVersion: "0.1.0"}
		if cfg.Store == nil {
			return c.JSON(status)
		}
		ctx, cancel := requestContext()
		defer cancel()
		has, err := cfg.Store.HasAnyAdmin(ctx)
		if err != nil {
			return c.JSON(status)
		}
		status.HasAdmin = has
		status.Required = !has
		return c.JSON(status)
	})

	public.Post("/setup", authLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "postgres is required")
		}
		var req SetupRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		if req.Email == "" || req.Password == "" {
			return fiber.NewError(fiber.StatusBadRequest, "email and password are required")
		}
		if err := store.ValidatePassword(req.Password); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		ctx, cancel := requestContext()
		defer cancel()
		has, err := cfg.Store.HasAnyAdmin(ctx)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if has {
			return fiber.NewError(fiber.StatusForbidden, "setup already completed")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		user, err := cfg.Store.CreateSetupAdmin(ctx, req.Email, string(hash))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}

		if req.OrgName != "" {
			orgReq := store.CreateOrganizationRequest{Name: req.OrgName}
			if _, err := cfg.Store.CreateOrganization(ctx, orgReq, user.ID, &user.ID); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "failed to create organization: "+err.Error())
			}
		}

		if req.NodeName != "" || req.NodeFqdn != "" {
			nodeName := req.NodeName
			if nodeName == "" {
				nodeName = "Primary Node"
			}
			nodeFqdn := req.NodeFqdn
			if nodeFqdn == "" {
				nodeFqdn = "node1.example.com"
			}
			nodeReq := store.CreateNodeRequest{
				Name:        nodeName,
				BaseURL:     "http://" + nodeFqdn + ":9090",
				FQDN:        nodeFqdn,
				Scheme:      "http",
				Region:      "default",
				DaemonBase:  "/srv/game-panel/servers",
				DaemonListen: 9090,
				DaemonSFTP:   2022,
				MemoryMB:     4096,
				DiskMB:       51200,
				UploadSizeMB: 100,
			}
			if _, _, err := cfg.Store.CreateNode(ctx, nodeReq, &user.ID); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "failed to create node: "+err.Error())
			}
		}

		if req.SmtpHost != "" {
			port, _ := strconv.Atoi(req.SmtpPort)
			if port <= 0 {
				port = 587
			}
			mailSettings := store.PanelMailSettings{
				SMTPHost:        req.SmtpHost,
				SMTPPort:        port,
				SMTPEncryption:  req.SmtpEncryption,
				SMTPUsername:    req.SmtpUser,
				SMTPPassword:    req.SmtpPass,
				MailFromAddress: req.SmtpFrom,
				MailFromName:    "GamePanel",
			}
			if mailSettings.MailFromAddress == "" {
				mailSettings.MailFromAddress = req.Email
			}
			if err := cfg.Store.UpdatePanelMailSettings(ctx, mailSettings); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "failed to save SMTP settings: "+err.Error())
			}
		}

		if req.BackupDriver != "" {
			settings, _ := cfg.Store.GetPanelSettings(ctx)
			settings.BackupProvider = req.BackupDriver
			if req.BackupDriver == "s3" {
				settings.S3BackupEnabled = true
				settings.S3Bucket = req.S3Bucket
				settings.S3Region = req.S3Region
				settings.S3Endpoint = req.S3Endpoint
			}
			if err := cfg.Store.UpdatePanelSettings(ctx, settings); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, "failed to save backup settings: "+err.Error())
			}
		}

		return c.JSON(fiber.Map{"ok": true, "userId": user.ID, "email": user.Email})
	})
}
