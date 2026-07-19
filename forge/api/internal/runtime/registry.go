package runtime

import (
	"context"
	"sync"

	"gamepanel/forge/internal/events"
)

type MetricsSnapshot struct {
	RuntimeOperationsTotal        uint64 `json:"runtime_operations_total"`
	RuntimeOperationFailuresTotal uint64 `json:"runtime_operation_failures_total"`
	RuntimeCapabilityChecksTotal  uint64 `json:"runtime_capability_checks_total"`
}

type Registry struct {
	mu        sync.RWMutex
	runtimes  map[string]Runtime
	publisher events.Publisher
	metrics   MetricsSnapshot
}

func NewRegistry(publishers ...events.Publisher) *Registry {
	var publisher events.Publisher
	if len(publishers) > 0 {
		publisher = publishers[0]
	}
	return &Registry{runtimes: map[string]Runtime{}, publisher: publisher}
}

func (r *Registry) Register(runtime Runtime) {
	if r == nil || runtime == nil || runtime.Name() == "" {
		return
	}
	wrapped := &instrumentedRuntime{runtime: runtime, registry: r}
	r.mu.Lock()
	existing, replacing := r.runtimes[runtime.Name()]
	var previous Capabilities
	if replacing {
		previous = existing.Capabilities()
	}
	r.runtimes[runtime.Name()] = wrapped
	r.mu.Unlock()
	r.publish(context.Background(), events.EventRuntimeRegistered, "runtime", runtime.Name(), map[string]any{
		"name":         runtime.Name(),
		"capabilities": runtime.Capabilities(),
	})
	if replacing && previous != runtime.Capabilities() {
		r.publish(context.Background(), events.EventRuntimeCapabilityChanged, "runtime", runtime.Name(), map[string]any{
			"name":     runtime.Name(),
			"previous": previous,
			"current":  runtime.Capabilities(),
		})
	}
}

func (r *Registry) Get(name string) (Runtime, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	runtime, ok := r.runtimes[name]
	r.mu.RUnlock()
	if !ok {
		r.publish(context.Background(), events.EventRuntimeUnavailable, "runtime", name, map[string]any{"name": name})
	}
	return runtime, ok
}

func (r *Registry) Default() Runtime {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if rt, ok := r.runtimes[DockerProvider]; ok {
		return rt
	}
	for _, rt := range r.runtimes {
		return rt
	}
	return nil
}

func (r *Registry) Providers() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	providers := make([]string, 0, len(r.runtimes))
	for name := range r.runtimes {
		providers = append(providers, name)
	}
	return providers
}

func (r *Registry) CheckCapability(name string, capability Capability) bool {
	if r == nil {
		return false
	}
	r.increment(func(metrics *MetricsSnapshot) {
		metrics.RuntimeCapabilityChecksTotal++
	})
	runtime, ok := r.Get(name)
	if !ok {
		return false
	}
	return runtime.Capabilities().Supports(capability)
}

func (r *Registry) Metrics() MetricsSnapshot {
	if r == nil {
		return MetricsSnapshot{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.metrics
}

func (r *Registry) recordOperation(err error) {
	r.increment(func(metrics *MetricsSnapshot) {
		metrics.RuntimeOperationsTotal++
		if err != nil {
			metrics.RuntimeOperationFailuresTotal++
		}
	})
}

func (r *Registry) increment(update func(*MetricsSnapshot)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	update(&r.metrics)
}

func (r *Registry) publish(ctx context.Context, eventType events.EventType, resourceType, resourceID string, payload map[string]any) {
	if r == nil || r.publisher == nil {
		return
	}
	_ = r.publisher.Publish(ctx, events.NewEnvelope(eventType, "runtime-registry", resourceType, resourceID, payload))
}

type instrumentedRuntime struct {
	runtime  Runtime
	registry *Registry
}

func (r *instrumentedRuntime) Name() string {
	return r.runtime.Name()
}

func (r *instrumentedRuntime) Capabilities() Capabilities {
	return r.runtime.Capabilities()
}

func (r *instrumentedRuntime) SupportsMigration() bool {
	r.registry.increment(func(metrics *MetricsSnapshot) {
		metrics.RuntimeCapabilityChecksTotal++
	})
	return r.runtime.SupportsMigration()
}

func (r *instrumentedRuntime) CreateServer(ctx context.Context, target Target, req CreateServerRequest) (CreateResponse, error) {
	response, err := r.runtime.CreateServer(ctx, target, req)
	r.registry.recordOperation(err)
	return response, err
}

func (r *instrumentedRuntime) InstallServer(ctx context.Context, target Target, req InstallRequest) (InstallResponse, error) {
	response, err := r.runtime.InstallServer(ctx, target, req)
	r.registry.recordOperation(err)
	return response, err
}

func (r *instrumentedRuntime) ReinstallServer(ctx context.Context, target Target, req InstallRequest) (InstallResponse, error) {
	reinstaller, ok := r.runtime.(Reinstaller)
	if !ok {
		return InstallResponse{}, ErrNotImplemented
	}
	response, err := reinstaller.ReinstallServer(ctx, target, req)
	r.registry.recordOperation(err)
	return response, err
}

func (r *instrumentedRuntime) SyncServerConfiguration(ctx context.Context, target Target, config ServerConfiguration) error {
	err := r.runtime.SyncServerConfiguration(ctx, target, config)
	r.registry.recordOperation(err)
	return err
}

func (r *instrumentedRuntime) ResizeServer(ctx context.Context, target Target, memoryMB, cpu int64) error {
	err := r.runtime.ResizeServer(ctx, target, memoryMB, cpu)
	r.registry.recordOperation(err)
	return err
}

func (r *instrumentedRuntime) DeleteServer(ctx context.Context, target Target) (PowerResponse, error) {
	response, err := r.runtime.DeleteServer(ctx, target)
	r.registry.recordOperation(err)
	return response, err
}

func (r *instrumentedRuntime) StartServer(ctx context.Context, target Target) (PowerResponse, error) {
	response, err := r.runtime.StartServer(ctx, target)
	r.registry.recordOperation(err)
	return response, err
}

func (r *instrumentedRuntime) StopServer(ctx context.Context, target Target) (PowerResponse, error) {
	response, err := r.runtime.StopServer(ctx, target)
	r.registry.recordOperation(err)
	return response, err
}

func (r *instrumentedRuntime) RestartServer(ctx context.Context, target Target) (PowerResponse, error) {
	response, err := r.runtime.RestartServer(ctx, target)
	r.registry.recordOperation(err)
	return response, err
}

func (r *instrumentedRuntime) KillServer(ctx context.Context, target Target) (PowerResponse, error) {
	response, err := r.runtime.KillServer(ctx, target)
	r.registry.recordOperation(err)
	return response, err
}

func (r *instrumentedRuntime) Stats(ctx context.Context, target Target) (Stats, error) {
	stats, err := r.runtime.Stats(ctx, target)
	r.registry.recordOperation(err)
	return stats, err
}

func (r *instrumentedRuntime) Exists(ctx context.Context, target Target) (bool, error) {
	exists, err := r.runtime.Exists(ctx, target)
	r.registry.recordOperation(err)
	return exists, err
}

func (r *instrumentedRuntime) Inspect(ctx context.Context, target Target) (Inspection, error) {
	inspection, err := r.runtime.Inspect(ctx, target)
	r.registry.recordOperation(err)
	return inspection, err
}

func (r *instrumentedRuntime) PrepareMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	response, err := r.runtime.PrepareMigration(ctx, req)
	r.registry.recordOperation(err)
	return response, err
}

func (r *instrumentedRuntime) ExecuteMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	response, err := r.runtime.ExecuteMigration(ctx, req)
	r.registry.recordOperation(err)
	return response, err
}

func (r *instrumentedRuntime) CancelMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	response, err := r.runtime.CancelMigration(ctx, req)
	r.registry.recordOperation(err)
	return response, err
}
