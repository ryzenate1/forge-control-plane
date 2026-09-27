package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var pluginNamePattern = regexp.MustCompile(`\A[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}\z`)

// ErrPluginNotFound is returned when a plugin id does not resolve to a stored
// plugin. Callers match on it instead of guessing from a generic store error.
var ErrPluginNotFound = errors.New("plugin not found")

// pluginHookTimeout bounds a single hook invocation so one wedged plugin cannot
// hold up the whole hook chain.
const pluginHookTimeout = 10 * time.Second

// maxManifestBytes caps the manifest accepted from an install request.
const maxManifestBytes = 1024 * 1024

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

// mustGet resolves a plugin and fails loudly when it is absent. Repositories
// disagree on whether "not found" is an error or a nil result, so both shapes
// are normalised here rather than being allowed to read as a success.
func (s *Service) mustGet(ctx context.Context, id string) (*Plugin, error) {
	if strings.TrimSpace(id) == "" {
		return nil, ErrPluginNotFound
	}
	plugin, err := s.store.GetPlugin(ctx, id)
	if errors.Is(err, ErrPluginNotFound) {
		return nil, ErrPluginNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("look up plugin: %w", err)
	}
	if plugin == nil {
		return nil, ErrPluginNotFound
	}
	return plugin, nil
}

// findByName reports whether a name is already taken. A store failure is an
// error, never "free to use".
func (s *Service) findByName(ctx context.Context, name string) (*Plugin, error) {
	existing, err := s.store.FindPluginByName(ctx, name)
	if errors.Is(err, ErrPluginNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("check existing plugin %q: %w", name, err)
	}
	return existing, nil
}

func (s *Service) Install(ctx context.Context, name, source, manifestJSON string) (*Plugin, error) {
	if !pluginNamePattern.MatchString(name) {
		return nil, fmt.Errorf("plugin name may only contain letters, numbers, dot, underscore, and hyphen")
	}
	if len(manifestJSON) > maxManifestBytes {
		return nil, fmt.Errorf("plugin manifest exceeds 1 MiB")
	}
	var manifest PluginManifest
	if err := json.Unmarshal([]byte(manifestJSON), &manifest); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}
	if manifest.Name != "" && manifest.Name != name {
		return nil, fmt.Errorf("manifest name %q does not match plugin name %q", manifest.Name, name)
	}
	// A manifest that declares no version cannot participate in dependency or
	// min/max range resolution, so reject it at the boundary.
	if strings.TrimSpace(manifest.Version) == "" {
		return nil, fmt.Errorf("manifest must declare a version")
	}
	if manifest.Settings != nil && !json.Valid(manifest.Settings) {
		return nil, fmt.Errorf("manifest settings must be valid JSON")
	}
	if manifest.Entrypoint != "" {
		cleanEntrypoint := filepath.Clean(manifest.Entrypoint)
		if filepath.IsAbs(cleanEntrypoint) || cleanEntrypoint == ".." || strings.HasPrefix(cleanEntrypoint, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("plugin entrypoint must remain inside the plugin directory")
		}
	}
	// Hook targets are resolved against the plugin directory later; keep them
	// relative so a manifest cannot name an arbitrary host path.
	for hook, target := range manifest.Hooks {
		clean := filepath.Clean(target)
		if target == "" || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("hook %q must reference a path inside the plugin directory", hook)
		}
	}

	existing, err := s.findByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("plugin %q is already installed", name)
	}

	if s.pluginsDir != "" {
		pluginDir, err := safePluginPath(s.pluginsDir, name)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(pluginDir, 0755); err != nil {
			return nil, fmt.Errorf("create plugin directory: %w", err)
		}
		manifestData, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("encode manifest: %w", err)
		}
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
	plugin, err := s.mustGet(ctx, id)
	if err != nil {
		return err
	}

	// Remove the row first: if the directory removal fails the plugin is still
	// installed and visible, rather than a ghost record pointing at nothing.
	if err := s.store.DeletePlugin(ctx, id); err != nil {
		return fmt.Errorf("delete plugin record: %w", err)
	}

	if s.pluginsDir != "" {
		pluginDir, err := safePluginPath(s.pluginsDir, plugin.Name)
		if err != nil {
			return err
		}
		if err := os.RemoveAll(pluginDir); err != nil {
			return fmt.Errorf("remove plugin directory: %w", err)
		}
	}

	return nil
}

func (s *Service) Enable(ctx context.Context, id string) error {
	if _, err := s.mustGet(ctx, id); err != nil {
		return err
	}
	return s.store.UpdatePluginState(ctx, id, PluginStateEnabled, "")
}

func (s *Service) Disable(ctx context.Context, id string) error {
	if _, err := s.mustGet(ctx, id); err != nil {
		return err
	}
	return s.store.UpdatePluginState(ctx, id, PluginStateDisabled, "")
}

func (s *Service) UpdateSettings(ctx context.Context, id string, settings json.RawMessage) error {
	if _, err := s.mustGet(ctx, id); err != nil {
		return err
	}
	// Reject anything that is not valid JSON rather than coercing it into a
	// jsonb column and reporting success.
	if len(settings) == 0 || !json.Valid(settings) {
		return fmt.Errorf("plugin settings must be valid JSON")
	}
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

		hookCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		result, err := h.handler(hookCtx, plugin, args)
		cancel()
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
		if !entry.IsDir() || !pluginNamePattern.MatchString(entry.Name()) {
			continue
		}

		pluginDir, err := safePluginPath(s.pluginsDir, entry.Name())
		if err != nil {
			continue
		}
		manifestPath := filepath.Join(pluginDir, "manifest.json")
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

func safePluginPath(root, name string) (string, error) {
	if !pluginNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid plugin name")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve plugin root: %w", err)
	}
	candidate := filepath.Join(absoluteRoot, name)
	relative, err := filepath.Rel(absoluteRoot, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("plugin path escapes plugin root")
	}
	return candidate, nil
}
