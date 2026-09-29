package http

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// TestRespondDBProvisionErrorMapping locks in the status mapping for
// beacon/provisioner failures: client mistakes are 400, missing records are
// 404, state preconditions are 409, downstream timeouts are 504 and
// unreachable beacons are 503. Genuine server failures stay 500.
func TestRespondDBProvisionErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, 0},
		{"unsupported engine", errors.New(`unsupported database engine "oracle"`), http.StatusBadRequest},
		{"invalid input", errors.New("invalid container id"), http.StatusBadRequest},
		{"missing field", errors.New("backupId is required"), http.StatusBadRequest},
		{"no rows", errors.New("no rows in result set"), http.StatusNotFound},
		{"not running", errors.New("database is not running"), http.StatusConflict},
		{"not provisioned", errors.New("database container not provisioned"), http.StatusConflict},
		{"not yet provisioned", errors.New("container not yet provisioned"), http.StatusConflict},
		{"backup state", errors.New("backup is not in completed state"), http.StatusConflict},
		{"unimplemented", errors.New("password rotation via Docker exec not yet implemented"), http.StatusNotImplemented},
		{"deadline text", errors.New(`provision via beacon: Post "http://127.0.0.1:9090/x": context deadline exceeded`), http.StatusGatewayTimeout},
		{"deadline sentinel", context.DeadlineExceeded, http.StatusGatewayTimeout},
		{"connection refused", errors.New("dial tcp 127.0.0.1:9090: connect: connection refused"), http.StatusServiceUnavailable},
		{"genuine failure", errors.New("persist provisioned service: pq: disk full"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New(fiber.Config{DisableStartupMessage: true})
			var got int
			app.Get("/", func(c *fiber.Ctx) error {
				err := respondDBProvisionError(c, tc.err)
				if err == nil {
					return c.SendStatus(http.StatusNoContent)
				}
				var fe *fiber.Error
				if errors.As(err, &fe) {
					got = fe.Code
					return c.Status(fe.Code).SendString(fe.Message)
				}
				return err
			})
			resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			io.Copy(io.Discard, resp.Body)
			if tc.err == nil {
				if resp.StatusCode != http.StatusNoContent {
					t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
				}
				return
			}
			if got != tc.want {
				t.Fatalf("status = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestRespondManagedDBServiceErrorMapping covers the managed-database
// service mapper with the same expectations.
func TestRespondManagedDBServiceErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not running", errors.New("database is not running"), http.StatusConflict},
		{"unimplemented", errors.New("password rotation via Docker exec not yet implemented"), http.StatusNotImplemented},
		{"no rows wrapped", errors.New("get managed database: no rows in result set"), http.StatusNotFound},
		{"genuine failure", errors.New("create backup record: pq: disk full"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := fiber.New(fiber.Config{DisableStartupMessage: true})
			var got int
			app.Get("/", func(c *fiber.Ctx) error {
				err := respondManagedDBServiceError(c, tc.err)
				var fe *fiber.Error
				if errors.As(err, &fe) {
					got = fe.Code
					return c.Status(fe.Code).SendString(fe.Message)
				}
				return err
			})
			resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			io.Copy(io.Discard, resp.Body)
			if got != tc.want {
				t.Fatalf("status = %d, want %d", got, tc.want)
			}
		})
	}
}
