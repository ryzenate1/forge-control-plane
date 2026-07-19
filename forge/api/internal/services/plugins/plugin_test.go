package plugins

import (
	"context"
	"encoding/json"
	"testing"
)

type mockPluginStore struct {
	plugins map[string]Plugin
}

func (m *mockPluginStore) ListPlugins(ctx context.Context) ([]Plugin, error) {
	var result []Plugin
	for _, p := range m.plugins {
		result = append(result, p)
	}
	return result, nil
}

func (m *mockPluginStore) GetPlugin(ctx context.Context, id string) (*Plugin, error) {
	p, ok := m.plugins[id]
	if !ok {
		return nil, nil
	}
	return &p, nil
}

func (m *mockPluginStore) CreatePlugin(ctx context.Context, plugin *Plugin) error {
	m.plugins[plugin.ID] = *plugin
	return nil
}

func (m *mockPluginStore) UpdatePluginState(ctx context.Context, id string, state PluginState, errorMsg string) error {
	p, ok := m.plugins[id]
	if !ok {
		return nil
	}
	p.State = state
	p.Error = errorMsg
	m.plugins[id] = p
	return nil
}

func (m *mockPluginStore) UpdatePluginSettings(ctx context.Context, id string, settings json.RawMessage) error {
	p, ok := m.plugins[id]
	if !ok {
		return nil
	}
	p.Settings = settings
	m.plugins[id] = p
	return nil
}

func (m *mockPluginStore) DeletePlugin(ctx context.Context, id string) error {
	delete(m.plugins, id)
	return nil
}

func (m *mockPluginStore) FindPluginByName(ctx context.Context, name string) (*Plugin, error) {
	for _, p := range m.plugins {
		if p.Name == name {
			return &p, nil
		}
	}
	return nil, nil
}

func TestPluginInstall(t *testing.T) {
	store := &mockPluginStore{plugins: make(map[string]Plugin)}
	svc := New(store, "")

	manifest := `{"name": "test-plugin", "description": "A test plugin", "version": "1.0.0", "author": "Test"}`
	plugin, err := svc.Install(context.Background(), "test-plugin", "local", manifest)
	if err != nil {
		t.Fatal(err)
	}

	if plugin.Name != "test-plugin" {
		t.Errorf("expected name 'test-plugin', got %q", plugin.Name)
	}

	if plugin.State != PluginStateInstalled {
		t.Errorf("expected state 'installed', got %q", plugin.State)
	}

	_, err = svc.Install(context.Background(), "test-plugin", "local", manifest)
	if err == nil {
		t.Error("expected error for duplicate install")
	}
}

func TestPluginLifecycle(t *testing.T) {
	store := &mockPluginStore{plugins: make(map[string]Plugin)}
	svc := New(store, "")

	plugin, _ := svc.Install(context.Background(), "lifecycle-test", "local", `{"name": "lifecycle-test", "version": "1.0.0"}`)

	if err := svc.Enable(context.Background(), plugin.ID); err != nil {
		t.Fatal(err)
	}
	updated, _ := svc.Get(context.Background(), plugin.ID)
	if updated.State != PluginStateEnabled {
		t.Errorf("expected state 'enabled', got %q", updated.State)
	}

	if err := svc.Disable(context.Background(), plugin.ID); err != nil {
		t.Fatal(err)
	}
	updated, _ = svc.Get(context.Background(), plugin.ID)
	if updated.State != PluginStateDisabled {
		t.Errorf("expected state 'disabled', got %q", updated.State)
	}

	if err := svc.Uninstall(context.Background(), plugin.ID); err != nil {
		t.Fatal(err)
	}
	p, _ := svc.Get(context.Background(), plugin.ID)
	if p != nil {
		t.Error("expected nil plugin after uninstall")
	}
}

func TestPluginHook(t *testing.T) {
	store := &mockPluginStore{plugins: make(map[string]Plugin)}
	svc := New(store, "")

	plugin, _ := svc.Install(context.Background(), "hook-test", "local", `{"name": "hook-test", "version": "1.0.0"}`)
	svc.Enable(context.Background(), plugin.ID)

	called := false
	svc.RegisterHook(plugin.ID, "server.created", func(ctx context.Context, p *Plugin, args map[string]any) (map[string]any, error) {
		called = true
		return map[string]any{"result": "handled"}, nil
	})

	results, err := svc.ExecuteHook(context.Background(), "server.created", map[string]any{"serverId": "srv-1"})
	if err != nil {
		t.Fatal(err)
	}

	if !called {
		t.Error("hook handler was not called")
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}
