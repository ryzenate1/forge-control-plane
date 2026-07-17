package websocketlimiter

import (
	"testing"
)

func TestLimiterBucketAuthEvent(t *testing.T) {
	lb := NewLimiterBucket()
	if !lb.Allow(AuthenticationEvent) {
		t.Fatal("first auth event should be allowed")
	}
	if !lb.Allow(AuthenticationEvent) {
		t.Fatal("second auth event should be allowed (burst=2)")
	}
	if lb.Allow(AuthenticationEvent) {
		t.Fatal("third auth event should be denied (burst=2, rate=1/5s)")
	}
}

func TestLimiterBucketSendLogs(t *testing.T) {
	lb := NewLimiterBucket()
	if !lb.Allow(SendLogsEvent) {
		t.Fatal("first send logs should be allowed")
	}
	if !lb.Allow(SendLogsEvent) {
		t.Fatal("second send logs should be allowed (burst=2)")
	}
	if lb.Allow(SendLogsEvent) {
		t.Fatal("third send logs should be denied")
	}
}

func TestLimiterBucketSendCommand(t *testing.T) {
	lb := NewLimiterBucket()
	for i := 0; i < 10; i++ {
		if !lb.Allow(SendCommandEvent) {
			t.Fatalf("command %d should be allowed within burst of 10", i)
		}
	}
	if lb.Allow(SendCommandEvent) {
		t.Fatal("11th command should be denied")
	}
}

func TestLimiterBucketDefaultEvent(t *testing.T) {
	lb := NewLimiterBucket()
	unknown := Event("unknown_event")
	if !lb.Allow(unknown) {
		t.Fatal("first unknown event should be allowed")
	}
	if !lb.Allow(unknown) {
		t.Fatal("second unknown event should be allowed")
	}
	if !lb.Allow(unknown) {
		t.Fatal("third unknown event should be allowed")
	}
	if !lb.Allow(unknown) {
		t.Fatal("fourth unknown event should be allowed (burst=4)")
	}
	if lb.Allow(unknown) {
		t.Fatal("fifth unknown event should be denied (burst=4)")
	}
}

func TestLimiterBucketSetState(t *testing.T) {
	lb := NewLimiterBucket()
	if !lb.Allow(SetStateEvent) {
		t.Fatal("first set state should be allowed")
	}
	if !lb.Allow(SetStateEvent) {
		t.Fatal("second set state should be allowed")
	}
	if !lb.Allow(SetStateEvent) {
		t.Fatal("third set state should be allowed")
	}
	if !lb.Allow(SetStateEvent) {
		t.Fatal("fourth set state should be allowed (burst=4)")
	}
	if lb.Allow(SetStateEvent) {
		t.Fatal("fifth set state should be denied")
	}
}

func TestLimiterBucketSendStats(t *testing.T) {
	lb := NewLimiterBucket()
	if !lb.Allow(SendStatsEvent) {
		t.Fatal("first send stats should be allowed")
	}
	if !lb.Allow(SendStatsEvent) {
		t.Fatal("second send stats should be allowed")
	}
	if !lb.Allow(SendStatsEvent) {
		t.Fatal("third send stats should be allowed")
	}
	if !lb.Allow(SendStatsEvent) {
		t.Fatal("fourth send stats should be allowed (burst=4)")
	}
	if lb.Allow(SendStatsEvent) {
		t.Fatal("fifth send stats should be denied")
	}
}
