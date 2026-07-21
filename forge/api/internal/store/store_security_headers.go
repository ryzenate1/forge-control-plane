package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type SecurityHeaderConfig struct {
	ID                     string            `json:"id"`
	DomainID               string            `json:"domainId"`
	HSTSEnabled            bool              `json:"hstsEnabled"`
	HSTSMaxAge             int               `json:"hstsMaxAge"`
	HSTSIncludeSubdomains  bool              `json:"hstsIncludeSubdomains"`
	HSTSPreload            bool              `json:"hstsPreload"`
	XFrameOptions          string            `json:"xFrameOptions"`
	XContentTypeOptions    string            `json:"xContentTypeOptions"`
	ReferrerPolicy         string            `json:"referrerPolicy"`
	CSPEnabled             bool              `json:"cspEnabled"`
	CSPPolicy              string            `json:"cspPolicy,omitempty"`
	PermissionsPolicy      string            `json:"permissionsPolicy,omitempty"`
	CustomHeaders          map[string]string `json:"customHeaders,omitempty"`
	CreatedAt              time.Time         `json:"createdAt"`
	UpdatedAt              time.Time         `json:"updatedAt"`
}

func (s *Store) CreateSecurityHeaders(ctx context.Context, h SecurityHeaderConfig) (*SecurityHeaderConfig, error) {
	if h.ID == "" {
		h.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	h.CreatedAt = now
	h.UpdatedAt = now

	customJSON := []byte("{}")
	if h.CustomHeaders != nil {
		customJSON, _ = json.Marshal(h.CustomHeaders)
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO security_headers (id, domain_id, hsts_enabled, hsts_max_age, hsts_include_subdomains,
		                              hsts_preload, x_frame_options, x_content_type_options, referrer_policy,
		                              csp_enabled, csp_policy, permissions_policy, custom_headers,
		                              created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
	`, h.ID, h.DomainID, h.HSTSEnabled, h.HSTSMaxAge, h.HSTSIncludeSubdomains,
		h.HSTSPreload, h.XFrameOptions, h.XContentTypeOptions, h.ReferrerPolicy,
		h.CSPEnabled, h.CSPPolicy, h.PermissionsPolicy, string(customJSON),
		h.CreatedAt, h.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

func (s *Store) GetSecurityHeaders(ctx context.Context, id string) (*SecurityHeaderConfig, error) {
	var h SecurityHeaderConfig
	var customJSON []byte

	err := s.db.QueryRow(ctx, `
		SELECT id, domain_id, hsts_enabled, hsts_max_age, hsts_include_subdomains,
		       hsts_preload, x_frame_options, x_content_type_options, referrer_policy,
		       csp_enabled, COALESCE(csp_policy,''), COALESCE(permissions_policy,''),
		       COALESCE(custom_headers,'{}'::jsonb), created_at, updated_at
		FROM security_headers WHERE id = $1
	`, id).Scan(&h.ID, &h.DomainID, &h.HSTSEnabled, &h.HSTSMaxAge, &h.HSTSIncludeSubdomains,
		&h.HSTSPreload, &h.XFrameOptions, &h.XContentTypeOptions, &h.ReferrerPolicy,
		&h.CSPEnabled, &h.CSPPolicy, &h.PermissionsPolicy,
		&customJSON, &h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if len(customJSON) > 0 {
		json.Unmarshal(customJSON, &h.CustomHeaders)
	}
	if h.CustomHeaders == nil {
		h.CustomHeaders = make(map[string]string)
	}
	return &h, nil
}

func (s *Store) GetSecurityHeadersByDomain(ctx context.Context, domainID string) (*SecurityHeaderConfig, error) {
	var h SecurityHeaderConfig
	var customJSON []byte

	err := s.db.QueryRow(ctx, `
		SELECT id, domain_id, hsts_enabled, hsts_max_age, hsts_include_subdomains,
		       hsts_preload, x_frame_options, x_content_type_options, referrer_policy,
		       csp_enabled, COALESCE(csp_policy,''), COALESCE(permissions_policy,''),
		       COALESCE(custom_headers,'{}'::jsonb), created_at, updated_at
		FROM security_headers WHERE domain_id = $1
	`, domainID).Scan(&h.ID, &h.DomainID, &h.HSTSEnabled, &h.HSTSMaxAge, &h.HSTSIncludeSubdomains,
		&h.HSTSPreload, &h.XFrameOptions, &h.XContentTypeOptions, &h.ReferrerPolicy,
		&h.CSPEnabled, &h.CSPPolicy, &h.PermissionsPolicy,
		&customJSON, &h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if len(customJSON) > 0 {
		json.Unmarshal(customJSON, &h.CustomHeaders)
	}
	if h.CustomHeaders == nil {
		h.CustomHeaders = make(map[string]string)
	}
	return &h, nil
}

func (s *Store) UpdateSecurityHeaders(ctx context.Context, h SecurityHeaderConfig) error {
	h.UpdatedAt = time.Now().UTC()

	customJSON := []byte("{}")
	if h.CustomHeaders != nil {
		customJSON, _ = json.Marshal(h.CustomHeaders)
	}

	tag, err := s.db.Exec(ctx, `
		UPDATE security_headers SET hsts_enabled=$1, hsts_max_age=$2, hsts_include_subdomains=$3,
		       hsts_preload=$4, x_frame_options=$5, x_content_type_options=$6, referrer_policy=$7,
		       csp_enabled=$8, csp_policy=$9, permissions_policy=$10, custom_headers=$11,
		       updated_at=$12
		WHERE id=$13
	`, h.HSTSEnabled, h.HSTSMaxAge, h.HSTSIncludeSubdomains, h.HSTSPreload,
		h.XFrameOptions, h.XContentTypeOptions, h.ReferrerPolicy,
		h.CSPEnabled, h.CSPPolicy, h.PermissionsPolicy, string(customJSON),
		h.UpdatedAt, h.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("security headers not found")
	}
	return nil
}

func (s *Store) DeleteSecurityHeaders(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM security_headers WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("security headers not found")
	}
	return nil
}

func (s *Store) DeleteSecurityHeadersByDomain(ctx context.Context, domainID string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM security_headers WHERE domain_id = $1`, domainID)
	return err
}
