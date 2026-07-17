package config

import "gamepanel/forge/internal/config"

func Load(paths ...string) (*config.Config, error) {
	m, err := config.NewManager(paths...)
	if err != nil {
		return nil, err
	}
	cfg := m.All()
	return &cfg, nil
}
