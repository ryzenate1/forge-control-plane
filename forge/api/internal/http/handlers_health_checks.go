package http

import (
	"time"

	healthchecksvc "gamepanel/forge/internal/services/healthcheckrunner"

	"github.com/gofiber/fiber/v2"
)

// registerHealthCheckRoutes surfaces the live health-check runner
// (services/healthcheckrunner) — a background system that is already started,
// reacting to unhealthy targets and feeding the reconciler. Until now its
// per-target state was not observable over HTTP. These read-only endpoints
// close that gap for the admin Health dashboard.
//
// Registered under /target-health rather than /health/* on purpose: server.go
// owns a /health/:check wildcard (and the web mock treats any "/health" path as
// the health report), so a distinct prefix keeps these routes unambiguous.
func registerHealthCheckRoutes(protected fiber.Router, cfg Config, svc *healthchecksvc.Service) {
	if svc == nil {
		return
	}
	checks := protected.Group("/target-health", requireRole("admin"))

	type targetView struct {
		ID                   string     `json:"id"`
		GroupID              string     `json:"groupId,omitempty"`
		ServerID             string     `json:"serverId,omitempty"`
		Status               string     `json:"status"`
		ConsecutiveFailures  int        `json:"consecutiveFailures"`
		ConsecutiveSuccesses int        `json:"consecutiveSuccesses"`
		SuspectedSince       *time.Time `json:"suspectedSince,omitempty"`
		LastCheckAt          time.Time  `json:"lastCheckAt"`
		LastSuccessAt        *time.Time `json:"lastSuccessAt,omitempty"`
		LastFailureAt        *time.Time `json:"lastFailureAt,omitempty"`
		HealthyThreshold     int        `json:"healthyThreshold"`
		UnhealthyThreshold   int        `json:"unhealthyThreshold"`
	}

	checks.Get("/targets", func(c *fiber.Ctx) error {
		ctx, cancel := requestContext()
		defer cancel()
		states := svc.ListUnhealthyTargets(ctx)
		out := make([]targetView, 0, len(states))
		for _, st := range states {
			if st == nil {
				continue
			}
			out = append(out, targetView{
				ID:                   st.ID,
				GroupID:              st.GroupID,
				ServerID:             st.ServerID,
				Status:               string(st.Status),
				ConsecutiveFailures:  st.ConsecutiveFailures,
				ConsecutiveSuccesses: st.ConsecutiveSuccesses,
				SuspectedSince:       st.SuspectedSince,
				LastCheckAt:          st.LastCheckAt,
				LastSuccessAt:        st.LastSuccessAt,
				LastFailureAt:        st.LastFailureAt,
				HealthyThreshold:     st.HealthyThreshold,
				UnhealthyThreshold:   st.UnhealthyThreshold,
			})
		}
		return c.JSON(fiber.Map{"data": out})
	})

	checks.Get("/metrics", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"data": svc.Metrics()})
	})
}
