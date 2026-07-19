package bootstrap

import (
	"gamepanel/forge/internal/modules/gameservers"
	"gamepanel/forge/internal/platform/modules"
)

func ModuleRegistry() (*modules.Registry, error) {
	registry := modules.NewRegistry()
	if err := registry.Register(gameservers.New()); err != nil {
		return nil, err
	}
	return registry, nil
}
