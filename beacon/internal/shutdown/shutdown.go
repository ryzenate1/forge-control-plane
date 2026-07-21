package shutdown

import "time"

// ShutdownManager handles graceful shutdown of the application
type ShutdownManager struct {
	done    chan struct{}
	timeout time.Duration
}

// NewShutdownManager creates a new ShutdownManager
func NewShutdownManager(timeout time.Duration) *ShutdownManager {
	return &ShutdownManager{
		done:    make(chan struct{}),
		timeout: timeout,
	}
}

// Wait blocks until a shutdown signal is received or the done channel is closed
func (s *ShutdownManager) Wait() {
	<-s.done
}

// Shutdown initiates a graceful shutdown
func (s *ShutdownManager) Shutdown() {
	close(s.done)
	time.Sleep(s.timeout)
}
