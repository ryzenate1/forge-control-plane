package runtime

import (
	"context"
	"sort"
	"sync"

	"gamepanel/forge/internal/daemon"
)

// MultiRuntimeAdapter routes runtime operations to the appropriate runtime adapter
// based on the target's runtime provider
type MultiRuntimeAdapter struct {
	mu             sync.RWMutex
	runtimes       map[string]Runtime
	defaultRuntime Runtime
}

// NewMultiRuntimeAdapter creates a new multi-runtime adapter
func NewMultiRuntimeAdapter(defaultRuntime Runtime) *MultiRuntimeAdapter {
	return &MultiRuntimeAdapter{
		runtimes:       make(map[string]Runtime),
		defaultRuntime: defaultRuntime,
	}
}

// Register adds a runtime adapter for a specific provider
func (m *MultiRuntimeAdapter) Register(provider string, runtime Runtime) {
	if runtime == nil || provider == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runtimes[NormalizeProvider(provider)] = runtime
}

// GetRuntime returns the runtime for a specific provider
func (m *MultiRuntimeAdapter) GetRuntime(provider string) (Runtime, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	runtime, ok := m.runtimes[NormalizeProvider(provider)]
	return runtime, ok
}

// RegisteredProvider describes one adapter currently wired into the dispatcher.
type RegisteredProvider struct {
	Provider     string
	Capabilities Capabilities
	Migration    bool
}

// Registered lists the providers this control plane can actually dispatch to,
// each with the capabilities its adapter advertises.
//
// The create-workload surface reads this rather than keeping its own list, so a
// runtime can only be offered when an adapter is genuinely registered — a
// hardcoded menu drifts away from the wiring and ends up advertising engines
// that would fail the moment they were chosen.
func (m *MultiRuntimeAdapter) Registered() []RegisteredProvider {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]RegisteredProvider, 0, len(m.runtimes))
	for name, rt := range m.runtimes {
		out = append(out, RegisteredProvider{
			Provider:     name,
			Capabilities: rt.Capabilities(),
			Migration:    rt.SupportsMigration(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Provider < out[j].Provider })
	return out
}

// resolveRuntime returns the runtime for a target, or an error.
//
// This used to fall through to the default runtime for any provider it did not
// recognise, so asking for LXC or KVM — or a typo — quietly produced a Docker
// container while the request was recorded as honoured. An unrecognised
// provider is now rejected. Falling back to the default remains correct only
// when the provider is one Forge supports but has no dedicated adapter for.
func (m *MultiRuntimeAdapter) resolveRuntime(target Target) (Runtime, error) {
	if err := ValidateProvider(target.Provider); err != nil {
		return nil, err
	}
	if target.Provider != "" {
		if runtime, ok := m.GetRuntime(target.Provider); ok {
			return runtime, nil
		}
	}
	if m.defaultRuntime == nil {
		return nil, ErrRuntimeUnavailable
	}
	return m.defaultRuntime, nil
}

// Name returns the name of this adapter
func (m *MultiRuntimeAdapter) Name() string {
	return "multi-runtime"
}

// Capabilities returns the combined capabilities of all registered runtimes
func (m *MultiRuntimeAdapter) Capabilities() Capabilities {
	var caps Capabilities
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, rt := range m.runtimes {
		caps = caps.Union(rt.Capabilities())
	}
	if m.defaultRuntime != nil {
		caps = caps.Union(m.defaultRuntime.Capabilities())
	}
	return caps
}

// SupportsMigration returns true if any runtime supports migration
func (m *MultiRuntimeAdapter) SupportsMigration() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, rt := range m.runtimes {
		if rt.SupportsMigration() {
			return true
		}
	}
	return m.defaultRuntime != nil && m.defaultRuntime.SupportsMigration()
}

// CreateServer creates a server using the appropriate runtime
func (m *MultiRuntimeAdapter) CreateServer(ctx context.Context, target Target, req CreateServerRequest) (CreateResponse, error) {
	rt, err := m.resolveRuntime(target)
	if err != nil {
		return CreateResponse{}, err
	}
	return rt.CreateServer(ctx, target, req)
}

// InstallServer installs a server using the appropriate runtime
func (m *MultiRuntimeAdapter) InstallServer(ctx context.Context, target Target, req InstallRequest) (InstallResponse, error) {
	rt, err := m.resolveRuntime(target)
	if err != nil {
		return InstallResponse{}, err
	}
	return rt.InstallServer(ctx, target, req)
}

// ReinstallServer reinstalls a server using the appropriate runtime
func (m *MultiRuntimeAdapter) ReinstallServer(ctx context.Context, target Target, req InstallRequest) (InstallResponse, error) {
	rt, err := m.resolveRuntime(target)
	if err != nil {
		return InstallResponse{}, err
	}
	if reinstaller, ok := rt.(Reinstaller); ok {
		return reinstaller.ReinstallServer(ctx, target, req)
	}
	return InstallResponse{}, ErrUnsupportedRuntimeOperation
}

// SyncServerConfiguration syncs server configuration using the appropriate runtime
func (m *MultiRuntimeAdapter) SyncServerConfiguration(ctx context.Context, target Target, config ServerConfiguration) error {
	rt, err := m.resolveRuntime(target)
	if err != nil {
		return err
	}
	return rt.SyncServerConfiguration(ctx, target, config)
}

// ResizeServer resizes server resources using the appropriate runtime
func (m *MultiRuntimeAdapter) ResizeServer(ctx context.Context, target Target, memoryMB, cpu int64) error {
	rt, err := m.resolveRuntime(target)
	if err != nil {
		return err
	}
	return rt.ResizeServer(ctx, target, memoryMB, cpu)
}

// DeleteServer deletes a server using the appropriate runtime
func (m *MultiRuntimeAdapter) DeleteServer(ctx context.Context, target Target) (PowerResponse, error) {
	rt, err := m.resolveRuntime(target)
	if err != nil {
		return PowerResponse{}, err
	}
	return rt.DeleteServer(ctx, target)
}

// StartServer starts a server using the appropriate runtime
func (m *MultiRuntimeAdapter) StartServer(ctx context.Context, target Target) (PowerResponse, error) {
	rt, err := m.resolveRuntime(target)
	if err != nil {
		return PowerResponse{}, err
	}
	return rt.StartServer(ctx, target)
}

// StopServer stops a server using the appropriate runtime
func (m *MultiRuntimeAdapter) StopServer(ctx context.Context, target Target) (PowerResponse, error) {
	rt, err := m.resolveRuntime(target)
	if err != nil {
		return PowerResponse{}, err
	}
	return rt.StopServer(ctx, target)
}

// RestartServer restarts a server using the appropriate runtime
func (m *MultiRuntimeAdapter) RestartServer(ctx context.Context, target Target) (PowerResponse, error) {
	rt, err := m.resolveRuntime(target)
	if err != nil {
		return PowerResponse{}, err
	}
	return rt.RestartServer(ctx, target)
}

// KillServer kills a server using the appropriate runtime
func (m *MultiRuntimeAdapter) KillServer(ctx context.Context, target Target) (PowerResponse, error) {
	rt, err := m.resolveRuntime(target)
	if err != nil {
		return PowerResponse{}, err
	}
	return rt.KillServer(ctx, target)
}

// Stats returns server stats using the appropriate runtime
func (m *MultiRuntimeAdapter) Stats(ctx context.Context, target Target) (Stats, error) {
	rt, err := m.resolveRuntime(target)
	if err != nil {
		return Stats{}, err
	}
	return rt.Stats(ctx, target)
}

// Exists checks if a server exists using the appropriate runtime
func (m *MultiRuntimeAdapter) Exists(ctx context.Context, target Target) (bool, error) {
	rt, err := m.resolveRuntime(target)
	if err != nil {
		return false, err
	}
	return rt.Exists(ctx, target)
}

// Inspect returns server inspection info using the appropriate runtime
func (m *MultiRuntimeAdapter) Inspect(ctx context.Context, target Target) (Inspection, error) {
	rt, err := m.resolveRuntime(target)
	if err != nil {
		return Inspection{}, err
	}
	return rt.Inspect(ctx, target)
}

// PrepareMigration prepares a migration using the default runtime
func (m *MultiRuntimeAdapter) PrepareMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	rt, err := m.resolveRuntime(Target{})
	if err == nil && rt != nil {
		return rt.PrepareMigration(ctx, req)
	}
	return MigrationResponse{}, ErrMigrationManagedByControlPlane
}

// ExecuteMigration executes a migration using the default runtime
func (m *MultiRuntimeAdapter) ExecuteMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	rt, err := m.resolveRuntime(Target{})
	if err == nil && rt != nil {
		return rt.ExecuteMigration(ctx, req)
	}
	return MigrationResponse{}, ErrMigrationManagedByControlPlane
}

// CancelMigration cancels a migration
func (m *MultiRuntimeAdapter) CancelMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	rt, err := m.resolveRuntime(Target{})
	if err == nil && rt != nil {
		return rt.CancelMigration(ctx, req)
	}
	return MigrationResponse{}, ErrMigrationManagedByControlPlane
}

// TransferClient returns the daemon client for transfer operations
func (m *MultiRuntimeAdapter) TransferClient() *daemon.Client {
	if m.defaultRuntime != nil {
		if provider, ok := m.defaultRuntime.(interface{ TransferClient() *daemon.Client }); ok {
			return provider.TransferClient()
		}
	}
	return nil
}
