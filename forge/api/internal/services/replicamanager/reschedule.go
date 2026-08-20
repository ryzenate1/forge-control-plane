package replicamanager

import (
	"fmt"
	"math"
	"time"
)

// ReschedulePolicy mirrors Nomad's ReschedulePolicy but minimal for forge.
// Controls how failed placements are retried: attempt cap, interval window,
// delay function (constant/exponential/fibonacci) and max delay cap.
// Additive, feature-flagged via ReschedulePolicy.Enabled().
type ReschedulePolicy struct {
	// Attempts limits rescheduling attempts within Interval. 0 means no limit
	// when Unlimited is true; otherwise 0 disables rescheduling.
	Attempts int `json:"attempts"`
	// Interval is the window in which Attempts are counted.
	Interval time.Duration `json:"interval"`
	// Delay is the initial delay before first retry.
	Delay time.Duration `json:"delay"`
	// DelayFunction is "constant", "exponential" or "fibonacci".
	DelayFunction string `json:"delayFunction"`
	// MaxDelay caps the delay when DelayFunction != "constant".
	MaxDelay time.Duration `json:"maxDelay"`
	// Unlimited allows infinite attempts (Nomad DefaultServiceJobReschedulePolicy).
	Unlimited bool `json:"unlimited"`
}

// DefaultReschedulePolicy is the service-default: exponential backoff, unlimited attempts.
var DefaultReschedulePolicy = ReschedulePolicy{
	Attempts:      0,
	Interval:      24 * time.Hour,
	Delay:         30 * time.Second,
	DelayFunction: "exponential",
	MaxDelay:      1 * time.Hour,
	Unlimited:     true,
}

// DefaultBatchReschedulePolicy mirrors Nomad's batch default: single attempt.
var DefaultBatchReschedulePolicy = ReschedulePolicy{
	Attempts:      1,
	Interval:      24 * time.Hour,
	Delay:         5 * time.Second,
	DelayFunction: "constant",
	MaxDelay:      0,
	Unlimited:     false,
}

// Enabled returns true if rescheduling is allowed.
func (r *ReschedulePolicy) Enabled() bool {
	if r == nil {
		return false
	}
	return r.Unlimited || r.Attempts > 0
}

// Validate checks policy parameters, mirroring Nomad's validation minimal.
func (r *ReschedulePolicy) Validate() error {
	if r == nil || !r.Enabled() {
		return nil
	}
	if r.Delay < 5*time.Second {
		return fmt.Errorf("reschedule delay cannot be less than 5s (got %v)", r.Delay)
	}
	if r.DelayFunction != "constant" && r.DelayFunction != "exponential" && r.DelayFunction != "fibonacci" {
		return fmt.Errorf("invalid delay function %q, must be one of [constant exponential fibonacci]", r.DelayFunction)
	}
	if r.DelayFunction != "constant" {
		if r.MaxDelay < 5*time.Second {
			return fmt.Errorf("reschedule max delay cannot be less than 5s (got %v)", r.MaxDelay)
		}
		if r.MaxDelay < r.Delay {
			return fmt.Errorf("max delay %v cannot be less than delay %v", r.MaxDelay, r.Delay)
		}
	}
	if !r.Unlimited && r.Attempts > 0 && r.Interval < 15*time.Second {
		return fmt.Errorf("reschedule interval cannot be less than 15s (got %v)", r.Interval)
	}
	return nil
}

// NextDelay computes the delay for the given attempt (1-indexed) using the policy's delay function.
func (r *ReschedulePolicy) NextDelay(attempt int) time.Duration {
	if r == nil || attempt <= 0 {
		if r != nil {
			return r.Delay
		}
		return 30 * time.Second
	}
	switch r.DelayFunction {
	case "constant":
		return r.Delay
	case "exponential":
		// delay * 2^(attempt-1) capped at MaxDelay
		delay := float64(r.Delay) * math.Pow(2, float64(attempt-1))
		d := time.Duration(delay)
		if r.MaxDelay > 0 && d > r.MaxDelay {
			return r.MaxDelay
		}
		return d
	case "fibonacci":
		// fib sequence: 1,1,2,3,5,...
		if attempt == 1 || attempt == 2 {
			if r.MaxDelay > 0 && r.Delay > r.MaxDelay {
				return r.MaxDelay
			}
			return r.Delay
		}
		a, b := r.Delay, r.Delay
		var next time.Duration
		for i := 3; i <= attempt; i++ {
			next = a + b
			if r.MaxDelay > 0 && next > r.MaxDelay {
				// Once ceiling hit, switch to linear MaxDelay steps (Nomad behavior)
				next = b + r.MaxDelay
			}
			a, b = b, next
		}
		if next == 0 {
			next = b
		}
		return next
	default:
		return r.Delay
	}
}

// ShouldRetry determines if a failed instance should be retried given attempts and time since last failure.
// Returns (eligible bool, nextDelay time.Duration, reason string).
func (r *ReschedulePolicy) ShouldRetry(attempts int, lastFailure time.Time, now time.Time) (bool, time.Duration, string) {
	if r == nil || !r.Enabled() {
		return false, 0, "reschedule disabled"
	}
	if !r.Unlimited && r.Attempts > 0 && attempts >= r.Attempts {
		// Check if interval has elapsed to reset window (simplified: if lastFailure + Interval < now, allow)
		if now.Sub(lastFailure) < r.Interval {
			return false, 0, fmt.Sprintf("attempt cap %d reached within interval %v", r.Attempts, r.Interval)
		}
		// Interval passed — would reset, but we track attempts monotonically, so we still block until manually reset.
		// For minimal, we treat as blocked; successful placement will reset attempts.
		return false, 0, fmt.Sprintf("attempt cap %d exceeded", r.Attempts)
	}
	delay := r.NextDelay(attempts + 1) // next attempt
	elapsed := now.Sub(lastFailure)
	if elapsed < delay {
		return false, delay - elapsed, fmt.Sprintf("backoff %v remaining (delay %v)", delay-elapsed, delay)
	}
	return true, 0, ""
}
