package http

import (
	"bytes"
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

func newTestApp(cfg Config, h fiber.Handler) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/api/v1/oauth2/token", h)
	return app
}

func TestParseBasicAuth(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		wantOK   bool
		user     string
		password string
	}{
		{"empty", "", false, "", ""},
		{"non-basic", "Bearer foo", false, "", ""},
		{"valid", "Basic " + base64.StdEncoding.EncodeToString([]byte("alice:s3cret")), true, "alice", "s3cret"},
		{"no colon", "Basic " + base64.StdEncoding.EncodeToString([]byte("nocolon")), false, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, p, ok := parseBasicAuth(tt.header)
			if ok != tt.wantOK || u != tt.user || p != tt.password {
				t.Fatalf("got (%q,%q,%v) want (%q,%q,%v)", u, p, ok, tt.user, tt.password, tt.wantOK)
			}
		})
	}
}

func TestSplitScopes(t *testing.T) {
	got := splitScopes("foo bar  baz")
	if len(got) != 3 || got[0] != "foo" || got[1] != "bar" || got[2] != "baz" {
		t.Fatalf("unexpected: %#v", got)
	}
}

func TestContains(t *testing.T) {
	if !contains([]string{"a", "b"}, "a") {
		t.Fatal("expected true")
	}
	if contains([]string{"a", "b"}, "c") {
		t.Fatal("expected false")
	}
}

func TestIssueOAuth2Token_NoStore(t *testing.T) {
	// The endpoint should respond 503 when the store is nil.
	cfg := Config{AuthSecret: "test"}
	app := newTestApp(cfg, IssueOAuth2Token(cfg))
	body := strings.NewReader("grant_type=client_credentials&client_id=foo&client_secret=bar")
	req := httptest.NewRequest("POST", "/api/v1/oauth2/token", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req, 1500)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if resp.StatusCode != 503 {
		t.Fatalf("expected 503, got %d", resp.StatusCode)
	}
}

func TestVerifyOAuthToken_Unsigned(t *testing.T) {
	// Verify should reject unsigned/random tokens.
	cfg := Config{AuthSecret: "test-secret"}
	_, _, err := VerifyOAuthToken(cfg, "garbage")
	if err == nil {
		t.Fatal("expected error for garbage token")
	}
}

func TestVerifyOAuthToken_RequiresRevocationStore(t *testing.T) {
	// A cryptographically valid token must still fail closed when its
	// revocation state cannot be checked.
	cfg := Config{AuthSecret: "roundtrip-secret"}
	expiresAt := time.Now().Add(time.Hour)
	claims := jwt.MapClaims{
		"iss":       "forge-panel",
		"sub":       "user-1",
		"aud":       "client-abc",
		"iat":       time.Now().Unix(),
		"exp":       expiresAt.Unix(),
		"jti":       "jti-1",
		"scope":     "servers.read servers.write",
		"client_id": "client-abc",
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(cfg.AuthSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, _, err := VerifyOAuthToken(cfg, signed); err == nil {
		t.Fatal("expected verification to fail without a revocation store")
	}
}

func TestVerifyOAuthToken_Expired(t *testing.T) {
	cfg := Config{AuthSecret: "expired-secret"}
	claims := jwt.MapClaims{
		"iss": "forge-panel",
		"sub": "user-1",
		"exp": time.Now().Add(-time.Hour).Unix(),
		"jti": "jti-2",
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := tok.SignedString([]byte(cfg.AuthSecret))
	if _, _, err := VerifyOAuthToken(cfg, signed); err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestVerifyOAuthToken_WrongIssuer(t *testing.T) {
	cfg := Config{AuthSecret: "wrong-issuer-secret"}
	claims := jwt.MapClaims{
		"iss": "evil-panel",
		"sub": "user-1",
		"exp": time.Now().Add(time.Hour).Unix(),
		"jti": "jti-3",
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := tok.SignedString([]byte(cfg.AuthSecret))
	if _, _, err := VerifyOAuthToken(cfg, signed); err == nil {
		t.Fatal("expected error for wrong issuer")
	}
}

func TestHashOAuthClientSecret(t *testing.T) {
	h, err := HashOAuthClientSecret("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix([]byte(h), []byte("$2")) {
		t.Fatalf("expected bcrypt prefix, got %q", h[:5])
	}
}
