package throttle

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestConsoleThrottleAllow(t *testing.T) {
	ct := NewConsoleThrottle(Config{Enabled: true, Lines: 5, Period: 100}, nil)
	for i := 0; i < 5; i++ {
		if !ct.Allow() {
			t.Fatalf("request %d should be allowed", i)
		}
	}
	if ct.Allow() {
		t.Fatal("request beyond limit should be denied")
	}
}

func TestConsoleThrottleStrike(t *testing.T) {
	var strikes int32
	ct := NewConsoleThrottle(Config{Enabled: true, Lines: 2, Period: 100}, func() {
		atomic.AddInt32(&strikes, 1)
	})
	ct.Allow()
	ct.Allow()
	ct.Allow()
	ct.Allow()
	s := atomic.LoadInt32(&strikes)
	if s != 1 {
		t.Fatalf("expected exactly 1 strike, got %d", s)
	}
}

func TestConsoleThrottleDisabled(t *testing.T) {
	ct := NewConsoleThrottle(Config{Enabled: false, Lines: 1, Period: 100}, nil)
	for i := 0; i < 100; i++ {
		if !ct.Allow() {
			t.Fatalf("disabled throttle should always allow, failed at %d", i)
		}
	}
}

func TestConsoleThrottleReset(t *testing.T) {
	ct := NewConsoleThrottle(Config{Enabled: true, Lines: 2, Period: 100}, nil)
	ct.Allow()
	ct.Allow()
	if ct.Allow() {
		t.Fatal("should be denied")
	}
	ct.Reset()
	if !ct.Allow() {
		t.Fatal("should be allowed after reset")
	}
}

func TestConsoleThrottleDefaults(t *testing.T) {
	ct := NewConsoleThrottle(Config{Enabled: true}, nil)
	if ct == nil {
		t.Fatal("expected non-nil throttle")
	}
	for i := 0; i < 100; i++ {
		ct.Allow()
	}
}

func TestConsoleThrottlePeriodRecovery(t *testing.T) {
	ct := NewConsoleThrottle(Config{Enabled: true, Lines: 2, Period: 50}, nil)
	ct.Allow()
	ct.Allow()
	if ct.Allow() {
		t.Fatal("should be denied")
	}
	time.Sleep(60 * time.Millisecond)
	if !ct.Allow() {
		t.Fatal("should recover after period elapses")
	}
}
