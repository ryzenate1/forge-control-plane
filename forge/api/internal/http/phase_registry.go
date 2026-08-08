package http

import (
	"log/slog"
	"sort"

	"github.com/gofiber/fiber/v2"
)

// PhaseRegistrar wires a build phase's API surface onto the shared Fiber
// routers. Each phase owns exactly one registrar registered via
// RegisterPhaseRegistrar (idempotent, executed in priority order inside
// registerPhaseHooks).
//
//	v1        — the public /api/v1 group (before session middleware)
//	protected — the authenticated /api/v1 group (session + CSRF + 2FA applied)
//	cfg       — the shared http.Config carrying every service built in main
type PhaseRegistrar func(v1 fiber.Router, protected fiber.Router, cfg *Config) error

type registeredPhase struct {
	name     string
	priority int
	fn       PhaseRegistrar
}

var phaseRegistrars []registeredPhase

// RegisterPhaseRegistrar registers a phase route registrar. Duplicate names
// are ignored (first registration wins) so the codebase stays rerunnable.
func RegisterPhaseRegistrar(name string, priority int, fn PhaseRegistrar) {
	if fn == nil {
		return
	}
	for _, existing := range phaseRegistrars {
		if existing.name == name {
			return
		}
	}
	phaseRegistrars = append(phaseRegistrars, registeredPhase{name: name, priority: priority, fn: fn})
}

// registerPhaseHooks runs every registered phase registrar in priority order.
// Failures are logged but do not prevent later phases from registering — an
// immutable contract so deployment observability is unaffected.
func registerPhaseHooks(v1 fiber.Router, protected fiber.Router, cfg *Config) {
	sorted := append([]registeredPhase(nil), phaseRegistrars...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].priority < sorted[j].priority })
	for _, phase := range sorted {
		if err := phase.fn(v1, protected, cfg); err != nil {
			if cfg.Logger != nil {
				cfg.Logger.Error("phase registrar failed",
					slog.String("phase", phase.name),
					slog.String("error", err.Error()),
				)
			}
		}
	}
}