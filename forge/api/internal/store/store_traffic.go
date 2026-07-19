package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type TrafficRuleRow struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	ServerID   string    `json:"serverId"`
	Domain     string    `json:"domain"`
	Path       string    `json:"path"`
	TargetPort int       `json:"targetPort"`
	Protocol   string    `json:"protocol"`
	Strategy   string    `json:"strategy"`
	Weight     int       `json:"weight"`
	Headers    []byte    `json:"headers"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type TrafficPolicyRow struct {
	ID                      string    `json:"id"`
	Name                    string    `json:"name"`
	RateLimit               int       `json:"rateLimit"`
	RateLimitBurst          int       `json:"rateLimitBurst"`
	IPWhitelist             []string  `json:"ipWhitelist"`
	IPBlacklist             []string  `json:"ipBlacklist"`
	TLSEnabled              bool      `json:"tlsEnabled"`
	TLSCertFile             string    `json:"tlsCertFile"`
	TLSKeyFile              string    `json:"tlsKeyFile"`
	CircuitBreaker          bool      `json:"circuitBreaker"`
	CircuitBreakerThreshold int       `json:"circuitBreakerThreshold"`
	CircuitBreakerTimeout   int       `json:"circuitBreakerTimeout"`
	CreatedAt               time.Time `json:"createdAt"`
	UpdatedAt               time.Time `json:"updatedAt"`
}

func (s *Store) ListTrafficRules(ctx context.Context) ([]TrafficRuleRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, COALESCE(server_id, ''), domain, COALESCE(path, '/'),
		       target_port, COALESCE(protocol, 'http'), COALESCE(strategy, 'round_robin'),
		       weight, COALESCE(headers, '{}'::jsonb), enabled, created_at, updated_at
		FROM traffic_rules
		ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := make([]TrafficRuleRow, 0)
	for rows.Next() {
		var r TrafficRuleRow
		if err := rows.Scan(&r.ID, &r.Name, &r.ServerID, &r.Domain, &r.Path,
			&r.TargetPort, &r.Protocol, &r.Strategy,
			&r.Weight, &r.Headers, &r.Enabled, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (s *Store) GetTrafficRule(ctx context.Context, id string) (*TrafficRuleRow, error) {
	var r TrafficRuleRow
	err := s.db.QueryRow(ctx, `
		SELECT id, name, COALESCE(server_id, ''), domain, COALESCE(path, '/'),
		       target_port, COALESCE(protocol, 'http'), COALESCE(strategy, 'round_robin'),
		       weight, COALESCE(headers, '{}'::jsonb), enabled, created_at, updated_at
		FROM traffic_rules
		WHERE id = $1
	`, id).Scan(&r.ID, &r.Name, &r.ServerID, &r.Domain, &r.Path,
		&r.TargetPort, &r.Protocol, &r.Strategy,
		&r.Weight, &r.Headers, &r.Enabled, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

func (s *Store) ListTrafficRulesByServer(ctx context.Context, serverID string) ([]TrafficRuleRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, COALESCE(server_id, ''), domain, COALESCE(path, '/'),
		       target_port, COALESCE(protocol, 'http'), COALESCE(strategy, 'round_robin'),
		       weight, COALESCE(headers, '{}'::jsonb), enabled, created_at, updated_at
		FROM traffic_rules
		WHERE server_id = $1
		ORDER BY created_at
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := make([]TrafficRuleRow, 0)
	for rows.Next() {
		var r TrafficRuleRow
		if err := rows.Scan(&r.ID, &r.Name, &r.ServerID, &r.Domain, &r.Path,
			&r.TargetPort, &r.Protocol, &r.Strategy,
			&r.Weight, &r.Headers, &r.Enabled, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (s *Store) CreateTrafficRule(ctx context.Context, rule TrafficRuleRow) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO traffic_rules (id, name, server_id, domain, path, target_port,
		                           protocol, strategy, weight, headers, enabled,
		                           created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, rule.ID, rule.Name, rule.ServerID, rule.Domain, rule.Path,
		rule.TargetPort, rule.Protocol, rule.Strategy,
		rule.Weight, rule.Headers, rule.Enabled,
		rule.CreatedAt, rule.UpdatedAt)
	return err
}

func (s *Store) UpdateTrafficRule(ctx context.Context, rule TrafficRuleRow) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE traffic_rules
		SET name = $1, server_id = $2, domain = $3, path = $4, target_port = $5,
		    protocol = $6, strategy = $7, weight = $8, headers = $9, enabled = $10,
		    updated_at = $11
		WHERE id = $12
	`, rule.Name, rule.ServerID, rule.Domain, rule.Path,
		rule.TargetPort, rule.Protocol, rule.Strategy,
		rule.Weight, rule.Headers, rule.Enabled,
		rule.UpdatedAt, rule.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("traffic rule not found")
	}
	return nil
}

func (s *Store) DeleteTrafficRule(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM traffic_rules WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("traffic rule not found")
	}
	return nil
}

func (s *Store) ListTrafficPolicies(ctx context.Context) ([]TrafficPolicyRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, rate_limit, rate_limit_burst,
		       COALESCE(ip_whitelist, '{}'), COALESCE(ip_blacklist, '{}'),
		       tls_enabled, COALESCE(tls_cert_file, ''), COALESCE(tls_key_file, ''),
		       circuit_breaker, circuit_breaker_threshold, circuit_breaker_timeout,
		       created_at, updated_at
		FROM traffic_policies
		ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	policies := make([]TrafficPolicyRow, 0)
	for rows.Next() {
		var p TrafficPolicyRow
		if err := rows.Scan(&p.ID, &p.Name, &p.RateLimit, &p.RateLimitBurst,
			&p.IPWhitelist, &p.IPBlacklist,
			&p.TLSEnabled, &p.TLSCertFile, &p.TLSKeyFile,
			&p.CircuitBreaker, &p.CircuitBreakerThreshold, &p.CircuitBreakerTimeout,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (s *Store) GetTrafficPolicy(ctx context.Context, id string) (*TrafficPolicyRow, error) {
	var p TrafficPolicyRow
	err := s.db.QueryRow(ctx, `
		SELECT id, name, rate_limit, rate_limit_burst,
		       COALESCE(ip_whitelist, '{}'), COALESCE(ip_blacklist, '{}'),
		       tls_enabled, COALESCE(tls_cert_file, ''), COALESCE(tls_key_file, ''),
		       circuit_breaker, circuit_breaker_threshold, circuit_breaker_timeout,
		       created_at, updated_at
		FROM traffic_policies
		WHERE id = $1
	`, id).Scan(&p.ID, &p.Name, &p.RateLimit, &p.RateLimitBurst,
		&p.IPWhitelist, &p.IPBlacklist,
		&p.TLSEnabled, &p.TLSCertFile, &p.TLSKeyFile,
		&p.CircuitBreaker, &p.CircuitBreakerThreshold, &p.CircuitBreakerTimeout,
		&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

func (s *Store) CreateTrafficPolicy(ctx context.Context, policy TrafficPolicyRow) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO traffic_policies (id, name, rate_limit, rate_limit_burst,
		                              ip_whitelist, ip_blacklist,
		                              tls_enabled, tls_cert_file, tls_key_file,
		                              circuit_breaker, circuit_breaker_threshold,
		                              circuit_breaker_timeout, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, policy.ID, policy.Name, policy.RateLimit, policy.RateLimitBurst,
		policy.IPWhitelist, policy.IPBlacklist,
		policy.TLSEnabled, policy.TLSCertFile, policy.TLSKeyFile,
		policy.CircuitBreaker, policy.CircuitBreakerThreshold, policy.CircuitBreakerTimeout,
		policy.CreatedAt, policy.UpdatedAt)
	return err
}

func (s *Store) UpdateTrafficPolicy(ctx context.Context, policy TrafficPolicyRow) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE traffic_policies
		SET name = $1, rate_limit = $2, rate_limit_burst = $3,
		    ip_whitelist = $4, ip_blacklist = $5,
		    tls_enabled = $6, tls_cert_file = $7, tls_key_file = $8,
		    circuit_breaker = $9, circuit_breaker_threshold = $10,
		    circuit_breaker_timeout = $11, updated_at = $12
		WHERE id = $13
	`, policy.Name, policy.RateLimit, policy.RateLimitBurst,
		policy.IPWhitelist, policy.IPBlacklist,
		policy.TLSEnabled, policy.TLSCertFile, policy.TLSKeyFile,
		policy.CircuitBreaker, policy.CircuitBreakerThreshold, policy.CircuitBreakerTimeout,
		policy.UpdatedAt, policy.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("traffic policy not found")
	}
	return nil
}

func (s *Store) DeleteTrafficPolicy(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM traffic_policies WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("traffic policy not found")
	}
	return nil
}
