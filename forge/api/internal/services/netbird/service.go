// Package netbird wraps the NetBird management REST API so Forge can operate
// a WireGuard-based mesh VPN control plane (peers, networks, groups, routes,
// ACLs, DNS and setup keys) alongside its own node/routing model.
package netbird

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"gamepanel/forge/internal/store"
)

// errNotConfigured is returned by mutating calls when NETBIRD_API_URL or
// NETBIRD_API_TOKEN is missing; read calls return empty results instead so
// admin dashboards render cleanly on deployments without a VPN control plane.
var errNotConfigured = errors.New("netbird management API is not configured (set NETBIRD_API_URL and NETBIRD_API_TOKEN)")

// GroupRef is the minimum group representation embedded in peer, route and
// ACL payloads (NetBird's GroupMinimum).
type GroupRef struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	PeersCount int    `json:"peers_count,omitempty"`
}

// Peer is a NetBird-registered machine on the mesh.
type Peer struct {
	ID                          string     `json:"id"`
	Name                        string     `json:"name"`
	Hostname                    string     `json:"hostname,omitempty"`
	IP                          string     `json:"ip"`
	Connected                   bool       `json:"connected"`
	OS                          string     `json:"os,omitempty"`
	Version                     string     `json:"version,omitempty"`
	UserID                      string     `json:"user_id,omitempty"`
	DnsLabel                    string     `json:"dns_label,omitempty"`
	Groups                      []GroupRef `json:"groups,omitempty"`
	LastSeen                    time.Time  `json:"last_seen,omitempty"`
	CreatedAt                   time.Time  `json:"created_at,omitempty"`
	ApprovalRequired            bool       `json:"approval_required,omitempty"`
	PendingApproval             bool       `json:"pending_approval,omitempty"`
	Ephemeral                   bool       `json:"ephemeral,omitempty"`
	SshEnabled                  bool       `json:"ssh_enabled,omitempty"`
	LoginExpired                bool       `json:"login_expired,omitempty"`
	LoginExpirationEnabled      bool       `json:"login_expiration_enabled,omitempty"`
	InactivityExpirationEnabled bool       `json:"inactivity_expiration_enabled,omitempty"`
	CountryCode                 string     `json:"country_code,omitempty"`
	CityName                    string     `json:"city_name,omitempty"`
}

// Network groups NetBird resources (peers, subnets, domains) behind a single
// access-control unit.
type Network struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	Policies          []string `json:"policies,omitempty"`
	Resources         []string `json:"resources,omitempty"`
	Routers           []string `json:"routers,omitempty"`
	RoutingPeersCount int      `json:"routing_peers_count,omitempty"`
}

// Group is a named set of peers used as ACL/route subjects.
type Group struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Peers      []string `json:"peers,omitempty"`
	PeersCount int      `json:"peers_count,omitempty"`
}

// Route publishes a CIDR (or domain list) through a peer or peer group.
type Route struct {
	ID                  string   `json:"id"`
	Description         string   `json:"description,omitempty"`
	NetworkID           string   `json:"network_id,omitempty"`
	Network             string   `json:"network,omitempty"`
	NetworkType         string   `json:"network_type,omitempty"`
	Domains             []string `json:"domains,omitempty"`
	Peer                string   `json:"peer,omitempty"`
	PeerGroups          []string `json:"peer_groups,omitempty"`
	Groups              []string `json:"groups,omitempty"`
	AccessControlGroups []string `json:"access_control_groups,omitempty"`
	Enabled             bool     `json:"enabled"`
	Metric              int      `json:"metric,omitempty"`
	Masquerade          bool     `json:"masquerade,omitempty"`
	KeepRoute           bool     `json:"keep_route,omitempty"`
}

// ACLPortRange is an inclusive port range inside an ACL sub-rule.
type ACLPortRange struct {
	From int `json:"from"`
	To   int `json:"to"`
}

// ACLSubRule is one rule inside an ACL policy.
type ACLSubRule struct {
	ID            string         `json:"id,omitempty"`
	Name          string         `json:"name,omitempty"`
	Description   string         `json:"description,omitempty"`
	Enabled       bool           `json:"enabled"`
	Action        string         `json:"action,omitempty"`
	Protocol      string         `json:"protocol,omitempty"`
	Bidirectional bool           `json:"bidirectional,omitempty"`
	Sources       []GroupRef     `json:"sources,omitempty"`
	Destinations  []GroupRef     `json:"destinations,omitempty"`
	Ports         []string       `json:"ports,omitempty"`
	PortRanges    []ACLPortRange `json:"port_ranges,omitempty"`
}

// ACLRule is a NetBird access-control policy (the modern replacement for the
// legacy /acl endpoint).
type ACLRule struct {
	ID          string       `json:"id,omitempty"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Enabled     bool         `json:"enabled"`
	Rules       []ACLSubRule `json:"rules,omitempty"`
	Revision    int64        `json:"revision,omitempty"`
}

// DNSConfig mirrors NetBird's account-level DNS settings.
type DNSConfig struct {
	DisabledManagementGroups []string `json:"disabled_management_groups"`
}

// SetupKey provisions one or many clients onto the mesh.
type SetupKey struct {
	ID                  string    `json:"id"`
	Key                 string    `json:"key,omitempty"`
	Name                string    `json:"name"`
	Type                string    `json:"type"`
	Description         string    `json:"description,omitempty"`
	Valid               bool      `json:"valid"`
	Revoked             bool      `json:"revoked"`
	State               string    `json:"state,omitempty"`
	Expires             time.Time `json:"expires,omitempty"`
	LastUsed            time.Time `json:"last_used,omitempty"`
	CreatedAt           time.Time `json:"created_at,omitempty"`
	UsedTimes           int       `json:"used_times,omitempty"`
	UsageLimit          int       `json:"usage_limit,omitempty"`
	Ephemeral           bool      `json:"ephemeral,omitempty"`
	AutoGroups          []string  `json:"auto_groups,omitempty"`
	AllowCustomName     bool      `json:"allow_custom_name,omitempty"`
	AllowExtraDnsLabels bool      `json:"allow_extra_dns_labels,omitempty"`
}

// SetupKeyRequest is the create payload for a setup key.
type SetupKeyRequest struct {
	Name            string   `json:"name"`
	Description     string   `json:"description,omitempty"`
	Type            string   `json:"type"`
	ExpiresIn       int      `json:"expires_in"`
	UsedTimes       int      `json:"used_times,omitempty"`
	AllowCustomName bool     `json:"allow_custom_name,omitempty"`
	AutoGroups      []string `json:"auto_groups,omitempty"`
	Ephemeral       bool     `json:"ephemeral,omitempty"`
}

// Service is a thin, nil-safe client for the NetBird management API.
type Service struct {
	store   *store.Store
	logger  *slog.Logger
	baseURL string
	token   string
	client  *http.Client
}

// New builds the service from NETBIRD_API_URL / NETBIRD_API_TOKEN. An
// unconfigured service is still safe to construct and call.
func New(db *store.Store, log *slog.Logger) *Service {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("NETBIRD_API_URL")), "/")
	if baseURL != "" && !strings.HasPrefix(baseURL, "http") {
		baseURL = "https://" + baseURL
	}
	return &Service{
		store:   db,
		logger:  log,
		baseURL: baseURL,
		token:   strings.TrimSpace(os.Getenv("NETBIRD_API_TOKEN")),
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

// Configured reports whether both the management URL and API token are set.
func (s *Service) Configured() bool {
	return s != nil && s.baseURL != "" && s.token != ""
}

func (s *Service) log() *slog.Logger {
	if s.logger != nil {
		return s.logger
	}
	return slog.Default()
}

// do issues a request against the NetBird management API. A non-2xx status is
// surfaced as an error carrying the response excerpt; out is left untouched on
// 204/empty bodies.
func (s *Service) do(ctx context.Context, method, path string, body, out any) error {
	if !s.Configured() {
		return errNotConfigured
	}

	var reqBody io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("netbird: encode %s %s: %w", method, path, err)
		}
		reqBody = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("netbird: build request %s %s: %w", method, path, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Token "+s.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("netbird: %s %s failed: %w", method, path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return fmt.Errorf("netbird: read %s %s response: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("netbird: %s %s returned %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil && len(bytes.TrimSpace(data)) > 0 && resp.StatusCode != http.StatusNoContent {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("netbird: decode %s %s response: %w", method, path, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Peers
// ---------------------------------------------------------------------------

// ListPeers returns every peer enrolled in the mesh, or an empty slice when
// NetBird is not configured.
func (s *Service) ListPeers(ctx context.Context) ([]Peer, error) {
	if !s.Configured() {
		return []Peer{}, nil
	}
	var peers []Peer
	if err := s.do(ctx, http.MethodGet, "/api/peers", nil, &peers); err != nil {
		return nil, err
	}
	if peers == nil {
		peers = []Peer{}
	}
	return peers, nil
}

// GetPeer fetches a single peer by ID; returns (nil, nil) when unconfigured.
func (s *Service) GetPeer(ctx context.Context, peerID string) (*Peer, error) {
	if !s.Configured() {
		return nil, nil
	}
	var peer Peer
	path := "/api/peers/" + url.PathEscape(peerID)
	if err := s.do(ctx, http.MethodGet, path, nil, &peer); err != nil {
		return nil, err
	}
	return &peer, nil
}

// updatePeerApproval flips pending_approval on a peer (NetBird's approve/deny
// mechanism on the PUT /api/peers/{id} endpoint).
func (s *Service) updatePeerApproval(ctx context.Context, peerID string, approved bool) (*Peer, error) {
	if !s.Configured() {
		return nil, errNotConfigured
	}
	path := "/api/peers/" + url.PathEscape(peerID)
	body := map[string]any{"pending_approval": !approved}
	var peer Peer
	if err := s.do(ctx, http.MethodPut, path, body, &peer); err != nil {
		return nil, err
	}
	s.log().Info("netbird peer approval updated", slog.String("peer_id", peerID), slog.Bool("approved", approved))
	return &peer, nil
}

// ApprovePeer admits a pending peer into the mesh.
func (s *Service) ApprovePeer(ctx context.Context, peerID string) (*Peer, error) {
	return s.updatePeerApproval(ctx, peerID, true)
}

// DenyPeer keeps (or pushes back) a peer in the pending-approval state.
func (s *Service) DenyPeer(ctx context.Context, peerID string) (*Peer, error) {
	return s.updatePeerApproval(ctx, peerID, false)
}

// DeletePeer removes a peer from the mesh.
func (s *Service) DeletePeer(ctx context.Context, peerID string) error {
	if !s.Configured() {
		return errNotConfigured
	}
	if err := s.do(ctx, http.MethodDelete, "/api/peers/"+url.PathEscape(peerID), nil, nil); err != nil {
		return err
	}
	s.log().Info("netbird peer deleted", slog.String("peer_id", peerID))
	return nil
}

// ---------------------------------------------------------------------------
// Networks
// ---------------------------------------------------------------------------

// ListNetworks returns all mesh networks, or an empty slice when unconfigured.
func (s *Service) ListNetworks(ctx context.Context) ([]Network, error) {
	if !s.Configured() {
		return []Network{}, nil
	}
	var networks []Network
	if err := s.do(ctx, http.MethodGet, "/api/networks", nil, &networks); err != nil {
		return nil, err
	}
	if networks == nil {
		networks = []Network{}
	}
	return networks, nil
}

// CreateNetwork posts a new network.
func (s *Service) CreateNetwork(ctx context.Context, network Network) (*Network, error) {
	if !s.Configured() {
		return nil, errNotConfigured
	}
	body := map[string]any{"name": network.Name}
	if network.Description != "" {
		body["description"] = network.Description
	}
	var created Network
	if err := s.do(ctx, http.MethodPost, "/api/networks", body, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// UpdateNetwork replaces name/description of an existing network.
func (s *Service) UpdateNetwork(ctx context.Context, networkID string, network Network) (*Network, error) {
	if !s.Configured() {
		return nil, errNotConfigured
	}
	body := map[string]any{"name": network.Name}
	if network.Description != "" {
		body["description"] = network.Description
	}
	var updated Network
	path := "/api/networks/" + url.PathEscape(networkID)
	if err := s.do(ctx, http.MethodPut, path, body, &updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

// DeleteNetwork removes a network.
func (s *Service) DeleteNetwork(ctx context.Context, networkID string) error {
	if !s.Configured() {
		return errNotConfigured
	}
	return s.do(ctx, http.MethodDelete, "/api/networks/"+url.PathEscape(networkID), nil, nil)
}

// ---------------------------------------------------------------------------
// Groups
// ---------------------------------------------------------------------------

// ListGroups returns all peer groups, or an empty slice when unconfigured.
func (s *Service) ListGroups(ctx context.Context) ([]Group, error) {
	if !s.Configured() {
		return []Group{}, nil
	}
	var groups []Group
	if err := s.do(ctx, http.MethodGet, "/api/groups", nil, &groups); err != nil {
		return nil, err
	}
	if groups == nil {
		groups = []Group{}
	}
	return groups, nil
}

// CreateGroup creates a group, optionally seeded with peer IDs.
func (s *Service) CreateGroup(ctx context.Context, name string, peers []string) (*Group, error) {
	if !s.Configured() {
		return nil, errNotConfigured
	}
	body := map[string]any{"name": name}
	if len(peers) > 0 {
		body["peers"] = peers
	}
	var group Group
	if err := s.do(ctx, http.MethodPost, "/api/groups", body, &group); err != nil {
		return nil, err
	}
	return &group, nil
}

// DeleteGroup removes a group (NetBird rejects groups still referenced by
// routes or ACLs; the upstream error is surfaced verbatim).
func (s *Service) DeleteGroup(ctx context.Context, groupID string) error {
	if !s.Configured() {
		return errNotConfigured
	}
	return s.do(ctx, http.MethodDelete, "/api/groups/"+url.PathEscape(groupID), nil, nil)
}

// ---------------------------------------------------------------------------
// Routes
// ---------------------------------------------------------------------------

// ListRoutes returns all mesh routes, or an empty slice when unconfigured.
func (s *Service) ListRoutes(ctx context.Context) ([]Route, error) {
	if !s.Configured() {
		return []Route{}, nil
	}
	var routes []Route
	if err := s.do(ctx, http.MethodGet, "/api/routes", nil, &routes); err != nil {
		return nil, err
	}
	if routes == nil {
		routes = []Route{}
	}
	return routes, nil
}

// CreateRoute publishes a subnet or domain route.
func (s *Service) CreateRoute(ctx context.Context, route Route) (*Route, error) {
	if !s.Configured() {
		return nil, errNotConfigured
	}
	var created Route
	if err := s.do(ctx, http.MethodPost, "/api/routes", route, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// DeleteRoute removes a route.
func (s *Service) DeleteRoute(ctx context.Context, routeID string) error {
	if !s.Configured() {
		return errNotConfigured
	}
	return s.do(ctx, http.MethodDelete, "/api/routes/"+url.PathEscape(routeID), nil, nil)
}

// ---------------------------------------------------------------------------
// ACLs (policies)
// ---------------------------------------------------------------------------

// ListACLs returns all access-control policies, or an empty slice when
// unconfigured.
func (s *Service) ListACLs(ctx context.Context) ([]ACLRule, error) {
	if !s.Configured() {
		return []ACLRule{}, nil
	}
	var rules []ACLRule
	if err := s.do(ctx, http.MethodGet, "/api/policies", nil, &rules); err != nil {
		return nil, err
	}
	if rules == nil {
		rules = []ACLRule{}
	}
	return rules, nil
}

// CreateACL adds an access-control policy.
func (s *Service) CreateACL(ctx context.Context, rule ACLRule) (*ACLRule, error) {
	if !s.Configured() {
		return nil, errNotConfigured
	}
	var created ACLRule
	if err := s.do(ctx, http.MethodPost, "/api/policies", rule, &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// DeleteACL removes an access-control policy.
func (s *Service) DeleteACL(ctx context.Context, aclID string) error {
	if !s.Configured() {
		return errNotConfigured
	}
	return s.do(ctx, http.MethodDelete, "/api/policies/"+url.PathEscape(aclID), nil, nil)
}

// ---------------------------------------------------------------------------
// DNS
// ---------------------------------------------------------------------------

// GetDNSSettings returns account DNS settings; (nil, nil) when unconfigured.
func (s *Service) GetDNSSettings(ctx context.Context) (*DNSConfig, error) {
	if !s.Configured() {
		return nil, nil
	}
	var cfg DNSConfig
	if err := s.do(ctx, http.MethodGet, "/api/dns/settings", nil, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// UpdateDNSSettings replaces account DNS settings.
func (s *Service) UpdateDNSSettings(ctx context.Context, cfg DNSConfig) (*DNSConfig, error) {
	if !s.Configured() {
		return nil, errNotConfigured
	}
	if cfg.DisabledManagementGroups == nil {
		cfg.DisabledManagementGroups = []string{}
	}
	var updated DNSConfig
	if err := s.do(ctx, http.MethodPut, "/api/dns/settings", cfg, &updated); err != nil {
		return nil, err
	}
	return &updated, nil
}

// ---------------------------------------------------------------------------
// Setup keys
// ---------------------------------------------------------------------------

// CreateSetupKey issues a new provisioning key. The returned key carries the
// plaintext secret exactly once (NetBird only reveals it on creation).
func (s *Service) CreateSetupKey(ctx context.Context, req SetupKeyRequest) (*SetupKey, error) {
	if !s.Configured() {
		return nil, errNotConfigured
	}
	if req.Type == "" {
		req.Type = "reusable"
	}
	var key SetupKey
	if err := s.do(ctx, http.MethodPost, "/api/setup-keys", req, &key); err != nil {
		return nil, err
	}
	s.log().Info("netbird setup key created", slog.String("key_id", key.ID), slog.String("name", key.Name))
	return &key, nil
}

// RevokeSetupKey marks a setup key revoked so it can no longer enroll peers.
func (s *Service) RevokeSetupKey(ctx context.Context, keyID string) error {
	if !s.Configured() {
		return errNotConfigured
	}
	body := map[string]any{"valid": false, "revoked": true}
	path := "/api/setup-keys/" + url.PathEscape(keyID)
	if err := s.do(ctx, http.MethodPut, path, body, nil); err != nil {
		return err
	}
	s.log().Info("netbird setup key revoked", slog.String("key_id", keyID))
	return nil
}
