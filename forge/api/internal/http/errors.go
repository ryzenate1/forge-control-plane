package http

import (
	"context"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// configLocalKey is the fiber locals key under which the server Config is
// stored for request-scoped helpers such as respondInternalError.
const configLocalKey = "httpConfig"

// ConfigFromCtx returns the Config attached to the request by NewServer, if
// any. Handlers invoked outside a NewServer-built app (e.g. unit tests) fall
// back to zero Config.
func ConfigFromCtx(c *fiber.Ctx) (Config, bool) {
	cfg, ok := c.Locals(configLocalKey).(Config)
	return cfg, ok
}

// isProductionEnv reports whether the request is served from a production
// deployment, used to decide whether internal error details may be exposed.
func isProductionEnv(c *fiber.Ctx) bool {
	if cfg, ok := ConfigFromCtx(c); ok {
		return cfg.AppEnv == "production"
	}
	return false
}

// logInternalError records the full error details server-side. Details are
// never echoed to the client in production.
func logInternalError(c *fiber.Ctx, err error) {
	cfg, ok := ConfigFromCtx(c)
	if !ok || cfg.Logger == nil {
		return
	}
	requestID, _ := c.Locals("requestId").(string)
	cfg.Logger.Error("internal server error",
		"method", c.Method(),
		"path", c.Path(),
		"requestId", requestID,
		"error", err,
	)
}

// respondInternalError returns a generic 500 response. The underlying error
// is logged in full but only surfaced in non-production environments.
// Callers must use this instead of echoing err.Error() directly so internal
// implementation details never leak to clients.
func respondInternalError(c *fiber.Ctx, err error) error {
	if err != nil {
		logInternalError(c, err)
	}
	msg := "an internal error occurred"
	if err != nil && !isProductionEnv(c) {
		msg = err.Error()
	}
	return fiber.NewError(fiber.StatusInternalServerError, msg)
}

// respondStoreError maps a store-layer error onto the HTTP status that
// describes it, falling back to respondInternalError when the error is not a
// recognised client-side condition. Store methods report these conditions as
// error text rather than sentinel values, so the classification is textual;
// resource, uniqueness and dependency failures are the caller's fault and must
// not be reported as 500.
func respondStoreError(c *fiber.Ctx, err error) error {
	if err == nil {
		return nil
	}
	if fe, ok := err.(*fiber.Error); ok {
		return fe
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "not found"), strings.Contains(lower, "no rows"):
		return fiber.NewError(fiber.StatusNotFound, msg)
	case strings.Contains(lower, "already exists"),
		strings.Contains(lower, "duplicate"),
		strings.Contains(lower, "unique constraint"):
		return fiber.NewError(fiber.StatusConflict, msg)
	case strings.Contains(lower, "in use"),
		strings.Contains(lower, "still has"),
		strings.Contains(lower, "foreign key"):
		return fiber.NewError(fiber.StatusConflict, msg)
	case strings.Contains(lower, "invalid"), strings.Contains(lower, "required"),
		strings.Contains(lower, "must be"), strings.Contains(lower, "out of range"),
		strings.Contains(lower, "unsupported"):
		return fiber.NewError(fiber.StatusBadRequest, msg)
	}
	return respondInternalError(c, err)
}

// respondDBProvisionError maps database provisioner/beacon failures onto the
// status that describes them. Client mistakes (unsupported engine, bad input)
// go out as 400; a beacon call that outlives the request goes out as 504 so
// callers can distinguish "timed out, may still complete" from "crashed";
// refused/unreachable beacon connections go out as 503; genuine server-side
// failures keep the previous 500 behaviour.
func respondDBProvisionError(c *fiber.Ctx, err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(lower, "context deadline exceeded"):
		return fiber.NewError(fiber.StatusGatewayTimeout, "database provisioner timed out; the operation may still complete in the background")
	case errors.Is(err, context.Canceled) || strings.Contains(lower, "context canceled"):
		return fiber.NewError(fiber.StatusServiceUnavailable, "database operation was interrupted; retry the request")
	case strings.Contains(lower, "connection refused"),
		strings.Contains(lower, "no such host"),
		strings.Contains(lower, "connection reset"),
		strings.Contains(lower, "is not available"),
		strings.Contains(lower, "not reachable"):
		return fiber.NewError(fiber.StatusServiceUnavailable, msg)
	case strings.Contains(lower, "not found"), strings.Contains(lower, "no rows"):
		return fiber.NewError(fiber.StatusNotFound, msg)
	case strings.Contains(lower, "not yet implemented"), strings.Contains(lower, "not implemented"):
		return fiber.NewError(fiber.StatusNotImplemented, msg)
	case strings.Contains(lower, "not running"),
		strings.Contains(lower, "not provisioned"),
		strings.Contains(lower, "not yet provisioned"),
		strings.Contains(lower, "not in completed state"),
		strings.Contains(lower, "is unavailable"):
		return fiber.NewError(fiber.StatusConflict, msg)
	case strings.Contains(lower, "unsupported"),
		strings.Contains(lower, "invalid"),
		strings.Contains(lower, "required"):
		return fiber.NewError(fiber.StatusBadRequest, msg)
	}
	return respondInternalError(c, err)
}
