package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

type captchaProvider string

const (
	captchaProviderRecaptcha captchaProvider = "recaptcha"
	captchaProviderTurnstile captchaProvider = "turnstile"
)

type captchaVerifyResponse struct {
	Success bool `json:"success"`
}

func detectCaptchaProvider(responseField string) captchaProvider {
	if strings.HasPrefix(responseField, "0.") || strings.HasPrefix(responseField, "1.") || strings.HasPrefix(responseField, "2.") || strings.HasPrefix(responseField, "3.") {
		return captchaProviderTurnstile
	}
	return captchaProviderRecaptcha
}

func getCaptchaVerifyURL(provider captchaProvider) string {
	switch provider {
	case captchaProviderTurnstile:
		return "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	default:
		return "https://www.google.com/recaptcha/api/siteverify"
	}
}

func verifyCaptchaToken(ctx context.Context, secret, token, remoteIP string) error {
	if secret == "" || token == "" {
		return fmt.Errorf("captcha token or secret is empty")
	}

	provider := detectCaptchaProvider(token)
	verifyURL := getCaptchaVerifyURL(provider)

	client := &http.Client{Timeout: 10 * time.Second}
	data := url.Values{
		"secret":   {secret},
		"response": {token},
	}
	if remoteIP != "" {
		data.Set("remoteip", remoteIP)
	}

	resp, err := client.PostForm(verifyURL, data)
	if err != nil {
		return fmt.Errorf("captcha verify request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("captcha verify read failed: %w", err)
	}

	var result captchaVerifyResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("captcha verify decode failed: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("captcha verification failed")
	}

	return nil
}

func getCaptchaSettings(ctx context.Context, cfg Config) (enabled bool, secretKey string, siteKey string) {
	if cfg.Store == nil {
		return false, "", ""
	}
	settings, err := cfg.Store.GetPanelSettings(ctx)
	if err != nil {
		return false, "", ""
	}
	return settings.RecaptchaEnabled, settings.RecaptchaSecretKey, settings.RecaptchaSiteKey
}

func CaptchaMiddleware(cfg Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()

		enabled, secretKey, _ := getCaptchaSettings(ctx, cfg)
		if !enabled || secretKey == "" {
			return c.Next()
		}

		token := c.Get("g-recaptcha-response")
		if token == "" {
			token = c.Get("cf-turnstile-response")
		}
		if token == "" {
			var body struct {
				RecaptchaToken string `json:"recaptchaToken"`
			}
			if err := c.BodyParser(&body); err == nil && body.RecaptchaToken != "" {
				token = body.RecaptchaToken
			}
		}

		if token == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "captcha token is required",
			})
		}

		if err := verifyCaptchaToken(ctx, secretKey, token, c.IP()); err != nil {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "captcha verification failed",
			})
		}

		return c.Next()
	}
}
