package system

import (
	"sync"
	"testing"
	"time"
)

func TestDedupKey(t *testing.T) {
	k1 := dedupKey("write", "/data/file.txt")
	k2 := dedupKey("write", "/data/file.txt")
	k3 := dedupKey("delete", "/data/file.txt")
	k4 := dedupKey("write", "/data/other.txt")

	if k1 != k2 {
		t.Fatal("identical action+path should produce same key")
	}
	if k1 == k3 {
		t.Fatal("different action should produce different key")
	}
	if k1 == k4 {
		t.Fatal("different path should produce different key")
	}
}

func TestDuplicateEntriesAreDeduped(t *testing.T) {
	var mu sync.Mutex
	var flushed []ActivityEntry

	d := NewActivityDedup(time.Minute, 500, func(serverID string, entries []ActivityEntry) {
		mu.Lock()
		defer mu.Unlock()
		flushed = append(flushed, entries...)
	})

	// Record the same action+path 5 times — should collapse to 1.
	for i := 0; i < 5; i++ {
		d.Record("srv-1", "sftp.write", "/data/file.txt", "10.0.0.1")
	}

	d.Flush()

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 1 {
		t.Fatalf("expected 1 deduped entry, got %d", len(flushed))
	}
	if flushed[0].Action != "sftp.write" || flushed[0].Path != "/data/file.txt" {
		t.Fatalf("unexpected entry: %+v", flushed[0])
	}
}

func TestDifferentPathsNotDeduped(t *testing.T) {
	var mu sync.Mutex
	var flushed []ActivityEntry

	d := NewActivityDedup(time.Minute, 500, func(serverID string, entries []ActivityEntry) {
		mu.Lock()
		defer mu.Unlock()
		flushed = append(flushed, entries...)
	})

	d.Record("srv-1", "sftp.write", "/data/a.txt", "10.0.0.1")
	d.Record("srv-1", "sftp.write", "/data/b.txt", "10.0.0.1")
	d.Record("srv-1", "sftp.write", "/data/c.txt", "10.0.0.1")

	d.Flush()

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 3 {
		t.Fatalf("expected 3 entries for different paths, got %d", len(flushed))
	}
}

func TestDifferentActionsNotDeduped(t *testing.T) {
	var mu sync.Mutex
	var flushed []ActivityEntry

	d := NewActivityDedup(time.Minute, 500, func(serverID string, entries []ActivityEntry) {
		mu.Lock()
		defer mu.Unlock()
		flushed = append(flushed, entries...)
	})

	d.Record("srv-1", "sftp.write", "/data/file.txt", "10.0.0.1")
	d.Record("srv-1", "sftp.delete", "/data/file.txt", "10.0.0.1")

	d.Flush()

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 2 {
		t.Fatalf("expected 2 entries for different actions, got %d", len(flushed))
	}
}

func TestFlushCallbackReceivesCorrectServerID(t *testing.T) {
	var mu sync.Mutex
	results := make(map[string]int)

	d := NewActivityDedup(time.Minute, 500, func(serverID string, entries []ActivityEntry) {
		mu.Lock()
		defer mu.Unlock()
		results[serverID] += len(entries)
	})

	d.Record("srv-1", "sftp.write", "/a.txt", "10.0.0.1")
	d.Record("srv-2", "sftp.write", "/b.txt", "10.0.0.2")
	d.Record("srv-1", "sftp.delete", "/c.txt", "10.0.0.1")

	d.Flush()

	mu.Lock()
	defer mu.Unlock()
	if results["srv-1"] != 2 {
		t.Fatalf("expected 2 entries for srv-1, got %d", results["srv-1"])
	}
	if results["srv-2"] != 1 {
		t.Fatalf("expected 1 entry for srv-2, got %d", results["srv-2"])
	}
}

func TestFlushClearsEntries(t *testing.T) {
	callCount := 0
	d := NewActivityDedup(time.Minute, 500, func(serverID string, entries []ActivityEntry) {
		callCount++
	})

	d.Record("srv-1", "sftp.write", "/a.txt", "10.0.0.1")
	d.Flush()
	d.Flush() // second flush should have nothing

	if callCount != 1 {
		t.Fatalf("expected flush callback called once, got %d", callCount)
	}
}

func TestMaxBatchSplitsEntries(t *testing.T) {
	var mu sync.Mutex
	var batchSizes []int

	d := NewActivityDedup(time.Minute, 3, func(serverID string, entries []ActivityEntry) {
		mu.Lock()
		defer mu.Unlock()
		batchSizes = append(batchSizes, len(entries))
	})

	// Record 7 distinct entries.
	for i := 0; i < 7; i++ {
		d.Record("srv-1", "sftp.write", "/data/"+string(rune('a'+i))+".txt", "10.0.0.1")
	}

	d.Flush()

	mu.Lock()
	defer mu.Unlock()

	// Expect batches to be split: at least two callbacks needed for 7 items.
	total := 0
	for _, s := range batchSizes {
		if s > 3 {
			t.Fatalf("batch size %d exceeds maxBatch 3", s)
		}
		total += s
	}
	if total != 7 {
		t.Fatalf("expected 7 total entries across batches, got %d", total)
	}
}

func TestAutoFlushViaTicker(t *testing.T) {
	var mu sync.Mutex
	var flushed []ActivityEntry

	d := NewActivityDedup(50*time.Millisecond, 500, func(serverID string, entries []ActivityEntry) {
		mu.Lock()
		defer mu.Unlock()
		flushed = append(flushed, entries...)
	})

	d.Record("srv-1", "sftp.write", "/data/auto.txt", "10.0.0.1")
	d.Start()
	defer d.Stop()

	// Wait for the ticker to fire at least once.
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) == 0 {
		t.Fatal("expected auto-flush to have fired at least once")
	}
}

func TestStopTerminatesGoroutine(t *testing.T) {
	var mu sync.Mutex
	callCount := 0

	d := NewActivityDedup(50*time.Millisecond, 500, func(serverID string, entries []ActivityEntry) {
		mu.Lock()
		defer mu.Unlock()
		callCount++
	})

	d.Record("srv-1", "sftp.write", "/data/stop.txt", "10.0.0.1")
	d.Start()
	d.Stop()

	// The final flush from Stop() should have sent the pending entry.
	mu.Lock()
	countAfterStop := callCount
	mu.Unlock()

	if countAfterStop == 0 {
		t.Fatal("expected Stop() to perform a final flush")
	}

	// Record another entry after stop — it should NOT auto-flush.
	d.Record("srv-1", "sftp.write", "/data/after.txt", "10.0.0.1")
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if callCount != countAfterStop {
		t.Fatal("expected no more flushes after Stop()")
	}
}

func TestStopWithoutStartIsNoop(t *testing.T) {
	d := NewActivityDedup(time.Minute, 500, func(serverID string, entries []ActivityEntry) {})
	// Should not panic.
	d.Stop()
}

func TestStartIdempotent(t *testing.T) {
	d := NewActivityDedup(time.Minute, 500, func(serverID string, entries []ActivityEntry) {})
	d.Start()
	d.Start() // second call should be no-op
	d.Stop()
}

func TestDedupKeepsLatestTimestamp(t *testing.T) {
	var mu sync.Mutex
	var flushed []ActivityEntry

	d := NewActivityDedup(time.Minute, 500, func(serverID string, entries []ActivityEntry) {
		mu.Lock()
		defer mu.Unlock()
		flushed = append(flushed, entries...)
	})

	// Record twice — the second call happens after the first, so its
	// timestamp should be kept (time.Now() is called inside Record).
	d.Record("srv-1", "sftp.write", "/data/file.txt", "10.0.0.1")
	time.Sleep(5 * time.Millisecond)
	d.Record("srv-1", "sftp.write", "/data/file.txt", "10.0.0.2")

	d.Flush()

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 1 {
		t.Fatalf("expected 1 deduped entry, got %d", len(flushed))
	}
	// The latest record should have the second IP.
	if flushed[0].IP != "10.0.0.2" {
		t.Fatalf("expected latest IP 10.0.0.2, got %s", flushed[0].IP)
	}
}

func TestConcurrentRecords(t *testing.T) {
	var mu sync.Mutex
	var flushed []ActivityEntry

	d := NewActivityDedup(time.Minute, 500, func(serverID string, entries []ActivityEntry) {
		mu.Lock()
		defer mu.Unlock()
		flushed = append(flushed, entries...)
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.Record("srv-1", "sftp.write", "/data/file.txt", "10.0.0.1")
		}()
	}
	wg.Wait()

	d.Flush()

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 1 {
		t.Fatalf("expected 1 deduped entry after 100 concurrent writes, got %d", len(flushed))
	}
}
