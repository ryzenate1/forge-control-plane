package runtime

import (
	"context"
	"testing"

	"gamepanel/forge/internal/events"
)

type mockPublisher struct {
	published []events.Envelope
}

func (m *mockPublisher) Publish(_ context.Context, envelope events.Envelope) error {
	m.published = append(m.published, envelope)
	return nil
}

type registryRuntimeStub struct {
	name             string
	caps             Capabilities
	supportsMigrate  bool
}

func (r *registryRuntimeStub) Name() string                                { return r.name }
func (r *registryRuntimeStub) Capabilities() Capabilities                   { return r.caps }
func (r *registryRuntimeStub) SupportsMigration() bool                     { return r.supportsMigrate }
func (r *registryRuntimeStub) CreateServer(context.Context, Target, CreateServerRequest) (CreateResponse, error) {
	return CreateResponse{}, nil
}
func (r *registryRuntimeStub) InstallServer(context.Context, Target, InstallRequest) (InstallResponse, error) {
	return InstallResponse{}, nil
}
func (r *registryRuntimeStub) SyncServerConfiguration(context.Context, Target, ServerConfiguration) error { return nil }
func (r *registryRuntimeStub) DeleteServer(context.Context, Target) (PowerResponse, error)                 { return PowerResponse{}, nil }
func (r *registryRuntimeStub) StartServer(context.Context, Target) (PowerResponse, error)                  { return PowerResponse{}, nil }
func (r *registryRuntimeStub) StopServer(context.Context, Target) (PowerResponse, error)                   { return PowerResponse{}, nil }
func (r *registryRuntimeStub) RestartServer(context.Context, Target) (PowerResponse, error)                { return PowerResponse{}, nil }
func (r *registryRuntimeStub) KillServer(context.Context, Target) (PowerResponse, error)                   { return PowerResponse{}, nil }
func (r *registryRuntimeStub) ResizeServer(context.Context, Target, int64, int64) error                   { return nil }
func (r *registryRuntimeStub) Stats(context.Context, Target) (Stats, error)                                { return Stats{}, nil }
func (r *registryRuntimeStub) Exists(context.Context, Target) (bool, error)                                { return true, nil }
func (r *registryRuntimeStub) Inspect(context.Context, Target) (Inspection, error)                         { return Inspection{}, nil }
func (r *registryRuntimeStub) PrepareMigration(context.Context, MigrationRequest) (MigrationResponse, error) {
	return MigrationResponse{}, nil
}
func (r *registryRuntimeStub) ExecuteMigration(context.Context, MigrationRequest) (MigrationResponse, error) {
	return MigrationResponse{}, nil
}
func (r *registryRuntimeStub) CancelMigration(context.Context, MigrationRequest) (MigrationResponse, error) {
	return MigrationResponse{}, nil
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry should not return nil")
	}
	if r.Metrics() != (MetricsSnapshot{}) {
		t.Error("New registry should have zero metrics")
	}
}

func TestNewRegistryWithPublisher(t *testing.T) {
	mp := &mockPublisher{}
	r := NewRegistry(mp)
	if r == nil {
		t.Fatal("NewRegistry with publisher should not return nil")
	}
}

func TestRegistryRegisterNil(t *testing.T) {
	r := NewRegistry()
	r.Register(nil)
	if len(r.Providers()) != 0 {
		t.Error("Register nil should not add any provider")
	}
}

func TestRegistryRegisterEmptyName(t *testing.T) {
	r := NewRegistry()
	r.Register(&registryRuntimeStub{name: ""})
	if len(r.Providers()) != 0 {
		t.Error("Register with empty name should not add any provider")
	}
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	stub := &registryRuntimeStub{name: "test-runtime"}
	r.Register(stub)
	got, ok := r.Get("test-runtime")
	if !ok {
		t.Fatal("Get should find registered runtime")
	}
	if got.Name() != "test-runtime" {
		t.Errorf("Get returned runtime with name %q", got.Name())
	}
}

func TestRegistryGetNotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("Get for nonexistent should return false")
	}
}

func TestRegistryGetOnNil(t *testing.T) {
	var r *Registry
	rt, ok := r.Get("anything")
	if rt != nil || ok {
		t.Error("Get on nil Registry should return nil, false")
	}
}

func TestRegistryDefaultPrefersDocker(t *testing.T) {
	r := NewRegistry()
	stub := &registryRuntimeStub{name: "other"}
	dockerStub := &registryRuntimeStub{name: DockerProvider}
	r.Register(dockerStub)
	r.Register(stub)
	def := r.Default()
	if def == nil {
		t.Fatal("Default should return a runtime")
	}
	if def.Name() != DockerProvider {
		t.Errorf("Default should prefer Docker, got %q", def.Name())
	}
}

func TestRegistryDefaultFallsBackToFirst(t *testing.T) {
	r := NewRegistry()
	r.Register(&registryRuntimeStub{name: "only-runtime"})
	def := r.Default()
	if def == nil || def.Name() != "only-runtime" {
		t.Errorf("Default should fall back to the only runtime, got %v", def)
	}
}

func TestRegistryDefaultEmpty(t *testing.T) {
	r := NewRegistry()
	def := r.Default()
	if def != nil {
		t.Error("Default should be nil when no runtimes registered")
	}
}

func TestRegistryDefaultOnNil(t *testing.T) {
	var r *Registry
	def := r.Default()
	if def != nil {
		t.Error("Default on nil Registry should return nil")
	}
}

func TestRegistryProviders(t *testing.T) {
	r := NewRegistry()
	r.Register(&registryRuntimeStub{name: "a"})
	r.Register(&registryRuntimeStub{name: "b"})
	providers := r.Providers()
	if len(providers) != 2 {
		t.Errorf("Providers should return 2 entries, got %d", len(providers))
	}
	m := make(map[string]bool)
	for _, p := range providers {
		m[p] = true
	}
	if !m["a"] || !m["b"] {
		t.Error("Providers should include both registered names")
	}
}

func TestRegistryProvidersEmpty(t *testing.T) {
	r := NewRegistry()
	providers := r.Providers()
	if len(providers) != 0 {
		t.Errorf("Providers should be empty, got %v", providers)
	}
}

func TestRegistryProvidersOnNil(t *testing.T) {
	var r *Registry
	providers := r.Providers()
	if providers != nil {
		t.Error("Providers on nil Registry should return nil")
	}
}

func TestRegistryCheckCapabilityTrue(t *testing.T) {
	r := NewRegistry()
	stub := &registryRuntimeStub{name: "test", caps: Capabilities{Containers: true}}
	r.Register(stub)
	if !r.CheckCapability("test", CapabilityContainers) {
		t.Error("CheckCapability should return true when runtime supports capability")
	}
}

func TestRegistryCheckCapabilityFalse(t *testing.T) {
	r := NewRegistry()
	stub := &registryRuntimeStub{name: "test", caps: Capabilities{Containers: false}}
	r.Register(stub)
	if r.CheckCapability("test", CapabilityContainers) {
		t.Error("CheckCapability should return false when runtime does not support capability")
	}
}

func TestRegistryCheckCapabilityNotFound(t *testing.T) {
	r := NewRegistry()
	if r.CheckCapability("nonexistent", CapabilityContainers) {
		t.Error("CheckCapability should return false for unknown runtime")
	}
}

func TestRegistryCheckCapabilityOnNil(t *testing.T) {
	var r *Registry
	if r.CheckCapability("anything", CapabilityContainers) {
		t.Error("CheckCapability on nil Registry should return false")
	}
}

func TestRegistryCheckCapabilityIncrementsMetric(t *testing.T) {
	r := NewRegistry()
	stub := &registryRuntimeStub{name: "test", caps: Capabilities{Containers: true}}
	r.Register(stub)
	r.CheckCapability("test", CapabilityContainers)
	metrics := r.Metrics()
	if metrics.RuntimeCapabilityChecksTotal != 1 {
		t.Errorf("Capability checks metric should be 1, got %d", metrics.RuntimeCapabilityChecksTotal)
	}
}

func TestRegistryMetricsInitial(t *testing.T) {
	r := NewRegistry()
	metrics := r.Metrics()
	if metrics.RuntimeOperationsTotal != 0 || metrics.RuntimeOperationFailuresTotal != 0 || metrics.RuntimeCapabilityChecksTotal != 0 {
		t.Error("Initial metrics should be all zero")
	}
}

func TestRegistryMetricsOnNil(t *testing.T) {
	var r *Registry
	metrics := r.Metrics()
	if metrics != (MetricsSnapshot{}) {
		t.Error("Metrics on nil Registry should return zero value")
	}
}

func TestRegistryRegisterPublishesEvent(t *testing.T) {
	mp := &mockPublisher{}
	r := NewRegistry(mp)
	stub := &registryRuntimeStub{name: "test", caps: Capabilities{Containers: true}}
	r.Register(stub)
	if len(mp.published) < 1 {
		t.Fatal("Register should publish an event")
	}
	if mp.published[0].Type != events.EventRuntimeRegistered {
		t.Errorf("Event type = %v, want %v", mp.published[0].Type, events.EventRuntimeRegistered)
	}
	if mp.published[0].ResourceID != "test" {
		t.Errorf("ResourceID = %q, want %q", mp.published[0].ResourceID, "test")
	}
}

func TestRegistryRegisterReplacePublishesCapabilityChanged(t *testing.T) {
	mp := &mockPublisher{}
	r := NewRegistry(mp)
	r.Register(&registryRuntimeStub{name: "test", caps: Capabilities{Containers: true}})
	mp.published = nil
	r.Register(&registryRuntimeStub{name: "test", caps: Capabilities{Containers: false}})
	foundChanged := false
	for _, e := range mp.published {
		if e.Type == events.EventRuntimeCapabilityChanged {
			foundChanged = true
			break
		}
	}
	if !foundChanged {
		t.Error("Replacing a runtime with different capabilities should publish capability changed event")
	}
}

func TestRegistryRegisterReplaceSameCapabilitiesNoChangedEvent(t *testing.T) {
	mp := &mockPublisher{}
	r := NewRegistry(mp)
	r.Register(&registryRuntimeStub{name: "test", caps: Capabilities{Containers: true}})
	mp.published = nil
	r.Register(&registryRuntimeStub{name: "test", caps: Capabilities{Containers: true}})
	for _, e := range mp.published {
		if e.Type == events.EventRuntimeCapabilityChanged {
			t.Error("Replacing with same capabilities should not publish capability changed event")
		}
	}
}

func TestRegistryGetPublishesUnavailableWhenNotFound(t *testing.T) {
	mp := &mockPublisher{}
	r := NewRegistry(mp)
	_, ok := r.Get("nonexistent")
	if ok {
		t.Fatal("Get should return false for nonexistent")
	}
	if len(mp.published) < 1 {
		t.Fatal("Get when runtime not found should publish an event")
	}
	if mp.published[0].Type != events.EventRuntimeUnavailable {
		t.Errorf("Event type = %v, want %v", mp.published[0].Type, events.EventRuntimeUnavailable)
	}
}

func TestRegistryRecordOperationIncrementsMetrics(t *testing.T) {
	r := NewRegistry()
	r.recordOperation(nil)
	r.recordOperation(nil)
	r.recordOperation(errSomeError)
	metrics := r.Metrics()
	if metrics.RuntimeOperationsTotal != 3 {
		t.Errorf("Operations total = %d, want 3", metrics.RuntimeOperationsTotal)
	}
	if metrics.RuntimeOperationFailuresTotal != 1 {
		t.Errorf("Operation failures total = %d, want 1", metrics.RuntimeOperationFailuresTotal)
	}
}

var errSomeError = context.Canceled
