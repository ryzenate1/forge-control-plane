package modules

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"

	"gamepanel/forge/internal/platform/operations"
	"gamepanel/forge/internal/platform/workloads"
)

type Permission struct{ Key, Description string }
type Route struct{ Method, Path, Audience string }

type Module interface {
	Name() string
	WorkloadKinds() []workloads.Kind
	OperationDrivers() []operations.Driver
	Permissions() []Permission
	Routes() []Route
	Start(context.Context) error
}

type Registry struct {
	mu         sync.RWMutex
	modules    map[string]Module
	kindOwners map[workloads.Kind]string
}

func NewRegistry() *Registry {
	return &Registry{modules: map[string]Module{}, kindOwners: map[workloads.Kind]string{}}
}

func (r *Registry) Register(module Module) error {
	if module == nil {
		return errors.New("module is required")
	}
	name := strings.TrimSpace(module.Name())
	if name == "" {
		return errors.New("module name is required")
	}
	kinds, err := workloads.UniqueKinds(module.WorkloadKinds())
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.modules[name]; exists {
		return errors.New("module already registered: " + name)
	}
	for _, kind := range kinds {
		if owner, exists := r.kindOwners[kind]; exists {
			return errors.New("workload kind " + string(kind) + " already owned by " + owner)
		}
	}
	r.modules[name] = module
	for _, kind := range kinds {
		r.kindOwners[kind] = name
	}
	return nil
}

func (r *Registry) Modules() []Module {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Module, 0, len(r.modules))
	for _, m := range r.modules {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}
func (r *Registry) Module(name string) (Module, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.modules[name]
	return m, ok
}
func (r *Registry) Owner(kind workloads.Kind) (Module, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	name, ok := r.kindOwners[kind]
	if !ok {
		return nil, false
	}
	m, ok := r.modules[name]
	return m, ok
}
func (r *Registry) Start(ctx context.Context) error {
	for _, module := range r.Modules() {
		if err := module.Start(ctx); err != nil {
			return errors.New("start module " + module.Name() + ": " + err.Error())
		}
	}
	return nil
}
