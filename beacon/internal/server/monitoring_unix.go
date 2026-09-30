package server

import (
	"time"
)

// daemonUptimeSeconds is the wall time since the daemon process started. It is
// distinct from host uptime: a machine that has run for weeks and a Beacon that
// restarted five seconds ago must not report one number and hide which one it
// was.
func daemonUptimeSeconds(started time.Time) int64 {
	if started.IsZero() {
		return -1
	}
	up := int64(time.Since(started).Seconds())
	if up < 0 {
		return -1
	}
	return up
}

// suspendDuration estimates how long the machine was suspended between started
// and now, as the difference between wall-clock elapsed and monotonic elapsed.
//
// Both clocks here are the ones Go already carries: time.Since uses the
// monotonic reading attached to started (CLOCK_MONOTONIC on Linux,
// CLOCK_UPTIME_RAW on Darwin — neither advances while the machine sleeps),
// while subtracting the wall fields measures real calendar time. Suspended
// time therefore appears in wall but not in monotonic, and the difference is
// the suspend estimate. An NTP step shows up the same way, which is why the
// series is labelled an estimate.
//
// Zero means "no evidence of suspend", and negative (a clock moved backwards)
// is reported as zero rather than pretending to know. Note that a started
// value without a monotonic reading — one that was deserialised or Round(0)ed
// — makes both measurements identical, so the result is 0: absence of a
// reading, not a measured zero.
func suspendDuration(started time.Time) time.Duration {
	if started.IsZero() {
		return 0
	}
	// Strip the monotonic reading from both sides so this is pure calendar time.
	wall := time.Now().Round(0).Sub(started.Round(0))
	mono := time.Since(started)
	diff := wall - mono
	if diff < 0 {
		return 0
	}
	return diff
}
