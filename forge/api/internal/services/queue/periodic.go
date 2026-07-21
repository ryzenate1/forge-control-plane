package queue

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type PeriodicSchedule interface {
	Next(current time.Time) time.Time
}

type PeriodicJobConstructor func() (JobType, any, *Job)

type PeriodicJob struct {
	constructor   PeriodicJobConstructor
	id            string
	schedule      PeriodicSchedule
	runOnStart    bool
	nextRun       time.Time
}

type PeriodicJobOpts struct {
	ID        string
	RunOnStart bool
}

func NewPeriodicJob(schedule PeriodicSchedule, constructor PeriodicJobConstructor, opts *PeriodicJobOpts) *PeriodicJob {
	j := &PeriodicJob{
		constructor: constructor,
		schedule:    schedule,
	}
	if opts != nil {
		j.id = opts.ID
		j.runOnStart = opts.RunOnStart
	}
	return j
}

func PeriodicInterval(d time.Duration) PeriodicSchedule {
	return &periodicIntervalSchedule{interval: d}
}

type periodicIntervalSchedule struct {
	interval time.Duration
}

func (s *periodicIntervalSchedule) Next(t time.Time) time.Time {
	return t.Add(s.interval)
}

type PeriodicJobScheduler struct {
	service *Service
	jobs    []*PeriodicJob
	mu      sync.Mutex
	wg      sync.WaitGroup
	cancel  context.CancelFunc
}

func NewPeriodicJobScheduler(svc *Service) *PeriodicJobScheduler {
	return &PeriodicJobScheduler{service: svc}
}

func (s *PeriodicJobScheduler) Add(job *PeriodicJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	job.nextRun = job.schedule.Next(now)
	s.jobs = append(s.jobs, job)
}

func (s *PeriodicJobScheduler) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)
	s.wg.Add(1)
	go s.loop(ctx)
}

func (s *PeriodicJobScheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

func (s *PeriodicJobScheduler) loop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.tick(now.UTC())
		}
	}
}

func (s *PeriodicJobScheduler) tick(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.jobs {
		if j.nextRun.IsZero() {
			if j.runOnStart {
				j.nextRun = now
			} else {
				j.nextRun = j.schedule.Next(now)
			}
		}
		if !now.Before(j.nextRun) {
			go s.execute(j)
			j.nextRun = j.schedule.Next(now)
		}
	}
}

func (s *PeriodicJobScheduler) execute(job *PeriodicJob) {
	jobType, payload, opts := job.constructor()
	if jobType == "" {
		return
	}
	ctx := context.Background()
	dispatchOpts := opts
	if dispatchOpts == nil {
		dispatchOpts = &Job{}
	}
	_, err := s.service.DispatchIdempotent(ctx, "", jobType, dispatchOpts.ServerID, dispatchOpts.NodeID, payload, dispatchOpts.Priority)
	if err != nil {
		slog.Error("periodic job dispatch failed", "id", job.id, "type", jobType, "error", err)
	}
}
