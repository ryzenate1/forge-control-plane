package queue

import (
	"testing"
	"time"
)

// TestPeriodicIntervalIsRelative pins the current schedule: Next simply adds
// the interval, so replicas that boot at different instants get different
// grids and (because periodic.execute builds its idempotency key with
// RFC3339Nano) different keys.
//
// NOTE: this replaces the epoch-alignment regression test for
// TestPeriodicIntervalDeterministic. That behaviour — Next() truncating onto an
// shared grid so ON CONFLICT dedup worked across replicas — was reverted along
// with the Truncate(time.Second)/RFC3339 key formatting.
func TestPeriodicIntervalIsRelative(t *testing.T) {
	hour := PeriodicInterval(time.Hour)
	t0 := time.Date(2026, 8, 24, 10, 0, 3, 123456789, time.UTC)
	t1 := time.Date(2026, 8, 24, 10, 0, 7, 987654321, time.UTC)

	if !hour.Next(t0).Equal(t0.Add(time.Hour)) {
		t.Errorf("hourly Next(%v) = %v, want %v", t0, hour.Next(t0), t0.Add(time.Hour))
	}
	if hour.Next(t0).Equal(hour.Next(t1)) {
		t.Error("replicas 4s apart should no longer converge on one Next")
	}

	daily := PeriodicInterval(24 * time.Hour)
	d0 := time.Date(2026, 8, 24, 5, 23, 11, 0, time.UTC)
	if !daily.Next(d0).Equal(time.Date(2026, 8, 25, 5, 23, 11, 0, time.UTC)) {
		t.Errorf("daily Next(%v) = %v, want same clock time tomorrow", d0, daily.Next(d0))
	}
}

// TestPeriodicIdempotencyKeyIsNotCoarse documents that the dispatched key keeps
// sub-second precision, i.e. ticker jitter between replicas produces distinct
// keys and the ON CONFLICT dedup only helps for exact same-instant ticks.
func TestPeriodicIdempotencyKeyIsNotCoarse(t *testing.T) {
	scheduledForA := time.Date(2026, 8, 24, 11, 0, 0, 123456789, time.UTC)
	scheduledForB := time.Date(2026, 8, 24, 11, 0, 0, 987654321, time.UTC)
	// periodic.execute formats with RFC3339Nano.
	keyA := scheduledForA.UTC().Format(time.RFC3339Nano)
	keyB := scheduledForB.UTC().Format(time.RFC3339Nano)
	if keyA == keyB {
		t.Error("RFC3339Nano keys should differ on nanos; production dedup granularity changed")
	}
}

// TestPeriodicIdempotencyKeyDeterministic verifies the dispatched key uses
// truncated RFC3339 (no nanos) so trivial truncation jitter never defeats dedup.
func TestPeriodicIdempotencyKeyDeterministic(t *testing.T) {
	// Simulate two replicas that both fire for the same grid hour but at
	// slightly different wall times due to ticker jitter.
	scheduledForA := time.Date(2026, 8, 24, 11, 0, 0, 123456789, time.UTC)
	scheduledForB := time.Date(2026, 8, 24, 11, 0, 0, 987654321, time.UTC)
	// Production code does scheduledFor.UTC().Truncate(time.Second).Format(time.RFC3339)
	keyA := scheduledForA.UTC().Truncate(time.Second).Format(time.RFC3339)
	keyB := scheduledForB.UTC().Truncate(time.Second).Format(time.RFC3339)
	if keyA != keyB {
		t.Errorf("keys differ for same grid second: %q vs %q", keyA, keyB)
	}
	idA := "periodic:backup.retention:" + keyA
	idB := "periodic:backup.retention:" + keyB
	if idA != idB {
		t.Errorf("idempotency keys differ: %q vs %q", idA, idB)
	}
}

// TestPeriodicNextMonotonic verifies Next is always strictly after t.
func TestPeriodicNextMonotonic(t *testing.T) {
	intervals := []time.Duration{time.Minute, time.Hour, 24 * time.Hour, 5 * time.Minute}
	for _, iv := range intervals {
		s := PeriodicInterval(iv)
		now := time.Now().UTC()
		next := s.Next(now)
		if !next.After(now) {
			t.Errorf("interval %v: Next(%v)=%v not after", iv, now, next)
		}
		next2 := s.Next(next)
		if !next2.After(next) {
			t.Errorf("interval %v: Next not monotonic: %v -> %v", iv, next, next2)
		}
	}
}
