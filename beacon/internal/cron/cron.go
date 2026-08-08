package cron

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
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
	jobWG     sync.WaitGroup
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
		if job.Interval <= 0 || job.RunFunc == nil {
			cancel()
			return fmt.Errorf("cron job %q has an invalid interval or callback", job.Name)
		}
		_, err := s.scheduler.Every(job.Interval).SingletonMode().Do(func() {
			s.jobWG.Add(1)
			defer s.jobWG.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					slog.Error("cron job panicked", "job", job.Name, "panic", recovered, "stack", string(debug.Stack()))
				}
			}()
			if err := job.RunFunc(ctx); err != nil && ctx.Err() == nil {
				slog.Warn("cron job failed", "job", job.Name, "error", err)
			}
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
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.scheduler.Stop()
	cancel := s.cancel
	s.running = false
	s.mu.Unlock()

	finished := make(chan struct{})
	go func() {
		s.jobWG.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(30 * time.Second):
		if cancel != nil {
			cancel()
		}
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			slog.Error("cron jobs did not stop after cancellation")
		}
	}
	if cancel != nil {
		cancel()
	}
}

func (s *Scheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}
