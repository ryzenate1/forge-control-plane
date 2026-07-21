package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ProxyDomain struct {
	ID               string    `json:"id"`
	Hostname         string    `json:"hostname"`
	ServiceID        string    `json:"serviceId"`
	ServiceType      string    `json:"serviceType"`
	HTTPS            bool      `json:"https"`
	Port             int       `json:"port"`
	CertType         string    `json:"certType"`
	CertData         string    `json:"certData,omitempty"`
	CertKey          string    `json:"certKey,omitempty"`
	AutoRenew        bool      `json:"autoRenew"`
	Path             string    `json:"path"`
	StripPath        bool      `json:"stripPath"`
	ForwardAuthURL   string    `json:"forwardAuthUrl,omitempty"`
	ForwardAuthHeaders []map[string]string `json:"forwardAuthHeaders,omitempty"`
	WebSocket        bool      `json:"websocket"`
	RateLimit        int       `json:"rateLimit"`
	RateLimitBurst   int       `json:"rateLimitBurst"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type ProxyDomainFilter struct {
	ServiceID   *string
	ServiceType *string
	CertType    *string
	HTTPS       *bool
	Limit       int
	Offset      int
}

func (s *Store) CreateProxyDomain(ctx context.Context, d ProxyDomain) (*ProxyDomain, error) {
	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	d.CreatedAt = now
	d.UpdatedAt = now

	headersJSON := []byte("[]")
	if d.ForwardAuthHeaders != nil {
		headersJSON, _ = json.Marshal(d.ForwardAuthHeaders)
	}

	_, err := s.db.Exec(ctx, `
		INSERT INTO proxy_domains (id, hostname, service_id, service_type, https, port,
		                           cert_type, cert_data, cert_key, auto_renew, path,
		                           strip_path, forward_auth_url, forward_auth_headers,
		                           websocket, rate_limit, rate_limit_burst, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
	`, d.ID, d.Hostname, nullIfEmpty(d.ServiceID), d.ServiceType, d.HTTPS, d.Port,
		d.CertType, d.CertData, d.CertKey, d.AutoRenew, d.Path,
		d.StripPath, d.ForwardAuthURL, string(headersJSON),
		d.WebSocket, d.RateLimit, d.RateLimitBurst, d.CreatedAt, d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) GetProxyDomain(ctx context.Context, id string) (*ProxyDomain, error) {
	var d ProxyDomain
	var serviceID *string
	var headersJSON []byte

	err := s.db.QueryRow(ctx, `
		SELECT id, hostname, COALESCE(service_id,''), service_type, https, port,
		       cert_type, COALESCE(cert_data,''), COALESCE(cert_key,''), auto_renew,
		       path, strip_path, COALESCE(forward_auth_url,''), COALESCE(forward_auth_headers,'[]'::jsonb),
		       websocket, rate_limit, rate_limit_burst, created_at, updated_at
		FROM proxy_domains WHERE id = $1
	`, id).Scan(&d.ID, &d.Hostname, &serviceID, &d.ServiceType, &d.HTTPS, &d.Port,
		&d.CertType, &d.CertData, &d.CertKey, &d.AutoRenew,
		&d.Path, &d.StripPath, &d.ForwardAuthURL, &headersJSON,
		&d.WebSocket, &d.RateLimit, &d.RateLimitBurst, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	d.ServiceID = coalesceStr(serviceID)
	if len(headersJSON) > 0 {
		json.Unmarshal(headersJSON, &d.ForwardAuthHeaders)
	}
	return &d, nil
}

func (s *Store) GetProxyDomainByHostname(ctx context.Context, hostname string) (*ProxyDomain, error) {
	var d ProxyDomain
	var serviceID *string
	var headersJSON []byte

	err := s.db.QueryRow(ctx, `
		SELECT id, hostname, COALESCE(service_id,''), service_type, https, port,
		       cert_type, COALESCE(cert_data,''), COALESCE(cert_key,''), auto_renew,
		       path, strip_path, COALESCE(forward_auth_url,''), COALESCE(forward_auth_headers,'[]'::jsonb),
		       websocket, rate_limit, rate_limit_burst, created_at, updated_at
		FROM proxy_domains WHERE hostname = $1
	`, hostname).Scan(&d.ID, &d.Hostname, &serviceID, &d.ServiceType, &d.HTTPS, &d.Port,
		&d.CertType, &d.CertData, &d.CertKey, &d.AutoRenew,
		&d.Path, &d.StripPath, &d.ForwardAuthURL, &headersJSON,
		&d.WebSocket, &d.RateLimit, &d.RateLimitBurst, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	d.ServiceID = coalesceStr(serviceID)
	if len(headersJSON) > 0 {
		json.Unmarshal(headersJSON, &d.ForwardAuthHeaders)
	}
	return &d, nil
}

func (s *Store) ListProxyDomains(ctx context.Context, filter ProxyDomainFilter) ([]ProxyDomain, error) {
	query := `SELECT id, hostname, COALESCE(service_id,''), service_type, https, port,
	                 cert_type, COALESCE(cert_data,''), COALESCE(cert_key,''), auto_renew,
	                 path, strip_path, COALESCE(forward_auth_url,''), COALESCE(forward_auth_headers,'[]'::jsonb),
	                 websocket, rate_limit, rate_limit_burst, created_at, updated_at
	          FROM proxy_domains WHERE 1=1`
	args := []any{}
	argIdx := 1

	if filter.ServiceID != nil {
		query += " AND service_id = $" + itoa(argIdx)
		args = append(args, *filter.ServiceID)
		argIdx++
	}
	if filter.ServiceType != nil {
		query += " AND service_type = $" + itoa(argIdx)
		args = append(args, *filter.ServiceType)
		argIdx++
	}
	if filter.CertType != nil {
		query += " AND cert_type = $" + itoa(argIdx)
		args = append(args, *filter.CertType)
		argIdx++
	}
	if filter.HTTPS != nil {
		query += " AND https = $" + itoa(argIdx)
		args = append(args, *filter.HTTPS)
		argIdx++
	}

	query += " ORDER BY created_at DESC"

	if filter.Limit > 0 {
		query += " LIMIT $" + itoa(argIdx)
		args = append(args, filter.Limit)
		argIdx++
	}
	if filter.Offset > 0 {
		query += " OFFSET $" + itoa(argIdx)
		args = append(args, filter.Offset)
		argIdx++
	}

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	domains := make([]ProxyDomain, 0)
	for rows.Next() {
		var d ProxyDomain
		var serviceID *string
		var headersJSON []byte
		if err := rows.Scan(&d.ID, &d.Hostname, &serviceID, &d.ServiceType, &d.HTTPS, &d.Port,
			&d.CertType, &d.CertData, &d.CertKey, &d.AutoRenew,
			&d.Path, &d.StripPath, &d.ForwardAuthURL, &headersJSON,
			&d.WebSocket, &d.RateLimit, &d.RateLimitBurst, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		d.ServiceID = coalesceStr(serviceID)
		if len(headersJSON) > 0 {
			json.Unmarshal(headersJSON, &d.ForwardAuthHeaders)
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}

func (s *Store) UpdateProxyDomain(ctx context.Context, d ProxyDomain) error {
	d.UpdatedAt = time.Now().UTC()

	headersJSON := []byte("[]")
	if d.ForwardAuthHeaders != nil {
		headersJSON, _ = json.Marshal(d.ForwardAuthHeaders)
	}

	tag, err := s.db.Exec(ctx, `
		UPDATE proxy_domains SET hostname=$1, service_id=$2, service_type=$3, https=$4,
		       port=$5, cert_type=$6, cert_data=$7, cert_key=$8, auto_renew=$9,
		       path=$10, strip_path=$11, forward_auth_url=$12, forward_auth_headers=$13,
		       websocket=$14, rate_limit=$15, rate_limit_burst=$16, updated_at=$17
		WHERE id=$18
	`, d.Hostname, nullIfEmpty(d.ServiceID), d.ServiceType, d.HTTPS, d.Port,
		d.CertType, d.CertData, d.CertKey, d.AutoRenew, d.Path,
		d.StripPath, d.ForwardAuthURL, string(headersJSON),
		d.WebSocket, d.RateLimit, d.RateLimitBurst, d.UpdatedAt, d.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("proxy domain not found")
	}
	return nil
}

func (s *Store) DeleteProxyDomain(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM proxy_domains WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("proxy domain not found")
	}
	return nil
}
