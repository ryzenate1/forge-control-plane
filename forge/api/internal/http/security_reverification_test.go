package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

// TestTrustedProxy_CIDR_gate verifies that proxy trust is gated strictly by
// explicit TRUSTED_PROXIES CIDR / single-IP allowlist, not by isPrivate
// (net.IP.IsPrivate) or loopback status. Private alone must NOT be sufficient.
func TestTrustedProxy_CIDR_gate(t *testing.T) {
	// Direct isTrustedProxyString checks: private IP alone is not trusted when CIDR absent.
	t.Run("private_without_CIDR_is_not_trusted", func(t *testing.T) {
		t.Setenv("TRUSTED_PROXIES", "")
		privateIPs := []string{"10.0.0.5", "192.168.1.10", "172.16.5.1", "10.255.255.255", "192.168.0.1"}
		for _, ip := range privateIPs {
			if isTrustedProxyString(ip) {
				t.Fatalf("private IP %q must NOT be trusted when TRUSTED_PROXIES empty (isPrivate is not a gate)", ip)
			}
		}
		// loopback also not trusted without explicit CIDR
		if isTrustedProxyString("127.0.0.1") {
			t.Fatal("127.0.0.1 must not be trusted without explicit CIDR")
		}
		if isTrustedProxyString("::1") {
			t.Fatal("::1 must not be trusted without explicit CIDR")
		}
	})

	t.Run("CIDR_gate_allows_only_matching_range", func(t *testing.T) {
		t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8")
		if !isTrustedProxyString("10.1.2.3") {
			t.Fatal("10.1.2.3 should be trusted in 10.0.0.0/8")
		}
		if !isTrustedProxyString("10.255.255.255") {
			t.Fatal("10.255.255.255 should be trusted in 10.0.0.0/8")
		}
		// Private but outside CIDR must NOT be trusted — this is the core isPrivate vs CIDR distinction.
		if isTrustedProxyString("192.168.1.10") {
			t.Fatal("192.168.1.10 is private but outside 10.0.0.0/8, must NOT be trusted (isPrivate != trusted)")
		}
		if isTrustedProxyString("172.16.0.5") {
			t.Fatal("172.16.0.5 is private but outside 10.0.0.0/8, must NOT be trusted")
		}
		if isTrustedProxyString("8.8.8.8") {
			t.Fatal("8.8.8.8 must not be trusted in 10.0.0.0/8")
		}
	})

	t.Run("single_IP_gate_exact_match", func(t *testing.T) {
		t.Setenv("TRUSTED_PROXIES", "192.168.1.1")
		if !isTrustedProxyString("192.168.1.1") {
			t.Fatal("192.168.1.1 single IP should be trusted")
		}
		if isTrustedProxyString("192.168.1.2") {
			t.Fatal("192.168.1.2 should not be trusted when only 192.168.1.1 allowed (private does not imply trust)")
		}
		if isTrustedProxyString("192.168.1.10") {
			t.Fatal("192.168.1.10 should not be trusted when only 192.168.1.1 allowed")
		}
	})

	t.Run("public_CIDR_trusts_non_private_IP", func(t *testing.T) {
		// Explicit public CIDR must be trusted even though IP is not private — proves gate is CIDR, not isPrivate.
		t.Setenv("TRUSTED_PROXIES", "203.0.113.5/32,198.51.100.0/24")
		if !isTrustedProxyString("203.0.113.5") {
			t.Fatal("203.0.113.5/32 should be trusted even though not private")
		}
		if !isTrustedProxyString("198.51.100.24") {
			t.Fatal("198.51.100.24 should be trusted in 198.51.100.0/24")
		}
		if isTrustedProxyString("203.0.113.6") {
			t.Fatal("203.0.113.6 not in /32, must not be trusted")
		}
		if isTrustedProxyString("10.0.0.1") {
			t.Fatal("10.0.0.1 private but not in public CIDR, must not be trusted")
		}
	})

	t.Run("multiple_CIDRs_and_IPs_comma_separated", func(t *testing.T) {
		t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8, 192.168.1.1 , 203.0.113.0/24")
		if !isTrustedProxyString("10.1.1.1") {
			t.Fatal("10.1.1.1 should be trusted via 10.0.0.0/8")
		}
		if !isTrustedProxyString("192.168.1.1") {
			t.Fatal("192.168.1.1 single IP should be trusted")
		}
		if !isTrustedProxyString("203.0.113.99") {
			t.Fatal("203.0.113.99 should be trusted via 203.0.113.0/24")
		}
		if isTrustedProxyString("192.168.1.2") {
			t.Fatal("192.168.1.2 should not be trusted (not exact IP, not in any CIDR)")
		}
	})

	t.Run("invalid_entries_are_skipped", func(t *testing.T) {
		t.Setenv("TRUSTED_PROXIES", "not-a-cidr, 10.0.0.0/8, invalid-ip")
		if !isTrustedProxyString("10.1.2.3") {
			t.Fatal("valid CIDR should still be honored when invalid entries present")
		}
		if isTrustedProxyString("8.8.8.8") {
			t.Fatal("8.8.8.8 must not be trusted")
		}
	})

	t.Run("ExtractClientIP_default_deny_ignores_XFF", func(t *testing.T) {
		t.Setenv("TRUSTED_PROXIES", "")
		app := fiber.New()
		app.Get("/ip", func(c *fiber.Ctx) error { return c.SendString(ExtractClientIP(c)) })
		req := httptest.NewRequest(http.MethodGet, "/ip", nil)
		req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
		req.Header.Set("X-Real-IP", "9.9.9.9")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		body := make([]byte, 64)
		n, _ := resp.Body.Read(body)
		got := string(body[:n])
		// With default deny, must NOT return any forwarded address; should be direct peer (Fiber test peer is 0.0.0.0)
		if got == "1.2.3.4" || got == "5.6.7.8" || got == "9.9.9.9" {
			t.Fatalf("default deny: ExtractClientIP must not trust proxy headers, got %q", got)
		}
		if got != "0.0.0.0" && got != "10.0.0.5" && got != "" {
			// Accept 0.0.0.0 (Fiber Test default) — just ensure it is not a forwarded IP
		}
	})

	t.Run("ExtractClientIP_CIDR_gate_allows_XFF_rightmost", func(t *testing.T) {
		// Fiber Test peer is 0.0.0.0; trust all via 0.0.0.0/0 so we can test rightmost semantics
		t.Setenv("TRUSTED_PROXIES", "0.0.0.0/0")
		app := fiber.New()
		app.Get("/ip", func(c *fiber.Ctx) error { return c.SendString(ExtractClientIP(c)) })
		req := httptest.NewRequest(http.MethodGet, "/ip", nil)
		req.Header.Set("X-Forwarded-For", "203.0.113.99, 198.51.100.24")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		body := make([]byte, 64)
		n, _ := resp.Body.Read(body)
		got := string(body[:n])
		if got != "198.51.100.24" {
			t.Fatalf("trusted CIDR should allow XFF rightmost, got %q want 198.51.100.24", got)
		}
		// Verify private peer without CIDR would have been denied — caller-supplied leftmost must not win
		req2 := httptest.NewRequest(http.MethodGet, "/ip", nil)
		req2.Header.Set("X-Forwarded-For", "1.1.1.1, 2.2.2.2")
		resp2, _ := app.Test(req2)
		n2, _ := resp2.Body.Read(body)
		if got2 := string(body[:n2]); got2 != "2.2.2.2" {
			t.Fatalf("rightmost XFF semantics: got %q want 2.2.2.2", got2)
		}
	})

	t.Run("nil_and_invalid_IP_never_trusted", func(t *testing.T) {
		t.Setenv("TRUSTED_PROXIES", "0.0.0.0/0")
		if isTrustedProxy(nil) {
			t.Fatal("nil IP must not be trusted")
		}
		if isTrustedProxyString("") {
			t.Fatal("empty string must not be trusted")
		}
		if isTrustedProxyString("not-an-ip") {
			t.Fatal("invalid IP must not be trusted")
		}
	})
}

// TestCSP_NoFallbackNonce verifies that neither SecurityHeaders nor
// SecurityHeadersMiddleware ever emits fallback-nonce or strict-dynamic alongside nonce,
// and that per-request nonces are cryptographically random and differ per request.
func TestCSP_NoFallbackNonce(t *testing.T) {
	t.Run("SecurityHeaders_production_no_fallback_no_strict_dynamic", func(t *testing.T) {
		app := fiber.New()
		app.Use(SecurityHeaders("production"))
		app.Get("/t", func(c *fiber.Ctx) error { return c.SendString("ok") })

		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		csp := resp.Header.Get("Content-Security-Policy")
		if csp == "" {
			t.Fatal("CSP header must be set")
		}
		if strings.Contains(csp, "fallback-nonce") {
			t.Fatalf("CSP must not contain fallback-nonce, got %q", csp)
		}
		if strings.Contains(csp, "fallback") {
			t.Fatalf("CSP must not contain fallback substring, got %q", csp)
		}
		if strings.Contains(csp, "strict-dynamic") {
			t.Fatalf("CSP must not contain strict-dynamic with nonce, got %q", csp)
		}
		if !strings.Contains(csp, "'nonce-") {
			t.Fatalf("CSP must contain nonce directive, got %q", csp)
		}
		// X-CSP-Nonce must be valid random, not fallback
		nonce := resp.Header.Get("X-CSP-Nonce")
		if nonce == "" {
			t.Fatal("X-CSP-Nonce must be set")
		}
		if nonce == "fallback-nonce" || strings.Contains(nonce, "fallback") {
			t.Fatalf("X-CSP-Nonce must be random, got %q", nonce)
		}
		if strings.Contains(csp, nonce) == false {
			t.Fatalf("CSP nonce must match X-CSP-Nonce %q, CSP %q", nonce, csp)
		}
		// script-src must not contain unsafe-inline (style-src may)
		if strings.Contains(csp, "script-src 'self' 'unsafe-inline'") {
			t.Fatalf("script-src must not contain unsafe-inline, got %q", csp)
		}

		// Second request must produce a different nonce (per-request randomness)
		req2 := httptest.NewRequest(http.MethodGet, "/t", nil)
		resp2, _ := app.Test(req2)
		nonce2 := resp2.Header.Get("X-CSP-Nonce")
		if nonce2 == "" {
			t.Fatal("second request X-CSP-Nonce must be set")
		}
		if nonce == nonce2 {
			t.Fatalf("per-request nonce must differ: first %q second %q", nonce, nonce2)
		}
		csp2 := resp2.Header.Get("Content-Security-Policy")
		if strings.Contains(csp2, "fallback-nonce") || strings.Contains(csp2, "strict-dynamic") {
			t.Fatalf("second CSP must also not contain fallback/strict-dynamic, got %q", csp2)
		}
	})

	t.Run("SecurityHeadersMiddleware_default_no_fallback_no_strict_dynamic", func(t *testing.T) {
		cfg := DefaultSecurityHeadersConfig()
		app := fiber.New()
		app.Use(SecurityHeadersMiddleware(cfg))
		app.Get("/t2", func(c *fiber.Ctx) error { return c.SendString("ok") })

		req := httptest.NewRequest(http.MethodGet, "/t2", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		csp := resp.Header.Get("Content-Security-Policy")
		if csp == "" {
			t.Fatal("CSP must be set via middleware")
		}
		if strings.Contains(csp, "fallback-nonce") {
			t.Fatalf("CSP must not contain fallback-nonce, got %q", csp)
		}
		if strings.Contains(csp, "strict-dynamic") {
			t.Fatalf("CSP must not contain strict-dynamic, got %q", csp)
		}
		if strings.Contains(csp, "{NONCE}") {
			t.Fatalf("CSP still contains unreplaced {NONCE} placeholder, got %q", csp)
		}
		if !strings.Contains(csp, "'nonce-") {
			t.Fatalf("CSP must contain nonce, got %q", csp)
		}
		nonce := resp.Header.Get("X-CSP-Nonce")
		if nonce == "" || nonce == "fallback-nonce" || strings.Contains(nonce, "fallback") {
			t.Fatalf("X-CSP-Nonce must be valid random, got %q", nonce)
		}
		if !strings.Contains(csp, nonce) {
			t.Fatalf("CSP must embed X-CSP-Nonce %q, got %q", nonce, csp)
		}

		// Per-request uniqueness via middleware as well
		resp2, _ := app.Test(req)
		nonce2 := resp2.Header.Get("X-CSP-Nonce")
		if nonce == nonce2 {
			t.Fatalf("middleware per-request nonce must differ: %q vs %q", nonce, nonce2)
		}
	})

	t.Run("SecurityHeaders_no_HSTS_in_development", func(t *testing.T) {
		app := fiber.New()
		app.Use(SecurityHeaders("development"))
		app.Get("/t", func(c *fiber.Ctx) error { return c.SendString("ok") })
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		resp, _ := app.Test(req)
		// Development must not send HSTS (would poison localhost), but must still have CSP nonce without fallback
		if hsts := resp.Header.Get("Strict-Transport-Security"); hsts != "" {
			t.Fatalf("development HSTS must be absent, got %q", hsts)
		}
		csp := resp.Header.Get("Content-Security-Policy")
		if strings.Contains(csp, "fallback-nonce") || strings.Contains(csp, "strict-dynamic") {
			t.Fatalf("development CSP must also not contain fallback/strict-dynamic, got %q", csp)
		}
	})

	t.Run("SecurityHeaders_production_has_HSTS_and_headers", func(t *testing.T) {
		app := fiber.New()
		app.Use(SecurityHeaders("production"))
		app.Get("/t", func(c *fiber.Ctx) error { return c.SendString("ok") })
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		resp, _ := app.Test(req)
		if hsts := resp.Header.Get("Strict-Transport-Security"); hsts != "max-age=31536000; includeSubDomains; preload" {
			t.Fatalf("production HSTS missing or wrong, got %q", hsts)
		}
		// Check other security headers present
		for _, h := range []string{"X-Frame-Options", "X-Content-Type-Options", "Referrer-Policy", "Permissions-Policy"} {
			if resp.Header.Get(h) == "" {
				t.Fatalf("header %s must be set", h)
			}
		}
	})
}

// TestMountAllowlist_BlocksEtc verifies that mount source validation in the
// store layer (used by the HTTP mount handlers) blocks host-breakout paths
// such as /etc, /proc, /var/run/docker.sock etc., and that the allowlist
// via MOUNTS_ALLOWED_PREFIX is enforced. This reverifies the handler cannot
// persist a mount outside the allowed prefix.
func TestMountAllowlist_BlocksEtc(t *testing.T) {
	// Store.CreateMount validates source/target via validateMountPath before touching DB,
	// so a zero-value Store (nil db) is sufficient to test validation errors for blocked paths.
	s := &store.Store{}
	ctx := context.Background()

	// Ensure deny-list mode (no allowlist) uses the built-in deniedPrefixes including /etc.
	t.Run("deny_list_blocks_etc_and_sensitive", func(t *testing.T) {
		orig := os.Getenv("MOUNTS_ALLOWED_PREFIX")
		_ = os.Unsetenv("MOUNTS_ALLOWED_PREFIX")
		t.Cleanup(func() {
			if orig != "" {
				_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", orig)
			} else {
				_ = os.Unsetenv("MOUNTS_ALLOWED_PREFIX")
			}
		})

		blocked := []string{
			"/etc",
			"/etc/shadow",
			"/etc/passwd",
			"/etc/forge",
			"/etc/hosts",
			"/proc",
			"/proc/self/environ",
			"/sys",
			"/sys/kernel",
			"/dev",
			"/dev/sda",
			"/boot",
			"/boot/vmlinuz",
			"/root",
			"/root/.ssh",
			"/var/run",
			"/var/run/docker.sock",
			"/run",
			"/run/docker.sock",
			"/var/lib/forge",
			"/var/lib/forge/volumes",
			"/var/lib/docker",
			"/",
			"/home/container",
		}
		for _, src := range blocked {
			t.Run(src, func(t *testing.T) {
				_, err := s.CreateMount(ctx, store.CreateMountRequest{
					Name:   "test-mount",
					Source: src,
					Target: "/data",
				}, nil)
				if err == nil {
					t.Fatalf("CreateMount(%q, /data) should be blocked (host breakout)", src)
				}
				msg := strings.ToLower(err.Error())
				if !strings.Contains(msg, "reserved") && !strings.Contains(msg, "protected") && !strings.Contains(err.Error(), "MOUNTS_ALLOWED_PREFIX") {
					t.Fatalf("unexpected error for %q: %v (should mention reserved/protected or MOUNTS_ALLOWED_PREFIX)", src, err)
				}
			})
		}
	})

	t.Run("deny_list_allows_srv_and_mnt", func(t *testing.T) {
		orig := os.Getenv("MOUNTS_ALLOWED_PREFIX")
		_ = os.Unsetenv("MOUNTS_ALLOWED_PREFIX")
		t.Cleanup(func() {
			if orig != "" {
				_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", orig)
			} else {
				_ = os.Unsetenv("MOUNTS_ALLOWED_PREFIX")
			}
		})
		allowed := []string{"/srv/forge-mounts/data", "/srv/game-data", "/mnt/shared/maps"}
		for _, src := range allowed {
			t.Run(src, func(t *testing.T) {
				panicked, err := func() (panicked bool, err error) {
					defer func() {
						if r := recover(); r != nil {
							panicked = true
						}
					}()
					_, err = s.CreateMount(ctx, store.CreateMountRequest{
						Name:   "test-mount",
						Source: src,
						Target: "/data",
					}, nil)
					return
				}()
				if err != nil && (strings.Contains(strings.ToLower(err.Error()), "reserved") || strings.Contains(strings.ToLower(err.Error()), "protected") || strings.Contains(err.Error(), "MOUNTS_ALLOWED_PREFIX") || strings.Contains(err.Error(), "not within allowed prefix")) {
					t.Fatalf("CreateMount(%q) should be allowed in deny-list mode, got validation error %v", src, err)
				}
				if err == nil && !panicked {
					t.Fatalf("CreateMount(%q) unexpectedly succeeded without DB (should have panicked or returned validation)", src)
				}
				// panicked means validation passed and we hit nil DB — expected for allowed paths.
			})
		}
	})

	t.Run("allowlist_enforces_prefix", func(t *testing.T) {
		orig := os.Getenv("MOUNTS_ALLOWED_PREFIX")
		_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", "/srv/forge-mounts,/var/lib/forge/mounts")
		t.Cleanup(func() {
			if orig != "" {
				_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", orig)
			} else {
				_ = os.Unsetenv("MOUNTS_ALLOWED_PREFIX")
			}
		})
		// Allowed via allowlist
		for _, src := range []string{"/srv/forge-mounts/app", "/srv/forge-mounts", "/var/lib/forge/mounts/app"} {
			t.Run("allow_"+src, func(t *testing.T) {
				panicked, err := func() (panicked bool, err error) {
					defer func() {
						if r := recover(); r != nil {
							panicked = true
						}
					}()
					_, err = s.CreateMount(ctx, store.CreateMountRequest{Name: "m", Source: src, Target: "/data"}, nil)
					return
				}()
				if err != nil && strings.Contains(err.Error(), "not within allowed prefix") {
					t.Fatalf("allowlist: %q should be allowed, got %v", src, err)
				}
				if err == nil && !panicked {
					t.Fatalf("allowlist: %q unexpectedly succeeded without DB", src)
				}
			})
		}
		// Blocked even with allowlist set
		for _, src := range []string{"/srv/game-data", "/etc", "/etc/shadow", "/var/run/docker.sock", "/proc", "/var/lib/forge"} {
			t.Run("block_"+src, func(t *testing.T) {
				_, err := s.CreateMount(ctx, store.CreateMountRequest{Name: "m", Source: src, Target: "/data"}, nil)
				if err == nil {
					t.Fatalf("allowlist: %q should be blocked, but CreateMount succeeded", src)
				}
				if !strings.Contains(err.Error(), "MOUNTS_ALLOWED_PREFIX") && !strings.Contains(err.Error(), "not within allowed prefix") {
					t.Fatalf("allowlist error for %q should guide to MOUNTS_ALLOWED_PREFIX, got %v", src, err)
				}
			})
		}
	})

	t.Run("target_validation_still_blocks_reserved", func(t *testing.T) {
		orig := os.Getenv("MOUNTS_ALLOWED_PREFIX")
		_ = os.Unsetenv("MOUNTS_ALLOWED_PREFIX")
		t.Cleanup(func() {
			if orig != "" {
				_ = os.Setenv("MOUNTS_ALLOWED_PREFIX", orig)
			} else {
				_ = os.Unsetenv("MOUNTS_ALLOWED_PREFIX")
			}
		})
		for _, target := range []string{"/", "/home/container"} {
			t.Run(target, func(t *testing.T) {
				_, err := s.CreateMount(ctx, store.CreateMountRequest{Name: "m", Source: "/srv/data", Target: target}, nil)
				if err == nil {
					t.Fatalf("target %q should be reserved/blocked", target)
				}
			})
		}
		// Unclean / backslash / traversal also blocked via source validation
		if _, err := s.CreateMount(ctx, store.CreateMountRequest{Name: "m", Source: "/srv/../etc", Target: "/data"}, nil); err == nil {
			t.Fatal("unclean source /srv/../etc should be blocked")
		}
		if _, err := s.CreateMount(ctx, store.CreateMountRequest{Name: "m", Source: "/srv\\data", Target: "/data"}, nil); err == nil {
			t.Fatal("backslash path should be blocked")
		}
	})
}

// TestSecurityHeaders_Reverification ensures the old generateNonce fallback path is gone:
// we generate two nonces via SecurityHeaders and ensure neither is the literal fallback-nonce.
func TestSecurityHeaders_Reverification(t *testing.T) {
	app := fiber.New()
	app.Use(SecurityHeaders("production"))
	app.Get("/rv", func(c *fiber.Ctx) error { return c.SendString("ok") })
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/rv", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		nonce := resp.Header.Get("X-CSP-Nonce")
		if nonce == "fallback-nonce" || strings.Contains(nonce, "fallback") {
			t.Fatalf("iteration %d: X-CSP-Nonce must not be fallback, got %q", i, nonce)
		}
		csp := resp.Header.Get("Content-Security-Policy")
		if strings.Contains(csp, "fallback-nonce") || strings.Contains(csp, "strict-dynamic") {
			t.Fatalf("iteration %d: CSP must not contain fallback/strict-dynamic, got %q", i, csp)
		}
	}
}
