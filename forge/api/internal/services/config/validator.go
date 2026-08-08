package config

import (
	"context"
	"fmt"
	"sync"

	"gamepanel/forge/internal/config"
)

type Validator struct {
	mu       sync.RWMutex
	lastCfg  *config.Config
	lastErrs []config.ValidationError
}

func NewValidator() *Validator {
	return &Validator{}
}

func (v *Validator) Validate(ctx context.Context, cfg *config.Config) []config.ValidationError {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.lastCfg = cfg
	v.lastErrs = config.Validate(cfg)
	return v.lastErrs
}

func (v *Validator) IsHealthy() (bool, string) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if len(v.lastErrs) == 0 {
		return true, "configuration is valid"
	}
	return false, fmt.Sprintf("configuration has %d error(s)", len(v.lastErrs))
}
