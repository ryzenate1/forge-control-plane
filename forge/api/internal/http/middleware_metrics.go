package http

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// normalizeMetricPath replaces identifier-looking path segments with a
// placeholder so unmatched routes cannot explode metric cardinality.
func normalizeMetricPath(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if len(segment) >= 8 || strings.ContainsAny(segment, "0123456789") {
			segments[i] = ":id"
		}
	}
	return strings.Join(segments, "/")
}

// MetricsMiddleware records RED metrics (Rate, Errors, Duration) for every HTTP request.
// Route labels are bounded via normalizeMetricPath and c.Route().Path so cardinality stays
// limited to the number of registered route templates rather than distinct IDs.
func MetricsMiddleware(collector *MetricsCollector) fiber.Handler {
	if collector == nil {
		collector = NewMetricsCollector()
	}
	return func(c *fiber.Ctx) error {
		collector.IncrementActiveConnections()
		start := time.Now()
		err := c.Next()
		duration := time.Since(start)
		status := c.Response().StatusCode()
		// Fiber's Route may be nil for 404s; fall back to normalized raw path but bound.
		route := ""
		if r := c.Route(); r != nil {
			route = r.Path
		}
		if route == "" {
			route = c.Path()
			// For unmatched routes, collapse to a generic label to avoid high cardinality
			// from random probing paths.
			if route == "" {
				route = "unmatched"
			} else {
				route = normalizeMetricPath(route)
				// Further bound: if the normalized unmatched path is still arbitrary,
				// keep only the first segment.
				if len(route) > 60 {
					route = "/other"
				}
			}
		}
		method := c.Method()
		collector.RecordRequest(method, route, status, duration)
		collector.DecrementActiveConnections()
		if err != nil {
			collector.RecordError("handler_error")
		}
		return err
	}
}
