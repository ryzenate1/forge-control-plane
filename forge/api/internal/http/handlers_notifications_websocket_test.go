package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestNotificationWebSocketRequiresUpgrade(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/notifications/ws", handleNotificationWebSocket(nil))
	req := httptest.NewRequest(http.MethodGet, "/notifications/ws", nil)
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("status=%d, want %d", res.StatusCode, http.StatusUpgradeRequired)
	}
}
