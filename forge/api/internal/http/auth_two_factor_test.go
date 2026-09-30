package http

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"gamepanel/forge/internal/store"

	"github.com/gofiber/fiber/v2"
)

func TestTwoFactorSetupPathIsExemptUnderAPIPrefix(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/account/two-factor", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"exempt": isTwoFactorExempt(c)})
	})

	res, err := app.Test(httptest.NewRequest("GET", "/api/v1/account/two-factor", nil))
	if err != nil {
		t.Fatalf("request exemption route: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected status: %d", res.StatusCode)
	}

	var response struct {
		Exempt bool `json:"exempt"`
	}
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Exempt {
		t.Fatal("prefixed two-factor setup route was not exempt")
	}
}

func TestTwoFactorPolicyDoesNotRequireSecretExposure(t *testing.T) {
	user := store.User{UseTOTP: true, TOTPSecret: nil}
	if !userHasTwoFactor(user) {
		t.Fatal("enabled two-factor user was rejected because secret was intentionally omitted")
	}
}
