package store

import (
	"context"
	"encoding/json"
	"errors"
)

func (s *Store) GetPanelSettings(ctx context.Context) (PanelSettings, error) {
	if s.db == nil {
		return DefaultPanelSettings(), errors.New("no database connection")
	}
	row := s.db.QueryRow(ctx, `
		SELECT company_name, require_2fa, default_locale,
		       smtp_host, smtp_port, smtp_encryption, smtp_username, COALESCE(smtp_password,''), COALESCE(smtp_password_encrypted,''),
		       mail_from_address, mail_from_name,
		       recaptcha_enabled, recaptcha_site_key, COALESCE(recaptcha_secret_key,''), COALESCE(recaptcha_secret_key_encrypted,''),
		       connection_timeout, request_timeout,
		       auto_alloc_enabled, auto_alloc_start_port, auto_alloc_end_port,
		       s3_backup_enabled, s3_endpoint, s3_region, s3_bucket, s3_access_key_id,
		       COALESCE(s3_secret_access_key,''), COALESCE(s3_secret_access_key_encrypted,''),
		       s3_prefix, s3_use_path_style
		FROM panel_settings
		WHERE id = TRUE
	`)
	var ps PanelSettings
	var smtpPlaintext, smtpEncrypted, recaptchaPlaintext, recaptchaEncrypted string
	var s3Plaintext, s3Encrypted string
	err := row.Scan(
		&ps.CompanyName, &ps.Require2FA, &ps.DefaultLocale,
		&ps.SMTPHost, &ps.SMTPPort, &ps.SMTPEncryption, &ps.SMTPUsername, &smtpPlaintext, &smtpEncrypted,
		&ps.MailFromAddress, &ps.MailFromName,
		&ps.RecaptchaEnabled, &ps.RecaptchaSiteKey, &recaptchaPlaintext, &recaptchaEncrypted,
		&ps.ConnectionTimeout, &ps.RequestTimeout,
		&ps.AutoAllocEnabled, &ps.AutoAllocStartPort, &ps.AutoAllocEndPort,
		&ps.S3BackupEnabled, &ps.S3Endpoint, &ps.S3Region, &ps.S3Bucket, &ps.S3AccessKeyID, &s3Plaintext, &s3Encrypted,
		&ps.S3Prefix, &ps.S3UsePathStyle,
	)
	if err != nil {
		return DefaultPanelSettings(), err
	}
	// Handle legacy integer values for Require2FA
	if ps.Require2FA == "0" || ps.Require2FA == "1" {
		ps.Require2FA = map[string]string{"0": "none", "1": "admin"}[ps.Require2FA]
	}
	ps.SMTPPassword, err = s.decryptSecret(smtpEncrypted, smtpPlaintext, secretAAD("panel_settings", "true", "smtp_password"))
	if err != nil {
		return DefaultPanelSettings(), err
	}
	ps.RecaptchaSecretKey, err = s.decryptSecret(recaptchaEncrypted, recaptchaPlaintext, secretAAD("panel_settings", "true", "recaptcha_secret_key"))
	if err != nil {
		return DefaultPanelSettings(), err
	}
	ps.S3SecretAccessKey, err = s.decryptSecret(s3Encrypted, s3Plaintext, secretAAD("panel_settings", "true", "s3_secret_access_key"))
	if err != nil {
		return DefaultPanelSettings(), err
	}
	ps = mergeExpandedPanelSettings(ctx, s, ps)
	return ps, nil
}

func mergeExpandedPanelSettings(ctx context.Context, s *Store, ps PanelSettings) PanelSettings {
	var raw []byte
	var discordEncrypted, slackEncrypted, telegramEncrypted string
	if err := s.db.QueryRow(ctx, `SELECT settings, COALESCE(discord_webhook_url_encrypted,''), COALESCE(slack_webhook_url_encrypted,''), COALESCE(telegram_bot_token_encrypted,'') FROM panel_settings_expanded WHERE id = TRUE`).Scan(&raw, &discordEncrypted, &slackEncrypted, &telegramEncrypted); err != nil {
		return ps
	}
	if len(raw) == 0 {
		return ps
	}
	_ = json.Unmarshal(raw, &ps)
	ps.DiscordWebhookURL, _ = s.decryptSecret(discordEncrypted, "", secretAAD("panel_settings_expanded", "true", "discordWebhookUrl"))
	ps.SlackWebhookURL, _ = s.decryptSecret(slackEncrypted, "", secretAAD("panel_settings_expanded", "true", "slackWebhookUrl"))
	ps.TelegramBotToken, _ = s.decryptSecret(telegramEncrypted, "", secretAAD("panel_settings_expanded", "true", "telegramBotToken"))
	return ps
}

func (s *Store) UpdatePanelSettings(ctx context.Context, ps PanelSettings) error {
	if s.db == nil {
		return errors.New("no database connection")
	}
	smtpEncrypted, err := s.encryptSecret(ps.SMTPPassword, secretAAD("panel_settings", "true", "smtp_password"))
	if err != nil {
		return err
	}
	recaptchaEncrypted, err := s.encryptSecret(ps.RecaptchaSecretKey, secretAAD("panel_settings", "true", "recaptcha_secret_key"))
	if err != nil {
		return err
	}
	s3Encrypted, err := s.encryptSecret(ps.S3SecretAccessKey, secretAAD("panel_settings", "true", "s3_secret_access_key"))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO panel_settings (
			id, company_name, require_2fa, default_locale,
			smtp_host, smtp_port, smtp_encryption, smtp_username, smtp_password, smtp_password_encrypted,
			mail_from_address, mail_from_name,
			recaptcha_enabled, recaptcha_site_key, recaptcha_secret_key, recaptcha_secret_key_encrypted,
			connection_timeout, request_timeout,
			auto_alloc_enabled, auto_alloc_start_port, auto_alloc_end_port,
			s3_backup_enabled, s3_endpoint, s3_region, s3_bucket, s3_access_key_id, s3_secret_access_key, s3_secret_access_key_encrypted, s3_prefix, s3_use_path_style,
			updated_at
		) VALUES (
			TRUE, $1, $2, $3, $4, $5, $6, $7, '', $8, $9, $10, $11, $12, '', $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, now()
		)
		ON CONFLICT (id) DO UPDATE SET
			company_name = EXCLUDED.company_name,
			require_2fa = EXCLUDED.require_2fa,
			default_locale = EXCLUDED.default_locale,
			smtp_host = EXCLUDED.smtp_host,
			smtp_port = EXCLUDED.smtp_port,
			smtp_encryption = EXCLUDED.smtp_encryption,
			smtp_username = EXCLUDED.smtp_username,
			smtp_password = '',
			smtp_password_encrypted = EXCLUDED.smtp_password_encrypted,
			mail_from_address = EXCLUDED.mail_from_address,
			mail_from_name = EXCLUDED.mail_from_name,
			recaptcha_enabled = EXCLUDED.recaptcha_enabled,
			recaptcha_site_key = EXCLUDED.recaptcha_site_key,
			recaptcha_secret_key = '',
			recaptcha_secret_key_encrypted = EXCLUDED.recaptcha_secret_key_encrypted,
			connection_timeout = EXCLUDED.connection_timeout,
			request_timeout = EXCLUDED.request_timeout,
			auto_alloc_enabled = EXCLUDED.auto_alloc_enabled,
			auto_alloc_start_port = EXCLUDED.auto_alloc_start_port,
			auto_alloc_end_port = EXCLUDED.auto_alloc_end_port,
			s3_backup_enabled = EXCLUDED.s3_backup_enabled,
			s3_endpoint = EXCLUDED.s3_endpoint,
			s3_region = EXCLUDED.s3_region,
			s3_bucket = EXCLUDED.s3_bucket,
			s3_access_key_id = EXCLUDED.s3_access_key_id,
			s3_secret_access_key = '',
			s3_secret_access_key_encrypted = EXCLUDED.s3_secret_access_key_encrypted,
			s3_prefix = EXCLUDED.s3_prefix,
			s3_use_path_style = EXCLUDED.s3_use_path_style,
			updated_at = now()
	`,
		ps.CompanyName, ps.Require2FA, ps.DefaultLocale,
		ps.SMTPHost, ps.SMTPPort, ps.SMTPEncryption, ps.SMTPUsername, smtpEncrypted,
		ps.MailFromAddress, ps.MailFromName,
		ps.RecaptchaEnabled, ps.RecaptchaSiteKey, recaptchaEncrypted,
		ps.ConnectionTimeout, ps.RequestTimeout,
		ps.AutoAllocEnabled, ps.AutoAllocStartPort, ps.AutoAllocEndPort,
		ps.S3BackupEnabled, ps.S3Endpoint, ps.S3Region, ps.S3Bucket, ps.S3AccessKeyID, s3Encrypted,
		ps.S3Prefix, ps.S3UsePathStyle,
	)
	if err != nil {
		return err
	}
	discordEncrypted, err := s.encryptSecret(ps.DiscordWebhookURL, secretAAD("panel_settings_expanded", "true", "discordWebhookUrl"))
	if err != nil {
		return err
	}
	slackEncrypted, err := s.encryptSecret(ps.SlackWebhookURL, secretAAD("panel_settings_expanded", "true", "slackWebhookUrl"))
	if err != nil {
		return err
	}
	telegramEncrypted, err := s.encryptSecret(ps.TelegramBotToken, secretAAD("panel_settings_expanded", "true", "telegramBotToken"))
	if err != nil {
		return err
	}
	redacted := ps
	redacted.SMTPPassword, redacted.RecaptchaSecretKey = "", ""
	redacted.DiscordWebhookURL, redacted.SlackWebhookURL, redacted.TelegramBotToken = "", "", ""
	body, err := json.Marshal(redacted)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO panel_settings_expanded (id, settings, discord_webhook_url_encrypted, slack_webhook_url_encrypted, telegram_bot_token_encrypted, updated_at)
		VALUES (TRUE, $1::jsonb, $2, $3, $4, now())
		ON CONFLICT (id) DO UPDATE SET
			settings = EXCLUDED.settings,
			discord_webhook_url_encrypted=EXCLUDED.discord_webhook_url_encrypted,
			slack_webhook_url_encrypted=EXCLUDED.slack_webhook_url_encrypted,
			telegram_bot_token_encrypted=EXCLUDED.telegram_bot_token_encrypted,
			updated_at = now()
	`, string(body), discordEncrypted, slackEncrypted, telegramEncrypted)
	return err
}
