package application

import (
	"context"
	"encoding/json"
	"errors"
	"gamepanel/forge/internal/modules/gameservers/domain"
	"gamepanel/forge/internal/modules/gameservers/ports"
	"gamepanel/forge/internal/platform/operations"
)

type PowerRequest struct {
	ServerID       string
	Signal         domain.PowerSignal
	IdempotencyKey string
}
type PowerService struct {
	servers    ports.ServerRepository
	operations operations.Dispatcher
}

func NewPowerService(servers ports.ServerRepository, dispatcher operations.Dispatcher) *PowerService {
	return &PowerService{servers: servers, operations: dispatcher}
}
func (s *PowerService) Request(ctx context.Context, request PowerRequest) (operations.Operation, error) {
	if request.ServerID == "" || !request.Signal.Valid() {
		return operations.Operation{}, errors.New("valid server id and power signal are required")
	}
	server, err := s.servers.Get(ctx, request.ServerID)
	if err != nil {
		return operations.Operation{}, err
	}
	if server.Suspended && (request.Signal == domain.PowerStart || request.Signal == domain.PowerRestart) {
		return operations.Operation{}, errors.New("cannot start a suspended server")
	}
	payload, _ := json.Marshal(map[string]string{"signal": string(request.Signal)})
	return s.operations.Dispatch(ctx, operations.Request{Kind: "server." + string(request.Signal), ResourceType: "server", ResourceID: request.ServerID, IdempotencyKey: request.IdempotencyKey, DesiredGeneration: server.DesiredGeneration + 1, Input: payload})
}
