package shutdown

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

// ShutdownManager handles graceful shutdown of the application
type ShutdownManager struct {
	signals chan os.Signal
	done    chan struct{}
	timeout time.Duration
}

// NewShutdownManager creates a new ShutdownManager
func NewShutdownManager(timeout time.Duration) *ShutdownManager {
	return &ShutdownManager{
		signals: make(chan os.Signal, 1),
		done:    make(chan struct{}),
		timeout: timeout,
	}
}

// Wait blocks until a shutdown signal is received or the done channel is closed
func (s *ShutdownManager) Wait() {
	signal.Notify(s.signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-s.signals:
	case <-s.done:
	}
}

// Shutdown initiates a graceful shutdown
func (s *ShutdownManager) Shutdown() {
	close(s.done)
	time.Sleep(s.timeout)
}
