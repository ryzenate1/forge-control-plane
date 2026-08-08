package queue

import (
	"sync"
	"time"
)

type PeriodicSchedule interface {
	Next(from time.Time) time.Time
}

type PeriodicInterval struct {
	Interval time.Duration
}

func (s *PeriodicInterval) Next(from time.Time) time.Time {
	return from.Add(s.Interval)
}

func PeriodicIntervalSchedule(interval time.Duration) *PeriodicInterval {
	if interval <= 0 {
		panic("periodic interval must be positive")
	}
	return &PeriodicInterval{Interval: interval}
}

type PeriodicJob struct {
	Schedule PeriodicSchedule
	JobFunc  func() (JobArgs, *InsertOpts)
}

type PeriodicJobBundle struct {
	mu   sync.Mutex
	jobs map[string]*PeriodicJob
}

func NewPeriodicJobBundle() *PeriodicJobBundle {
	return &PeriodicJobBundle{
		jobs: make(map[string]*PeriodicJob),
	}
}

func (b *PeriodicJobBundle) Add(id string, job *PeriodicJob) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.jobs[id] = job
}

func (b *PeriodicJobBundle) AddMany(jobs map[string]*PeriodicJob) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, job := range jobs {
		b.jobs[id] = job
	}
}

func (b *PeriodicJobBundle) Remove(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.jobs, id)
}

func (b *PeriodicJobBundle) RemoveByID(ids ...string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, id := range ids {
		delete(b.jobs, id)
	}
}

func (b *PeriodicJobBundle) Jobs() map[string]*PeriodicJob {
	b.mu.Lock()
	defer b.mu.Unlock()
	jobs := make(map[string]*PeriodicJob, len(b.jobs))
	for k, v := range b.jobs {
		jobs[k] = v
	}
	return jobs
}
