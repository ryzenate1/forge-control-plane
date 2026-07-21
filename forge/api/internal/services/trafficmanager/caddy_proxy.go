package trafficmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
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
	healthStatus    map[string]bool
	resolver        NodeResolver
}

func (p *CaddyReverseProxy) SetNodeResolver(r NodeResolver) {
	p.resolver = r
}

func NewCaddyReverseProxy(adminAddr string) *CaddyReverseProxy {
	if adminAddr == "" {
		adminAddr = os.Getenv("CADDY_ADMIN_URL")
	}
	return &CaddyReverseProxy{
		adminAddr:    adminAddr,
		client:       &http.Client{Timeout: 10 * time.Second},
		healthStatus: make(map[string]bool),
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
	result := make(map[string]int)

	addr := p.adminAddr
	if addr == "" {
		addr = "localhost:2019"
	}

	configJSON, err := p.getRunningConfig(context.Background(), addr)
	if err != nil {
		return result
	}

	var cfg map[string]any
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return result
	}

	apps, _ := cfg["apps"].(map[string]any)
	if apps == nil {
		return result
	}
	httpCfg, _ := apps["http"].(map[string]any)
	if httpCfg == nil {
		return result
	}
	servers, _ := httpCfg["servers"].(map[string]any)
	if servers == nil {
		return result
	}

	serverNames := []string{"gamepanel", "gamepanel-domains"}
	for _, name := range serverNames {
		srv, _ := servers[name].(map[string]any)
		if srv == nil {
			continue
		}
		routesRaw, _ := srv["routes"].([]any)
		for _, rRaw := range routesRaw {
			route, _ := rRaw.(map[string]any)
			if route == nil {
				continue
			}
			rid, _ := route["@id"].(string)
			if rid == "" {
				continue
			}
			count := countUpstreamsInRoute(route)
			if count > 0 {
				result[rid] = count
			}
		}
	}

	return result
}

func countUpstreamsInRoute(route map[string]any) int {
	handlesRaw, _ := route["handle"].([]any)
	count := 0
	for _, hRaw := range handlesRaw {
		handle, _ := hRaw.(map[string]any)
		if handle == nil {
			continue
		}
		switch handler, _ := handle["handler"].(string); handler {
		case "reverse_proxy":
			upstreamsRaw, _ := handle["upstreams"].([]any)
			count += len(upstreamsRaw)
		case "subroute":
			subRoutesRaw, _ := handle["routes"].([]any)
			for _, srRaw := range subRoutesRaw {
				sr, _ := srRaw.(map[string]any)
				if sr == nil {
					continue
				}
				count += countUpstreamsInRoute(sr)
			}
		}
	}
	return count
}

func (p *CaddyReverseProxy) Kind() AdapterKind { return AdapterCaddy }

func (p *CaddyReverseProxy) ValidateConfig(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	addr := p.adminAddr
	if addr == "" {
		addr = "localhost:2019"
	}

	config, err := p.getRunningConfig(ctx, addr)
	if err != nil {
		return fmt.Errorf("get config for validation: %w", err)
	}
	if len(config) == 0 {
		return errors.New("no running config to validate")
	}

	return p.validateConfig(ctx, addr, []byte(config))
}

func (p *CaddyReverseProxy) Reload(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	addr := p.adminAddr
	if addr == "" {
		addr = "localhost:2019"
	}

	config, err := p.getRunningConfig(ctx, addr)
	if err != nil {
		return fmt.Errorf("get config for reload: %w", err)
	}
	if len(config) == 0 {
		return nil
	}

	var raw map[string]any
	if err := json.Unmarshal(config, &raw); err != nil {
		return fmt.Errorf("unmarshal config for reload: %w", err)
	}

	return p.applyConfig(ctx, addr, raw)
}

func (p *CaddyReverseProxy) Rollback(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.lastValidConfig) == 0 {
		return errors.New("no previous valid config to roll back to")
	}

	addr := p.adminAddr
	if addr == "" {
		addr = "localhost:2019"
	}

	return p.restoreConfig(ctx, addr, p.lastValidConfig)
}

func (p *CaddyReverseProxy) CleanupStale(ctx context.Context, activeRuleIDs map[string]bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	addr := p.adminAddr
	if addr == "" {
		addr = "localhost:2019"
	}

	configJSON, err := p.getRunningConfig(ctx, addr)
	if err != nil {
		return fmt.Errorf("get config for cleanup: %w", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return fmt.Errorf("unmarshal config for cleanup: %w", err)
	}

	apps, _ := cfg["apps"].(map[string]any)
	if apps == nil {
		return nil
	}
	httpCfg, _ := apps["http"].(map[string]any)
	if httpCfg == nil {
		return nil
	}
	servers, _ := httpCfg["servers"].(map[string]any)
	if servers == nil {
		return nil
	}

	modified := false
	serverNames := []string{"gamepanel", "gamepanel-domains"}
	for _, name := range serverNames {
		srv, _ := servers[name].(map[string]any)
		if srv == nil {
			continue
		}
		routesRaw, _ := srv["routes"].([]any)
		if len(routesRaw) == 0 {
			continue
		}
		var keptRoutes []any
		for _, rRaw := range routesRaw {
			route, _ := rRaw.(map[string]any)
			if route == nil {
				keptRoutes = append(keptRoutes, rRaw)
				continue
			}
			rid, _ := route["@id"].(string)
			if rid == "" || !strings.HasPrefix(rid, "gamepanel-") {
				keptRoutes = append(keptRoutes, rRaw)
				continue
			}
			if rid == "gamepanel-https-redirect" || strings.HasPrefix(rid, "gamepanel-domain-") {
				keptRoutes = append(keptRoutes, rRaw)
				continue
			}
			ruleID := strings.TrimPrefix(rid, "gamepanel-")
			if strings.HasSuffix(ruleID, "-group") {
				ruleID = strings.TrimSuffix(ruleID, "-group")
			}
			if !activeRuleIDs[ruleID] {
				modified = true
				continue
			}
			keptRoutes = append(keptRoutes, rRaw)
		}
		srv["routes"] = keptRoutes
		servers[name] = srv
	}

	if !modified {
		return nil
	}

	return p.applyConfig(ctx, addr, cfg)
}

func (p *CaddyReverseProxy) Health(ctx context.Context) AdapterHealth {
	return AdapterHealth{Status: HealthHealthy}
}

func (p *CaddyReverseProxy) SetUpstreamHealth(ctx context.Context, ruleID string, targetHost string, targetPort int, healthy bool) error {
	key := ruleID + "|" + targetHost + fmt.Sprintf(":%d", targetPort)
	targetDial := fmt.Sprintf("%s:%d", targetHost, targetPort)

	p.mu.Lock()
	prev, exists := p.healthStatus[key]
	if exists && prev == healthy {
		p.mu.Unlock()
		return nil
	}
	if healthy {
		delete(p.healthStatus, key)
	} else {
		p.healthStatus[key] = false
	}
	p.mu.Unlock()

	if healthy {
		slog.Info("caddy upstream marked healthy", "ruleID", ruleID, "target", targetDial)
	} else {
		slog.Warn("caddy upstream marked unhealthy", "ruleID", ruleID, "target", targetDial)
	}

	addr := p.adminAddr
	if addr == "" {
		addr = "localhost:2019"
	}

	configJSON, err := p.getRunningConfig(ctx, addr)
	if err != nil {
		slog.Warn("caddy not reachable for upstream health update", "error", err)
		return nil
	}

	var cfg map[string]any
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		return fmt.Errorf("unmarshal config for health update: %w", err)
	}

	modified := p.modifyCaddyUpstream(cfg, ruleID, targetDial, healthy)
	if !modified {
		return nil
	}

	return p.applyConfig(ctx, addr, cfg)
}

func (p *CaddyReverseProxy) modifyCaddyUpstream(cfg map[string]any, ruleID, targetDial string, healthy bool) bool {
	apps, _ := cfg["apps"].(map[string]any)
	if apps == nil {
		return false
	}
	httpCfg, _ := apps["http"].(map[string]any)
	if httpCfg == nil {
		return false
	}
	servers, _ := httpCfg["servers"].(map[string]any)
	if servers == nil {
		return false
	}

	modified := false
	serverNames := []string{"gamepanel", "gamepanel-domains"}
	for _, name := range serverNames {
		srv, _ := servers[name].(map[string]any)
		if srv == nil {
			continue
		}
		routesRaw, _ := srv["routes"].([]any)
		if len(routesRaw) == 0 {
			continue
		}
		for _, rRaw := range routesRaw {
			route, _ := rRaw.(map[string]any)
			if route == nil {
				continue
			}
			rid, _ := route["@id"].(string)
			if rid == "" || !strings.Contains(rid, ruleID) {
				continue
			}
			if p.modifyUpstreamsInRoute(route, targetDial, healthy) {
				modified = true
			}
		}
	}
	return modified
}

func (p *CaddyReverseProxy) modifyUpstreamsInRoute(route map[string]any, targetDial string, healthy bool) bool {
	handlesRaw, _ := route["handle"].([]any)
	if len(handlesRaw) == 0 {
		return false
	}
	modified := false
	for _, hRaw := range handlesRaw {
		handle, _ := hRaw.(map[string]any)
		if handle == nil {
			continue
		}
		if handler, _ := handle["handler"].(string); handler != "reverse_proxy" {
			continue
		}
		upstreamsRaw, _ := handle["upstreams"].([]any)
		if upstreamsRaw == nil {
			continue
		}
		var newUpstreams []any
		for _, uRaw := range upstreamsRaw {
			u, _ := uRaw.(map[string]any)
			if u == nil {
				newUpstreams = append(newUpstreams, uRaw)
				continue
			}
			dial, _ := u["dial"].(string)
			if dial == targetDial {
				if !healthy {
					modified = true
					continue
				}
			} else {
				newUpstreams = append(newUpstreams, u)
			}
		}
		if healthy {
			newUpstreams = append(newUpstreams, map[string]any{
				"dial": targetDial,
			})
			modified = true
		}
		handle["upstreams"] = newUpstreams
	}
	return modified
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
		targetHost := dr.TargetHost
		if targetHost == "" {
			targetHost = "localhost"
		}
		targetPort := dr.TargetPort
		if targetPort == 0 {
			targetPort = 8080
		}
		dial := fmt.Sprintf("%s:%d", targetHost, targetPort)
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
											"dial": dial,
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
	groups := p.groupRules(rules)
	routes := make([]map[string]any, 0, len(groups))
	for _, grp := range groups {
		route := p.buildGroupedRoute(grp)
		routes = append(routes, route)
	}
	return routes
}

type routeGroup struct {
	domain   string
	path     string
	protocol string
	rules    []*RoutingRule
}

func (p *CaddyReverseProxy) groupRules(rules []*RoutingRule) []routeGroup {
	groups := make(map[string]*routeGroup)
	var order []string

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		key := rule.Domain + "|" + rule.Path + "|" + rule.Protocol
		grp, ok := groups[key]
		if !ok {
			grp = &routeGroup{
				domain:   rule.Domain,
				path:     rule.Path,
				protocol: rule.Protocol,
				rules:    make([]*RoutingRule, 0),
			}
			groups[key] = grp
			order = append(order, key)
		}
		grp.rules = append(grp.rules, rule)
	}

	result := make([]routeGroup, len(order))
	for i, key := range order {
		result[i] = *groups[key]
	}
	return result
}

func (p *CaddyReverseProxy) buildGroupedRoute(grp routeGroup) map[string]any {
	upstreams := make([]map[string]any, 0, len(grp.rules))
	var webSocket bool
	var lbPolicy string

	for _, rule := range grp.rules {
		targetHost := rule.TargetHost
		if targetHost == "" {
			targetHost = "localhost"
		}
		upstream := map[string]any{
			"dial": fmt.Sprintf("%s:%d", targetHost, rule.TargetPort),
		}
		if rule.Weight > 0 {
			upstream["weight"] = rule.Weight
		}
		upstreams = append(upstreams, upstream)
		if rule.WebSocket {
			webSocket = true
		}
		if rule.Strategy != "" && rule.Strategy != "round_robin" {
			lbPolicy = rule.Strategy
		}
	}

	groupID := grp.rules[0].ID + "-group"
	route := map[string]any{
		"@id": "gamepanel-" + groupID,
		"match": []map[string]any{
			{
				"host": []string{grp.domain},
				"path": []string{grp.path + "*"},
			},
		},
		"handle": []map[string]any{
			{
				"handler":   "reverse_proxy",
				"upstreams": upstreams,
			},
		},
	}

	if webSocket {
		route["handle"].([]map[string]any)[0]["header_up"] = map[string]string{
			"Connection": "{http.request.header.Connection}",
			"Upgrade":    "{http.request.header.Upgrade}",
		}
	}

	if lbPolicy != "" {
		route["handle"].([]map[string]any)[0]["lb_policy"] = lbPolicy
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
