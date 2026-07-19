package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

type PluginState string

const (
	PluginStateInstalled PluginState = "installed"
	PluginStateEnabled   PluginState = "enabled"
	PluginStateDisabled  PluginState = "disabled"
	PluginStateError     PluginState = "error"
	PluginStateUpdating  PluginState = "updating"
)

type PluginManifest struct {
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Version      string            `json:"version"`
	Author       string            `json:"author"`
	License      string            `json:"license,omitempty"`
	Homepage     string            `json:"homepage,omitempty"`
	Entrypoint   string            `json:"entrypoint,omitempty"`
	Permissions  []string          `json:"permissions,omitempty"`
	Hooks        map[string]string `json:"hooks,omitempty"`
	Settings     json.RawMessage   `json:"settings,omitempty"`
	Dependencies map[string]string `json:"dependencies,omitempty"`
	MinVersion   string            `json:"minVersion,omitempty"`
	MaxVersion   string            `json:"maxVersion,omitempty"`
}

type Plugin struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Manifest    PluginManifest  `json:"manifest"`
	Source      string          `json:"source"`
	State       PluginState     `json:"state"`
	InstalledAt time.Time       `json:"installedAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
	Settings    json.RawMessage `json:"settings,omitempty"`
	Error       string          `json:"error,omitempty"`
}

type PluginStore interface {
	ListPlugins(ctx context.Context) ([]Plugin, error)
	GetPlugin(ctx context.Context, id string) (*Plugin, error)
	CreatePlugin(ctx context.Context, plugin *Plugin) error
	UpdatePluginState(ctx context.Context, id string, state PluginState, errorMsg string) error
	UpdatePluginSettings(ctx context.Context, id string, settings json.RawMessage) error
	DeletePlugin(ctx context.Context, id string) error
	FindPluginByName(ctx context.Context, name string) (*Plugin, error)
}

type HookHandler func(ctx context.Context, plugin *Plugin, args map[string]any) (map[string]any, error)

type Service struct {
	store PluginStore
	hooks map[string][]struct {
		pluginID string
		handler  HookHandler
	}
	mu         sync.RWMutex
	pluginsDir string
}

func New(store PluginStore, pluginsDir string) *Service {
	return &Service{
		store: store,
		hooks: make(map[string][]struct {
			pluginID string
			handler  HookHandler
		}),
		pluginsDir: pluginsDir,
	}
}

func (s *Service) List(ctx context.Context) ([]Plugin, error) {
	return s.store.ListPlugins(ctx)
}

func (s *Service) Get(ctx context.Context, id string) (*Plugin, error) {
	return s.store.GetPlugin(ctx, id)
}

func (s *Service) Install(ctx context.Context, name, source, manifestJSON string) (*Plugin, error) {
	var manifest PluginManifest
	if err := json.Unmarshal([]byte(manifestJSON), &manifest); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	existing, err := s.store.FindPluginByName(ctx, name)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("plugin %q is already installed", name)
	}

	if s.pluginsDir != "" {
		pluginDir := filepath.Join(s.pluginsDir, name)
		if err := os.MkdirAll(pluginDir, 0755); err != nil {
			return nil, fmt.Errorf("create plugin directory: %w", err)
		}
		manifestData, _ := json.MarshalIndent(manifest, "", "  ")
		if err := os.WriteFile(filepath.Join(pluginDir, "manifest.json"), manifestData, 0644); err != nil {
			return nil, fmt.Errorf("write manifest: %w", err)
		}
	}

	plugin := &Plugin{
		ID:          uuid.NewString(),
		Name:        name,
		Manifest:    manifest,
		Source:      source,
		State:       PluginStateInstalled,
		InstalledAt: time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
		Settings:    manifest.Settings,
	}

	if err := s.store.CreatePlugin(ctx, plugin); err != nil {
		return nil, fmt.Errorf("store plugin: %w", err)
	}

	return plugin, nil
}

func (s *Service) Uninstall(ctx context.Context, id string) error {
	plugin, err := s.store.GetPlugin(ctx, id)
	if err != nil {
		return err
	}

	if s.pluginsDir != "" {
		pluginDir := filepath.Join(s.pluginsDir, plugin.Name)
		os.RemoveAll(pluginDir)
	}

	return s.store.DeletePlugin(ctx, id)
}

func (s *Service) Enable(ctx context.Context, id string) error {
	return s.store.UpdatePluginState(ctx, id, PluginStateEnabled, "")
}

func (s *Service) Disable(ctx context.Context, id string) error {
	return s.store.UpdatePluginState(ctx, id, PluginStateDisabled, "")
}

func (s *Service) UpdateSettings(ctx context.Context, id string, settings json.RawMessage) error {
	return s.store.UpdatePluginSettings(ctx, id, settings)
}

func (s *Service) RegisterHook(pluginID, hook string, handler HookHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hooks[hook] = append(s.hooks[hook], struct {
		pluginID string
		handler  HookHandler
	}{
		pluginID: pluginID,
		handler:  handler,
	})
}

func (s *Service) ExecuteHook(ctx context.Context, hook string, args map[string]any) ([]map[string]any, error) {
	s.mu.RLock()
	handlers := s.hooks[hook]
	s.mu.RUnlock()

	results := make([]map[string]any, 0, len(handlers))
	for _, h := range handlers {
		plugin, err := s.store.GetPlugin(ctx, h.pluginID)
		if err != nil || plugin.State != PluginStateEnabled {
			continue
		}

		result, err := h.handler(ctx, plugin, args)
		if err != nil {
			continue
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *Service) Discover(ctx context.Context) ([]Plugin, error) {
	if s.pluginsDir == "" {
		return nil, nil
	}

	entries, err := os.ReadDir(s.pluginsDir)
	if err != nil {
		return nil, err
	}

	var discovered []Plugin
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		manifestPath := filepath.Join(s.pluginsDir, entry.Name(), "manifest.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}

		var manifest PluginManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			continue
		}

		existing, err := s.store.FindPluginByName(ctx, entry.Name())
		if err == nil && existing != nil {
			discovered = append(discovered, *existing)
			continue
		}

		plugin := Plugin{
			ID:          uuid.NewString(),
			Name:        entry.Name(),
			Manifest:    manifest,
			Source:      "local",
			State:       PluginStateInstalled,
			InstalledAt: time.Now().UTC(),
			UpdatedAt:   time.Now().UTC(),
		}

		if err := s.store.CreatePlugin(ctx, &plugin); err != nil {
			continue
		}
		discovered = append(discovered, plugin)
	}

	return discovered, nil
}
