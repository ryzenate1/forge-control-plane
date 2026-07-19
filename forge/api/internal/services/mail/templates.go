package mail

import (
	"bytes"
	"html/template"
	"strings"
	"time"
)

type EmailTemplate string

const (
	TemplatePasswordReset     EmailTemplate = "password_reset"
	TemplateWelcome           EmailTemplate = "welcome"
	TemplateAccountCreated    EmailTemplate = "account_created"
	TemplateServerCreated     EmailTemplate = "server_created"
	TemplateServerSuspended   EmailTemplate = "server_suspended"
	TemplateServerUnsuspended EmailTemplate = "server_unsuspended"
	TemplateBackupComplete    EmailTemplate = "backup_complete"
	TemplateBackupFailed      EmailTemplate = "backup_failed"
	Template2FAEnabled        EmailTemplate = "2fa_enabled"
	Template2FADisabled       EmailTemplate = "2fa_disabled"
	TemplatePasswordChanged   EmailTemplate = "password_changed"
	TemplateEmailChanged      EmailTemplate = "email_changed"
	TemplateInvitation        EmailTemplate = "invitation"
	TemplateSubuserInvited    EmailTemplate = "subuser_invited"
	TemplateSubuserRemoved    EmailTemplate = "subuser_removed"
	TemplateInstallComplete   EmailTemplate = "install_complete"
	TemplateMaintenance       EmailTemplate = "maintenance"
	TemplateLowResources      EmailTemplate = "low_resources"
	TemplateNodeOffline       EmailTemplate = "node_offline"
)

type EmailData struct {
	RecipientName  string
	RecipientEmail string
	PanelURL       string
	CompanyName    string
	ProductName    string
	CurrentYear    int
	ResetURL       string
	ServerName     string
	ServerID       string
	NodeName       string
	InviterName    string
	InviteURL      string
	BackupName     string
	BackupSize     string
	Reason         string
	ResourceType   string
	UsagePercent   string
	ActorName      string
	ActorEmail     string
	NewEmail       string
	NewPassword    string
	FailureReason  string
}

var templateMap = map[EmailTemplate]string{
	TemplatePasswordReset: `<!DOCTYPE html>
<html><head><meta charset="utf-8"><style>
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;margin:0;padding:0;background:#f6f9fc}
.container{max-width:600px;margin:0 auto;padding:40px 20px}
.header{text-align:center;padding:20px 0}
.header h1{color:#1a1a2e;margin:0;font-size:24px}
.content{background:#ffffff;border-radius:8px;padding:32px;box-shadow:0 2px 8px rgba(0,0,0,0.08)}
.button{display:inline-block;padding:12px 24px;background:#4f46e5;color:#ffffff;text-decoration:none;border-radius:6px;font-weight:600}
.footer{text-align:center;padding:20px;color:#6b7280;font-size:12px}
</style></head><body>
<div class="container">
<div class="header"><h1>{{.ProductName}}</h1></div>
<div class="content">
<h2>Password Reset Request</h2>
<p>Hello{{if .RecipientName}} {{.RecipientName}}{{end}},</p>
<p>We received a request to reset the password for your {{.ProductName}} account associated with <strong>{{.RecipientEmail}}</strong>.</p>
<p style="text-align:center;margin:32px 0"><a href="{{.ResetURL}}" class="button">Reset Password</a></p>
<p>This link will expire in 30 minutes. If you did not request a password reset, please ignore this email.</p>
<hr style="border:none;border-top:1px solid #e5e7eb;margin:24px 0">
<p style="color:#6b7280;font-size:14px">If the button above does not work, copy and paste this URL into your browser:</p>
<p style="color:#4f46e5;font-size:12px;word-break:break-all">{{.ResetURL}}</p>
</div>
<div class="footer">
<p>&copy; {{.CurrentYear}} {{.CompanyName}}. All rights reserved.</p>
<p>{{.ProductName}} &mdash; Game Server Management Panel</p>
</div>
</div></body></html>`,

	TemplateWelcome: `<!DOCTYPE html>
<html><head><meta charset="utf-8"><style>
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;margin:0;padding:0;background:#f6f9fc}
.container{max-width:600px;margin:0 auto;padding:40px 20px}
.header{text-align:center;padding:20px 0}
.header h1{color:#1a1a2e;margin:0;font-size:24px}
.content{background:#ffffff;border-radius:8px;padding:32px;box-shadow:0 2px 8px rgba(0,0,0,0.08)}
.button{display:inline-block;padding:12px 24px;background:#4f46e5;color:#ffffff;text-decoration:none;border-radius:6px;font-weight:600}
.footer{text-align:center;padding:20px;color:#6b7280;font-size:12px}
</style></head><body>
<div class="container">
<div class="header"><h1>Welcome to {{.ProductName}}</h1></div>
<div class="content">
<h2>Account Created</h2>
<p>Hello{{if .RecipientName}} {{.RecipientName}}{{end}},</p>
<p>Your {{.ProductName}} account has been created successfully.</p>
{{if .NewPassword}}<p>Your temporary password is: <strong>{{.NewPassword}}</strong></p>
<p style="color:#dc2626">Please change your password after logging in.</p>{{end}}
<p style="text-align:center;margin:32px 0"><a href="{{.PanelURL}}" class="button">Go to Panel</a></p>
</div>
<div class="footer">
<p>&copy; {{.CurrentYear}} {{.CompanyName}}. All rights reserved.</p>
</div>
</div></body></html>`,

	TemplateServerCreated: `<!DOCTYPE html>
<html><head><meta charset="utf-8"><style>
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;margin:0;padding:0;background:#f6f9fc}
.container{max-width:600px;margin:0 auto;padding:40px 20px}
.content{background:#ffffff;border-radius:8px;padding:32px;box-shadow:0 2px 8px rgba(0,0,0,0.08)}
.button{display:inline-block;padding:12px 24px;background:#4f46e5;color:#ffffff;text-decoration:none;border-radius:6px;font-weight:600}
.footer{text-align:center;padding:20px;color:#6b7280;font-size:12px}
</style></head><body>
<div class="container">
<div class="content">
<h2>Server Created</h2>
<p>Hello{{if .RecipientName}} {{.RecipientName}}{{end}},</p>
<p>Your new server <strong>{{.ServerName}}</strong> has been created and is being installed.</p>
<p>You will be able to manage your server once the installation completes.</p>
<p style="text-align:center;margin:32px 0"><a href="{{.PanelURL}}/server/{{.ServerID}}" class="button">Manage Server</a></p>
</div>
<div class="footer"><p>&copy; {{.CurrentYear}} {{.CompanyName}}.</p></div>
</div></body></html>`,

	TemplateServerSuspended: `<html><body><div><h2>Server Suspended</h2><p>Your server <strong>{{.ServerName}}</strong> has been suspended.</p>{{if .Reason}}<p>Reason: {{.Reason}}</p>{{end}}<p>Please contact support for more information.</p></div></body></html>`,

	TemplateServerUnsuspended: `<html><body><div><h2>Server Unsuspended</h2><p>Your server <strong>{{.ServerName}}</strong> has been unsuspended and is now active.</p></div></body></html>`,

	TemplateBackupComplete: `<!DOCTYPE html>
<html><head><meta charset="utf-8"><style>
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;margin:0;padding:0;background:#f6f9fc}
.container{max-width:600px;margin:0 auto;padding:40px 20px}
.header{text-align:center;padding:20px 0}
.header h1{color:#1a1a2e;margin:0;font-size:24px}
.content{background:#ffffff;border-radius:8px;padding:32px;box-shadow:0 2px 8px rgba(0,0,0,0.08)}
.footer{text-align:center;padding:20px;color:#6b7280;font-size:12px}
</style></head><body>
<div class="container">
<div class="header"><h1>{{.ProductName}}</h1></div>
<div class="content">
<h2>Backup Complete</h2>
<p>Hello{{if .RecipientName}} {{.RecipientName}}{{end}},</p>
<p>Backup <strong>{{.BackupName}}</strong> for server <strong>{{.ServerName}}</strong> has completed successfully.</p>
{{if .BackupSize}}<p>Size: <strong>{{.BackupSize}}</strong></p>{{end}}
</div>
<div class="footer"><p>&copy; {{.CurrentYear}} {{.CompanyName}}.</p></div>
</div></body></html>`,

	TemplateBackupFailed: `<!DOCTYPE html>
<html><head><meta charset="utf-8"><style>
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;margin:0;padding:0;background:#f6f9fc}
.container{max-width:600px;margin:0 auto;padding:40px 20px}
.header{text-align:center;padding:20px 0}
.header h1{color:#1a1a2e;margin:0;font-size:24px}
.content{background:#ffffff;border-radius:8px;padding:32px;box-shadow:0 2px 8px rgba(0,0,0,0.08)}
.footer{text-align:center;padding:20px;color:#6b7280;font-size:12px}
</style></head><body>
<div class="container">
<div class="header"><h1>{{.ProductName}}</h1></div>
<div class="content">
<h2>Backup Failed</h2>
<p>Hello{{if .RecipientName}} {{.RecipientName}}{{end}},</p>
<p>Backup <strong>{{.BackupName}}</strong> for server <strong>{{.ServerName}}</strong> has failed.</p>
{{if .FailureReason}}<p>Reason: {{.FailureReason}}</p>{{end}}
<p>Please check the panel for more details and try again if needed.</p>
</div>
<div class="footer"><p>&copy; {{.CurrentYear}} {{.CompanyName}}.</p></div>
</div></body></html>`,

	TemplatePasswordChanged: `<html><body><div><h2>Password Changed</h2><p>Your {{.ProductName}} account password was successfully changed.</p><p>If you did not make this change, please contact support immediately.</p></div></body></html>`,

	Template2FAEnabled: `<html><body><div><h2>Two-Factor Authentication Enabled</h2><p>Two-factor authentication has been enabled on your account.</p><p>If you did not enable this, please contact support immediately.</p></div></body></html>`,

	Template2FADisabled: `<html><body><div><h2>Two-Factor Authentication Disabled</h2><p>Two-factor authentication has been disabled on your account.</p></div></body></html>`,

	TemplateInvitation: `<html><body><div><h2>Invitation</h2><p>{{.InviterName}} has invited you to join {{.ProductName}}.</p><p style="text-align:center;margin:32px 0"><a href="{{.InviteURL}}" style="display:inline-block;padding:12px 24px;background:#4f46e5;color:white;text-decoration:none;border-radius:6px">Accept Invitation</a></p></div></body></html>`,

	TemplateSubuserInvited: `<!DOCTYPE html>
<html><head><meta charset="utf-8"><style>
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;margin:0;padding:0;background:#f6f9fc}
.container{max-width:600px;margin:0 auto;padding:40px 20px}
.header{text-align:center;padding:20px 0}
.header h1{color:#1a1a2e;margin:0;font-size:24px}
.content{background:#ffffff;border-radius:8px;padding:32px;box-shadow:0 2px 8px rgba(0,0,0,0.08)}
.button{display:inline-block;padding:12px 24px;background:#4f46e5;color:#ffffff;text-decoration:none;border-radius:6px;font-weight:600}
.footer{text-align:center;padding:20px;color:#6b7280;font-size:12px}
</style></head><body>
<div class="container">
<div class="header"><h1>{{.ProductName}}</h1></div>
<div class="content">
<h2>Server Access Granted</h2>
<p>Hello{{if .RecipientName}} {{.RecipientName}}{{end}},</p>
<p>You have been added as a subuser to server <strong>{{.ServerName}}</strong>.</p>
{{if .ActorName}}<p>This action was performed by <strong>{{.ActorName}}</strong>.</p>{{end}}
<p style="text-align:center;margin:32px 0"><a href="{{.PanelURL}}/server/{{.ServerID}}" class="button">View Server</a></p>
</div>
<div class="footer"><p>&copy; {{.CurrentYear}} {{.CompanyName}}.</p></div>
</div></body></html>`,

	TemplateSubuserRemoved: `<!DOCTYPE html>
<html><head><meta charset="utf-8"><style>
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;margin:0;padding:0;background:#f6f9fc}
.container{max-width:600px;margin:0 auto;padding:40px 20px}
.header{text-align:center;padding:20px 0}
.header h1{color:#1a1a2e;margin:0;font-size:24px}
.content{background:#ffffff;border-radius:8px;padding:32px;box-shadow:0 2px 8px rgba(0,0,0,0.08)}
.footer{text-align:center;padding:20px;color:#6b7280;font-size:12px}
</style></head><body>
<div class="container">
<div class="header"><h1>{{.ProductName}}</h1></div>
<div class="content">
<h2>Server Access Revoked</h2>
<p>Hello{{if .RecipientName}} {{.RecipientName}}{{end}},</p>
<p>Your access to server <strong>{{.ServerName}}</strong> has been removed.</p>
{{if .ActorName}}<p>This action was performed by <strong>{{.ActorName}}</strong>.</p>{{end}}
<p>If you believe this was a mistake, please contact the server owner or an administrator.</p>
</div>
<div class="footer"><p>&copy; {{.CurrentYear}} {{.CompanyName}}.</p></div>
</div></body></html>`,

	TemplateInstallComplete: `<!DOCTYPE html>
<html><head><meta charset="utf-8"><style>
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;margin:0;padding:0;background:#f6f9fc}
.container{max-width:600px;margin:0 auto;padding:40px 20px}
.header{text-align:center;padding:20px 0}
.header h1{color:#1a1a2e;margin:0;font-size:24px}
.content{background:#ffffff;border-radius:8px;padding:32px;box-shadow:0 2px 8px rgba(0,0,0,0.08)}
.button{display:inline-block;padding:12px 24px;background:#4f46e5;color:#ffffff;text-decoration:none;border-radius:6px;font-weight:600}
.footer{text-align:center;padding:20px;color:#6b7280;font-size:12px}
</style></head><body>
<div class="container">
<div class="header"><h1>{{.ProductName}}</h1></div>
<div class="content">
<h2>Server Installation Complete</h2>
<p>Hello{{if .RecipientName}} {{.RecipientName}}{{end}},</p>
<p>Your server <strong>{{.ServerName}}</strong> has finished installing and is now ready to use.</p>
<p style="text-align:center;margin:32px 0"><a href="{{.PanelURL}}/server/{{.ServerID}}" class="button">Manage Server</a></p>
</div>
<div class="footer"><p>&copy; {{.CurrentYear}} {{.CompanyName}}.</p></div>
</div></body></html>`,

	TemplateMaintenance: `<html><body><div><h2>Scheduled Maintenance</h2><p>{{.ProductName}} will undergo scheduled maintenance.</p>{{if .Reason}}<p>{{.Reason}}</p>{{end}}<p>During this time, some services may be unavailable.</p></div></body></html>`,

	TemplateLowResources: `<html><body><div><h2>Resource Alert</h2><p>Your server <strong>{{.ServerName}}</strong> is running low on {{.ResourceType}}.</p><p>Current usage: {{.UsagePercent}}%</p><p>Consider upgrading your plan or optimizing resource usage.</p></div></body></html>`,

	TemplateNodeOffline: `<html><body><div><h2>Node Offline Alert</h2><p>Node <strong>{{.NodeName}}</strong> has gone offline.</p><p>Servers hosted on this node may be unavailable until the node is back online.</p></div></body></html>`,

	TemplateEmailChanged: `<html><body><div><h2>Email Address Changed</h2><p>Your {{.ProductName}} account email has been changed to <strong>{{.NewEmail}}</strong>.</p><p>If you did not make this change, please contact support immediately.</p></div></body></html>`,
}

type TemplateRenderer struct {
	templates map[EmailTemplate]*template.Template
}

func NewTemplateRenderer() *TemplateRenderer {
	tr := &TemplateRenderer{
		templates: make(map[EmailTemplate]*template.Template),
	}
	for name, tmplStr := range templateMap {
		tr.templates[name] = template.Must(template.New(string(name)).Parse(tmplStr))
	}
	return tr
}

func (tr *TemplateRenderer) Render(t EmailTemplate, data EmailData) (string, string, error) {
	tmpl, ok := tr.templates[t]
	if !ok {
		return "", "", nil
	}

	data.CurrentYear = time.Now().Year()
	if data.CompanyName == "" {
		data.CompanyName = "GamePanel"
	}
	if data.ProductName == "" {
		data.ProductName = "GamePanel"
	}

	var htmlBuf bytes.Buffer
	if err := tmpl.Execute(&htmlBuf, data); err != nil {
		return "", "", err
	}

	text := stripHTML(htmlBuf.String())

	return text, htmlBuf.String(), nil
}

func stripHTML(html string) string {
	var buf bytes.Buffer
	inTag := false
	for _, r := range html {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			buf.WriteRune(r)
		}
	}
	result := strings.TrimSpace(buf.String())
	lines := strings.Split(result, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return strings.Join(cleaned, "\n")
}
