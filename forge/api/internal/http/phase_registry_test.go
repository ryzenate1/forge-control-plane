package http

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// swapRegistrars replaces the package-level registrar slice for the duration
// of a test and restores it afterwards. The real 16 registrars are installed
// by init() in this package; a test that appended to them would both run
// production wiring and leak state into later tests.
func swapRegistrars(t *testing.T, phases []registeredPhase) {
	t.Helper()
	phaseMu.Lock()
	saved := phaseRegistrars
	phaseRegistrars = phases
	phaseMu.Unlock()
	t.Cleanup(func() {
		phaseMu.Lock()
		phaseRegistrars = saved
		phaseMu.Unlock()
	})
}

func testRouters() (fiber.Router, fiber.Router) {
	app := fiber.New()
	return app.Group("/api/v1"), app.Group("/api/v1")
}

func quietConfig() *Config {
	return &Config{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

// A registrar that cannot mount its routes must fail startup. Before this,
// registerPhaseHooks returned void and the error was dropped, so the API
// booted with a hole in its surface and reported success.
func TestRegisterPhaseHooksReturnsRegistrarFailure(t *testing.T) {
	boom := errors.New("catalog service unreachable")
	swapRegistrars(t, []registeredPhase{
		{name: "ok", priority: 1, fn: func(_, _ fiber.Router, _ *Config) error { return nil }},
		{name: "broken", priority: 2, fn: func(_, _ fiber.Router, _ *Config) error { return boom }},
	})

	v1, protected := testRouters()
	err := registerPhaseHooks(v1, protected, quietConfig())
	if err == nil {
		t.Fatal("registerPhaseHooks returned nil for a failing registrar; a partially mounted API must not report success")
	}
	if !errors.Is(err, boom) {
		t.Fatalf("returned error does not wrap the registrar error: %v", err)
	}
}

// Every registrar runs even after one fails, so one broken phase does not
// mask the state of the others.
func TestRegisterPhaseHooksRunsAllPhasesDespiteFailure(t *testing.T) {
	var ran []string
	mark := func(name string, ret error) PhaseRegistrar {
		return func(_, _ fiber.Router, _ *Config) error {
			ran = append(ran, name)
			return ret
		}
	}
	swapRegistrars(t, []registeredPhase{
		{name: "first", priority: 10, fn: mark("first", errors.New("nope"))},
		{name: "second", priority: 20, fn: mark("second", nil)},
		{name: "third", priority: 30, fn: mark("third", errors.New("also nope"))},
	})

	v1, protected := testRouters()
	err := registerPhaseHooks(v1, protected, quietConfig())
	if err == nil {
		t.Fatal("expected an error covering both failures")
	}
	if len(ran) != 3 {
		t.Fatalf("expected all 3 registrars to run, got %v", ran)
	}
	// Both failures must be represented, not just the first one encountered.
	for _, want := range []string{"nope", "also nope"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("aggregated error %q does not mention failure %q", err, want)
		}
	}
}

// Registrars run in ascending priority order, not init() order.
func TestRegisterPhaseHooksRunsInPriorityOrder(t *testing.T) {
	var ran []string
	mk := func(name string) PhaseRegistrar {
		return func(_, _ fiber.Router, _ *Config) error { ran = append(ran, name); return nil }
	}
	swapRegistrars(t, []registeredPhase{
		{name: "late", priority: 900, fn: mk("late")},
		{name: "early", priority: 10, fn: mk("early")},
		{name: "middle", priority: 100, fn: mk("middle")},
	})

	v1, protected := testRouters()
	if err := registerPhaseHooks(v1, protected, quietConfig()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"early", "middle", "late"}
	for i := range want {
		if ran[i] != want[i] {
			t.Fatalf("phases ran in order %v, want %v", ran, want)
		}
	}
}

// A deliberate skip is reported, not fatal, and never recorded as mounted.
// "Not mounted" must never be indistinguishable from "mounted".
func TestRegisterPhaseHooksSkipIsReportedNotFatal(t *testing.T) {
	swapRegistrars(t, []registeredPhase{
		{name: "mounted-phase", priority: 1, fn: func(_, _ fiber.Router, _ *Config) error { return nil }},
		{name: "skipped-phase", priority: 2, fn: func(_, _ fiber.Router, _ *Config) error {
			return fmt.Errorf("%w: store not configured", ErrPhaseSkipped)
		}},
	})

	v1, protected := testRouters()
	if err := registerPhaseHooks(v1, protected, quietConfig()); err != nil {
		t.Fatalf("a deliberate skip must not fail startup, got: %v", err)
	}

	report := PhaseRegistrationReport()
	if len(report) != 2 {
		t.Fatalf("expected 2 phase statuses, got %d", len(report))
	}
	byName := map[string]PhaseStatus{}
	for _, s := range report {
		byName[s.Name] = s
	}
	if got := byName["mounted-phase"]; !got.Mounted || got.Skipped {
		t.Errorf("mounted-phase recorded as Mounted=%v Skipped=%v", got.Mounted, got.Skipped)
	}
	if got := byName["skipped-phase"]; got.Mounted || !got.Skipped {
		t.Errorf("skipped-phase recorded as Mounted=%v Skipped=%v; a skip must never report as mounted", got.Mounted, got.Skipped)
	}
	if got := byName["skipped-phase"]; !errors.Is(got.Err, ErrPhaseSkipped) {
		t.Errorf("skip reason not retained: %v", got.Err)
	}
}

// A failure must not be hidden just because no logger was injected. The old
// implementation logged only when cfg.Logger != nil, which made every
// failure invisible in tests and in any path that left Logger unset.
func TestRegisterPhaseHooksFailsWithNilLogger(t *testing.T) {
	swapRegistrars(t, []registeredPhase{
		{name: "broken", priority: 1, fn: func(_, _ fiber.Router, _ *Config) error { return errors.New("down") }},
	})

	v1, protected := testRouters()
	if err := registerPhaseHooks(v1, protected, &Config{}); err == nil {
		t.Fatal("failure was swallowed when cfg.Logger was nil")
	}
}

func TestRegisterPhaseRegistrarRejectsDuplicateName(t *testing.T) {
	swapRegistrars(t, nil)
	fn := func(_, _ fiber.Router, _ *Config) error { return nil }
	RegisterPhaseRegistrar("dupe", 1, fn)

	defer func() {
		if recover() == nil {
			t.Fatal("duplicate phase name was accepted; the second registration's routes would never mount")
		}
	}()
	RegisterPhaseRegistrar("dupe", 2, fn)
}

func TestRegisterPhaseRegistrarRejectsDuplicatePriority(t *testing.T) {
	swapRegistrars(t, nil)
	fn := func(_, _ fiber.Router, _ *Config) error { return nil }
	RegisterPhaseRegistrar("first", 50, fn)

	defer func() {
		if recover() == nil {
			t.Fatal("duplicate priority was accepted; mount order would depend on init() order")
		}
	}()
	RegisterPhaseRegistrar("second", 50, fn)
}

func TestRegisterPhaseRegistrarRejectsNilRegistrar(t *testing.T) {
	swapRegistrars(t, nil)
	defer func() {
		if recover() == nil {
			t.Fatal("nil registrar was accepted silently")
		}
	}()
	RegisterPhaseRegistrar("nil-fn", 1, nil)
}

// The real registrars installed by this package's init() must have unique
// names and priorities. This holds by construction now that
// RegisterPhaseRegistrar panics, so package init would already have failed —
// this test states the invariant explicitly and reports the real count.
func TestProductionPhaseRegistrarsAreUnique(t *testing.T) {
	phaseMu.Lock()
	phases := append([]registeredPhase(nil), phaseRegistrars...)
	phaseMu.Unlock()

	if len(phases) == 0 {
		t.Fatal("no phase registrars installed by init()")
	}
	names := map[string]bool{}
	prios := map[int]string{}
	for _, p := range phases {
		if names[p.name] {
			t.Errorf("duplicate phase name %q", p.name)
		}
		names[p.name] = true
		if other, clash := prios[p.priority]; clash {
			t.Errorf("phases %q and %q share priority %d", p.name, other, p.priority)
		}
		prios[p.priority] = p.name
	}
	t.Logf("%d phase registrars installed", len(phases))
}
