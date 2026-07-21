package envvars

import (
	"context"
	"errors"

	"gamepanel/forge/internal/store"
)

type envVarStore interface {
	CreateEnvironmentVariable(ctx context.Context, req store.CreateEnvVarRequest, actorID *string) (store.EnvironmentVariable, error)
	GetEnvironmentVariable(ctx context.Context, id string) (store.EnvironmentVariable, error)
	ListEnvironmentVariables(ctx context.Context, scopeType, scopeID string) ([]store.EnvironmentVariable, error)
	UpdateEnvironmentVariable(ctx context.Context, id string, req store.UpdateEnvVarRequest, actorID *string) (store.EnvironmentVariable, error)
	DeleteEnvironmentVariable(ctx context.Context, id string, actorID *string) error
	ResolveEnvironmentVariables(ctx context.Context, orgID, projectID, envID, serviceID string) (map[string]string, error)
	GetEnvVarRevisions(ctx context.Context, variableID string) ([]store.EnvVarRevision, error)
}

type Service struct {
	store envVarStore
}

func New(st envVarStore) *Service {
	return &Service{store: st}
}

type CreateEnvVarInput struct {
	OrgID         *string
	ProjectID     *string
	EnvironmentID *string
	ServiceID     *string
	Scope         string
	Key           string
	Value         string
	IsSensitive   bool
	Actor         *string
}

type UpdateEnvVarInput struct {
	Value       string
	IsSensitive bool
	Actor       *string
}

func (svc *Service) Create(ctx context.Context, input CreateEnvVarInput) (store.EnvironmentVariable, error) {
	if input.Key == "" {
		return store.EnvironmentVariable{}, errors.New("key is required")
	}
	req := store.CreateEnvVarRequest{
		OrgID:         input.OrgID,
		ProjectID:     input.ProjectID,
		EnvironmentID: input.EnvironmentID,
		ServiceID:     input.ServiceID,
		Scope:         input.Scope,
		Key:           input.Key,
		Value:         input.Value,
		IsSensitive:   input.IsSensitive,
	}
	return svc.store.CreateEnvironmentVariable(ctx, req, input.Actor)
}

func (svc *Service) Get(ctx context.Context, id string) (store.EnvironmentVariable, error) {
	return svc.store.GetEnvironmentVariable(ctx, id)
}

func (svc *Service) List(ctx context.Context, scopeType, scopeID string) ([]store.EnvironmentVariable, error) {
	return svc.store.ListEnvironmentVariables(ctx, scopeType, scopeID)
}

func (svc *Service) Update(ctx context.Context, id string, input UpdateEnvVarInput) (store.EnvironmentVariable, error) {
	req := store.UpdateEnvVarRequest{
		Value:       input.Value,
		IsSensitive: input.IsSensitive,
	}
	return svc.store.UpdateEnvironmentVariable(ctx, id, req, input.Actor)
}

func (svc *Service) Delete(ctx context.Context, id string, actorID *string) error {
	return svc.store.DeleteEnvironmentVariable(ctx, id, actorID)
}

func (svc *Service) Resolve(ctx context.Context, orgID, projectID, envID, serviceID string) (map[string]string, error) {
	return svc.store.ResolveEnvironmentVariables(ctx, orgID, projectID, envID, serviceID)
}

func (svc *Service) Revisions(ctx context.Context, variableID string) ([]store.EnvVarRevision, error) {
	return svc.store.GetEnvVarRevisions(ctx, variableID)
}
