package server

import (
	"time"

	"golang.org/x/sys/unix"
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
// A negative result means the clock moved backwards — reported as zero rather
// than pretending to know.
func suspendDuration(started time.Time) time.Duration {
	if started.IsZero() {
		return 0
	}
	wall := time.Since(started)
	var mono unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &mono); err != nil {
		return 0
	}
	monotonic := time.Duration(mono.Sec) * time.Second
	startMono := monotonic - wall
	if startMono < 0 {
		return 0
	}
	return startMono
}
