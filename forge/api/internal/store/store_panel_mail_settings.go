package store

import (
	"context"
	"errors"
)

type PanelMailSettings struct {
	SMTPHost        string `json:"smtpHost"`
	SMTPPort        int    `json:"smtpPort"`
	SMTPEncryption  string `json:"smtpEncryption"`
	SMTPUsername    string `json:"smtpUsername"`
	SMTPPassword    string `json:"smtpPassword"`
	MailFromAddress string `json:"mailFromAddress"`
	MailFromName    string `json:"mailFromName"`
}

type PanelAdvancedSettings struct {
	RecaptchaEnabled     bool   `json:"recaptchaEnabled"`
	RecaptchaWebsiteKey  string `json:"recaptchaWebsiteKey"`
	RecaptchaSecretKey   string `json:"recaptchaSecretKey"`
	GuzzleConnectTimeout int    `json:"guzzleConnectTimeout"`
	GuzzleRequestTimeout int    `json:"guzzleRequestTimeout"`
	AutoAllocEnabled     bool   `json:"autoAllocEnabled"`
	AutoAllocStartPort   int    `json:"autoAllocStartPort"`
	AutoAllocEndPort     int    `json:"autoAllocEndPort"`
}

func defaultPanelMailSettings() PanelMailSettings {
	return PanelMailSettings{
		SMTPPort:       587,
		SMTPEncryption: "tls",
	}
}

func defaultPanelAdvancedSettings() PanelAdvancedSettings {
	return PanelAdvancedSettings{
		GuzzleConnectTimeout: 30,
		GuzzleRequestTimeout: 30,
		AutoAllocStartPort:   25565,
		AutoAllocEndPort:     25600,
	}
}

func (s *Store) GetPanelMailSettings(ctx context.Context) (PanelMailSettings, error) {
	if s.db == nil {
		return defaultPanelMailSettings(), errors.New("no database connection")
	}
	row := s.db.QueryRow(ctx, `
		SELECT smtp_host, smtp_port, smtp_encryption, smtp_username, COALESCE(smtp_password,''), COALESCE(smtp_password_encrypted,''), mail_from_address, mail_from_name
		FROM panel_mail_settings WHERE id = TRUE
	`)
	var m PanelMailSettings
	var plaintext, encrypted string
	err := row.Scan(&m.SMTPHost, &m.SMTPPort, &m.SMTPEncryption, &m.SMTPUsername, &plaintext, &encrypted, &m.MailFromAddress, &m.MailFromName)
	if err != nil {
		return defaultPanelMailSettings(), err
	}
	m.SMTPPassword, err = s.decryptSecret(encrypted, plaintext, secretAAD("panel_mail_settings", "true", "smtp_password"))
	if err != nil {
		return defaultPanelMailSettings(), err
	}
	return m, nil
}

func (s *Store) UpdatePanelMailSettings(ctx context.Context, m PanelMailSettings) error {
	if s.db == nil {
		return errors.New("no database connection")
	}
	encryptedPassword, err := s.encryptSecret(m.SMTPPassword, secretAAD("panel_mail_settings", "true", "smtp_password"))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO panel_mail_settings (
			id, smtp_host, smtp_port, smtp_encryption, smtp_username, smtp_password, smtp_password_encrypted,
			mail_from_address, mail_from_name, updated_at
		) VALUES (TRUE, $1, $2, $3, $4, '', $5, $6, $7, now())
		ON CONFLICT (id) DO UPDATE SET
			smtp_host = EXCLUDED.smtp_host,
			smtp_port = EXCLUDED.smtp_port,
			smtp_encryption = EXCLUDED.smtp_encryption,
			smtp_username = EXCLUDED.smtp_username,
			smtp_password = '',
			smtp_password_encrypted = CASE WHEN EXCLUDED.smtp_password_encrypted = '' THEN panel_mail_settings.smtp_password_encrypted ELSE EXCLUDED.smtp_password_encrypted END,
			mail_from_address = EXCLUDED.mail_from_address,
			mail_from_name = EXCLUDED.mail_from_name,
			updated_at = now()
	`,
		m.SMTPHost, m.SMTPPort, m.SMTPEncryption, m.SMTPUsername, encryptedPassword,
		m.MailFromAddress, m.MailFromName,
	)
	return err
}

func (s *Store) GetPanelAdvancedSettings(ctx context.Context) (PanelAdvancedSettings, error) {
	if s.db == nil {
		return defaultPanelAdvancedSettings(), errors.New("no database connection")
	}
	row := s.db.QueryRow(ctx, `
		SELECT recaptcha_enabled, recaptcha_website_key, COALESCE(recaptcha_secret_key,''), COALESCE(recaptcha_secret_key_encrypted,''),
		       guzzle_connect_timeout, guzzle_request_timeout,
		       auto_alloc_enabled, auto_alloc_start_port, auto_alloc_end_port
		FROM panel_advanced_settings WHERE id = TRUE
	`)
	var a PanelAdvancedSettings
	var plaintext, encrypted string
	err := row.Scan(
		&a.RecaptchaEnabled, &a.RecaptchaWebsiteKey, &plaintext, &encrypted,
		&a.GuzzleConnectTimeout, &a.GuzzleRequestTimeout,
		&a.AutoAllocEnabled, &a.AutoAllocStartPort, &a.AutoAllocEndPort,
	)
	if err != nil {
		return defaultPanelAdvancedSettings(), err
	}
	a.RecaptchaSecretKey, err = s.decryptSecret(encrypted, plaintext, secretAAD("panel_advanced_settings", "true", "recaptcha_secret_key"))
	if err != nil {
		return defaultPanelAdvancedSettings(), err
	}
	return a, nil
}

func (s *Store) UpdatePanelAdvancedSettings(ctx context.Context, a PanelAdvancedSettings) error {
	if s.db == nil {
		return errors.New("no database connection")
	}
	encryptedSecret, err := s.encryptSecret(a.RecaptchaSecretKey, secretAAD("panel_advanced_settings", "true", "recaptcha_secret_key"))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO panel_advanced_settings (
			id, recaptcha_enabled, recaptcha_website_key, recaptcha_secret_key, recaptcha_secret_key_encrypted,
			guzzle_connect_timeout, guzzle_request_timeout,
			auto_alloc_enabled, auto_alloc_start_port, auto_alloc_end_port, updated_at
		) VALUES (TRUE, $1, $2, '', $3, $4, $5, $6, $7, $8, now())
		ON CONFLICT (id) DO UPDATE SET
			recaptcha_enabled = EXCLUDED.recaptcha_enabled,
			recaptcha_website_key = EXCLUDED.recaptcha_website_key,
			recaptcha_secret_key = '',
			recaptcha_secret_key_encrypted = EXCLUDED.recaptcha_secret_key_encrypted,
			guzzle_connect_timeout = EXCLUDED.guzzle_connect_timeout,
			guzzle_request_timeout = EXCLUDED.guzzle_request_timeout,
			auto_alloc_enabled = EXCLUDED.auto_alloc_enabled,
			auto_alloc_start_port = EXCLUDED.auto_alloc_start_port,
			auto_alloc_end_port = EXCLUDED.auto_alloc_end_port,
			updated_at = now()
	`,
		a.RecaptchaEnabled, a.RecaptchaWebsiteKey, encryptedSecret,
		a.GuzzleConnectTimeout, a.GuzzleRequestTimeout,
		a.AutoAllocEnabled, a.AutoAllocStartPort, a.AutoAllocEndPort,
	)
	return err
}
