package fencing

import (
	"context"
	"time"

	"gamepanel/forge/internal/events"
	"gamepanel/forge/internal/store"
)

type Service struct {
	store     *store.Store
	publisher events.Publisher
}

func New(store *store.Store, publisher events.Publisher) *Service {
	return &Service{store: store, publisher: publisher}
}

func (s *Service) Handle(ctx context.Context, envelope events.Envelope) error {
	switch envelope.Type {
	case events.EventNodeRecovered:
		return s.FenceNode(ctx, envelope.ResourceID)
	default:
		return nil
	}
}

func (s *Service) FenceNode(ctx context.Context, nodeID string) error {
	servers, err := s.store.ListServersForNode(ctx, nodeID)
	if err != nil {
		return err
	}
	leaseExpiry := time.Now().UTC().Add(24 * time.Hour)
	for _, server := range servers {
		server.Generation++
		server.WorkloadLeaseExpiry = &leaseExpiry
		if err := s.store.UpdateServerGeneration(ctx, server.ID, server.Generation, &leaseExpiry); err != nil {
			return err
		}
	}
	if s.publisher != nil {
		_ = s.publisher.Publish(ctx, events.NewEnvelope(events.EventNodeFenced,
			"fencing", "node", nodeID, map[string]any{
				"serverCount": len(servers),
			}))
	}
	return nil
}
