// Package incus talks to the Incus REST API (https://linuxcontainers.org/incus)
// to manage system containers and virtual machines. It complements the existing
// runtimesvc.Registry and daemon.Client rather than replacing them: a Forge node
// whose runtimeProvider is "incus" is driven directly over HTTPS with a client
// TLS certificate (Incus's standard auth model), so the panel acts as a trusted
// Incus client instead of proxying through a beacon.
package incus

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/store"
)

// RuntimeProvider is the value stored on a Node.RuntimeProvider to mark it as an
// Incus host. Handlers resolve nodes by this marker.
const RuntimeProvider = "incus"

// defaultPort is Incus's REST API port (incus.localhost / remote daemon).
const defaultPort = 8443

// Instance mirrors the subset of the Incus instance API shape used by the
// panel. Field tags follow the on-the-wire JSON keys returned by /1.0/instances.
type Instance struct {
	Name     string            `json:"name"`
	Project  string            `json:"project,omitempty"`
	InstanceType string        `json:"instance_type,omitempty"`
	Status   string            `json:"status"`
	StatusCode uint            `json:"status_code,omitempty"`
	Config   map[string]string `json:"config,omitempty"`
	Devices  map[string]map[string]string `json:"devices,omitempty"`
	Profiles []string          `json:"profiles,omitempty"`
	Ephemeral bool             `json:"ephemeral,omitempty"`
	CreatedAt *time.Time       `json:"created_at,omitempty"`
	ExpandedConfig map[string]string `json:"expanded_config,omitempty"`
}

// Image mirrors the Incus image API shape (/1.0/images).
type Image struct {
	Aliases      []ImageAlias `json:"aliases,omitempty"`
	Architecture int          `json:"architecture,omitempty"`
	CreatedAt    *time.Time   `json:"created_at,omitempty"`
	ExpiresAt    *time.Time   `json:"expires_at,omitempty"`
	Fingerprint  string       `json:"fingerprint"`
	Filename     string       `json:"filename,omitempty"`
	Size         int64        `json:"size,omitempty"`
	Type         string       `json:"type,omitempty"`
	UploadedAt   *time.Time   `json:"uploaded_at,omitempty"`
	Properties   map[string]string `json:"properties,omitempty"`
}

// ImageAlias is a human-friendly name for an image fingerprint.
type ImageAlias struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Profile mirrors the Incus profile API shape (/1.0/profiles).
type Profile struct {
	Name        string                       `json:"name"`
	Description string                       `json:"description,omitempty"`
	Config      map[string]string            `json:"config,omitempty"`
	Devices     map[string]map[string]string `json:"devices,omitempty"`
}

// StoragePool mirrors the Incus storage pool API shape (/1.0/storage/pools).
type StoragePool struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Driver      string            `json:"driver,omitempty"`
	Config      map[string]string `json:"config,omitempty"`
	Status      string            `json:"status,omitempty"`
	UsedBy      []string          `json:"used_by,omitempty"`
}

// ClusterMember mirrors the Incus cluster member API shape (/1.0/cluster/members).
type ClusterMember struct {
	ServerName   string   `json:"server_name"`
	URL          string   `json:"url,omitempty"`
	Database     bool     `json:"database,omitempty"`
	Extensions   []string `json:"extensions,omitempty"`
	FailureDomain string  `json:"failure_domain,omitempty"`
	Roles        []string `json:"roles,omitempty"`
}

// Network mirrors the Incus network API shape (/1.0/networks).
type Network struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Type        string            `json:"type,omitempty"`
	Managed     bool              `json:"managed,omitempty"`
	Status      string            `json:"status,omitempty"`
	Config      map[string]string `json:"config,omitempty"`
	UsedBy      []string          `json:"used_by,omitempty"`
}

// Connection holds per-node Incus endpoint details. Credentials are supplied via
// configuration (env or SetConnection) — they are intentionally not persisted in
// clear text on the Node row.
type Connection struct {
	Host     string
	Port     int
	CertPEM  string
	KeyPEM   string
	CACertPEM string
	Token    string
	Insecure bool
}

// Service is the Incus client. It is safe for concurrent use and every method
// is nil-safe: a nil Service, or a nil Store, returns a descriptive error rather
// than panicking, so the admin routes stay registered even when Incus is unset.
type Service struct {
	store  *store.Store
	logger *slog.Logger

	mu          sync.RWMutex
	connections map[string]Connection
	clients     map[string]*http.Client

	// defaults are read from the environment at construction and used when a
	// node has no explicit override.
	defaults Connection
}

// New builds an Incus service. It is nil-safe: db may be nil (read-only,
// config-driven connections only). TLS material is sourced from the environment
// (INCUS_TLS_CERT / INCUS_TLS_KEY / INCUS_CA_CERT) unless overridden per node.
func New(db *store.Store, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		store:       db,
		logger:      log,
		connections: make(map[string]Connection),
		clients:     make(map[string]*http.Client),
		defaults: Connection{
			Port:      envInt("INCUS_PORT", defaultPort),
			CertPEM:   strings.TrimSpace(os.Getenv("INCUS_TLS_CERT")),
			KeyPEM:    strings.TrimSpace(os.Getenv("INCUS_TLS_KEY")),
			CACertPEM: strings.TrimSpace(os.Getenv("INCUS_CA_CERT")),
			Token:     strings.TrimSpace(os.Getenv("INCUS_TRUST_TOKEN")),
			Insecure:  envBool("INCUS_TLS_INSECURE"),
		},
	}
}

// SetConnection registers or replaces the Incus endpoint for a node. An empty
// field falls back to the environment-derived default at request time.
func (s *Service) SetConnection(nodeID string, c Connection) {
	if s == nil || nodeID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.connections == nil {
		s.connections = make(map[string]Connection)
	}
	s.connections[nodeID] = c
	// Invalidate any cached client for this node.
	if s.clients != nil {
		delete(s.clients, nodeID)
	}
}

// Available reports whether the service can reach a configured Incus endpoint
// (either environment credentials or at least one registered connection). Used
// by handlers to answer 503 without attempting a request.
func (s *Service) Available() bool {
	if s == nil {
		return false
	}
	if strings.TrimSpace(s.defaults.CertPEM) != "" || strings.TrimSpace(s.defaults.Token) != "" {
		return true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.connections) > 0
}

// resolveConnection builds the effective connection for a node, merging the
// per-node override with environment defaults and the Store node record (for the
// host). It errors when the node is unknown or has no host to dial.
func (s *Service) resolveConnection(ctx context.Context, nodeID string) (Connection, error) {
	if s == nil {
		return Connection{}, errors.New("incus service unavailable")
	}
	c := s.defaults
	if nodeID != "" {
		s.mu.RLock()
		if override, ok := s.connections[nodeID]; ok {
			if override.Host != "" {
				c.Host = override.Host
			}
			if override.Port != 0 {
				c.Port = override.Port
			}
			if override.CertPEM != "" {
				c.CertPEM = override.CertPEM
			}
			if override.KeyPEM != "" {
				c.KeyPEM = override.KeyPEM
			}
			if override.CACertPEM != "" {
				c.CACertPEM = override.CACertPEM
			}
			if override.Token != "" {
				c.Token = override.Token
			}
			c.Insecure = override.Insecure
		}
		s.mu.RUnlock()
	}

	// Fall back to deriving the host from the node record when not configured.
	if c.Host == "" && nodeID != "" && s.store != nil {
		node, err := s.store.GetNode(ctx, nodeID)
		if err != nil {
			return Connection{}, fmt.Errorf("resolve incus node %q: %w", nodeID, err)
		}
		host, port, err := hostFromNode(node)
		if err != nil {
			return Connection{}, err
		}
		c.Host = host
		if c.Port == 0 || c.Port == defaultPort {
			c.Port = port
		}
	}
	if c.Port == 0 {
		c.Port = defaultPort
	}
	if c.Host == "" {
		return Connection{}, errors.New("no incus host configured (set a node with runtime=incus or INCUS_HOST / SetConnection)")
	}
	return c, nil
}

// hostFromNode derives an Incus host:port from a Node's BaseURL / FQDN.
func hostFromNode(node store.Node) (string, int, error) {
	raw := strings.TrimSpace(node.BaseURL)
	if raw == "" {
		raw = strings.TrimSpace(node.FQDN)
	}
	if raw == "" {
		return "", 0, fmt.Errorf("node %q has no base URL or FQDN to reach incus", node.ID)
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", 0, fmt.Errorf("parse incus endpoint %q: %w", raw, err)
	}
	host := u.Hostname()
	port := defaultPort
	if p := u.Port(); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}
	if host == "" {
		return "", 0, fmt.Errorf("incus endpoint %q has no host", raw)
	}
	return host, port, nil
}

// httpClient builds (and caches) a TLS client-certificate http.Client for a node.
func (s *Service) httpClient(nodeID string, c Connection) (*http.Client, error) {
	s.mu.RLock()
	if cl, ok := s.clients[nodeID]; ok && cl != nil {
		s.mu.RUnlock()
		return cl, nil
	}
	s.mu.RUnlock()

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: c.Insecure}
	if c.CertPEM != "" && c.KeyPEM != "" {
		pair, err := tls.X509KeyPair([]byte(c.CertPEM), []byte(c.KeyPEM))
		if err != nil {
			return nil, fmt.Errorf("load incus client certificate: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{pair}
	}
	if c.CACertPEM != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(c.CACertPEM)) {
			return nil, errors.New("parse incus CA certificate failed")
		}
		tlsCfg.RootCAs = pool
	}
	cl := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig:     tlsCfg,
			MaxIdleConnsPerHost: 8,
			IdleConnTimeout:     60 * time.Second,
		},
	}
	s.mu.Lock()
	if s.clients == nil {
		s.clients = make(map[string]*http.Client)
	}
	s.clients[nodeID] = cl
	s.mu.Unlock()
	return cl, nil
}

// do performs a request against a node's Incus endpoint and, when out is
// non-nil, decodes the API's sync "metadata" payload into it. Operations
// (async) responses are treated as success when the HTTP status is 2xx.
func (s *Service) do(ctx context.Context, nodeID, method, path string, query url.Values, body any, out any) error {
	if s == nil {
		return errors.New("incus service unavailable")
	}
	conn, err := s.resolveConnection(ctx, nodeID)
	if err != nil {
		return err
	}
	client, err := s.httpClient(nodeID, conn)
	if err != nil {
		return err
	}

	target := fmt.Sprintf("https://%s:%d%s", conn.Host, conn.Port, path)
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		buf, merr := json.Marshal(body)
		if merr != nil {
			return fmt.Errorf("encode incus request: %w", merr)
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// Incus trust tokens are only used during the initial certificate add; the
	// client TLS certificate is the steady-state credential. We forward the
	// token as an authorization header so a configured trust can be adopted.
	if conn.Token != "" {
		req.Header.Set("Authorization", "Bearer "+conn.Token)
	}

	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("incus request %s %s: %w", method, path, err)
	}
	defer res.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("incus %s %s: status %d: %s", method, path, res.StatusCode, strings.TrimSpace(string(payload)))
	}
	if out == nil {
		return nil
	}
	// Incus responses are {"type":"sync","metadata":...} (or type 1/2 aliases).
	var envelope struct {
		Type     string          `json:"type"`
		Metadata json.RawMessage `json:"metadata"`
		Status   string          `json:"status"`
		Err      string          `json:"error"`
		Code     int             `json:"code"`
	}
	if jerr := json.Unmarshal(payload, &envelope); jerr == nil && (envelope.Metadata != nil || envelope.Type != "") {
		if envelope.Err != "" {
			return errors.New(envelope.Err)
		}
		return json.Unmarshal(envelope.Metadata, out)
	}
	return json.Unmarshal(payload, out)
}

// ListInstances returns every instance on a node.
func (s *Service) ListInstances(ctx context.Context, nodeID string) ([]Instance, error) {
	if s == nil {
		return nil, errors.New("incus service unavailable")
	}
	var out []Instance
	q := url.Values{"recursion": {"1"}, "recycle": {"1"}, "all-projects": {"true"}}
	if err := s.do(ctx, nodeID, http.MethodGet, "/1.0/instances", q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetInstance returns a single instance.
func (s *Service) GetInstance(ctx context.Context, nodeID, name string) (*Instance, error) {
	if s == nil {
		return nil, errors.New("incus service unavailable")
	}
	var out Instance
	if err := s.do(ctx, nodeID, http.MethodGet, "/1.0/instances/"+url.PathEscape(name), url.Values{"recursion": {"1"}}, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateInstance creates an instance from an Incus InstancesPost document.
func (s *Service) CreateInstance(ctx context.Context, nodeID string, spec map[string]any) error {
	if s == nil {
		return errors.New("incus service unavailable")
	}
	return s.do(ctx, nodeID, http.MethodPost, "/1.0/instances", nil, spec, nil)
}

// powerState issues an instance lifecycle action via the /state sub-resource.
func (s *Service) powerState(ctx context.Context, nodeID, name, action string, force bool) error {
	body := map[string]any{"action": action}
	if force {
		body["force"] = true
	}
	return s.do(ctx, nodeID, http.MethodPost, "/1.0/instances/"+url.PathEscape(name)+"/state", nil, body, nil)
}

// StartInstance boots a stopped instance.
func (s *Service) StartInstance(ctx context.Context, nodeID, name string) error {
	if s == nil {
		return errors.New("incus service unavailable")
	}
	return s.powerState(ctx, nodeID, name, "start", false)
}

// StopInstance stops a running instance (force triggers an immediate shutdown).
func (s *Service) StopInstance(ctx context.Context, nodeID, name string, force bool) error {
	if s == nil {
		return errors.New("incus service unavailable")
	}
	return s.powerState(ctx, nodeID, name, "stop", force)
}

// RestartInstance reboots a running instance.
func (s *Service) RestartInstance(ctx context.Context, nodeID, name string) error {
	if s == nil {
		return errors.New("incus service unavailable")
	}
	return s.powerState(ctx, nodeID, name, "restart", false)
}

// DeleteInstance removes an instance.
func (s *Service) DeleteInstance(ctx context.Context, nodeID, name string, force bool) error {
	if s == nil {
		return errors.New("incus service unavailable")
	}
	q := url.Values{}
	if force {
		q.Set("force", "true")
	}
	return s.do(ctx, nodeID, http.MethodDelete, "/1.0/instances/"+url.PathEscape(name), q, nil, nil)
}

// ListImages returns every image on a node.
func (s *Service) ListImages(ctx context.Context, nodeID string) ([]Image, error) {
	if s == nil {
		return nil, errors.New("incus service unavailable")
	}
	var out []Image
	q := url.Values{"recursion": {"1"}}
	if err := s.do(ctx, nodeID, http.MethodGet, "/1.0/images", q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListProfiles returns every profile on a node.
func (s *Service) ListProfiles(ctx context.Context, nodeID string) ([]Profile, error) {
	if s == nil {
		return nil, errors.New("incus service unavailable")
	}
	var out []Profile
	q := url.Values{"recursion": {"1"}}
	if err := s.do(ctx, nodeID, http.MethodGet, "/1.0/profiles", q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListStoragePools returns every storage pool on a node.
func (s *Service) ListStoragePools(ctx context.Context, nodeID string) ([]StoragePool, error) {
	if s == nil {
		return nil, errors.New("incus service unavailable")
	}
	var out []StoragePool
	q := url.Values{"recursion": {"1"}}
	if err := s.do(ctx, nodeID, http.MethodGet, "/1.0/storage/pools", q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListNetworks returns every managed network on a node.
func (s *Service) ListNetworks(ctx context.Context, nodeID string) ([]Network, error) {
	if s == nil {
		return nil, errors.New("incus service unavailable")
	}
	var out []Network
	q := url.Values{"recursion": {"1"}}
	if err := s.do(ctx, nodeID, http.MethodGet, "/1.0/networks", q, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListClusterMembers returns every cluster member (empty for standalone hosts).
func (s *Service) ListClusterMembers(ctx context.Context, nodeID string) ([]ClusterMember, error) {
	if s == nil {
		return nil, errors.New("incus service unavailable")
	}
	var out []ClusterMember
	if err := s.do(ctx, nodeID, http.MethodGet, "/1.0/cluster/members", url.Values{"recursion": {"1"}}, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetServerMetrics returns the server's /1.0 status document (environment,
// configuration and API extensions) as a generic map.
func (s *Service) GetServerMetrics(ctx context.Context, nodeID string) (map[string]any, error) {
	if s == nil {
		return nil, errors.New("incus service unavailable")
	}
	var out map[string]any
	if err := s.do(ctx, nodeID, http.MethodGet, "/1.0", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func envInt(key string, def int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	var v int
	if _, err := fmt.Sscanf(raw, "%d", &v); err != nil || v <= 0 {
		return def
	}
	return v
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
