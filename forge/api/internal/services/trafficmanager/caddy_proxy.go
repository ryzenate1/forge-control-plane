package trafficmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/services/domains"
)

type CaddyReverseProxy struct {
	adminAddr       string
	client          *http.Client
	mu              sync.Mutex
	lastValidConfig json.RawMessage
}

func NewCaddyReverseProxy(adminAddr string) *CaddyReverseProxy {
	return &CaddyReverseProxy{
		adminAddr: adminAddr,
		client:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *CaddyReverseProxy) UpdateRoutes(ctx context.Context, rules []*RoutingRule, policies map[string]*TrafficPolicy) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	return p.updateRoutesAtomic(ctx, rules, policies)
}

func (p *CaddyReverseProxy) RemoveRoutes(ctx context.Context, ruleIDs []string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	addr := p.adminAddr
	if addr == "" {
		addr = "localhost:2019"
	}

	for _, id := range ruleIDs {
		req, err := http.NewRequestWithContext(ctx, "DELETE",
			fmt.Sprintf("http://%s/id/gamepanel-%s", addr, id),
			nil)
		if err != nil {
			return fmt.Errorf("caddy delete request: %w", err)
		}
		resp, err := p.client.Do(req)
		if err != nil {
			return fmt.Errorf("caddy delete api: %w", err)
		}
		resp.Body.Close()
	}
	return nil
}

type CertConfig struct {
	Certificate string   `json:"certificate"`
	PrivateKey  string   `json:"privateKey"`
	Domains     []string `json:"domains"`
}

func (p *CaddyReverseProxy) SetCertificate(ctx context.Context, cert CertConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	addr := p.adminAddr
	if addr == "" {
		addr = "localhost:2019"
	}

	for _, domain := range cert.Domains {
		key := fmt.Sprintf("tls/certificates/%s", domain)
		certPayload := map[string]any{
			"certificate": cert.Certificate,
			"key":         cert.PrivateKey,
		}
		body, err := json.Marshal(certPayload)
		if err != nil {
			return fmt.Errorf("marshal cert payload: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, "POST",
			fmt.Sprintf("http://%s/%s", addr, key),
			bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("set tls cert request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := p.client.Do(req)
		if err != nil {
			return fmt.Errorf("set tls cert api: %w", err)
		}
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			return fmt.Errorf("set tls cert failed: HTTP %d", resp.StatusCode)
		}
	}

	return nil
}

func (p *CaddyReverseProxy) RemoveCertificate(ctx context.Context, domains []string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	addr := p.adminAddr
	if addr == "" {
		addr = "localhost:2019"
	}

	for _, domain := range domains {
		key := fmt.Sprintf("tls/certificates/%s", domain)
		req, err := http.NewRequestWithContext(ctx, "DELETE",
			fmt.Sprintf("http://%s/%s", addr, key),
			nil)
		if err != nil {
			return fmt.Errorf("remove tls cert request: %w", err)
		}
		resp, err := p.client.Do(req)
		if err != nil {
			return fmt.Errorf("remove tls cert api: %w", err)
		}
		resp.Body.Close()
	}
	return nil
}

func (p *CaddyReverseProxy) GetActiveConnections() map[string]int {
	return make(map[string]int)
}

func (p *CaddyReverseProxy) UpdateDomainRoutes(ctx context.Context, domainRoutes []domains.VerifiedDomainRoute) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	addr := p.adminAddr
	if addr == "" {
		addr = "localhost:2019"
	}

	config := p.buildDomainConfig(addr, domainRoutes)
	body, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal domain config: %w", err)
	}

	if err := p.validateConfig(ctx, addr, body); err != nil {
		return fmt.Errorf("validate domain config: %w", err)
	}

	previousConfig, err := p.getRunningConfig(ctx, addr)
	if err != nil {
		return fmt.Errorf("snapshot running config: %w", err)
	}

	if err := p.applyConfig(ctx, addr, config); err != nil {
		if restoreErr := p.restoreConfig(ctx, addr, previousConfig); restoreErr != nil {
			return fmt.Errorf("apply domain config: %w; restore: %v", err, restoreErr)
		}
		return fmt.Errorf("apply domain config: %w", err)
	}

	return nil
}

func (p *CaddyReverseProxy) buildDomainConfig(addr string, domainRoutes []domains.VerifiedDomainRoute) map[string]any {
	routes := make([]map[string]any, 0, len(domainRoutes)+1)

	for _, dr := range domainRoutes {
		hosts := []string{dr.Domain}
		if dr.Wildcard {
			hosts = append(hosts, strings.TrimPrefix(dr.Domain, "*."))
		}
		routes = append(routes, map[string]any{
			"@id": "gamepanel-domain-" + dr.Domain,
			"match": []map[string]any{
				{
					"host": hosts,
				},
			},
			"handle": []map[string]any{
				{
					"handler": "subroute",
					"routes": []map[string]any{
						{
							"handle": []map[string]any{
								{
									"handler": "reverse_proxy",
									"upstreams": []map[string]any{
										{
											"dial": "localhost:8080",
										},
									},
									"headers": map[string]any{
										"request": map[string]any{
											"set": map[string]any{
												"X-Forwarded-Host":  []string{"{http.request.host}"},
												"X-Forwarded-Proto": []string{"{http.request.scheme}"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		})
	}

	return map[string]any{
		"apps": map[string]any{
			"http": map[string]any{
				"servers": map[string]any{
					"gamepanel-domains": map[string]any{
						"listen": []string{":80"},
						"routes": routes,
					},
				},
			},
		},
	}
}

func (p *CaddyReverseProxy) updateRoutesAtomic(ctx context.Context, rules []*RoutingRule, policies map[string]*TrafficPolicy) error {
	addr := p.adminAddr
	if addr == "" {
		addr = "localhost:2019"
	}

	routes := p.buildRoutes(rules, policies)
	serverConfig := p.buildServerConfig(routes, policies)

	body, err := json.Marshal(serverConfig)
	if err != nil {
		return fmt.Errorf("caddy marshal config: %w", err)
	}

	if err := p.validateConfig(ctx, addr, body); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	previousConfig, err := p.getRunningConfig(ctx, addr)
	if err != nil {
		return fmt.Errorf("failed to snapshot running config: %w", err)
	}
	p.lastValidConfig = previousConfig

	if err := p.applyConfig(ctx, addr, serverConfig); err != nil {
		if restoreErr := p.restoreConfig(ctx, addr, previousConfig); restoreErr != nil {
			return fmt.Errorf("apply failed: %w; restore also failed: %v", err, restoreErr)
		}
		return fmt.Errorf("apply failed, restored previous config: %w", err)
	}

	return nil
}

func (p *CaddyReverseProxy) buildServerConfig(routes []map[string]any, policies map[string]*TrafficPolicy) map[string]any {
	return map[string]any{
		"apps": map[string]any{
			"http": map[string]any{
				"servers": map[string]any{
					"gamepanel": map[string]any{
						"listen": []string{":80", ":443"},
						"routes": p.buildPolicyRoutes(routes, policies),
					},
				},
			},
		},
	}
}

func (p *CaddyReverseProxy) buildPolicyRoutes(routes []map[string]any, policies map[string]*TrafficPolicy) []map[string]any {
	var result []map[string]any

	hasHTTPSRedirect := false
	var policyHandles []map[string]any

	for _, policy := range policies {
		handles := p.buildPolicyHandles(policy)
		policyHandles = append(policyHandles, handles...)
		if policy.TLSEnabled {
			hasHTTPSRedirect = true
		}
	}

	if hasHTTPSRedirect {
		redirectRoute := p.buildHTTPSRedirectRoute()
		result = append(result, redirectRoute)
	}

	for _, route := range routes {
		enriched := p.enrichRouteWithPolicy(route, policyHandles)
		result = append(result, enriched)
	}

	return result
}

func (p *CaddyReverseProxy) buildHTTPSRedirectRoute() map[string]any {
	return map[string]any{
		"@id": "gamepanel-https-redirect",
		"match": []map[string]any{
			{
				"not": []map[string]any{
					{"protocol": "https"},
				},
			},
		},
		"handle": []map[string]any{
			{
				"handler": "static_response",
				"headers": map[string]any{
					"Location": []string{"https://{http.request.host}{http.request.uri}"},
				},
				"status_code": "301",
			},
		},
	}
}

func (p *CaddyReverseProxy) enrichRouteWithPolicy(route map[string]any, policyHandles []map[string]any) map[string]any {
	if len(policyHandles) == 0 {
		return route
	}

	existingHandle, ok := route["handle"].([]map[string]any)
	if !ok || len(existingHandle) == 0 {
		return route
	}

	combined := make([]map[string]any, 0, len(policyHandles)+len(existingHandle))
	combined = append(combined, policyHandles...)
	combined = append(combined, existingHandle...)

	result := make(map[string]any, len(route)+1)
	for k, v := range route {
		if k != "handle" {
			result[k] = v
		}
	}
	result["handle"] = combined
	return result
}

func (p *CaddyReverseProxy) buildPolicyHandles(policy *TrafficPolicy) []map[string]any {
	var handles []map[string]any

	if policy.RateLimit > 0 {
		handles = append(handles, map[string]any{
			"handler": "rate_limit",
			"rate":    fmt.Sprintf("%d/s", policy.RateLimit),
			"burst":   policy.RateLimitBurst,
		})
	}

	if len(policy.IPBlacklist) > 0 {
		handles = append(handles, map[string]any{
			"handler": "subroute",
			"routes": []map[string]any{
				{
					"match": []map[string]any{
						{
							"remote_ip": map[string]any{
								"ranges": policy.IPBlacklist,
							},
						},
					},
					"handle": []map[string]any{
						{
							"handler":     "static_response",
							"status_code": "403",
							"body":        "Forbidden",
						},
					},
				},
			},
		})
	}

	if len(policy.IPWhitelist) > 0 {
		handles = append(handles, map[string]any{
			"handler": "subroute",
			"routes": []map[string]any{
				{
					"match": []map[string]any{
						{
							"not": []map[string]any{
								{
									"remote_ip": map[string]any{
										"ranges": policy.IPWhitelist,
									},
								},
							},
						},
					},
					"handle": []map[string]any{
						{
							"handler":     "static_response",
							"status_code": "403",
							"body":        "Forbidden",
						},
					},
				},
			},
		})
	}

	if policy.CircuitBreaker {
		threshold := policy.CircuitBreakerThreshold
		if threshold == 0 {
			threshold = 5
		}
		timeout := policy.CircuitBreakerTimeout
		if timeout == 0 {
			timeout = 30
		}
		handles = append(handles, map[string]any{
			"handler":      "circuit_breaker",
			"max_failures": threshold,
			"timeout":      fmt.Sprintf("%ds", timeout),
		})
	}

	return handles
}

func (p *CaddyReverseProxy) buildRoutes(rules []*RoutingRule, policies map[string]*TrafficPolicy) []map[string]any {
	routes := make([]map[string]any, 0, len(rules))
	for _, rule := range rules {
		route := p.buildRoute(rule)
		routes = append(routes, route)
	}
	return routes
}

func (p *CaddyReverseProxy) buildRoute(rule *RoutingRule) map[string]any {
	targetHost := rule.TargetHost
	if targetHost == "" {
		targetHost = "localhost"
	}

	route := map[string]any{
		"@id": "gamepanel-" + rule.ID,
		"match": []map[string]any{
			{
				"host": []string{rule.Domain},
				"path": []string{rule.Path + "*"},
			},
		},
		"handle": []map[string]any{
			{
				"handler": "reverse_proxy",
				"upstreams": []map[string]any{
					{
						"dial": fmt.Sprintf("%s:%d", targetHost, rule.TargetPort),
					},
				},
			},
		},
	}

	if rule.WebSocket {
		route["handle"].([]map[string]any)[0]["header_up"] = map[string]string{
			"Connection": "{http.request.header.Connection}",
			"Upgrade":    "{http.request.header.Upgrade}",
		}
	}

	lbPolicy := rule.Strategy
	if lbPolicy != "" && lbPolicy != "round_robin" {
		handle := route["handle"].([]map[string]any)[0]
		handle["lb_policy"] = lbPolicy
	}

	return route
}

func (p *CaddyReverseProxy) validateConfig(ctx context.Context, addr string, configJSON []byte) error {
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("http://%s/load", addr),
		bytes.NewReader(configJSON))
	if err != nil {
		return fmt.Errorf("validate request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cache-Control", "must-revalidate")

	resp, err := p.client.Do(req)
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

func (p *CaddyReverseProxy) getRunningConfig(ctx context.Context, addr string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("http://%s/config/", addr),
		nil)
	if err != nil {
		return nil, fmt.Errorf("get config request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get config api: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	return json.RawMessage(body), nil
}

func (p *CaddyReverseProxy) applyConfig(ctx context.Context, addr string, config map[string]any) error {
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

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("apply api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("apply failed: HTTP %d - %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return nil
}

func (p *CaddyReverseProxy) restoreConfig(ctx context.Context, addr string, config json.RawMessage) error {
	if len(config) == 0 {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("http://%s/config/", addr),
		bytes.NewReader(config))
	if err != nil {
		return fmt.Errorf("restore request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("restore api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("restore failed: HTTP %d - %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return nil
}
