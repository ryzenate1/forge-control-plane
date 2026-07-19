package server

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type OperationType string

const (
	OpStart    OperationType = "start"
	OpStop     OperationType = "stop"
	OpRestart  OperationType = "restart"
	OpInstall  OperationType = "install"
	OpReinstall OperationType = "reinstall"
)

type OperationStatus string

const (
	StatusPending   OperationStatus = "pending"
	StatusRunning   OperationStatus = "running"
	StatusCompleted OperationStatus = "completed"
	StatusFailed    OperationStatus = "failed"
	StatusCancelled OperationStatus = "cancelled"
)

type Operation struct {
	ID          string
	ServerID    string
	Type        OperationType
	Status      OperationStatus
	Error       string
	CreatedAt   time.Time
	StartedAt   time.Time
	CompletedAt time.Time
}

type OperationHandler func(ctx context.Context, op *Operation) error

var (
	ErrQueueFull     = errors.New("operation queue is full")
	ErrQueueStopped  = errors.New("operation queue is stopped")
	ErrOperationNotFound = errors.New("operation not found")
)

type OperationQueue struct {
	mu          sync.Mutex
	operations  map[string]*Operation
	serverOps   map[string][]string
	ch          chan *Operation
	concurrency int
	handler     OperationHandler
	cancel      context.CancelFunc
	done        chan struct{}
	nextID      int
}

func NewOperationQueue(concurrency int, handler OperationHandler) *OperationQueue {
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > 4 {
		concurrency = 4
	}
	return &OperationQueue{
		operations:  make(map[string]*Operation),
		serverOps:   make(map[string][]string),
		ch:          make(chan *Operation, 64),
		concurrency: concurrency,
		handler:     handler,
		done:        make(chan struct{}),
	}
}

func (q *OperationQueue) Start(ctx context.Context) {
	ctx, q.cancel = context.WithCancel(ctx)
	var wg sync.WaitGroup
	for i := 0; i < q.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.worker(ctx)
		}()
	}
	go func() {
		wg.Wait()
		close(q.done)
	}()
}

func (q *OperationQueue) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case op, ok := <-q.ch:
			if !ok {
				return
			}
			q.processOp(ctx, op)
		}
	}
}

func (q *OperationQueue) processOp(ctx context.Context, op *Operation) {
	q.mu.Lock()
	op.Status = StatusRunning
	op.StartedAt = time.Now()
	q.mu.Unlock()

	var opErr string
	if q.handler != nil {
		if err := q.handler(ctx, op); err != nil {
			opErr = err.Error()
		}
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	op.CompletedAt = time.Now()
	if opErr != "" {
		op.Status = StatusFailed
		op.Error = opErr
	} else {
		op.Status = StatusCompleted
	}
}

func (q *OperationQueue) Enqueue(ctx context.Context, serverID string, opType OperationType) (*Operation, error) {
	q.mu.Lock()
	q.nextID++
	op := &Operation{
		ID:        fmt.Sprintf("op-%d", q.nextID),
		ServerID:  serverID,
		Type:      opType,
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}
	q.operations[op.ID] = op
	q.serverOps[serverID] = append(q.serverOps[serverID], op.ID)
	q.mu.Unlock()

	select {
	case <-ctx.Done():
		q.mu.Lock()
		op.Status = StatusCancelled
		q.mu.Unlock()
		return op, ctx.Err()
	case q.ch <- op:
		return op, nil
	}
}

func (q *OperationQueue) GetStatus(opID string) (*Operation, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	op, ok := q.operations[opID]
	if !ok {
		return nil, ErrOperationNotFound
	}
	cp := *op
	return &cp, nil
}

func (q *OperationQueue) ListByServer(serverID string) []Operation {
	q.mu.Lock()
	defer q.mu.Unlock()
	ids := q.serverOps[serverID]
	result := make([]Operation, 0, len(ids))
	for _, id := range ids {
		if op, ok := q.operations[id]; ok {
			result = append(result, *op)
		}
	}
	return result
}

func (q *OperationQueue) Shutdown() {
	if q.cancel != nil {
		q.cancel()
	}
	close(q.ch)
	<-q.done

	q.mu.Lock()
	defer q.mu.Unlock()
	for _, op := range q.operations {
		if op.Status == StatusPending || op.Status == StatusRunning {
			op.Status = StatusCancelled
			op.CompletedAt = time.Now()
		}
	}
}
