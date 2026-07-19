package runtime

import (
	"context"
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
	m.runtimes[provider] = runtime
}

// GetRuntime returns the runtime for a specific provider
func (m *MultiRuntimeAdapter) GetRuntime(provider string) (Runtime, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	runtime, ok := m.runtimes[provider]
	return runtime, ok
}

// getRuntimeForTarget returns the appropriate runtime for a target
func (m *MultiRuntimeAdapter) getRuntimeForTarget(target Target) Runtime {
	if target.Provider != "" {
		if runtime, ok := m.GetRuntime(target.Provider); ok {
			return runtime
		}
	}
	return m.defaultRuntime
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
	rt := m.getRuntimeForTarget(target)
	if rt == nil {
		return CreateResponse{}, ErrRuntimeUnavailable
	}
	return rt.CreateServer(ctx, target, req)
}

// InstallServer installs a server using the appropriate runtime
func (m *MultiRuntimeAdapter) InstallServer(ctx context.Context, target Target, req InstallRequest) (InstallResponse, error) {
	rt := m.getRuntimeForTarget(target)
	if rt == nil {
		return InstallResponse{}, ErrRuntimeUnavailable
	}
	return rt.InstallServer(ctx, target, req)
}

// ReinstallServer reinstalls a server using the appropriate runtime
func (m *MultiRuntimeAdapter) ReinstallServer(ctx context.Context, target Target, req InstallRequest) (InstallResponse, error) {
	rt := m.getRuntimeForTarget(target)
	if rt == nil {
		return InstallResponse{}, ErrRuntimeUnavailable
	}
	if reinstaller, ok := rt.(Reinstaller); ok {
		return reinstaller.ReinstallServer(ctx, target, req)
	}
	return InstallResponse{}, ErrNotImplemented
}

// SyncServerConfiguration syncs server configuration using the appropriate runtime
func (m *MultiRuntimeAdapter) SyncServerConfiguration(ctx context.Context, target Target, config ServerConfiguration) error {
	rt := m.getRuntimeForTarget(target)
	if rt == nil {
		return ErrRuntimeUnavailable
	}
	return rt.SyncServerConfiguration(ctx, target, config)
}

// ResizeServer resizes server resources using the appropriate runtime
func (m *MultiRuntimeAdapter) ResizeServer(ctx context.Context, target Target, memoryMB, cpu int64) error {
	rt := m.getRuntimeForTarget(target)
	if rt == nil {
		return ErrRuntimeUnavailable
	}
	return rt.ResizeServer(ctx, target, memoryMB, cpu)
}

// DeleteServer deletes a server using the appropriate runtime
func (m *MultiRuntimeAdapter) DeleteServer(ctx context.Context, target Target) (PowerResponse, error) {
	rt := m.getRuntimeForTarget(target)
	if rt == nil {
		return PowerResponse{}, ErrRuntimeUnavailable
	}
	return rt.DeleteServer(ctx, target)
}

// StartServer starts a server using the appropriate runtime
func (m *MultiRuntimeAdapter) StartServer(ctx context.Context, target Target) (PowerResponse, error) {
	rt := m.getRuntimeForTarget(target)
	if rt == nil {
		return PowerResponse{}, ErrRuntimeUnavailable
	}
	return rt.StartServer(ctx, target)
}

// StopServer stops a server using the appropriate runtime
func (m *MultiRuntimeAdapter) StopServer(ctx context.Context, target Target) (PowerResponse, error) {
	rt := m.getRuntimeForTarget(target)
	if rt == nil {
		return PowerResponse{}, ErrRuntimeUnavailable
	}
	return rt.StopServer(ctx, target)
}

// RestartServer restarts a server using the appropriate runtime
func (m *MultiRuntimeAdapter) RestartServer(ctx context.Context, target Target) (PowerResponse, error) {
	rt := m.getRuntimeForTarget(target)
	if rt == nil {
		return PowerResponse{}, ErrRuntimeUnavailable
	}
	return rt.RestartServer(ctx, target)
}

// KillServer kills a server using the appropriate runtime
func (m *MultiRuntimeAdapter) KillServer(ctx context.Context, target Target) (PowerResponse, error) {
	rt := m.getRuntimeForTarget(target)
	if rt == nil {
		return PowerResponse{}, ErrRuntimeUnavailable
	}
	return rt.KillServer(ctx, target)
}

// Stats returns server stats using the appropriate runtime
func (m *MultiRuntimeAdapter) Stats(ctx context.Context, target Target) (Stats, error) {
	rt := m.getRuntimeForTarget(target)
	if rt == nil {
		return Stats{}, ErrRuntimeUnavailable
	}
	return rt.Stats(ctx, target)
}

// Exists checks if a server exists using the appropriate runtime
func (m *MultiRuntimeAdapter) Exists(ctx context.Context, target Target) (bool, error) {
	rt := m.getRuntimeForTarget(target)
	if rt == nil {
		return false, ErrRuntimeUnavailable
	}
	return rt.Exists(ctx, target)
}

// Inspect returns server inspection info using the appropriate runtime
func (m *MultiRuntimeAdapter) Inspect(ctx context.Context, target Target) (Inspection, error) {
	rt := m.getRuntimeForTarget(target)
	if rt == nil {
		return Inspection{}, ErrRuntimeUnavailable
	}
	return rt.Inspect(ctx, target)
}

// PrepareMigration prepares a migration using the default runtime
func (m *MultiRuntimeAdapter) PrepareMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	rt := m.getRuntimeForTarget(Target{})
	if rt != nil {
		return rt.PrepareMigration(ctx, req)
	}
	return MigrationResponse{}, ErrNotImplemented
}

// ExecuteMigration executes a migration using the default runtime
func (m *MultiRuntimeAdapter) ExecuteMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	rt := m.getRuntimeForTarget(Target{})
	if rt != nil {
		return rt.ExecuteMigration(ctx, req)
	}
	return MigrationResponse{}, ErrNotImplemented
}

// CancelMigration cancels a migration
func (m *MultiRuntimeAdapter) CancelMigration(ctx context.Context, req MigrationRequest) (MigrationResponse, error) {
	rt := m.getRuntimeForTarget(Target{})
	if rt != nil {
		return rt.CancelMigration(ctx, req)
	}
	return MigrationResponse{}, ErrNotImplemented
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
