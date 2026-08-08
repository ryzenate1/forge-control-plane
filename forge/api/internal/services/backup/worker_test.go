package backup

import (
	"testing"
	"time"

	"gamepanel/forge/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPolicyDue_EmptyInterval(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	p := store.BackupPolicy{Interval: "", NextRunAt: &future}
	assert.False(t, policyDue(p, now), "policy without interval must never be due")
}

func TestPolicyDue_NilNextRunAt(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	p := store.BackupPolicy{Interval: "0 * * * *", NextRunAt: nil}
	assert.False(t, policyDue(p, now), "first tick must only initialize the schedule, not execute")
}

func TestPolicyDue_NotYetDue(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	future := now.Add(30 * time.Minute)
	p := store.BackupPolicy{Interval: "0 * * * *", NextRunAt: &future}
	assert.False(t, policyDue(p, now), "policy scheduled in the future must not be due")
}

func TestPolicyDue_DueExactlyNow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	p := store.BackupPolicy{Interval: "0 * * * *", NextRunAt: &now}
	assert.True(t, policyDue(p, now), "policy scheduled at exactly now must be due")
}

func TestPolicyDue_DueInPast(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	past := now.Add(-10 * time.Minute)
	p := store.BackupPolicy{Interval: "0 * * * *", NextRunAt: &past}
	assert.True(t, policyDue(p, past), "policy scheduled in the past must be due")
}

// TestPolicyDue_NextCronRunRoundTrip verifies the invariant the worker relies
// on: Schedule.Next always returns a strictly-greater time, so a policy is
// never due at the instant it is (re)scheduled, and becomes due once the
// scheduled time arrives.
func TestPolicyDue_NextCronRunRoundTrip(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	base := time.Date(2026, 8, 3, 10, 15, 0, 0, time.UTC)
	policy := store.BackupPolicy{Interval: "*/10 * * * *"}

	next, err := svc.NextCronRun(policy, base)
	require.NoError(t, err)
	assert.True(t, next.After(base), "next run must be strictly after base time")

	assert.False(t, policyDue(policy, base), "policy must not be due before the next run")
	due := store.BackupPolicy{Interval: policy.Interval, NextRunAt: &next}
	assert.True(t, policyDue(due, next), "policy must be due at the scheduled time")

	future := next.Add(-time.Second)
	assert.False(t, policyDue(due, future), "policy must not be due one second early")
}

func TestPolicyDue_DailySchedule(t *testing.T) {
	t.Parallel()
	svc := New(nil)
	base := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	policy := store.BackupPolicy{Interval: "0 0 * * *"}

	next, err := svc.NextCronRun(policy, base)
	require.NoError(t, err)
	want := time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, want, next, "hourly policy must next fire at midnight")

	p := store.BackupPolicy{Interval: policy.Interval, NextRunAt: &next}
	assert.True(t, policyDue(p, next))
	assert.False(t, policyDue(p, next.Add(-time.Minute)))
}
