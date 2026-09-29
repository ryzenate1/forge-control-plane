package http

import (
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"github.com/gofiber/fiber/v2"
)

// PhaseRegistrar wires a build phase's API surface onto the shared Fiber
// routers. Each phase owns exactly one registrar registered via
// RegisterPhaseRegistrar (executed in priority order inside
// registerPhaseHooks).
//
//	v1        — the public /api/v1 group (before session middleware)
//	protected — the authenticated /api/v1 group (session + CSRF + 2FA applied)
//	cfg       — the shared http.Config carrying every service built in main
//
// A registrar has exactly three legitimate outcomes:
//
//	nil                      — every route this phase owns is mounted.
//	%w of ErrPhaseSkipped    — the phase is deliberately not mounted because an
//	                           optional dependency is absent. Reported at WARN,
//	                           does not block startup.
//	any other error          — the phase could not mount its routes. This fails
//	                           startup; see registerPhaseHooks.
//
// Returning nil without mounting the routes is a bug: it reports success for
// work that was not performed. Use ErrPhaseSkipped instead.
type PhaseRegistrar func(v1 fiber.Router, protected fiber.Router, cfg *Config) error

// ErrPhaseSkipped marks a phase that intentionally did not mount its routes
// because an optional dependency was not configured. Wrap it with the reason:
//
//	return fmt.Errorf("%w: store not configured", ErrPhaseSkipped)
//
// A skip is recorded and logged, never silent — an operator can always tell
// which parts of the API surface are absent and why. It is deliberately
// distinct from a nil return so that "not mounted" can never masquerade as
// "mounted".
var ErrPhaseSkipped = errors.New("phase skipped")

type registeredPhase struct {
	name     string
	priority int
	fn       PhaseRegistrar
}

// PhaseStatus is the outcome of one registrar during registerPhaseHooks.
type PhaseStatus struct {
	Name     string
	Priority int
	// Mounted is true only when the registrar returned nil.
	Mounted bool
	// Skipped is true when the registrar returned ErrPhaseSkipped.
	Skipped bool
	// Err is the reason for a skip or a failure; nil when Mounted.
	Err error
}

var (
	phaseMu         sync.Mutex
	phaseRegistrars []registeredPhase

	phaseReportMu sync.Mutex
	phaseReport   []PhaseStatus
)

// RegisterPhaseRegistrar registers a phase route registrar, to be run by
// registerPhaseHooks in ascending priority order.
//
// A nil registrar or a duplicate name is a programming error and panics at
// init time rather than being dropped: silently discarding a registration
// would remove routes from the API surface with no way to notice, which is
// the failure mode this registry exists to prevent. (Compare
// net/http.ServeMux, which panics on a duplicate pattern for the same
// reason.) Priorities must also be unique, so registration order never
// depends on the order the compiler happens to run init() in.
func RegisterPhaseRegistrar(name string, priority int, fn PhaseRegistrar) {
	if fn == nil {
		panic(fmt.Sprintf("http: RegisterPhaseRegistrar(%q): nil registrar", name))
	}
	if name == "" {
		panic("http: RegisterPhaseRegistrar: empty phase name")
	}

	phaseMu.Lock()
	defer phaseMu.Unlock()

	for _, existing := range phaseRegistrars {
		if existing.name == name {
			panic(fmt.Sprintf("http: RegisterPhaseRegistrar(%q): duplicate phase name; "+
				"the second registration would be dropped and its routes would never mount", name))
		}
		if existing.priority == priority {
			panic(fmt.Sprintf("http: RegisterPhaseRegistrar(%q): priority %d already used by %q; "+
				"equal priorities make mount order depend on init() order", name, priority, existing.name))
		}
	}
	phaseRegistrars = append(phaseRegistrars, registeredPhase{name: name, priority: priority, fn: fn})
}

// registerPhaseHooks runs every registered phase registrar in ascending
// priority order and reports what happened.
//
// Every registrar runs even after one fails, so a single broken phase does
// not hide the state of the others — but unlike the previous implementation
// the failures are returned rather than swallowed. The caller must treat a
// non-nil error as fatal: a partially mounted API that reports a successful
// startup is indistinguishable, from the outside, from a healthy panel whose
// routes have quietly become 404s.
//
// Skips (ErrPhaseSkipped) are logged at WARN and do not produce an error.
// Failures are logged at ERROR and are joined into the return value.
// Logging falls back to slog.Default() when cfg.Logger is nil so that a
// missing logger can never make a failure invisible.
func registerPhaseHooks(v1 fiber.Router, protected fiber.Router, cfg *Config) error {
	phaseMu.Lock()
	sorted := append([]registeredPhase(nil), phaseRegistrars...)
	phaseMu.Unlock()

	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].priority < sorted[j].priority })

	log := slog.Default()
	if cfg != nil && cfg.Logger != nil {
		log = cfg.Logger
	}

	report := make([]PhaseStatus, 0, len(sorted))
	var failures []error
	mounted, skipped := 0, 0

	for _, phase := range sorted {
		status := PhaseStatus{Name: phase.name, Priority: phase.priority}

		err := phase.fn(v1, protected, cfg)
		switch {
		case err == nil:
			status.Mounted = true
			mounted++
		case errors.Is(err, ErrPhaseSkipped):
			status.Skipped = true
			status.Err = err
			skipped++
			log.Warn("phase routes not mounted",
				slog.String("phase", phase.name),
				slog.Int("priority", phase.priority),
				slog.String("reason", err.Error()),
			)
		default:
			status.Err = err
			failures = append(failures, fmt.Errorf("phase %q: %w", phase.name, err))
			log.Error("phase registrar failed",
				slog.String("phase", phase.name),
				slog.Int("priority", phase.priority),
				slog.String("error", err.Error()),
			)
		}
		report = append(report, status)
	}

	phaseReportMu.Lock()
	phaseReport = report
	phaseReportMu.Unlock()

	log.Info("phase route registration complete",
		slog.Int("registrars", len(sorted)),
		slog.Int("mounted", mounted),
		slog.Int("skipped", skipped),
		slog.Int("failed", len(failures)),
	)

	if len(failures) > 0 {
		return fmt.Errorf("%d of %d phase registrars failed: %w",
			len(failures), len(sorted), errors.Join(failures...))
	}
	return nil
}

// PhaseRegistrationReport returns the outcome of the most recent
// registerPhaseHooks run, in mount order. It is empty before a server has
// been built. Exposed so startup checks and tests can assert which parts of
// the API surface are actually mounted rather than assuming all of them are.
func PhaseRegistrationReport() []PhaseStatus {
	phaseReportMu.Lock()
	defer phaseReportMu.Unlock()
	return append([]PhaseStatus(nil), phaseReport...)
}
