package queue

import (
	"strings"
	"testing"
)

// Regression tests for Phase-1 finding F-18: retry accounting.
//
// Contract: a lease steal (worker death) must NOT consume a retry — the work
// did not fail, the worker did. Only an observed handler failure (Retry) or
// terminal failure (Fail) increments retry_count. Previously Dequeue added
// +1 on stealing a 'running' candidate AND Retry added another +1, so a
// steal followed by one real failure consumed two retries.

func TestDequeueDoesNotAccountRetries(t *testing.T) {
	// The RETURNING clause may read retry_count; the UPDATE SET clause must
	// never assign it (that would double-count steals + failures).
	if strings.Contains(dequeueSQL, "SET") && strings.Contains(dequeueSQL, "retry_count=") {
		t.Fatalf("dequeueSQL assigns retry_count during claim (steal must not consume a retry):\n%s", dequeueSQL)
	}
}

func TestRetryIsSoleRetryAccountant(t *testing.T) {
	if !strings.Contains(retrySQL, "retry_count=retry_count+1") {
		t.Fatalf("retrySQL must increment retry_count exactly once per real failure:\n%s", retrySQL)
	}
}
