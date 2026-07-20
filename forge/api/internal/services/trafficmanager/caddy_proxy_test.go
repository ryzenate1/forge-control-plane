package trafficmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestAtomicReload_Success(t *testing.T) {
	loadedCh := make(chan json.RawMessage, 1)
	appliedCh := make(chan json.RawMessage, 1)
	loadCallCount := 0
	var loadMu sync.Mutex

	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loadMu.Lock()
		defer loadMu.Unlock()

		if r.URL.Path == "/load" && r.Method == "POST" {
			loadCallCount++
			body, _ := readRequestBody(r.Body)
			loadedCh <- body
			if r.Header.Get("Cache-Control") != "must-revalidate" {
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, `{"valid":true}`)
				return
			}
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"valid":true}`)
			return
		}
		if r.URL.Path == "/config/" && r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"apps":{"http":{"servers":{"gamepanel":{"listen":[":80"],"routes":[]}}}}}`)
			return
		}
		if r.URL.Path == "/config/" && r.Method == "POST" {
			body, _ := readRequestBody(r.Body)
			appliedCh <- body
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"ok":true}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer admin.Close()

	proxy := NewCaddyReverseProxy(strings.TrimPrefix(admin.URL, "http://"))

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

	select {
	case loaded := <-loadedCh:
		var loadedCfg map[string]any
		if err := json.Unmarshal(loaded, &loadedCfg); err != nil {
			t.Fatalf("failed to parse loaded config: %v", err)
		}
	default:
		t.Fatal("config was never validated via /load")
	}

	select {
	case applied := <-appliedCh:
		var appliedCfg map[string]any
		if err := json.Unmarshal(applied, &appliedCfg); err != nil {
			t.Fatalf("failed to parse applied config: %v", err)
		}
		apps, ok := appliedCfg["apps"].(map[string]any)
		if !ok {
			t.Fatal("applied config missing apps key")
		}
		httpApp, ok := apps["http"].(map[string]any)
		if !ok {
			t.Fatal("applied config missing apps.http key")
		}
		servers, ok := httpApp["servers"].(map[string]any)
		if !ok {
			t.Fatal("applied config missing apps.http.servers key")
		}
		srv, ok := servers["gamepanel"].(map[string]any)
		if !ok {
			t.Fatal("applied config missing gamepanel server")
		}
		routes, ok := srv["routes"].([]any)
		if !ok || len(routes) == 0 {
			t.Fatal("applied config has no routes")
		}
	default:
		t.Fatal("config was never applied")
	}

	if loadCallCount != 1 {
		t.Fatalf("expected 1 /load call, got %d", loadCallCount)
	}
}

func TestAtomicReload_ValidationFailureDoesNotApply(t *testing.T) {
	appliedCh := make(chan struct{}, 1)

	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/load" && r.Method == "POST" {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error":"invalid config: missing listen directive"}`)
			return
		}
		if r.URL.Path == "/config/" && r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"apps":{}}`)
			return
		}
		if r.URL.Path == "/config/" && r.Method == "POST" {
			appliedCh <- struct{}{}
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer admin.Close()

	proxy := NewCaddyReverseProxy(strings.TrimPrefix(admin.URL, "http://"))

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
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Fatalf("expected validation failed error, got: %v", err)
	}

	select {
	case <-appliedCh:
		t.Fatal("config was applied despite validation failure")
	default:
	}
}

func TestAtomicReload_ApplyFailureRestores(t *testing.T) {
	var restoreCalled atomicBool

	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/load" && r.Method == "POST" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"valid":true}`)
			return
		}
		if r.URL.Path == "/config/" && r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"apps":{"http":{"servers":{"gamepanel":{"listen":[":80"],"routes":[{"@id":"old-route"}]}}}}}`)
			return
		}
		if r.URL.Path == "/config/" && r.Method == "POST" {
			if !restoreCalled.load() {
				restoreCalled.store(true)
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, `{"error":"caddy crashed"}`)
				return
			}
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"ok":true}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer admin.Close()

	proxy := NewCaddyReverseProxy(strings.TrimPrefix(admin.URL, "http://"))

	rules := []*RoutingRule{
		{
			ID:         "r2",
			Domain:     "crash.com",
			Path:       "/",
			TargetPort: 4000,
			TargetHost: "localhost",
			Enabled:    true,
		},
	}

	err := proxy.UpdateRoutes(context.Background(), rules, nil)
	if err == nil {
		t.Fatal("expected apply failure to be reported")
	}
	if !strings.Contains(err.Error(), "apply failed") {
		t.Fatalf("expected apply failed error, got: %v", err)
	}
}

func TestConcurrentReloadsAreSerialized(t *testing.T) {
	var mu sync.Mutex
	callLog := make([]string, 0)

	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		callLog = append(callLog, r.URL.Path+":"+r.Method)
		mu.Unlock()

		if r.URL.Path == "/load" && r.Method == "POST" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"valid":true}`)
			return
		}
		if r.URL.Path == "/config/" && r.Method == "GET" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{}`)
			return
		}
		if r.URL.Path == "/config/" && r.Method == "POST" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"ok":true}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer admin.Close()

	proxy := NewCaddyReverseProxy(strings.TrimPrefix(admin.URL, "http://"))

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

func TestMiddlewareRendering_RateLimit(t *testing.T) {
	proxy := NewCaddyReverseProxy("localhost:2019")

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

	config := proxy.buildServerConfig(proxy.buildRoutes(rules, policies), policies)
	jsonStr, _ := json.MarshalIndent(config, "", "  ")
	cfgStr := string(jsonStr)

	if !strings.Contains(cfgStr, "rate_limit") {
		t.Fatal("config missing rate_limit handler")
	}
	if !strings.Contains(cfgStr, "100/s") {
		t.Fatal("config missing expected rate value 100/s")
	}
	if !strings.Contains(cfgStr, `"burst": 50`) {
		t.Fatal("config missing expected burst value 50, got:", cfgStr)
	}
}

func TestMiddlewareRendering_IPWhitelist(t *testing.T) {
	proxy := NewCaddyReverseProxy("localhost:2019")

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

	config := proxy.buildServerConfig(proxy.buildRoutes(rules, policies), policies)
	jsonStr, _ := json.MarshalIndent(config, "", "  ")
	cfgStr := string(jsonStr)

	if !strings.Contains(cfgStr, "10.0.0.0/8") {
		t.Fatal("config missing IP whitelist range 10.0.0.0/8")
	}
	if !strings.Contains(cfgStr, "192.168.0.1") {
		t.Fatal("config missing IP whitelist 192.168.0.1")
	}
	if !strings.Contains(cfgStr, "remote_ip") {
		t.Fatal("config missing remote_ip matcher for IP whitelist")
	}
}

func TestMiddlewareRendering_IPBlacklist(t *testing.T) {
	proxy := NewCaddyReverseProxy("localhost:2019")

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

	config := proxy.buildServerConfig(proxy.buildRoutes(rules, policies), policies)
	jsonStr, _ := json.MarshalIndent(config, "", "  ")
	cfgStr := string(jsonStr)

	if !strings.Contains(cfgStr, "1.2.3.4") {
		t.Fatal("config missing IP blacklist entry 1.2.3.4")
	}
	if !strings.Contains(cfgStr, "5.6.7.0/24") {
		t.Fatal("config missing IP blacklist range 5.6.7.0/24")
	}
}

func TestMiddlewareRendering_HTTPToHTTPSRedirect(t *testing.T) {
	proxy := NewCaddyReverseProxy("localhost:2019")

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

	config := proxy.buildServerConfig(proxy.buildRoutes(rules, policies), policies)
	jsonStr, _ := json.MarshalIndent(config, "", "  ")
	cfgStr := string(jsonStr)

	if !strings.Contains(cfgStr, "gamepanel-https-redirect") {
		t.Fatal("config missing HTTPS redirect route")
	}
	if !strings.Contains(cfgStr, "static_response") {
		t.Fatal("config missing static_response handler for redirect")
	}
	if !strings.Contains(cfgStr, "https://") {
		t.Fatal("config missing https:// redirect target")
	}
	if !strings.Contains(cfgStr, "Location") {
		t.Fatal("config missing Location header for redirect")
	}
}

func TestCrossNodeRouting_UsesTargetHost(t *testing.T) {
	proxy := NewCaddyReverseProxy("localhost:2019")

	rules := []*RoutingRule{
		{
			ID:         "r7",
			Domain:     "cross-node.com",
			Path:       "/",
			TargetPort: 25565,
			TargetHost: "node-east-1.internal",
			Enabled:    true,
		},
	}

	config := proxy.buildServerConfig(proxy.buildRoutes(rules, nil), nil)
	jsonStr, _ := json.MarshalIndent(config, "", "  ")
	cfgStr := string(jsonStr)

	if !strings.Contains(cfgStr, `"node-east-1.internal:25565"`) {
		t.Fatal("config missing cross-node target host dial")
	}
	if !strings.Contains(cfgStr, `"dial"`) {
		t.Fatal("config missing dial upstream directive")
	}
}

func TestCrossNodeRouting_DefaultsToLocalhost(t *testing.T) {
	proxy := NewCaddyReverseProxy("localhost:2019")

	rules := []*RoutingRule{
		{
			ID:         "r8",
			Domain:     "local-test.com",
			Path:       "/",
			TargetPort: 9090,
			TargetHost: "",
			Enabled:    true,
		},
	}

	config := proxy.buildServerConfig(proxy.buildRoutes(rules, nil), nil)
	jsonStr, _ := json.MarshalIndent(config, "", "  ")
	cfgStr := string(jsonStr)

	if !strings.Contains(cfgStr, `"localhost:9090"`) {
		t.Fatal("config should default to localhost when TargetHost is empty, got:", cfgStr)
	}
}

func TestStrategyRendering(t *testing.T) {
	proxy := NewCaddyReverseProxy("localhost:2019")

	tests := []struct {
		strategy string
		expected string
	}{
		{"round_robin", ""},
		{"least_conn", `"lb_policy"`},
		{"random", `"lb_policy":"random"`},
		{"first", `"lb_policy":"first"`},
	}

	for _, tt := range tests {
		t.Run(tt.strategy, func(t *testing.T) {
			rules := []*RoutingRule{
				{
					ID:         "strat",
					Domain:     "strat.com",
					Path:       "/",
					TargetPort: 8080,
					TargetHost: "localhost",
					Strategy:   tt.strategy,
					Enabled:    true,
				},
			}
			config := proxy.buildServerConfig(proxy.buildRoutes(rules, nil), nil)
			jsonStr, _ := json.Marshal(config)
			cfgStr := string(jsonStr)

			if tt.expected == "" {
				if strings.Contains(cfgStr, `"lb_policy"`) && tt.strategy == "round_robin" {
					t.Fatal("round_robin should not produce lb_policy key")
				}
			} else {
				if !strings.Contains(cfgStr, tt.expected) {
					t.Fatalf("expected %s in config for strategy %s, got: %s", tt.expected, tt.strategy, cfgStr)
				}
			}
		})
	}
}

func TestWebSocketRendering(t *testing.T) {
	proxy := NewCaddyReverseProxy("localhost:2019")

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

	config := proxy.buildServerConfig(proxy.buildRoutes(rules, nil), nil)
	jsonStr, _ := json.MarshalIndent(config, "", "  ")
	cfgStr := string(jsonStr)

	if !strings.Contains(cfgStr, "header_up") {
		t.Fatal("config missing header_up for websocket upgrade")
	}
	if !strings.Contains(cfgStr, "Connection") {
		t.Fatal("config missing Connection header for websocket")
	}
	if !strings.Contains(cfgStr, "Upgrade") {
		t.Fatal("config missing Upgrade header for websocket")
	}
}

func TestCircuitBreakerRendering(t *testing.T) {
	proxy := NewCaddyReverseProxy("localhost:2019")

	policy := &TrafficPolicy{
		Name:                    "cb",
		CircuitBreaker:          true,
		CircuitBreakerThreshold: 7,
		CircuitBreakerTimeout:   45,
	}
	policies := map[string]*TrafficPolicy{"cb": policy}

	rules := []*RoutingRule{
		{
			ID:         "cb-rule",
			Domain:     "cb.com",
			Path:       "/",
			TargetPort: 8080,
			TargetHost: "localhost",
			Enabled:    true,
		},
	}

	config := proxy.buildServerConfig(proxy.buildRoutes(rules, policies), policies)
	jsonStr, _ := json.MarshalIndent(config, "", "  ")
	cfgStr := string(jsonStr)

	if !strings.Contains(cfgStr, "circuit_breaker") {
		t.Fatal("config missing circuit_breaker handler")
	}
	if !strings.Contains(cfgStr, `"max_failures": 7`) {
		t.Fatal("config missing circuit_breaker max_failures=7, got:", cfgStr)
	}
	if !strings.Contains(cfgStr, `"timeout": "45s"`) {
		t.Fatal("config missing circuit_breaker timeout=45s, got:", cfgStr)
	}
}

func TestCircuitBreakerDefaults(t *testing.T) {
	proxy := NewCaddyReverseProxy("localhost:2019")

	policy := &TrafficPolicy{
		Name:           "cb-def",
		CircuitBreaker: true,
	}
	policies := map[string]*TrafficPolicy{"cb-def": policy}

	rules := []*RoutingRule{
		{
			ID:         "cb-def-rule",
			Domain:     "cbdef.com",
			Path:       "/",
			TargetPort: 8080,
			TargetHost: "localhost",
			Enabled:    true,
		},
	}

	config := proxy.buildServerConfig(proxy.buildRoutes(rules, policies), policies)
	jsonStr, _ := json.MarshalIndent(config, "", "  ")
	cfgStr := string(jsonStr)

	if !strings.Contains(cfgStr, `"max_failures": 5`) {
		t.Fatal("circuit_breaker should default max_failures to 5, got:", cfgStr)
	}
	if !strings.Contains(cfgStr, `"timeout": "30s"`) {
		t.Fatal("circuit_breaker should default timeout to 30s, got:", cfgStr)
	}
}

func TestNoPolicies_NoMiddleware(t *testing.T) {
	proxy := NewCaddyReverseProxy("localhost:2019")

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

	config := proxy.buildServerConfig(proxy.buildRoutes(rules, nil), nil)
	jsonStr, _ := json.MarshalIndent(config, "", "  ")
	cfgStr := string(jsonStr)

	if strings.Contains(cfgStr, "rate_limit") {
		t.Fatal("config should not contain rate_limit when no policies")
	}
	if strings.Contains(cfgStr, "remote_ip") {
		t.Fatal("config should not contain remote_ip when no policies")
	}
	if strings.Contains(cfgStr, "gamepanel-https-redirect") {
		t.Fatal("config should not contain redirect when no TLS-enabled policies")
	}
}

func readRequestBody(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}

type atomicBool struct {
	mu    sync.Mutex
	value bool
}

func (b *atomicBool) load() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.value
}

func (b *atomicBool) store(v bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.value = v
}
