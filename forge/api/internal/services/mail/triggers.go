package mail

import (
	"context"
	"encoding/json"
	"fmt"
)

type EmailTrigger interface {
	Name() string
	ShouldSend(data map[string]any) bool
	BuildData(data map[string]any) (EmailData, error)
}

type TriggerConfig struct {
	Enabled bool            `json:"enabled"`
	Events  []string        `json:"events"`
	Config  json.RawMessage `json:"config,omitempty"`
}

type TriggerService struct {
	renderer    *TemplateRenderer
	worker      *Worker
	panelURL    string
	companyName string
	productName string
}

func (ts *TriggerService) Worker() *Worker {
	return ts.worker
}

func NewTriggerService(renderer *TemplateRenderer, worker *Worker, panelURL, companyName, productName string) *TriggerService {
	return &TriggerService{
		renderer:    renderer,
		worker:      worker,
		panelURL:    panelURL,
		companyName: companyName,
		productName: productName,
	}
}

func (ts *TriggerService) SendPasswordReset(ctx context.Context, email, resetURL, recipientName string) error {
	text, html, err := ts.renderer.Render(TemplatePasswordReset, EmailData{
		RecipientName:  recipientName,
		RecipientEmail: email,
		PanelURL:       ts.panelURL,
		CompanyName:    ts.companyName,
		ProductName:    ts.productName,
		ResetURL:       resetURL,
	})
	if err != nil {
		return err
	}
	return ts.worker.Enqueue(ctx, email, "Password Reset Request", text, html)
}

func (ts *TriggerService) SendWelcome(ctx context.Context, email, recipientName, password string) error {
	text, html, err := ts.renderer.Render(TemplateWelcome, EmailData{
		RecipientName:  recipientName,
		RecipientEmail: email,
		PanelURL:       ts.panelURL,
		CompanyName:    ts.companyName,
		ProductName:    ts.productName,
		NewPassword:    password,
	})
	if err != nil {
		return err
	}
	return ts.worker.Enqueue(ctx, email, "Welcome to "+ts.productName, text, html)
}

func (ts *TriggerService) SendServerCreated(ctx context.Context, email, recipientName, serverName, serverID string) error {
	text, html, err := ts.renderer.Render(TemplateServerCreated, EmailData{
		RecipientName:  recipientName,
		RecipientEmail: email,
		PanelURL:       ts.panelURL,
		CompanyName:    ts.companyName,
		ProductName:    ts.productName,
		ServerName:     serverName,
		ServerID:       serverID,
	})
	if err != nil {
		return err
	}
	return ts.worker.Enqueue(ctx, email, fmt.Sprintf("Server Created: %s", serverName), text, html)
}

func (ts *TriggerService) SendServerSuspended(ctx context.Context, email, serverName, reason string) error {
	text, html, err := ts.renderer.Render(TemplateServerSuspended, EmailData{
		RecipientEmail: email,
		PanelURL:       ts.panelURL,
		CompanyName:    ts.companyName,
		ProductName:    ts.productName,
		ServerName:     serverName,
		Reason:         reason,
	})
	if err != nil {
		return err
	}
	return ts.worker.Enqueue(ctx, email, fmt.Sprintf("Server Suspended: %s", serverName), text, html)
}

func (ts *TriggerService) SendServerUnsuspended(ctx context.Context, email, serverName string) error {
	text, html, err := ts.renderer.Render(TemplateServerUnsuspended, EmailData{
		RecipientEmail: email,
		PanelURL:       ts.panelURL,
		CompanyName:    ts.companyName,
		ProductName:    ts.productName,
		ServerName:     serverName,
	})
	if err != nil {
		return err
	}
	return ts.worker.Enqueue(ctx, email, fmt.Sprintf("Server Unsuspended: %s", serverName), text, html)
}

func (ts *TriggerService) SendBackupComplete(ctx context.Context, email, serverName, backupName, backupSize string) error {
	text, html, err := ts.renderer.Render(TemplateBackupComplete, EmailData{
		RecipientEmail: email,
		PanelURL:       ts.panelURL,
		CompanyName:    ts.companyName,
		ProductName:    ts.productName,
		ServerName:     serverName,
		BackupName:     backupName,
		BackupSize:     backupSize,
	})
	if err != nil {
		return err
	}
	return ts.worker.Enqueue(ctx, email, fmt.Sprintf("Backup Complete: %s", backupName), text, html)
}

func (ts *TriggerService) SendPasswordChanged(ctx context.Context, email string) error {
	text, html, err := ts.renderer.Render(TemplatePasswordChanged, EmailData{
		RecipientEmail: email,
		PanelURL:       ts.panelURL,
		CompanyName:    ts.companyName,
		ProductName:    ts.productName,
	})
	if err != nil {
		return err
	}
	return ts.worker.Enqueue(ctx, email, "Password Changed", text, html)
}

func (ts *TriggerService) Send2FAEnabled(ctx context.Context, email string) error {
	text, html, err := ts.renderer.Render(Template2FAEnabled, EmailData{
		RecipientEmail: email,
		PanelURL:       ts.panelURL,
		CompanyName:    ts.companyName,
		ProductName:    ts.productName,
	})
	if err != nil {
		return err
	}
	return ts.worker.Enqueue(ctx, email, "Two-Factor Authentication Enabled", text, html)
}

func (ts *TriggerService) Send2FADisabled(ctx context.Context, email string) error {
	text, html, err := ts.renderer.Render(Template2FADisabled, EmailData{
		RecipientEmail: email,
		PanelURL:       ts.panelURL,
		CompanyName:    ts.companyName,
		ProductName:    ts.productName,
	})
	if err != nil {
		return err
	}
	return ts.worker.Enqueue(ctx, email, "Two-Factor Authentication Disabled", text, html)
}

func (ts *TriggerService) SendInvitation(ctx context.Context, email, inviterName, inviteURL string) error {
	text, html, err := ts.renderer.Render(TemplateInvitation, EmailData{
		RecipientEmail: email,
		PanelURL:       ts.panelURL,
		CompanyName:    ts.companyName,
		ProductName:    ts.productName,
		InviterName:    inviterName,
		InviteURL:      inviteURL,
	})
	if err != nil {
		return err
	}
	return ts.worker.Enqueue(ctx, email, fmt.Sprintf("Invitation from %s", inviterName), text, html)
}

func (ts *TriggerService) SendSubuserInvited(ctx context.Context, email, recipientName, actorName, serverName, serverID string) error {
	text, html, err := ts.renderer.Render(TemplateSubuserInvited, EmailData{
		RecipientName:  recipientName,
		RecipientEmail: email,
		PanelURL:       ts.panelURL,
		CompanyName:    ts.companyName,
		ProductName:    ts.productName,
		ActorName:      actorName,
		ServerName:     serverName,
		ServerID:       serverID,
	})
	if err != nil {
		return err
	}
	return ts.worker.Enqueue(ctx, email, fmt.Sprintf("Access Granted: %s", serverName), text, html)
}

func (ts *TriggerService) SendSubuserRemoved(ctx context.Context, email, recipientName, actorName, serverName string) error {
	text, html, err := ts.renderer.Render(TemplateSubuserRemoved, EmailData{
		RecipientName:  recipientName,
		RecipientEmail: email,
		PanelURL:       ts.panelURL,
		CompanyName:    ts.companyName,
		ProductName:    ts.productName,
		ActorName:      actorName,
		ServerName:     serverName,
	})
	if err != nil {
		return err
	}
	return ts.worker.Enqueue(ctx, email, fmt.Sprintf("Access Removed: %s", serverName), text, html)
}

func (ts *TriggerService) SendInstallComplete(ctx context.Context, email, recipientName, serverName, serverID string) error {
	text, html, err := ts.renderer.Render(TemplateInstallComplete, EmailData{
		RecipientName:  recipientName,
		RecipientEmail: email,
		PanelURL:       ts.panelURL,
		CompanyName:    ts.companyName,
		ProductName:    ts.productName,
		ServerName:     serverName,
		ServerID:       serverID,
	})
	if err != nil {
		return err
	}
	return ts.worker.Enqueue(ctx, email, fmt.Sprintf("Installation Complete: %s", serverName), text, html)
}

func (ts *TriggerService) SendBackupFailed(ctx context.Context, email, serverName, backupName, failureReason string) error {
	text, html, err := ts.renderer.Render(TemplateBackupFailed, EmailData{
		RecipientEmail: email,
		PanelURL:       ts.panelURL,
		CompanyName:    ts.companyName,
		ProductName:    ts.productName,
		ServerName:     serverName,
		BackupName:     backupName,
		FailureReason:  failureReason,
	})
	if err != nil {
		return err
	}
	return ts.worker.Enqueue(ctx, email, fmt.Sprintf("Backup Failed: %s", backupName), text, html)
}
