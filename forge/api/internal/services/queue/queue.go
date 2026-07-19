package queue

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
)

type JobType string

const (
	JobServerStart     JobType = "server.start"
	JobServerStop      JobType = "server.stop"
	JobServerRestart   JobType = "server.restart"
	JobServerInstall   JobType = "server.install"
	JobServerUninstall JobType = "server.uninstall"
	JobBackupCreate    JobType = "backup.create"
	JobBackupRestore   JobType = "backup.restore"
	JobServerTransfer  JobType = "server.transfer"
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
	ID          string          `json:"id"`
	Type        JobType         `json:"type"`
	Status      JobStatus       `json:"status"`
	ServerID    string          `json:"serverId"`
	NodeID      string          `json:"nodeId"`
	Payload     json.RawMessage `json:"payload"`
	Result      json.RawMessage `json:"result,omitempty"`
	Error       string          `json:"error,omitempty"`
	Priority    int             `json:"priority"`
	MaxRetries  int             `json:"maxRetries"`
	RetryCount  int             `json:"retryCount"`
	CreatedAt   time.Time       `json:"createdAt"`
	StartedAt   *time.Time      `json:"startedAt,omitempty"`
	CompletedAt *time.Time      `json:"completedAt,omitempty"`
}

type QueueStore interface {
	Enqueue(ctx context.Context, job *Job) error
	Dequeue(ctx context.Context, nodeID string) (*Job, error)
	Acknowledge(ctx context.Context, jobID string) error
	Fail(ctx context.Context, jobID string, err error) error
	ListPending(ctx context.Context, nodeID string) ([]Job, error)
	GetJob(ctx context.Context, jobID string) (*Job, error)
}

type HandlerFunc func(ctx context.Context, job *Job) error

type Service struct {
	store    QueueStore
	handlers map[JobType]HandlerFunc
	workers  int
	mu       sync.RWMutex
	wg       sync.WaitGroup
	cancel   context.CancelFunc
}

func New(store QueueStore, workers int) *Service {
	if workers <= 0 {
		workers = 5
	}
	return &Service{
		store:    store,
		handlers: make(map[JobType]HandlerFunc),
		workers:  workers,
	}
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
		go s.worker(ctx, i)
	}
}

func (s *Service) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

func (s *Service) worker(ctx context.Context, id int) {
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			job, err := s.store.Dequeue(ctx, "")
			if err != nil || job == nil {
				time.Sleep(time.Second)
				continue
			}
			s.process(ctx, job)
		}
	}
}

func (s *Service) process(ctx context.Context, job *Job) {
	s.mu.RLock()
	handler, ok := s.handlers[job.Type]
	s.mu.RUnlock()

	if !ok {
		s.store.Fail(ctx, job.ID, nil)
		return
	}

	now := time.Now().UTC()
	job.StartedAt = &now
	job.Status = JobStatusRunning

	if err := handler(ctx, job); err != nil {
		job.RetryCount++
		if job.RetryCount >= job.MaxRetries {
			s.store.Fail(ctx, job.ID, err)
		} else {
			s.store.Enqueue(ctx, job)
		}
		return
	}

	s.store.Acknowledge(ctx, job.ID)
}

func (s *Service) Dispatch(ctx context.Context, jobType JobType, serverID, nodeID string, payload any, priority int) (*Job, error) {
	data, _ := json.Marshal(payload)
	job := &Job{
		ID:         uuid.NewString(),
		Type:       jobType,
		Status:     JobStatusPending,
		ServerID:   serverID,
		NodeID:     nodeID,
		Payload:    data,
		Priority:   priority,
		MaxRetries: 3,
		CreatedAt:  time.Now().UTC(),
	}
	return job, s.store.Enqueue(ctx, job)
}
