package plugins

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
)

type memStore struct {
	mu      sync.Mutex
	plugins map[string]Plugin
	byName  map[string]string
}

func (m *memStore) ListPlugins(ctx context.Context) ([]Plugin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []Plugin
	for _, p := range m.plugins {
		result = append(result, p)
	}
	return result, nil
}

func (m *memStore) GetPlugin(ctx context.Context, id string) (*Plugin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.plugins[id]
	if !ok {
		return nil, nil
	}
	return &p, nil
}

func (m *memStore) CreatePlugin(ctx context.Context, plugin *Plugin) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.plugins[plugin.ID] = *plugin
	m.byName[plugin.Name] = plugin.ID
	return nil
}

func (m *memStore) UpdatePluginState(ctx context.Context, id string, state PluginState, errorMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.plugins[id]
	if ok {
		p.State = state
		p.Error = errorMsg
		m.plugins[id] = p
	}
	return nil
}

func (m *memStore) UpdatePluginSettings(ctx context.Context, id string, settings json.RawMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.plugins[id]
	if ok {
		p.Settings = settings
		m.plugins[id] = p
	}
	return nil
}

func (m *memStore) DeletePlugin(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.plugins, id)
	return nil
}

func (m *memStore) FindPluginByName(ctx context.Context, name string) (*Plugin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.byName[name]
	if !ok {
		return nil, nil
	}
	p := m.plugins[id]
	return &p, nil
}

func TestPluginFullLifecycleIntegration(t *testing.T) {
	store := &memStore{
		plugins: make(map[string]Plugin),
		byName:  make(map[string]string),
	}
	svc := New(store, "")

	ctx := context.Background()

	p, err := svc.Install(ctx, "test-plugin", "github", `{"name":"test-plugin", "version":"1.0.0", "author":"test"}`)
	if err != nil {
		t.Fatal(err)
	}
	if p.State != PluginStateInstalled {
		t.Errorf("expected installed, got %q", p.State)
	}

	svc.Enable(ctx, p.ID)
	enabled, _ := svc.Get(ctx, p.ID)
	if enabled.State != PluginStateEnabled {
		t.Errorf("expected enabled, got %q", enabled.State)
	}

	plugins, err := svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 {
		t.Errorf("expected 1 plugin, got %d", len(plugins))
	}

	svc.Disable(ctx, p.ID)
	disabled, _ := svc.Get(ctx, p.ID)
	if disabled.State != PluginStateDisabled {
		t.Errorf("expected disabled, got %q", disabled.State)
	}

	svc.Uninstall(ctx, p.ID)
	got, err := svc.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil plugin after uninstall")
	}
}
