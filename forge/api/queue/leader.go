package queue

import (
	"context"
	"log/slog"
	"runtime"
	"sync/atomic"
	"time"
)

type LeaderConfig struct {
	ClientID      string
	Schema        string
	ElectInterval time.Duration
	TTL           time.Duration
}

type Elector struct {
	exec     Executor
	config   *LeaderConfig
	isLeader atomic.Bool

	leaderCh chan bool
	stopCh   chan struct{}
	doneCh   chan struct{}
	logger   *slog.Logger
}

func NewElector(exec Executor, config *LeaderConfig) *Elector {
	if config.ElectInterval <= 0 {
		config.ElectInterval = 5 * time.Second
	}
	if config.TTL <= 0 {
		config.TTL = 15 * time.Second
	}
	return &Elector{
		exec:     exec,
		config:   config,
		leaderCh: make(chan bool, 1),
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
		logger:   slog.Default(),
	}
}

func (e *Elector) Start(ctx context.Context) {
	go func() {
		defer close(e.doneCh)
		defer func() {
			if r := recover(); r != nil {
				buf := make([]byte, 4096)
				n := runtime.Stack(buf, false)
				e.logger.Error("leader elector panic recovered", "panic", r, "stack", string(buf[:n]))
			}
		}()

		for {
			select {
			case <-e.stopCh:
				e.resign(ctx)
				return
			default:
			}

			leader, err := e.exec.LeaderAttemptElect(ctx, &LeaderElectParams{
				LeaderID: e.config.ClientID,
				Now:      timeNowPtr(),
				Schema:   e.config.Schema,
				TTL:      e.config.TTL,
			})
			if err != nil || leader == nil {
				if e.isLeader.Load() {
					e.isLeader.Store(false)
					e.notify(false)
				}
				select {
				case <-e.stopCh:
					return
				case <-time.After(e.config.ElectInterval):
				}
				continue
			}

			if !e.isLeader.Swap(true) {
				e.notify(true)
			}

			e.runLeaderLoop(ctx)
		}
	}()
}

func (e *Elector) runLeaderLoop(ctx context.Context) {
	ticker := time.NewTicker(e.config.ElectInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			_, err := e.exec.LeaderAttemptReelect(ctx, &LeaderReelectParams{
				ElectedAt: time.Now(),
				LeaderID:  e.config.ClientID,
				Now:       timeNowPtr(),
				Schema:    e.config.Schema,
				TTL:       e.config.TTL,
			})
			if err != nil {
				e.isLeader.Store(false)
				e.notify(false)
				return
			}
		}
	}
}

func (e *Elector) Stop() {
	close(e.stopCh)
	<-e.doneCh
}

func (e *Elector) IsLeader() bool {
	return e.isLeader.Load()
}

func (e *Elector) Listen() <-chan bool {
	return e.leaderCh
}

func (e *Elector) notify(isLeader bool) {
	select {
	case e.leaderCh <- isLeader:
	default:
	}
}

func (e *Elector) resign(ctx context.Context) {
	_, _ = e.exec.LeaderResign(ctx, &LeaderResignParams{
		LeaderID: e.config.ClientID,
		Schema:   e.config.Schema,
	})
}

func timeNowPtr() *time.Time {
	t := time.Now().UTC()
	return &t
}
