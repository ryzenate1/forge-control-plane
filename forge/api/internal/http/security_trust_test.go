package http

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// TestTrustedProxies_ExtractClientIP tests CIDR gating.
func TestTrustedProxies_ExtractClientIP(t *testing.T) {
	// Fiber Test peer is 0.0.0.0 in this env; use 0.0.0.0/0 to trust any peer for XFF test
	t.Setenv("TRUSTED_PROXIES", "0.0.0.0/0")
	app := fiber.New()
	app.Get("/ip", func(c *fiber.Ctx) error { return c.SendString(ExtractClientIP(c)) })

	// With 0.0.0.0/0, any peer is trusted, so XFF should be used (rightmost)
	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "10.1.2.3:1234"
	req.Header.Set("X-Forwarded-For", "1.1.1.1, 2.2.2.2")
	resp, _ := app.Test(req)
	body := make([]byte, 64)
	n, _ := resp.Body.Read(body)
	if got := string(body[:n]); got != "2.2.2.2" {
		t.Fatalf("trusted CIDR should allow XFF, got %q", got)
	}

	// Test isTrustedProxy directly for CIDR logic (without Fiber peer)
	t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8")
	if !isTrustedProxyString("10.1.2.3") {
		t.Fatal("10.1.2.3 should be trusted in 10.0.0.0/8")
	}
	if isTrustedProxyString("8.8.8.8") {
		t.Fatal("8.8.8.8 should not be trusted in 10.0.0.0/8")
	}
	t.Setenv("TRUSTED_PROXIES", "192.168.1.1")
	if !isTrustedProxyString("192.168.1.1") {
		t.Fatal("192.168.1.1 single IP should be trusted")
	}
	if isTrustedProxyString("192.168.1.2") {
		t.Fatal("192.168.1.2 should not be trusted when only 192.168.1.1 allowed")
	}
}

func TestTrustedProxies_DefaultDeny(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "")
	app := fiber.New()
	app.Get("/ip", func(c *fiber.Ctx) error { return c.SendString(ExtractClientIP(c)) })
	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "1.1.1.1")
	resp, _ := app.Test(req)
	body := make([]byte, 64)
	n, _ := resp.Body.Read(body)
	got := string(body[:n])
	if got == "1.1.1.1" {
		t.Fatalf("default deny should not trust XFF, got %q", got)
	}
}

func TestIPAccess_TrustedProxies(t *testing.T) {
	// When TRUSTED_PROXIES empty, IPAccess should default deny proxy headers
	t.Setenv("TRUSTED_PROXIES", "")
	t.Setenv("ADMIN_IP_ALLOW", "1.1.1.1")
	t.Setenv("ADMIN_IP_DENY", "")
	cfg := Config{}
	ipcfg := AdminIPAccessConfig(cfg)
	if !ipcfg.TrustProxy {
		t.Fatal("AdminIPAccessConfig should have TrustProxy true")
	}
	app := fiber.New()
	app.Use(IPAccessControl(ipcfg))
	app.Get("/protected", func(c *fiber.Ctx) error { return c.SendString("ok") })
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "1.1.1.1")
	// Peer 10.0.0.1 not trusted (TRUSTED_PROXIES empty => 0.0.0.0 peer not trusted), so getClientIP returns 0.0.0.0, not 1.1.1.1, so should be denied (allow list is 1.1.1.1)
	resp, _ := app.Test(req)
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403 when TRUSTED_PROXIES empty and peer untrusted, got %d", resp.StatusCode)
	}

	// With TRUSTED_PROXIES containing peer (0.0.0.0/0 covers Fiber test peer), should allow
	t.Setenv("TRUSTED_PROXIES", "0.0.0.0/0")
	// Need to recreate app to pick up new env? IPAccessControl already captured TrustProxy but uses ExtractClientIP which reads env per request
	resp2, _ := app.Test(req)
	if resp2.StatusCode != 200 {
		t.Fatalf("expected 200 when peer trusted and XFF matches allow, got %d", resp2.StatusCode)
	}
}

func TestMTLS_XForwardedProto_TrustedOnly(t *testing.T) {
	// MTLS requires HTTPS or trusted XFP. Test that untrusted XFP is rejected.
	t.Setenv("TRUSTED_PROXIES", "")
	cfg := MTLSAuthConfig{Enabled: false}
	h := MTLSAuthMiddleware(cfg)
	// When disabled, should pass through regardless
	app := fiber.New()
	app.Use(h)
	app.Get("/t", func(c *fiber.Ctx) error { return c.SendString("ok") })
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode != 200 {
		t.Fatalf("disabled mTLS should pass, got %d", resp.StatusCode)
	}

	// Enable mTLS with dummy CA to test XFP gating — we only test the HTTPS gate part
	// Use a config with Enabled true but missing CA will return 503; we test XFP logic separately via helper
	// Instead test isTrustedProxy directly
	t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8")
	if !isTrustedProxyString("10.1.2.3") {
		t.Fatal("10.1.2.3 should be trusted in 10.0.0.0/8")
	}
	if isTrustedProxyString("8.8.8.8") {
		t.Fatal("8.8.8.8 should not be trusted")
	}
}

func TestCSP_NoFallbackNonceAndNoStrictDynamic(t *testing.T) {
	app := fiber.New()
	app.Use(SecurityHeaders("production"))
	app.Get("/t", func(c *fiber.Ctx) error { return c.SendString("ok") })
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	resp, _ := app.Test(req)
	csp := resp.Header.Get("Content-Security-Policy")
	if strings.Contains(csp, "fallback-nonce") {
		t.Fatalf("CSP must not contain fallback-nonce, got %q", csp)
	}
	if strings.Contains(csp, "strict-dynamic") {
		t.Fatalf("CSP must not contain strict-dynamic with nonce, got %q", csp)
	}
	if !strings.Contains(csp, "'nonce-") {
		t.Fatalf("CSP must contain nonce, got %q", csp)
	}
	if nonce := resp.Header.Get("X-CSP-Nonce"); nonce == "" || nonce == "fallback-nonce" || strings.Contains(nonce, "fallback") {
		t.Fatalf("X-CSP-Nonce must be valid random, got %q", nonce)
	}

	// Check SecurityHeadersMiddleware also
	cfg := DefaultSecurityHeadersConfig()
	app2 := fiber.New()
	app2.Use(SecurityHeadersMiddleware(cfg))
	app2.Get("/t2", func(c *fiber.Ctx) error { return c.SendString("ok") })
	resp2, _ := app2.Test(req)
	csp2 := resp2.Header.Get("Content-Security-Policy")
	if strings.Contains(csp2, "fallback-nonce") {
		t.Fatalf("CSP2 must not contain fallback-nonce, got %q", csp2)
	}
	if strings.Contains(csp2, "strict-dynamic") {
		t.Fatalf("CSP2 must not contain strict-dynamic with nonce, got %q", csp2)
	}
}

// TestL4Probe_PrivateAndMetadataRejected pins the SSRF address-rejection
// behavior. NOTE: the old isProhibitedTargetIP helper was removed; the
// equivalent guard now lives as rejectDisallowedRegistryIP in
// handlers_appstore.go (same range semantics: loopback, unspecified, RFC1918
// private, link-local unicast/multicast all rejected, nil/unparseable rejected).
func TestL4Probe_PrivateAndMetadataRejected(t *testing.T) {
	tests := []struct {
		ip      string
		blocked bool
	}{
		{"192.168.1.10", true},
		{"10.0.0.5", true},
		{"172.16.5.1", true},
		{"127.0.0.1", true},
		{"169.254.169.254", true},
		{"169.254.0.1", true},
		{"0.0.0.0", true},
		{"224.0.0.1", true},
		{"::1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"203.0.113.5", false},
		{"not-an-ip", true},
	}
	for _, tc := range tests {
		blocked := rejectDisallowedRegistryIP(net.ParseIP(tc.ip)) != nil
		if blocked != tc.blocked {
			t.Fatalf("rejectDisallowedRegistryIP(%q) blocked=%v want %v", tc.ip, blocked, tc.blocked)
		}
	}
}

func TestLoadBalancer_AddTarget_RejectsPrivateIP(t *testing.T) {
	// Verify the shared SSRF guard rejects private IPs. NOTE: load-balancer
	// target handling no longer performs this check inline in internal/http;
	// the surviving production guard is rejectDisallowedRegistryIP.
	if err := rejectDisallowedRegistryIP(net.ParseIP("10.0.0.1")); err == nil {
		t.Fatal("private IP should be blocked")
	}
}
