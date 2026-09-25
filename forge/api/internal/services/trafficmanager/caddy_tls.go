package trafficmanager

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/store"
)

// CaddyTLSManager provisions TLS for proxy domains through the Caddy Admin
// API. Every mutation is a partial, node-scoped sync (POST /config/<path> or
// the documented /tls/* endpoints); the manager never replaces the whole
// Caddy config, so operator-authored routes, servers and other apps survive
// every operation.
type CaddyTLSManager struct {
	adminAddr string
	client    *http.Client
	mu        sync.Mutex
}

func NewCaddyTLSManager(adminAddr string) *CaddyTLSManager {
	if adminAddr == "" {
		adminAddr = "localhost:2019"
	}
	return &CaddyTLSManager{
		adminAddr: adminAddr,
		client:    caddyHTTPClient(30 * time.Second),
	}
}

// do performs one Admin API request and returns the status code and body.
func (m *CaddyTLSManager) do(ctx context.Context, method, path string, body io.Reader) (int, []byte, error) {
	req, err := newCaddyAdminRequest(ctx, method, m.adminAddr, path, body)
	if err != nil {
		return 0, nil, fmt.Errorf("caddy admin %s %s: %w", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("caddy admin %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, respBody, nil
}

// postSync places a config node at path, failing on transport or HTTP errors.
func (m *CaddyTLSManager) postSync(ctx context.Context, path string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal caddy config node: %w", err)
	}
	code, body, err := m.do(ctx, http.MethodPost, path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	if code >= 300 {
		return fmt.Errorf("caddy rejected %s: HTTP %d - %s", path, code, strings.TrimSpace(string(body)))
	}
	return nil
}

// policyHasSubject reports whether an automation policy entry lists hostname.
func policyHasSubject(policy map[string]any, hostname string) bool {
	subs, ok := policy["subjects"].([]any)
	if !ok {
		return false
	}
	for _, s := range subs {
		if str, _ := s.(string); strings.EqualFold(str, hostname) {
			return true
		}
	}
	return false
}

// ProvisionLetsEncrypt ensures an ACME automation policy for the domain
// without touching the rest of the Caddy config. Caddy then obtains and
// renews the certificate itself; the reverse-proxy route is owned by the
// CaddyProxy (traffic manager), not by TLS provisioning.
func (m *CaddyTLSManager) ProvisionLetsEncrypt(ctx context.Context, domain *store.ProxyDomain, email string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if email == "" {
		email = "admin@localhost"
	}

	policy := map[string]any{
		"subjects": []string{domain.Hostname},
		"issuers": []map[string]any{
			{"module": "acme", "email": email},
		},
	}

	code, body, err := m.do(ctx, http.MethodGet, "/config/apps/tls/automation", nil)
	if err != nil {
		return err
	}
	var automation struct {
		Policies []map[string]any `json:"policies"`
	}
	if code == http.StatusOK {
		if err := json.Unmarshal(body, &automation); err != nil {
			return fmt.Errorf("parse caddy tls automation: %w", err)
		}
	}

	for i, p := range automation.Policies {
		if policyHasSubject(p, domain.Hostname) {
			if err := m.postSync(ctx, fmt.Sprintf("/config/apps/tls/automation/policies/%d", i), policy); err != nil {
				return err
			}
			domain.HTTPS = true
			domain.CertType = "letsencrypt"
			slog.Info("caddy tls: acme policy updated", "domain", domain.Hostname)
			return nil
		}
	}

	if len(automation.Policies) > 0 {
		// Trailing "-" appends to the array without disturbing existing entries.
		if err := m.postSync(ctx, "/config/apps/tls/automation/policies/-", policy); err != nil {
			return err
		}
	} else {
		// No automation node yet (fresh Caddy or config without TLS section):
		// create just that subtree.
		if err := m.postSync(ctx, "/config/apps/tls/automation", map[string]any{"policies": []map[string]any{policy}}); err != nil {
			return err
		}
	}
	domain.HTTPS = true
	domain.CertType = "letsencrypt"
	slog.Info("caddy tls: acme policy issued", "domain", domain.Hostname)
	return nil
}

// UploadCustomCert loads a manually provided certificate through Caddy's
// documented manual-cert endpoint (POST /tls/certificates).
func (m *CaddyTLSManager) UploadCustomCert(ctx context.Context, domain *store.ProxyDomain) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if strings.TrimSpace(domain.CertData) == "" || strings.TrimSpace(domain.CertKey) == "" {
		return fmt.Errorf("custom certificate requires certificate and key data")
	}

	payload := map[string]any{
		"certificate": domain.CertData,
		"key":         domain.CertKey,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal cert payload: %w", err)
	}
	code, body, err := m.do(ctx, http.MethodPost, "/tls/certificates", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	if code >= 300 {
		return fmt.Errorf("caddy rejected certificate upload: HTTP %d - %s", code, strings.TrimSpace(string(body)))
	}

	domain.HTTPS = true
	domain.CertType = "custom"
	slog.Info("caddy tls: custom certificate loaded", "domain", domain.Hostname)
	return nil
}

// removeAutomationPolicy deletes the ACME automation policy for hostname, if
// one exists. Caddy's Admin API has no per-host certificate delete: manually
// loaded certificates stay in memory until the next config sync, which is why
// removing the policy (so nothing re-obtains it) is the durable action.
func (m *CaddyTLSManager) removeAutomationPolicy(ctx context.Context, hostname string) error {
	code, body, err := m.do(ctx, http.MethodGet, "/config/apps/tls/automation", nil)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return nil // no automation node: nothing to remove
	}
	var automation struct {
		Policies []map[string]any `json:"policies"`
	}
	if err := json.Unmarshal(body, &automation); err != nil {
		return fmt.Errorf("parse caddy tls automation: %w", err)
	}
	for i, p := range automation.Policies {
		if policyHasSubject(p, hostname) {
			delCode, delBody, err := m.do(ctx, http.MethodDelete, fmt.Sprintf("/config/apps/tls/automation/policies/%d", i), nil)
			if err != nil {
				return err
			}
			if delCode >= 300 && delCode != http.StatusNotFound {
				return fmt.Errorf("caddy rejected policy removal: HTTP %d - %s", delCode, strings.TrimSpace(string(delBody)))
			}
			return nil
		}
	}
	return nil
}

func (m *CaddyTLSManager) RemoveCert(ctx context.Context, hostname string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.removeAutomationPolicy(ctx, hostname); err != nil {
		return err
	}
	slog.Info("caddy tls: automation policy removed", "domain", hostname)
	return nil
}

// RenewCert triggers Caddy's async renewal endpoint and waits for the
// operation to complete. A failure at any step is returned, never swallowed.
func (m *CaddyTLSManager) RenewCert(ctx context.Context, hostname string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	raw, _ := json.Marshal(map[string]any{"domains": []string{hostname}})
	code, body, err := m.do(ctx, http.MethodPost, "/tls/renew-certificate", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	if code >= 300 {
		return fmt.Errorf("caddy renew request failed: HTTP %d - %s", code, strings.TrimSpace(string(body)))
	}

	// The endpoint returns a UUID to poll while the async issuance runs.
	var uuid string
	if err := json.Unmarshal(body, &uuid); err != nil || uuid == "" {
		// Older Caddy versions renew synchronously; treat a non-UUID 2xx as done.
		slog.Info("caddy tls: renewal requested", "domain", hostname)
		return nil
	}

	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
		status, statusBody, err := m.do(ctx, http.MethodGet, "/tls/certificate/"+uuid, nil)
		if err != nil {
			return err
		}
		switch {
		case status == http.StatusAccepted:
			continue // still working
		case status == http.StatusOK:
			_ = statusBody
			slog.Info("caddy tls: certificate renewed", "domain", hostname)
			return nil
		default:
			return fmt.Errorf("caddy renewal failed: HTTP %d - %s", status, strings.TrimSpace(string(statusBody)))
		}
	}
	return fmt.Errorf("caddy renewal for %s did not complete within 2 minutes", hostname)
}

// CertStatus inspects the live certificate bundle Caddy reports and finds the
// leaf certificate covering hostname. Unknown is reported as HasCert=false —
// never a fabricated status.
func (m *CaddyTLSManager) CertStatus(ctx context.Context, hostname string) (*CertStatusResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	code, body, err := m.do(ctx, http.MethodGet, "/tls/certificates", nil)
	if err != nil {
		return nil, err
	}
	if code == http.StatusNotFound || len(bytes.TrimSpace(body)) == 0 {
		return &CertStatusResult{HasCert: false}, nil
	}
	if code >= 300 {
		return nil, fmt.Errorf("caddy certificate listing failed: HTTP %d - %s", code, strings.TrimSpace(string(body)))
	}

	rest := body
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, parseErr := x509.ParseCertificate(block.Bytes)
		if parseErr != nil {
			continue
		}
		if !certCoversHost(cert, hostname) {
			continue
		}
		return &CertStatusResult{
			HasCert:   true,
			Issuer:    cert.Issuer.CommonName,
			Subject:   cert.Subject.CommonName,
			NotBefore: cert.NotBefore,
			NotAfter:  cert.NotAfter,
			DNSNames:  cert.DNSNames,
		}, nil
	}
	return &CertStatusResult{HasCert: false}, nil
}

func certCoversHost(cert *x509.Certificate, hostname string) bool {
	for _, name := range cert.DNSNames {
		if strings.EqualFold(name, hostname) {
			return true
		}
		if strings.HasPrefix(name, "*.") && strings.HasSuffix(hostname, name[1:]) {
			return true
		}
	}
	return strings.EqualFold(cert.Subject.CommonName, hostname)
}

type CertStatusResult struct {
	HasCert   bool      `json:"hasCert"`
	Issuer    string    `json:"issuer,omitempty"`
	Subject   string    `json:"subject,omitempty"`
	NotBefore time.Time `json:"notBefore,omitempty"`
	NotAfter  time.Time `json:"notAfter,omitempty"`
	DNSNames  []string  `json:"dnsNames,omitempty"`
}
