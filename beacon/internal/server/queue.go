package server

import (
	"context"

	"gamepanel/beacon/internal/commands"
)

// Compatibility aliases keep Beacon's existing handler API stable while the
// durable command engine lives behind its own package boundary.
type OperationType = commands.Type
type OperationStatus = commands.Status
type Operation = commands.Operation
type OperationHandler = commands.Handler
type OperationQueue = commands.Queue

const (
	OpStart     = commands.Start
	OpStop      = commands.Stop
	OpRestart   = commands.Restart
	OpKill      = commands.Kill
	OpInstall   = commands.Install
	OpReinstall = commands.Reinstall

	StatusPending   = commands.StatusPending
	StatusRunning   = commands.StatusRunning
	StatusCompleted = commands.StatusCompleted
	StatusFailed    = commands.StatusFailed
	StatusCancelled = commands.StatusCancelled
)

var (
	ErrQueueFull         = commands.ErrQueueFull
	ErrQueueStopped      = commands.ErrQueueStopped
	ErrOperationNotFound = commands.ErrOperationNotFound
)

func NewOperationQueue(concurrency int, handler OperationHandler) *OperationQueue {
	return commands.NewQueue(concurrency, handler)
}

func NewPersistentOperationQueue(path string, concurrency int, handler OperationHandler) (*OperationQueue, error) {
	return commands.NewPersistentQueue(path, concurrency, handler)
}

type commandQueue interface {
	Start(context.Context)
	Enqueue(context.Context, string, OperationType) (*Operation, error)
	EnqueueCommand(context.Context, string, string, OperationType) (*Operation, error)
	GetStatus(string) (*Operation, error)
	ListByServer(string) []Operation
	Shutdown()
}

var _ commandQueue = (*OperationQueue)(nil)
