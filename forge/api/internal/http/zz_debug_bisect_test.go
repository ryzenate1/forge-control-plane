package http

import (
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestDebugBisectRouting(t *testing.T) {
	secret := "test-secret-for-routing-debug-1234567890"
	newApp := func(withV1Use, withProtectedUse bool) *fiber.App {
		app := fiber.New(fiber.Config{DisableStartupMessage: true})
		v1 := app.Group("/api/v1")
		if withV1Use {
			v1.Use(func(c *fiber.Ctx) error { return c.Next() })
		}
		protected := v1.Group("", authMiddleware(secret, nil), func(c *fiber.Ctx) error { return c.Next() })
		if withProtectedUse {
			protected.Use(func(c *fiber.Ctx) error { return c.Next() })
		}
		protected.Post("/auth/password/change", func(c *fiber.Ctx) error {
			fmt.Println("RAN protected-change, matched:", c.Route().Path)
			return c.SendString("protected-change")
		})
		v1.Post("/auth/password/email", func(c *fiber.Ctx) error {
			fmt.Println("RAN public-email, matched:", c.Route().Path)
			return c.SendString("public-email")
		})
		v1.Post("/auth/password/reset", func(c *fiber.Ctx) error {
			return c.SendString("public-reset")
		})
		return app
	}
	for _, combo := range [][2]bool{{false, false}, {true, false}, {false, true}, {true, true}} {
		app := newApp(combo[0], combo[1])
		for _, p := range []string{"/api/v1/auth/password/email", "/api/v1/auth/password/reset"} {
			req := httptest.NewRequest("POST", p, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			res, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(res.Body)
			res.Body.Close()
			fmt.Printf("BISECT v1use=%v protuse=%v %s -> %d %s\n", combo[0], combo[1], p, res.StatusCode, strings.TrimSpace(string(b)))
		}
	}
	_ = fiber.StatusOK
}
