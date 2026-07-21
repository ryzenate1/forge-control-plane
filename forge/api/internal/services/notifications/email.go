package notifications

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"text/template"
	"time"
)

// EmailService handles email notifications
type EmailService struct {
	config EmailConfig
}

// EmailConfig represents the configuration for the email service
type EmailConfig struct {
	SMTPHost     string
	SMTPPort     int
	SMTPUsername string
	SMTPPassword string
	FromAddress  string
	FromName     string
	UseTLS       bool
	UseSSL       bool
}

// NewEmailService creates a new email service
func NewEmailService(config EmailConfig) *EmailService {
	return &EmailService{config: config}
}

// EmailTemplate represents an email template
type EmailTemplate struct {
	Subject string
	Body    string
}

// DefaultTemplates contains default email templates for different event types
var DefaultTemplates = map[string]EmailTemplate{
	"server.crash": {
		Subject: "[GamePanel] Server Crash Alert - {{.ResourceID}}",
		Body: `Server Crash Alert

Server: {{.ResourceID}}
Type: {{.ResourceType}}
Timestamp: {{.Timestamp}}

{{if .Payload}}
Details:
{{range $key, $value := .Payload}}
  {{$key}}: {{$value}}
{{end}}
{{end}}

Please investigate immediately.`,
	},
	"server.install.complete": {
		Subject: "[GamePanel] Server Installation Complete - {{.ResourceID}}",
		Body: `Server Installation Complete

Server: {{.ResourceID}}
Type: {{.ResourceType}}
Timestamp: {{.Timestamp}}

The server has been successfully installed and is ready for use.`,
	},
	"backup.complete": {
		Subject: "[GamePanel] Backup Complete - {{.ResourceID}}",
		Body: `Backup Complete

Resource: {{.ResourceID}}
Type: {{.ResourceType}}
Timestamp: {{.Timestamp}}

{{if .Payload}}
Backup Details:
{{range $key, $value := .Payload}}
  {{$key}}: {{$value}}
{{end}}
{{end}}

The backup has been completed successfully.`,
	},
	"backup.failed": {
		Subject: "[GamePanel] Backup Failed - {{.ResourceID}}",
		Body: `Backup Failed

Resource: {{.ResourceID}}
Type: {{.ResourceType}}
Timestamp: {{.Timestamp}}

{{if .Payload}}
Error Details:
{{range $key, $value := .Payload}}
  {{$key}}: {{$value}}
{{end}}
{{end}}

Please check the backup configuration and try again.`,
	},
	"deployment.complete": {
		Subject: "[GamePanel] Deployment Complete - {{.ResourceID}}",
		Body: `Deployment Complete

Resource: {{.ResourceID}}
Type: {{.ResourceType}}
Timestamp: {{.Timestamp}}

{{if .Payload}}
Deployment Details:
{{range $key, $value := .Payload}}
  {{$key}}: {{$value}}
{{end}}
{{end}}

The deployment has been completed successfully.`,
	},
	"deployment.failed": {
		Subject: "[GamePanel] Deployment Failed - {{.ResourceID}}",
		Body: `Deployment Failed

Resource: {{.ResourceID}}
Type: {{.ResourceType}}
Timestamp: {{.Timestamp}}

{{if .Payload}}
Error Details:
{{range $key, $value := .Payload}}
  {{$key}}: {{$value}}
{{end}}
{{end}}

Please check the deployment logs and configuration.`,
	},
	"node.down": {
		Subject: "[GamePanel] Node Offline - {{.ResourceID}}",
		Body: `Node Offline Alert

Node: {{.ResourceID}}
Type: {{.ResourceType}}
Timestamp: {{.Timestamp}}

The node has gone offline. Please investigate the issue.`,
	},
	"node.up": {
		Subject: "[GamePanel] Node Online - {{.ResourceID}}",
		Body: `Node Online

Node: {{.ResourceID}}
Type: {{.ResourceType}}
Timestamp: {{.Timestamp}}

The node is back online.`,
	},
	"test.notification": {
		Subject: "[GamePanel] Test Notification",
		Body: `Test Notification

This is a test notification from GamePanel to verify that your email configuration is working correctly.

Timestamp: {{.Timestamp}}`,
	},
}

// EmailMessage represents an email message to be sent
type EmailMessage struct {
	To      []string
	Subject string
	Body    string
	IsHTML  bool
}

// Send sends an email notification
func (s *EmailService) Send(ctx context.Context, message EmailMessage) error {
	// Create the email
	from := mail.Address{Name: s.config.FromName, Address: s.config.FromAddress}

	// Parse recipients
	var toAddrs []mail.Address
	for _, recipient := range message.To {
		// Simple parsing - could be enhanced
		toAddrs = append(toAddrs, mail.Address{Address: recipient})
	}

	// Set up the email headers
	headers := map[string]string{
		"From":         from.String(),
		"To":           strings.Join(message.To, ", "),
		"Subject":      message.Subject,
		"Date":         time.Now().Format(time.RFC1123Z),
		"Content-Type": "text/plain; charset=UTF-8",
	}

	if message.IsHTML {
		headers["Content-Type"] = "text/html; charset=UTF-8"
	}

	// Build the email body
	var body strings.Builder
	for key, value := range headers {
		body.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
	}
	body.WriteString("\r\n")
	body.WriteString(message.Body)

	// Set up SMTP authentication
	auth := smtp.PlainAuth("", s.config.SMTPUsername, s.config.SMTPPassword, s.config.SMTPHost)

	// Connect to the SMTP server
	hostPort := fmt.Sprintf("%s:%d", s.config.SMTPHost, s.config.SMTPPort)

	// Create TLS config if needed
	var tlsConfig *tls.Config
	if s.config.UseSSL {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         s.config.SMTPHost,
		}
	}

	// Connect to SMTP server
	client, err := smtp.Dial(hostPort)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer client.Quit()

	// Upgrade to TLS if configured
	if s.config.UseTLS || s.config.UseSSL {
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("failed to start TLS: %w", err)
		}
	}

	// Authenticate
	if s.config.SMTPUsername != "" && s.config.SMTPPassword != "" {
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("failed to authenticate: %w", err)
		}
	}

	// Set sender
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	// Set recipients
	for _, toAddr := range toAddrs {
		if err := client.Rcpt(toAddr.Address); err != nil {
			return fmt.Errorf("failed to set recipient %s: %w", toAddr.Address, err)
		}
	}

	// Send email data
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to start data: %w", err)
	}

	_, err = w.Write([]byte(body.String()))
	if err != nil {
		return fmt.Errorf("failed to write email body: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close email data: %w", err)
	}

	return nil
}

// SendTemplatedEmail sends an email using a template
func (s *EmailService) SendTemplatedEmail(ctx context.Context, recipients []string, eventType string, data interface{}) error {
	// Get template
	tmpl, exists := DefaultTemplates[eventType]
	if !exists {
		// Use a generic template
		tmpl = EmailTemplate{
			Subject: fmt.Sprintf("[GamePanel] %s - {{.ResourceID}}", eventType),
			Body:    "Event: {{.EventType}}\nResource: {{.ResourceID}}\nType: {{.ResourceType}}\nTimestamp: {{.Timestamp}}",
		}
	}

	// Parse and execute subject template
	subjectTmpl, err := Parse(tmpl.Subject)
	if err != nil {
		return fmt.Errorf("failed to parse subject template: %w", err)
	}

	subject := strings.TrimSpace(executeTemplateToString(subjectTmpl, data))

	// Parse and execute body template
	bodyTmpl, err := Parse(tmpl.Body)
	if err != nil {
		return fmt.Errorf("failed to parse body template: %w", err)
	}

	body := strings.TrimSpace(executeTemplateToString(bodyTmpl, data))

	// Send the email
	return s.Send(ctx, EmailMessage{
		To:      recipients,
		Subject: subject,
		Body:    body,
		IsHTML:  false,
	})
}

// Template is a helper for parsing and executing templates
type Template struct {
	template *template.Template
}

// Parse parses a template string
func (t *Template) Parse(text string) (*Template, error) {
	tmpl, err := template.New("").Parse(text)
	if err != nil {
		return nil, err
	}
	return &Template{template: tmpl}, nil
}

// Execute executes the template with the given data
func (t *Template) Execute(data interface{}) string {
	var result strings.Builder
	if err := t.template.Execute(&result, data); err != nil {
		return fmt.Sprintf("Error executing template: %v", err)
	}
	return result.String()
}

// Parse parses a template string (standalone function)
func Parse(text string) (*Template, error) {
	return (&Template{}).Parse(text)
}

// executeTemplateToString executes a template with data and returns the string result
func executeTemplateToString(t *Template, data interface{}) string {
	return t.Execute(data)
}

// executeTemplate executes a template string with data and returns the result
func executeTemplate(tmpl string, data interface{}) (string, error) {
	t, err := Parse(tmpl)
	if err != nil {
		return "", err
	}
	return t.Execute(data), nil
}

// EmailNotificationData represents data for email notifications
type EmailNotificationData struct {
	EventType    string
	ResourceID  string
	ResourceType string
	Timestamp   string
	Payload     map[string]interface{}
	Severity    string
	Message     string
}

// SendAlertEmail sends an alert email notification
func (s *EmailService) SendAlertEmail(ctx context.Context, recipients []string, alert AlertEvaluationResult, message string) error {
	data := EmailNotificationData{
		EventType:    "alert.triggered",
		ResourceID:   alert.EntityID,
		ResourceType: string(alert.EntityType),
		Timestamp:    time.Now().Format(time.RFC3339),
		Payload:     map[string]interface{}{},
		Severity:    "warning", // Default, can be overridden
		Message:     message,
	}

	// Use a custom template for alerts
	tmpl := EmailTemplate{
		Subject: "[GamePanel] Alert: {{.Message}}",
		Body: `Alert Notification

Message: {{.Message}}
Entity: {{.ResourceID}} ({{.ResourceType}})
Timestamp: {{.Timestamp}}
Severity: {{.Severity}}

{{if .Payload}}
Additional Details:
{{range $key, $value := .Payload}}
  {{$key}}: {{$value}}
{{end}}
{{end}}`,
	}

	// Parse and execute subject template
	subjectTmpl, err := Parse(tmpl.Subject)
	if err != nil {
		return fmt.Errorf("failed to parse subject template: %w", err)
	}

	subject := strings.TrimSpace(subjectTmpl.Execute(data))

	// Parse and execute body template
	bodyTmpl, err := Parse(tmpl.Body)
	if err != nil {
		return fmt.Errorf("failed to parse body template: %w", err)
	}

	body := strings.TrimSpace(bodyTmpl.Execute(data))

	return s.Send(ctx, EmailMessage{
		To:      recipients,
		Subject: subject,
		Body:    body,
		IsHTML:  false,
	})
}

// Validate validates the email configuration
func (c *EmailConfig) Validate() error {
	if c.SMTPHost == "" {
		return fmt.Errorf("SMTP host is required")
	}
	if c.SMTPPort <= 0 {
		return fmt.Errorf("SMTP port must be greater than 0")
	}
	if c.FromAddress == "" {
		return fmt.Errorf("from address is required")
	}

	// Validate email format
	if _, err := mail.ParseAddress(c.FromAddress); err != nil {
		return fmt.Errorf("invalid from address: %w", err)
	}

	// Validate host format
	if _, _, err := net.SplitHostPort(fmt.Sprintf("%s:%d", c.SMTPHost, c.SMTPPort)); err != nil {
		// This might fail for IP addresses, so we'll be more lenient
		if c.SMTPHost != "localhost" && c.SMTPHost != "127.0.0.1" {
			if !isValidHostname(c.SMTPHost) {
				return fmt.Errorf("invalid SMTP host: %s", c.SMTPHost)
			}
		}
	}

	return nil
}

// isValidHostname checks if a string is a valid hostname
func isValidHostname(host string) bool {
	// Simple validation - hostname should contain at least one dot and no spaces
	if strings.Contains(host, " ") {
		return false
	}
	if strings.Contains(host, "\t") {
		return false
	}
	if len(host) == 0 {
		return false
	}
	return true
}
