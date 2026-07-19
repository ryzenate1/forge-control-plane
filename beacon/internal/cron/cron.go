package cron

import (
	"context"
	"sync"
	"time"

	"github.com/go-co-op/gocron"
)

type Job struct {
	Name     string
	Interval time.Duration
	RunFunc  func(ctx context.Context) error
}

type Scheduler struct {
	scheduler *gocron.Scheduler
	jobs      []*Job
	mu        sync.Mutex
	running   bool
	cancel    context.CancelFunc
}

func NewScheduler(timezone string) (*Scheduler, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, err
	}
	return &Scheduler{
		scheduler: gocron.NewScheduler(loc),
	}, nil
}

func (s *Scheduler) AddJob(name string, interval time.Duration, fn func(ctx context.Context) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = append(s.jobs, &Job{
		Name:     name,
		Interval: interval,
		RunFunc:  fn,
	})
}

func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	for _, j := range s.jobs {
		job := j
		_, err := s.scheduler.Every(job.Interval).Do(func() {
			_ = job.RunFunc(ctx)
		})
		if err != nil {
			cancel()
			return err
		}
	}
	s.scheduler.StartAsync()
	s.running = true
	return nil
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.scheduler.Stop()
	if s.cancel != nil {
		s.cancel()
	}
	s.running = false
}

func (s *Scheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}
