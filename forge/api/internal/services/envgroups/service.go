// Package envgroups implements named env-var groups for environments: JSON
// bundles of KEY=VALUE entries that can be declared on manifest services
// (ServiceDef.Groups) and applied to an environment at once. Storage lives in
// the env_var_groups table; the service mirrors the envvars package's shape.
package envgroups

import (
	"context"
	"errors"
	"strings"

	"gamepanel/forge/internal/store"
)

// GroupStore is the persistence surface used by the service.
type GroupStore interface {
	EnsureEnvVarEnv(ctx context.Context, envID string) error
	ListEnvVarGroups(ctx context.Context, envID string) ([]store.EnvVarGroup, error)
	GetEnvVarGroup(ctx context.Context, envID, name string) (store.EnvVarGroup, error)
	UpsertEnvVarGroup(ctx context.Context, envID, name, description string, variables map[string]string, actorID *string) (store.EnvVarGroup, error)
	DeleteEnvVarGroup(ctx context.Context, envID, name string, actorID *string) error
}

// Service exposes group CRUD over the store.
type Service struct {
	store GroupStore
}

// New builds the group service.
func New(st GroupStore) *Service {
	return &Service{store: st}
}

// List returns every group of an environment, ordered by name.
func (svc *Service) List(ctx context.Context, envID string) ([]store.EnvVarGroup, error) {
	return svc.store.ListEnvVarGroups(ctx, envID)
}

// Get returns one group.
func (svc *Service) Get(ctx context.Context, envID, name string) (store.EnvVarGroup, error) {
	return svc.store.GetEnvVarGroup(ctx, envID, name)
}

// Save upserts a full group bundle; variables replaces the previous content.
func (svc *Service) Save(ctx context.Context, envID, name, description string, variables map[string]string, actorID *string) (store.EnvVarGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return store.EnvVarGroup{}, errors.New("group name is required")
	}
	return svc.store.UpsertEnvVarGroup(ctx, envID, name, description, variables, actorID)
}

// Delete removes a group.
func (svc *Service) Delete(ctx context.Context, envID, name string, actorID *string) error {
	return svc.store.DeleteEnvVarGroup(ctx, envID, name, actorID)
}