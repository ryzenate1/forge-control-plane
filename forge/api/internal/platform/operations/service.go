package operations

import (
	"context"
	"errors"
	"sync"
)

// Service is the platform operation dispatcher. It persists an operation
// before passing it to a capability-specific executor, so retries and status
// inspection survive API restarts.
type Service struct {
	repository Repository
	mu         sync.RWMutex
	drivers    map[string]Driver
}

func NewService(repository Repository, drivers ...Driver) (*Service, error) {
	if repository == nil {
		return nil, errors.New("operation repository is required")
	}
	service := &Service{repository: repository, drivers: map[string]Driver{}}
	for _, driver := range drivers {
		if err := service.RegisterDriver(driver); err != nil {
			return nil, err
		}
	}
	return service, nil
}

func (s *Service) RegisterDriver(driver Driver) error {
	if driver == nil || driver.Kind() == "" {
		return errors.New("operation driver and kind are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.drivers[driver.Kind()]; exists {
		return errors.New("operation driver already registered: " + driver.Kind())
	}
	s.drivers[driver.Kind()] = driver
	return nil
}

func (s *Service) Dispatch(ctx context.Context, request Request) (Operation, error) {
	return s.repository.Create(ctx, request)
}

// Execute runs a persisted operation through its registered driver. Workers
// call this method; HTTP handlers only dispatch durable intent.
func (s *Service) Execute(ctx context.Context, id string) error {
	operation, err := s.repository.Get(ctx, id)
	if err != nil {
		return err
	}
	s.mu.RLock()
	driver := s.drivers[operation.Kind]
	s.mu.RUnlock()
	if driver == nil {
		return s.repository.UpdateStatus(ctx, operation.ID, StatusFailed, "no operation driver registered")
	}
	if err := s.repository.UpdateStatus(ctx, operation.ID, StatusRunning, ""); err != nil {
		return err
	}
	if err := driver.Execute(ctx, operation); err != nil {
		_ = s.repository.UpdateStatus(ctx, operation.ID, StatusFailed, err.Error())
		return err
	}
	return s.repository.UpdateStatus(ctx, operation.ID, StatusSucceeded, "")
}

var _ Dispatcher = (*Service)(nil)
