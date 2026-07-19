package http

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestCaptchaMiddleware_PassthroughWhenNoStore(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/test", CaptchaMiddleware(Config{}), func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"ok": true})
	})

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 when no store configured, got %d", resp.StatusCode)
	}
}

func TestGetCaptchaSettings_NilContext(t *testing.T) {
	enabled, secret, site := getCaptchaSettings(nil, Config{})
	if enabled || secret != "" || site != "" {
		t.Fatal("expected empty captcha settings with nil context")
	}
}

func TestGetCaptchaSettings_NilStore(t *testing.T) {
	enabled, secret, site := getCaptchaSettings(context.Background(), Config{})
	if enabled || secret != "" || site != "" {
		t.Fatal("expected empty captcha settings with nil store")
	}
}

func TestVerifyCaptchaToken_EmptySecret(t *testing.T) {
	err := verifyCaptchaToken(context.Background(), "", "test-token", "127.0.0.1")
	if err == nil {
		t.Fatal("expected error with empty secret")
	}
}

func TestVerifyCaptchaToken_EmptyToken(t *testing.T) {
	err := verifyCaptchaToken(context.Background(), "secret", "", "127.0.0.1")
	if err == nil {
		t.Fatal("expected error with empty token")
	}
}

func TestDetectCaptchaProvider_Turnstile(t *testing.T) {
	provider := detectCaptchaProvider("0.abc123")
	if provider != captchaProviderTurnstile {
		t.Fatalf("expected turnstile provider for '0.' prefixed token, got %s", provider)
	}
}

func TestDetectCaptchaProvider_Recaptcha(t *testing.T) {
	provider := detectCaptchaProvider("some-recaptcha-token")
	if provider != captchaProviderRecaptcha {
		t.Fatalf("expected recaptcha provider for regular token, got %s", provider)
	}
}

func TestGetCaptchaVerifyURL_Turnstile(t *testing.T) {
	url := getCaptchaVerifyURL(captchaProviderTurnstile)
	expected := "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	if url != expected {
		t.Fatalf("expected %s, got %s", expected, url)
	}
}

func TestGetCaptchaVerifyURL_Recaptcha(t *testing.T) {
	url := getCaptchaVerifyURL(captchaProviderRecaptcha)
	expected := "https://www.google.com/recaptcha/api/siteverify"
	if url != expected {
		t.Fatalf("expected %s, got %s", expected, url)
	}
}
