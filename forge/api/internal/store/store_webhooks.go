package store

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

type WebhookType string

const (
	WebhookTypeRegular WebhookType = "regular"
	WebhookTypeDiscord WebhookType = "discord"
	maskedStoreSecret  string      = "********"
)

type Webhook struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	Description      string      `json:"description"`
	URL              string      `json:"url"`
	WebhookType      WebhookType `json:"webhookType"`
	Events           []string    `json:"events"`
	Enabled          bool        `json:"enabled"`
	Secret           string      `json:"secret,omitempty"`
	DiscordUsername  string      `json:"discordUsername,omitempty"`
	DiscordAvatarURL string      `json:"discordAvatarUrl,omitempty"`
	DiscordContent   string      `json:"discordContent,omitempty"`
	CreatedAt        time.Time   `json:"createdAt"`
	UpdatedAt        time.Time   `json:"updatedAt"`
}

type CreateWebhookRequest struct {
	Name             string
	Description      string
	URL              string
	WebhookType      WebhookType
	Events           []string
	Enabled          bool
	Secret           string
	DiscordUsername  string
	DiscordAvatarURL string
	DiscordContent   string
}

func (s *Store) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, COALESCE(description,''), url, webhook_type,
		       COALESCE(events,'{}'), enabled, (COALESCE(secret,'') <> '' OR COALESCE(secret_encrypted,'') <> ''),
		       COALESCE(discord_username,''), COALESCE(discord_avatar_url,''),
		       COALESCE(discord_content,''), created_at, updated_at
		FROM webhooks ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// Return [] rather than null when no hooks exist; this is the list endpoint's JSON contract.
	whs := []Webhook{}
	for rows.Next() {
		var wh Webhook
		var hasSecret bool
		if err := rows.Scan(&wh.ID, &wh.Name, &wh.Description, &wh.URL, &wh.WebhookType,
			&wh.Events, &wh.Enabled, &hasSecret,
			&wh.DiscordUsername, &wh.DiscordAvatarURL, &wh.DiscordContent,
			&wh.CreatedAt, &wh.UpdatedAt); err != nil {
			return nil, err
		}
		if hasSecret {
			wh.Secret = maskedStoreSecret
		}
		whs = append(whs, wh)
	}
	return whs, rows.Err()
}

func (s *Store) getWebhookInternal(ctx context.Context, id string) (Webhook, error) {
	var wh Webhook
	var plaintext, encrypted string
	err := s.db.QueryRow(ctx, `
		SELECT id, name, COALESCE(description,''), url, webhook_type,
		       COALESCE(events,'{}'), enabled, COALESCE(secret,''), COALESCE(secret_encrypted,''),
		       COALESCE(discord_username,''), COALESCE(discord_avatar_url,''),
		       COALESCE(discord_content,''), created_at, updated_at
		FROM webhooks WHERE id = $1
	`, id).Scan(&wh.ID, &wh.Name, &wh.Description, &wh.URL, &wh.WebhookType,
		&wh.Events, &wh.Enabled, &plaintext, &encrypted,
		&wh.DiscordUsername, &wh.DiscordAvatarURL, &wh.DiscordContent,
		&wh.CreatedAt, &wh.UpdatedAt)
	if err != nil {
		return Webhook{}, errors.New("webhook not found")
	}
	wh.Secret, err = s.decryptSecret(encrypted, plaintext, secretAAD("webhooks", wh.ID, "secret"))
	if err != nil {
		return Webhook{}, err
	}
	return wh, nil
}

func (s *Store) GetWebhook(ctx context.Context, id string) (Webhook, error) {
	wh, err := s.getWebhookInternal(ctx, id)
	if err != nil {
		return Webhook{}, err
	}
	if wh.Secret != "" {
		wh.Secret = maskedStoreSecret
	}
	return wh, nil
}

func (s *Store) CreateWebhook(ctx context.Context, req CreateWebhookRequest) (Webhook, error) {
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.URL) == "" {
		return Webhook{}, errors.New("name and URL are required")
	}
	if err := ValidateWebhookURL(ctx, req.URL); err != nil {
		return Webhook{}, err
	}
	id := uuid.NewString()
	encrypted, err := s.encryptSecret(req.Secret, secretAAD("webhooks", id, "secret"))
	if err != nil {
		return Webhook{}, err
	}
	now := time.Now().UTC()
	if req.Events == nil {
		req.Events = []string{}
	}
	if _, err := s.db.Exec(ctx, `
		INSERT INTO webhooks (id, name, description, url, webhook_type, events, enabled, secret, secret_encrypted,
		                      discord_username, discord_avatar_url, discord_content, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'',$8,$9,$10,$11,$12,$13)
	`, id, strings.TrimSpace(req.Name), strings.TrimSpace(req.Description),
		strings.TrimSpace(req.URL), req.WebhookType, req.Events, req.Enabled,
		encrypted, req.DiscordUsername, req.DiscordAvatarURL, req.DiscordContent,
		now, now); err != nil {
		return Webhook{}, err
	}
	return s.GetWebhook(ctx, id)
}

func (s *Store) UpdateWebhook(ctx context.Context, id string, req CreateWebhookRequest) (Webhook, error) {
	existing, err := s.getWebhookInternal(ctx, id)
	if err != nil {
		return Webhook{}, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = existing.Name
	}
	urlValue := strings.TrimSpace(req.URL)
	if urlValue == "" {
		urlValue = existing.URL
	}
	if err := ValidateWebhookURL(ctx, urlValue); err != nil {
		return Webhook{}, err
	}
	if req.Events == nil {
		req.Events = existing.Events
	}
	if strings.TrimSpace(req.Secret) == "" || req.Secret == maskedStoreSecret {
		req.Secret = existing.Secret
	}
	encrypted, err := s.encryptSecret(req.Secret, secretAAD("webhooks", id, "secret"))
	if err != nil {
		return Webhook{}, err
	}
	webhookType := req.WebhookType
	if webhookType == "" {
		webhookType = existing.WebhookType
	}
	if _, err := s.db.Exec(ctx, `
		UPDATE webhooks SET name=$1, description=$2, url=$3, webhook_type=$4,
		       events=$5, enabled=$6, secret='', secret_encrypted=$7, discord_username=$8,
		       discord_avatar_url=$9, discord_content=$10, updated_at=$11
		WHERE id=$12
	`, name, strings.TrimSpace(req.Description), urlValue, webhookType,
		req.Events, req.Enabled, encrypted,
		req.DiscordUsername, req.DiscordAvatarURL, req.DiscordContent,
		time.Now().UTC(), id); err != nil {
		return Webhook{}, err
	}
	return s.GetWebhook(ctx, id)
}

func ValidateWebhookURL(ctx context.Context, raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return errors.New("webhook URL must use http or https and include a host")
	}
	if u.User != nil {
		return errors.New("webhook URL credentials are not allowed")
	}
	ips := []net.IP{}
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		ips = append(ips, ip)
	} else {
		resolved, err := net.DefaultResolver.LookupIP(ctx, "ip", u.Hostname())
		if err != nil {
			return errors.New("webhook URL host could not be resolved")
		}
		ips = resolved
	}
	if len(ips) == 0 {
		return errors.New("webhook URL host has no addresses")
	}
	for _, ip := range ips {
		if ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
			return errors.New("webhook URL must resolve only to public addresses")
		}
	}
	return nil
}

func (s *Store) DeleteWebhook(ctx context.Context, id string) error {
	cmd, err := s.db.Exec(ctx, `DELETE FROM webhooks WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return errors.New("webhook not found")
	}
	return nil
}
