package bootstrap

import (
	"gamepanel/forge/internal/modules/apphosting"
	"gamepanel/forge/internal/modules/backups"
	"gamepanel/forge/internal/modules/containers"
	"gamepanel/forge/internal/modules/databases"
	"gamepanel/forge/internal/modules/gameservers"
	"gamepanel/forge/internal/modules/networking"
	"gamepanel/forge/internal/modules/storage"
	"gamepanel/forge/internal/platform/modules"
)

func ModuleRegistry() (*modules.Registry, error) {
	registry := modules.NewRegistry()
	if err := registry.Register(apphosting.New()); err != nil {
		return nil, err
	}
	if err := registry.Register(gameservers.New()); err != nil {
		return nil, err
	}
	if err := registry.Register(databases.New()); err != nil {
		return nil, err
	}
	if err := registry.Register(containers.New()); err != nil {
		return nil, err
	}
	if err := registry.Register(networking.New()); err != nil {
		return nil, err
	}
	if err := registry.Register(backups.New()); err != nil {
		return nil, err
	}
	if err := registry.Register(storage.New()); err != nil {
		return nil, err
	}
	return registry, nil
}
