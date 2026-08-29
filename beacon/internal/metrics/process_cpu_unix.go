//go:build unix

package metrics

import "golang.org/x/sys/unix"

// processCPUTimes returns the cumulative user and system CPU time consumed by
// the current process, in seconds, from getrusage.
func processCPUTimes() (userSeconds, systemSeconds float64) {
	var usage unix.Rusage
	if err := unix.Getrusage(unix.RUSAGE_SELF, &usage); err != nil {
		return 0, 0
	}
	return timevalSeconds(&usage.Utime), timevalSeconds(&usage.Stime)
}

func timevalSeconds(value *unix.Timeval) float64 {
	return float64(value.Sec) + float64(value.Usec)/1e6
}
