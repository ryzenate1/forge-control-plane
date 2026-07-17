package shutdown

import (
	"testing"
	"time"
)

func TestShutdownManager(t *testing.T) {
	// Test Wait and Shutdown
	manager := NewShutdownManager(1 * time.Second)

	// Start a goroutine to simulate shutdown
	go func() {
		time.Sleep(500 * time.Millisecond)
		manager.Shutdown()
	}()

	// Wait should return when Shutdown is called
	start := time.Now()
	manager.Wait()
	duration := time.Since(start)

	// Verify the wait time is approximately equal to the timeout
	if duration < 500*time.Millisecond || duration > 1500*time.Millisecond {
		t.Errorf("Expected wait duration between 500ms and 1500ms, got %v", duration)
	}
}
