package trafficmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/store"
)

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
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (m *CaddyTLSManager) ProvisionLetsEncrypt(ctx context.Context, domain *store.ProxyDomain, email string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	addr := m.adminAddr
	config := m.buildTLSConfig(domain, email)
	body, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal tls config: %w", err)
	}

	if err := m.validateConfig(ctx, addr, body); err != nil {
		return fmt.Errorf("validate tls config: %w", err)
	}

	if err := m.applyConfig(ctx, addr, config); err != nil {
		return fmt.Errorf("apply tls config: %w", err)
	}

	domain.HTTPS = true
	domain.CertType = "letsencrypt"
	return nil
}

func (m *CaddyTLSManager) UploadCustomCert(ctx context.Context, domain *store.ProxyDomain) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	addr := m.adminAddr

	certPayload := map[string]any{
		"certificate": domain.CertData,
		"key":         domain.CertKey,
		"domains":     []string{domain.Hostname},
	}
	body, err := json.Marshal(certPayload)
	if err != nil {
		return fmt.Errorf("marshal cert payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("http://%s/tls/certificates/%s", addr, domain.Hostname),
		bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("upload cert request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("upload cert api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("upload cert failed: HTTP %d - %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	domain.HTTPS = true
	domain.CertType = "custom"
	return nil
}

func (m *CaddyTLSManager) RemoveCert(ctx context.Context, hostname string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	addr := m.adminAddr
	req, err := http.NewRequestWithContext(ctx, "DELETE",
		fmt.Sprintf("http://%s/tls/certificates/%s", addr, hostname), nil)
	if err != nil {
		return fmt.Errorf("remove cert request: %w", err)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("remove cert api: %w", err)
	}
	resp.Body.Close()
	return nil
}

func (m *CaddyTLSManager) RenewCert(ctx context.Context, hostname string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	addr := m.adminAddr
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("http://%s/tls/certificates/%s/renew", addr, hostname), nil)
	if err != nil {
		return fmt.Errorf("renew cert request: %w", err)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("renew cert api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("renew cert failed: HTTP %d - %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return nil
}

func (m *CaddyTLSManager) CertStatus(ctx context.Context, hostname string) (*CertStatusResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	addr := m.adminAddr
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("http://%s/tls/certificates/%s", addr, hostname), nil)
	if err != nil {
		return nil, fmt.Errorf("cert status request: %w", err)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cert status api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &CertStatusResult{HasCert: false}, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read cert status: %w", err)
	}

	var result CertStatusResult
	result.HasCert = true
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse cert status: %w", err)
	}
	return &result, nil
}

type CertStatusResult struct {
	HasCert   bool      `json:"hasCert"`
	Issuer    string    `json:"issuer,omitempty"`
	Subject   string    `json:"subject,omitempty"`
	NotBefore time.Time `json:"notBefore,omitempty"`
	NotAfter  time.Time `json:"notAfter,omitempty"`
	DNSNames  []string  `json:"dnsNames,omitempty"`
}

func (m *CaddyTLSManager) buildTLSConfig(domain *store.ProxyDomain, email string) map[string]any {
	config := map[string]any{
		"apps": map[string]any{
			"http": map[string]any{
				"servers": map[string]any{
					"srv-" + domain.Hostname: map[string]any{
						"listen": []string{":443"},
						"routes": []map[string]any{
							{
								"match": []map[string]any{
									{
										"host": []string{domain.Hostname},
									},
								},
								"handle": []map[string]any{
									{
										"handler": "reverse_proxy",
										"upstreams": []map[string]any{
											{
												"dial": fmt.Sprintf("127.0.0.1:%d", domain.Port),
											},
										},
									},
								},
							},
						},
						"tls_connection_policies": []map[string]any{
							{
								"match": map[string]any{
									"snif": []string{domain.Hostname},
								},
							},
						},
					},
				},
			},
			"tls": map[string]any{
				"automation": map[string]any{
					"policies": []map[string]any{
						{
							"subjects": []string{domain.Hostname},
							"issuer": map[string]any{
								"module": "acme",
								"email":  email,
							},
						},
					},
				},
			},
		},
	}
	return config
}

func (m *CaddyTLSManager) validateConfig(ctx context.Context, addr string, configJSON []byte) error {
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("http://%s/load", addr),
		bytes.NewReader(configJSON))
	if err != nil {
		return fmt.Errorf("validate request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("validate api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("config invalid: HTTP %d - %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return nil
}

func (m *CaddyTLSManager) applyConfig(ctx context.Context, addr string, config map[string]any) error {
	body, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal apply config: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("http://%s/config/", addr),
		bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("apply request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("apply api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("apply failed: HTTP %d - %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	slog.Info("caddy tls config applied", "hostname", config)
	return nil
}
