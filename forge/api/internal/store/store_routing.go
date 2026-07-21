package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type RoutingStore interface {
	CreateRoutingRule(ctx context.Context, rule RoutingRuleRow) error
	UpdateRoutingRule(ctx context.Context, rule RoutingRuleRow) error
	DeleteRoutingRule(ctx context.Context, id string) error
	GetRoutingRule(ctx context.Context, id string) (*RoutingRuleRow, error)
	ListRoutingRules(ctx context.Context) ([]RoutingRuleRow, error)
	ListRoutingRulesForServer(ctx context.Context, serverID string) ([]RoutingRuleRow, error)
}

type RoutingRuleRow struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	ServerID   string            `json:"serverId"`
	Domain     string            `json:"domain"`
	Path       string            `json:"path"`
	TargetHost string            `json:"targetHost"`
	TargetPort int               `json:"targetPort"`
	Protocol   string            `json:"protocol"`
	Strategy   string            `json:"strategy"`
	Weight     int               `json:"weight"`
	Headers    map[string]string `json:"headers"`
	Enabled    bool              `json:"enabled"`
	WebSocket  bool              `json:"webSocketSupport"`
	CreatedAt  time.Time         `json:"createdAt"`
	UpdatedAt  time.Time         `json:"updatedAt"`
}

var _ RoutingStore = (*Store)(nil)

func (s *Store) CreateRoutingRule(ctx context.Context, rule RoutingRuleRow) error {
	headersJSON, err := json.Marshal(rule.Headers)
	if err != nil {
		return err
	}
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now().UTC()
	}
	if rule.UpdatedAt.IsZero() {
		rule.UpdatedAt = rule.CreatedAt
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO traffic_rules (id, name, server_id, domain, path, target_host, target_port,
		                           protocol, strategy, weight, headers, enabled, web_socket,
		                           created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`, rule.ID, rule.Name, nullIfEmpty(rule.ServerID), rule.Domain, rule.Path,
		rule.TargetHost, rule.TargetPort, rule.Protocol, rule.Strategy,
		rule.Weight, headersJSON, rule.Enabled, rule.WebSocket,
		rule.CreatedAt, rule.UpdatedAt)
	return err
}

func (s *Store) UpdateRoutingRule(ctx context.Context, rule RoutingRuleRow) error {
	headersJSON, err := json.Marshal(rule.Headers)
	if err != nil {
		return err
	}
	rule.UpdatedAt = time.Now().UTC()
	tag, err := s.db.Exec(ctx, `
		UPDATE traffic_rules
		SET name = $1, server_id = $2, domain = $3, path = $4, target_host = $5,
		    target_port = $6, protocol = $7, strategy = $8, weight = $9, headers = $10,
		    enabled = $11, web_socket = $12, updated_at = $13
		WHERE id = $14
	`, rule.Name, nullIfEmpty(rule.ServerID), rule.Domain, rule.Path,
		rule.TargetHost, rule.TargetPort, rule.Protocol, rule.Strategy,
		rule.Weight, headersJSON, rule.Enabled, rule.WebSocket,
		rule.UpdatedAt, rule.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("routing rule not found")
	}
	return nil
}

func (s *Store) DeleteRoutingRule(ctx context.Context, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM traffic_rules WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("routing rule not found")
	}
	return nil
}

func (s *Store) GetRoutingRule(ctx context.Context, id string) (*RoutingRuleRow, error) {
	var r RoutingRuleRow
	var serverID *string
	var headersJSON []byte
	err := s.db.QueryRow(ctx, `
		SELECT id, name, server_id, domain, COALESCE(path, '/'),
		       COALESCE(target_host, ''), target_port, COALESCE(protocol, 'http'),
		       COALESCE(strategy, 'round_robin'),
		       weight, COALESCE(headers, '{}'::jsonb), enabled,
		       COALESCE(web_socket, false), created_at, updated_at
		FROM traffic_rules
		WHERE id = $1
	`, id).Scan(&r.ID, &r.Name, &serverID, &r.Domain, &r.Path,
		&r.TargetHost, &r.TargetPort, &r.Protocol, &r.Strategy,
		&r.Weight, &headersJSON, &r.Enabled, &r.WebSocket,
		&r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	r.ServerID = coalesceStr(serverID)
	if len(headersJSON) > 0 {
		_ = json.Unmarshal(headersJSON, &r.Headers)
	}
	if r.Headers == nil {
		r.Headers = make(map[string]string)
	}
	return &r, nil
}

func (s *Store) ListRoutingRules(ctx context.Context) ([]RoutingRuleRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, server_id, domain, COALESCE(path, '/'),
		       COALESCE(target_host, ''), target_port, COALESCE(protocol, 'http'),
		       COALESCE(strategy, 'round_robin'),
		       weight, COALESCE(headers, '{}'::jsonb), enabled,
		       COALESCE(web_socket, false), created_at, updated_at
		FROM traffic_rules
		ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := make([]RoutingRuleRow, 0)
	for rows.Next() {
		var r RoutingRuleRow
		var serverID *string
		var headersJSON []byte
		if err := rows.Scan(&r.ID, &r.Name, &serverID, &r.Domain, &r.Path,
			&r.TargetHost, &r.TargetPort, &r.Protocol, &r.Strategy,
			&r.Weight, &headersJSON, &r.Enabled, &r.WebSocket,
			&r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.ServerID = coalesceStr(serverID)
		if len(headersJSON) > 0 {
			_ = json.Unmarshal(headersJSON, &r.Headers)
		}
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (s *Store) ListRoutingRulesForServer(ctx context.Context, serverID string) ([]RoutingRuleRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, server_id, domain, COALESCE(path, '/'),
		       COALESCE(target_host, ''), target_port, COALESCE(protocol, 'http'),
		       COALESCE(strategy, 'round_robin'),
		       weight, COALESCE(headers, '{}'::jsonb), enabled,
		       COALESCE(web_socket, false), created_at, updated_at
		FROM traffic_rules
		WHERE server_id = $1
		ORDER BY created_at
	`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := make([]RoutingRuleRow, 0)
	for rows.Next() {
		var r RoutingRuleRow
		var serverIDPtr *string
		var headersJSON []byte
		if err := rows.Scan(&r.ID, &r.Name, &serverIDPtr, &r.Domain, &r.Path,
			&r.TargetHost, &r.TargetPort, &r.Protocol, &r.Strategy,
			&r.Weight, &headersJSON, &r.Enabled, &r.WebSocket,
			&r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.ServerID = coalesceStr(serverIDPtr)
		if len(headersJSON) > 0 {
			_ = json.Unmarshal(headersJSON, &r.Headers)
		}
		if r.Headers == nil {
			r.Headers = make(map[string]string)
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func coalesceStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// PolicyPersistence wrapper methods to implement the trafficmanager.PolicyPersistence interface
// These delegate to the existing TrafficPolicy methods

func (s *Store) CreatePolicy(ctx context.Context, policy TrafficPolicyRow) error {
	return s.CreateTrafficPolicy(ctx, policy)
}

func (s *Store) UpdatePolicy(ctx context.Context, policy TrafficPolicyRow) error {
	return s.UpdateTrafficPolicy(ctx, policy)
}

func (s *Store) DeletePolicy(ctx context.Context, id string) error {
	return s.DeleteTrafficPolicy(ctx, id)
}

func (s *Store) GetPolicy(ctx context.Context, id string) (*TrafficPolicyRow, error) {
	return s.GetTrafficPolicy(ctx, id)
}

func (s *Store) ListPolicies(ctx context.Context) ([]TrafficPolicyRow, error) {
	return s.ListTrafficPolicies(ctx)
}
