package trafficmanager

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gamepanel/forge/internal/services/domains"

	"gopkg.in/yaml.v3"
)

type TraefikReverseProxy struct {
	mu              sync.Mutex
	configDir       string
	adminAddr       string
	client          *http.Client
	lastValidConfig yaml.Node
	startedAt       time.Time
	errCount        int
	healthStatus    map[string]bool
}

type TraefikFileConfig struct {
	HTTP *TraefikHTTPConfig `yaml:"http,omitempty"`
	TCP  *TraefikTCPConfig  `yaml:"tcp,omitempty"`
}

type TraefikHTTPConfig struct {
	Routers     map[string]TraefikRouter     `yaml:"routers,omitempty"`
	Services    map[string]TraefikService    `yaml:"services,omitempty"`
	Middlewares map[string]TraefikMiddleware `yaml:"middlewares,omitempty"`
}

type TraefikRouter struct {
	Rule        string      `yaml:"rule"`
	Service     string      `yaml:"service"`
	EntryPoints []string    `yaml:"entryPoints"`
	Middlewares []string    `yaml:"middlewares,omitempty"`
	TLS         *TraefikTLS `yaml:"tls,omitempty"`
	Priority    int         `yaml:"priority,omitempty"`
}

type TraefikTLS struct {
	CertResolver string `yaml:"certResolver,omitempty"`
	Options      string `yaml:"options,omitempty"`
}

type TraefikService struct {
	LoadBalancer *TraefikLoadBalancer `yaml:"loadBalancer,omitempty"`
	Weighted     *TraefikWeighted     `yaml:"weighted,omitempty"`
}

type TraefikLoadBalancer struct {
	Servers            []TraefikServer            `yaml:"servers"`
	PassHostHeader     bool                       `yaml:"passHostHeader"`
	Sticky             *TraefikSticky             `yaml:"sticky,omitempty"`
	HealthCheck        *TraefikHealthCheck        `yaml:"healthCheck,omitempty"`
	ResponseForwarding *TraefikResponseForwarding `yaml:"responseForwarding,omitempty"`
}

type TraefikServer struct {
	URL    string `yaml:"url"`
	Weight int    `yaml:"weight,omitempty"`
}

type TraefikSticky struct {
	Cookie *TraefikCookie `yaml:"cookie,omitempty"`
}

type TraefikCookie struct {
	Name     string `yaml:"name,omitempty"`
	Secure   bool   `yaml:"secure,omitempty"`
	HTTPOnly bool   `yaml:"httpOnly,omitempty"`
}

type TraefikHealthCheck struct {
	Path     string `yaml:"path,omitempty"`
	Interval string `yaml:"interval,omitempty"`
	Timeout  string `yaml:"timeout,omitempty"`
}

type TraefikResponseForwarding struct {
	FlushInterval string `yaml:"flushInterval,omitempty"`
}

type TraefikWeighted struct {
	Services []TraefikWeightedService `yaml:"services"`
}

type TraefikWeightedService struct {
	Name   string `yaml:"name"`
	Weight int    `yaml:"weight"`
}

type TraefikMiddleware struct {
	AddPrefix      *TraefikAddPrefix      `yaml:"addPrefix,omitempty"`
	StripPrefix    *TraefikStripPrefix    `yaml:"stripPrefix,omitempty"`
	RateLimit      *TraefikRateLimit      `yaml:"rateLimit,omitempty"`
	IPWhiteList    *TraefikIPWhiteList    `yaml:"ipWhiteList,omitempty"`
	IPBlackList    *TraefikIPBlackList    `yaml:"ipBlackList,omitempty"`
	RedirectRegex  *TraefikRedirectRegex  `yaml:"redirectRegex,omitempty"`
	RedirectScheme *TraefikRedirectScheme `yaml:"redirectScheme,omitempty"`
	ForwardAuth    *TraefikForwardAuth    `yaml:"forwardAuth,omitempty"`
	BasicAuth      *TraefikBasicAuth      `yaml:"basicAuth,omitempty"`
	Headers        *TraefikHeaders        `yaml:"headers,omitempty"`
	CircuitBreaker *TraefikCircuitBreaker `yaml:"circuitBreaker,omitempty"`
	Retry          *TraefikRetry          `yaml:"retry,omitempty"`
	Errors         *TraefikErrors         `yaml:"errors,omitempty"`
	Compress       *TraefikCompress       `yaml:"compress,omitempty"`
	Chain          *TraefikChain          `yaml:"chain,omitempty"`
}

type TraefikAddPrefix struct {
	Prefix string `yaml:"prefix"`
}

type TraefikStripPrefix struct {
	Prefixes   []string `yaml:"prefixes"`
	ForceSlash bool     `yaml:"forceSlash,omitempty"`
}

type TraefikRateLimit struct {
	Average int    `yaml:"average"`
	Burst   int    `yaml:"burst"`
	Period  string `yaml:"period,omitempty"`
}

type TraefikIPWhiteList struct {
	SourceRange []string    `yaml:"sourceRange"`
	IPStrategy  *IPStrategy `yaml:"ipStrategy,omitempty"`
}

type TraefikIPBlackList struct {
	SourceRange []string    `yaml:"sourceRange"`
	IPStrategy  *IPStrategy `yaml:"ipStrategy,omitempty"`
}

type IPStrategy struct {
	Depth       int      `yaml:"depth,omitempty"`
	ExcludedIPs []string `yaml:"excludedIPs,omitempty"`
}

type TraefikRedirectRegex struct {
	Regex       string `yaml:"regex"`
	Replacement string `yaml:"replacement"`
	Permanent   bool   `yaml:"permanent"`
}

type TraefikRedirectScheme struct {
	Scheme    string `yaml:"scheme"`
	Port      string `yaml:"port,omitempty"`
	Permanent bool   `yaml:"permanent"`
}

type TraefikForwardAuth struct {
	Address             string   `yaml:"address"`
	TrustForwardHeader  bool     `yaml:"trustForwardHeader"`
	AuthResponseHeaders []string `yaml:"authResponseHeaders,omitempty"`
	AuthRequestHeaders  []string `yaml:"authRequestHeaders,omitempty"`
}

type TraefikBasicAuth struct {
	Users        []string `yaml:"users"`
	RemoveHeader bool     `yaml:"removeHeader"`
}

type TraefikHeaders struct {
	CustomRequestHeaders  map[string]string `yaml:"customRequestHeaders,omitempty"`
	CustomResponseHeaders map[string]string `yaml:"customResponseHeaders,omitempty"`
	SSLRedirect           bool              `yaml:"sslRedirect,omitempty"`
	STSSeconds            int               `yaml:"stsSeconds,omitempty"`
	STSIncludeSubdomains  bool              `yaml:"stsIncludeSubdomains,omitempty"`
	FrameDeny             bool              `yaml:"frameDeny,omitempty"`
	ContentTypeNosniff    bool              `yaml:"contentTypeNosniff,omitempty"`
	BrowserXSSFilter      bool              `yaml:"browserXSSFilter,omitempty"`
	ContentSecurityPolicy string            `yaml:"contentSecurityPolicy,omitempty"`
}

type TraefikCircuitBreaker struct {
	Expression string `yaml:"expression"`
}

type TraefikRetry struct {
	Attempts int `yaml:"attempts"`
}

type TraefikErrors struct {
	Status  []string `yaml:"status"`
	Service string   `yaml:"service"`
	Query   string   `yaml:"query"`
}

type TraefikCompress struct {
	ExcludedContentTypes []string `yaml:"excludedContentTypes,omitempty"`
}

type TraefikChain struct {
	Middlewares []string `yaml:"middlewares"`
}

type TraefikTCPConfig struct {
	Routers  map[string]TraefikTCPRouter `yaml:"routers,omitempty"`
	Services map[string]TraefikService   `yaml:"services,omitempty"`
}

type TraefikTCPRouter struct {
	Rule        string      `yaml:"rule"`
	Service     string      `yaml:"service"`
	EntryPoints []string    `yaml:"entryPoints"`
	Middlewares []string    `yaml:"middlewares,omitempty"`
	TLS         *TraefikTLS `yaml:"tls,omitempty"`
}

func NewTraefikReverseProxy(configDir, adminAddr string) *TraefikReverseProxy {
	if configDir == "" {
		configDir = "/etc/traefik/dynamic"
	}
	if adminAddr == "" {
		adminAddr = "localhost:8080"
	}
	return &TraefikReverseProxy{
		configDir:    configDir,
		adminAddr:    adminAddr,
		client:       &http.Client{Timeout: 10 * time.Second},
		startedAt:    time.Now(),
		healthStatus: make(map[string]bool),
	}
}

func (p *TraefikReverseProxy) Kind() AdapterKind {
	return AdapterTraefik
}

func (p *TraefikReverseProxy) UpdateRoutes(ctx context.Context, rules []*RoutingRule, policies map[string]*TrafficPolicy) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := os.MkdirAll(p.configDir, 0755); err != nil {
		return fmt.Errorf("traefik mkdir config dir: %w", err)
	}

	cfg := p.buildConfig(rules, policies)

	if err := p.validateYAMLConfig(cfg); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	previousCfg := p.loadRunningConfig()

	if err := p.writeConfig(cfg); err != nil {
		return fmt.Errorf("write traefik config: %w", err)
	}

	if err := p.reloadTraefik(ctx); err != nil {
		if restoreErr := p.restorePreviousConfig(previousCfg); restoreErr != nil {
			return fmt.Errorf("apply failed: %w; restore also failed: %v", err, restoreErr)
		}
		return fmt.Errorf("apply failed, restored previous config: %w", err)
	}

	return nil
}

func (p *TraefikReverseProxy) RemoveRoutes(ctx context.Context, ruleIDs []string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	cfg := p.loadRunningConfig()

	for _, id := range ruleIDs {
		routerKey := fmt.Sprintf("gamepanel-%s", id)
		serviceKey := fmt.Sprintf("gamepanel-svc-%s", id)

		if cfg.HTTP != nil {
			delete(cfg.HTTP.Routers, routerKey)
			delete(cfg.HTTP.Services, serviceKey)
		}
	}

	if err := p.writeConfig(cfg); err != nil {
		return fmt.Errorf("write traefik config after removal: %w", err)
	}

	return p.reloadTraefik(ctx)
}

func (p *TraefikReverseProxy) GetActiveConnections() map[string]int {
	result := make(map[string]int)

	entries, err := os.ReadDir(p.configDir)
	if err != nil {
		return result
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yml") {
			result[entry.Name()] = 0
		}
	}

	return result
}

func (p *TraefikReverseProxy) UpdateDomainRoutes(ctx context.Context, domainRoutes []domains.VerifiedDomainRoute) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := os.MkdirAll(p.configDir, 0755); err != nil {
		return fmt.Errorf("traefik mkdir config dir: %w", err)
	}

	cfg := p.loadRunningConfig()
	if cfg.HTTP == nil {
		cfg.HTTP = &TraefikHTTPConfig{
			Routers:     make(map[string]TraefikRouter),
			Services:    make(map[string]TraefikService),
			Middlewares: make(map[string]TraefikMiddleware),
		}
	}

	for _, dr := range domainRoutes {
		routerKey := fmt.Sprintf("gamepanel-domain-%s", dr.Domain)
		serviceKey := fmt.Sprintf("gamepanel-domain-svc-%s", dr.Domain)

		hostRule := fmt.Sprintf("Host(`%s`)", dr.Domain)
		if dr.Wildcard {
			baseDomain := strings.TrimPrefix(dr.Domain, "*.")
			hostRule = fmt.Sprintf("Host(`%s`) || Host(`*.%s`)", baseDomain, baseDomain)
		}

		cfg.HTTP.Routers[routerKey] = TraefikRouter{
			Rule:        hostRule,
			Service:     serviceKey,
			EntryPoints: []string{"web", "websecure"},
			Middlewares: nil,
		}

		targetHost := dr.TargetHost
		if targetHost == "" {
			targetHost = "localhost"
		}
		targetPort := dr.TargetPort
		if targetPort == 0 {
			targetPort = 8080
		}

		cfg.HTTP.Services[serviceKey] = TraefikService{
			LoadBalancer: &TraefikLoadBalancer{
				Servers: []TraefikServer{
					{URL: fmt.Sprintf("http://%s:%d", targetHost, targetPort)},
				},
				PassHostHeader: true,
			},
		}
	}

	if err := p.writeConfig(cfg); err != nil {
		return fmt.Errorf("write domain config: %w", err)
	}

	return p.reloadTraefik(ctx)
}

func (p *TraefikReverseProxy) SetCertificate(ctx context.Context, cert CertConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	tlsPath := filepath.Join(p.configDir, "tls.yml")
	tlsCfg := p.loadTLSConfig(tlsPath)

	for range cert.Domains {
		tlsCfg.TLS = append(tlsCfg.TLS, TraefikTLSCertificate{
			CertFile: cert.Certificate,
			KeyFile:  cert.PrivateKey,
		})
	}

	out, err := yaml.Marshal(tlsCfg)
	if err != nil {
		return fmt.Errorf("marshal tls config: %w", err)
	}

	if err := os.WriteFile(tlsPath, out, 0644); err != nil {
		return fmt.Errorf("write tls config: %w", err)
	}

	return p.reloadTraefik(ctx)
}

func (p *TraefikReverseProxy) RemoveCertificate(ctx context.Context, domains []string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	tlsPath := filepath.Join(p.configDir, "tls.yml")
	domainSet := make(map[string]bool, len(domains))
	for _, d := range domains {
		domainSet[d] = true
	}

	tlsCfg := p.loadTLSConfig(tlsPath)
	var filtered []TraefikTLSCertificate
	for _, c := range tlsCfg.TLS {
		keep := true
		for _, d := range domains {
			if strings.Contains(c.CertFile, d) || strings.Contains(c.KeyFile, d) {
				keep = false
				break
			}
		}
		if keep {
			filtered = append(filtered, c)
		}
	}
	tlsCfg.TLS = filtered

	out, err := yaml.Marshal(tlsCfg)
	if err != nil {
		return fmt.Errorf("marshal tls config: %w", err)
	}

	if err := os.WriteFile(tlsPath, out, 0644); err != nil {
		return fmt.Errorf("write tls config: %w", err)
	}

	return p.reloadTraefik(ctx)
}

func (p *TraefikReverseProxy) ValidateConfig(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	cfg := p.loadRunningConfig()
	return p.validateYAMLConfig(cfg)
}

func (p *TraefikReverseProxy) Reload(ctx context.Context) error {
	return p.reloadTraefik(ctx)
}

func (p *TraefikReverseProxy) Rollback(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	backupPath := filepath.Join(p.configDir, "routes.backup.yml")
	activePath := filepath.Join(p.configDir, "routes.yml")

	data, err := os.ReadFile(backupPath)
	if err != nil {
		return nil
	}
	if len(data) == 0 {
		return nil
	}

	if err := os.WriteFile(activePath, data, 0644); err != nil {
		return fmt.Errorf("rollback write: %w", err)
	}

	return p.reloadTraefik(ctx)
}

func (p *TraefikReverseProxy) CleanupStale(ctx context.Context, activeRuleIDs map[string]bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	cfg := p.loadRunningConfig()

	cleanKey := func(key, prefix string) string {
		return strings.TrimPrefix(key, prefix)
	}

	var staleRouters []string
	if cfg.HTTP != nil {
		for key := range cfg.HTTP.Routers {
			if !strings.HasPrefix(key, "gamepanel-") {
				continue
			}
			ruleID := strings.TrimPrefix(key, "gamepanel-")
			if !activeRuleIDs[ruleID] {
				staleRouters = append(staleRouters, key)
			}
		}

		for _, key := range staleRouters {
			delete(cfg.HTTP.Routers, key)
		}
	}

	var staleTCPRouters []string
	if cfg.TCP != nil {
		for key := range cfg.TCP.Routers {
			if !strings.HasPrefix(key, "gamepanel-tcp-") {
				continue
			}
			ruleID := cleanKey(key, "gamepanel-tcp-")
			if !activeRuleIDs[ruleID] {
				staleTCPRouters = append(staleTCPRouters, key)
			}
		}

		for _, key := range staleTCPRouters {
			delete(cfg.TCP.Routers, key)
		}
	}

	var staleServices []string
	if cfg.HTTP != nil {
		for key := range cfg.HTTP.Services {
			if !strings.HasPrefix(key, "gamepanel-svc-") {
				continue
			}
			svcID := strings.TrimPrefix(key, "gamepanel-svc-")
			if !activeRuleIDs[svcID] {
				staleServices = append(staleServices, key)
			}
		}

		for _, key := range staleServices {
			delete(cfg.HTTP.Services, key)
		}
	}

	var staleTCPServices []string
	if cfg.TCP != nil {
		for key := range cfg.TCP.Services {
			if !strings.HasPrefix(key, "gamepanel-tcp-svc-") {
				continue
			}
			svcID := cleanKey(key, "gamepanel-tcp-svc-")
			if !activeRuleIDs[svcID] {
				staleTCPServices = append(staleTCPServices, key)
			}
		}

		for _, key := range staleTCPServices {
			delete(cfg.TCP.Services, key)
		}
	}

	changed := len(staleRouters) > 0 || len(staleTCPRouters) > 0 ||
		len(staleServices) > 0 || len(staleTCPServices) > 0
	if !changed {
		return nil
	}

	if err := p.writeConfig(cfg); err != nil {
		return fmt.Errorf("cleanup stale: %w", err)
	}
	return p.reloadTraefik(ctx)
}

func (p *TraefikReverseProxy) Health(ctx context.Context) AdapterHealth {
	health := AdapterHealth{
		Status:   HealthHealthy,
		Uptime:   max(time.Since(p.startedAt), time.Nanosecond),
		ErrCount: p.errCount,
	}

	entries, err := os.ReadDir(p.configDir)
	if err != nil {
		health.Status = HealthDegraded
		health.Message = fmt.Sprintf("config dir not accessible: %v", err)
		return health
	}

	hasConfig := false
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yml") {
			hasConfig = true
			break
		}
	}

	if !hasConfig {
		health.Message = "no config files present"
		return health
	}

	resp, err := p.client.Get(fmt.Sprintf("http://%s/api/http/routers", p.adminAddr))
	if err != nil {
		health.Status = HealthDegraded
		health.Message = fmt.Sprintf("traefik api unreachable: %v", err)
		return health
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		health.Status = HealthHealthy
	} else {
		health.Status = HealthDegraded
		health.Message = fmt.Sprintf("traefik returned HTTP %d", resp.StatusCode)
	}

	return health
}

func (p *TraefikReverseProxy) SetUpstreamHealth(ctx context.Context, ruleID string, targetHost string, targetPort int, healthy bool) error {
	key := ruleID + "|" + targetHost + fmt.Sprintf(":%d", targetPort)
	targetURL := fmt.Sprintf("http://%s:%d", targetHost, targetPort)

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
		slog.Info("traefik upstream marked healthy", "ruleID", ruleID, "target", targetURL)
	} else {
		slog.Warn("traefik upstream marked unhealthy", "ruleID", ruleID, "target", targetURL)
	}

	cfg := p.loadRunningConfig()

	httpRouterKey := fmt.Sprintf("gamepanel-%s", ruleID)
	tcpRouterKey := fmt.Sprintf("gamepanel-tcp-%s", ruleID)

	modified := false

	if cfg.HTTP != nil {
		svcKey := ""
		if _, ok := cfg.HTTP.Routers[httpRouterKey]; ok {
			svcKey = cfg.HTTP.Routers[httpRouterKey].Service
		}
		if svcKey != "" {
			if svc, ok := cfg.HTTP.Services[svcKey]; ok && svc.LoadBalancer != nil {
				servers := svc.LoadBalancer.Servers
				var newServers []TraefikServer
				for _, s := range servers {
					if !healthy && s.URL == targetURL {
						modified = true
						continue
					}
					newServers = append(newServers, s)
				}
				if healthy && !modified && cfg.HTTP.Routers[httpRouterKey].EntryPoints[0] != "tcp" {
					newServers = append(newServers, TraefikServer{URL: targetURL})
					modified = true
				}
				svc.LoadBalancer.Servers = newServers
				cfg.HTTP.Services[svcKey] = svc
			}
		}
	}

	if cfg.TCP != nil {
		svcKey := ""
		if _, ok := cfg.TCP.Routers[tcpRouterKey]; ok {
			svcKey = cfg.TCP.Routers[tcpRouterKey].Service
		}
		if svcKey != "" {
			if svc, ok := cfg.TCP.Services[svcKey]; ok && svc.LoadBalancer != nil {
				servers := svc.LoadBalancer.Servers
				var newServers []TraefikServer
				for _, s := range servers {
					if !healthy && s.URL == targetURL {
						modified = true
						continue
					}
					newServers = append(newServers, s)
				}
				if healthy && !modified {
					newServers = append(newServers, TraefikServer{URL: targetURL})
					modified = true
				}
				svc.LoadBalancer.Servers = newServers
				cfg.TCP.Services[svcKey] = svc
			}
		}
	}

	if !modified {
		return nil
	}

	if err := p.writeConfig(cfg); err != nil {
		return fmt.Errorf("write config for health update: %w", err)
	}

	return p.reloadTraefik(ctx)
}

func (p *TraefikReverseProxy) buildConfig(rules []*RoutingRule, policies map[string]*TrafficPolicy) *TraefikFileConfig {
	cfg := &TraefikFileConfig{
		HTTP: &TraefikHTTPConfig{
			Routers:     make(map[string]TraefikRouter),
			Services:    make(map[string]TraefikService),
			Middlewares: make(map[string]TraefikMiddleware),
		},
	}

	p.buildMiddlewares(cfg, policies)

	groups := p.groupRules(rules)
	for _, grp := range groups {
		p.buildGroupedRoute(cfg, grp, policies)
	}

	return cfg
}

type traefikRuleGroup struct {
	domain   string
	path     string
	protocol string
	rules    []*RoutingRule
}

func (p *TraefikReverseProxy) groupRules(rules []*RoutingRule) []traefikRuleGroup {
	groups := make(map[string]*traefikRuleGroup)
	var order []string

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		key := rule.Domain + "|" + rule.Path + "|" + rule.Protocol
		grp, ok := groups[key]
		if !ok {
			grp = &traefikRuleGroup{
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

	result := make([]traefikRuleGroup, len(order))
	for i, key := range order {
		result[i] = *groups[key]
	}
	return result
}

func (p *TraefikReverseProxy) buildGroupedRoute(cfg *TraefikFileConfig, grp traefikRuleGroup, policies map[string]*TrafficPolicy) {
	primary := grp.rules[0]

	if primary.Protocol == "tcp" || primary.Protocol == "udp" {
		p.buildTCPGroupedRoute(cfg, grp, policies)
		return
	}

	routerKey := fmt.Sprintf("gamepanel-%s", primary.ID)
	serviceKey := fmt.Sprintf("gamepanel-svc-%s", primary.ID)

	var mws []string
	hasTLS := false

	for polID, pol := range p.collectApplicablePolicies(primary, policies) {
		mwName := fmt.Sprintf("gamepolicy-%s", polID)
		if pol.RateLimit > 0 {
			mws = append(mws, mwName+"-ratelimit")
		}
		if len(pol.IPWhitelist) > 0 {
			mws = append(mws, mwName+"-whitelist")
		}
		if len(pol.IPBlacklist) > 0 {
			mws = append(mws, mwName+"-blacklist")
		}
		if pol.CircuitBreaker {
			mws = append(mws, mwName+"-cb")
		}
		if pol.TLSEnabled {
			hasTLS = true
			mws = append(mws, mwName+"-redirect")
		}
	}

	entryPoints := []string{"web"}
	var tls *TraefikTLS
	if hasTLS {
		entryPoints = []string{"websecure"}
		tls = &TraefikTLS{}
	}

	cfg.HTTP.Routers[routerKey] = TraefikRouter{
		Rule:        fmt.Sprintf("Host(`%s`) && PathPrefix(`%s`)", grp.domain, grp.path),
		Service:     serviceKey,
		EntryPoints: entryPoints,
		Middlewares: mws,
		TLS:         tls,
	}

	servers := make([]TraefikServer, 0, len(grp.rules))
	var hasWebSocket bool
	var stickyStrategy string

	for _, rule := range grp.rules {
		targetHost := rule.TargetHost
		if targetHost == "" {
			targetHost = "localhost"
		}
		serverURL := fmt.Sprintf("http://%s:%d", targetHost, rule.TargetPort)
		server := TraefikServer{URL: serverURL}
		if rule.Weight > 0 {
			server.Weight = rule.Weight
		}
		servers = append(servers, server)

		if rule.WebSocket {
			hasWebSocket = true
		}
		if rule.Strategy != "" && rule.Strategy != "round_robin" {
			stickyStrategy = rule.Strategy
		}
	}

	svc := TraefikService{
		LoadBalancer: &TraefikLoadBalancer{
			Servers:        servers,
			PassHostHeader: true,
		},
	}

	if hasWebSocket {
		svc.LoadBalancer.ResponseForwarding = &TraefikResponseForwarding{
			FlushInterval: "0ms",
		}
	}

	if stickyStrategy != "" {
		svc.LoadBalancer.Sticky = &TraefikSticky{
			Cookie: &TraefikCookie{
				Name:     fmt.Sprintf("_gp_%s", primary.ID[:min(8, len(primary.ID))]),
				Secure:   false,
				HTTPOnly: true,
			},
		}
	}

	cfg.HTTP.Services[serviceKey] = svc
}

func (p *TraefikReverseProxy) buildTCPGroupedRoute(cfg *TraefikFileConfig, grp traefikRuleGroup, policies map[string]*TrafficPolicy) {
	primary := grp.rules[0]
	if cfg.TCP == nil {
		cfg.TCP = &TraefikTCPConfig{
			Routers:  make(map[string]TraefikTCPRouter),
			Services: make(map[string]TraefikService),
		}
	}

	tcpRouterKey := fmt.Sprintf("gamepanel-tcp-%s", primary.ID)
	tcpServiceKey := fmt.Sprintf("gamepanel-tcp-svc-%s", primary.ID)

	var mws []string

	for polID, pol := range p.collectApplicablePolicies(primary, policies) {
		mwName := fmt.Sprintf("gamepolicy-%s", polID)
		if len(pol.IPWhitelist) > 0 {
			mws = append(mws, mwName+"-whitelist")
		}
		if len(pol.IPBlacklist) > 0 {
			mws = append(mws, mwName+"-blacklist")
		}
	}

	entryPoints := []string{"tcp"}
	if primary.Protocol == "udp" {
		entryPoints = []string{"udp"}
	}

	cfg.TCP.Routers[tcpRouterKey] = TraefikTCPRouter{
		Rule:        fmt.Sprintf("HostSNI(`%s`)", primary.Domain),
		Service:     tcpServiceKey,
		EntryPoints: entryPoints,
		Middlewares: mws,
	}

	servers := make([]TraefikServer, 0, len(grp.rules))
	for _, rule := range grp.rules {
		targetHost := rule.TargetHost
		if targetHost == "" {
			targetHost = "localhost"
		}
		addr := fmt.Sprintf("%s:%d", targetHost, rule.TargetPort)
		servers = append(servers, TraefikServer{URL: addr})
	}

	cfg.TCP.Services[tcpServiceKey] = TraefikService{
		LoadBalancer: &TraefikLoadBalancer{
			Servers: servers,
		},
	}
}

func (p *TraefikReverseProxy) buildMiddlewares(cfg *TraefikFileConfig, policies map[string]*TrafficPolicy) {
	for id, policy := range policies {
		mwName := fmt.Sprintf("gamepolicy-%s", id)

		if policy.RateLimit > 0 {
			cfg.HTTP.Middlewares[mwName+"-ratelimit"] = TraefikMiddleware{
				RateLimit: &TraefikRateLimit{
					Average: policy.RateLimit,
					Burst:   policy.RateLimitBurst,
					Period:  "1s",
				},
			}
		}

		if len(policy.IPWhitelist) > 0 {
			cfg.HTTP.Middlewares[mwName+"-whitelist"] = TraefikMiddleware{
				IPWhiteList: &TraefikIPWhiteList{
					SourceRange: policy.IPWhitelist,
				},
			}
		}

		if len(policy.IPBlacklist) > 0 {
			cfg.HTTP.Middlewares[mwName+"-blacklist"] = TraefikMiddleware{
				IPBlackList: &TraefikIPBlackList{
					SourceRange: policy.IPBlacklist,
				},
			}
		}

		if policy.CircuitBreaker {
			threshold := policy.CircuitBreakerThreshold
			if threshold == 0 {
				threshold = 5
			}
			cfg.HTTP.Middlewares[mwName+"-cb"] = TraefikMiddleware{
				CircuitBreaker: &TraefikCircuitBreaker{
					Expression: fmt.Sprintf("NetworkErrorRatio() > %0.2f", 1.0/float64(threshold)),
				},
			}
		}

		if policy.TLSEnabled {
			cfg.HTTP.Middlewares[mwName+"-redirect"] = TraefikMiddleware{
				RedirectScheme: &TraefikRedirectScheme{
					Scheme:    "https",
					Permanent: true,
				},
			}
		}
	}
}

func (p *TraefikReverseProxy) collectApplicablePolicies(rule *RoutingRule, policies map[string]*TrafficPolicy) map[string]*TrafficPolicy {
	if len(policies) == 0 {
		return nil
	}
	// For now, return all policies since there is no direct rule-to-policy
	// mapping in the current data model. In the future this could filter
	// by server groups or tags.
	result := make(map[string]*TrafficPolicy, len(policies))
	for k, v := range policies {
		result[k] = v
	}
	return result
}

func (p *TraefikReverseProxy) validateYAMLConfig(cfg *TraefikFileConfig) error {
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal for validation: %w", err)
	}

	var decoded TraefikFileConfig
	if err := yaml.Unmarshal(out, &decoded); err != nil {
		return fmt.Errorf("config round-trip invalid: %w", err)
	}

	if decoded.HTTP != nil {
		for key, router := range decoded.HTTP.Routers {
			if router.Rule == "" {
				return fmt.Errorf("router %q has empty rule", key)
			}
			if router.Service == "" {
				return fmt.Errorf("router %q has empty service", key)
			}
		}
		for key, svc := range decoded.HTTP.Services {
			if svc.LoadBalancer == nil && svc.Weighted == nil {
				return fmt.Errorf("service %q has no load balancer or weighted config", key)
			}
			if svc.LoadBalancer != nil && len(svc.LoadBalancer.Servers) == 0 {
				return fmt.Errorf("service %q loadBalancer has no servers", key)
			}
		}
	}

	return nil
}

func (p *TraefikReverseProxy) loadRunningConfig() *TraefikFileConfig {
	cfg := &TraefikFileConfig{
		HTTP: &TraefikHTTPConfig{
			Routers:     make(map[string]TraefikRouter),
			Services:    make(map[string]TraefikService),
			Middlewares: make(map[string]TraefikMiddleware),
		},
	}

	configPath := filepath.Join(p.configDir, "routes.yml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return cfg
	}

	var loaded TraefikFileConfig
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		return cfg
	}

	if loaded.HTTP != nil {
		if loaded.HTTP.Routers != nil {
			cfg.HTTP.Routers = loaded.HTTP.Routers
		}
		if loaded.HTTP.Services != nil {
			cfg.HTTP.Services = loaded.HTTP.Services
		}
		if loaded.HTTP.Middlewares != nil {
			cfg.HTTP.Middlewares = loaded.HTTP.Middlewares
		}
	}
	if loaded.TCP != nil {
		cfg.TCP = loaded.TCP
	}

	return cfg
}

func (p *TraefikReverseProxy) writeConfig(cfg *TraefikFileConfig) error {
	configPath := filepath.Join(p.configDir, "routes.yml")
	backupPath := filepath.Join(p.configDir, "routes.backup.yml")

	if input, err := os.ReadFile(configPath); err == nil {
		_ = os.WriteFile(backupPath, input, 0644)
	}

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal traefik config: %w", err)
	}

	if err := os.WriteFile(configPath, out, 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	return nil
}

func (p *TraefikReverseProxy) reloadTraefik(ctx context.Context) error {
	resp, err := p.client.Post(
		fmt.Sprintf("http://%s/api/refresh", p.adminAddr),
		"application/json",
		bytes.NewReader([]byte("{}")),
	)
	if err != nil {
		p.errCount++
		return fmt.Errorf("traefik reload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		p.errCount++
		return fmt.Errorf("traefik reload failed: HTTP %d - %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

func (p *TraefikReverseProxy) restorePreviousConfig(cfg *TraefikFileConfig) error {
	if cfg == nil {
		return nil
	}

	out, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal for restore: %w", err)
	}

	configPath := filepath.Join(p.configDir, "routes.yml")
	return os.WriteFile(configPath, out, 0644)
}

type TraefikFileTLSConfig struct {
	TLS []TraefikTLSCertificate `yaml:"tls,omitempty"`
}

type TraefikTLSCertificate struct {
	CertFile string   `yaml:"certFile"`
	KeyFile  string   `yaml:"keyFile"`
	Stores   []string `yaml:"stores,omitempty"`
}

func (p *TraefikReverseProxy) loadTLSConfig(tlsPath string) TraefikFileTLSConfig {
	data, err := os.ReadFile(tlsPath)
	if err != nil {
		return TraefikFileTLSConfig{}
	}

	var cfg TraefikFileTLSConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return TraefikFileTLSConfig{}
	}

	return cfg
}

func (p *TraefikReverseProxy) writeConfigWithBackup(data []byte) error {
	configPath := filepath.Join(p.configDir, "routes.yml")

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
