package http

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
)

type SetupStatus struct {
	Required   bool   `json:"required"`
	HasAdmin   bool   `json:"hasAdmin"`
	AppVersion string `json:"appVersion"`
	// PendingSetup flags wizard sections that still hold default values.
	// Omitted when it could not be determined — unknown is not configured.
	PendingSetup *store.SetupPendingState `json:"pendingSetup,omitempty"`
}

// SetupSkippableStep mirrors SetupSkippableStep in
// packages/shared-types/src/api.ts. The administrator step is absent on
// purpose: setup cannot complete without an admin account.
var setupSkippableSteps = map[string]bool{
	"organization": true,
	"node":         true,
	"email":        true,
	"backup":       true,
	"domain":       true,
}

// normalizeSetupSkippedSteps lowercases, trims, dedupes and validates the
// skip list. An unknown step is a 400, never a silent ignore: accepting an
// unknown skip would let a client believe a section was recorded as skipped
// when it was not.
func normalizeSetupSkippedSteps(raw []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, step := range raw {
		normalized := strings.ToLower(strings.TrimSpace(step))
		if normalized == "" {
			continue
		}
		if !setupSkippableSteps[normalized] {
			return nil, fmt.Errorf("unknown setup step %q", step)
		}
		if !seen[normalized] {
			seen[normalized] = true
			out = append(out, normalized)
		}
	}
	return out, nil
}

// applySetupSkips clears the fields of skipped steps so values smuggled in
// alongside a skip are ignored rather than persisted. Skip wins over input.
func applySetupSkips(req *SetupRequest, skipped map[string]bool) {
	if skipped["organization"] {
		req.OrgName = ""
	}
	if skipped["node"] {
		req.NodeName = ""
		req.NodeFqdn = ""
	}
	if skipped["email"] {
		req.SmtpHost = ""
		req.SmtpPort = ""
		req.SmtpUser = ""
		req.SmtpPass = ""
		req.SmtpFrom = ""
		req.SmtpEncryption = ""
	}
	if skipped["backup"] {
		req.BackupDriver = ""
		req.S3Bucket = ""
		req.S3Region = ""
		req.S3Endpoint = ""
	}
	if skipped["domain"] {
		req.DomainName = ""
		req.TlsEmail = ""
	}
}

type SetupRequest struct {
	Email          string `json:"email"`
	Password       string `json:"password"`
	Name           string `json:"name"`
	OrgName        string `json:"orgName"`
	NodeName       string `json:"nodeName"`
	NodeFqdn       string `json:"nodeFqdn"`
	SmtpHost       string `json:"smtpHost"`
	SmtpPort       string `json:"smtpPort"`
	SmtpUser       string `json:"smtpUser"`
	SmtpPass       string `json:"smtpPass"`
	SmtpFrom       string `json:"smtpFrom"`
	SmtpEncryption string `json:"smtpEncryption"`
	BackupDriver   string `json:"backupDriver"`
	S3Bucket       string `json:"s3Bucket"`
	S3Region       string `json:"s3Region"`
	S3Endpoint     string `json:"s3Endpoint"`
	DomainName     string `json:"domainName"`
	TlsEmail       string `json:"tlsEmail"`
	// SkippedSteps records wizard steps the operator explicitly skipped
	// ("organization", "node", "email", "backup", "domain"). Skipped steps'
	// fields are cleared server-side before anything is persisted.
	SkippedSteps []string `json:"skippedSteps"`
}

func registerSetupRoutes(public fiber.Router, cfg Config, authLimiter fiber.Handler) {
	public.Get("/setup/status", func(c *fiber.Ctx) error {
		appVersion := os.Getenv("APP_VERSION")
		if appVersion == "" {
			appVersion = "0.1.0"
		}
		status := SetupStatus{Required: false, HasAdmin: false, AppVersion: appVersion}
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
		if pending, err := cfg.Store.GetSetupPendingState(ctx); err == nil {
			status.PendingSetup = pending
		}
		return c.JSON(status)
	})

	public.Post("/setup", authLimiter, func(c *fiber.Ctx) error {
		if cfg.Store == nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, T(c, "setup.postgres_required", "postgres is required"))
		}
		var req SetupRequest
		if err := c.BodyParser(&req); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, T(c, "setup.invalid_request", "invalid request body"))
		}
		skipped, err := normalizeSetupSkippedSteps(req.SkippedSteps)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, T(c, "setup.invalid_skip_step", "unknown skipped setup step")+": "+err.Error())
		}
		skipSet := map[string]bool{}
		for _, step := range skipped {
			skipSet[step] = true
		}
		applySetupSkips(&req, skipSet)
		if req.Email == "" || req.Password == "" {
			return fiber.NewError(fiber.StatusBadRequest, T(c, "setup.email_password_required", "email and password are required"))
		}
		if req.NodeName != "" && req.NodeFqdn == "" {
			return fiber.NewError(fiber.StatusBadRequest, T(c, "setup.fqdn_required", "nodeFqdn is required when nodeName is set"))
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
			return fiber.NewError(fiber.StatusForbidden, T(c, "setup.already_completed", "setup already completed"))
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), store.BcryptCost())
		if err != nil {
			return respondInternalError(c, err)
		}
		user, err := cfg.Store.CreateSetupAdmin(ctx, req.Email, string(hash))
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}

		if req.OrgName != "" {
			orgReq := store.CreateOrganizationRequest{Name: req.OrgName}
			if _, err := cfg.Store.CreateOrganization(ctx, orgReq, user.ID, &user.ID); err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, T(c, "setup.org_create_failed", "failed to create organization")+": "+err.Error())
			}
		}

		var setupNodeID string
		var setupNodeToken string
		if req.NodeName != "" || req.NodeFqdn != "" {
			nodeName := req.NodeName
			if nodeName == "" {
				nodeName = T(c, "setup.default_node_name", "Primary Node")
			}
			locations, err := cfg.Store.ListLocations(ctx)
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, T(c, "setup.location_list_failed", "failed to list locations")+": "+err.Error())
			}
			var location store.Location
			if len(locations) > 0 {
				location = locations[0]
			} else {
				location, err = cfg.Store.CreateLocation(ctx, store.CreateLocationRequest{
					Short: "default",
					Long:  "Default Location",
				}, &user.ID)
				if err != nil {
					return fiber.NewError(fiber.StatusInternalServerError, T(c, "setup.location_create_failed", "failed to create default location")+": "+err.Error())
				}
			}
			nodeReq := store.CreateNodeRequest{
				Name:         nodeName,
				BaseURL:      "https://" + req.NodeFqdn + ":9090",
				FQDN:         req.NodeFqdn,
				Scheme:       "https",
				Region:       "default",
				LocationID:   location.ID,
				DaemonBase:   "/srv/game-panel/servers",
				DaemonListen: 9090,
				DaemonSFTP:   2022,
				MemoryMB:     4096,
				DiskMB:       51200,
				UploadSizeMB: 100,
			}
			node, nodeToken, err := cfg.Store.CreateNode(ctx, nodeReq, &user.ID)
			if err != nil {
				return fiber.NewError(fiber.StatusInternalServerError, T(c, "setup.node_create_failed", "failed to create node")+": "+err.Error())
			}
			setupNodeID = node.ID
			setupNodeToken = nodeToken
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
				return fiber.NewError(fiber.StatusInternalServerError, T(c, "setup.smtp_save_failed", "failed to save SMTP settings")+": "+err.Error())
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
				return fiber.NewError(fiber.StatusInternalServerError, T(c, "setup.backup_save_failed", "failed to save backup settings")+": "+err.Error())
			}
		}

		token, err := issueConfiguredToken(cfg, user)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "could not issue session token")
		}
		setSessionCookies(c, token, deriveSessionCSRFToken(cfg.AuthSecret, token), tokenExpiry(cfg))
		response := fiber.Map{"ok": true, "userId": user.ID, "email": user.Email, "skippedSteps": skipped}
		if setupNodeID != "" {
			response["nodeId"] = setupNodeID
			response["nodeToken"] = setupNodeToken
		}
		return c.JSON(response)
	})
}
