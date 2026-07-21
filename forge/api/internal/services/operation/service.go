package operation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusQueued     Status = "queued"
	StatusRunning    Status = "running"
	StatusSucceeded  Status = "succeeded"
	StatusFailed     Status = "failed"
	StatusRetrying   Status = "retrying"
	StatusCancelled  Status = "cancelled"
)

type OperationType string

const (
	OpServerStart     OperationType = "server.start"
	OpServerStop      OperationType = "server.stop"
	OpServerRestart   OperationType = "server.restart"
	OpServerKill      OperationType = "server.kill"
	OpServerInstall   OperationType = "server.install"
	OpServerUninstall OperationType = "server.uninstall"
	OpServerTransfer  OperationType = "server.transfer"
	OpBackupCreate    OperationType = "backup.create"
	OpBackupRestore   OperationType = "backup.restore"
	OpDeployPromote   OperationType = "deployment.promote"
	OpComposeDeploy   OperationType = "compose.deploy"
	OpComposeUpdate   OperationType = "compose.update"
	OpComposeDelete   OperationType = "compose.delete"
	OpComposeStart    OperationType = "compose.start"
	OpComposeStop     OperationType = "compose.stop"
	OpComposeRestart  OperationType = "compose.restart"
)

type Operation struct {
	ID              string          `json:"id"`
	Kind            string          `json:"kind"`
	ResourceType    string          `json:"resourceType"`
	ResourceID      string          `json:"resourceId"`
	Status          Status          `json:"status"`
	Error           string          `json:"error,omitempty"`
	Input           json.RawMessage `json:"input,omitempty"`
	IdempotencyKey  string          `json:"idempotencyKey,omitempty"`
	DesiredGen      int             `json:"desired_generation"`
	ObservedGen     int             `json:"observed_generation"`
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
	StartedAt       *time.Time      `json:"startedAt,omitempty"`
	CompletedAt     *time.Time      `json:"completedAt,omitempty"`
}

type Store interface {
	Create(ctx context.Context, op *Operation) error
	Dequeue(ctx context.Context) (*Operation, error)
	Get(ctx context.Context, id string) (*Operation, error)
	ListByResource(ctx context.Context, resourceType, resourceID string) ([]Operation, error)
	ListPending(ctx context.Context, limit int) ([]Operation, error)
	UpdateStatus(ctx context.Context, id string, status Status, errMsg string) error
	Cancel(ctx context.Context, id string) error
}

type HandlerFunc func(ctx context.Context, op *Operation) error

type Config struct {
	MaxWorkers    int
	PollInterval  time.Duration
	MaxRetries    int
	BaseBackoff   time.Duration
	MaxBackoff    time.Duration
}

func DefaultConfig() Config {
	return Config{
		MaxWorkers:   5,
		PollInterval: time.Second,
		MaxRetries:   3,
		BaseBackoff:  time.Second,
		MaxBackoff:   30 * time.Second,
	}
}

type Service struct {
	store    Store
	handlers map[OperationType]HandlerFunc
	config   Config
	workerID string
	mu       sync.RWMutex
	wg       sync.WaitGroup
	cancel   context.CancelFunc
	active   map[string]context.CancelFunc
	activeMu sync.Mutex
}

func New(store Store) *Service {
	return NewWithConfig(store, DefaultConfig())
}

func NewWithConfig(store Store, config Config) *Service {
	if config.MaxWorkers <= 0 {
		config.MaxWorkers = 5
	}
	if config.PollInterval <= 0 {
		config.PollInterval = time.Second
	}
	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}
	if config.BaseBackoff <= 0 {
		config.BaseBackoff = time.Second
	}
	if config.MaxBackoff <= 0 {
		config.MaxBackoff = 30 * time.Second
	}
	return &Service{
		store:    store,
		handlers: make(map[OperationType]HandlerFunc),
		config:   config,
		workerID: uuid.NewString(),
		active:   make(map[string]context.CancelFunc),
	}
}

func (s *Service) RegisterHandler(opType OperationType, handler HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[opType] = handler
}

func (s *Service) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)
	for i := 0; i < s.config.MaxWorkers; i++ {
		s.wg.Add(1)
		go s.worker(ctx)
	}
}

func (s *Service) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.activeMu.Lock()
	for _, cancel := range s.active {
		cancel()
	}
	s.activeMu.Unlock()
	s.wg.Wait()
}

func (s *Service) worker(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(s.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			op, err := s.store.Dequeue(ctx)
			if err != nil {
				slog.Error("operation worker: dequeue", "error", err)
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
				continue
			}
			if op == nil {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
				continue
			}
			s.process(ctx, op)
		}
	}
}

func (s *Service) process(ctx context.Context, op *Operation) {
	opCtx, opCancel := context.WithCancel(ctx)
	s.activeMu.Lock()
	s.active[op.ID] = opCancel
	s.activeMu.Unlock()

	defer func() {
		s.activeMu.Lock()
		delete(s.active, op.ID)
		s.activeMu.Unlock()
		opCancel()

		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			err := fmt.Errorf("panic in operation %s: %v\nstack: %s", op.ID, r, buf[:n])
			slog.Error("operation handler panic", "id", op.ID, "error", err)
			_ = s.store.UpdateStatus(ctx, op.ID, StatusFailed, err.Error())
		}
	}()

	if err := s.store.UpdateStatus(ctx, op.ID, StatusRunning, ""); err != nil {
		slog.Error("operation: failed to mark running", "id", op.ID, "error", err)
		return
	}

	s.mu.RLock()
	handler, ok := s.handlers[OperationType(op.Kind)]
	s.mu.RUnlock()

	if !ok {
		_ = s.store.UpdateStatus(ctx, op.ID, StatusFailed, "no handler registered for operation type "+op.Kind)
		return
	}

	err := handler(opCtx, op)
	if err != nil {
		opRetry, retryErr := s.store.Get(ctx, op.ID)
		if retryErr == nil {
			retryCount := 0
			if opRetry.Status == StatusRetrying {
				retryCount = 1
			}
			retryCount++
			if retryCount > s.config.MaxRetries {
				_ = s.store.UpdateStatus(ctx, op.ID, StatusFailed, err.Error())
				return
			}
			backoff := s.config.BaseBackoff
			for i := 1; i < retryCount; i++ {
				backoff *= 2
				if backoff > s.config.MaxBackoff {
					backoff = s.config.MaxBackoff
					break
				}
			}
			time.Sleep(backoff)
			_ = s.store.UpdateStatus(ctx, op.ID, StatusRetrying, err.Error())
		}
		return
	}
	_ = s.store.UpdateStatus(ctx, op.ID, StatusSucceeded, "")
}

func (s *Service) Cancel(ctx context.Context, id string) error {
	s.activeMu.Lock()
	if cancel, ok := s.active[id]; ok {
		cancel()
	}
	s.activeMu.Unlock()
	return s.store.Cancel(ctx, id)
}

func (s *Service) Get(ctx context.Context, id string) (*Operation, error) {
	return s.store.Get(ctx, id)
}

func (s *Service) ListByResource(ctx context.Context, resourceType, resourceID string) ([]Operation, error) {
	return s.store.ListByResource(ctx, resourceType, resourceID)
}

type powerPayload struct {
	Signal string `json:"signal"`
}

func (s *Service) DispatchPower(ctx context.Context, serverID, signal string, idempotencyKey string) (*Operation, error) {
	payload, _ := json.Marshal(powerPayload{Signal: signal})
	id := uuid.NewString()
	if idempotencyKey != "" {
		id = uuid.NewSHA1(uuid.NameSpaceURL, []byte("forge-op:"+idempotencyKey)).String()
	}
	op := &Operation{
		ID:           id,
		Kind:         string(OpServerStart),
		ResourceType: "server",
		ResourceID:   serverID,
		Status:       StatusQueued,
		Input:        payload,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	switch signal {
	case "stop":
		op.Kind = string(OpServerStop)
	case "restart":
		op.Kind = string(OpServerRestart)
	case "kill":
		op.Kind = string(OpServerKill)
	}
	if err := s.store.Create(ctx, op); err != nil {
		return nil, err
	}
	return op, nil
}

type composePayload struct {
	StackID string `json:"stackId"`
}

func (s *Service) DispatchCompose(ctx context.Context, stackID string, action string) (*Operation, error) {
	payload, _ := json.Marshal(composePayload{StackID: stackID})
	opType := OpComposeStart
	switch action {
	case "deploy":
		opType = OpComposeDeploy
	case "update":
		opType = OpComposeUpdate
	case "delete":
		opType = OpComposeDelete
	case "stop":
		opType = OpComposeStop
	case "restart":
		opType = OpComposeRestart
	}
	op := &Operation{
		ID:           uuid.NewString(),
		Kind:         string(opType),
		ResourceType: "compose_stack",
		ResourceID:   stackID,
		Status:       StatusQueued,
		Input:        payload,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := s.store.Create(ctx, op); err != nil {
		return nil, err
	}
	return op, nil
}
