package mail

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	stdmail "net/mail"
	"net/smtp"
	"strings"
	"time"

	"gamepanel/forge/internal/store"
)

type SMTPSender struct{ Timeout time.Duration }

func (s SMTPSender) Send(ctx context.Context, settings store.PanelMailSettings, recipient, subject, textBody, htmlBody string) error {
	if err := ValidateSettings(settings); err != nil {
		return err
	}
	if _, err := stdmail.ParseAddress(recipient); err != nil {
		return fmt.Errorf("invalid recipient: %w", err)
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	address := net.JoinHostPort(settings.SMTPHost, fmt.Sprintf("%d", settings.SMTPPort))
	conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("connect SMTP server: %w", err)
	}
	defer conn.Close()
	stopCancelClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancelClose()
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	host := strings.TrimSpace(settings.SMTPHost)
	tlsConfig := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	mode := normalizeTLSMode(settings.SMTPEncryption)
	if mode == "ssl" {
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("SMTP TLS handshake: %w", err)
		}
		conn = tlsConn
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("start SMTP session: %w", err)
	}
	defer client.Close()
	if mode == "tls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("SMTP server does not support required STARTTLS")
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("start SMTP TLS: %w", err)
		}
	}
	if settings.SMTPUsername != "" {
		if err := client.Auth(smtp.PlainAuth("", settings.SMTPUsername, settings.SMTPPassword, host)); err != nil {
			return fmt.Errorf("SMTP authentication: %w", err)
		}
	}
	if err := client.Mail(settings.MailFromAddress); err != nil {
		return fmt.Errorf("SMTP MAIL FROM: %w", err)
	}
	if err := client.Rcpt(recipient); err != nil {
		return fmt.Errorf("SMTP RCPT TO: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA: %w", err)
	}
	if _, err := writer.Write(buildMessage(settings, recipient, subject, textBody, htmlBody)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write SMTP message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish SMTP message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("quit SMTP session: %w", err)
	}
	return nil
}

func ValidateSettings(settings store.PanelMailSettings) error {
	if strings.TrimSpace(settings.SMTPHost) == "" {
		return errors.New("SMTP host is required")
	}
	if settings.SMTPPort < 1 || settings.SMTPPort > 65535 {
		return errors.New("SMTP port must be between 1 and 65535")
	}
	if _, err := stdmail.ParseAddress(settings.MailFromAddress); err != nil {
		return fmt.Errorf("invalid mail from address: %w", err)
	}
	switch normalizeTLSMode(settings.SMTPEncryption) {
	case "", "tls", "ssl":
	default:
		return errors.New("SMTP encryption must be none, tls, or ssl")
	}
	return nil
}

func normalizeTLSMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "none":
		return ""
	case "tls", "starttls":
		return "tls"
	case "ssl", "smtps":
		return "ssl"
	default:
		return strings.ToLower(strings.TrimSpace(mode))
	}
}

func buildMessage(settings store.PanelMailSettings, recipient, subject, textBody, htmlBody string) []byte {
	fromAddress := stdmail.Address{Name: settings.MailFromName, Address: settings.MailFromAddress}
	from := fromAddress.String()
	subject = strings.NewReplacer("\r", " ", "\n", " ").Replace(subject)
	var b strings.Builder
	w := bufio.NewWriter(&b)
	fmt.Fprintf(w, "From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\n", from, recipient, subject)
	if htmlBody == "" {
		fmt.Fprintf(w, "Content-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s", textBody)
	} else {
		boundary := "forge-mail-boundary"
		fmt.Fprintf(w, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n--%s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n--%s\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n--%s--\r\n", boundary, boundary, textBody, boundary, htmlBody, boundary)
	}
	_ = w.Flush()
	return []byte(b.String())
}
