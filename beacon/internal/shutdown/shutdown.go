package shutdown

import (
	"context"
	"sync"
	"time"
)

// ShutdownManager broadcasts shutdown exactly once. Timeout is used only by
// WaitContext; Shutdown itself never blocks the initiating goroutine.
type ShutdownManager struct {
	done    chan struct{}
	timeout time.Duration
	once    sync.Once
}

func NewShutdownManager(timeout time.Duration) *ShutdownManager {
	return &ShutdownManager{done: make(chan struct{}), timeout: timeout}
}

func (s *ShutdownManager) Wait() {
	<-s.done
}

func (s *ShutdownManager) WaitContext(ctx context.Context) error {
	waitCtx := ctx
	cancel := func() {}
	if s.timeout > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, s.timeout)
	}
	defer cancel()
	select {
	case <-s.done:
		return nil
	case <-waitCtx.Done():
		return waitCtx.Err()
	}
}

func (s *ShutdownManager) Shutdown() {
	s.once.Do(func() { close(s.done) })
}
