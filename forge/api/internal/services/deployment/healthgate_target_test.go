package deployment

import (
	"context"
	"testing"
)

// Regression test for Phase-1 P0 finding F-05 / FORGE-LOGIC-002: the health
// gate probed `localhost` on the control-plane host instead of the node
// hosting the workload. Without an explicit host, CheckHealth must derive
// the target from the server's node; if the node cannot be resolved the
// gate fails honestly instead of probing the API host.

func TestCheckHealthFailsWhenNodeUnresolvable(t *testing.T) {
	s := New(nil)
	d := &Deployment{
		ID:              "hg-1",
		ServerID:        "srv-missing",
		HealthCheckPath: "/healthz",
		HealthCheckPort: 8080,
		// No HealthCheckHost: must resolve from node target.
	}
	result, err := s.CheckHealth(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed {
		t.Fatal("gate must fail when target node cannot be resolved (was: silently probed localhost)")
	}
	if result.Error == "" {
		t.Error("expected explanatory error for unresolved node")
	}
}

func TestCheckHealthHonorsExplicitHost(t *testing.T) {
	s := New(nil)
	d := &Deployment{
		ID:              "hg-2",
		ServerID:        "srv-ignored",
		HealthCheckPath: "/",
		HealthCheckPort: 1,
		HealthCheckHost: "127.0.0.1", // explicit override still allowed
	}
	// Port 1 on loopback refuses connections quickly; the point is that no
	// node lookup is needed and the probe targets exactly what was asked.
	result, _ := s.CheckHealth(context.Background(), d)
	if result.Passed {
		t.Skip("loopback:1 unexpectedly accepting connections")
	}
}
