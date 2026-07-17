package server

import (
	"context"
	"testing"
	"time"
)

func TestStatsCollectorCollectNilRuntime(t *testing.T) {
	sc := NewStatsCollector(nil, 10)
	stats, err := sc.Collect(context.Background(), "srv-1")
	if err != nil {
		t.Fatal(err)
	}
	if stats.CPU != 0 || stats.MemoryMB != 0 {
		t.Fatalf("expected zero stats with nil runtime, got %+v", stats)
	}
	if stats.Timestamp.IsZero() {
		t.Fatal("expected non-zero timestamp")
	}
}

func TestStatsCollectorHistory(t *testing.T) {
	sc := NewStatsCollector(nil, 5)

	for i := 0; i < 8; i++ {
		sc.Collect(context.Background(), "srv-1")
	}

	history := sc.GetHistory("srv-1")
	if len(history) != 5 {
		t.Fatalf("expected max 5 history entries, got %d", len(history))
	}
}

func TestStatsCollectorHistoryDefault(t *testing.T) {
	sc := NewStatsCollector(nil, 0)
	if sc.maxHistory != 60 {
		t.Fatalf("expected default maxHistory 60, got %d", sc.maxHistory)
	}
}

func TestStatsCollectorRegisterUnregister(t *testing.T) {
	sc := NewStatsCollector(nil, 10)
	sc.RegisterServer("srv-1")
	sc.RegisterServer("srv-2")

	sc.Collect(context.Background(), "srv-1")
	sc.Collect(context.Background(), "srv-2")

	if len(sc.GetHistory("srv-1")) != 1 {
		t.Fatal("expected 1 history entry for srv-1")
	}

	sc.UnregisterServer("srv-1")
	if len(sc.GetHistory("srv-1")) != 0 {
		t.Fatal("expected 0 history entries after unregister")
	}
}

func TestStatsCollectorGetHistoryEmpty(t *testing.T) {
	sc := NewStatsCollector(nil, 10)
	h := sc.GetHistory("nonexistent")
	if len(h) != 0 {
		t.Fatalf("expected empty history, got %d", len(h))
	}
}

func TestStatsCollectorStart(t *testing.T) {
	sc := NewStatsCollector(nil, 100)
	sc.RegisterServer("srv-1")

	ctx, cancel := context.WithCancel(context.Background())
	sc.Start(ctx, 50*time.Millisecond)

	time.Sleep(180 * time.Millisecond)
	cancel()

	history := sc.GetHistory("srv-1")
	if len(history) < 2 {
		t.Fatalf("expected at least 2 collected entries, got %d", len(history))
	}
}

func TestStatsCollectorReturnsCopy(t *testing.T) {
	sc := NewStatsCollector(nil, 10)
	sc.Collect(context.Background(), "srv-1")

	h1 := sc.GetHistory("srv-1")
	h2 := sc.GetHistory("srv-1")
	if len(h1) != 1 || len(h2) != 1 {
		t.Fatal("expected 1 entry in each")
	}
	h1[0].CPU = 999
	if h2[0].CPU == 999 {
		t.Fatal("GetHistory should return a copy")
	}
}
