package metrics

import (
	"testing"
	"time"
)

func TestCollectProcess(t *testing.T) {
	started := time.Now().Add(-time.Hour)
	process := CollectProcess(started)

	if process.Goroutines <= 0 {
		t.Fatalf("expected positive goroutine count, got %d", process.Goroutines)
	}
	if process.MemAllocBytes == 0 {
		t.Fatal("expected non-zero heap allocation")
	}
	if process.MemHeapBytes < process.MemAllocBytes {
		t.Fatalf("heap reserved (%d) must not be smaller than allocation (%d)", process.MemHeapBytes, process.MemAllocBytes)
	}
	if process.UserCPUSeconds < 0 || process.SystemCPUSeconds < 0 {
		t.Fatalf("negative CPU time: user=%f system=%f", process.UserCPUSeconds, process.SystemCPUSeconds)
	}
	if !process.StartTime.Equal(started) {
		t.Fatalf("expected start time %v, got %v", started, process.StartTime)
	}
}

func TestPrometheusCollector(t *testing.T) {
	collector := NewPrometheusCollector()

	// Test RecordServerStatus
	collector.RecordServerStatus("healthy")

	// Test RecordBackupDuration
	collector.RecordBackupDuration(10 * time.Second)

	// Test RecordRequestLatency
	collector.RecordRequestLatency("GET", "/api/status", 500*time.Millisecond)
}
