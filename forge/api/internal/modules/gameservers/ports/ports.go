package ports

import (
	"context"
	"gamepanel/forge/internal/modules/gameservers/domain"
)

type ServerRepository interface {
	Get(context.Context, string) (domain.Server, error)
}

type ServerWriter interface {
	Save(context.Context, domain.Server) error
}
type AllocationRepository interface {
	ListForServer(context.Context, string) ([]domain.Allocation, error)
}
type Runtime interface {
	Power(context.Context, string, domain.PowerSignal, string) error
}
type Scheduler interface {
	Place(context.Context, domain.Server) (string, error)
}
type BackupService interface {
	Create(context.Context, string) (string, error)
	Restore(context.Context, string, string) error
}
