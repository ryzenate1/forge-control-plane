package replicamanager

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestManagerStartStop(t *testing.T) {
	m := &Manager{
		store:   nil,
		started: atomic.Bool{},
		stopCh:  make(chan struct{}),
	}
	m.started.Store(false)

	if m.started.Load() {
		t.Fatal("should not be started initially")
	}

	// Start with nil store - should NOT set started
	m.Start(context.Background())
	if m.started.Load() {
		t.Fatal("Start with nil store should not mark as started")
	}
}

func TestManagerDoubleStartGuard(t *testing.T) {
	m := &Manager{
		store:   nil,
		started: atomic.Bool{},
	}
	m.started.Store(false)

	m.Start(context.Background())
	m.Start(context.Background())
	// Should not panic; second start should be no-op
}

func TestManagerStartNilStore(t *testing.T) {
	m := &Manager{}
	m.Start(context.Background())
	if m.started.Load() {
		t.Fatal("Start with nil store should not mark as started")
	}
}

func TestManagerStartContextCancelled(t *testing.T) {
	m := &Manager{store: nil}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m.Start(ctx)
	// Should not panic or hang
	time.Sleep(10 * time.Millisecond)
}

func TestManagerStopWithoutStart(t *testing.T) {
	m := &Manager{}
	m.Stop()
	// Should not panic
}

func TestManagerNilStop(t *testing.T) {
	var m *Manager
	m.Stop()
	// Should not panic
}

func TestManagerNilStart(t *testing.T) {
	var m *Manager
	m.Start(context.Background())
	// Should not panic
}

func TestManagerStopPanicOnNilStopCh(t *testing.T) {
	m := &Manager{
		started: atomic.Bool{},
	}
	m.started.Store(true)
	// stopCh is nil; Stop should not panic
	m.Stop()
}
