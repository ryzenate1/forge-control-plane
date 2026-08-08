package queue

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"time"
)

type producer struct {
	queue     string
	config    QueueConfig
	clientCfg *Config
	exec      Executor
	workers   *Workers

	clientID string

	sem chan struct{}

	activeJobs sync.WaitGroup
	activeMu   sync.Mutex

	cancel  context.CancelFunc
	stopped bool
}

func newProducer(queue string, config QueueConfig, clientCfg *Config, exec Executor, clientID string) *producer {
	return &producer{
		queue:     queue,
		config:    config,
		clientCfg: clientCfg,
		exec:      exec,
		workers:   clientCfg.Workers,
		clientID:  clientID,
		sem:       make(chan struct{}, config.MaxWorkers),
	}
}

func (p *producer) Start(ctx context.Context) {
	ctx, p.cancel = context.WithCancel(ctx)
	go p.loop(ctx)
}

func (p *producer) loop(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			buf := make([]byte, 4096)
			n := runtime.Stack(buf, false)
			p.clientCfg.Logger.Error("producer loop panic recovered",
				"queue", p.queue, "panic", r, "stack", string(buf[:n]))
		}
	}()
	logger := p.clientCfg.Logger.With(slog.String("queue", p.queue))

	p.fetchAndProcess(ctx)

	ticker := time.NewTicker(p.clientCfg.FetchPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("producer loop stopped")
			return
		case <-ticker.C:
			p.fetchAndProcess(ctx)
		}
	}
}

func (p *producer) fetchAndProcess(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	now := time.Now()
	jobs, err := p.exec.JobGetAvailable(ctx, &JobGetAvailableParams{
		ClientID:  p.clientID,
		MaxToLock: p.config.MaxWorkers,
		Now:       &now,
		Queue:     p.queue,
		Schema:    p.clientCfg.Schema,
	})
	if err != nil {
		p.clientCfg.Logger.Error("failed to fetch available jobs",
			"queue", p.queue, "error", err)
		return
	}

	for _, job := range jobs {
		select {
		case <-ctx.Done():
			return
		case p.sem <- struct{}{}:
		}

		p.activeJobs.Add(1)
		go func(job *JobRow) {
			defer p.activeJobs.Done()
			defer func() { <-p.sem }()
			p.processJob(ctx, job)
		}(job)
	}
}

func (p *producer) processJob(ctx context.Context, job *JobRow) {
	logger := p.clientCfg.Logger.With(
		slog.String("queue", p.queue),
		slog.Int64("job_id", job.ID),
		slog.String("kind", job.Kind),
	)

	info, ok := p.workers.Lookup(job.Kind)
	if !ok {
		logger.Error("unknown job kind, marking as discarded")

		now := time.Now()
		_, _ = p.exec.JobSetStateIfRunningMany(ctx, &JobSetStateIfRunningManyParams{
			Jobs: []*JobSetStateIfRunningParams{
				{
					ID:          job.ID,
					Attempt:     &job.Attempt,
					State:       JobStateDiscarded,
					FinalizedAt: &now,
					Schema:      p.clientCfg.Schema,
				},
			},
			Schema: p.clientCfg.Schema,
		})
		return
	}

	exec := &jobExecutor{
		clientRetryPolicy: p.clientCfg.RetryPolicy,
		jobRow:            job,
		workFunc:          info.workFunc,
		logger:            logger,
	}

	jobCtx := ctx
	cancel := func() {}
	if p.clientCfg.JobTimeout > 0 {
		jobCtx, cancel = context.WithTimeout(ctx, p.clientCfg.JobTimeout)
	}
	defer cancel()
	res := exec.Execute(jobCtx)
	exec.reportResult(ctx, p.exec, res, p.clientCfg.Schema)
}

func (p *producer) Stop(ctx context.Context) {
	if p.cancel != nil {
		p.cancel()
	}

	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				p.clientCfg.Logger.Error("producer stop panic recovered",
					"queue", p.queue, "panic", r)
			}
		}()
		p.activeJobs.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		p.clientCfg.Logger.Warn("producer stop: caller context expired", "queue", p.queue, "error", ctx.Err())
	case <-time.After(30 * time.Second):
		p.clientCfg.Logger.Warn("producer stop: timeout waiting for active jobs",
			"queue", p.queue)
	}
}
