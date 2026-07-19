package observability

import (
	"context"
	"testing"
	"time"
)

func TestCollectSystemMetrics(t *testing.T) {
	m, err := CollectSystemMetrics()
	if err != nil {
		t.Fatal(err)
	}
	if m.GoroutineCount <= 0 {
		t.Fatalf("expected positive goroutine count, got %d", m.GoroutineCount)
	}
	if m.HeapAlloc == 0 {
		t.Fatal("expected non-zero heap alloc")
	}
	if m.HeapSys == 0 {
		t.Fatal("expected non-zero heap sys")
	}
	if m.NumCPU <= 0 {
		t.Fatalf("expected positive CPU count, got %d", m.NumCPU)
	}
	if m.MemoryUsage < 0 || m.MemoryUsage > 100 {
		t.Fatalf("expected memory usage 0-100, got %f", m.MemoryUsage)
	}
	if m.Timestamp.IsZero() {
		t.Fatal("expected non-zero timestamp")
	}
}

func TestMetricsHistory_Add(t *testing.T) {
	h := NewMetricsHistory(5)
	for i := 0; i < 3; i++ {
		h.Add(SystemMetrics{
			GoroutineCount: i + 1,
			Timestamp:      time.Now(),
		})
	}
	snapshots := h.GetSnapshots()
	if len(snapshots) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(snapshots))
	}
}

func TestMetricsHistory_MaxSize(t *testing.T) {
	h := NewMetricsHistory(3)
	for i := 0; i < 10; i++ {
		h.Add(SystemMetrics{
			GoroutineCount: i + 1,
			Timestamp:      time.Now(),
		})
	}
	snapshots := h.GetSnapshots()
	if len(snapshots) != 3 {
		t.Fatalf("expected 3 snapshots (max), got %d", len(snapshots))
	}
	if snapshots[0].GoroutineCount != 8 {
		t.Fatalf("expected first snapshot goroutine count 8, got %d", snapshots[0].GoroutineCount)
	}
}

func TestMetricsHistory_Latest(t *testing.T) {
	h := NewMetricsHistory(5)

	if got := h.Latest(); got != nil {
		t.Fatal("expected nil for empty history")
	}

	h.Add(SystemMetrics{GoroutineCount: 42})
	latest := h.Latest()
	if latest == nil {
		t.Fatal("expected non-nil latest")
	}
	if latest.GoroutineCount != 42 {
		t.Fatalf("expected 42, got %d", latest.GoroutineCount)
	}
}

func TestMetricsHistory_DefaultMaxSize(t *testing.T) {
	h := NewMetricsHistory(0)
	if h.maxSize != 60 {
		t.Fatalf("expected default max size 60, got %d", h.maxSize)
	}
}

func TestMetricsHistory_StartCollection(t *testing.T) {
	h := NewMetricsHistory(10)
	ctx, cancel := context.WithCancel(context.Background())

	h.StartCollection(ctx, 50*time.Millisecond)
	time.Sleep(200 * time.Millisecond)
	cancel()

	snapshots := h.GetSnapshots()
	if len(snapshots) < 2 {
		t.Fatalf("expected at least 2 snapshots from collection, got %d", len(snapshots))
	}
}

func TestMetricsHistory_GetSnapshots_ReturnsCopy(t *testing.T) {
	h := NewMetricsHistory(5)
	h.Add(SystemMetrics{GoroutineCount: 1})

	s1 := h.GetSnapshots()
	s1[0].GoroutineCount = 999

	s2 := h.GetSnapshots()
	if s2[0].GoroutineCount != 1 {
		t.Fatal("GetSnapshots should return a copy")
	}
}
