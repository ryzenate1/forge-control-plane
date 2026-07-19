package observers

import (
	"context"

	"gamepanel/forge/internal/events"
	mailservice "gamepanel/forge/internal/services/mail"
)

type ServerObserver struct {
	mailTrigger *mailservice.TriggerService
}

func NewServerObserver(mailTrigger *mailservice.TriggerService) *ServerObserver {
	return &ServerObserver{
		mailTrigger: mailTrigger,
	}
}

func (o *ServerObserver) Name() string { return "server_observer" }

func (o *ServerObserver) Handle(ctx context.Context, event events.Envelope) error {
	_ = event
	return nil
}
