package resourcelimits

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Service owns the process-configuration lifecycle: what a process type is
// allowed to consume, how many of it exist, how it is probed, and whether the
// probes say the current release is trustworthy.
//
// It is deliberately transport-agnostic. The HTTP handlers translate Fiber
// requests into these method calls, and the deploy path calls ApplyToCompose —
// so the same rules apply whether a limit was set from the panel, the API or a
// future CLI.
type Service struct {
	store Store
	// now is a seam for the grace-window maths. Production uses time.Now.
	now func() time.Time
}

// New builds a Service over any Store implementation.
func New(store Store) *Service {
	if store == nil {
		panic("resourcelimits: New requires a non-nil store")
	}
	return &Service{store: store, now: time.Now}
}

// NewFromPool builds the production Service over the shared Postgres pool.
func NewFromPool(db *pgxpool.Pool) *Service {
	return New(NewPostgresStore(db))
}

// Store exposes the underlying persistence for the few callers (the prune job)
// that need direct access.
func (s *Service) Store() Store {
	return s.store
}

// ---------------------------------------------------------------------------
// Process configuration CRUD
// ---------------------------------------------------------------------------

// ListProcessConfigs returns every stored configuration for an application,
// ordered by process type.
func (s *Service) ListProcessConfigs(ctx context.Context, applicationID string) ([]ProcessConfig, error) {
	if strings.TrimSpace(applicationID) == "" {
		return nil, invalidf("application id is required")
	}
	return s.store.ListProcessConfigs(ctx, applicationID)
}

// GetProcessConfig returns one (application, process type) row.
func (s *Service) GetProcessConfig(ctx context.Context, applicationID, processType string) (ProcessConfig, error) {
	normalized, err := NormalizeProcessType(processType)
	if err != nil {
		return ProcessConfig{}, err
	}
	return s.store.GetProcessConfig(ctx, applicationID, normalized)
}

// UpsertProcessConfig applies a partial payload to one process type.
//
// Merge semantics matter because the UI edits one card at a time: a field the
// payload omits keeps its stored value, while a field set to zero on the two
// limit columns means "remove the constraint" (see the note on
// ProcessConfigInput). A process type with no stored row starts from the
// documented defaults — one replica, enabled, gating on.
func (s *Service) UpsertProcessConfig(ctx context.Context, applicationID, processType string, in ProcessConfigInput) (ProcessConfig, error) {
	if strings.TrimSpace(applicationID) == "" {
		return ProcessConfig{}, invalidf("application id is required")
	}
	normalized, err := NormalizeProcessType(processType)
	if err != nil {
		return ProcessConfig{}, err
	}

	existing, err := s.store.GetProcessConfig(ctx, applicationID, normalized)
	created := false
	switch {
	case err == nil:
		// Merge onto the stored row below.
	case errors.Is(err, ErrProcessConfigNotFound):
		existing = ProcessConfig{
			ApplicationID:     applicationID,
			ProcessType:       normalized,
			Replicas:          1,
			Enabled:           true,
			HealthCheckGating: true,
		}
		created = true
	default:
		return ProcessConfig{}, err
	}
	existing.ApplicationID = applicationID
	existing.ProcessType = normalized

	if in.CPULimit != nil {
		value := *in.CPULimit
		existing.CPULimit = &value
	}
	if in.MemoryLimit != nil {
		value := *in.MemoryLimit
		existing.MemoryLimit = &value
	}
	if in.MemoryLimitMB != nil {
		mb := *in.MemoryLimitMB
		if mb < 0 {
			return ProcessConfig{}, invalidf("memory limit (MB) must not be negative")
		}
		// A zero MB value is the UI's "no limit": BytesPerMB arithmetic would
		// otherwise turn it into a nonsensical zero-byte constraint.
		if mb == 0 {
			existing.MemoryLimit = nil
		} else {
			bytes := mb * BytesPerMB
			existing.MemoryLimit = &bytes
		}
	}
	// Replicas is a pointer precisely so "the card did not touch it" and
	// "the operator typed 0" are distinguishable here.
	if in.Replicas != nil {
		existing.Replicas = *in.Replicas
	}
	if in.Enabled != nil {
		existing.Enabled = *in.Enabled
	}
	if in.HealthCheckGating != nil {
		existing.HealthCheckGating = *in.HealthCheckGating
	}
	if in.HealthCheckType != nil {
		existing.HealthCheckType = HealthCheckType(strings.ToLower(strings.TrimSpace(*in.HealthCheckType)))
		if !existing.HealthCheckType.Valid() {
			return ProcessConfig{}, invalidf("health check type must be one of http, tcp, command or empty to clear")
		}
	}
	if in.HealthCheckPath != nil {
		existing.HealthCheckPath = *in.HealthCheckPath
	}
	if in.HealthCheckPort != nil {
		existing.HealthCheckPort = *in.HealthCheckPort
	}
	if in.HealthCheckCommand != nil {
		existing.HealthCheckCommand = *in.HealthCheckCommand
	}
	if in.HealthCheckInterval != nil {
		existing.HealthCheckInterval = *in.HealthCheckInterval
	}
	if in.HealthCheckTimeout != nil {
		existing.HealthCheckTimeout = *in.HealthCheckTimeout
	}
	if in.HealthCheckRetries != nil {
		existing.HealthCheckRetries = *in.HealthCheckRetries
	}
	if in.HealthCheckStartPeriod != nil {
		existing.HealthCheckStartPeriod = *in.HealthCheckStartPeriod
	}

	// On create, an omitted replicas field must land as one replica rather than
	// the struct zero, which compose would refuse to render.
	if created && in.Replicas == nil {
		existing.Replicas = 1
	}

	// Normalize only fills defaults for a process that actually has a check; an
	// empty type clears the probe fields, which is how "no check" is stored.
	// Both are caller errors, so any future plain errors.New inside them still
	// reaches the handler as a 400 rather than a 500.
	if err := existing.Normalize(); err != nil {
		return ProcessConfig{}, asValidation(err)
	}
	if err := existing.Validate(); err != nil {
		return ProcessConfig{}, asValidation(err)
	}
	// Validate bounds the retries window; Normalize leaves Replicas alone, so
	// the upper bound is checked here where the value is final.
	if existing.Replicas > MaxReplicas {
		return ProcessConfig{}, invalidf("replicas must be between 0 and %d", MaxReplicas)
	}
	return s.store.UpsertProcessConfig(ctx, existing)
}

// DeleteProcessConfig removes a process type's configuration. It reports
// whether a row actually went away so the handler can answer 404 instead of a
// confident 200 for a process type that never existed.
func (s *Service) DeleteProcessConfig(ctx context.Context, applicationID, processType string) (bool, error) {
	normalized, err := NormalizeProcessType(processType)
	if err != nil {
		return false, err
	}
	return s.store.DeleteProcessConfig(ctx, applicationID, normalized)
}

// ScaleProcess is the Dokku `ps:scale web=3` shortcut: it changes only the
// replica count and refuses to invent a configuration row, because scaling a
// process type nobody has defined would report success for a no-op.
func (s *Service) ScaleProcess(ctx context.Context, applicationID, processType string, replicas int) (ProcessConfig, error) {
	if replicas < 0 {
		return ProcessConfig{}, invalidf("replicas must not be negative")
	}
	if replicas > MaxReplicas {
		return ProcessConfig{}, invalidf("replicas must be between 0 and %d", MaxReplicas)
	}
	normalized, err := NormalizeProcessType(processType)
	if err != nil {
		return ProcessConfig{}, err
	}
	current, err := s.store.GetProcessConfig(ctx, applicationID, normalized)
	if err != nil {
		return ProcessConfig{}, err
	}
	if current.Replicas == replicas {
		return current, nil
	}
	value := replicas
	return s.UpsertProcessConfig(ctx, applicationID, normalized, ProcessConfigInput{Replicas: &value})
}

// GetApplicationByServer resolves the application a server tab should edit.
// Returns ErrNoBindableApplication when the server hosts no application.
func (s *Service) GetApplicationByServer(ctx context.Context, serverID string) (ApplicationRef, error) {
	if strings.TrimSpace(serverID) == "" {
		return ApplicationRef{}, invalidf("server id is required")
	}
	return s.store.GetApplicationByServer(ctx, serverID)
}

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

// ReportHealth records one probe result, matching the signature the deploy and
// monitoring paths call. The application is resolved from the server so a
// Beacon-side reporter does not need to know app identity; when the server
// hosts no application the observation is still stored with a null application
// id — losing the evidence would be worse than an unjoinable row.
func (s *Service) ReportHealth(ctx context.Context, serverID, processType string, healthy bool, detail string) error {
	_, err := s.RecordHealth(ctx, HealthObservation{
		ServerID:    serverID,
		ProcessType: processType,
		Healthy:     healthy,
		Detail:      detail,
	})
	return err
}

// RecordHealth is ReportHealth plus resolved identity, so the caller can show
// which application a row belongs to.
func (s *Service) RecordHealth(ctx context.Context, obs HealthObservation) (HealthObservation, error) {
	serverID := strings.TrimSpace(obs.ServerID)
	if serverID == "" {
		return HealthObservation{}, invalidf("health observations must name the server they came from")
	}
	processType, err := NormalizeProcessType(obs.ProcessType)
	if err != nil {
		return HealthObservation{}, err
	}
	obs.ServerID = serverID
	obs.ProcessType = processType
	if obs.ObservedAt.IsZero() {
		obs.ObservedAt = s.now().UTC()
	}
	if obs.ApplicationID == nil || *obs.ApplicationID == "" {
		if ref, err := s.store.GetApplicationByServer(ctx, serverID); err == nil {
			obs.ApplicationID = &ref.ID
		} else if !errors.Is(err, ErrNoBindableApplication) {
			return HealthObservation{}, err
		}
	}
	if len(obs.Detail) > 2000 {
		obs.Detail = obs.Detail[:2000]
	}
	return s.store.InsertHealthObservation(ctx, obs)
}

// ListServerHealth returns the most recent observations for one server, newest
// first. This is the `/servers/:serverId/health` history endpoint's whole job.
func (s *Service) ListServerHealth(ctx context.Context, serverID string, limit int) ([]HealthObservation, error) {
	if strings.TrimSpace(serverID) == "" {
		return nil, invalidf("server id is required")
	}
	return s.store.ListHealthObservationsByServer(ctx, serverID, limit)
}

// HealthStatus rolls the stored configurations and the observation log up into
// one row per process type — what the UI paints, and the reasoning
// RollbackDecision is built from.
func (s *Service) HealthStatus(ctx context.Context, applicationID string) ([]ProcessHealthState, error) {
	if strings.TrimSpace(applicationID) == "" {
		return nil, invalidf("application id is required")
	}
	configs, err := s.store.ListProcessConfigs(ctx, applicationID)
	if err != nil {
		return nil, err
	}
	observations, err := s.recentObservations(ctx, applicationID, configs)
	if err != nil {
		return nil, err
	}

	states := make([]ProcessHealthState, 0, len(configs))
	for _, cfg := range configs {
		states = append(states, s.evaluate(cfg, observations))
	}
	sort.Slice(states, func(i, j int) bool { return states[i].ProcessType < states[j].ProcessType })
	return states, nil
}

// recentObservations loads everything the widest grace window among configs
// could still care about, in one query — instead of one query per process type.
func (s *Service) recentObservations(ctx context.Context, applicationID string, configs []ProcessConfig) ([]HealthObservation, error) {
	window := 0
	for _, cfg := range configs {
		if w := cfg.GraceWindowSeconds(); w > window {
			window = w
		}
	}
	if window == 0 {
		// Nothing gates, so nothing is evaluated; still return an empty slice
		// rather than a nil that callers would have to re-check.
		return []HealthObservation{}, nil
	}
	since := s.now().UTC().Add(-time.Duration(window) * time.Second)
	return s.store.ListHealthObservationsByApplication(ctx, applicationID, since)
}

// evaluate turns one config plus the observation log into one UI row.
//
// The distinction it exists to enforce: an unhealthy probe and no probe are
// different facts, and collapsing them would let a deploy roll back because
// nobody reported anything, or let it stand because nobody reported anything.
func (s *Service) evaluate(cfg ProcessConfig, observations []HealthObservation) ProcessHealthState {
	state := ProcessHealthState{
		ProcessType:  cfg.ProcessType,
		Replicas:     cfg.Replicas,
		CPULimit:     cfg.CPULimit,
		MemoryLimit:  cfg.MemoryLimit,
		HealthCheck:  string(cfg.HealthCheckType),
		Gating:       cfg.GatesDeploy(),
		Enabled:      cfg.Enabled,
		GraceSeconds: cfg.GraceWindowSeconds(),
		Status:       HealthStatusUnknown,
	}
	if !cfg.Enabled {
		state.Detail = "the configuration is disabled, so nothing is enforced and nothing is probed"
		return state
	}
	if !cfg.HasHealthCheck() {
		state.Detail = "no health check is configured; the release is not verified"
		return state
	}

	cutoff := s.now().UTC().Add(-time.Duration(state.GraceSeconds) * time.Second)
	var latest *HealthObservation
	var stale bool
	for i := range observations {
		obs := observations[i]
		if obs.ProcessType != cfg.ProcessType {
			continue
		}
		if obs.ObservedAt.Before(cutoff) {
			// Observations arrive newest-first, so anything seen after this
			// point is older still.
			stale = latest == nil
			continue
		}
		if latest == nil {
			obsCopy := obs
			latest = &obsCopy
			observedAt := obs.ObservedAt
			state.LastObservedAt = &observedAt
		}
		state.ObservedCount++
	}

	switch {
	case latest != nil && latest.Healthy:
		state.Status = HealthStatusHealthy
		if latest.Detail != "" {
			state.Detail = latest.Detail
		}
	case latest != nil:
		state.Status = HealthStatusUnhealthy
		state.Detail = latest.Detail
		if state.Detail == "" {
			state.Detail = "the most recent probe failed"
		}
	case stale:
		state.Status = HealthStatusUnknown
		state.Detail = fmt.Sprintf("the newest probe is older than the %ds grace window", state.GraceSeconds)
	default:
		state.Status = HealthStatusPending
		state.Detail = "no probe result has been reported within the grace window yet"
	}
	return state
}

// RollbackDecision is ShouldRollback's answer with the reasoning attached, so
// the UI and the deploy path report the same thing.
type RollbackDecision struct {
	Rollback      bool                 `json:"rollback"`
	Reason        string               `json:"reason"`
	WindowSeconds int                  `json:"graceWindowSeconds"`
	Gating        []ProcessHealthState `json:"gatingProcesses"`
	CheckedAt     time.Time            `json:"checkedAt"`
}

// ShouldRollback reports whether the current release failed its own health
// checks inside the grace window — Dokku `checks` behaviour, where a failing
// check aborts the deploy.
//
// Absence of evidence never triggers a rollback: a process nobody has probed
// yet, or a check nobody disabled but that simply has not reported, rolls
// nothing back. That asymmetry is deliberate — auto-rollback on silence would
// turn a broken reporter into a stopped application.
func (s *Service) ShouldRollback(ctx context.Context, applicationID string) (bool, error) {
	decision, err := s.RollbackDecision(ctx, applicationID)
	if err != nil {
		return false, err
	}
	return decision.Rollback, nil
}

// RollbackDecision is the explained form of ShouldRollback.
func (s *Service) RollbackDecision(ctx context.Context, applicationID string) (RollbackDecision, error) {
	configs, err := s.store.ListProcessConfigs(ctx, applicationID)
	if err != nil {
		return RollbackDecision{}, err
	}
	observations, err := s.recentObservations(ctx, applicationID, configs)
	if err != nil {
		return RollbackDecision{}, err
	}

	gating := make([]ProcessHealthState, 0, len(configs))
	decision := RollbackDecision{CheckedAt: s.now().UTC(), Gating: gating}
	for _, cfg := range configs {
		if !cfg.GatesDeploy() {
			continue
		}
		if w := cfg.GraceWindowSeconds(); w > decision.WindowSeconds {
			decision.WindowSeconds = w
		}
		decision.Gating = append(decision.Gating, s.evaluate(cfg, observations))
	}
	sort.Slice(decision.Gating, func(i, j int) bool {
		return decision.Gating[i].ProcessType < decision.Gating[j].ProcessType
	})
	if len(decision.Gating) == 0 {
		decision.Reason = "no process type gates deploys on health, so there is nothing to roll back"
		return decision, nil
	}

	var failed, pending []string
	healthy := 0
	for _, state := range decision.Gating {
		switch state.Status {
		case HealthStatusUnhealthy:
			failed = append(failed, state.ProcessType)
		case HealthStatusHealthy:
			healthy++
		default:
			pending = append(pending, state.ProcessType)
		}
	}
	switch {
	case len(failed) > 0:
		decision.Rollback = true
		decision.Reason = fmt.Sprintf("health check failed for %s within the %ds grace window", strings.Join(failed, ", "), decision.WindowSeconds)
	case len(pending) > 0:
		decision.Reason = fmt.Sprintf("%d of %d gating process types have not reported a passing probe yet (%s)",
			len(pending), len(decision.Gating), strings.Join(pending, ", "))
	default:
		decision.Reason = fmt.Sprintf("all %d gating process types report healthy", healthy)
	}
	return decision, nil
}

// PruneHealthObservations trims the append-only probe log.
func (s *Service) PruneHealthObservations(ctx context.Context, cutoff time.Time) (int64, error) {
	if cutoff.IsZero() {
		return 0, invalidf("prune cutoff is required")
	}
	if cutoff.After(s.now()) {
		return 0, invalidf("prune cutoff must be in the past")
	}
	return s.store.PruneHealthObservations(ctx, cutoff)
}
