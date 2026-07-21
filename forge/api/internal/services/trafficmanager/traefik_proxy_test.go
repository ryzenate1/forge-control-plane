package trafficmanager

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gamepanel/forge/internal/services/domains"
	"gamepanel/forge/internal/store"

	"gopkg.in/yaml.v3"
)

func TestTraefik_ValidRoute(t *testing.T) {
	dir := t.TempDir()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/refresh" && r.Method == "POST" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"ok":true}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer admin.Close()

	proxy := NewTraefikReverseProxy(dir, strings.TrimPrefix(admin.URL, "http://"))

	rules := []*RoutingRule{
		{
			ID:         "r1",
			Domain:     "example.com",
			Path:       "/",
			TargetPort: 8080,
			TargetHost: "localhost",
			Enabled:    true,
			Strategy:   "round_robin",
		},
	}

	err := proxy.UpdateRoutes(context.Background(), rules, nil)
	if err != nil {
		t.Fatalf("UpdateRoutes failed: %v", err)
	}

	configPath := filepath.Join(dir, "routes.yml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}

	if !strings.Contains(string(data), "example.com") {
		t.Fatal("config missing domain")
	}
	if !strings.Contains(string(data), "localhost:8080") {
		t.Fatal("config missing target URL")
	}
}

func TestTraefik_InvalidRoute(t *testing.T) {
	dir := t.TempDir()
	proxy := NewTraefikReverseProxy(dir, "localhost:8080")

	rules := []*RoutingRule{
		{
			ID:         "bad",
			Domain:     "",
			Path:       "",
			TargetPort: 0,
			TargetHost: "",
			Enabled:    true,
		},
	}

	err := proxy.UpdateRoutes(context.Background(), rules, nil)
	if err == nil {
		t.Fatal("expected validation error for empty domain, got nil")
	}
}

func TestTraefik_WebSocket(t *testing.T) {
	dir := t.TempDir()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/refresh" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"ok":true}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer admin.Close()

	proxy := NewTraefikReverseProxy(dir, strings.TrimPrefix(admin.URL, "http://"))

	rules := []*RoutingRule{
		{
			ID:         "ws",
			Domain:     "ws.example.com",
			Path:       "/ws/",
			TargetPort: 9090,
			TargetHost: "localhost",
			WebSocket:  true,
			Enabled:    true,
		},
	}

	err := proxy.UpdateRoutes(context.Background(), rules, nil)
	if err != nil {
		t.Fatalf("UpdateRoutes failed: %v", err)
	}

	configPath := filepath.Join(dir, "routes.yml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}

	var cfg TraefikFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	svcKey := "gamepanel-svc-ws"
	svc, ok := cfg.HTTP.Services[svcKey]
	if !ok {
		t.Fatalf("service %s not found", svcKey)
	}
	if svc.LoadBalancer == nil {
		t.Fatal("service has no loadBalancer")
	}
	if svc.LoadBalancer.ResponseForwarding == nil {
		t.Fatal("websocket route missing ResponseForwarding flush interval")
	}
	if svc.LoadBalancer.ResponseForwarding.FlushInterval != "0ms" {
		t.Fatalf("expected flushInterval 0ms, got %s", svc.LoadBalancer.ResponseForwarding.FlushInterval)
	}
}

func TestTraefik_UnhealthyBackend(t *testing.T) {
	dir := t.TempDir()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer admin.Close()

	proxy := NewTraefikReverseProxy(dir, strings.TrimPrefix(admin.URL, "http://"))

	health := proxy.Health(context.Background())
	if health.Status != HealthHealthy && health.Status != HealthDegraded {
		t.Fatalf("unexpected health status: %s", health.Status)
	}
}

func TestTraefik_TwoReplicas(t *testing.T) {
	dir := t.TempDir()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer admin.Close()

	proxy := NewTraefikReverseProxy(dir, strings.TrimPrefix(admin.URL, "http://"))

	pol := &TrafficPolicy{
		ID:                      "pol-replicas",
		Name:                    "replica-policy",
		RateLimit:               100,
		RateLimitBurst:          50,
		CircuitBreaker:          true,
		CircuitBreakerThreshold: 5,
		CircuitBreakerTimeout:   30,
	}
	policies := map[string]*TrafficPolicy{"pol-replicas": pol}

	rules := []*RoutingRule{
		{
			ID:         "r-replica-1",
			Domain:     "scaled.example.com",
			Path:       "/",
			TargetPort: 8080,
			TargetHost: "10.0.0.1",
			Enabled:    true,
			Weight:     2,
		},
		{
			ID:         "r-replica-2",
			Domain:     "scaled.example.com",
			Path:       "/",
			TargetPort: 8081,
			TargetHost: "10.0.0.2",
			Enabled:    true,
			Weight:     1,
		},
	}

	err := proxy.UpdateRoutes(context.Background(), rules, policies)
	if err != nil {
		t.Fatalf("UpdateRoutes failed: %v", err)
	}

	configPath := filepath.Join(dir, "routes.yml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}

	var cfg TraefikFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if cfg.HTTP == nil {
		t.Fatal("cfg.HTTP is nil")
	}

	// Replicas correctly share one load-balanced service; routing must retain
	// both backends rather than create one Traefik service per replica.
	if len(cfg.HTTP.Services) != 1 {
		t.Fatalf("expected one load-balanced service, got %d", len(cfg.HTTP.Services))
	}
	for _, svc := range cfg.HTTP.Services {
		if svc.LoadBalancer == nil || len(svc.LoadBalancer.Servers) != 2 {
			t.Fatalf("expected two replica backends, got %+v", svc.LoadBalancer)
		}
	}
}

func TestTraefik_StaleBackend(t *testing.T) {
	dir := t.TempDir()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer admin.Close()

	proxy := NewTraefikReverseProxy(dir, strings.TrimPrefix(admin.URL, "http://"))

	err := proxy.UpdateRoutes(context.Background(), []*RoutingRule{
		{ID: "active", Domain: "active.com", Path: "/", TargetPort: 8080, TargetHost: "localhost", Enabled: true},
	}, nil)
	if err != nil {
		t.Fatalf("initial UpdateRoutes failed: %v", err)
	}

	err = proxy.CleanupStale(context.Background(), map[string]bool{"active": true})
	if err != nil {
		t.Fatalf("CleanupStale failed: %v", err)
	}

	configPath := filepath.Join(dir, "routes.yml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config file not found: %v", err)
	}

	var cfg TraefikFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if _, ok := cfg.HTTP.Routers["gamepanel-active"]; !ok {
		t.Fatal("active route was removed by cleanup")
	}

	err = proxy.CleanupStale(context.Background(), map[string]bool{})
	if err != nil {
		t.Fatalf("CleanupStale (all stale) failed: %v", err)
	}

	data2, _ := os.ReadFile(configPath)
	var cfg2 TraefikFileConfig
	yaml.Unmarshal(data2, &cfg2)

	if _, ok := cfg2.HTTP.Routers["gamepanel-active"]; ok {
		t.Fatal("stale route was not cleaned up")
	}
}

func TestTraefik_ProxyReloadFailure(t *testing.T) {
	dir := t.TempDir()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"reload failed"}`)
	}))
	defer admin.Close()

	proxy := NewTraefikReverseProxy(dir, strings.TrimPrefix(admin.URL, "http://"))

	rules := []*RoutingRule{
		{
			ID:         "fail",
			Domain:     "fail.com",
			Path:       "/",
			TargetPort: 3000,
			TargetHost: "localhost",
			Enabled:    true,
		},
	}

	err := proxy.UpdateRoutes(context.Background(), rules, nil)
	if err == nil {
		t.Fatal("expected error from reload failure")
	}
	if !strings.Contains(err.Error(), "reload failed") {
		t.Fatalf("expected reload failed error, got: %v", err)
	}
}

func TestTraefik_AdapterRestart(t *testing.T) {
	dir := t.TempDir()

	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer admin.Close()

	proxy := NewTraefikReverseProxy(dir, strings.TrimPrefix(admin.URL, "http://"))

	health := proxy.Health(context.Background())
	if health.Uptime <= 0 {
		t.Fatal("expected positive uptime")
	}

	proxy.mu.Lock()
	proxy.startedAt = time.Now()
	proxy.mu.Unlock()

	afterHealth := proxy.Health(context.Background())
	if afterHealth.Uptime <= 0 {
		t.Fatal("expected positive uptime after restart")
	}
}

func TestTraefik_Rollback(t *testing.T) {
	dir := t.TempDir()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer admin.Close()

	proxy := NewTraefikReverseProxy(dir, strings.TrimPrefix(admin.URL, "http://"))

	err := proxy.UpdateRoutes(context.Background(), []*RoutingRule{
		{ID: "v1", Domain: "v1.example.com", Path: "/", TargetPort: 8080, TargetHost: "localhost", Enabled: true},
	}, nil)
	if err != nil {
		t.Fatalf("initial update failed: %v", err)
	}

	err = proxy.Rollback(context.Background())
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}
}

func TestTraefik_CrossNodeTarget(t *testing.T) {
	dir := t.TempDir()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer admin.Close()

	proxy := NewTraefikReverseProxy(dir, strings.TrimPrefix(admin.URL, "http://"))

	rules := []*RoutingRule{
		{
			ID:         "cross",
			Domain:     "cross-node.example.com",
			Path:       "/",
			TargetPort: 25565,
			TargetHost: "node-east-1.internal",
			Enabled:    true,
		},
	}

	err := proxy.UpdateRoutes(context.Background(), rules, nil)
	if err != nil {
		t.Fatalf("UpdateRoutes failed: %v", err)
	}

	configPath := filepath.Join(dir, "routes.yml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config file not written: %v", err)
	}

	var cfg TraefikFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	svcKey := "gamepanel-svc-cross"
	svc, ok := cfg.HTTP.Services[svcKey]
	if !ok {
		t.Fatalf("service %s not found", svcKey)
	}
	if len(svc.LoadBalancer.Servers) == 0 {
		t.Fatal("no servers in load balancer")
	}
	if !strings.Contains(svc.LoadBalancer.Servers[0].URL, "node-east-1.internal:25565") {
		t.Fatalf("expected cross-node target, got %s", svc.LoadBalancer.Servers[0].URL)
	}
}

func TestTraefik_MiddlewareRendering_RateLimit(t *testing.T) {
	dir := t.TempDir()
	proxy := NewTraefikReverseProxy(dir, "localhost:8080")

	policy := &TrafficPolicy{
		ID:             "pol1",
		Name:           "RateLimitPolicy",
		RateLimit:      100,
		RateLimitBurst: 50,
	}
	policies := map[string]*TrafficPolicy{"pol1": policy}

	rules := []*RoutingRule{
		{
			ID:         "r3",
			Domain:     "ratelimited.com",
			Path:       "/api/",
			TargetPort: 8080,
			TargetHost: "localhost",
			Enabled:    true,
		},
	}

	cfg := proxy.buildConfig(rules, policies)
	data, _ := yaml.Marshal(cfg)
	cfgStr := string(data)

	if !strings.Contains(cfgStr, "rateLimit") {
		t.Fatal("config missing rateLimit middleware")
	}
	if !strings.Contains(cfgStr, "average: 100") {
		t.Fatal("config missing expected rate 100")
	}
	if !strings.Contains(cfgStr, "burst: 50") {
		t.Fatal("config missing expected burst 50")
	}
}

func TestTraefik_MiddlewareRendering_IPWhitelist(t *testing.T) {
	dir := t.TempDir()
	proxy := NewTraefikReverseProxy(dir, "localhost:8080")

	policy := &TrafficPolicy{
		ID:          "pol2",
		Name:        "IPWhitelist",
		IPWhitelist: []string{"10.0.0.0/8", "192.168.0.1"},
	}
	policies := map[string]*TrafficPolicy{"pol2": policy}

	rules := []*RoutingRule{
		{
			ID:         "r4",
			Domain:     "whitelist.com",
			Path:       "/",
			TargetPort: 3000,
			TargetHost: "localhost",
			Enabled:    true,
		},
	}

	cfg := proxy.buildConfig(rules, policies)
	data, _ := yaml.Marshal(cfg)
	cfgStr := string(data)

	if !strings.Contains(cfgStr, "10.0.0.0/8") {
		t.Fatal("config missing IP whitelist range 10.0.0.0/8")
	}
	if !strings.Contains(cfgStr, "192.168.0.1") {
		t.Fatal("config missing IP whitelist 192.168.0.1")
	}
	if !strings.Contains(cfgStr, "ipWhiteList") {
		t.Fatal("config missing ipWhiteList middleware")
	}
}

func TestTraefik_MiddlewareRendering_IPBlacklist(t *testing.T) {
	dir := t.TempDir()
	proxy := NewTraefikReverseProxy(dir, "localhost:8080")

	policy := &TrafficPolicy{
		ID:          "pol3",
		Name:        "IPBlacklist",
		IPBlacklist: []string{"1.2.3.4", "5.6.7.0/24"},
	}
	policies := map[string]*TrafficPolicy{"pol3": policy}

	rules := []*RoutingRule{
		{
			ID:         "r5",
			Domain:     "blacklist.com",
			Path:       "/",
			TargetPort: 5000,
			TargetHost: "localhost",
			Enabled:    true,
		},
	}

	cfg := proxy.buildConfig(rules, policies)
	data, _ := yaml.Marshal(cfg)
	cfgStr := string(data)

	if !strings.Contains(cfgStr, "1.2.3.4") {
		t.Fatal("config missing IP blacklist entry 1.2.3.4")
	}
	if !strings.Contains(cfgStr, "5.6.7.0/24") {
		t.Fatal("config missing IP blacklist range 5.6.7.0/24")
	}
}

func TestTraefik_MiddlewareRendering_HTTPSRedirect(t *testing.T) {
	dir := t.TempDir()
	proxy := NewTraefikReverseProxy(dir, "localhost:8080")

	policy := &TrafficPolicy{
		ID:         "pol4",
		Name:       "TLSEnabled",
		TLSEnabled: true,
	}
	policies := map[string]*TrafficPolicy{"pol4": policy}

	rules := []*RoutingRule{
		{
			ID:         "r6",
			Domain:     "tls-site.com",
			Path:       "/",
			TargetPort: 443,
			TargetHost: "localhost",
			Enabled:    true,
		},
	}

	cfg := proxy.buildConfig(rules, policies)
	data, _ := yaml.Marshal(cfg)
	cfgStr := string(data)

	if !strings.Contains(cfgStr, "https") {
		t.Fatal("config missing https redirect scheme")
	}
}

func TestTraefik_ConfigValidation(t *testing.T) {
	dir := t.TempDir()
	proxy := NewTraefikReverseProxy(dir, "localhost:8080")

	cfg := &TraefikFileConfig{
		HTTP: &TraefikHTTPConfig{
			Routers: map[string]TraefikRouter{
				"valid": {
					Rule:    "Host(`example.com`)",
					Service: "svc",
				},
			},
			Services: map[string]TraefikService{
				"svc": {
					LoadBalancer: &TraefikLoadBalancer{
						Servers: []TraefikServer{
							{URL: "http://localhost:8080"},
						},
					},
				},
			},
		},
	}

	err := proxy.validateYAMLConfig(cfg)
	if err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	invalidCfg := &TraefikFileConfig{
		HTTP: &TraefikHTTPConfig{
			Routers: map[string]TraefikRouter{
				"bad": {
					Rule:    "",
					Service: "",
				},
			},
		},
	}

	err = proxy.validateYAMLConfig(invalidCfg)
	if err == nil {
		t.Fatal("expected validation error for empty rule, got nil")
	}
}

func TestTraefik_Kind(t *testing.T) {
	proxy := NewTraefikReverseProxy("", "")
	if proxy.Kind() != AdapterTraefik {
		t.Fatalf("expected traefik kind, got %s", proxy.Kind())
	}
}

func TestTraefik_RemoveRoutes(t *testing.T) {
	dir := t.TempDir()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer admin.Close()

	proxy := NewTraefikReverseProxy(dir, strings.TrimPrefix(admin.URL, "http://"))

	err := proxy.UpdateRoutes(context.Background(), []*RoutingRule{
		{ID: "remove-me", Domain: "remove.com", Path: "/", TargetPort: 8080, TargetHost: "localhost", Enabled: true},
	}, nil)
	if err != nil {
		t.Fatalf("UpdateRoutes failed: %v", err)
	}

	err = proxy.RemoveRoutes(context.Background(), []string{"remove-me"})
	if err != nil {
		t.Fatalf("RemoveRoutes failed: %v", err)
	}

	configPath := filepath.Join(dir, "routes.yml")
	data, _ := os.ReadFile(configPath)

	if strings.Contains(string(data), "remove-me") {
		t.Fatal("route was not removed")
	}
}

func TestTraefik_DomainRoutes(t *testing.T) {
	dir := t.TempDir()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer admin.Close()

	proxy := NewTraefikReverseProxy(dir, strings.TrimPrefix(admin.URL, "http://"))

	verified := []domains.VerifiedDomainRoute{
		{Domain: "example.com", Wildcard: false, ServerID: "srv-1"},
		{Domain: "*.wild.example.com", Wildcard: true, ServerID: "srv-2"},
	}

	err := proxy.UpdateDomainRoutes(context.Background(), verified)
	if err != nil {
		t.Fatalf("UpdateDomainRoutes failed: %v", err)
	}

	configPath := filepath.Join(dir, "routes.yml")
	data, _ := os.ReadFile(configPath)

	if !strings.Contains(string(data), "example.com") {
		t.Fatal("config missing main domain")
	}
	if !strings.Contains(string(data), "wild.example.com") {
		t.Fatal("config missing wildcard domain")
	}
}

func TestTraefik_ConcurrentReloadsSerialized(t *testing.T) {
	dir := t.TempDir()
	var mu sync.Mutex
	callLog := make([]string, 0)

	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callLog = append(callLog, r.URL.Path)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer admin.Close()

	proxy := NewTraefikReverseProxy(dir, strings.TrimPrefix(admin.URL, "http://"))

	var wg sync.WaitGroup
	errs := make(chan error, 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			rules := []*RoutingRule{
				{
					ID:         fmt.Sprintf("cr-%d", idx),
					Domain:     fmt.Sprintf("c%d.com", idx),
					Path:       "/",
					TargetPort: 9000 + idx,
					TargetHost: "localhost",
					Enabled:    true,
				},
			}
			errs <- proxy.UpdateRoutes(context.Background(), rules, nil)
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent UpdateRoutes failed: %v", err)
		}
	}
}

func TestTraefik_NoPoliciesNoMiddleware(t *testing.T) {
	dir := t.TempDir()
	proxy := NewTraefikReverseProxy(dir, "localhost:8080")

	rules := []*RoutingRule{
		{
			ID:         "no-pol",
			Domain:     "nopol.com",
			Path:       "/",
			TargetPort: 8080,
			TargetHost: "localhost",
			Enabled:    true,
		},
	}

	cfg := proxy.buildConfig(rules, nil)
	data, _ := yaml.Marshal(cfg)
	cfgStr := string(data)

	if strings.Contains(cfgStr, "rateLimit") {
		t.Fatal("config should not contain rateLimit when no policies")
	}
	if strings.Contains(cfgStr, "ipWhiteList") {
		t.Fatal("config should not contain ipWhiteList when no policies")
	}
	if strings.Contains(cfgStr, "ipBlackList") {
		t.Fatal("config should not contain ipBlackList when no policies")
	}
}

func TestTraefik_Health(t *testing.T) {
	dir := t.TempDir()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer admin.Close()

	proxy := NewTraefikReverseProxy(dir, strings.TrimPrefix(admin.URL, "http://"))

	err := proxy.UpdateRoutes(context.Background(), []*RoutingRule{
		{ID: "initial", Domain: "initial.com", Path: "/", TargetPort: 8080, TargetHost: "localhost", Enabled: true},
	}, nil)
	if err != nil {
		t.Fatalf("UpdateRoutes failed: %v", err)
	}

	health := proxy.Health(context.Background())
	if health.ErrCount > 0 {
		t.Fatalf("expected 0 errors, got %d", health.ErrCount)
	}
}

func TestTraefik_GetActiveConnections(t *testing.T) {
	dir := t.TempDir()
	proxy := NewTraefikReverseProxy(dir, "localhost:8080")

	conns := proxy.GetActiveConnections()
	if conns == nil {
		t.Fatal("GetActiveConnections returned nil")
	}
}

func TestTraefik_ValidateConfig(t *testing.T) {
	dir := t.TempDir()
	proxy := NewTraefikReverseProxy(dir, "localhost:8080")

	cfg := &TraefikFileConfig{
		HTTP: &TraefikHTTPConfig{
			Routers:  make(map[string]TraefikRouter),
			Services: make(map[string]TraefikService),
		},
	}

	err := proxy.validateYAMLConfig(cfg)
	if err != nil {
		t.Fatalf("empty config should be valid: %v", err)
	}
}

func TestService_AdapterSelection(t *testing.T) {
	store := newMockTrafficStore()
	proxy := NewCaddyReverseProxy("localhost:2019")
	traefikProxy := NewTraefikReverseProxy(t.TempDir(), "localhost:8080")

	svc := NewWithAdapter(store, nil, proxy, traefikProxy)

	if svc.AdapterKind(context.Background()) != AdapterTraefik {
		t.Fatalf("expected AdapterTraefik, got %s", svc.AdapterKind(context.Background()))
	}

	health := svc.AdapterHealth(context.Background())
	if health.Status == HealthUnknown {
		t.Fatal("adapter health should not be unknown")
	}

	svc.SetAdapter(nil)
	if svc.AdapterKind(context.Background()) != AdapterCaddy {
		t.Fatal("expected fallback to Caddy after setting adapter to nil")
	}
}

type mockTrafficStore struct{}

func newMockTrafficStore() *mockTrafficStore {
	return &mockTrafficStore{}
}

func (m *mockTrafficStore) GetServer(ctx context.Context, id string) (store.Server, error) {
	return store.Server{}, nil
}

func (m *mockTrafficStore) GetNode(ctx context.Context, id string) (store.Node, error) {
	return store.Node{}, nil
}
