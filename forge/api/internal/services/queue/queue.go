package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"
)

type JobType string

const (
	JobServerStart        JobType = "server.start"
	JobServerStop         JobType = "server.stop"
	JobServerRestart      JobType = "server.restart"
	JobServerKill         JobType = "server.kill"
	JobServerInstall      JobType = "server.install"
	JobServerUninstall    JobType = "server.uninstall"
	JobBackupCreate       JobType = "backup.create"
	JobBackupRestore      JobType = "backup.restore"
	JobServerTransfer     JobType = "server.transfer"
	JobComposeDeploy      JobType = "compose.deploy"
	JobComposeUpdate      JobType = "compose.update"
	JobComposeDelete      JobType = "compose.delete"
	JobComposeStart       JobType = "compose.start"
	JobComposeStop        JobType = "compose.stop"
	JobComposeRestart     JobType = "compose.restart"
)

type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
	JobStatusCancelled JobStatus = "cancelled"
)

type Job struct {
	ID             string          `json:"id"`
	Type           JobType         `json:"type"`
	Status         JobStatus       `json:"status"`
	ServerID       string          `json:"serverId"`
	NodeID         string          `json:"nodeId"`
	Payload        json.RawMessage `json:"payload"`
	Result         json.RawMessage `json:"result,omitempty"`
	Error          string          `json:"error,omitempty"`
	Priority       int             `json:"priority"`
	MaxRetries     int             `json:"maxRetries"`
	RetryCount     int             `json:"retryCount"`
	IdempotencyKey string          `json:"idempotencyKey,omitempty"`
	AvailableAt    time.Time       `json:"availableAt"`
	LockedBy       string          `json:"lockedBy,omitempty"`
	LockedUntil    *time.Time      `json:"lockedUntil,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
	StartedAt      *time.Time      `json:"startedAt,omitempty"`
	CompletedAt    *time.Time      `json:"completedAt,omitempty"`
}

type QueueStore interface {
	Enqueue(context.Context, *Job) error
	Dequeue(context.Context, string, string, time.Duration) (*Job, error)
	Acknowledge(context.Context, string) error
	Fail(context.Context, string, error) error
	Retry(context.Context, string, error, time.Time) error
	Heartbeat(context.Context, string, string, time.Duration) error
	ListPending(context.Context, string) ([]Job, error)
	GetJob(context.Context, string) (*Job, error)
}

type HandlerFunc func(context.Context, *Job) error

type Service struct {
	store    QueueStore
	handlers map[JobType]HandlerFunc
	workers  int
	workerID string
	lease    time.Duration
	mu       sync.RWMutex
	wg       sync.WaitGroup
	cancel   context.CancelFunc
	active   map[string]context.CancelFunc
	activeMu sync.Mutex
}

func New(store QueueStore, workers int) *Service {
	if workers <= 0 {
		workers = 5
	}
	return &Service{store: store, handlers: make(map[JobType]HandlerFunc), workers: workers, workerID: uuid.NewString(), lease: 30 * time.Second, active: make(map[string]context.CancelFunc)}
}

func (s *Service) RegisterHandler(jobType JobType, handler HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[jobType] = handler
}

func (s *Service) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)
	for i := 0; i < s.workers; i++ {
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
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		job, err := s.store.Dequeue(ctx, "", s.workerID, s.lease)
		if err != nil || job == nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		s.process(ctx, job)
	}
}

func (s *Service) process(ctx context.Context, job *Job) {
	jobCtx, jobCancel := context.WithCancel(ctx)
	s.activeMu.Lock()
	s.active[job.ID] = jobCancel
	s.activeMu.Unlock()

	defer func() {
		s.activeMu.Lock()
		delete(s.active, job.ID)
		s.activeMu.Unlock()
		jobCancel()

		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			err := fmt.Errorf("panic in job %s: %v\nstack: %s", job.ID, r, buf[:n])
			if job.RetryCount+1 >= job.MaxRetries {
				_ = s.store.Fail(ctx, job.ID, err)
			} else {
				backoff := time.Duration(1<<min(job.RetryCount, 6)) * time.Second
				_ = s.store.Retry(ctx, job.ID, err, time.Now().UTC().Add(backoff))
			}
		}
	}()

	s.mu.RLock()
	handler, ok := s.handlers[job.Type]
	s.mu.RUnlock()
	if !ok {
		_ = s.store.Fail(ctx, job.ID, errors.New("no handler registered for job type "+string(job.Type)))
		return
	}
	done := make(chan struct{})
	go s.keepLease(jobCtx, job.ID, done)
	err := handler(jobCtx, job)
	close(done)
	if err != nil {
		if job.RetryCount+1 >= job.MaxRetries {
			_ = s.store.Fail(ctx, job.ID, err)
		} else {
			backoff := time.Duration(1<<min(job.RetryCount, 6)) * time.Second
			_ = s.store.Retry(ctx, job.ID, err, time.Now().UTC().Add(backoff))
		}
		return
	}
	_ = s.store.Acknowledge(ctx, job.ID)
}

func (s *Service) keepLease(ctx context.Context, jobID string, done <-chan struct{}) {
	ticker := time.NewTicker(s.lease / 3)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			_ = s.store.Heartbeat(ctx, jobID, s.workerID, s.lease)
		}
	}
}

func (s *Service) Dispatch(ctx context.Context, jobType JobType, serverID, nodeID string, payload any, priority int) (*Job, error) {
	return s.DispatchIdempotent(ctx, "", jobType, serverID, nodeID, payload, priority)
}

func (s *Service) DispatchIdempotent(ctx context.Context, idempotencyKey string, jobType JobType, serverID, nodeID string, payload any, priority int) (*Job, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	if idempotencyKey != "" {
		id = uuid.NewSHA1(uuid.NameSpaceURL, []byte("forge-job:"+idempotencyKey)).String()
	}
	now := time.Now().UTC()
	job := &Job{ID: id, Type: jobType, Status: JobStatusPending, ServerID: serverID, NodeID: nodeID,
		Payload: data, Priority: priority, MaxRetries: 3, IdempotencyKey: idempotencyKey, AvailableAt: now, CreatedAt: now}
	return job, s.store.Enqueue(ctx, job)
}

func (s *Service) Cancel(ctx context.Context, id string) error {
	s.activeMu.Lock()
	if cancel, ok := s.active[id]; ok {
		cancel()
	}
	s.activeMu.Unlock()
	return s.store.Fail(ctx, id, errors.New("job cancelled"))
}

func (s *Service) Get(ctx context.Context, id string) (*Job, error) { return s.store.GetJob(ctx, id) }
