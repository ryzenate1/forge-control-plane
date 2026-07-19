package observers

import (
	"context"
	"gamepanel/forge/internal/events"
)

type UserObserver struct{}

func NewUserObserver() *UserObserver {
	return &UserObserver{}
}

func (o *UserObserver) Name() string { return "user_observer" }

func (o *UserObserver) Handle(ctx context.Context, event events.Envelope) error {
	return nil
}
