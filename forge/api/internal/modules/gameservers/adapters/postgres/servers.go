package postgres

import (
	"context"

	"gamepanel/forge/internal/modules/gameservers/domain"
	"gamepanel/forge/internal/modules/gameservers/ports"
	"gamepanel/forge/internal/store"
)

// ServerStore is the narrow database surface required by this adapter. It
// keeps the game-server application layer independent of the legacy Store.
type ServerStore interface {
	GetServer(context.Context, string) (store.Server, error)
}

type ServerRepository struct{ store ServerStore }

func NewServerRepository(value ServerStore) *ServerRepository {
	return &ServerRepository{store: value}
}

func (repository *ServerRepository) Get(ctx context.Context, id string) (domain.Server, error) {
	server, err := repository.store.GetServer(ctx, id)
	if err != nil {
		return domain.Server{}, err
	}
	return domain.Server{
		ID: id, WorkloadID: id, Name: server.Name, NodeID: server.NodeID,
		TemplateID: server.Template, OwnerID: server.OwnerID, Suspended: server.Suspended,
		DesiredState: string(server.DesiredState), ObservedState: string(server.ActualState),
		DesiredGeneration: 1,
	}, nil
}

var _ ports.ServerRepository = (*ServerRepository)(nil)
